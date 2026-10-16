#!/usr/bin/env bash
# prek7-metrics-probe.sh — what serves /metrics at the pinned commit
# (CONTRACT P23, §5.4; ADR-0038). Brings the baseline cluster up from the
# committed compose files, then, from each component's nemesis helper (its
# network namespace), fetches /metrics and a path no handler registers on
# the component's HTTP port (gateway 15000, pooler 16000, orchestrator
# 17000) and on OpenTelemetry's Prometheus exporter default port 9464, and
# prints one JSON line per fetch: HTTP status line, body bytes and sha256,
# the first body line, and the number of Prometheus "# TYPE" lines. Then
# brings the cluster down. Exit 0 when every fetch was attempted.
set -u -o pipefail
up=$(bin/commitproof cluster up --profile "${1:-baseline}" | tail -n 1)
if ! grep -q '"healthy":true' <<<"$up"; then
	echo "{\"error\":\"cluster up did not reach HEALTHY\"}"
	bin/commitproof cluster down >/dev/null
	exit 3
fi
for t in gw-z1:15000 gw-z2:15000 gw-z3:15000 pg-n1:16000 pg-n2:16000 pg-n3:16000 orch-z1:17000 orch-z2:17000 orch-z3:17000; do
	svc=${t%%:*} port=${t##*:}
	for p in "$port:/metrics" "$port:/cp-no-such-path" "9464:/metrics"; do
		pp=${p%%:*} path=${p#*:}
		out=$(timeout 20 docker exec "commitproof-nem-$svc-1" sh -c "wget -S -q -T 5 -O /tmp/body http://127.0.0.1:$pp$path 2>/tmp/hdr; echo rc=\$?; head -n 1 /tmp/hdr | tr -d '\r'; wc -c </tmp/body; sha256sum </tmp/body | cut -d' ' -f1; head -n 1 /tmp/body | tr -d '\r' | cut -c1-60; grep -c '^# TYPE ' /tmp/body; rm -f /tmp/body /tmp/hdr")
		mapfile -t f <<<"$out"
		printf '{"ns":"%s","port":%s,"path":"%s","wget":"%s","status":"%s","bytes":%s,"sha256":"%s","first_line":"%s","prometheus_type_lines":%s}\n' \
			"$svc" "$pp" "$path" "${f[0]:-}" "$(sed 's/^ *//;s/"/\\"/g' <<<"${f[1]:-}")" "${f[2]:-0}" "${f[3]:-}" "$(sed 's/"/\\"/g' <<<"${f[4]:-}")" "${f[5]:-0}"
	done
done
bin/commitproof cluster down | tail -n 1 | grep -q '"exit_code":0' || { echo '{"error":"cluster down failed"}'; exit 3; }
echo '{"command":"prek7-metrics-probe","exit_code":0}'
