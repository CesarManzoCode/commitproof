#!/usr/bin/env bash
# k7-recheck.sh — after a checker fix (a HARNESS_DEFECT, CONTRACT §11.1),
# re-checks every sealed run directory of <runs-dir> with bin/cpcheck
# (a pure function of the sealed history: checks.json is outside the seal,
# §5.6) and reports, per run, whether the verdict, the class set or the
# bytes of checks.json changed. The previous checks.json is kept under
# <keep>/<run-id>.checks.json. No sealed file is touched. Only runs whose
# checks.json is not committed may be re-checked (fail closed).
#
# usage: k7-recheck.sh <runs-dir> <keep-dir>
set -euo pipefail
dir=${1:?runs directory} keep=${2:?directory for the previous checks.json}
mkdir -p "$keep"
classes() { jq -c '[.checkers[] | select(.valid == false) | .anomalies[].class] | unique' "$1"; }
n=0 nv=0 nc=0 nb=0
for d in "$dir"/*/; do
	d=${d%/}
	id=$(basename "$d")
	case "$id" in *.elle) continue ;; esac
	[ -f "$d/seal.json" ] && [ -f "$d.checks.json" ] || continue
	git ls-files --error-unmatch "$d.checks.json" >/dev/null 2>&1 && { echo "k7-recheck: $d.checks.json is committed" >&2; exit 2; }
	cp "$d.checks.json" "$keep/$id.checks.json"
	ov=$(jq -r .verdict "$d.checks.json") oc=$(classes "$d.checks.json") os=$(sha256sum "$d.checks.json" | cut -d' ' -f1)
	bin/cpcheck check "$d" >/dev/null || true
	nv_=$(jq -r .verdict "$d.checks.json") nc_=$(classes "$d.checks.json") ns=$(sha256sum "$d.checks.json" | cut -d' ' -f1)
	n=$((n + 1))
	[ "$ov" != "$nv_" ] && nv=$((nv + 1))
	[ "$oc" != "$nc_" ] && nc=$((nc + 1))
	[ "$os" != "$ns" ] && nb=$((nb + 1))
	echo "{\"run\":\"$id\",\"verdict\":[\"$ov\",\"$nv_\"],\"classes\":[$oc,$nc_],\"checks_changed\":$([ "$os" != "$ns" ] && echo true || echo false)}"
done
echo "{\"command\":\"k7-recheck\",\"runs\":$n,\"verdict_changed\":$nv,\"classes_changed\":$nc,\"checks_changed\":$nb,\"exit_code\":0}"
