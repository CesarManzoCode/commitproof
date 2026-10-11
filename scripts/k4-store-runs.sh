#!/usr/bin/env bash
# k4-store-runs.sh — CONTRACT §13 local evidence policy for K4 (P25). Every
# sealed run directory under <dir> is archived as <dir>/<run-id>.tar.zst
# (zstd tar; still sealed by the hashes of its uncompressed files; cpgate
# reads the archive) — except derived runs (minimization candidates and
# replays) of mutant and H1 failures, which are summarized: <run-id>.stub.tar.zst
# holds manifest.json, plan.json and seal.json (cpgate checks them against
# the seal and checks.json), and the full archive goes to <local>/ (git-
# ignored, kept for re-checking on this machine). Primary runs and every
# run of a real-build failure are committed in full.
# usage: k4-store-runs.sh <runs-dir> <local-full-dir>
set -euo pipefail
dir=${1:?runs directory} local=${2:?local directory for full archives of stubbed runs}
mkdir -p "$local"
field() { sed -n "s/.*\"$2\":\"\\([^\"]*\\)\".*/\\1/p" "$1/manifest.json"; }
for d in "$dir"/*/; do
	d=${d%/}
	id=$(basename "$d")
	case "$id" in *.elle) continue ;; esac
	[ -f "$d/seal.json" ] || { echo "k4-store-runs: $d is not sealed" >&2; exit 2; }
	bin/cpcheck verify-seal "$d" >/dev/null
	tar -C "$dir" --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - "$id" | zstd -q -19 -T0 -o "$dir/$id.tar.zst.tmp"
	derived=no
	grep -q '"replay_of"\|"candidate_of"' "$d/manifest.json" && derived=yes
	if [ "$derived" = yes ] && { [ "$(field "$d" mutant)" != real ] || [ "$(field "$d" profile)" = h1 ]; }; then
		mv "$dir/$id.tar.zst.tmp" "$local/$id.tar.zst"
		tar -C "$dir" --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - "$id/manifest.json" "$id/plan.json" "$id/seal.json" |
			zstd -q -19 -T0 -o "$dir/$id.stub.tar.zst"
		kind=stub
	else
		mv "$dir/$id.tar.zst.tmp" "$dir/$id.tar.zst"
		kind=full
	fi
	chmod -R u+w "$d" && rm -rf "${d:?}"
	f="$dir/$id.tar.zst"
	[ "$kind" = stub ] && f="$dir/$id.stub.tar.zst"
	echo "{\"run\":\"$id\",\"stored\":\"$kind\",\"archive\":\"$f\",\"bytes\":$(stat -c %s "$f")}"
done
