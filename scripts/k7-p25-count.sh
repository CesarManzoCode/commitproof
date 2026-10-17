#!/usr/bin/env bash
# k7-p25-count.sh — the P25 count of the working tree (CONTRACT §5.7, §13;
# ADR-0047): every byte under evidence/ plus every run archive (*.tar.zst)
# under findings/, against 275 MB; the largest files listed. Exit 4 when the
# count exceeds P25 (K7 stops BLOCKED; nothing is dropped).
#
# usage: k7-p25-count.sh
set -euo pipefail
budget=275000000 # P25 (ADR-0047)
e=$(find evidence -type f -printf '%s\n' | awk '{s+=$1} END {print s+0}')
f=$(find findings -name '*.tar.zst' -printf '%s\n' | awk '{s+=$1} END {print s+0}')
total=$((e + f))
per=$(for d in evidence/*/; do printf '%s %s\n' "$(basename "$d")" "$(find "$d" -type f -printf '%s\n' | awk '{s+=$1} END {print s+0}')"; done |
	jq -cRn '[inputs | split(" ") | {(.[0]): (.[1] | tonumber)}] | add')
big=$(find evidence findings -type f -printf '%s %p\n' | sort -n | tail -n 1)
code=0
[ "$total" -gt "$budget" ] && code=4
printf '{"command":"k7-p25-count","evidence_bytes":%s,"findings_archive_bytes":%s,"count_bytes":%s,"budget_bytes":%s,"margin_bytes":%s,"by_sprint":%s,"largest_file":"%s","largest_file_bytes":%s,"exit_code":%s}\n' \
	"$e" "$f" "$total" "$budget" "$((budget - total))" "$per" "${big#* }" "${big%% *}" "$code"
exit $code
