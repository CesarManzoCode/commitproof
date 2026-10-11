#!/usr/bin/env bash
# k2-evidence.sh — produces evidence/K2 for gate K2 (CONTRACT §15 K2): every
# command the gate reads runs through scripts/record-command.sh (argv, exit
# code, stdout/stderr sha256 in evidence/K2/commands.jsonl). A step whose
# exit code differs from the expected one stops the script: a failure is
# evidence, not something to retry around. Run from the repository root on
# a committed tree.
set -u -o pipefail
EV=evidence/K2
REC=(scripts/record-command.sh "$EV/commands.jsonl" "$EV/logs")
step() { # expected-exit name argv...
	local want=$1 name=$2
	shift 2
	echo "== $name: $*" >&2
	"${REC[@]}" "$name" -- "$@" >/dev/null
	local rc=$?
	if [ "$rc" -ne "$want" ]; then
		echo "k2-evidence: step $name exited $rc, expected $want (see $EV/logs/$name.stderr)" >&2
		exit 1
	fi
}
mkdir -p "$EV/runs"
step 0 k1-precondition scripts/k1-precondition.sh
step 0 build make build
step 0 elle-oracle make elle-oracle
step 0 gencorpus-verify go run ./internal/tools/gencorpus --verify --out testdata
step 0 corpus bin/cpcheck corpus testdata/corpus --elle-oracle --out "$EV/corpus"
step 0 pathological-copy cp -r testdata/pathological/porcupine-timeout/run "$EV/runs/porcupine-timeout"
step 2 pathological bin/cpcheck check "$EV/runs/porcupine-timeout"
step 0 wire-tests go test -count=1 -json -run '^TestWire' ./internal/workload
for rule in $(go run ./internal/tools/k2check --list-rules); do
	step 1 "sabotage-$rule" scripts/sabotage-corpus.sh "$rule" testdata/corpus "$EV/sabotage/$rule"
done
step 0 no-sabotage-symbols go test -count=1 -v -run '^TestProductBinariesHaveNoSabotage$' ./internal/independence
# Informational: the checkers on K1's real smoke run (a copy in runs/; the
# result is recorded whatever it is).
K1RUN=$(ls -d evidence/K1/runs/*/ | head -1)
K1RUN=${K1RUN%/}
if [ -d runs/.k2-k1smoke ]; then chmod -R u+w runs/.k2-k1smoke && rm -rf runs/.k2-k1smoke; fi
mkdir -p runs/.k2-k1smoke && cp -r "$K1RUN" runs/.k2-k1smoke/ && chmod -R u+w runs/.k2-k1smoke
"${REC[@]}" k1-smoke-check -- bin/cpcheck check --elle-oracle "runs/.k2-k1smoke/$(basename "$K1RUN")" >/dev/null || true
step 0 lint-workflow make lint-workflow
step 0 ci make ci-unit ci-checkers
