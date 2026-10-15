#!/usr/bin/env bash
# k6-evidence.sh — produces evidence/K6 for gate K6 (CONTRACT §15 K6,
# ADR-0036). Every command the gate reads runs through
# scripts/record-command.sh (argv, exit code, stdout/stderr sha256 in
# evidence/K6/commands.jsonl). Resumable like k5-evidence.sh: a step whose
# record exists with the accepted exit code is skipped; an interrupted step
# (logs, no record) stops the script. Run from the repository root on a
# committed tree. K6_EV overrides the evidence directory (dry runs).
#
# usage: k6-evidence.sh setup|render|fresh|ci|gate
#   setup   binaries (make build), the precondition (cpgate K1–K5 on copies
#           of the closed evidence), the reference run staged sealed
#   render  three renders of the reference run (render-1 writes the
#           committed example under report/), a cpreport built with
#           cp_sabotage_report_nondeterminism and its three renders (K6.1's
#           checker-level control), seal enforcement (K6.4), and the K4
#           campaign summary page (the per-campaign example)
#   fresh   git clone of HEAD, go clean -cache -modcache, build, render (K6.2)
#   ci      make lint-workflow; make ci-unit ci-checkers ci-cluster-smoke
#           ci-nemesis-smoke ci-mutant-smoke ci-report (a new attempt name
#           after a failed one)
#   gate    cpgate K6 --negative-controls (cpgate-k6, cpgate-k6-2, …)
set -u -o pipefail
export GOTOOLCHAIN=go1.26.9 CGO_ENABLED=0
EV=${K6_EV:-evidence/K6}
REF_ID=20261011T231036Z-C2-3-real-f4505b54 # ADR-0036
REF_SRC=evidence/K4/runs/$REF_ID.tar.zst
REF=$EV/reference-run/$REF_ID.tar.zst
SCRATCH=runs/.k6
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
		echo "k6-evidence: $name is recorded with exit $prev, expected $want; inspect $EV/logs/$name.*" >&2
		exit 1
	fi
	if interrupted "$name"; then
		echo "k6-evidence: $name was interrupted (logs, no record); move its logs aside or re-run under a new name" >&2
		exit 1
	fi
	echo "== $(date -u +%H:%M:%S) $name: $*" >&2
	"${REC[@]}" "$name" -- "$@" >/dev/null
	local rc=$?
	if [ "$want" != any ] && [ "$rc" -ne "$want" ]; then
		echo "k6-evidence: step $name exited $rc, expected $want (see $EV/logs/$name.stderr)" >&2
		exit 1
	fi
}
fresh_out() { # a scratch output directory that does not exist yet
	local d="$SCRATCH/$1"
	if [ -e "$d" ]; then chmod -R u+w "$d" && rm -rf "${d:?}"; fi
	echo "$d"
}

setup() {
	step 0 build make build
	step 0 k6-precondition scripts/k6-precondition.sh
	step 0 stage-reference scripts/k6-stage-reference.sh "$REF_SRC" "$EV/reference-run"
}

render() {
	step 0 render-1 bin/cpreport render "$REF" --out "$EV/report"
	for n in 2 3; do
		[ -n "$(recorded "render-$n")" ] || step 0 "render-$n" bin/cpreport render "$REF" --out "$(fresh_out "render-$n")"
	done
	[ -n "$(recorded sabotage-build)" ] || step 0 sabotage-build go build -trimpath -tags cp_sabotage_report_nondeterminism -o "$SCRATCH/sabotage/cpreport" ./cmd/cpreport
	for n in 1 2 3; do
		[ -n "$(recorded "sabotage-render-$n")" ] || step 0 "sabotage-render-$n" "$SCRATCH/sabotage/cpreport" render "$REF" --out "$(fresh_out "sabotage-render-$n")"
	done
	step 0 seal-enforcement scripts/k6-seal-enforcement.sh "$REF"
	step 0 render-campaign-k4 bin/cpreport render evidence/K4/runs --out "$EV/campaign-k4"
}

fresh() {
	step 0 fresh-clone scripts/k6-fresh-clone.sh "$REF"
}

ci() {
	[ -n "$(recorded lint-workflow)" ] || step 0 lint-workflow make lint-workflow
	local name
	name=$(next ci)
	step any "$name" make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke ci-mutant-smoke ci-report
	[ "$(recorded "$name")" = 0 ] || { echo "k6-evidence: $name exited $(recorded "$name"); see $EV/logs/$name.*" >&2; exit 1; }
}

gate() {
	step any "$(next cpgate-k6)" bin/cpgate K6 --evidence "$EV" --negative-controls
}

case "${1:-}" in
setup | render | fresh | ci | gate) "$1" ;;
*)
	echo "usage: $0 setup|render|fresh|ci|gate" >&2
	exit 3
	;;
esac
