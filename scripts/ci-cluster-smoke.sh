#!/usr/bin/env bash
# ci-cluster-smoke.sh — CONTRACT §13 `make ci-cluster-smoke`: build the pinned
# images (builder cache), cluster up/verify/down, one 45 s smoke run (which
# brings its own cluster up and down), cpcheck verify-seal on the run.
# The cluster is always torn down, also on failure.
set -euo pipefail

CP=bin/commitproof
OUT=${CP_SMOKE_OUT:-runs}

cleanup() { "$CP" cluster down --profile baseline >/dev/null 2>&1 || true; }
trap cleanup EXIT

"$CP" images build
"$CP" cluster up --profile baseline
"$CP" cluster verify --profile baseline
"$CP" cluster down --profile baseline

mkdir -p "$OUT"
summary=$("$CP" run --campaign smoke --seed 42 --out "$OUT" | tail -n 1)
echo "$summary"
run_dir=$(sed -n 's/.*"run_dir":"\([^"]*\)".*/\1/p' <<<"$summary")
[ -n "$run_dir" ] || { echo "ci-cluster-smoke: run printed no run_dir" >&2; exit 2; }
bin/cpcheck verify-seal "$run_dir"
