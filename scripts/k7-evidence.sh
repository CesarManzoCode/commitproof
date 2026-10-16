#!/usr/bin/env bash
# k7-evidence.sh — produces evidence/K7, the record of sprint K7 (full
# campaign, statistics and release; owner decisions 1–6 of 2026-10-15 on
# K6b's open items). Every command it runs goes through
# scripts/record-command.sh (argv, exit code, stdout/stderr sha256 in
# evidence/K7/commands.jsonl). Resumable like k6b-evidence.sh: a step whose
# record exists with the accepted exit code is skipped; an interrupted step
# (logs, no record) stops the script. Run from the repository root on a
# committed tree, from a copy of this script (bash re-reads a script while
# it runs). K7_EV overrides the evidence directory (dry runs).
#
# usage: k7-evidence.sh <group>
#   setup     binaries (make build); cpgate K1–K6 on copies of the closed
#             evidence (precondition)
#   m1oracle  ADR-0046 (owner decision 4): elle-cli on M1's five D1 runs,
#             each extracted from its committed full archive into
#             runs/.k7/oracle (seal verified, seal root = K4's checks.json);
#             cpcheck check --elle-oracle; if it cannot finish, cpcheck
#             without the oracle (the checks.json) and elle-cli's validity
#             and process passes recorded on their own (K4's approach); the
#             record evidence/K7/oracle/<run-id>.json
set -u -o pipefail
export GOTOOLCHAIN=go1.26.9 CGO_ENABLED=0
EV=${K7_EV:-evidence/K7}
SCRATCH=runs/.k7
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
		echo "k7-evidence: $name is recorded with exit $prev, expected $want; inspect $EV/logs/$name.*" >&2
		exit 1
	fi
	if interrupted "$name"; then
		echo "k7-evidence: $name was interrupted (logs, no record); move its logs aside or re-run under a new name" >&2
		exit 1
	fi
	echo "== $(date -u +%H:%M:%S) $name: $*" >&2
	"${REC[@]}" "$name" -- "$@" >/dev/null
	local rc=$?
	if [ "$want" != any ] && [ "$rc" -ne "$want" ]; then
		echo "k7-evidence: step $name exited $rc, expected $want (see $EV/logs/$name.stderr)" >&2
		exit 1
	fi
}

setup() {
	step 0 build make build
	step 0 precondition scripts/prek7-precondition.sh
}

# oracle_run <run-dir> <source>: ADR-0046's procedure on one run directory.
oracle_run() {
	local d=$1 src=$2 id rc iso model
	id=$(basename "$d")
	mkdir -p "$EV/oracle"
	step any "oracle-$id" bin/cpcheck check "$d" --elle-oracle
	rc=$(recorded "oracle-$id")
	if [ "$rc" != 0 ] && [ "$rc" != 1 ]; then
		# cpcheck could not finish elle-cli's passes: its checks.json without
		# the oracle, then the validity pass and the process pass recorded
		# on their own (K4's approach, LIM-31).
		step any "check-$id" bin/cpcheck check "$d"
		if [ -f "$d.elle/history.elle.json" ]; then
			iso=$(grep -o '"iso":"[a-z-]*"' "$d/plan.json" | sort -u | sed 's/.*:"\(.*\)"/\1/')
			model=serializable
			[ "$iso" = repeatable-read ] && model=snapshot-isolation
			step any "elle-validity-$id" docker run --rm --label com.docker.compose.project=commitproof --network none \
				--read-only --tmpfs /tmp -v "./$d.elle:/h:ro" commitproof/elle-oracle:9c2a18b -m list-append -c "$model" \
				--output json --cycle-search-timeout 10000 /h/history.elle.json
			step any "elle-process-$id" docker run --rm --label com.docker.compose.project=commitproof --network none \
				--read-only --tmpfs /tmp -v "./$d.elle:/h:ro" commitproof/elle-oracle:9c2a18b -m list-append -c "$model" \
				--output json --cycle-search-timeout 10000 \
				-a G0,G1a,G1b,G1c,G-single,G-nonadjacent,G2-item,incompatible-order,dirty-update,G0-process,G1c-process,G-single-process,G-nonadjacent-process,G2-item-process \
				/h/history.elle.json
		fi
	fi
	step any "$(next "k7oracle-$id")" go run ./internal/tools/k7oracle --ev "$EV" --checks "$d.checks.json" --source "$src" --out "$EV/oracle/$id.json"
}

m1oracle() {
	local o=$SCRATCH/oracle id d
	mkdir -p "$o"
	step 0 "$(next build)" make build
	for id in 20261011T215900Z-D1-1-M1-59a95336 20261011T220118Z-D1-2-M1-8b22bb67 20261011T220335Z-D1-3-M1-da159fa0 \
		20261011T220553Z-D1-4-M1-ad3fca18 20261011T220810Z-D1-5-M1-3319a30c; do
		d=$o/$id
		[ -d "$d" ] || tar --zstd -C "$o" -xf "evidence/K4/runs/$id.tar.zst"
		step 0 "verify-seal-$id" bin/cpcheck verify-seal "$d"
		[ "$(jq -r .merkle_root "$d/seal.json")" = "$(jq -r .seal_root "evidence/K4/runs/$id.checks.json")" ] ||
			{ echo "k7-evidence: $id: seal root differs from K4's checks.json" >&2; exit 1; }
		oracle_run "$d" "D1 on M1 (K4, cited by K7; ADR-0039)"
		cp "$d.checks.json" "$EV/oracle/$id.checks.json"
	done
}

case "${1:-}" in
setup | m1oracle) "$1" ;;
*)
	echo "usage: $0 setup|m1oracle" >&2
	exit 3
	;;
esac
