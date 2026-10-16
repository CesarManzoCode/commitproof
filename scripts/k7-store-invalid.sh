#!/usr/bin/env bash
# k7-store-invalid.sh — stores the INVALID attempts of a K7 matrix cell that
# a HARNESS_DEFECT made INVALID (CONTRACT §11.1, §12.1, §15 K7.1: INVALID
# runs listed with their causes, never counted) as summaries in <dest>
# (evidence/K7/runs): stub (manifest, plan, seal) and checks.json, as
# ADR-0039 stores every run no claim names in full. Their verify-seal fails
# by construction (the defect is in the history's well-formedness), so the
# step checks instead that the seal root equals checks.json's, that the
# verdict is INVALID, and that every verify-seal problem matches <pattern>
# (the files' hashes and the chains hold). The full directories stay in
# <runs-dir> on the machine that ran them.
#
# usage: k7-store-invalid.sh <runs-dir> <dest> <pattern>
set -euo pipefail
dir=${1:?runs directory} dest=${2:?destination} pat=${3:?problem pattern}
mkdir -p "$dest"
n=0
for d in "$dir"/*/; do
	d=${d%/}
	id=$(basename "$d")
	[ -f "$d/seal.json" ] || { echo "k7-store-invalid: $id is not sealed" >&2; exit 2; }
	[ -f "$dest/$id.stub.tar.zst" ] && continue
	vs=$(bin/cpcheck verify-seal "$d" || true)
	root=$(jq -r .merkle_root "$d/seal.json")
	[ "$(echo "$vs" | jq -r .seal_root)" = "$root" ] && [ "$(jq -r .seal_root "$d.checks.json")" = "$root" ] ||
		{ echo "k7-store-invalid: $id: seal roots differ" >&2; exit 2; }
	[ "$(jq -r .verdict "$d.checks.json")" = INVALID ] || { echo "k7-store-invalid: $id is not INVALID" >&2; exit 2; }
	other=$(echo "$vs" | jq -r --arg p "$pat" '[.problems[] | select(test($p) | not)] | length')
	[ "$other" = 0 ] || { echo "k7-store-invalid: $id: $other verify-seal problems not matching $pat" >&2; exit 2; }
	tar -C "$dir" --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - "$id/manifest.json" "$id/plan.json" "$id/seal.json" |
		zstd -q -19 -T0 -o "$dest/$id.stub.tar.zst"
	cp "$d.checks.json" "$dest/$id.checks.json"
	n=$((n + 1))
	echo "{\"run\":\"$id\",\"stored\":\"summary\",\"seal_root\":\"$root\",\"verify_seal_problems\":$(echo "$vs" | jq '.problems | length')}"
done
echo "{\"command\":\"k7-store-invalid\",\"stored\":$n,\"exit_code\":0}"
