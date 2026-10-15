#!/usr/bin/env bash
# prek7-precondition.sh — the pre-K7 readiness precondition: K1–K6 are DONE
# and their gates pass with this commit's cpgate. cpgate writes gate.json
# into the directory it evaluates and closed evidence must not change, so
# each gate runs on a copy under the git-ignored runs/ (as
# k6-precondition.sh does for K1–K5).
set -u -o pipefail
rc=0
for k in K1 K2 K3 K4 K5 K6; do
	grep -q '^status: DONE$' "evidence/$k/closure.yaml" || { echo "{\"command\":\"prek7-precondition\",\"error\":\"evidence/$k/closure.yaml is not status: DONE\",\"exit_code\":3}"; exit 3; }
	c=runs/.prek7-precondition-$k
	if [ -d "$c" ]; then chmod -R u+w "$c" && rm -rf "${c:?}"; fi
	mkdir -p runs && cp -r "evidence/$k" "$c" && chmod -R u+w "$c"
	bin/cpgate "$k" --evidence "$c" || rc=1
done
exit $rc
