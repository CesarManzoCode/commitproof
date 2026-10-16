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
	step 0 cited-recheck scripts/k6b-cited-recheck.sh
}

case "${1:-}" in
setup | cited) "$1" ;;
*)
	echo "usage: $0 setup|cited" >&2
	exit 3
	;;
esac
