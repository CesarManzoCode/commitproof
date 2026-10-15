#!/usr/bin/env bash
# k5-d3-diagnosis.sh — what happens after each F2a of D3, for the design of
# D3′ (ADR-0033). Diagnosis only (ADR-0016): never a verdict. For every
# primary D3 run it prints, in time order, the multiorch lines of a leader
# appointment (problem, recruitment, promotion) and the leader pooler lines of
# its PostgreSQL restart (MonitorPostgres start, resignation, restart as a
# standby, recruit received), from the full archives kept on this machine
# (runs/k5-full; the mutant runs are committed as stubs, ADR-0030). Then the
# pinned-source lines the design relies on (e3cbdbb, the source cache that
# `images build` uses).
# usage: k5-d3-diagnosis.sh [<full-dir>] [<source-dir>]
set -euo pipefail
full=${1:-runs/k5-full} src=${2:-.cache/multigres}
[ -d "$full" ] || { echo "k5-d3-diagnosis: no $full" >&2; exit 3; }
[ "$(git -C "$src" rev-parse HEAD)" = e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73 ] || { echo "k5-d3-diagnosis: $src is not at the pinned commit" >&2; exit 3; }
tmp=$(mktemp -d runs/.k5-d3-diag-XXXXXX)
trap 'chmod -R u+w "$tmp" 2>/dev/null; rm -rf "$tmp"' EXIT
orch='"msg":"(executing appoint leader action|starting leader appointment|recruit failed|recruited pooler|promote succeeded|primary.promotion|recruitment insufficient|appoint leader action[^"]*)"'
pool='"msg":"(MonitorPostgres: (PostgreSQL initialized but not running, starting PostgreSQL|Postgres started successfully|rule names us leader but postgres is a standby; resigning|stale primary[^"]*)|recruit received|SetPrimary: stale primary, restarting as standby)"'
n=0
for a in "$full"/*-D3-*.tar.zst; do
	id=$(basename "$a" .tar.zst)
	m=$(zstd -dc "$a" | tar -xOf - "$id/manifest.json")
	grep -q '"replay_of"\|"candidate_of"' <<<"$m" && continue
	zstd -dc "$a" | tar -C "$tmp" -xf - "$id/logs" "$id/history.jsonl"
	echo "== $id"
	# The run's F2a injections (history t, seconds after the run's origin).
	grep '"type":"nemesis"' "$tmp/$id/history.jsonl" | grep '"phase":"inject"' | while read -r l; do
		echo "  inject $(grep -o '"id":"f[0-9]*"' <<<"$l" | head -n 1 | cut -d'"' -f4) on $(grep -o '"targets":\["[a-z0-9]*"' <<<"$l" | head -n 1 | cut -d'"' -f4) at t=$(grep -o '"t":[0-9]*' <<<"$l" | tail -n 1 | cut -d: -f2) ns"
	done
	{
		for f in orch-z1 orch-z2 orch-z3; do { grep -hE "$orch" "$tmp/$id/logs/$f.log" || true; } | sed "s/^/$f /"; done
		for f in pooler-n1 pooler-n2 pooler-n3; do { grep -hE "$pool" "$tmp/$id/logs/$f.log" || true; } | sed "s/^/$f /"; done
	} | awk '{print $2, $1, $0}' | sort -k1,1 | cut -d' ' -f3- |
		sed -E 's/^([a-z0-9-]+) ([0-9T:.-]+)Z \{.*"msg":"([^"]*)"(.*)$/  \2 \1: \3 \4/' |
		sed -E 's/,"(service_instance_id|cloud_availability_zone|service_version|multigres_[a-z]+|trace_id|span_id|pooler_name)":"[^"]*"//g' | cut -c1-260
	chmod -R u+w "$tmp/$id" && rm -rf "${tmp:?}/$id"
	n=$((n + 1))
done
[ "$n" -eq 10 ] || { echo "k5-d3-diagnosis: $n primary D3 runs, expected 10" >&2; exit 2; }
show() { # file first last
	echo "-- $1:$2-$3"
	sed -n "$2,$3p" "$src/$1"
}
echo "== pinned source (e3cbdbb)"
show go/services/multigateway/poolergateway/pooler_gateway.go 88 107
show go/common/constants/gateway.go 49 52
show go/services/multigateway/buffer/shard_buffer.go 99 128
show go/services/multigateway/poolergateway/load_balancer.go 559 570
show go/common/consensus/revocation.go 350 360
show go/services/multiorch/consensus/failover_backoff.go 60 77
show go/common/ha/backoff.go 57 63
show go/services/multipooler/internal/manager/postgres_monitor.go 1338 1349
show go/services/multipooler/internal/manager/postgres_monitor.go 1374 1384
