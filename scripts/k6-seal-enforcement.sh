#!/usr/bin/env bash
# k6-seal-enforcement.sh <reference run archive> — K6.4 evidence (CONTRACT
# §15 K6, ADR-0036): a run whose seal does not verify is refused by
# `cpreport render` with exit 2 and nothing is written. Cases, each on a
# copy outside the evidence:
#   control   the untampered run directory (with its checks.json): renders;
#   dir       the same directory with one byte of history.jsonl flipped
#             (the middle byte, as cpgate's K1.7 control does): refused;
#   archive   that tampered directory re-archived as <id>.tar.zst: refused.
# For each case the output directory is a fresh path that does not exist
# before the render; afterwards its existence and file count are recorded,
# and cpcheck verify-seal's verdict on the input. Prints cpreport's lines,
# then one JSON summary line. No temporary path is printed.
set -u -o pipefail
arch=${1:?reference run archive}
id=$(basename "$arch" .tar.zst)
checks="$(dirname "$arch")/$id.checks.json"
tmp=$(mktemp -d)
trap 'chmod -R u+w "$tmp"; rm -rf "$tmp"' EXIT
mkdir -p "$tmp/src" "$tmp/control" "$tmp/dir" "$tmp/archive"
zstd -q -dc "$arch" | tar -x -C "$tmp/src" || exit 3
cp -r "$tmp/src/$id" "$tmp/control/$id" && cp "$checks" "$tmp/control/$id.checks.json"
cp -r "$tmp/src/$id" "$tmp/dir/$id" && cp "$checks" "$tmp/dir/$id.checks.json"
f="$tmp/dir/$id/history.jsonl"
chmod u+w "$f"
off=$(( $(stat -c %s "$f") / 2 ))
b=$(dd if="$f" bs=1 skip="$off" count=1 2>/dev/null | od -An -tu1 | tr -d ' ')
printf "\\$(printf %03o $(( b ^ 1 )))" | dd of="$f" bs=1 seek="$off" count=1 conv=notrunc 2>/dev/null
chmod u-w "$f"
tar -C "$tmp/dir" -c "$id" | zstd -q -o "$tmp/archive/$id.tar.zst" && cp "$checks" "$tmp/archive/$id.checks.json"
cases=()
one() { # name tamper expect input verify-dir
	local name=$1 tamper=$2 expect=$3 in=$4 vdir=$5 out="$tmp/out-$1" line code exists files vs
	bin/cpreport render "$in" --out "$out" >"$tmp/stdout-$name"
	code=$?
	line=$(tail -n 1 "$tmp/stdout-$name")
	echo "$line"
	exists=false files=0
	if [ -e "$out" ]; then exists=true; files=$(find "$out" -type f | wc -l); fi
	vs=$(bin/cpcheck verify-seal "$vdir" | tail -n 1 | jq -r .valid)
	cases+=("$(jq -cn --arg name "$name" --arg tamper "$tamper" --arg expect "$expect" --argjson code "$code" \
		--arg err "$(jq -r '.error // ""' <<<"$line")" --argjson exists "$exists" --argjson files "$files" --arg vs "$vs" \
		'{name:$name,tamper:$tamper,expect:$expect,cpreport_exit:$code,cpreport_error:$err,out_exists:$exists,out_files:$files,verify_seal_valid:$vs}')")
}
one control "none" rendered "$tmp/control/$id" "$tmp/control/$id"
one dir "history.jsonl byte $off xor 1" refused "$tmp/dir/$id" "$tmp/dir/$id"
one archive "history.jsonl byte $off xor 1, re-archived" refused "$tmp/archive/$id.tar.zst" "$tmp/dir/$id"
printf '%s\n' "${cases[@]}" | jq -cs --arg id "$id" '{command:"k6-seal-enforcement",run_id:$id,cases:.,exit_code:0}'
