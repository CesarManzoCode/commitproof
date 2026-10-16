#!/usr/bin/env bash
# store-summaries.sh <runs-dir> <dest> [--measure-full] — ADR-0039's summary
# form of a run (CONTRACT §13): for every sealed run directory in
# <runs-dir>, <dest>/<run-id>.stub.tar.zst (manifest.json, plan.json,
# seal.json, as K4's and K5's stubs) and <dest>/<run-id>.checks.json (cpcheck's,
# which must carry the run's seal root). With --measure-full the run's full
# §13 archive is also built in a scratch directory and its size reported
# (not stored). One JSON line per run, then a summary.
set -euo pipefail
shopt -s nullglob
dir=${1:?runs dir} dest=${2:?destination} measure=${3:-}
mkdir -p "$dest"
scratch=$(mktemp -d runs/.store-summaries-XXXXXX)
trap 'rm -rf "$scratch"' EXIT
n=0 bytes=0
for d in "$dir"/*/; do
	d=${d%/}
	id=$(basename "$d")
	[ -f "$d/seal.json" ] || continue
	seal=$(jq -r .merkle_root "$d/seal.json")
	[ "$(jq -r .seal_root "$d.checks.json" 2>/dev/null)" = "$seal" ] || { echo "{\"run\":\"$id\",\"error\":\"checks.json missing or for another seal\",\"exit_code\":2}"; exit 2; }
	tar -C "$dir" --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - "$id/manifest.json" "$id/plan.json" "$id/seal.json" |
		zstd -q -19 -T0 -o "$dest/$id.stub.tar.zst"
	cp "$d.checks.json" "$dest/$id.checks.json"
	stub=$(stat -c %s "$dest/$id.stub.tar.zst") checks=$(stat -c %s "$dest/$id.checks.json")
	full=null
	if [ "$measure" = --measure-full ]; then
		tar -C "$dir" --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - "$id" | zstd -q -19 -T0 -o "$scratch/$id.tar.zst"
		full=$(stat -c %s "$scratch/$id.tar.zst")
		rm -f "$scratch/$id.tar.zst"
	fi
	n=$((n + 1)) bytes=$((bytes + stub + checks))
	echo "{\"run\":\"$id\",\"seal_root\":\"$seal\",\"verdict\":\"$(jq -r .verdict "$d.checks.json")\",\"stub_bytes\":$stub,\"checks_bytes\":$checks,\"full_archive_bytes\":$full}"
done
echo "{\"command\":\"store-summaries\",\"runs\":$n,\"bytes\":$bytes,\"dest\":\"$dest\",\"exit_code\":0}"
