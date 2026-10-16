#!/usr/bin/env bash
# k6b-smoke-wait.sh <matrix> <out> <capture.jsonl> — K6b's smoke
# verification of `make campaign-k7` (ADR-0041): waits for the campaign
# launched in the background (campaign-k7.sh wait) and meanwhile captures,
# once per container, the command line of every multipooler and
# multigateway the campaign's clusters run (docker inspect .Config.Cmd), so
# the record shows the slot flag in force in each run. Prints the capture's
# summary (per container start: name, created, the flag), then exits with
# the campaign's exit code.
set -u -o pipefail
matrix=${1:?matrix} out=${2:?out} cap=${3:?capture file}
: >"$cap"
stop=$(mktemp runs/.k6b/stop-XXXXXX)
rm -f "$stop"
(
	declare -A seen
	while [ ! -e "$stop" ]; do
		for id in $(docker ps -q --no-trunc --filter label=com.docker.compose.project=commitproof 2>/dev/null); do
			[ -n "${seen[$id]:-}" ] && continue
			line=$(timeout 10 docker inspect --format '{{json .Name}} {{json .Created}} {{json .Config.Cmd}}' "$id" 2>/dev/null) || continue
			seen[$id]=1
			read -r name created cmd <<<"$line"
			case $name in *pooler-n* | *gw-z*) ;; *) continue ;; esac
			jq -cn --argjson name "$name" --argjson created "$created" --argjson cmd "$cmd" \
				'{name: ($name|ltrimstr("/")), created: $created, slot_flag: ([$cmd[] | select(startswith("--enable-slot-based-replication="))][0] // "absent")}'
		done >>"$cap"
		sleep 2
	done
) &
cpid=$!
scripts/campaign-k7.sh wait "$matrix" "$out"
rc=$?
touch "$stop"
wait "$cpid"
rm -f "$stop"
jq -sc '{command: "k6b-smoke-capture", containers: length, by_flag: (group_by(.slot_flag) | map({key: .[0].slot_flag, value: (map(.name) | unique)}) | from_entries)}' "$cap"
exit $rc
