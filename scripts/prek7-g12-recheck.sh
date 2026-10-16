#!/usr/bin/env bash
# prek7-g12-recheck.sh — ADR-0037: no recorded run changes verdict. Every
# recorded K5 C11 run (primaries, minimization candidates, replays) is
# re-checked with this commit's cpcheck and its checks.json compared byte
# for byte with the recorded evidence/K5/runs/<id>.checks.json. Full
# archives come from evidence/K5/runs/ (committed in full) or, for stubs,
# from the local runs/k5-full/ (LIM-37). Scratch output in runs/.prek7.
set -u -o pipefail
S=runs/.prek7/g12-recheck
if [ -d "$S" ]; then chmod -R u+w "$S" && rm -rf "${S:?}"; fi
mkdir -p "$S"
rc=0 n=0 same=0
for checks in evidence/K5/runs/*-C11-*.checks.json; do
	id=$(basename "$checks" .checks.json)
	arch=evidence/K5/runs/$id.tar.zst src=evidence
	[ -f "$arch" ] || { arch=runs/k5-full/$id.tar.zst src=runs/k5-full; }
	if [ ! -f "$arch" ]; then
		echo "{\"run\":\"$id\",\"error\":\"no full archive\"}"
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
	echo "{\"run\":\"$id\",\"archive\":\"$src\",\"cpcheck_exit\":$code,\"verdict\":\"$verdict\",\"checks_identical\":$ident,\"recorded_sha256\":\"$(sha256sum "$checks" | cut -d' ' -f1)\"}"
done
echo "{\"command\":\"prek7-g12-recheck\",\"runs\":$n,\"identical\":$same,\"exit_code\":$rc}"
exit $rc
