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
#   images    binaries and images of this commit (the runner image is built
#             from the host binary's commit; make campaign-k7 does not
#             rebuild bin/), then images.lock.json is committed
#   start     the matrix: make campaign-k7 DRY_RUN=1 (130 runs), then
#             make campaign-k7 (detached; resumable)
#   wait      wait for the detached campaign (scripts/campaign-k7.sh wait),
#             recorded; an interrupted wait is re-run under the next name
#   oracle    ADR-0046 on every valid matrix primary of runs/k7 whose C3 is
#             invalid (its full directory; no cluster running): cpcheck
#             rewrites its checks.json with stats.elle before the store step
#   store     scripts/k7-store.sh runs/k7 evidence/K7/runs (P25; exit 4 =
#             the projection exceeds P25: stop BLOCKED, ADR-0045 §5)
#   results   the campaign log (zstd) and k7results: results.json,
#             runs-index.json
#   render    cpreport's campaign pages: K7's runs, and the cited K4 and K5
#             runs (K7.2)
#   triage    k7triage drafts for every valid real-build FAIL primary of
#             runs/k7; each repeat's triage record from its draft and oracle
#             record (scripts/k7-triage-final.py); new failures by hand
#   pkgreplay K7.4: for every findings/<id>/finding.json, its minimized run
#             extracted into runs/k7-pkg/<id>/ and the package's replay
#             command executed once more (pkg-replay-<id>); the replays
#             stored as summaries in evidence/K7/package-replays
#   liveness  K7.4's INVALID replay attempts that ended "recovery deadline
#             P06 exceeded" (runs/k7-pkg): scripts/k7-liveness.py on each,
#             then the summary of every liveness record (LIM-45)
#   index     k7results again with --pkg-replays: runs-index.json lists
#             K7.4's replays and INVALID attempts with their seal roots
#             (results.json must come out byte-identical)
#   packages  k7findings (finding.json's k7_replay) and each package's
#             README sections Recurrences in K7 and K7 replay
#             (scripts/k7-package-k7-sections.py)
#   pkgreports cpreport page and report excerpt of each package's original run
#   c3audit   scripts/k7-c3-audit.sh: every committed run held in full with a
#             C3 result re-checked in runs/.k7/c3audit with this commit's
#             cpcheck, C3's output compared (the C3 correction, ADR-0047)
#   cited     the cited D3′/D5 real-build runs in full into evidence/K7/cited
#             (scripts/k7-store-cited.sh, ADR-0045 §5, ADR-0047) and their
#             campaign page (render-cited-full)
#   p25       the P25 count of the tree (scripts/k7-p25-count.sh; exit 4
#             above 275 MB: BLOCKED, ADR-0047), recorded as p25-final
#   ci        make lint-workflow; every make ci-* target (attempt ci, ci-2, …)
#   gate      cpgate K7 --negative-controls (cpgate-k7, cpgate-k7-2, …), with
#             TMPDIR on disk (runs/.k7/tmp): it extracts run archives and
#             copies the evidence once per control, and /tmp may be tmpfs
#   tagcheck  scripts/k7-tag-check.sh on the latest CI's commit
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
			# Out of memory with the default heap (exit 255, or exit 1 when it
			# ran out while printing its answer): once more with 9 GB (K4).
			if grep -q OutOfMemoryError "$EV/logs/elle-validity-$id.stderr" && ! jq -e . "$EV/logs/elle-validity-$id.stdout" >/dev/null 2>&1; then
				step any "elle-validity-xmx9g-$id" docker run --rm --label com.docker.compose.project=commitproof --network none \
					--read-only --tmpfs /tmp -e JAVA_TOOL_OPTIONS=-Xmx9g -v "./$d.elle:/h:ro" commitproof/elle-oracle:9c2a18b -m list-append -c "$model" \
					--output json --cycle-search-timeout 10000 /h/history.elle.json
			fi
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

# m1triage: for the M1 D1 runs where elle-cli finished with classes C3
# does not report, the keys of elle-cli's cycles against the keys whose
# reads are incompatible (K7.3 triage, LIM-31).
m1triage() {
	local id d
	for id in 20261011T220335Z-D1-3-M1-da159fa0 20261011T220553Z-D1-4-M1-ad3fca18; do
		d=$SCRATCH/oracle/$id
		step 0 "cycle-keys-$id" python3 -I scripts/k7-cycle-keys.py "$d" "$d.elle/validity.json"
	done
	# The records again, with the triage written into them: every one
	# decided, every oracle-disagreement triaged (exit 0).
	for id in 20261011T215900Z-D1-1-M1-59a95336 20261011T220118Z-D1-2-M1-8b22bb67 20261011T220335Z-D1-3-M1-da159fa0 \
		20261011T220553Z-D1-4-M1-ad3fca18 20261011T220810Z-D1-5-M1-3319a30c; do
		d=$SCRATCH/oracle/$id
		step 0 "$(next "k7oracle-$id")" go run ./internal/tools/k7oracle --ev "$EV" --checks "$d.checks.json" \
			--source "D1 on M1 (K4, cited by K7; ADR-0039)" --out "$EV/oracle/$id.json"
	done
}

images() {
	step 0 "$(next build)" make build
	step 0 "$(next images-build)" bin/commitproof images build
}

start() {
	step 0 "$(next k7-dry-run)" make campaign-k7 DRY_RUN=1
	step 0 "$(next campaign-start)" make campaign-k7
}

wait_campaign() {
	step any "$(next campaign-wait)" scripts/campaign-k7.sh wait campaigns/k7.json runs/k7
}

# primaries <verdict-filter>: the valid matrix primaries of runs/k7 (no
# replay_of, no candidate_of) whose checks.json satisfies the jq filter.
primaries() {
	local d
	for d in runs/k7/*/; do
		d=${d%/}
		case "$d" in *.elle) continue ;; esac
		[ -f "$d/seal.json" ] && [ -f "$d.checks.json" ] || continue
		jq -e '(.replay_of == null) and (.candidate_of == null)' "$d/manifest.json" >/dev/null || continue
		jq -e "$1" "$d.checks.json" >/dev/null && echo "$d"
	done
}

idle() { # no project container may run (P10; ADR-0046: no cluster meanwhile)
	[ -z "$(docker ps -q --filter label=com.docker.compose.project=commitproof)" ] || { echo "k7-evidence: a project container is running" >&2; exit 1; }
	if [ -f runs/k7/campaign-k7.pid ] && kill -0 "$(cat runs/k7/campaign-k7.pid)" 2>/dev/null; then
		echo "k7-evidence: the campaign is still running" >&2
		exit 1
	fi
}

oracle() {
	idle
	local d m
	for d in $(primaries '.verdict == "FAIL" and any(.checkers[]; .id == "C3" and .valid == false)'); do
		m=$(jq -r '"K7 " + .campaign + "/" + .profile + " seed " + (.seed|tostring)' "$d/manifest.json")
		oracle_run "$d" "$m (matrix)"
	done
}

store() {
	local pending
	pending=$(make -s campaign-k7 DRY_RUN=1 | tail -n 1 | jq -r .runs_to_make)
	step 0 "$(next store)" scripts/k7-store.sh runs/k7 "$EV/runs" --pending "$pending"
}

results() {
	step 0 "$(next campaign-log)" zstd -q -19 -f runs/k7/campaign-k7.log -o "$EV/campaign-k7.log.zst"
	step 0 "$(next k7results)" go run ./internal/tools/k7results --runs "$EV/runs" --matrix campaigns/k7.json --log "$EV/campaign-k7.log.zst" --out "$EV"
}

render() {
	step 0 "$(next render-k7)" bin/cpreport render "$EV/runs" --out "$EV/report/k7"
	step 0 "$(next render-cited-k4)" bin/cpreport render evidence/K4/runs --out "$EV/report/cited-k4"
	step 0 "$(next render-cited-k5)" bin/cpreport render evidence/K5/runs --out "$EV/report/cited-k5"
}

triage() {
	local d id cell o
	mkdir -p "$EV/triage"
	for d in $(primaries '.verdict == "FAIL"'); do
		id=$(basename "$d")
		[ -n "$(recorded "k7triage-$id")" ] || step 0 "k7triage-$id" go run ./internal/tools/k7triage --run "$d"
		jq -e .repeat.repeat "$EV/logs/k7triage-$id.stdout" >/dev/null || { echo "== $id is a new failure: triaged by hand (triage/$id.json)" >&2; continue; }
		cell=$(jq -r --arg c "$(jq -r .campaign "$d/manifest.json")" --arg p "$(jq -r .profile "$d/manifest.json")" \
			'.entries[] | select(.campaign == $c and (.profile // "baseline") == $p) | .cell' campaigns/k7.json)
		o=()
		[ -f "$EV/oracle/$id.json" ] && o=("$EV/oracle/$id.json")
		[ -n "$(recorded "triage-final-$id")" ] || step 0 "triage-final-$id" python3 -I scripts/k7-triage-final.py "$EV/logs/k7triage-$id.stdout" "$cell" "$EV/triage/$id.json" "${o[@]}"
	done
}

# pkgreplay: K7.4 for each package (findings/<id>/finding.json's minimized
# run, from its committed archive).
pkgreplay() {
	idle
	local f fid run arch o r
	mkdir -p "$EV/package-replays"
	for f in findings/*/finding.json; do
		fid=$(jq -r .id "$f") run=$(jq -r .minimized_run.run_id "$f") arch=$(jq -r .minimized_run.archive "$f")
		o=runs/k7-pkg/$fid
		mkdir -p "$o"
		[ -d "$o/$run" ] || zstd -dc "$arch" | tar -C "$o" -xf -
		step 0 "$(next "verify-seal-pkg-$fid")" bin/cpcheck verify-seal "$o/$run"
		[ "$(jq -r .merkle_root "$o/$run/seal.json")" = "$(jq -r .minimized_run.seal_root "$f")" ] ||
			{ echo "k7-evidence: $fid: the extracted minimized run's seal differs from finding.json" >&2; exit 1; }
		if [ "$(recorded "pkg-replay-$fid")" != 0 ]; then
			step 0 "$(next "pkg-replay-$fid")" bin/commitproof replay "$o/$run" --times 5 --out "$o" --cpcheck bin/cpcheck
		fi
		# The replays as summaries (stub: manifest, plan, seal; checks.json),
		# as ADR-0045 stores replays.
		for r in "$o"/*/; do
			r=${r%/}
			[ -f "$r/seal.json" ] && jq -e --arg m "$run" '.replay_of == $m' "$r/manifest.json" >/dev/null || continue
			[ -f "$EV/package-replays/$(basename "$r").stub.tar.zst" ] && continue
			bin/cpcheck verify-seal "$r" >/dev/null || { echo "k7-evidence: $r: seal" >&2; exit 1; }
			tar -C "$o" --sort=name --owner=0 --group=0 --numeric-owner --mtime=@0 -cf - "$(basename "$r")/manifest.json" "$(basename "$r")/plan.json" \
				"$(basename "$r")/seal.json" | zstd -q -19 -T0 -o "$EV/package-replays/$(basename "$r").stub.tar.zst"
			cp "$r.checks.json" "$EV/package-replays/$(basename "$r").checks.json"
		done
	done
}

# pkgreports: §11.4's report excerpt of each package's original run —
# cpreport's page (render-pkg-<id>, kept in runs/.k7/pkg-report, not
# committed) and the excerpt from its data block, checks.json and the run's
# observations (excerpt-<id> → findings/<id>/report-excerpt.md).
pkgreports() {
	idle
	local f fid run arch o rn
	for f in findings/*/finding.json; do
		fid=$(jq -r .id "$f") run=$(jq -r .original_run.run_id "$f") arch=$(jq -r .original_run.archive "$f")
		o=$SCRATCH/pkg-orig
		mkdir -p "$o" "$SCRATCH/pkg-report/$fid"
		[ -d "$o/$run" ] || zstd -dc "$arch" | tar -C "$o" -xf -
		[ "$(recorded "render-pkg-$fid")" = 0 ] || step 0 "$(next "render-pkg-$fid")" bin/cpreport render "$arch" --out "$SCRATCH/pkg-report/$fid"
		rn=$(grep -oE "\"name\":\"render-pkg-$fid(-[0-9]+)?\"" "$EV/commands.jsonl" | tail -n 1 | sed 's/"name":"\(.*\)"/\1/')
		step 0 "$(next "excerpt-$fid")" python3 -I scripts/k7-report-excerpt.py "$SCRATCH/pkg-report/$fid/$run.html" "${arch%.tar.zst}.checks.json" \
			"$rn" "$EV/logs/$rn.stdout" "$arch" "$o/$run/observations.jsonl"
		cp "$EV/logs/$(grep -oE "\"name\":\"excerpt-$fid(-[0-9]+)?\"" "$EV/commands.jsonl" | tail -n 1 | sed 's/"name":"\(.*\)"/\1/').stdout" \
			"findings/$fid/report-excerpt.md"
	done
}

liveness() {
	local d id names=()
	for d in runs/k7-pkg/*/20*/; do
		d=${d%/}
		[ -f "$d/seal.json" ] || continue
		tail -n 1 "$d/history.jsonl" | grep -q '"reason":"recovery deadline P06 exceeded"' || continue
		id=$(basename "$d")
		step 0 "liveness-$id" python3 -I scripts/k7-liveness.py "$d"
	done
	while read -r id; do names+=("$EV/logs/$id.stdout"); done < <(grep -oE '"name":"liveness-20[^"]*"' "$EV/commands.jsonl" | sed 's/"name":"\(.*\)"/\1/' | sort -u)
	step 0 "$(next liveness-summary)" jq -c '{run_id, campaign, seed, last_healed_s, recovery: (.recovery_attempts|map_values({ok,info,fail})), nodes: (.nodes_after_last_heal|map_values({role: .last.role, rule: .last.rule, wal_moved}))}' "${names[@]}"
}

index() {
	step 0 "$(next k7results)" go run ./internal/tools/k7results --runs "$EV/runs" --matrix campaigns/k7.json --log "$EV/campaign-k7.log.zst" --out "$EV" \
		--pkg-replays "$EV/package-replays"
}

packages() {
	local d
	step 0 "$(next k7findings)" go run ./internal/tools/k7findings --packages "$EV/packages.json" --out findings
	for d in findings/*/; do
		d=${d%/}
		step 0 "$(next "pkg-sections-$(basename "$d")")" python3 -I scripts/k7-package-k7-sections.py "$d"
	done
}

c3audit() {
	idle
	step 0 "$(next c3-audit)" scripts/k7-c3-audit.sh runs/.k7/c3audit
}

cited() {
	step 0 "$(next store-cited)" scripts/k7-store-cited.sh runs/k5-full "$EV/cited"
	step 0 "$(next render-cited-full)" bin/cpreport render "$EV/cited" --out "$EV/report/cited-full"
}

p25() {
	step 0 "$(next p25-final)" scripts/k7-p25-count.sh
}

ci() {
	step 0 "$(next build)" make build
	[ "$(recorded lint-workflow)" = 0 ] || step 0 "$(next lint-workflow)" make lint-workflow
	local name
	name=$(next ci)
	step any "$name" make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke ci-mutant-smoke ci-report
	[ "$(recorded "$name")" = 0 ] || { echo "k7-evidence: $name exited $(recorded "$name"); see $EV/logs/$name.*" >&2; exit 1; }
}

lastci() { # the latest ci record's name
	grep -oE '"name":"ci(-[0-9]+)?"' "$EV/commands.jsonl" | tail -n 1 | sed 's/"name":"\(.*\)"/\1/'
}

gate() {
	mkdir -p "$SCRATCH/tmp"
	TMPDIR="$PWD/$SCRATCH/tmp" "${REC[@]}" "$(next cpgate-k7)" -- bin/cpgate K7 --evidence "$EV" --negative-controls
}

tagcheck() {
	local ci commit
	ci=$(lastci)
	commit=$(grep -oE 'buildinfo\.Commit=[0-9a-f]{40}' "$EV/logs/$ci.stdout" | head -n 1 | cut -d= -f2)
	[ -n "$commit" ] || { echo "k7-evidence: no commit in $ci's output" >&2; exit 1; }
	step 0 "$(next tag-check)" scripts/k7-tag-check.sh "$commit" d07fca6c83729ee37972b2762d38a12b50f3e6e4
}

case "${1:-}" in
setup | m1oracle | m1triage | images | start | oracle | store | results | render | triage | pkgreplay | liveness | index | packages | pkgreports | c3audit | cited | p25 | ci | gate | tagcheck) "$1" ;;
wait) wait_campaign ;;
*)
	echo "usage: $0 setup|m1oracle|m1triage|images|start|wait|oracle|store|results|render|triage|pkgreplay|liveness|index|packages|pkgreports|c3audit|cited|p25|ci|gate|tagcheck" >&2
	exit 3
	;;
esac
