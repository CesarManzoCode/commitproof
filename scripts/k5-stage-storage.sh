#!/usr/bin/env bash
# k5-stage-storage.sh — ADR-0032's measurements and the staged copy the gate
# is also evaluated on: a copy of evidence/K5 as a storage policy would commit
# it, built from the committed stubs and the full archives that
# k5-store-runs.sh kept under <full-dir> (no run is re-archived: a staged full
# archive is the file k5-store-runs.sh wrote). Run after the store step.
#
# Policies: adr0030 — C10 primary runs, real-build FAIL primaries and their
# minimized runs in full (ADR-0030); adr0032-option2 — the same without the
# C10 primaries that are not real-build FAILs; partial — the adr0030 set as
# partial archives (every file of the run but logs/; ADR-0032 option 4; for
# measurement only: cpgate reads full archives or stubs, so this copy is not
# evaluable). The staged copy gets its own results.json (k5results records
# each run's storage form). Prints one JSON line per run stored differently
# from a stub and a summary with the bytes evidence/ would hold in git: the
# tracked files outside evidence/K5 plus the staged copy. The copy lives
# outside evidence/ and is not evidence.
# usage: k5-stage-storage.sh adr0030|adr0032-option2|partial <out-dir> [<full-dir>]
set -euo pipefail
policy=${1:?policy} out=${2:?output directory} full=${3:-runs/k5-full}
case "$policy" in adr0030 | adr0032-option2 | partial) ;; *) echo "k5-stage-storage: unknown policy $policy" >&2; exit 3 ;; esac
ev=evidence/K5
case "$(realpath -m "$out")/" in "$(realpath -m evidence)/"*) echo "k5-stage-storage: $out is under evidence/" >&2; exit 3 ;; esac
[ -d "$full" ] || { echo "k5-stage-storage: no full archives in $full" >&2; exit 3; }
dst=$out/K5
if [ -d "$dst" ]; then chmod -R u+w "$dst" && rm -rf "${dst:?}"; fi
mkdir -p "$out"
cp -a "$ev" "$dst"
tmp=$(mktemp -d runs/.k5-stage-XXXXXX)
trap 'chmod -R u+w "$tmp" 2>/dev/null; rm -rf "$tmp"' EXIT
man() { # run id → its manifest.json (from the committed stub)
	zstd -dc "$dst/runs/$1.stub.tar.zst" | tar -xOf - "$1/manifest.json"
}
field() { sed -n "s/.*\"$2\":\"\\([^\"]*\\)\".*/\\1/p"; }
declare -A set=()
for s in "$dst"/runs/*.stub.tar.zst; do
	id=$(basename "$s" .stub.tar.zst)
	m=$(man "$id")
	grep -q '"replay_of"\|"candidate_of"' <<<"$m" && continue
	[ "$(field campaign <<<"$m")" = C10 ] && [ "$policy" != adr0032-option2 ] && set[$id]=1
	if [ "$(field mutant <<<"$m")" = real ] && grep -q '"verdict":"FAIL"' "$dst/runs/$id.checks.json"; then
		set[$id]=1
		j="$dst/runs/$id.minimize.json"
		[ -f "$j" ] || { echo "k5-stage-storage: real-build FAIL $id has no minimization journal" >&2; exit 2; }
		mr=$(sed -n 's/.*"minimized_run":"\([^"]*\)".*/\1/p' "$j")
		[ -n "$mr" ] && set[$mr]=1
	fi
done
for id in $(printf '%s\n' "${!set[@]}" | sort); do
	a="$full/$id.tar.zst"
	[ -f "$a" ] || { echo "k5-stage-storage: no full archive $a" >&2; exit 2; }
	if [ "$policy" = partial ]; then
		zstd -dc "$a" | tar -C "$tmp" -xf -
		tar -C "$tmp" --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 --exclude="$id/logs" -cf - "$id" |
			zstd -q -19 -T0 -o "$dst/runs/$id.partial.tar.zst"
		chmod -R u+w "$tmp/$id" && rm -rf "${tmp:?}/$id"
		f="$dst/runs/$id.partial.tar.zst"
	else
		cp "$a" "$dst/runs/$id.tar.zst"
		f="$dst/runs/$id.tar.zst"
	fi
	rm "$dst/runs/$id.stub.tar.zst"
	echo "{\"run\":\"$id\",\"stored\":\"$policy\",\"bytes\":$(stat -c %s "$f")}"
done
if [ "$policy" != partial ]; then
	go run ./internal/tools/k5results --runs "$dst/runs" --out "$dst/results.json" >/dev/null
fi
others=$(git ls-files -z evidence | grep -zv '^evidence/K5/' | xargs -0 stat -c %s | awk '{s+=$1} END {print s+0}')
k5=$(find "$dst" -type f -printf '%s\n' | awk '{s+=$1} END {print s+0}')
echo "{\"command\":\"k5-stage-storage\",\"policy\":\"$policy\",\"staged\":\"$dst\",\"runs_not_stubs\":${#set[@]},\"evidence_outside_k5_bytes\":$others,\"k5_bytes\":$k5,\"evidence_bytes\":$((others + k5)),\"budget\":200000000,\"fits\":$([ $((others + k5)) -le 200000000 ] && echo true || echo false),\"exit_code\":0}"
