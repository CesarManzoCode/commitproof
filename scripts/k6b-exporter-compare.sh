#!/usr/bin/env bash
# k6b-exporter-compare.sh <on.stdout> <off.stdout> <runs-dir> — ADR-0041's
# exporter measurement, compared: from the two k6b-exporter-ab summaries
# (records exporter-ab-on, exporter-ab-off), CPU time and peak memory summed
# per component (gateways, orchestrators, poolers, PostgreSQL nodes, the
# runner) and per role (the leader's pooler and PostgreSQL, read from each
# run's run-start HEALTHY observation under <runs-dir>), operation counts and
# ok-latency percentiles per workload, scrape outcomes, verdicts. Ratios are
# off → on in per mille (integers). A measurement of one pair, not a
# conclusion.
set -euo pipefail
on=$(tail -n 1 "${1:?on}") off=$(tail -n 1 "${2:?off}") runs=${3:?runs dir}
leader() { jq -r 'select(.value.src=="healthy") | .value.rule.leader' "$runs"/runs-"$1"/"$2"/observations.jsonl | head -n 1; }
lon=$(leader on "$(jq -r .run <<<"$on")") loff=$(leader off "$(jq -r .run <<<"$off")")
agg='def k: sub("^commitproof-"; "") | sub("-1$"; "");
	def group(f): [.containers | to_entries[] | select(.key | k | test(f))];
	def sum(f): (group(f) | {cpu_ms: (map(.value.cpu_ms) | add // 0), mem_peak_mb_sum: ((map(.value.mem_peak_bytes) | add // 0) / 1048576 | floor), n: length});
	{gateways: sum("^gw-z"), orchestrators: sum("^orch-z"), poolers: sum("^pooler-n"), postgres: sum("^pg-n"), runner: sum("^runner$"),
	 leader_pooler: sum("^pooler-" + $l + "$"), leader_postgres: sum("^pg-" + $l + "$"),
	 exporter_env: ([.containers | to_entries[] | select(.key | k | test("^(gw|orch|pooler)-")) | .value.otel_metrics_exporter] | unique)}'
a=$(jq -c --arg l "$lon" "$agg" <<<"$on") b=$(jq -c --arg l "$loff" "$agg" <<<"$off")
jq -cn --argjson on "$on" --argjson off "$off" --argjson a "$a" --argjson b "$b" --arg lon "$lon" --arg loff "$loff" '
	def permille(x; y): if x == 0 then null else ((y * 1000 / x) | floor) end;
	{command: "k6b-exporter-compare",
	 runs: {on: $on.run, off: $off.run}, leaders: {on: $lon, off: $loff}, verdicts: {on: $on.verdict, off: $off.verdict},
	 scrapes: {on: $on.metrics_scrapes, off: $off.metrics_scrapes}, exporter_env: {on: $a.exporter_env, off: $b.exporter_env},
	 cpu_ms: ([("gateways","orchestrators","poolers","postgres","runner","leader_pooler","leader_postgres")] | map({key: ., value: {on: $a[.].cpu_ms, off: $b[.].cpu_ms, off_per_mille_of_on: permille($a[.].cpu_ms; $b[.].cpu_ms)}}) | from_entries),
	 mem_peak_mb_sum: ([("gateways","orchestrators","poolers","postgres","runner")] | map({key: ., value: {on: $a[.].mem_peak_mb_sum, off: $b[.].mem_peak_mb_sum}}) | from_entries),
	 workloads: ([$on.workloads | keys[]] | map(. as $w | {key: $w, value: {completions: {on: $on.workloads[$w].completions, off: $off.workloads[$w].completions},
		ok: {on: $on.workloads[$w].ok, off: $off.workloads[$w].ok},
		ok_latency_us: {on: $on.workloads[$w].ok_latency_us, off: $off.workloads[$w].ok_latency_us}}}) | from_entries),
	 exit_code: 0}'
