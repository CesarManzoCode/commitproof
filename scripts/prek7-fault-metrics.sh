#!/usr/bin/env bash
# prek7-fault-metrics.sh <run dir>... — ADR-0038 on runs with faults: each
# sealed run (e.g. the CI-equivalent run's nemesis smoke runs) is rendered
# by cpreport into runs/.prek7/fault-render/ and its metrics block is
# cross-checked by metricscheck (cpgate's recomputation); one JSON line per
# run with the scrape counts per target (ok and failed) and the counter
# resets the report's data show (increase > last − first), then a summary.
# Exit 0 only if every run renders and agrees.
set -u -o pipefail
export GOTOOLCHAIN=go1.26.9 CGO_ENABLED=0
S=runs/.prek7/fault-render
if [ -d "$S" ]; then chmod -R u+w "$S" && rm -rf "${S:?}"; fi
mkdir -p "$S"
go build -o "$S/metricscheck" ./internal/tools/metricscheck || exit 3
rc=0 n=0
for d in "$@"; do
	id=$(basename "$d")
	bin/cpreport render "$d" --out "$S" >/dev/null || { echo "{\"run\":\"$id\",\"error\":\"render\"}"; rc=1; continue; }
	mc=$("$S/metricscheck" --report "$S/$id.html" --run "$d"); code=$?
	[ $code -eq 0 ] || rc=1
	n=$((n + 1))
	sed -n 's/.*<script type="application\/json" id="cp-data">\(.*\)<\/script>.*/\1/p' "$S/$id.html" |
		jq -c --arg id "$id" --argjson code "$code" --argjson mc "$mc" '{run: $id, metricscheck_exit: $code, problems: ($mc.problems // []),
			faults: [.faults[] | .kind] , scrapes: (.metrics | to_entries | map({target: .key, scrapes: .value.scrapes, failed: .value.failed})),
			counter_resets: [.metrics | to_entries[] | .key as $t | .value.families | to_entries[] | select(.value.kind == "counter" and .value.increase > (.value.last - .value.first)) | "\($t) \(.key)"]}'
done
echo "{\"command\":\"prek7-fault-metrics\",\"runs\":$n,\"exit_code\":$rc}"
exit $rc
