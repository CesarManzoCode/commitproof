#!/usr/bin/env bash
# k7-store-cited.sh — the cited D3′ and D5 real-build runs in full
# (ADR-0045 §5, ADR-0047; the owner's decision: preserve them). For each of
# the 10 runs (5 D3p, 5 D5, mutant real) that K5 committed as stubs in
# evidence/K5/runs, its full archive from <full-dir> (runs/k5-full, the
# machine that ran K5) is checked: manifest.json, plan.json and seal.json
# byte-identical to K5's stub, the seal verified by cpcheck on an extracted
# copy, its merkle root equal to K5's checks.json seal_root. Then the archive
# and K5's checks.json (unchanged) are copied into <dest>
# (evidence/K7/cited). K5's stubs are not touched. P25: the step refuses
# (exit 2, nothing copied) when the count after it would exceed 275 MB
# (every byte under evidence/ plus every run archive under findings/), and
# reports the count after it.
#
# usage: k7-store-cited.sh <full-dir> <dest>
set -euo pipefail
full=${1:?full archives directory} dest=${2:?destination}
budget=275000000 # P25 (ADR-0047)
stage=$(mktemp -d runs/.k7-cited-XXXXXX)
trap 'chmod -R u+w "$stage" 2>/dev/null; rm -rf "$stage"' EXIT
count() {
	local e f
	e=$(find evidence -type f -printf '%s\n' | awk '{s+=$1} END {print s+0}')
	f=$(find findings -name '*.tar.zst' -printf '%s\n' | awk '{s+=$1} END {print s+0}')
	echo $((e + f))
}
ids=()
for s in evidence/K5/runs/*-D3p-*-real-*.stub.tar.zst evidence/K5/runs/*-D5-*-real-*.stub.tar.zst; do
	ids+=("$(basename "$s" .stub.tar.zst)")
done
[ "${#ids[@]}" -eq 10 ] || { echo "k7-store-cited: ${#ids[@]} D3′/D5 real-build stubs in evidence/K5/runs, want 10" >&2; exit 2; }
add=0
for id in "${ids[@]}"; do
	a=$full/$id.tar.zst
	[ -f "$a" ] || { echo "k7-store-cited: $a missing" >&2; exit 2; }
	mkdir -p "$stage/full" "$stage/stub"
	zstd -dc "$a" | tar -C "$stage/full" -xf -
	zstd -dc "evidence/K5/runs/$id.stub.tar.zst" | tar -C "$stage/stub" -xf -
	for f in manifest.json plan.json seal.json; do
		cmp -s "$stage/full/$id/$f" "$stage/stub/$id/$f" || { echo "k7-store-cited: $id: $f differs from K5's stub" >&2; exit 2; }
	done
	bin/cpcheck verify-seal "$stage/full/$id" >/dev/null || { echo "k7-store-cited: $id: seal does not verify" >&2; exit 2; }
	root=$(jq -r .merkle_root "$stage/full/$id/seal.json")
	[ "$root" = "$(jq -r .seal_root "evidence/K5/runs/$id.checks.json")" ] || { echo "k7-store-cited: $id: seal root differs from K5's checks.json" >&2; exit 2; }
	chmod -R u+w "$stage/full" "$stage/stub" && rm -rf "${stage:?}/full" "${stage:?}/stub"
	[ -f "$dest/$id.tar.zst" ] && cmp -s "$a" "$dest/$id.tar.zst" && continue
	add=$((add + $(stat -c %s "$a") + $(stat -c %s "evidence/K5/runs/$id.checks.json")))
	echo "{\"run\":\"$id\",\"seal_root\":\"$root\",\"bytes\":$(stat -c %s "$a")}"
done
before=$(count)
if [ $((before + add)) -gt "$budget" ]; then
	echo "{\"command\":\"k7-store-cited\",\"error\":\"the P25 count would be $((before + add)) bytes, over $budget\",\"count_bytes\":$before,\"adding\":$add,\"exit_code\":2}"
	exit 2
fi
mkdir -p "$dest"
for id in "${ids[@]}"; do
	cp "$full/$id.tar.zst" "$dest/$id.tar.zst"
	cp "evidence/K5/runs/$id.checks.json" "$dest/$id.checks.json"
done
echo "{\"command\":\"k7-store-cited\",\"runs\":${#ids[@]},\"count_bytes_before\":$before,\"count_bytes\":$(count),\"budget_bytes\":$budget,\"exit_code\":0}"
