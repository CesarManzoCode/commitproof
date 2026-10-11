#!/usr/bin/env bash
# ci-checkers.sh — `make ci-checkers` (CONTRACT §13): the K2 synthetic corpus
# regenerates byte for byte; cpcheck classifies every entry as planted; C3
# agrees with elle-cli (pinned oracle image); every checker sabotage fails
# exactly its dependent labels; a pathological register history is UNKNOWN
# (exit 2), never PASS. Verdicts come from internal/tools/k2check (gate
# logic), not from cpcheck's own comparison. Scratch output in runs/.
set -euo pipefail
export GOTOOLCHAIN=go1.26.9 CGO_ENABLED=0
W=runs/.ci-checkers
if [ -d "$W" ]; then chmod -R u+w "$W" && rm -rf "${W:?}"; fi
mkdir -p "$W"
go run ./internal/tools/gencorpus --verify --out testdata
bin/cpcheck corpus testdata/corpus --elle-oracle --out "$W/corpus" >"$W/corpus.out" || true
for rule in $(go run ./internal/tools/k2check --list-rules); do
	scripts/sabotage-corpus.sh "$rule" testdata/corpus "$W/sabotage/$rule" >"$W/sabotage-$rule.out" || true
done
cp -r testdata/pathological/porcupine-timeout/run "$W/porcupine-timeout"
rc=0
bin/cpcheck check "$W/porcupine-timeout" >"$W/pathological.out" || rc=$?
if [ "$rc" -ne 2 ] || ! grep -q '"verdict":"UNKNOWN"' "$W/pathological.out"; then
	echo "ci-checkers: pathological history: exit $rc, $(tail -1 "$W/pathological.out")" >&2
	exit 1
fi
go run ./internal/tools/k2check --corpus testdata/corpus --results "$W/corpus/results.jsonl" \
	--raw "$W/corpus/elle/raw" --divergences evidence/K2/divergences.json --sabotage "$W/sabotage"
