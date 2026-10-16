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
	step 0 metrics-probe scripts/prek7-metrics-probe.sh baseline
}

case "${1:-}" in
setup | g12 | probe) "$1" ;;
*)
	echo "usage: $0 setup|g12|probe" >&2
	exit 3
	;;
esac
