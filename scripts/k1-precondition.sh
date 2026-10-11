#!/usr/bin/env bash
# k1-precondition.sh — the K2 precondition (prompts/K2.md): gate K1 passes.
# cpgate writes gate.json into the evidence directory it evaluates, and K1's
# evidence is closed, so the gate runs on a copy under the git-ignored runs/.
set -u -o pipefail
C=runs/.k1-precondition
if [ -d "$C" ]; then chmod -R u+w "$C" && rm -rf "${C:?}"; fi
mkdir -p runs
cp -r evidence/K1 "$C" && chmod -R u+w "$C"
grep -q '^status: DONE$' evidence/K1/closure.yaml || { echo '{"command":"k1-precondition","error":"evidence/K1/closure.yaml is not status: DONE","exit_code":3}'; exit 3; }
bin/cpgate K1 --evidence "$C"
