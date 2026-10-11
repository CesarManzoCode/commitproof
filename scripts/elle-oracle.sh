#!/usr/bin/env bash
# elle-oracle.sh — builds the Elle oracle image (CONTRACT §10.11, §14,
# ADR-0004): elle-cli at its pinned commit, compiled inside the pinned
# clojure image by build/Dockerfile.elle. The source is cloned into the
# git-ignored .cache/elle-cli and never vendored. Prints one JSON summary
# line: image, image id, jar sha256, base image, elle-cli commit.
set -u -o pipefail
COMMIT=9c2a18b3719f573dab06ae05c955c9c9d5225673
BASE='clojure:temurin-21-lein-2.13.0-bookworm@sha256:724a7f5840394bf9e659730e5039bf7498f31185e747e3586dbab626b717c34f'
IMAGE=commitproof/elle-oracle:9c2a18b
SRC=.cache/elle-cli
fail() { printf '{"command":"elle-oracle","exit_code":3,"error":"%s"}\n' "$1"; exit 3; }

grep -q "^FROM $BASE\$" build/Dockerfile.elle || fail "build/Dockerfile.elle does not use the pinned base $BASE"
if [ ! -d "$SRC/.git" ]; then
	mkdir -p .cache
	timeout 300 git clone --quiet https://github.com/ligurio/elle-cli.git "$SRC" || fail "clone elle-cli"
fi
timeout 120 git -C "$SRC" fetch --quiet origin "$COMMIT" 2>/dev/null || true
git -C "$SRC" checkout --quiet --detach "$COMMIT" || fail "checkout $COMMIT"
[ "$(git -C "$SRC" rev-parse HEAD)" = "$COMMIT" ] || fail "elle-cli HEAD is not $COMMIT"
[ -z "$(git -C "$SRC" status --porcelain)" ] || fail "elle-cli working tree is modified"
timeout 1800 docker build --quiet -t "$IMAGE" -f build/Dockerfile.elle "$SRC" >&2 || fail "docker build"
id=$(timeout 60 docker image inspect --format '{{.Id}}' "$IMAGE") || fail "inspect"
jar=$(timeout 120 docker run --rm --network none --entrypoint sha256sum "$IMAGE" /elle-cli.jar | cut -d' ' -f1) || fail "jar hash"
ver=$(timeout 120 docker run --rm --network none "$IMAGE" --version | tr -d '\r\n') || fail "version"
printf '{"base":"%s","command":"elle-oracle","elle_cli_commit":"%s","exit_code":0,"image":"%s","image_id":"%s","jar_sha256":"%s","version":"%s"}\n' \
	"$BASE" "$(git -C "$SRC" rev-parse HEAD)" "$IMAGE" "$id" "$jar" "$ver"
