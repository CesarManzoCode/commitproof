#!/usr/bin/env bash
# k5-compress-measure.sh — the owner's first step for K5 under P25 (decision
# of 2026-10-14 on ADR-0032): try lossless compression of the K5 runs before
# raising the budget. Measures, on a staged ADR-0030 copy (k5-stage-storage.sh,
# outside evidence/), what stronger lossless compression of the same bytes
# would save: every full archive and every stub recompressed with
# `zstd --ultra -22 --long=27` (a format cpgate's decoder reads: .tar.zst,
# window 128 MiB), xz -9e for reference (a format cpgate does not read), and
# all stubs together as one solid archive (also a format cpgate does not
# read). Nothing under evidence/ is changed. Prints one JSON line per full
# archive and a summary with the bytes evidence/ would hold in git at the
# best measured compression and whether that fits P25 (200 MB).
# usage: k5-compress-measure.sh <work-dir> [<full-dir>]
set -euo pipefail
work=${1:?work directory} full=${2:-runs/k5-full}
case "$(realpath -m "$work")/" in "$(realpath -m evidence)/"*) echo "k5-compress-measure: $work is under evidence/" >&2; exit 3; esac
mkdir -p "$work"
stage=$work/staged
summary=$(scripts/k5-stage-storage.sh adr0030 "$stage" "$full" | tail -n 1)
others=$(sed -n 's/.*"evidence_outside_k5_bytes":\([0-9]*\).*/\1/p' <<<"$summary")
k5=$(sed -n 's/.*"k5_bytes":\([0-9]*\).*/\1/p' <<<"$summary")
[ -n "$others" ] && [ -n "$k5" ] || { echo "k5-compress-measure: no staging summary" >&2; exit 2; }
z22() { zstd -dc "$1" | zstd -q --ultra -22 --long=27 -T0 -c | wc -c; }
xz9() { zstd -dc "$1" | xz -9e -T0 -c | wc -c; }
f19=0 f22=0 fxz=0 n=0
for a in "$stage"/K5/runs/*.tar.zst; do
	case "$a" in *.stub.tar.zst) continue ;; esac
	id=$(basename "$a" .tar.zst)
	s19=$(stat -c %s "$a") s22=$(z22 "$a") sxz=$(xz9 "$a")
	f19=$((f19 + s19)) f22=$((f22 + s22)) fxz=$((fxz + sxz)) n=$((n + 1))
	echo "{\"run\":\"$id\",\"zstd19_bytes\":$s19,\"zstd22_long27_bytes\":$s22,\"xz9e_bytes\":$sxz}"
done
s19=0 s22=0 m=0
solid=$work/stubs
rm -rf "$solid" && mkdir -p "$solid"
for a in "$stage"/K5/runs/*.stub.tar.zst; do
	s19=$((s19 + $(stat -c %s "$a"))) s22=$((s22 + $(z22 "$a"))) m=$((m + 1))
	zstd -dc "$a" | tar -C "$solid" -xf -
done
ssolid=$(tar -C "$solid" --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - . | zstd -q --ultra -22 --long=27 -T0 -c | wc -c)
chmod -R u+w "$solid" && rm -rf "${solid:?}"
best=$((k5 - (f19 - fxz) - (s19 - ssolid)))
echo "{\"command\":\"k5-compress-measure\",\"full_archives\":$n,\"full_zstd19_bytes\":$f19,\"full_zstd22_long27_bytes\":$f22,\"full_xz9e_bytes\":$fxz,\"stubs\":$m,\"stubs_zstd19_bytes\":$s19,\"stubs_zstd22_long27_bytes\":$s22,\"stubs_solid_zstd22_long27_bytes\":$ssolid,\"evidence_outside_k5_bytes\":$others,\"k5_staged_bytes\":$k5,\"k5_best_measured_bytes\":$best,\"evidence_best_measured_bytes\":$((others + best)),\"budget\":200000000,\"fits\":$([ $((others + best)) -le 200000000 ] && echo true || echo false),\"exit_code\":0}"
