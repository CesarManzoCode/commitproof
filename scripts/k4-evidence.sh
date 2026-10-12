#!/usr/bin/env bash
# k4-evidence.sh — produces evidence/K4 for gate K4 (CONTRACT §15 K4). Every
# command the gate reads runs through scripts/record-command.sh (argv, exit
# code, stdout/stderr sha256 in evidence/K4/commands.jsonl). Resumable: a
# step whose record already exists with an accepted exit code is skipped, so
# the script can be re-run after an interruption; the campaign itself is
# resumable (sealed runs are never recomputed) and is re-recorded under a
# new name (campaign-k4-<n>) until one invocation completes the matrix.
# Run from the repository root on a committed tree, one cluster at a time.
#
# usage: k4-evidence.sh setup|rebuild|campaign|oracle|ci|store|gate|all
#   setup     precondition, builds (real, M1, M2, M4, h1), quorum table,
#             h1 deployment record
#   rebuild   after a harness fix: binaries and the real images rebuilt from
#             the fixed commit (the variant images and their lock entries are
#             kept: their sources did not change), recorded as build-<n>,
#             images-build-<n>, images-verify-<n>
#   campaign  campaigns/k4.json into evidence/K4/runs (hours; background)
#   oracle    elle-cli cross-check of every real-build FAIL with C3 anomalies
#   ci        make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke
#             ci-mutant-smoke; make lint-workflow
#   store     runs archived (full) or summarized (stubs), lock and patches
#             copied, results.json and h1.json written
#   gate      cpgate K4 --negative-controls
set -u -o pipefail
EV=evidence/K4
REC=(scripts/record-command.sh "$EV/commands.jsonl" "$EV/logs")
mkdir -p "$EV/runs" "$EV/logs"

recorded() { # name → exit code of its record, or empty
	[ -f "$EV/commands.jsonl" ] || return 0
	grep -F "\"name\":\"$1\"" "$EV/commands.jsonl" | tail -n 1 | sed -n 's/.*"exit_code":\([0-9]*\).*/\1/p'
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
		echo "k4-evidence: $name is recorded with exit $prev, expected $want; inspect $EV/logs/$name.*" >&2
		exit 1
	fi
	echo "== $(date -u +%H:%M:%S) $name: $*" >&2
	"${REC[@]}" "$name" -- "$@" >/dev/null
	local rc=$?
	if [ "$want" != any ] && [ "$rc" -ne "$want" ]; then
		echo "k4-evidence: step $name exited $rc, expected $want (see $EV/logs/$name.stderr)" >&2
		exit 1
	fi
}

setup() {
	step 0 k4-precondition scripts/k4-precondition.sh
	step 0 build make build
	step 0 images-build bin/commitproof images build
	for m in M1 M2 M4; do step 0 "images-build-$m" bin/commitproof images build --mutant "$m"; done
	step 0 images-build-h1 bin/commitproof images build --profile h1
	step 0 quorumxcheck bin/quorumxcheck --out "$EV/quorum-table.json"
	# H1 derivation (§9.4, ADR-0025): the historical topology from zero to
	# HEALTHY and back, recorded (the DH1 runs bring it up again each).
	step any h1-cluster-up bin/commitproof cluster up --profile h1
	step any h1-cluster-verify bin/commitproof cluster verify --profile h1
	step 0 h1-cluster-down bin/commitproof cluster down --profile h1
}

rebuild() {
	local n=2
	while [ -n "$(recorded "build-$n")" ]; do n=$((n + 1)); done
	step 0 "build-$n" make build
	step 0 "images-build-$n" bin/commitproof images build
	step 0 "images-verify-$n" bin/commitproof images verify
}

campaign() {
	local n=1 prev
	while :; do
		prev=$(recorded "campaign-k4-$n")
		case "$prev" in
		"") break ;;
		0 | 1) echo "== campaign complete (campaign-k4-$n exit $prev)" >&2; return 0 ;;
		*) n=$((n + 1)) ;;
		esac
	done
	step any "campaign-k4-$n" bin/commitproof campaign --matrix campaigns/k4.json --out "$EV/runs"
	prev=$(recorded "campaign-k4-$n")
	[ "$prev" = 0 ] || [ "$prev" = 1 ] || { echo "k4-evidence: campaign-k4-$n exit $prev; re-run to resume" >&2; exit 1; }
}

oracle() {
	# Real-build primary runs that FAIL with C3 anomalies get the elle-cli
	# cross-check (§10.11; K4.7); cpcheck rewrites their checks.json with it.
	for c in "$EV"/runs/*.checks.json; do
		d=${c%.checks.json}
		[ -d "$d" ] || continue
		grep -q '"mutant":"real"' "$d/manifest.json" || continue
		grep -q '"profile":"baseline"' "$d/manifest.json" || continue
		grep -q '"replay_of"\|"candidate_of"' "$d/manifest.json" && continue
		grep -q '"verdict":"FAIL"' "$c" || continue
		python3 -c "import json,sys; c=json.load(open(sys.argv[1])); sys.exit(0 if any(x['id']=='C3' and x['valid'] is False for x in c['checkers']) else 1)" "$c" || continue
		step any "oracle-$(basename "$d")" bin/cpcheck check "$d" --elle-oracle
		# cpcheck runs three elle-cli passes (validity, process, realtime). If
		# one fails (elle-cli exits 255 with OutOfMemoryError in its process-
		# order cycle search on ~18 000-operation histories), the validity
		# pass — the one that decides validity and the anomaly classes — is
		# recorded on its own as the cross-check, and the process pass is
		# recorded to show the failure (LIMITATIONS).
		if [ "$(recorded "oracle-$(basename "$d")")" != 1 ] && [ -f "$d.elle/history.elle.json" ]; then
			iso=$(grep -o '"iso":"[a-z-]*"' "$d/plan.json" | sort -u | sed 's/.*:"\(.*\)"/\1/')
			model=serializable
			[ "$iso" = repeatable-read ] && model=snapshot-isolation
			step any "elle-validity-$(basename "$d")" docker run --rm --label com.docker.compose.project=commitproof --network none \
				--read-only --tmpfs /tmp -v "./$d.elle:/h:ro" commitproof/elle-oracle:9c2a18b -m list-append -c "$model" \
				--output json --cycle-search-timeout 10000 /h/history.elle.json
			step any "elle-process-$(basename "$d")" docker run --rm --label com.docker.compose.project=commitproof --network none \
				--read-only --tmpfs /tmp -v "./$d.elle:/h:ro" commitproof/elle-oracle:9c2a18b -m list-append -c "$model" \
				--output json --cycle-search-timeout 10000 \
				-a G0,G1a,G1b,G1c,G-single,G-nonadjacent,G2-item,incompatible-order,dirty-update,G0-process,G1c-process,G-single-process,G-nonadjacent-process,G2-item-process \
				/h/history.elle.json
		fi
	done
}

ci() {
	step 0 lint-workflow make lint-workflow
	# A failed attempt stays recorded; the next one is ci-<n> (K4.8 reads the
	# latest attempt).
	local name=ci n=1 prev
	while prev=$(recorded "$name") && [ -n "$prev" ]; do
		[ "$prev" = 0 ] && { echo "== skip $name (recorded, exit 0)" >&2; return 0; }
		n=$((n + 1))
		name="ci-$n"
	done
	step 0 "$name" make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke ci-mutant-smoke
}

store() {
	step 0 store-runs scripts/k4-store-runs.sh "$EV/runs" runs/k4-full
	mkdir -p "$EV/mutants"
	for m in M1 M2 M4; do cp "mutants/$m.patch" "$EV/mutants/$m.patch"; done
	cp images.lock.json "$EV/images.lock.json"
	step 0 results go run ./internal/tools/k4results --runs "$EV/runs" --out "$EV/results.json" --h1 "$EV/h1.json"
}

gate() {
	"${REC[@]}" cpgate-k4 -- bin/cpgate K4 --evidence "$EV" --negative-controls
}

case "${1:-}" in
setup) setup ;;
rebuild) rebuild ;;
campaign) campaign ;;
oracle) oracle ;;
ci) ci ;;
store) store ;;
gate) gate ;;
all) setup && campaign && oracle && ci && store && gate ;;
*) echo "usage: $0 setup|rebuild|campaign|oracle|ci|store|gate|all" >&2; exit 3 ;;
esac
