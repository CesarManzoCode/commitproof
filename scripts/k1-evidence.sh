#!/usr/bin/env bash
# k1-evidence.sh — produces evidence/K1 for gate K1 (CONTRACT §15 K1) by
# running every command the gate table names, each one through
# scripts/record-command.sh (argv, exit code, stdout/stderr sha256 in
# evidence/K1/commands.jsonl). It stops at the first failing step: a failure
# is evidence, not something to retry around.
#
# Run from the repository root on a committed tree (the binaries record the
# commit; uncommitted code makes them "dirty").
set -u -o pipefail

EV=${CP_EVIDENCE:-evidence/K1}
REC=(scripts/record-command.sh "$EV/commands.jsonl" "$EV/logs")
# Scratch state inside the git-ignored runs/ (relative: recorded argv carry
# no host-specific paths, AGENTS.md §4).
STATE=runs/.k1-evidence-state
rm -rf "$STATE" && mkdir -p "$STATE"
trap 'rm -rf "$STATE"' EXIT

step() { # name argv...
	local name=$1
	shift
	echo "== $name: $*" >&2
	# Sub-commands (docker, git, compose) of commitproof are recorded too.
	CP_CMDLOG_DIR="$EV/cmdlog/$name" "${REC[@]}" "$name" -- "$@" >/dev/null
	local rc=$?
	if [ "$rc" -ne 0 ]; then
		echo "k1-evidence: step $name exited $rc (see $EV/logs/$name.stderr)" >&2
		bin/commitproof cluster down --profile baseline >/dev/null 2>&1 || true
		exit "$rc"
	fi
}

only=${1:-all}
want() { [ "$only" = all ] || [ "$only" = "$1" ]; }

step "build-$only" make build

if want images; then
	# K1.1 and K1.2: build, prune the builder cache (allowed only here,
	# AGENTS.md §5), rebuild without cache; keep both locks.
	step images-build-1 bin/commitproof images build
	cp images.lock.json "$EV/images.lock.build1.json"
	step builder-prune docker builder prune -af
	step images-build-2 bin/commitproof images build --no-cache
	cp images.lock.json "$EV/images.lock.build2.json"
fi

if want derivation; then
	# K1.3
	rm -rf "$EV/derivation"
	step check-dockerfile-derivation make check-dockerfile-derivation DERIVATION_OUT="$EV/derivation"
fi

if want updown; then
	# K1.4: three attempts from zero; one `cluster up` each, never retried.
	for k in 1 2 3; do
		step "updown-$k-up" bin/commitproof cluster up --profile baseline
		step "updown-$k-verify" bin/commitproof cluster verify --profile baseline
		step "updown-$k-down" bin/commitproof cluster down --profile baseline
	done
fi

if want plan; then
	# K1.6
	step plan-42-a bin/commitproof plan --campaign smoke --seed 42
	step plan-42-b bin/commitproof plan --campaign smoke --seed 42
	step plan-42-c bin/commitproof plan --campaign smoke --seed 42
	step go-clean-cache go clean -cache
	step rebuild make build
	step plan-42-d bin/commitproof plan --campaign smoke --seed 42
	step plan-43 bin/commitproof plan --campaign smoke --seed 43
fi

if want smoke; then
	# K1.5 and K1.7: the smoke run with docker stats sampled every second.
	mkdir -p "$EV/runs"
	"${REC[@]}" memsample -- bin/commitproof __memsample --out "$EV/memory-samples.jsonl" --stop-file "$STATE/stop" >/dev/null &
	sampler=$!
	sleep 2
	echo "== smoke-run" >&2
	"${REC[@]}" smoke-run -- bin/commitproof run --campaign smoke --seed 42 --out "$EV/runs" >/dev/null
	rc=$?
	sleep 2
	touch "$STATE/stop"
	wait "$sampler"
	[ "$rc" -eq 0 ] || { echo "k1-evidence: smoke-run exited $rc" >&2; exit "$rc"; }
	run_dir=$(tail -n 1 "$EV/logs/smoke-run.stdout" | sed -n 's/.*"run_dir":"\([^"]*\)".*/\1/p')
	step verify-seal bin/cpcheck verify-seal "$run_dir"
fi

if want lint; then
	# Workflow validated locally with the pinned actionlint image (K1 prompt §3).
	step lint-workflow make lint-workflow
	step check-flags scripts/check-flags.sh
fi

if want ci; then
	# K1.8
	step ci make ci-unit ci-cluster-smoke
fi

if want gate; then
	"${REC[@]}" cpgate-k1 -- bin/cpgate K1 --evidence "$EV" --negative-controls
	exit $?
fi
