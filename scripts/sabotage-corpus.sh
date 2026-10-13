#!/usr/bin/env bash
# sabotage-corpus.sh — CONTRACT §15 K2.6 and K5.1: builds cpcheck with the
# checker sabotage tag cp_sabotage_<rule> (never a product build; the binary
# goes to the git-ignored runs/) and runs it on a synthetic corpus. The
# corpus run is expected to fail exactly the labels that depend on the rule
# (internal/gate/k2deps.go, k5deps.go); cpgate judges that, not this script.
#
# usage: scripts/sabotage-corpus.sh <rule> <corpus-dir> <out-dir>
set -u -o pipefail
rule=${1:?rule} corpus=${2:?corpus dir} out=${3:?out dir}
export GOTOOLCHAIN=go1.26.9 CGO_ENABLED=0
bin=runs/.sabotage/cpcheck-$rule
mkdir -p runs/.sabotage
go build -trimpath -tags "cp_sabotage_$rule" -o "$bin" ./cmd/cpcheck || exit 3
"$bin" corpus "$corpus" --out "$out" --results-only
