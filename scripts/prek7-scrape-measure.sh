#!/usr/bin/env bash
# prek7-scrape-measure.sh <sealed run dir> [<run archive>] — the cost of
# the /metrics scrapes of ADR-0038 in one run: bytes of the observation
# lines with src "metrics" (uncompressed, and alone at zstd -19, the stored
# level), the other observation lines likewise, per target the scrapes and
# failures, the run's length (last history event t) and the bytes per
# run-minute; with the archive, its size. One JSON line on stdout.
set -u -o pipefail
d=${1:?run dir} arch=${2:-}
[ -f "$d/seal.json" ] || { echo '{"error":"not a sealed run"}'; exit 3; }
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
grep -F '"src":"metrics"' "$d/observations.jsonl" >"$tmp/metrics" || true
grep -vF '"src":"metrics"' "$d/observations.jsonl" >"$tmp/other" || true
zstd -q -19 -c "$tmp/metrics" >"$tmp/metrics.zst"
zstd -q -19 -c "$tmp/other" >"$tmp/other.zst"
zstd -q -19 -c "$d/observations.jsonl" >"$tmp/all.zst"
last_ns=$(tail -n 1 "$d/history.jsonl" | jq -r .t)
per_target=$(jq -sc 'map(.value) | group_by(.target) | map({target: .[0].target, scrapes: length, failed: (map(select(.ok != true)) | length), body_bytes_mean: ((map(select(.ok == true) | .bytes) | add // 0) / ((map(select(.ok == true)) | length) | if . == 0 then 1 else . end) | floor)})' "$tmp/metrics")
sz() { stat -c %s "$1"; }
mb=$(sz "$tmp/metrics") mz=$(sz "$tmp/metrics.zst") ob=$(sz "$tmp/other") oz=$(sz "$tmp/other.zst") ab=$(sz "$d/observations.jsonl") az=$(sz "$tmp/all.zst")
archive=null
[ -n "$arch" ] && archive=$(sz "$arch")
jq -cn --argjson mb "$mb" --argjson mz "$mz" --argjson ob "$ob" --argjson oz "$oz" --argjson ab "$ab" --argjson az "$az" \
	--argjson lines "$(wc -l <"$tmp/metrics")" --argjson ns "$last_ns" --argjson pt "$per_target" --argjson archive "$archive" --arg run "$(basename "$d")" '
	($ns / 60000000000) as $min |
	{command: "prek7-scrape-measure", run: $run, run_ns: $ns,
	 metrics_lines: $lines, metrics_bytes: $mb, metrics_zstd19_bytes: $mz,
	 other_observation_bytes: $ob, other_observation_zstd19_bytes: $oz,
	 observations_bytes: $ab, observations_zstd19_bytes: $az,
	 metrics_bytes_per_run_minute: (($mb / $min) | floor), metrics_zstd19_bytes_per_run_minute: (($mz / $min) | floor),
	 run_archive_bytes: $archive, per_target: $pt, exit_code: 0}'
