#!/usr/bin/env bash
# k7-store.sh — K7's store step (CONTRACT §13, P25; ADR-0039, ADR-0045).
# Every sealed run directory of <runs-dir> (runs/k7: the matrix, its
# follow-ups, the findings' replays) is written once into <dest>
# (evidence/K7/runs):
#   - in full (<run-id>.tar.zst, the §13 archive, sealed by the hashes of
#     its uncompressed files) when it is the original or the minimized run
#     of a new finding: the original and minimized_run of a finished
#     minimization journal (<run-id>.minimize.json, copied beside it; with
#     skip_repeats only new failures have one, ADR-0045);
#   - otherwise as a summary (<run-id>.stub.tar.zst: manifest, plan, seal;
#     and <run-id>.checks.json).
# The run directories under <runs-dir> stay: they are the full archives
# kept on the machine that ran them (§13). A run stored as a summary that
# the policy now keeps in full is replaced by its archive (manifest, plan
# and seal checked against the stub). A stored checks.json that differs
# from the run's current one is updated only while it is not committed
# (fail closed on a committed file).
#
# P25: the count is every byte under evidence/ plus every run archive
# (*.tar.zst) under findings/, in bytes/10⁶ MB. The step refuses (exit 2,
# nothing written) when the count after it would exceed 250 MB. After it
# writes, it reports the count and ADR-0045 §5's projection: count + pending
# runs × the mean size of K7's stored summaries (95 KB while there is none)
# + 5 MB of K7's remaining records and text; exit 4 when the projection
# exceeds 250 MB (the caller stops BLOCKED).
#
# usage: k7-store.sh <runs-dir> <dest> [--pending N]
set -euo pipefail
shopt -s nullglob
dir=${1:?runs directory} dest=${2:?destination} pending=0
[ "${3:-}" = --pending ] && pending=${4:?N}
budget=250000000
mkdir -p "$dest"
stage=$(mktemp -d runs/.k7-store-XXXXXX)
trap 'chmod -R u+w "$stage" 2>/dev/null; rm -rf "$stage"' EXIT
count() {
	local e f
	e=$(find evidence -type f -printf '%s\n' | awk '{s+=$1} END {print s+0}')
	f=0
	[ -d findings ] && f=$(find findings -type f -name '*.tar.zst' -printf '%s\n' | awk '{s+=$1} END {print s+0}')
	echo $((e + f))
}
tracked() { git ls-files --error-unmatch "$1" >/dev/null 2>&1; }

# Runs kept in full: the original and the minimized run of every finished
# minimization journal.
declare -A full=()
for j in "$dir"/*.minimize.json; do
	jq -e '.done == true' "$j" >/dev/null || { echo "k7-store: $j is not finished (follow-up interrupted?)" >&2; exit 2; }
	o=$(jq -r .original "$j") mr=$(jq -r .minimized_run "$j")
	full[$o]=1
	[ -n "$mr" ] && [ "$mr" != null ] && full[$mr]=1
done

out=()
add=0 sub=0
for d in "$dir"/*/; do
	d=${d%/}
	id=$(basename "$d")
	[ -f "$d/seal.json" ] || continue
	case "$id" in *.elle) continue ;; esac
	seal=$(jq -r .merkle_root "$d/seal.json")
	[ "$(jq -r .seal_root "$d.checks.json" 2>/dev/null)" = "$seal" ] || { echo "k7-store: $id: checks.json missing or for another seal" >&2; exit 2; }
	if [ -n "${full[$id]:-}" ]; then
		[ -f "$dest/$id.tar.zst" ] && { cmp -s "$d.checks.json" "$dest/$id.checks.json" || { echo "k7-store: $id: stored checks.json differs" >&2; exit 2; }; continue; }
		bin/cpcheck verify-seal "$d" >/dev/null
		tar -C "$dir" --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - "$id" | zstd -q -19 -T0 -o "$stage/$id.tar.zst"
		if [ -f "$dest/$id.stub.tar.zst" ]; then
			tracked "$dest/$id.stub.tar.zst" && { echo "k7-store: $id: a committed summary would become a full archive; commit order wrong" >&2; exit 2; }
			sub=$((sub + $(stat -c %s "$dest/$id.stub.tar.zst")))
			out+=("$id unstub")
		else
			out+=("$id full")
		fi
		add=$((add + $(stat -c %s "$stage/$id.tar.zst")))
		[ -f "$dest/$id.checks.json" ] && sub=$((sub + $(stat -c %s "$dest/$id.checks.json")))
		add=$((add + $(stat -c %s "$d.checks.json")))
		[ -f "$dir/$id.minimize.json" ] && add=$((add + $(stat -c %s "$dir/$id.minimize.json")))
	else
		if [ -f "$dest/$id.stub.tar.zst" ]; then
			if ! cmp -s "$d.checks.json" "$dest/$id.checks.json"; then
				tracked "$dest/$id.checks.json" && { echo "k7-store: $id: its committed checks.json differs from the run's" >&2; exit 2; }
				out+=("$id checks")
				add=$((add + $(stat -c %s "$d.checks.json"))) sub=$((sub + $(stat -c %s "$dest/$id.checks.json")))
			fi
			continue
		fi
		[ -f "$dest/$id.tar.zst" ] && { echo "k7-store: $id is stored in full but is no finding's run" >&2; exit 2; }
		bin/cpcheck verify-seal "$d" >/dev/null
		tar -C "$dir" --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - "$id/manifest.json" "$id/plan.json" "$id/seal.json" |
			zstd -q -19 -T0 -o "$stage/$id.stub.tar.zst"
		out+=("$id stub")
		add=$((add + $(stat -c %s "$stage/$id.stub.tar.zst") + $(stat -c %s "$d.checks.json")))
	fi
done
before=$(count)
if [ $((before - sub + add)) -gt "$budget" ]; then
	echo "{\"command\":\"k7-store\",\"error\":\"the P25 count would be $((before - sub + add)) bytes, over $budget\",\"count_bytes\":$before,\"adding\":$add,\"removing\":$sub,\"exit_code\":2}"
	exit 2
fi
nf=0 ns=0
for x in "${out[@]}"; do
	id=${x% *} kind=${x#* }
	case "$kind" in
	full | unstub)
		mv "$stage/$id.tar.zst" "$dest/$id.tar.zst"
		if [ "$kind" = unstub ]; then
			mkdir -p "$stage/x"
			zstd -dc "$dest/$id.stub.tar.zst" | tar -C "$stage/x" -xf -
			for f in manifest.json plan.json seal.json; do
				cmp -s "$stage/x/$id/$f" "$dir/$id/$f" || { echo "k7-store: $id: $f differs from the stored stub" >&2; exit 2; }
			done
			chmod -R u+w "$stage/x" && rm -rf "${stage:?}/x"
			rm -f "$dest/$id.stub.tar.zst"
		fi
		cp "$dir/$id.checks.json" "$dest/$id.checks.json"
		[ -f "$dir/$id.minimize.json" ] && cp "$dir/$id.minimize.json" "$dest/$id.minimize.json"
		nf=$((nf + 1))
		echo "{\"run\":\"$id\",\"stored\":\"full\",\"bytes\":$(stat -c %s "$dest/$id.tar.zst")}" ;;
	stub)
		mv "$stage/$id.stub.tar.zst" "$dest/$id.stub.tar.zst"
		cp "$dir/$id.checks.json" "$dest/$id.checks.json"
		ns=$((ns + 1))
		echo "{\"run\":\"$id\",\"stored\":\"summary\",\"bytes\":$(($(stat -c %s "$dest/$id.stub.tar.zst") + $(stat -c %s "$dest/$id.checks.json")))}" ;;
	checks)
		cp "$dir/$id.checks.json" "$dest/$id.checks.json"
		echo "{\"run\":\"$id\",\"stored\":\"checks.json updated\"}" ;;
	esac
done
after=$(count)
# Mean size of K7's stored summaries (stub + checks.json).
read -r sn sb < <(for s in "$dest"/*.stub.tar.zst; do id=$(basename "$s" .stub.tar.zst); echo $(($(stat -c %s "$s") + $(stat -c %s "$dest/$id.checks.json"))); done | awk '{n++; s+=$1} END {print n+0, s+0}')
mean=95000
[ "$sn" -gt 0 ] && mean=$((sb / sn))
proj=$((after + pending * mean + 5000000))
code=0
[ "$proj" -gt "$budget" ] && code=4
echo "{\"command\":\"k7-store\",\"stored_full\":$nf,\"stored_summaries\":$ns,\"count_bytes\":$after,\"summaries_in_dest\":$sn,\"mean_summary_bytes\":$mean,\"pending_runs\":$pending,\"projection_bytes\":$proj,\"budget_bytes\":$budget,\"exit_code\":$code}"
exit $code
