#!/usr/bin/env bash
# prek7-backup-manifest.sh <out.tsv> — the backup manifest of the K4 and K5
# full run archives kept only on this machine (git-ignored runs/k4-full and
# runs/k5-full; LIM-30, LIM-37): one line per archive with its path, size in
# bytes, sha256, and whether the same file is committed in full under
# evidence/K4/runs or evidence/K5/runs ("in-git" when an identical archive
# is committed, else "local-only"). Prints the TSV and a JSON summary line.
set -u -o pipefail
out=${1:?out tsv}
printf 'path\tbytes\tsha256\tcopy\n' >"$out"
local_only=0 in_git=0 bytes=0 lbytes=0
for d in runs/k4-full runs/k5-full; do
	k=$([ "$d" = runs/k4-full ] && echo K4 || echo K5)
	for f in "$d"/*.tar.zst; do
		[ -f "$f" ] || continue
		sz=$(stat -c %s "$f") h=$(sha256sum "$f" | cut -d' ' -f1)
		g=evidence/$k/runs/$(basename "$f")
		copy=local-only
		if [ -f "$g" ] && [ "$(sha256sum "$g" | cut -d' ' -f1)" = "$h" ]; then copy=in-git; in_git=$((in_git + 1)); else local_only=$((local_only + 1)); lbytes=$((lbytes + sz)); fi
		bytes=$((bytes + sz))
		printf '%s\t%s\t%s\t%s\n' "$f" "$sz" "$h" "$copy" >>"$out"
	done
done
cat "$out"
printf '{"command":"prek7-backup-manifest","archives":%d,"bytes":%d,"local_only":%d,"local_only_bytes":%d,"in_git":%d,"manifest":"%s","manifest_sha256":"%s","exit_code":0}\n' \
	"$((local_only + in_git))" "$bytes" "$local_only" "$lbytes" "$in_git" "$out" "$(sha256sum "$out" | cut -d' ' -f1)"
