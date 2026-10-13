#!/usr/bin/env bash
# k5-store-runs.sh — CONTRACT §13 local evidence policy for K5 (P25,
# ADR-0030). Every sealed run directory under <dir> is archived as a zstd
# tar (still sealed by the hashes of its uncompressed files; cpgate reads the
# archive). Committed in full (<run-id>.tar.zst): every C10 primary run
# (K5.3 reads its observations) and the primary and minimized runs of every
# real-build FAIL (K5.5's triage). Every other run — the other primary runs,
# minimization candidates and replays — is committed as a stub
# (<run-id>.stub.tar.zst: manifest.json, plan.json, seal.json; cpgate checks
# them against the seal and checks.json), its full archive kept under
# <local>/ on this machine (LIM-30). The step refuses (exit 2, nothing
# moved into evidence) when the archives would make evidence/ exceed P25
# (200 MB, counted as bytes/10⁶ of the files under evidence/).
# usage: k5-store-runs.sh <runs-dir> <local-full-dir>
set -euo pipefail
dir=${1:?runs directory} local=${2:?local directory for full archives of stubbed runs}
budget=200000000
mkdir -p "$local"
stage=$(mktemp -d runs/.k5-store-XXXXXX)
trap 'rm -rf "$stage"' EXIT
field() { sed -n "s/.*\"$2\":\"\\([^\"]*\\)\".*/\\1/p" "$1/manifest.json"; }
# Runs kept in full: C10 primaries; real-build FAIL primaries and their
# minimized runs (from the minimization journals).
declare -A full=()
for d in "$dir"/*/; do
	d=${d%/}
	id=$(basename "$d")
	[ -f "$d/seal.json" ] || continue
	grep -q '"replay_of"\|"candidate_of"' "$d/manifest.json" && continue
	[ "$(field "$d" campaign)" = C10 ] && full[$id]=1
	if [ "$(field "$d" mutant)" = real ] && grep -q '"verdict":"FAIL"' "$dir/$id.checks.json" 2>/dev/null; then
		full[$id]=1
		j="$dir/$id.minimize.json"
		[ -f "$j" ] || { echo "k5-store-runs: real-build FAIL $id has no minimization journal" >&2; exit 2; }
		m=$(sed -n 's/.*"minimized_run":"\([^"]*\)".*/\1/p' "$j")
		[ -n "$m" ] && full[$m]=1
	fi
done
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
# Budget: the files under evidence/ after the move (run directories are
# replaced by their archives or stubs).
now=$(find evidence -type f -printf '%s\n' | awk '{s+=$1} END {print s+0}')
add=0
for x in "${out[@]}"; do
	id=${x% *} kind=${x#* }
	now=$((now - $(find "$dir/$id" -type f -printf '%s\n' | awk '{s+=$1} END {print s+0}')))
	if [ "$kind" = full ]; then
		add=$((add + $(stat -c %s "$stage/$id.tar.zst")))
	else
		add=$((add + $(stat -c %s "$stage/$id.stub.tar.zst")))
	fi
done
if [ $((now + add)) -gt "$budget" ]; then
	echo "{\"command\":\"k5-store-runs\",\"error\":\"evidence/ would be $((now + add)) bytes, over P25 ($budget)\",\"evidence_bytes\":$now,\"adding\":$add,\"exit_code\":2}"
	exit 2
fi
for x in "${out[@]}"; do
	id=${x% *} kind=${x#* }
	if [ "$kind" = full ]; then
		mv "$stage/$id.tar.zst" "$dir/$id.tar.zst"
		f="$dir/$id.tar.zst"
	else
		mv "$stage/$id.tar.zst" "$local/$id.tar.zst"
		mv "$stage/$id.stub.tar.zst" "$dir/$id.stub.tar.zst"
		f="$dir/$id.stub.tar.zst"
	fi
	chmod -R u+w "$dir/$id" && rm -rf "${dir:?}/$id"
	echo "{\"run\":\"$id\",\"stored\":\"$kind\",\"archive\":\"$f\",\"bytes\":$(stat -c %s "$f")}"
done
echo "{\"command\":\"k5-store-runs\",\"runs\":${#out[@]},\"evidence_bytes\":$((now + add)),\"budget\":$budget,\"exit_code\":0}"
