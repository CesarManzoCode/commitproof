#!/usr/bin/env bash
# prek7-metrics-controls.sh <run archive> <report> — ADR-0038's negative
# controls for the metrics section, each expected to make metricscheck
# fail (exit 1), and the confinement check:
#   forge     a copy of the report with the first "increase" of its metrics
#             block raised by one;
#   sabotage  the run rendered by a cpreport built with
#             cp_sabotage_report_metrics (each target's first scrape
#             skipped);
#   confined  that sabotaged cpreport renders the K6 reference run byte for
#             byte as the committed example (the sabotage reaches the
#             metrics section only).
# One JSON line per control; exit 0 only if every control behaved.
set -u -o pipefail
export GOTOOLCHAIN=go1.26.9 CGO_ENABLED=0
arch=${1:?run archive} rep=${2:?report}
S=runs/.prek7/metrics-controls
if [ -d "$S" ]; then chmod -R u+w "$S" && rm -rf "${S:?}"; fi
mkdir -p "$S"
go build -o "$S/metricscheck" ./internal/tools/metricscheck || exit 3
rc=0
# forge
sed -E '0,/"increase":([0-9]+)/s//"increase":\1__CP__/' "$rep" | perl -pe 's/"increase":(\d+)__CP__/"increase":@{[$1+1]}/' >"$S/forged.html"
cmp -s "$rep" "$S/forged.html" && { echo '{"control":"forge","error":"nothing forged"}'; rc=1; }
out=$("$S/metricscheck" --report "$S/forged.html" --run "$arch"); code=$?
echo "{\"control\":\"forge\",\"metricscheck_exit\":$code,\"failed_as_expected\":$([ $code -eq 1 ] && echo true || echo false),\"output\":$out}"
[ $code -eq 1 ] || rc=1
# sabotage
go build -trimpath -tags cp_sabotage_report_metrics -o "$S/cpreport-sabotage" ./cmd/cpreport || exit 3
mkdir -p "$S/sabotage-render"
"$S/cpreport-sabotage" render "$arch" --out "$S/sabotage-render" >/dev/null || { echo '{"control":"sabotage","error":"render failed"}'; exit 1; }
id=$(basename "$arch" .tar.zst)
out=$("$S/metricscheck" --report "$S/sabotage-render/$id.html" --run "$arch"); code=$?
echo "{\"control\":\"sabotage\",\"metricscheck_exit\":$code,\"failed_as_expected\":$([ $code -eq 1 ] && echo true || echo false),\"output\":$out}"
[ $code -eq 1 ] || rc=1
# confined
ref=$(ls evidence/K6/reference-run/*.tar.zst)
rid=$(basename "$ref" .tar.zst)
mkdir -p "$S/confined"
"$S/cpreport-sabotage" render "$ref" --out "$S/confined" >/dev/null || { echo '{"control":"confined","error":"render failed"}'; exit 1; }
if cmp -s "$S/confined/$rid.html" "evidence/K6/report/$rid.html"; then same=true; else same=false; rc=1; fi
echo "{\"control\":\"confined\",\"reference\":\"$rid\",\"identical_to_committed_example\":$same,\"sha256\":\"$(sha256sum "$S/confined/$rid.html" | cut -d' ' -f1)\"}"
echo "{\"command\":\"prek7-metrics-controls\",\"exit_code\":$rc}"
exit $rc
