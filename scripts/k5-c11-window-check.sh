#!/usr/bin/env bash
# k5-c11-window-check.sh — for the owner's decision of 2026-10-14, item 3
# (C11 seed 2, G12's literal rule): where the non-ok cp_steady operations of
# each storm window lie relative to the run's fault windows. Diagnosis only
# (ADR-0016): never a verdict; the run stays FAIL as C8 judged it. A fault
# window is the first to the last nemesis event of a fault id (§10.9's
# notion, ADR-0030); a storm window is C8's (§10.9; internal/checker/c8c9.go):
# the first invoke to the last completion of one phase's storm operations. Prints
# per storm window: cp_steady operations invoked in it, ok, non-ok, and how
# many non-ok were invoked inside a fault window and completed by its end;
# and the ok fraction with and without the operations invoked inside fault
# windows (G12's rule is the first; LIM-25).
# usage: k5-c11-window-check.sh <run-id> [<archive-dir>]
set -euo pipefail
id=${1:?run id} dir=${2:-evidence/K5/runs}
a=$dir/$id.tar.zst
[ -f "$a" ] || a=runs/k5-full/$id.tar.zst
[ -f "$a" ] || { echo "k5-c11-window-check: no full archive of $id" >&2; exit 3; }
tmp=$(mktemp -d runs/.k5-c11-XXXXXX)
trap 'chmod -R u+w "$tmp" 2>/dev/null; rm -rf "$tmp"' EXIT
zstd -dc "$a" | tar -C "$tmp" -xf - "$id/history.jsonl" "$id/plan.json"
echo "{\"archive\":\"$a\",\"archive_sha256\":\"$(sha256sum "$a" | cut -d' ' -f1)\"}"
python3 -I - "$tmp/$id" <<'EOF'
import json, os, sys
d = sys.argv[1]
ev = [json.loads(l) for l in open(os.path.join(d, "history.jsonl"))]
plan = json.load(open(os.path.join(d, "plan.json")))
start = [e["t"] for e in ev if e["type"] == "marker" and e["value"].get("name") == "workload-start"][0]
faults = {}
for e in ev:
    if e["type"] == "nemesis":
        f = faults.setdefault(e["fault"]["id"], {"kind": e["fault"]["kind"], "first": e["t"], "last": e["t"]})
        f["last"] = e["t"]
steady = {c["client"] for c in plan["clients"] if c.get("role") == "cp_steady"}
inv, ops, win = {}, [], {}
for e in ev:
    if e["type"] == "invoke":
        inv[e["i"]] = e
    elif e["type"] in ("ok", "fail", "info") and e.get("op") in inv:
        i = inv[e["op"]]
        if i.get("client") in steady:
            ops.append((i, e))
        v = i.get("value") if isinstance(i.get("value"), dict) else {}
        if v.get("role") == "cp_storm":
            a, b = win.get(v["phase"], (i["t"], e["t"]))
            win[v["phase"]] = (min(a, i["t"]), max(b, e["t"]))
phases = [win[k] for k in sorted(win)]
def in_fault(t):
    return [k for k, f in faults.items() if f["first"] <= t <= f["last"]]
s = lambda t: round((t - start) / 1e9, 3)
print(json.dumps({"fault_windows": {k: {"kind": f["kind"], "from_s": s(f["first"]), "to_s": s(f["last"])} for k, f in sorted(faults.items())}}, sort_keys=True))
for n, (a, b) in enumerate(phases, 1):
    w = [(i, c) for i, c in ops if a <= i["t"] < b]
    ok = [x for x in w if x[1]["type"] == "ok"]
    bad = [x for x in w if x[1]["type"] != "ok"]
    inside = [x for x in bad if in_fault(x[0]["t"])]
    done_inside = [x for x in inside if any(faults[k]["first"] <= x[1]["t"] <= faults[k]["last"] for k in in_fault(x[0]["t"]))]
    outside = [x for x in w if not in_fault(x[0]["t"])]
    outside_ok = [x for x in outside if x[1]["type"] == "ok"]
    print(json.dumps({"storm_window": n, "from_s": s(a), "to_s": s(b), "steady_ops": len(w), "ok": len(ok), "non_ok": len(bad),
                      "non_ok_invoked_inside_a_fault_window": len(inside), "of_those_completed_inside_it": len(done_inside),
                      "non_ok_invoked_s": [s(min(x[0]["t"] for x in bad)), s(max(x[0]["t"] for x in bad))] if bad else None,
                      "ok_fraction": round(len(ok) / len(w), 4) if w else None,
                      "ops_invoked_outside_fault_windows": len(outside), "ok_of_those": len(outside_ok),
                      "ok_fraction_outside_fault_windows": round(len(outside_ok) / len(outside), 4) if outside else None}, sort_keys=True))
EOF
