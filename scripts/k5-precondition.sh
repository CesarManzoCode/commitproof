#!/usr/bin/env bash
# k5-precondition.sh — the K5 precondition (prompts/K5.md): K1–K4 are DONE
# and their gates pass. cpgate writes gate.json into the directory it
# evaluates and closed evidence must not change, so each gate runs on a copy
# under the git-ignored runs/.
set -u -o pipefail
rc=0
for k in K1 K2 K3 K4; do
	grep -q '^status: DONE$' "evidence/$k/closure.yaml" || { echo "{\"command\":\"k5-precondition\",\"error\":\"evidence/$k/closure.yaml is not status: DONE\",\"exit_code\":3}"; exit 3; }
	c=runs/.k5-precondition-$k
	if [ -d "$c" ]; then chmod -R u+w "$c" && rm -rf "${c:?}"; fi
	mkdir -p runs && cp -r "evidence/$k" "$c" && chmod -R u+w "$c"
	bin/cpgate "$k" --evidence "$c" || rc=1
done
exit $rc
