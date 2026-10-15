#!/usr/bin/env bash
# k5-g10-listener-check.sh — the harness review of G10 the owner asked for
# (decision of 2026-10-14, item 4): is the W6 listener the cause of the lost
# notifications? Diagnosis only (ADR-0016): never a verdict. Prints
#   1. for every C9 run (primaries, INVALID attempts, minimization candidates,
#      replays; full archives in runs/k5-full), per listener client: its
#      sessions (one LISTEN each), ok and failed waits, notifications
#      received in all and after the run's first fault, and a summary;
#   2. the unit test of the listener's reconnect path
#      (TestListenerRelistensAfterReconnect);
#   3. the listener code (runner.runListener, workload.Listener) and the
#      pinned gateway code that keeps or rebuilds a session's notification
#      stream (notification_adapter.go at e3cbdbb).
# usage: k5-g10-listener-check.sh [<full-dir>] [<source-dir>]
set -euo pipefail
shopt -s nullglob
full=${1:-runs/k5-full} src=${2:-.cache/multigres}
[ -d "$full" ] || { echo "k5-g10-listener-check: no $full" >&2; exit 3; }
[ "$(git -C "$src" rev-parse HEAD)" = e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73 ] || { echo "k5-g10-listener-check: $src is not at the pinned commit" >&2; exit 3; }
tmp=$(mktemp -d runs/.k5-g10-XXXXXX)
trap 'chmod -R u+w "$tmp" 2>/dev/null; rm -rf "$tmp"' EXIT
n=0
for a in "$full"/*-C9-*.tar.zst; do
	id=$(basename "$a" .tar.zst)
	zstd -dc "$a" | tar -C "$tmp" -xf - "$id/history.jsonl" "$id/manifest.json"
	n=$((n + 1))
done
[ "$n" -gt 0 ] || { echo "k5-g10-listener-check: no C9 run in $full" >&2; exit 2; }
cat >"$tmp/listen.py" <<'EOF'
import json, os, sys
LISTENERS = (26, 27, 28)
tot = {"runs": 0, "runs_with_faults": 0, "sessions": 0, "reconnects": 0,
       "failed_waits_after_first_fault": 0, "received_after_first_fault": 0,
       "fault_free_runs": 0, "received_in_fault_free_runs": 0}
for d in sorted(x.rstrip("/") for x in sys.argv[1:]):
    ev = [json.loads(l) for l in open(os.path.join(d, "history.jsonl"))]
    man = json.load(open(os.path.join(d, "manifest.json")))
    kind = "replay" if man.get("replay_of") else "candidate" if man.get("candidate_of") else "primary"
    faults = [e["t"] for e in ev if e["type"] == "nemesis"]
    ff = min(faults) if faults else None
    inv, ses = {}, {}
    for e in ev:
        if e["type"] == "invoke":
            inv[e["i"]] = e
            continue
        if e["type"] not in ("ok", "fail", "info"):
            continue
        i = inv.get(e.get("op"))
        if not i or i.get("client") not in LISTENERS or i.get("f") not in ("listen", "wait"):
            continue
        s = ses.setdefault((i["client"], (i.get("value") or {}).get("conn")),
                           {"listen": None, "ok": 0, "failed": 0, "failed_after": 0, "recv": 0, "recv_after": 0})
        if i["f"] == "listen":
            s["listen"] = e["type"]
            continue
        if e["type"] != "ok":
            s["failed"] += 1
            if ff is not None and i["t"] > ff:
                s["failed_after"] += 1
            continue
        s["ok"] += 1
        p = (e.get("value") or {}).get("payloads") or []
        s["recv"] += len(p)
        if ff is not None and i["t"] > ff:
            s["recv_after"] += len(p)
    tot["runs"] += 1
    per = {}
    for (c, conn), s in ses.items():
        per.setdefault(c, []).append(s)
    for c, ss in per.items():
        tot["sessions"] += len(ss)
        tot["reconnects"] += len(ss) - 1
    if ff is None:
        tot["fault_free_runs"] += 1
        tot["received_in_fault_free_runs"] += sum(s["recv"] for s in ses.values())
    else:
        tot["runs_with_faults"] += 1
        tot["failed_waits_after_first_fault"] += sum(s["failed_after"] for s in ses.values())
        tot["received_after_first_fault"] += sum(s["recv_after"] for s in ses.values())
    print(json.dumps({"run": os.path.basename(d), "kind": kind, "first_fault_s": round(ff / 1e9, 3) if ff is not None else None,
                      "listeners": {str(c): [{"listen": s["listen"], "waits_ok": s["ok"], "waits_failed": s["failed"],
                                              "received": s["recv"], "received_after_first_fault": s["recv_after"]}
                                             for s in per[c]] for c in sorted(per)}}, sort_keys=True))
print(json.dumps({"summary": tot}, sort_keys=True))
EOF
echo "== 1. listener sessions in every C9 run ($n runs)"
python3 -I "$tmp/listen.py" "$tmp"/*/
echo "== 2. the listener's reconnect path (unit test)"
GOTOOLCHAIN=go1.26.9 go test -count=1 -v -run '^TestListenerRelistensAfterReconnect$' ./internal/workload
show() { # file first last
	echo "-- $1:$2-$3"
	sed -n "$2,$3p" "$1"
}
echo "== 3. code"
show internal/runner/k5.go 181 222
show internal/workload/k5.go 226 296
show "$src/go/services/multigateway/poolergateway/notification_adapter.go" 138 175
show "$src/go/services/multigateway/poolergateway/notification_adapter.go" 188 228
show "$src/go/services/multigateway/poolergateway/notification_adapter.go" 255 270
