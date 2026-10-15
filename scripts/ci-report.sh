#!/usr/bin/env bash
# ci-report.sh — `make ci-report` (CONTRACT §13, §15 K6): render the
# committed K6 reference run and compare the bytes with the committed
# example report. No normalization: any byte of difference fails.
set -u -o pipefail
ref=evidence/K6/reference-run
archs=("$ref"/*.tar.zst)
[ "${#archs[@]}" -eq 1 ] && [ -f "${archs[0]}" ] || { echo "ci-report: want exactly one run archive in $ref" >&2; exit 3; }
id=$(basename "${archs[0]}" .tar.zst)
want=evidence/K6/report/$id.html
[ -f "$want" ] || { echo "ci-report: $want is missing" >&2; exit 3; }
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
bin/cpreport render "${archs[0]}" --out "$tmp" || exit 1
if cmp "$tmp/$id.html" "$want"; then
	echo "ci-report: $id.html identical to the committed example ($(sha256sum "$want" | cut -d' ' -f1))"
else
	echo "ci-report: the render differs from $want" >&2
	exit 1
fi
