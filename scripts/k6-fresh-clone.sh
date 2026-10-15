#!/usr/bin/env bash
# k6-fresh-clone.sh <reference run archive> — K6.2 evidence (CONTRACT §15
# K6): `git clone` of this repository at HEAD into a temporary directory,
# `go clean -cache -modcache`, build cpreport from the clone, render the
# reference run, report the output's sha256.
#
# The Go caches are the clone's own (GOCACHE and GOMODCACHE inside the
# temporary directory), so `go clean -cache -modcache` empties caches that
# nothing else uses: the build downloads its modules and the go1.26.9
# toolchain again (PREREQUISITES §5) without touching the owner's caches
# (AGENTS.md §3: nothing outside the repository is deleted). The summary
# records that both caches held no file after the clean. No temporary path
# is printed.
set -u -o pipefail
arch=${1:?reference run archive}
id=$(basename "$arch" .tar.zst)
here=$(pwd)
tmp=$(mktemp -d)
trap 'chmod -R u+w "$tmp" 2>/dev/null; rm -rf "$tmp"' EXIT
export GOTOOLCHAIN=go1.26.9 CGO_ENABLED=0 GOCACHE="$tmp/gocache" GOMODCACHE="$tmp/gomodcache" GOFLAGS=-modcacherw GOPATH="$tmp/gopath"
commit=$(git rev-parse HEAD)
git clone --quiet --no-local "$here" "$tmp/repo" >&2
clone_rc=$?
cd "$tmp/repo" && git checkout --quiet "$commit" >&2 || clone_rc=1
head=$(git rev-parse HEAD)
[ "$head" = "$commit" ] || clone_rc=1
go clean -cache -modcache >&2
clean_rc=$?
count() { [ -d "$1" ] && find "$1" -type f ! -name README ! -name trim.txt | wc -l || echo 0; }
mc=$(count "$GOMODCACHE") gc=$(count "$GOCACHE")
emptied=false
[ "$mc" -eq 0 ] && [ "$gc" -eq 0 ] && emptied=true
go build -trimpath -o "$tmp/cpreport" ./cmd/cpreport >&2
build_rc=$?
cd "$here" || exit 3
"$tmp/cpreport" render "$arch" --out "$tmp/out" >"$tmp/render.stdout"
render_rc=$?
tail -n 1 "$tmp/render.stdout"
sum=""
[ -f "$tmp/out/$id.html" ] && sum=$(sha256sum "$tmp/out/$id.html" | cut -d' ' -f1)
gov=$("$GOMODCACHE"/golang.org/toolchain@*/bin/go version 2>/dev/null | head -n 1)
rc=0
[ "$clone_rc" -eq 0 ] && [ "$clean_rc" -eq 0 ] && [ "$build_rc" -eq 0 ] && [ "$render_rc" -eq 0 ] && [ "$emptied" = true ] && [ -n "$sum" ] || rc=1
jq -cn --arg commit "$head" --argjson c "$clone_rc" --argjson g "$clean_rc" --argjson b "$build_rc" --argjson r "$render_rc" \
	--argjson e "$emptied" --argjson mc "$mc" --argjson gc "$gc" --arg sum "$sum" --arg gov "$gov" --argjson rc "$rc" \
	'{command:"k6-fresh-clone",commit:$commit,clone:"git clone --no-local of HEAD",clone_exit:$c,go_clean:"go clean -cache -modcache",go_clean_exit:$g,caches_emptied:$e,modcache_files_after_clean:$mc,gocache_files_after_clean:$gc,build:"go build -trimpath ./cmd/cpreport",build_exit:$b,go_version:$gov,render_exit:$r,render_sha256:$sum,exit_code:$rc}'
exit $rc
