#!/usr/bin/env bash
# k6b-cited-recheck.sh — owner decision 2 (K6b): K7 cites K4's and K5's
# directed cells instead of re-running them. Every valid primary run of
# those cells (D1 on M1 and real, D2′ on M2 and real, D3′ on M3 and real,
# the probe suite on M4 and real, D5 on M5 and real, DH1 on h1) is
# re-checked with this commit's cpcheck and its checks.json compared byte
# for byte with the recorded one (evidence/K4|K5/runs/<id>.checks.json):
# identical means the current checkers give the cited runs the verdicts
# and classes recorded under their own contract versions. Full archives
# come from evidence/ (committed in full) or, for stubs, from the local
# runs/k4-full/ and runs/k5-full/ (LIM-30, LIM-37). Scratch in runs/.k6b.
set -u -o pipefail
S=runs/.k6b/cited-recheck
if [ -d "$S" ]; then chmod -R u+w "$S" && rm -rf "${S:?}"; fi
mkdir -p "$S"
rc=0 n=0 same=0
for k in K4 K5; do
	ids=$(jq -r '.runs[] | select(.kind=="primary" and .verdict!="INVALID" and (.campaign=="D1" or .campaign=="D2p" or .campaign=="D3p" or .campaign=="claims" or .campaign=="D5" or .campaign=="DH1")) | .run_id' "evidence/$k/results.json")
	full=runs/$(echo "$k" | tr K k)-full
	for id in $ids; do
		checks=evidence/$k/runs/$id.checks.json
		arch=evidence/$k/runs/$id.tar.zst src=evidence
		[ -f "$arch" ] || { arch=$full/$id.tar.zst src=$full; }
		if [ ! -f "$arch" ] || [ ! -f "$checks" ]; then
			echo "{\"run\":\"$id\",\"error\":\"no full archive or no recorded checks.json\"}"
			rc=1
			continue
		fi
		zstd -q -dc "$arch" | tar -x -C "$S" || { echo "{\"run\":\"$id\",\"error\":\"extract\"}"; rc=1; continue; }
		bin/cpcheck check "$S/$id" >"$S/$id.out" 2>"$S/$id.err"
		code=$?
		n=$((n + 1))
		if cmp -s "$S/$id.checks.json" "$checks"; then
			same=$((same + 1))
			ident=true
		else
			ident=false
			rc=1
		fi
		verdict=$(sed -n 's/.*"verdict":"\([A-Z]*\)".*/\1/p' "$S/$id.out" | tail -n 1)
		cv=$(jq -r '.contract_version' "$S/$id/plan.json")
		echo "{\"sprint\":\"$k\",\"run\":\"$id\",\"plan_contract_version\":\"$cv\",\"archive\":\"$src\",\"cpcheck_exit\":$code,\"verdict\":\"$verdict\",\"checks_identical\":$ident,\"recorded_sha256\":\"$(sha256sum "$checks" | cut -d' ' -f1)\"}"
		chmod -R u+w "$S/$id" && rm -rf "${S:?}/$id"
	done
done
echo "{\"command\":\"k6b-cited-recheck\",\"runs\":$n,\"identical\":$same,\"exit_code\":$rc}"
exit $rc
