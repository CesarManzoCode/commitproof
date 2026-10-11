#!/usr/bin/env bash
# ci-mutant-smoke.sh — CONTRACT §13 `make ci-mutant-smoke`: build mutant M1
# (pinned commit + mutants/M1.patch; the build fails unless every binary
# without the patched package equals the real build, §9.1), run the
# directed schedule D1 once on it, and require the C1 violation
# (lost-write) that D1 is designed to produce (§9.2).
#
# Pass condition: the run completes (not INVALID), its seal verifies, and
# cpcheck's verdict is FAIL with C1 lost-write. One run is a smoke test of
# the pipeline (workload → nemesis → history → checker against a planted
# defect), not the K4.3 detection statistic (5 runs, P14).
set -euo pipefail

CP=bin/commitproof
OUT=${CP_SMOKE_OUT:-runs}
mkdir -p "$OUT"
cleanup() { "$CP" cluster down --profile baseline >/dev/null 2>&1 || true; }
trap cleanup EXIT

"$CP" images build >/dev/null
"$CP" images build --mutant M1 >/dev/null
summary=$("$CP" run --campaign D1 --seed 1 --mutant M1 --out "$OUT" | tail -n 1)
echo "$summary"
status=$(sed -n 's/.*"status":"\([^"]*\)".*/\1/p' <<<"$summary")
run_dir=$(sed -n 's/.*"run_dir":"\([^"]*\)".*/\1/p' <<<"$summary")
[ "$status" = complete ] || { echo "ci-mutant-smoke: D1 on M1 status '$status'" >&2; exit 2; }
bin/cpcheck verify-seal "$run_dir" >/dev/null
set +e
check=$(bin/cpcheck check "$run_dir" | tail -n 1)
set -e
echo "$check"
verdict=$(sed -n 's/.*"verdict":"\([^"]*\)".*/\1/p' <<<"$check")
c1=$(sed -n 's/.*"C1":\([a-z"]*\).*/\1/p' <<<"$check")
if [ "$verdict" != FAIL ] || [ "$c1" != false ] || ! grep -q '"class":"lost-write"' "$run_dir.checks.json"; then
	echo "ci-mutant-smoke: D1 on M1 not detected (verdict $verdict, C1 $c1)" >&2
	exit 1
fi
echo "ci-mutant-smoke: OK (M1 via D1: FAIL with C1 lost-write)"
