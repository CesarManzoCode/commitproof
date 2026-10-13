#!/usr/bin/env bash
# k4-dockerd-dns.sh — diagnosis record for evidence/K4/NOTES.md §9 (ADR-0016:
# observations explain, never judge): dockerd's "[resolver] failed to query
# external DNS server" lines — names Docker's embedded DNS forwarded to the
# host's upstream resolver and got no answer for — counted by boot, by
# question (pooler-<n> folded to pooler-nX) and inside UTC windows given as
# label=<since>,<until>. Addresses are not printed.
#
# usage: k4-dockerd-dns.sh [label=YYYY-MM-DDTHH:MM:SSZ,YYYY-MM-DDTHH:MM:SSZ]...
set -euo pipefail
lines() { # journalctl args... → "question type" per failed forward
	timeout 60 journalctl -u docker --no-pager -o cat "$@" 2>/dev/null |
		grep -F 'failed to query external DNS server' |
		sed -n 's/.*question=";\([^\\]*\)\.\\tIN\\t *\([A-Z]*\)".*/\1 \2/p' |
		sed 's/pooler-n[0-9]/pooler-nX/'
}
summary() { # label, journalctl args... → one JSON line
	local label=$1
	shift
	lines "$@" | sort | uniq -c | awk -v l="$label" '
		{ n += $1; q = q sprintf("%s\"%s %s\":%d", (q == "" ? "" : ","), $2, $3, $1) }
		END { printf "{\"scope\":\"%s\",\"failed_forwards\":%d,\"by_question\":{%s}}\n", l, n, q }'
}
summary "boot -1 (before the power-off)" -b -1
summary "boot 0 (after the power-off)" -b 0
for w in "$@"; do
	label=${w%%=*} span=${w#*=}
	since=${span%%,*} until=${span#*,}
	since=${since/T/ } until=${until/T/ }
	summary "$label" --since "${since%Z} UTC" --until "${until%Z} UTC"
done
