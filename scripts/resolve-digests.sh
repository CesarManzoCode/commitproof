#!/usr/bin/env bash
# resolve-digests.sh — re-resolves every image pin of CONTRACT §14 with
# `docker buildx imagetools inspect` and writes the result as JSON (stdout).
# Each entry: the tag, the digest the contract pinned (if any), the digest the
# registry returns now, whether they match, and the linux/amd64 manifest.
# Only the index is fetched (one manifest GET per tag): Docker Hub allows 100
# anonymous manifest GETs per hour, and per-platform lookups exhaust it.
# Every registry call is bounded (P01-derived 60 s: registry round trips).
set -u -o pipefail

# ref|contract digest ("" = to be resolved)
PINS=(
	"golang:1.26.9-alpine|sha256:cdfd4fe2da6b225d8b40c6b7a105736e548e83ff56d5d8f9394446eeb5eb84e0"
	"debian:bookworm-slim|sha256:abd67ffcfa541b485a3dff59865ab629aa048a6c613e639d36e7456b0b229241"
	"postgres:17.7|sha256:2006493727bd5277eece187319af1ef82b4cf82cf4fc1ed00da0775b646ac2a4"
	"quay.io/coreos/etcd:v3.6.4|"
	"alpine:3.24.2|sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6"
	"clojure:temurin-21-lein-2.13.0-bookworm|sha256:724a7f5840394bf9e659730e5039bf7498f31185e747e3586dbab626b717c34f"
	"rhysd/actionlint:${ACTIONLINT_VERSION:?set ACTIONLINT_VERSION (latest release, without v)}|"
)

# inspect: up to 3 attempts 5 s apart (registries return transient 5xx).
inspect() {
	local n out
	for n in 1 2 3; do
		if out=$(timeout 60s docker buildx imagetools inspect "$@"); then
			printf '%s' "$out"
			return 0
		fi
		echo "resolve-digests: attempt $n of inspect $* failed" >&2
		sleep 5
	done
	return 1
}

first=1
printf '{"images":['
for pin in "${PINS[@]}"; do
	ref=${pin%%|*} want=${pin#*|}
	m=$(inspect "$ref" --format '{{json .Manifest}}') || {
		echo "resolve-digests: inspect $ref failed" >&2
		exit 2
	}
	got=$(jq -r .digest <<<"$m")
	amd64=$(jq -r '[.manifests[]? | select(.platform.os=="linux" and .platform.architecture=="amd64")][0].digest // .digest' <<<"$m")
	match=null
	[ -n "$want" ] && { [ "$want" = "$got" ] && match=true || match=false; }
	[ $first = 1 ] || printf ','
	first=0
	jq -cn --arg ref "$ref" --arg want "$want" --arg got "$got" --arg amd64 "$amd64" --argjson match "$match" \
		'{ref:$ref, contract_digest:(if $want=="" then null else $want end), resolved_digest:$got, match:$match, linux_amd64_manifest:$amd64}' | tr -d '\n'
done
printf '],"resolved_at":"%s","tool":"%s"}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$(docker buildx version | tr -d '\n')"
