#!/usr/bin/env bash
# prek7-evidence.sh — produces evidence/pre-K7, the readiness record before
# sprint K7 (owner decisions D-A … D-D of 2026-10-15). Every command it
# runs goes through scripts/record-command.sh (argv, exit code,
# stdout/stderr sha256 in evidence/pre-K7/commands.jsonl). Resumable like
# k6-evidence.sh: a step whose record exists with the accepted exit code is
# skipped; an interrupted step (logs, no record) stops the script. Run from
# the repository root on a committed tree. PREK7_EV overrides the evidence
# directory (dry runs).
#
# usage: prek7-evidence.sh <group>
#   setup     binaries (make build); cpgate K1–K6 on copies of the closed
#             evidence (precondition)
#   g12       ADR-0037: every recorded K5 C11 run re-checked, checks.json
#             byte-identical (no recorded verdict changes)
#   probe     ADR-0038: what serves /metrics at the pinned commit (the
#             committed baseline compose, before the exporter is enabled)
#   scrape    ADR-0038 on a real run: images rebuilt and locked, one smoke
#             run (P04, 45 s) with scrapes, cpcheck, the run archived under
#             scrape-run/ (§13 form), its scrape cost measured, two renders
#             (the first committed under scrape-report/), metricscheck, and
#             the metrics section's negative controls
#   feasible  inputs of the K7 feasibility note: measured wall time per run
#             (K4/K5), §12.3's bounds for N = 5 and 10, evidence sizes
#   upstream  D-C: read-only lookups on github.com/multigres/multigres
#             (clone, file diffs since the pinned commit, issue/PR search);
#             "upstream read <n>..." reads the issues/PRs the search found
#   ci        make lint-workflow; make ci-unit ci-checkers ci-cluster-smoke
#             ci-nemesis-smoke ci-mutant-smoke ci-report (a new attempt name
#             after a failed one)
#   final     cpgate K1–K6 on copies again, evidence size, the backup
#             manifest of the local-only full archives
#   lim33     D-D: LIM-33's origin experiment on plain PostgreSQL, 5 runs
#             (scripts/prek7-lim33.sh; raw outputs under lim33/), and their
#             summary
set -u -o pipefail
export GOTOOLCHAIN=go1.26.9 CGO_ENABLED=0
EV=${PREK7_EV:-evidence/pre-K7}
SCRATCH=runs/.prek7
REC=(scripts/record-command.sh "$EV/commands.jsonl" "$EV/logs")
mkdir -p "$EV/logs" "$SCRATCH"

recorded() { # name → exit code of its record, or empty
	[ -f "$EV/commands.jsonl" ] || return 0
	grep -F "\"name\":\"$1\"" "$EV/commands.jsonl" | tail -n 1 | sed -n 's/.*"exit_code":\([0-9]*\).*/\1/p'
}
interrupted() { # name → true when its logs exist without a record
	[ -z "$(recorded "$1")" ] && { [ -e "$EV/logs/$1.stdout" ] || [ -e "$EV/logs/$1.stderr" ]; }
}
next() { # base → the first unrecorded, uninterrupted name of base, base-2, …
	local name=$1 n=1
	while [ -n "$(recorded "$name")" ] || interrupted "$name"; do n=$((n + 1)); name="$1-$n"; done
	echo "$name"
}
step() { # expected-exit|any name argv...
	local want=$1 name=$2 prev
	shift 2
	prev=$(recorded "$name")
	if [ -n "$prev" ]; then
		if [ "$want" = any ] || [ "$prev" -eq "$want" ]; then
			echo "== skip $name (recorded, exit $prev)" >&2
			return 0
		fi
		echo "prek7-evidence: $name is recorded with exit $prev, expected $want; inspect $EV/logs/$name.*" >&2
		exit 1
	fi
	if interrupted "$name"; then
		echo "prek7-evidence: $name was interrupted (logs, no record); move its logs aside or re-run under a new name" >&2
		exit 1
	fi
	echo "== $(date -u +%H:%M:%S) $name: $*" >&2
	"${REC[@]}" "$name" -- "$@" >/dev/null
	local rc=$?
	if [ "$want" != any ] && [ "$rc" -ne "$want" ]; then
		echo "prek7-evidence: step $name exited $rc, expected $want (see $EV/logs/$name.stderr)" >&2
		exit 1
	fi
}

setup() {
	step 0 build make build
	step 0 precondition scripts/prek7-precondition.sh
}

g12() {
	step 0 "$(next build)" make build
	step 0 g12-recheck scripts/prek7-g12-recheck.sh
}

probe() {
	step 0 "$(next build)" make build
	[ "$(recorded metrics-probe)" = 0 ] || step 0 "$(next metrics-probe)" scripts/prek7-metrics-probe.sh baseline
}

scrape() {
	local out=$SCRATCH/scrape
	step 0 "$(next build)" make build
	[ "$(recorded images-build)" = 0 ] || step 0 "$(next images-build)" bin/commitproof images build
	if [ "$(recorded scrape-smoke)" != 0 ]; then
		mkdir -p "$out"
		step 0 "$(next scrape-smoke)" bin/commitproof run --campaign smoke --seed 42 --out "$out"
	fi
	local id
	id=$(sed -n 's/.*"run_dir":"[^"]*\/\([^"/]*\)".*/\1/p' "$EV/logs/$(grep -F '"name":"scrape-smoke' "$EV/commands.jsonl" | tail -n 1 | sed 's/.*"name":"\([^"]*\)".*/\1/').stdout" | tail -n 1)
	[ -n "$id" ] || { echo "prek7-evidence: no run_dir in the scrape-smoke record" >&2; exit 1; }
	step any scrape-check bin/cpcheck check "$out/$id"
	if [ ! -f "$EV/scrape-run/$id.tar.zst" ]; then
		mkdir -p "$SCRATCH/scrape-stage" "$EV/scrape-run"
		cp -a "$out/$id" "$SCRATCH/scrape-stage/"
		step 0 scrape-archive scripts/compress-runs.sh "$SCRATCH/scrape-stage"
		cp "$SCRATCH/scrape-stage/$id.tar.zst" "$out/$id.checks.json" "$EV/scrape-run/"
	fi
	step 0 scrape-measure scripts/prek7-scrape-measure.sh "$out/$id" "$EV/scrape-run/$id.tar.zst"
	step 0 scrape-render-1 bin/cpreport render "$EV/scrape-run/$id.tar.zst" --out "$EV/scrape-report"
	[ -n "$(recorded scrape-render-2)" ] || { rm -rf "$SCRATCH/scrape-render-2"; step 0 scrape-render-2 bin/cpreport render "$EV/scrape-run/$id.tar.zst" --out "$SCRATCH/scrape-render-2"; }
	step 0 scrape-metricscheck go run ./internal/tools/metricscheck --report "$EV/scrape-report/$id.html" --run "$EV/scrape-run/$id.tar.zst"
	step 0 metrics-controls scripts/prek7-metrics-controls.sh "$EV/scrape-run/$id.tar.zst" "$EV/scrape-report/$id.html"
}

feasible() {
	step 0 durations scripts/prek7-durations.sh
	step 0 k7bounds go run ./internal/tools/k7bounds
	step 0 "$(next evidence-size)" scripts/prek7-evidence-size.sh
}

upstream() {
	if [ "${1:-}" = read ]; then
		shift
		step 0 "$(next upstream-read)" scripts/prek7-upstream.sh read "$EV/upstream" "$@"
	else
		step 0 upstream-search scripts/prek7-upstream.sh search "$EV/upstream"
	fi
}

lim33() {
	local n
	for n in 1 2 3 4 5; do
		[ "$(recorded "lim33-run-$n")" = 0 ] || step 0 "$(next "lim33-run-$n")" scripts/prek7-lim33.sh "$n" "$EV/lim33"
	done
	step 0 lim33-summary bash -c 'for n in 1 2 3 4 5; do tail -n 1 "$1/logs/lim33-run-$n.stdout"; done | jq -sc "{command: \"lim33-summary\", runs: length, k1_pg_cancel_backend_read_then_vanished: (map(select(.k1_pg_cancel_backend_read_then_vanished)) | length), k2_cancelrequest_read_then_vanished: (map(select(.k2_cancelrequest_read_then_vanished)) | length), k3_disconnect_never_read: (map(select(.k3_disconnect_never_read)) | length), k3_disconnect_still_waiting: (map(select(.k3_disconnect_still_waiting)) | length), errors: (map(select(.error)) | length), exit_code: 0}"' _ "$EV"
}

ci() {
	step 0 "$(next build)" make build
	[ "$(recorded lint-workflow)" = 0 ] || step 0 "$(next lint-workflow)" make lint-workflow
	local name
	name=$(next ci)
	step any "$name" make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke ci-mutant-smoke ci-report
	[ "$(recorded "$name")" = 0 ] || { echo "prek7-evidence: $name exited $(recorded "$name"); see $EV/logs/$name.*" >&2; exit 1; }
}

case "${1:-}" in
setup | g12 | probe | scrape | feasible | lim33 | ci) "$1" ;;
upstream)
	shift
	upstream "$@"
	;;
*)
	echo "usage: $0 setup|g12|probe|scrape|feasible|lim33|ci|upstream [read <n>...]" >&2
	exit 3
	;;
esac
