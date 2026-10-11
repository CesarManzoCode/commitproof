#!/usr/bin/env bash
# k3-evidence.sh — produces evidence/K3 for gate K3 (CONTRACT §15 K3): every
# command the gate reads runs through scripts/record-command.sh (argv, exit
# code, stdout/stderr sha256 in evidence/K3/commands.jsonl). A step whose
# exit code differs from the expected one stops the script ("any": the
# result is recorded whatever it is — cpcheck verdicts on real runs, K3.5).
# Run from the repository root on a committed tree, one cluster at a time.
set -u -o pipefail
EV=evidence/K3
REC=(scripts/record-command.sh "$EV/commands.jsonl" "$EV/logs")
step() { # expected-exit|any name argv...
	local want=$1 name=$2
	shift 2
	echo "== $(date -u +%H:%M:%S) $name: $*" >&2
	"${REC[@]}" "$name" -- "$@" >/dev/null
	local rc=$?
	if [ "$want" != any ] && [ "$rc" -ne "$want" ]; then
		echo "k3-evidence: step $name exited $rc, expected $want (see $EV/logs/$name.stderr)" >&2
		bin/commitproof cluster down --profile baseline >/dev/null 2>&1 || true
		exit 1
	fi
}
run_dir() { tail -n 1 "$EV/logs/$1.stdout" | sed -n 's/.*"run_dir":"\([^"]*\)".*/\1/p'; }
camp() { case $1 in f5a | f5b) echo f5 ;; *) echo "$1" ;; esac; }

mkdir -p "$EV/runs"
step 0 k3-precondition scripts/k3-precondition.sh
step 0 build make build
step 0 images-build bin/commitproof images build
step 0 check-flags scripts/check-flags.sh
step 0 sabotage-nemesis make sabotage-nemesis
# slowfs at 0 ms: overhead, and the F4b decision inputs (HEALTHY, smoke checkers).
step 0 slowfs-overhead scripts/slowfs-overhead.sh
step 0 slowfs-smoke-run bin/commitproof run --campaign smoke --seed 42 --out "$EV/runs"
step any slowfs-smoke-check bin/cpcheck check "$(run_dir slowfs-smoke-run)"
# K3.1 (one run per fault kind, F5a+F5b together, one composite) and K3.5.
for c in f1a f1b f1c f1d f2a f2b f2c f2d f2e f3a f3b f4a f5 f6 f7 composite; do
	step 0 "k3-run-$c" bin/commitproof run --campaign "k3-$c" --seed 1 --out "$EV/runs"
	step any "k3-check-$c" bin/cpcheck check "$(run_dir "k3-run-$c")"
done
# K3.2: each injector sabotaged to a no-op → not-applied, INVALID, exit 2.
for k in f1a f1b f1c f1d f2a f2b f2c f2d f2e f3a f3b f4a f5a f5b f6 f7; do
	step 2 "k3-sab-inject-$k" "bin/sabotage/inject_$k/commitproof" run --campaign "k3-$(camp $k)" --seed 1 --out "$EV/runs"
	step 2 "k3-sabcheck-inject-$k" bin/cpcheck check "$(run_dir "k3-sab-inject-$k")"
done
# K3.3: heal sabotaged → not-healed, INVALID.
for k in f1a f3a; do
	step 2 "k3-sab-heal-$k" "bin/sabotage/heal_$k/commitproof" run --campaign "k3-$k" --seed 1 --out "$EV/runs"
	step 2 "k3-sabcheck-heal-$k" bin/cpcheck check "$(run_dir "k3-sab-heal-$k")"
done
# Observed reaction of Multigres per fault (diagnosis for NOTES, ADR-0016).
dirs=()
for c in f1a f1b f1c f1d f2a f2b f2c f2d f2e f3a f3b f4a f5 f6 f7 composite; do dirs+=("$(run_dir "k3-run-$c")"); done
step 0 k3obs go run ./internal/tools/k3obs "${dirs[@]}"
step 0 lint-workflow make lint-workflow
step 0 ci make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke
step 0 compress-runs scripts/compress-runs.sh "$EV/runs"
"${REC[@]}" cpgate-k3 -- bin/cpgate K3 --evidence "$EV" --negative-controls
