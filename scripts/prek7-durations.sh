#!/usr/bin/env bash
# prek7-durations.sh — measured wall time per run in K4 and K5, for the K7
# feasibility note. Every sealed K4/K5 run (committed archives and stubs
# under evidence/K4/runs and evidence/K5/runs, and the local full archives
# runs/k4-full, runs/k5-full) is named <UTC start yyyymmddThhmmssZ>-<campaign>-
# <seed>-<build>-<plan>. Runs are sequential (one cluster at a time, P10),
# so a run's wall time — cluster up, workload, recovery, final reads,
# teardown, cpcheck — is the time to the next run's start; gaps over 15 min
# (pauses between batches) are dropped. Prints per campaign the count and
# the median, mean and maximum seconds, then one summary line.
set -u -o pipefail
{
	ls evidence/K4/runs evidence/K5/runs runs/k4-full runs/k5-full 2>/dev/null
} | grep -E '^[0-9]{8}T[0-9]{6}Z-' | sed -E 's/(\.stub)?\.tar\.zst$|\.checks\.json$|\.minimize\.json$//' | sort -u |
	jq -R -s -c '
	split("\n") | map(select(length > 0)) |
	map({id: ., t: (.[0:16] | strptime("%Y%m%dT%H%M%SZ") | mktime), campaign: (split("-")[1])}) |
	sort_by(.t) as $r |
	[range(0; ($r | length) - 1) | {campaign: $r[.].campaign, s: ($r[. + 1].t - $r[.].t)} | select(.s > 0 and .s <= 900)] |
	group_by(.campaign) |
	map((map(.s) | sort) as $s | {campaign: .[0].campaign, runs: ($s | length), median_s: $s[($s | length) / 2 | floor], mean_s: (($s | add) / ($s | length) | floor), max_s: $s[-1]}) |
	(.[] ), {command: "prek7-durations", campaigns: length, runs: (map(.runs) | add), overall_mean_s: ((map(.mean_s * .runs) | add) / (map(.runs) | add) | floor)}' -c
