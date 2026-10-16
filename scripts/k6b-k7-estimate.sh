#!/usr/bin/env bash
# k6b-k7-estimate.sh — item 9 of K6b: the inputs of K7's re-estimate, from
# measurements only. For every cell of campaigns/k7.json: its plan (generated
# by this commit's `commitproof plan`) and the bytes of its ADR-0039 summary
# (the plan's stub archive — manifest, plan, seal — estimated as the zstd -19
# size of the plan plus the measured manifest+seal overhead of the K6b smoke
# stubs, and a checks.json of the K4/K5 mean size); the measured wall time
# per run (pre-K7 record `durations`; C12 from the K6b smoke run); the
# bytes per real-build finding measured on K4's and K5's findings (original
# and minimized full archives, candidates and replays as summaries); the
# size of evidence/ now. One JSON line per item, then a summary. The
# scenarios are computed in k7-estimate.md from these lines (and the stub
# overhead from the smoke stubs under evidence/K6b/smoke-not-evidence/).
set -euo pipefail
EV=${K6B_EV:-evidence/K6b}
scratch=$(mktemp -d runs/.k6b/estimate-XXXXXX)
trap 'rm -rf "$scratch"' EXIT
mean() { awk '{s+=$1; n++} END {if (n) printf "%d", s/n; else print 0}'; }
# Manifest + seal overhead of a stub: stub size minus the zstd -19 size of its plan.
over=$(for s in "$EV"/smoke-not-evidence/*.stub.tar.zst; do
	id=$(basename "$s" .stub.tar.zst)
	p=$(zstd -dc "$s" | tar -xOf - "$id/plan.json" | zstd -q -19 -c | wc -c)
	echo $(($(stat -c %s "$s") - p))
done | mean)
checks=$(cat evidence/K4/runs/*.checks.json evidence/K5/runs/*.checks.json "$EV"/smoke-not-evidence/*.checks.json 2>/dev/null | wc -c)
nchecks=$(ls evidence/K4/runs/*.checks.json evidence/K5/runs/*.checks.json "$EV"/smoke-not-evidence/*.checks.json 2>/dev/null | wc -l)
checks_mean=$((checks / nchecks))
total=0 n=0
while read -r cell campaign profile seed; do
	bin/commitproof plan --campaign "$campaign" --seed "$seed" --profile "$profile" 2>/dev/null | head -n 1 >"$scratch/plan.json"
	pz=$(zstd -q -19 -c "$scratch/plan.json" | wc -c)
	b=$((pz + over + checks_mean))
	total=$((total + b)) n=$((n + 1))
	echo "{\"cell\":\"$cell\",\"campaign\":\"$campaign\",\"profile\":\"$profile\",\"seed\":$seed,\"plan_zstd_bytes\":$pz,\"summary_bytes\":$b}"
done < <(jq -r '.entries[] | . as $e | .seeds[] | [$e.cell, $e.campaign, ($e.profile // "baseline"), .] | @tsv' campaigns/k7.json)
echo "{\"item\":\"summaries\",\"runs\":$n,\"bytes\":$total,\"stub_overhead_bytes\":$over,\"checks_mean_bytes\":$checks_mean}"
# Durations (s): pre-K7's measured medians (record durations: start to the
# next run's start, K4/K5); C12 and slotless C4 from the K6b smoke runs (first
# to last command of the run's commands.jsonl, cluster up to teardown).
jq -c 'select(.campaign != null) | {item: "duration", campaign, median_s, runs}' evidence/pre-K7/logs/durations.stdout
for d in runs/.k6b/smoke/*/; do
	[ -f "$d/seal.json" ] || continue
	jq -sc --arg run "$(basename "$d")" --arg profile "$(jq -r .profile "$d/manifest.json")" --arg campaign "$(jq -r .campaign "$d/manifest.json")" \
		'{item: "smoke-duration", run: $run, campaign: $campaign, profile: $profile, span_s: (((map(.end_mono_ns) | max) - (map(.start_mono_ns) | min)) / 1e9 | floor)}' "$d/commands.jsonl"
done
# Bytes per real-build finding (K4, K5): original and minimized full archives; candidates and replays as stubs + checks.
for k in K4 K5; do
	jq -r '.runs[] | select(.kind=="primary" and .mutant=="real" and .verdict=="FAIL") | [.run_id, .campaign, .follow_up.minimized_run, (.follow_up.replays | length)] | @tsv' "evidence/$k/results.json"
done | while read -r id campaign min reps; do
	o=$(stat -c %s "evidence/K"*"/runs/$id.tar.zst" 2>/dev/null | head -n 1)
	m=$(stat -c %s "evidence/K"*"/runs/$min.tar.zst" 2>/dev/null | head -n 1)
	c=$(jq '.candidates | length' evidence/K*/runs/"$id".minimize.json)
	echo "{\"item\":\"finding\",\"run\":\"$id\",\"campaign\":\"$campaign\",\"original_full_bytes\":${o:-0},\"minimized_full_bytes\":${m:-0},\"candidates\":$c,\"replays\":$reps}"
done
echo "{\"item\":\"evidence\",\"bytes\":$(du -sb evidence | cut -f1),\"p25_bytes\":250000000}"
echo "{\"command\":\"k6b-k7-estimate\",\"exit_code\":0}"
