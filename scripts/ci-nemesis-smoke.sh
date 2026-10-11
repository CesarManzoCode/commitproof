#!/usr/bin/env bash
# ci-nemesis-smoke.sh — CONTRACT §13 `make ci-nemesis-smoke`: one 45 s run
# per fault kind F1a, F2a, F3a, F7, and F4a when /dev/fuse exists (else a
# SKIPPED-WITH-REASON line). Each run brings its own cluster up and down.
#
# Pass condition: every run completes with every fault confirmed and healed
# (`commitproof run` exit 0, the run not INVALID), its seal verifies, and
# cpcheck's verdict is not INVALID. The verdict itself is printed verbatim:
# a FAIL or UNKNOWN is a result about Multigres, reported as a FINDING line,
# never hidden and never turned into a harness failure.
set -euo pipefail

CP=bin/commitproof
OUT=${CP_SMOKE_OUT:-runs}
mkdir -p "$OUT"
cleanup() { "$CP" cluster down --profile baseline >/dev/null 2>&1 || true; }
trap cleanup EXIT

kinds="f1a f2a f3a f7"
if [ -c /dev/fuse ]; then
	kinds="$kinds f4a"
else
	echo "ci-nemesis-smoke: F4a SKIPPED-WITH-REASON: /dev/fuse is absent on this runner (slowfs cannot mount; LIMITATIONS LIM-05)"
fi
"$CP" images build >/dev/null
for k in $kinds; do
	summary=$("$CP" run --campaign "nsmoke-$k" --seed 1 --out "$OUT" | tail -n 1)
	echo "$summary"
	status=$(sed -n 's/.*"status":"\([^"]*\)".*/\1/p' <<<"$summary")
	run_dir=$(sed -n 's/.*"run_dir":"\([^"]*\)".*/\1/p' <<<"$summary")
	[ "$status" = complete ] || { echo "ci-nemesis-smoke: nsmoke-$k status '$status'" >&2; exit 2; }
	bin/cpcheck verify-seal "$run_dir" >/dev/null
	set +e
	check=$(bin/cpcheck check "$run_dir" | tail -n 1)
	set -e
	verdict=$(sed -n 's/.*"verdict":"\([^"]*\)".*/\1/p' <<<"$check")
	echo "ci-nemesis-smoke: nsmoke-$k verdict $verdict"
	case "$verdict" in
	PASS) ;;
	FAIL | UNKNOWN) echo "ci-nemesis-smoke: FINDING nsmoke-$k verdict $verdict: $check" ;;
	*) echo "ci-nemesis-smoke: nsmoke-$k verdict '$verdict'" >&2; exit 2 ;;
	esac
done
echo "ci-nemesis-smoke: OK ($kinds)"
