#!/usr/bin/env bash
# compress-runs.sh — CONTRACT §13 local evidence policy: every sealed run
# directory under <dir> becomes <dir>/<run-id>.tar.zst (still sealed by the
# hashes of its uncompressed files; cpgate reads the archive), then the
# directory is removed. Entries sorted, owner 0, mtime 0; modes kept.
set -euo pipefail
dir=${1:?runs directory}
for d in "$dir"/*/; do
	d=${d%/}
	id=$(basename "$d")
	[ -f "$d/seal.json" ] || { echo "compress-runs: $d is not sealed" >&2; exit 2; }
	bin/cpcheck verify-seal "$d" >/dev/null
	tar -C "$dir" --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - "$id" | zstd -q -19 -T0 -o "$dir/$id.tar.zst"
	chmod -R u+w "$d" && rm -rf "${d:?}"
	echo "{\"run\":\"$id\",\"archive\":\"$dir/$id.tar.zst\",\"bytes\":$(stat -c %s "$dir/$id.tar.zst")}"
done
