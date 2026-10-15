#!/usr/bin/env bash
# k6-stage-reference.sh <run archive> <dest dir> — copies the K6 reference
# run (ADR-0036) sealed: its zstd tar and cpcheck's checks.json, byte for
# byte, then verifies the copy's seal with cpcheck. Prints one JSON summary
# line; exit 0 only if both copies are identical to their sources and the
# seal verifies.
set -u -o pipefail
src=${1:?run archive} dest=${2:?dest dir}
id=$(basename "$src" .tar.zst)
checks="$(dirname "$src")/$id.checks.json"
mkdir -p "$dest"
for f in "$dest/$id.tar.zst" "$dest/$id.checks.json"; do
	[ -e "$f" ] && { echo "{\"command\":\"k6-stage-reference\",\"error\":\"$(basename "$f") already staged\",\"exit_code\":3}"; exit 3; }
done
cp "$src" "$dest/$id.tar.zst" && cp "$checks" "$dest/$id.checks.json" || exit 3
tmp=$(mktemp -d)
trap 'chmod -R u+w "$tmp"; rm -rf "$tmp"' EXIT
zstd -q -dc "$dest/$id.tar.zst" | tar -x -C "$tmp"
vs=$(bin/cpcheck verify-seal "$tmp/$id" | tail -n 1)
h() { sha256sum "$1" | cut -d' ' -f1; }
a=$(h "$src") b=$(h "$dest/$id.tar.zst") c=$(h "$checks") d=$(h "$dest/$id.checks.json")
valid=$(jq -r .valid <<<"$vs") root=$(jq -r .seal_root <<<"$vs")
croot=$(jq -r .seal_root "$dest/$id.checks.json")
rc=0
[ "$a" = "$b" ] && [ "$c" = "$d" ] && [ "$valid" = true ] && [ "$root" = "$croot" ] || rc=1
jq -cn --arg id "$id" --arg src "$src" --arg a "$a" --arg b "$b" --arg c "$c" --arg d "$d" --arg valid "$valid" --arg root "$root" --arg croot "$croot" --argjson rc "$rc" \
	'{command:"k6-stage-reference",run_id:$id,source:$src,archive_sha256:$a,copy_sha256:$b,checks_sha256:$c,checks_copy_sha256:$d,seal_valid:($valid=="true"),seal_root:$root,checks_seal_root:$croot,identical:($a==$b and $c==$d),exit_code:$rc}'
exit $rc
