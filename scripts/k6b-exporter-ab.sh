#!/usr/bin/env bash
# k6b-exporter-ab.sh <on|off> <out> — the exporter measurement of ADR-0041
# (owner decision 5): the CI smoke plan (`smoke`, seed 42: W1–W4, 45 s, no
# fault) run with ADR-0038's exporter ("on", as K7 runs) or without it
# ("off": CP_OTEL_METRICS_EXPORTER=none, the K1–K6 configuration, which the
# caller sets in this command's environment so the recorded argv shows it).
# While the run lasts, a sampler reads every second, for each container of
# the project, its cgroup's cumulative CPU time (cpu.stat usage_usec) and
# memory (memory.current, memory.peak), and once its environment's
# OTEL_METRICS_EXPORTER (docker inspect). Then cpcheck checks the run and
# one JSON summary is printed: the run's verdict, per-workload outcomes and
# ok-latency percentiles (from history.jsonl), the scrape outcomes (from
# observations.jsonl), and per-container CPU and peak memory. A
# measurement, not evidence for any K7 cell (ADR-0041).
set -u -o pipefail
mode=${1:?on|off} out=${2:?out}
case $mode in
on) [ -z "${CP_OTEL_METRICS_EXPORTER:-}" ] || { echo '{"error":"mode on with CP_OTEL_METRICS_EXPORTER set","exit_code":3}'; exit 3; } ;;
off) [ "${CP_OTEL_METRICS_EXPORTER:-}" = none ] || { echo '{"error":"mode off needs CP_OTEL_METRICS_EXPORTER=none","exit_code":3}'; exit 3; } ;;
*) echo '{"error":"mode: on|off","exit_code":3}'; exit 3 ;;
esac
mkdir -p "$out/runs-$mode"
samples=$out/samples-$mode.jsonl stop=$out/stop-$mode
: >"$samples"
rm -f "$stop"
(
	declare -A name env
	while [ ! -e "$stop" ]; do
		ts=$(date -u +%s%3N)
		for id in $(docker ps -q --no-trunc --filter label=com.docker.compose.project=commitproof 2>/dev/null); do
			cg=/sys/fs/cgroup/system.slice/docker-$id.scope
			[ -r "$cg/cpu.stat" ] || continue
			if [ -z "${name[$id]:-}" ]; then
				line=$(timeout 10 docker inspect --format '{{.Name}}|{{range .Config.Env}}{{.}};{{end}}' "$id" 2>/dev/null) || continue
				name[$id]=${line%%|*}
				name[$id]=${name[$id]#/}
				e=$(printf '%s' "${line#*|}" | tr ';' '\n' | sed -n 's/^OTEL_METRICS_EXPORTER=//p')
				env[$id]=${e:-unset}
			fi
			cpu=$(awk '$1=="usage_usec"{print $2}' "$cg/cpu.stat" 2>/dev/null)
			cur=$(cat "$cg/memory.current" 2>/dev/null)
			peak=$(cat "$cg/memory.peak" 2>/dev/null)
			[ -n "$cpu" ] || continue
			printf '{"ts_ms":%s,"name":"%s","otel_metrics_exporter":"%s","cpu_usec":%s,"mem_current":%s,"mem_peak":%s}\n' \
				"$ts" "${name[$id]}" "${env[$id]}" "$cpu" "${cur:-0}" "${peak:-0}"
		done >>"$samples"
		sleep 1
	done
) &
sampler=$!
bin/commitproof run --campaign smoke --seed 42 --out "$out/runs-$mode" >"$out/run-$mode.out" 2>"$out/run-$mode.err"
rc=$?
touch "$stop"
wait "$sampler"
dir=$(sed -n 's/.*"run_dir":"\([^"]*\)".*/\1/p' "$out/run-$mode.out" | tail -n 1)
if [ "$rc" -ne 0 ] || [ -z "$dir" ] || [ ! -d "$dir" ]; then
	echo "{\"mode\":\"$mode\",\"error\":\"run exit $rc\",\"run\":$(tail -n 1 "$out/run-$mode.out" | jq -c . 2>/dev/null || echo null),\"exit_code\":1}"
	exit 1
fi
bin/cpcheck check "$dir" >"$out/cpcheck-$mode.out" 2>&1
crc=$?
id=$(basename "$dir")
hist=$(jq -sc '
	(map(select(.type=="invoke" and .f!="final-read")) | map({key:(.i|tostring), value:{wl, t}}) | from_entries) as $inv
	| map(select((.type=="ok" or .type=="fail" or .type=="info") and ($inv[(.op|tostring)] != null)))
	| map({wl: $inv[(.op|tostring)].wl, type, lat_us: (((.t - $inv[(.op|tostring)].t) / 1000) | floor)})
	| group_by(.wl)
	| map({key: .[0].wl, value: {
		completions: length,
		ok: (map(select(.type=="ok")) | length),
		fail: (map(select(.type=="fail")) | length),
		info: (map(select(.type=="info")) | length),
		ok_latency_us: (map(select(.type=="ok") | .lat_us) | sort | . as $l | ($l|length) as $n
			| if $n == 0 then null else {p50: $l[((0.50*$n)|ceil)-1], p95: $l[((0.95*$n)|ceil)-1], p99: $l[((0.99*$n)|ceil)-1], max: $l[$n-1]} end)}})
	| from_entries' "$dir/history.jsonl")
scr=$(jq -sc 'map(.value? // . | select(.src=="metrics")) | {scrapes: length, ok: (map(select(.ok==true)) | length), failed: (map(select(.ok!=true)) | length)}' "$dir/observations.jsonl")
cont=$(jq -sc 'group_by(.name) | map({key: .[0].name, value: {otel_metrics_exporter: .[0].otel_metrics_exporter, cpu_ms: ((map(.cpu_usec) | max) / 1000 | floor), mem_peak_bytes: (map(.mem_peak) | max), samples: length}}) | from_entries' "$samples")
verdict=$(jq -r '.verdict' "$dir.checks.json" 2>/dev/null)
wall=$(jq -sc '{last_marker_ms: (map(select(.type=="marker")) | last.t / 1e6 | floor)}' "$dir/history.jsonl" 2>/dev/null)
jq -cn --arg mode "$mode" --arg run "$id" --arg verdict "$verdict" --argjson crc "$crc" --argjson hist "$hist" --argjson scr "$scr" \
	--argjson cont "$cont" --argjson wall "${wall:-null}" --arg cv "$(jq -r .contract_version "$dir/plan.json")" \
	'{command:"k6b-exporter-ab", mode:$mode, run:$run, plan_contract_version:$cv, cpcheck_exit:$crc, verdict:$verdict,
	  history_end:$wall, workloads:$hist, metrics_scrapes:$scr, containers:$cont, exit_code:0}'
