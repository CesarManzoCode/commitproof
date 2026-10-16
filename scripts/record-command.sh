#!/usr/bin/env bash
# record-command.sh — runs one command and appends a record to an evidence
# commands.jsonl: argv, start/end (UTC, informational), exit code, sha256 of
# stdout and stderr. stdout/stderr are kept under <logdir>/<name>.{stdout,stderr}
# so the hashes can be re-checked. stdout is also echoed to the terminal.
#
# usage: record-command.sh <commands.jsonl> <logdir> <name> -- <argv...>
set -u -o pipefail

JSONL="${1:?commands.jsonl}" LOGDIR="${2:?logdir}" NAME="${3:?name}"
shift 3
[ "${1:-}" = "--" ] && shift
[ "$#" -gt 0 ] || { echo "record-command.sh: no command" >&2; exit 3; }

json_str() {
	local s
	s=$(printf '%s' "$1" | tr -d '\000-\010\013\014\016-\037')
	s=${s//\\/\\\\}
	s=${s//\"/\\\"}
	# Tab, newline and carriage return are kept, escaped: a raw one would
	# split or invalidate the record's JSON line.
	s=${s//$'\t'/\\t}
	s=${s//$'\n'/\\n}
	s=${s//$'\r'/\\r}
	printf '"%s"' "$s"
}

mkdir -p "$LOGDIR" "$(dirname "$JSONL")"
out="$LOGDIR/$NAME.stdout" err="$LOGDIR/$NAME.stderr"
# Earlier records reference their logs by hash; never overwrite them.
if [ -e "$out" ] || [ -e "$err" ]; then
	echo "record-command.sh: $out or $err already exists; choose a new name" >&2
	exit 3
fi
start=$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)
"$@" > >(tee "$out") 2>"$err"
rc=$?
wait
end=$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)

argv=""
for a in "$@"; do argv+="${argv:+,}$(json_str "$a")"; done
printf '{"argv":[%s],"end":"%s","exit_code":%d,"name":%s,"start":"%s","stderr":%s,"stderr_sha256":"%s","stdout":%s,"stdout_sha256":"%s"}\n' \
	"$argv" "$end" "$rc" "$(json_str "$NAME")" "$start" \
	"$(json_str "$err")" "$(sha256sum "$err" | cut -d' ' -f1)" \
	"$(json_str "$out")" "$(sha256sum "$out" | cut -d' ' -f1)" >>"$JSONL"
exit "$rc"
