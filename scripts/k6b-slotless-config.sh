#!/usr/bin/env bash
# k6b-slotless-config.sh — ADR-0041: profile slotless differs from baseline
# only in --enable-slot-based-replication on the seven poolers and gateways.
# Renders both profiles' Compose configuration (docker compose config, with
# the n4 profile, the default interpolation values of a run) and prints
# every path whose value differs; exit 1 if anything else differs.
set -u -o pipefail
export CP_SUBNET=10.77.0.0/24 CP_NET_PREFIX=10.77.0 CP_IMAGE_TAG=real CP_RUNNER_TAG=real CP_MULTIGRES_TAG=real CP_DOCKER_GID=0
unset CP_OTEL_METRICS_EXPORTER
render() { timeout 60 docker compose --project-name commitproof "$@" --project-directory deploy --profile n4 config --format json; }
b=$(render --file deploy/compose.yaml) || exit 3
s=$(render --file deploy/compose.yaml --file deploy/compose-slotless.yaml) || exit 3
diffs=$(jq -cn --argjson b "$b" --argjson s "$s" '
	[($b | paths(scalars)) as $p | select(($b | getpath($p)) != ($s | getpath($p))) | {path: ($p | map(tostring) | join(".")), baseline: ($b | getpath($p)), slotless: ($s | getpath($p))}]
	+ [($s | paths(scalars)) as $p | select(($b | getpath($p)) == null) | {path: ($p | map(tostring) | join(".")), baseline: null, slotless: ($s | getpath($p))}]')
ok=$(jq -c 'all(.[]; .baseline == "--enable-slot-based-replication=true" and .slotless == "--enable-slot-based-replication=false") and length == 7' <<<"$diffs")
services=$(jq -c '[.[] | .path | split(".")[1]] | unique' <<<"$diffs")
rc=0
[ "$ok" = true ] || rc=1
echo "{\"command\":\"k6b-slotless-config\",\"differences\":$diffs,\"services\":$services,\"only_the_flag_on_seven_services\":$ok,\"exit_code\":$rc}"
exit $rc
