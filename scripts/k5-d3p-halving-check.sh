#!/usr/bin/env bash
# k5-d3p-halving-check.sh — why the D3′ minimizations stop at 22.5 s
# (evidence/K5/NOTES.md §10). Diagnosis only (ADR-0016): never a verdict. For
# every D3′ minimization candidate whose last reduction is halve-duration
# (full archives in runs/k5-full), prints the plan's duration, the F2a's
# injection, the last W1/W4 workload invoke (clients below 10; the recovery
# writes of clients 10–12 excluded) and the W1/W4 operations in flight at the
# kill, with the candidate's verdict from the campaign record.
# usage: k5-d3p-halving-check.sh [<campaign-stdout>] [<full-dir>]
set -euo pipefail
log=${1:-evidence/K5/logs/campaign-k5-d3p.stdout} full=${2:-runs/k5-full}
tmp=$(mktemp -d runs/.k5-halving-XXXXXX)
trap 'chmod -R u+w "$tmp" 2>/dev/null; rm -rf "$tmp"' EXIT
python3 -I -c 'import json, sys
for l in open(sys.argv[1]):
    e = json.loads(l)
    c = e.get("candidate") or {}
    if e.get("event") == "minimize-candidate" and c["reductions"] and c["reductions"][-1] == "halve-duration":
        print(c["run"], c["verdict"], len([r for r in c["reductions"] if r == "halve-duration"]))' "$log" >"$tmp/cands"
[ -s "$tmp/cands" ] || { echo "k5-d3p-halving-check: no halve-duration candidate in $log" >&2; exit 2; }
while read -r id verdict halvings; do
	a=$full/$id.tar.zst
	[ -f "$a" ] || { echo "k5-d3p-halving-check: no full archive of $id" >&2; exit 3; }
	zstd -dc "$a" | tar -C "$tmp" -xf - "$id/history.jsonl" "$id/plan.json"
	python3 -I - "$tmp/$id" "$verdict" "$halvings" <<'EOF'
import json, os, sys
d, verdict, halvings = sys.argv[1], sys.argv[2], int(sys.argv[3])
ev = [json.loads(l) for l in open(os.path.join(d, "history.jsonl"))]
plan = json.load(open(os.path.join(d, "plan.json")))
start = [e["t"] for e in ev if e["type"] == "marker" and e["value"].get("name") == "workload-start"][0]
s = lambda t: round((t - start) / 1e9, 3)
kills = [e["t"] for e in ev if e["type"] == "nemesis" and e["fault"]["kind"] == "F2a" and e["fault"]["phase"] == "inject"]
inv = {e["i"]: e for e in ev if e["type"] == "invoke"}
w = [e for e in inv.values() if e.get("f") in ("append", "insert") and e.get("client", 99) < 10]
done = {e["op"]: e for e in ev if e["type"] in ("ok", "fail", "info") and e.get("op") in inv}
flight = [i for i in w for k in kills[:1] if i["t"] < k and (i["i"] not in done or done[i["i"]]["t"] > k)]
print(json.dumps({"run": os.path.basename(d), "verdict": verdict, "halvings": halvings, "duration_ms": plan["duration_ms"],
                  "first_F2a_s": s(kills[0]) if kills else None, "last_workload_invoke_s": s(max(i["t"] for i in w)) if w else None,
                  "workload_ops_in_flight_at_first_F2a": len(flight)}, sort_keys=True))
EOF
	chmod -R u+w "$tmp/$id" && rm -rf "${tmp:?}/$id"
done <"$tmp/cands"
