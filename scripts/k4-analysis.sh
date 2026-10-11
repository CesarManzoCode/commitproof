#!/usr/bin/env bash
# k4-analysis.sh — diagnosis records for evidence/K4/NOTES.md (ADR-0016:
# observations explain, never judge): k3obs (each fault's confirmation, heal,
# leader changes and client gaps) and k4obs (exposure, outcomes, errors,
# notices, CancelRequests, unknown-outcome writes in the final read) over the
# primary runs of evidence/K4/runs, recorded like every other step. Run after
# the campaign and before scripts/k4-evidence.sh store (needs the run
# directories).
set -u -o pipefail
EV=evidence/K4
REC=(scripts/record-command.sh "$EV/commands.jsonl" "$EV/logs")
dirs=()
for d in "$EV"/runs/*/; do
	d=${d%/}
	[ -f "$d/seal.json" ] || continue
	grep -q '"replay_of"\|"candidate_of"' "$d/manifest.json" && continue
	dirs+=("$d")
done
[ "${#dirs[@]}" -gt 0 ] || { echo "k4-analysis: no primary run directories" >&2; exit 3; }
for name in k3obs k4obs; do
	if grep -qF "\"name\":\"$name\"" "$EV/commands.jsonl" 2>/dev/null; then
		echo "== skip $name (recorded)" >&2
		continue
	fi
	"${REC[@]}" "$name" -- go run "./internal/tools/$name" "${dirs[@]}" >/dev/null || exit 1
done
