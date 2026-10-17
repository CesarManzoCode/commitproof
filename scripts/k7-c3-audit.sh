#!/usr/bin/env bash
# k7-c3-audit.sh — what the C3 correction of 32b660a (a reported cycle
# lacked one transaction; NOTES K7 §3) changes in the committed evidence
# (the owner's decision on ADR-0047: audit the claims it affects). For every
# committed run held in full (a *.tar.zst that is not a stub, under
# evidence/ and findings/) whose committed checks.json has a C3 result, the
# run is extracted into <scratch> (seal verified), re-checked with this
# commit's cpcheck (its checks.json written beside the extracted copy, never
# into evidence/), and C3's output compared with the committed one: valid,
# the classes of the listed anomalies and their totals (stats.counts), the
# classes of the observations (-process, -realtime and SI cycles, which no
# verdict reads), and how many anomalies and observations changed only
# their ops or explanation. Also the run's verdict. One JSON line per run,
# then a summary line; exit 0 when every run was compared (differences are
# results, reported), 1 when a run could not be extracted, verified or
# checked.
#
# usage: k7-c3-audit.sh <scratch-dir>
set -u -o pipefail
scratch=${1:?scratch directory}
mkdir -p "$scratch"
c3() { # checks.json → C3's comparable parts
	jq -c '{verdict: .verdict} + ([.checkers[] | select(.id == "C3")][0] // {} |
		{valid: (.valid // null),
		 classes: ([.anomalies[]?.class] | group_by(.) | map({(.[0]): length}) | add // {}),
		 counts: (.stats.counts // {}),
		 observations: ([.stats.observations[]?.class] | group_by(.) | map({(.[0]): length}) | add // {}),
		 cycles: [(.anomalies[]?, .stats.observations[]?) | {class, ops, explanation}]})' "$1"
}
n=0 same=0 cyc=0 diff=0 errs=0
while read -r a; do
	id=$(basename "$a" .tar.zst)
	c=${a%.tar.zst}.checks.json
	[ -f "$c" ] && jq -e '[.checkers[]? | select(.id == "C3")] | length == 1' "$c" >/dev/null 2>&1 || continue
	sub=$scratch/$(echo "$a" | cut -d/ -f2)
	mkdir -p "$sub"
	n=$((n + 1))
	if ! { [ -d "$sub/$id" ] || zstd -dc "$a" | tar -C "$sub" -xf -; } || ! bin/cpcheck verify-seal "$sub/$id" >/dev/null; then
		echo "{\"run\":\"$id\",\"archive\":\"$a\",\"error\":\"extract or seal\"}"
		errs=$((errs + 1))
		continue
	fi
	bin/cpcheck check "$sub/$id" >/dev/null
	rc=$?
	if [ "$rc" -gt 2 ] || [ ! -f "$sub/$id.checks.json" ]; then
		echo "{\"run\":\"$id\",\"archive\":\"$a\",\"error\":\"cpcheck exit $rc\"}"
		errs=$((errs + 1))
		continue
	fi
	old=$(c3 "$c") new=$(c3 "$sub/$id.checks.json")
	r=$(jq -nc --argjson o "$old" --argjson w "$new" --arg id "$id" --arg a "$a" '
		{run: $id, archive: $a, verdict: $o.verdict, verdict_now: $w.verdict,
		 c3_valid: $o.valid, c3_valid_now: $w.valid,
		 classes_same: ($o.classes == $w.classes and $o.counts == $w.counts),
		 observations_same: ($o.observations == $w.observations),
		 cycles_changed: ([range(0; ([$o.cycles, $w.cycles] | map(length) | max))] | map(select($o.cycles[.] != $w.cycles[.])) | length),
		 classes: $o.classes, classes_now: $w.classes, observations: $o.observations, observations_now: $w.observations}
		| .same = (.verdict == .verdict_now and .c3_valid == .c3_valid_now and .classes_same and .observations_same)')
	echo "$r"
	if [ "$(jq -r .same <<<"$r")" != true ]; then
		diff=$((diff + 1))
	elif [ "$(jq -r .cycles_changed <<<"$r")" != 0 ]; then
		cyc=$((cyc + 1))
	else
		same=$((same + 1))
	fi
done < <(find evidence findings -name '*.tar.zst' ! -name '*.stub.tar.zst' ! -name '*.unsealed.tar.zst' | sort)
code=0
[ "$errs" -gt 0 ] && code=1
echo "{\"command\":\"k7-c3-audit\",\"runs\":$n,\"identical\":$same,\"only_ops_or_explanations_changed\":$cyc,\"verdict_or_classes_changed\":$diff,\"errors\":$errs,\"exit_code\":$code}"
exit $code
