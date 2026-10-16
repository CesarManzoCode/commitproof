#!/usr/bin/env bash
# k6b-evidence.sh — produces evidence/K6b, the record of sprint K6b (what K7
# needs, before K7; owner decisions 1–7 of 2026-10-15). Every command it
# runs goes through scripts/record-command.sh (argv, exit code,
# stdout/stderr sha256 in evidence/K6b/commands.jsonl). Resumable like
# prek7-evidence.sh: a step whose record exists with the accepted exit code
# is skipped; an interrupted step (logs, no record) stops the script. Run
# from the repository root on a committed tree, from a copy of this script
# (bash re-reads a script while it runs). K6B_EV overrides the evidence
# directory (dry runs).
#
# usage: k6b-evidence.sh <group>
#   setup     binaries (make build); cpgate K1–K6 on copies of the closed
#             evidence (precondition)
#   cited     decision 2: every valid primary run of the directed cells K7
#             cites (D1, D2′, D3′, P1, D5 on mutant and real build; DH1)
#             re-checked with this commit's cpcheck, checks.json compared
#             byte for byte with the recorded one
#   config    ADR-0041: slotless differs from baseline only in the slot flag
#             of the seven poolers and gateways (rendered Compose configs);
#             K7's matrix and the smoke matrix as dry runs (make campaign-k7
#             DRY_RUN=1): the cells, their plans and the run count
#   images    binaries and images of this commit (the runner image must be
#             built from the host binary's commit), locked
#   smoke     decision 3 / item 8: make campaign-k7 on campaigns/k6b-smoke.json
#             (C12 seed 1001 on baseline, C4 seed 1001 on slotless) into
#             runs/.k6b/smoke, waited for with the slot flag of every pooler
#             and gateway captured; a second invocation (resume: no run);
#             cpcheck on each run; their summaries (stub, checks.json) under
#             smoke-not-evidence/ — not K7 evidence
#   exporter  decision 5: the smoke plan (seed 42) with and without ADR-0038's
#             exporter (CP_OTEL_METRICS_EXPORTER=none), each with per-container
#             CPU and memory; summaries under exporter-ab-not-evidence/;
#             the pair compared per component and role
#   estimate  item 9: summary bytes of every K7 cell's plan, durations,
#             bytes per real-build finding (K4/K5), evidence size; §12.3's
#             bounds for N = 5 and 10
set -u -o pipefail
export GOTOOLCHAIN=go1.26.9 CGO_ENABLED=0
EV=${K6B_EV:-evidence/K6b}
SCRATCH=runs/.k6b
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
		echo "k6b-evidence: $name is recorded with exit $prev, expected $want; inspect $EV/logs/$name.*" >&2
		exit 1
	fi
	if interrupted "$name"; then
		echo "k6b-evidence: $name was interrupted (logs, no record); move its logs aside or re-run under a new name" >&2
		exit 1
	fi
	echo "== $(date -u +%H:%M:%S) $name: $*" >&2
	"${REC[@]}" "$name" -- "$@" >/dev/null
	local rc=$?
	if [ "$want" != any ] && [ "$rc" -ne "$want" ]; then
		echo "k6b-evidence: step $name exited $rc, expected $want (see $EV/logs/$name.stderr)" >&2
		exit 1
	fi
}

setup() {
	step 0 build make build
	step 0 precondition scripts/prek7-precondition.sh
}

cited() {
	step 0 "$(next build)" make build
	[ "$(recorded cited-recheck)" = 0 ] || step 0 "$(next cited-recheck)" scripts/k6b-cited-recheck.sh
}

config() {
	step 0 "$(next build)" make build
	step 0 slotless-config scripts/k6b-slotless-config.sh
	step 0 "$(next k7-dry-run)" make campaign-k7 DRY_RUN=1
	step 0 "$(next smoke-dry-run)" make campaign-k7 DRY_RUN=1 K7_MATRIX=campaigns/k6b-smoke.json K7_OUT=$SCRATCH/smoke
}

images() {
	step 0 "$(next build)" make build
	step 0 "$(next images-build)" bin/commitproof images build
}

smoke() {
	local m=campaigns/k6b-smoke.json o=$SCRATCH/smoke d=$EV/smoke-not-evidence
	mkdir -p "$d"
	step 0 "$(next build)" make build
	if [ -z "$(recorded smoke-wait)" ]; then
		step 0 "$(next smoke-start)" make campaign-k7 K7_MATRIX=$m K7_OUT=$o
		step any smoke-wait scripts/k6b-smoke-wait.sh "$m" "$o" "$d/container-commands.jsonl"
	fi
	if [ -z "$(recorded smoke-resume-wait)" ]; then
		step 0 "$(next smoke-resume)" make campaign-k7 K7_MATRIX=$m K7_OUT=$o
		step any smoke-resume-wait scripts/campaign-k7.sh wait "$m" "$o"
	fi
	local r n=0
	for r in "$o"/*/; do
		r=${r%/}
		[ -f "$r/seal.json" ] || continue
		n=$((n + 1))
		step any "smoke-cpcheck-$n" bin/cpcheck check "$r"
	done
	step 0 smoke-store scripts/store-summaries.sh "$o" "$d" --measure-full
	[ -f "$d/campaign-k7.log" ] || cp "$o/campaign-k7.log" "$d/campaign-k7.log"
}

exporter() {
	local o=$SCRATCH/exporter-ab d=$EV/exporter-ab-not-evidence
	mkdir -p "$o" "$d"
	step 0 "$(next build)" make build
	[ "$(recorded exporter-ab-on)" = 0 ] || step 0 "$(next exporter-ab-on)" scripts/k6b-exporter-ab.sh on "$o"
	[ "$(recorded exporter-ab-off)" = 0 ] || step 0 "$(next exporter-ab-off)" env CP_OTEL_METRICS_EXPORTER=none scripts/k6b-exporter-ab.sh off "$o"
	step 0 exporter-ab-store-on scripts/store-summaries.sh "$o/runs-on" "$d" --measure-full
	step 0 exporter-ab-store-off scripts/store-summaries.sh "$o/runs-off" "$d" --measure-full
	local m
	for m in on off; do
		[ -f "$d/samples-$m.jsonl.zst" ] || zstd -q -19 -o "$d/samples-$m.jsonl.zst" "$o/samples-$m.jsonl"
	done
	step 0 exporter-ab-compare scripts/k6b-exporter-compare.sh "$EV/logs/exporter-ab-on.stdout" "$EV/logs/exporter-ab-off.stdout" "$o"
}

estimate() {
	step 0 "$(next build)" make build
	step 0 "$(next k7-estimate)" scripts/k6b-k7-estimate.sh
	step 0 "$(next k7bounds)" go run ./internal/tools/k7bounds
}

case "${1:-}" in
setup | cited | config | images | smoke | exporter | estimate) "$1" ;;
*)
	echo "usage: $0 setup|cited|config|images|smoke|exporter|estimate" >&2
	exit 3
	;;
esac
