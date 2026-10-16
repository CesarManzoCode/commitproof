#!/usr/bin/env bash
# prek7-evidence-size.sh — bytes under evidence/ (P25) per sprint
# directory and in total, the largest file in the repository's tracked and
# untracked content outside .git, runs/, bin/ and .cache/, and the sizes of
# the committed K4/K5 run archives, stubs and checks files by kind. One
# JSON line per item, then a summary line.
set -u -o pipefail
for d in evidence/*/; do
	printf '{"dir":"%s","bytes":%s}\n' "${d%/}" "$(du -sb "$d" | cut -f1)"
done
total=$(du -sb evidence | cut -f1)
big=$(find . -path ./.git -prune -o -path ./runs -prune -o -path ./bin -prune -o -path ./.cache -prune -o -type f -printf '%s %p\n' | sort -n | tail -n 1)
for k in K4 K5; do
	ls -l "evidence/$k/runs" | awk -v k="$k" '
		/\.stub\.tar\.zst$/ {s += $5; n++; next}
		/\.tar\.zst$/ {f += $5; m++; next}
		/\.checks\.json$/ {c += $5; j++}
		END {printf "{\"sprint\":\"%s\",\"full_archives\":%d,\"full_bytes\":%d,\"stubs\":%d,\"stub_bytes\":%d,\"checks\":%d,\"checks_bytes\":%d}\n", k, m, f, n, s, j, c}'
done
printf '{"command":"prek7-evidence-size","evidence_bytes":%s,"p25_bytes":%s,"remaining_bytes":%s,"largest_file_bytes":%s,"largest_file":"%s","exit_code":0}\n' \
	"$total" 250000000 "$((250000000 - total))" "${big%% *}" "${big#* }"
