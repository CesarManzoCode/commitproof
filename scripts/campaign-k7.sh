#!/usr/bin/env bash
# campaign-k7.sh — `make campaign-k7` and `make campaign-k7-status`
# (CONTRACT §13, §15 K7; ADR-0041). K7's matrix runs through
# `commitproof campaign`, detached from the terminal (setsid nohup), its
# output appended to <out>/campaign-k7.log, its pid in <out>/campaign-k7.pid.
# It is resumable: re-invoking `start` after an interruption continues where
# the driver stopped (sealed runs are reused, never recomputed). The
# driver's lock on <out> refuses a second campaign; ADR-0041 refuses any
# matrix run without ADR-0038's exporter (CP_OTEL_METRICS_EXPORTER set).
#
# usage: campaign-k7.sh start|status|dry-run|wait <matrix> <out>
#   start    launch the campaign in the background; print its pid and log
#   status   running or not, and the dry run's summary of <out>
#   dry-run  the cells with their plans and the run count; starts nothing
#   wait     wait until the launched campaign exits; print its summary line
#            and exit with its exit code (used by K6b's smoke verification)
set -u -o pipefail
cmd=${1:?start|status|dry-run|wait} matrix=${2:?matrix} out=${3:?out}
log=$out/campaign-k7.log pidf=$out/campaign-k7.pid
CP=bin/commitproof

alive() { [ -f "$pidf" ] && kill -0 "$(cat "$pidf")" 2>/dev/null; }
json() { printf '{"command":"campaign-k7","action":"%s","matrix":"%s","out":"%s","log":"%s"%s}\n' "$cmd" "$matrix" "$out" "$log" "$1"; }

case $cmd in
dry-run)
	exec "$CP" campaign --matrix "$matrix" --out "$out" --dry-run
	;;
start)
	if [ -n "${CP_OTEL_METRICS_EXPORTER:-}" ]; then
		json ',"error":"CP_OTEL_METRICS_EXPORTER is set; K7 runs keep the exporter (ADR-0041)","exit_code":3'
		exit 3
	fi
	# The binaries must be the ones the runner image was built from
	# (images.lock.json's commitproof commit, clean); `make campaign-k7`
	# does not rebuild them.
	bc=$("$CP" version 2>/dev/null | tail -n 1 | jq -r '.commit + " " + (.dirty|tostring)' 2>/dev/null)
	lc=$(jq -r '.commitproof.commit + " " + (.commitproof.dirty|tostring)' images.lock.json 2>/dev/null)
	if [ -z "$bc" ] || [ "$bc" != "$lc" ] || [ "${bc#* }" != false ]; then
		json ",\"error\":\"bin/commitproof is ${bc:-missing}, images.lock.json's runner is ${lc:-missing}: make build, then bin/commitproof images build\",\"exit_code\":3"
		exit 3
	fi
	mkdir -p "$out"
	if alive; then
		json ",\"error\":\"already running (pid $(cat "$pidf"))\",\"exit_code\":3"
		exit 3
	fi
	echo "== $(date -u +%Y-%m-%dT%H:%M:%SZ) campaign-k7 start: $CP campaign --matrix $matrix --out $out --cpcheck bin/cpcheck" >>"$log"
	setsid nohup "$CP" campaign --matrix "$matrix" --out "$out" --cpcheck bin/cpcheck >>"$log" 2>&1 </dev/null &
	echo $! >"$pidf"
	# A refused start (another campaign's lock, a bad matrix) exits at once.
	for _ in 1 2 3 4 5; do
		alive || break
		sleep 1
	done
	if ! alive && ! grep -q '"event":"run"' "$log"; then
		json ",\"error\":\"the campaign exited at once; see the log\",\"last\":$(tail -n 1 "$log" | jq -c . 2>/dev/null || echo null),\"exit_code\":3"
		exit 3
	fi
	json ",\"pid\":$(cat "$pidf"),\"exit_code\":0"
	;;
status)
	running=false
	alive && running=true
	dr=$("$CP" campaign --matrix "$matrix" --out "$out" --dry-run | tail -n 1)
	last=$(grep '^{' "$log" 2>/dev/null | tail -n 1)
	json ",\"running\":$running,\"dry_run\":${dr:-null},\"last_event\":${last:-null},\"exit_code\":0"
	;;
wait)
	while alive; do sleep 15; done
	sum=$(grep '"command":"campaign"' "$log" | tail -n 1)
	[ -n "$sum" ] || { json ',"error":"no campaign summary in the log","exit_code":2'; exit 2; }
	# Runs started since the last start (0 when a re-invocation only resumed).
	started=$(awk '/^== .* campaign-k7 start:/{n=0} /"event":"run"/{n++} END{print n+0}' "$log")
	code=$(echo "$sum" | jq -r '.exit_code')
	json ",\"runs_started_since_last_start\":$started,\"campaign\":$sum,\"exit_code\":$code"
	exit "$code"
	;;
*)
	echo "usage: $0 start|status|dry-run|wait <matrix> <out>" >&2
	exit 3
	;;
esac
