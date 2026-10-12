#!/usr/bin/env bash
# k4-analysis.sh — diagnosis records for evidence/K4/NOTES.md (ADR-0016:
# observations explain, never judge): k3obs (each fault's confirmation, heal,
# leader changes and client gaps) and k4obs (exposure, outcomes, errors,
# notices, CancelRequests, unknown-outcome writes in the final read) over the
# primary runs of evidence/K4/runs, and k4orch (multiorch's leader
# appointments in D2 and D2′ runs), recorded like every other step. Run after
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
# Each invocation covers the primary run directories present (the first, the
# K4 matrix; later ones, e.g. D2′'s, are recorded as k3obs-<n>, k4obs-<n>).
# Skipped when every such directory is already named in a record's argv.
for name in k3obs k4obs; do
	new=0
	for d in "${dirs[@]}"; do
		grep -F "\"name\":\"$name" "$EV/commands.jsonl" 2>/dev/null | grep -qF "\"$d\"" || new=1
	done
	if [ "$new" = 0 ]; then
		echo "== skip $name (recorded for these runs)" >&2
		continue
	fi
	rec=$name n=1
	while grep -qF "\"name\":\"$rec\"" "$EV/commands.jsonl" 2>/dev/null; do n=$((n + 1)); rec="$name-$n"; done
	"${REC[@]}" "$rec" -- go run "./internal/tools/$name" "${dirs[@]}" >/dev/null || exit 1
done
# k4orch: multiorch's leader appointments in the superseded D2 runs (full
# archives) and the D2′ runs (directories; derived runs are skipped by the
# tool), ADR-0027. Recorded once, after the D2′ campaign.
if ! grep -qF '"name":"k4orch"' "$EV/commands.jsonl" 2>/dev/null; then
	d2=()
	for f in "$EV"/runs/*-D2-*[0-9a-f].tar.zst "$EV"/runs/*-D2p-*/; do
		[ -e "$f" ] && d2+=("${f%/}")
	done
	"${REC[@]}" k4orch -- go run ./internal/tools/k4orch "${d2[@]}" >/dev/null || exit 1
fi
