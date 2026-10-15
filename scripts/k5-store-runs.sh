#!/usr/bin/env bash
# k5-store-runs.sh — CONTRACT §13 local evidence policy for K5 (P25,
# ADR-0030, ADR-0034). Every sealed run directory under <dir> is archived as
# a zstd tar (still sealed by the hashes of its uncompressed files; cpgate
# reads the archive). Committed in full (<run-id>.tar.zst): every C10 primary
# run (K5.3 reads its observations) and the primary and minimized runs of
# every real-build FAIL (K5.5's triage). Every other run — the other primary
# runs, minimization candidates and replays — is committed as a stub
# (<run-id>.stub.tar.zst: manifest.json, plan.json, seal.json; cpgate checks
# them against the seal and checks.json), its full archive kept under
# <local>/ on this machine (LIM-30).
#
# A run already committed as a stub that the policy keeps in full is
# replaced by its full archive from <local>/ (ADR-0034: the interim stubs of
# ADR-0032 become ADR-0030's storage). The archive must carry the stub's
# manifest.json, plan.json and seal.json byte for byte; the copy in <local>/
# is kept. The selection reads each run's manifest (from its directory or its
# stub), its checks.json and its minimization journal.
#
# The step refuses (exit 2, nothing moved into evidence) when the result
# would make evidence/ exceed P25 (250 MB, ADR-0034; counted as bytes/10⁶ of
# the files under evidence/).
#
# Policy (third argument): adr0030 (default) as above; adr0032-option2 —
# ADR-0032's option 2 — the same, except that a C10 primary run that is not
# a real-build FAIL is a stub; stubs — every run a stub (ADR-0032's interim;
# no stub is replaced). The budget check applies when <runs-dir> is under
# evidence/; a staged copy elsewhere (for evaluating the gate on another
# storage) is not evidence and is not counted.
# usage: k5-store-runs.sh <runs-dir> <local-full-dir> [adr0030|adr0032-option2|stubs]
set -euo pipefail
shopt -s nullglob
dir=${1:?runs directory} local=${2:?local directory for full archives of stubbed runs}
policy=${3:-adr0030}
case "$policy" in adr0030 | adr0032-option2 | stubs) ;; *) echo "k5-store-runs: unknown policy $policy" >&2; exit 3 ;; esac
budget=250000000
under_evidence=no
case "$(realpath -m "$dir")/" in "$(realpath -m evidence)/"*) under_evidence=yes ;; esac
mkdir -p "$local"
stage=$(mktemp -d runs/.k5-store-XXXXXX)
trap 'chmod -R u+w "$stage" 2>/dev/null; rm -rf "$stage"' EXIT
field() { sed -n "s/.*\"$1\":\"\\([^\"]*\\)\".*/\\1/p"; } # key; manifest on stdin
manifest() { # run id → its manifest.json, from its directory or its stub
	if [ -f "$dir/$1/manifest.json" ]; then
		cat "$dir/$1/manifest.json"
	else
		zstd -dc "$dir/$1.stub.tar.zst" | tar -xOf - "$1/manifest.json"
	fi
}
# Runs kept in full: C10 primaries; real-build FAIL primaries and their
# minimized runs (from the minimization journals).
declare -A full=()
ids=()
for d in "$dir"/*/; do
	d=${d%/}
	[ -f "$d/seal.json" ] && ids+=("$(basename "$d")")
done
for s in "$dir"/*.stub.tar.zst; do ids+=("$(basename "$s" .stub.tar.zst)"); done
for id in "${ids[@]}"; do
	m=$(manifest "$id")
	camp=$(field campaign <<<"$m") mut=$(field mutant <<<"$m")
	[ -n "$camp" ] && [ -n "$mut" ] || { echo "k5-store-runs: $id: no campaign or mutant in its manifest" >&2; exit 2; }
	grep -q '"replay_of"\|"candidate_of"' <<<"$m" && continue
	[ "$camp" = C10 ] && [ "$policy" = adr0030 ] && full[$id]=1
	if [ "$mut" = real ] && grep -q '"verdict":"FAIL"' "$dir/$id.checks.json" 2>/dev/null; then
		full[$id]=1
		j="$dir/$id.minimize.json"
		[ -f "$j" ] || { echo "k5-store-runs: real-build FAIL $id has no minimization journal" >&2; exit 2; }
		mr=$(sed -n 's/.*"minimized_run":"\([^"]*\)".*/\1/p' "$j")
		[ -n "$mr" ] && full[$mr]=1
	fi
done
[ "$policy" = stubs ] && full=()
# Archive into the stage; nothing in evidence changes until the budget check.
out=()
for d in "$dir"/*/; do
	d=${d%/}
	id=$(basename "$d")
	case "$id" in *.elle) continue ;; esac
	[ -f "$d/seal.json" ] || { echo "k5-store-runs: $d is not sealed" >&2; exit 2; }
	bin/cpcheck verify-seal "$d" >/dev/null
	tar -C "$dir" --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - "$id" | zstd -q -19 -T0 -o "$stage/$id.tar.zst"
	if [ -n "${full[$id]:-}" ]; then
		out+=("$id full")
	else
		tar -C "$dir" --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - "$id/manifest.json" "$id/plan.json" "$id/seal.json" |
			zstd -q -19 -T0 -o "$stage/$id.stub.tar.zst"
		out+=("$id stub")
	fi
done
# Committed stubs the policy keeps in full: their full archives, checked
# against the stubs (fail closed).
for s in "$dir"/*.stub.tar.zst; do
	id=$(basename "$s" .stub.tar.zst)
	[ -n "${full[$id]:-}" ] || continue
	a="$local/$id.tar.zst"
	[ -f "$a" ] || { echo "k5-store-runs: $id is kept in full but $a is missing" >&2; exit 2; }
	mkdir -p "$stage/x-stub" "$stage/x-full"
	zstd -dc "$s" | tar -C "$stage/x-stub" -xf -
	zstd -dc "$a" | tar -C "$stage/x-full" -xf - "$id/manifest.json" "$id/plan.json" "$id/seal.json"
	for f in manifest.json plan.json seal.json; do
		cmp -s "$stage/x-stub/$id/$f" "$stage/x-full/$id/$f" || { echo "k5-store-runs: $a: $f differs from the committed stub" >&2; exit 2; }
	done
	chmod -R u+w "$stage/x-stub" "$stage/x-full" && rm -rf "${stage:?}/x-stub" "${stage:?}/x-full"
	out+=("$id unstub")
done
# Budget: the files under evidence/ after the move (run directories are
# replaced by their archives or stubs; replaced stubs by full archives).
now=$(find evidence -type f -printf '%s\n' | awk '{s+=$1} END {print s+0}')
add=0
for x in "${out[@]}"; do
	id=${x% *} kind=${x#* }
	case "$kind" in
	full | stub)
		[ "$under_evidence" = yes ] && now=$((now - $(find "$dir/$id" -type f -printf '%s\n' | awk '{s+=$1} END {print s+0}')))
		if [ "$kind" = full ]; then
			add=$((add + $(stat -c %s "$stage/$id.tar.zst")))
		else
			add=$((add + $(stat -c %s "$stage/$id.stub.tar.zst")))
		fi ;;
	unstub)
		[ "$under_evidence" = yes ] && now=$((now - $(stat -c %s "$dir/$id.stub.tar.zst")))
		add=$((add + $(stat -c %s "$local/$id.tar.zst"))) ;;
	esac
done
if [ "$under_evidence" = yes ] && [ $((now + add)) -gt "$budget" ]; then
	echo "{\"command\":\"k5-store-runs\",\"error\":\"evidence/ would be $((now + add)) bytes, over P25 ($budget)\",\"evidence_bytes\":$now,\"adding\":$add,\"exit_code\":2}"
	exit 2
fi
for x in "${out[@]}"; do
	id=${x% *} kind=${x#* }
	case "$kind" in
	full)
		mv "$stage/$id.tar.zst" "$dir/$id.tar.zst"
		f="$dir/$id.tar.zst" ;;
	stub)
		mv "$stage/$id.tar.zst" "$local/$id.tar.zst"
		mv "$stage/$id.stub.tar.zst" "$dir/$id.stub.tar.zst"
		f="$dir/$id.stub.tar.zst" ;;
	unstub)
		cp "$local/$id.tar.zst" "$dir/$id.tar.zst"
		rm "$dir/$id.stub.tar.zst"
		echo "{\"run\":\"$id\",\"stored\":\"full\",\"replaced\":\"stub\",\"archive\":\"$dir/$id.tar.zst\",\"bytes\":$(stat -c %s "$dir/$id.tar.zst")}"
		continue ;;
	esac
	chmod -R u+w "$dir/$id" && rm -rf "${dir:?}/$id"
	echo "{\"run\":\"$id\",\"stored\":\"$kind\",\"archive\":\"$f\",\"bytes\":$(stat -c %s "$f")}"
done
echo "{\"command\":\"k5-store-runs\",\"policy\":\"$policy\",\"under_evidence\":\"$under_evidence\",\"runs\":${#out[@]},\"evidence_bytes\":$((now + add)),\"budget\":$budget,\"exit_code\":0}"
