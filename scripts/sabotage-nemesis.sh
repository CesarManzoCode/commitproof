#!/usr/bin/env bash
# sabotage-nemesis.sh — K3.2/K3.3 fixtures: for every nemesis sabotage tag
# (internal/nemesis/sabotage_<rule>_on.go), a host binary built with
# cp_sabotage_<rule> into bin/sabotage/<rule>/commitproof, and the runner
# image built by that binary with the same tag
# (commitproof/runner:sab-<rule>, labelled; never locked). Product binaries
# and images are untouched.
set -euo pipefail
export GOTOOLCHAIN=go1.26.9 CGO_ENABLED=0
PKG=github.com/CesarManzoCode/commitproof
commit=$(git rev-parse HEAD)
dirty=$(test -z "$(git status --porcelain -- . ':(exclude)evidence' ':(exclude)images.lock.json')" && echo false || echo true)
rules=${*:-$(ls internal/nemesis/sabotage_*_on.go | sed -E 's|.*/sabotage_(.*)_on\.go|\1|')}
for rule in $rules; do
	out=bin/sabotage/$rule
	mkdir -p "$out"
	go build -trimpath -tags "cp_sabotage_$rule" \
		-ldflags "-X $PKG/internal/buildinfo.Commit=$commit -X $PKG/internal/buildinfo.Dirty=$dirty" \
		-o "$out/commitproof" ./cmd/commitproof
	"$out/commitproof" images sabotage-runner | tail -n 1
done
