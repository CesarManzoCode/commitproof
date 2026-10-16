#!/usr/bin/env bash
# k6b-exposure-check.sh <run-dir>... — ADR-0044 on real runs: for each run
# (a plan of contract 1.7.0 or later) the exposure C9 wrote into its
# checks.json equals an independent computation from its history.jsonl with
# ADR-0036's definitions (jq, not the checker's code). Exit 1 if any run
# lacks the field or differs.
set -u -o pipefail
rc=0 n=0
for d in "$@"; do
	d=${d%/}
	n=$((n + 1))
	got=$(jq -c '.checkers[] | select(.id=="C9") | .stats.exposure // empty' "$d.checks.json")
	want=$(jq -sc '
		(map(select(.type=="invoke")) | map({key: (.i|tostring), value: .}) | from_entries) as $inv
		| {operations: (map(select(.type=="invoke" and .f != "final-read")) | length),
		   ok_writes: (map(select(.type=="ok") | $inv[(.op|tostring)] | select(.f != "final-read")
				| if (.wl=="ledger" and .f=="insert") or (.wl=="register" and (.f=="write" or .f=="cas")) or (.wl=="bank" and .f=="transfer") then 1
				  elif .wl=="append" then ([.value[]? | select(type=="array" and length==3 and .[0]=="append")] | length)
				  else 0 end) | add // 0),
		   confirmed_faults: (map(select(.type=="nemesis" and .fault.phase=="confirmed") | .fault.kind) | group_by(.) | map({key: .[0], value: length}) | from_entries),
		   fault_time_ns: (reduce (map(select(.type=="nemesis"))[]) as $e ({c: {}, t: 0};
				if $e.fault.phase=="confirmed" then .c[$e.fault.id] = $e.t
				elif $e.fault.phase=="heal" and (.c[$e.fault.id] != null) then .t += ($e.t - .c[$e.fault.id])
				else . end) | .t)}' "$d/history.jsonl")
	same=false
	if [ -n "$got" ] && [ "$(jq -S -c . <<<"$got")" = "$(jq -S -c . <<<"$want")" ]; then same=true; else rc=1; fi
	echo "{\"run\":\"$(basename "$d")\",\"plan_contract_version\":\"$(jq -r .contract_version "$d/plan.json")\",\"checks_json\":${got:-null},\"independent\":$want,\"equal\":$same}"
done
echo "{\"command\":\"k6b-exposure-check\",\"runs\":$n,\"exit_code\":$rc}"
exit $rc
