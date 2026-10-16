#!/usr/bin/env bash
# prek7-lim33.sh <run-number> — LIM-33's origin experiment on plain
# PostgreSQL (owner decision D-D of 2026-10-15): the pinned postgres image
# (CONTRACT §14), one primary A and one synchronous standby B
# (synchronous_standby_names = standby1, synchronous_commit on, default
# client_connection_check_interval 0), clients in a third container over
# TCP. With B cut off the network, three autocommit inserts wait in
# synchronous replication on A:
#   (wal_keep_size 1GB on both, so that pg_rewind finds A's WAL back to the
#   checkpoint before the divergence.)
#   k=1  cancelled by pg_cancel_backend from another session — what
#        Multigres' pooler does on context end (regular_conn.go
#        handleContextCancellation → CancelBackend);
#   k=2  cancelled by the client's own CancelRequest (psql on SIGINT);
#   k=3  the client disconnects (psql killed) — what CommitProof's pgx
#        client does at P01 (pgx's default DeadlineContextWatcherHandler:
#        a socket deadline, no CancelRequest).
# A reader on A reads before and after; then B is promoted, A is shut down,
# rewound from B (pg_rewind) and restarted as B's standby; final reads on
# both. Prints one JSON object (the run's observations) on stdout; the
# commands' raw outputs go to the log directory given as $2. All Docker
# resources carry com.docker.compose.project=commitproof and are removed
# at the end, also on failure.
set -u -o pipefail
r=${1:?run number} logs=${2:?log dir}
IMG=postgres:17.7@sha256:2006493727bd5277eece187319af1ef82b4cf82cf4fc1ed00da0775b646ac2a4
P=commitproof-lim33
NET=$P-net A=$P-a B=$P-b C=$P-client
LBL=(--label com.docker.compose.project=commitproof --label commitproof.experiment=lim33)
PGDATA=/var/lib/postgresql/data
mkdir -p "$logs"
L=$logs/run-$r.log
: >"$L"
log() { echo "== $(date -u +%H:%M:%S.%3N) $*" >>"$L"; }
cleanup() {
	docker rm -f "$A" "$B" "$C" >/dev/null 2>&1
	docker network rm "$NET" >/dev/null 2>&1
	docker volume rm "$P-va" "$P-vb" >/dev/null 2>&1
}
fail() { log "FAIL $*"; cleanup; jq -cn --arg e "$*" --argjson r "$r" '{command:"prek7-lim33",run:$r,error:$e,exit_code:1}'; exit 1; }
trap cleanup EXIT
cleanup
q() { # host sql → psql -At output (TCP, from the client container)
	timeout 30 docker exec "$C" psql -h "$1" -U postgres -At -v ON_ERROR_STOP=1 -c "$2" 2>&1
}
run() { log "$*"; timeout 120 "$@" >>"$L" 2>&1; }

run docker network create "${LBL[@]}" "$NET" || fail "network"
run docker volume create "${LBL[@]}" "$P-va" || fail "volume a"
run docker volume create "${LBL[@]}" "$P-vb" || fail "volume b"
run docker run -d "${LBL[@]}" --name "$A" --hostname "$A" --network "$NET" -v "$P-va:$PGDATA" \
	-e POSTGRES_HOST_AUTH_METHOD=trust "$IMG" postgres -c wal_level=replica -c wal_log_hints=on \
	-c max_wal_senders=10 -c wal_keep_size=1GB -c hot_standby=on -c synchronous_standby_names=standby1 || fail "start a"
run docker run -d "${LBL[@]}" --name "$C" --network "$NET" --entrypoint sleep "$IMG" infinity || fail "client"
for i in $(seq 60); do q "$A" "select 1" | grep -qx 1 && break; sleep 1; done
q "$A" "select 1" | grep -qx 1 || fail "a not ready"
run docker exec "$A" bash -c "echo 'host replication all all trust' >> $PGDATA/pg_hba.conf" || fail "pg_hba"
q "$A" "select pg_reload_conf()" >>"$L"
# Standby B: base backup of A, application_name standby1.
run docker run --rm "${LBL[@]}" --network "$NET" -v "$P-vb:$PGDATA" "$IMG" bash -c \
	"chown postgres:postgres $PGDATA && chmod 700 $PGDATA && gosu postgres pg_basebackup -h $A -U postgres -D $PGDATA -X stream -c fast -R && echo \"primary_conninfo = 'host=$A port=5432 user=postgres application_name=standby1'\" >> $PGDATA/postgresql.auto.conf" || fail "basebackup"
run docker run -d "${LBL[@]}" --name "$B" --hostname "$B" --network "$NET" -v "$P-vb:$PGDATA" "$IMG" \
	postgres -c wal_level=replica -c wal_log_hints=on -c max_wal_senders=10 -c wal_keep_size=1GB -c hot_standby=on || fail "start b"
for i in $(seq 60); do [ "$(q "$A" "select sync_state from pg_stat_replication where application_name='standby1'")" = sync ] && break; sleep 1; done
sync=$(q "$A" "select sync_state from pg_stat_replication where application_name='standby1'")
[ "$sync" = sync ] || fail "standby1 not synchronous: $sync"
settings=$(q "$A" "select string_agg(name||'='||setting, ',' order by name) from pg_settings where name in ('synchronous_commit','synchronous_standby_names','client_connection_check_interval','server_version')")
q "$A" "create table cp_lim33 (k int primary key, v text)" >>"$L" || fail "create"
q "$A" "insert into cp_lim33 values (0, 'replicated')" >>"$L" || fail "insert 0"
for i in $(seq 30); do [ "$(q "$B" "select count(*) from cp_lim33")" = 1 ] && break; sleep 0.5; done
[ "$(q "$B" "select count(*) from cp_lim33")" = 1 ] || fail "row 0 not on b"

# Cut B off; three inserts wait in synchronous replication on A.
run docker network disconnect "$NET" "$B" || fail "disconnect b"
for k in 1 2 3; do
	log "client k=$k: insert (background)"
	( timeout 300 docker exec "$C" psql -h "$A" -U postgres -c "insert into cp_lim33 values ($k, 'unreplicated-$k')" >"$logs/run-$r-client-$k.out" 2>&1; echo "exit=$?" >>"$logs/run-$r-client-$k.out" ) &
done
for i in $(seq 40); do [ "$(q "$A" "select count(*) from pg_stat_activity where wait_event='SyncRep'")" = 3 ] && break; sleep 0.25; done
waiting_before=$(q "$A" "select count(*) from pg_stat_activity where wait_event='SyncRep'")
read_before=$(q "$A" "select coalesce(string_agg(k::text, ',' order by k), '') from cp_lim33")
# k=1: pg_cancel_backend; k=2: the client's CancelRequest; k=3: disconnect.
cancel1=$(q "$A" "select pg_cancel_backend(pid) from pg_stat_activity where wait_event='SyncRep' and query like 'insert into cp_lim33 values (1,%'")
log "pg_cancel_backend k=1: $cancel1"
run docker exec "$C" pkill -INT -f "values \(2," || log "pkill INT k=2 found nothing"
run docker exec "$C" pkill -KILL -f "values \(3," || log "pkill KILL k=3 found nothing"
sleep 3
waiting_after=$(q "$A" "select coalesce(string_agg(regexp_replace(query, '.*values \(([0-9]+),.*', '\1'), ',' order by query), '') from pg_stat_activity where wait_event='SyncRep'")
read_after=$(q "$A" "select coalesce(string_agg(k::text, ',' order by k), '') from cp_lim33")
wait
# Failover: promote B (still cut off), shut A down, rewind it from B, restart it as B's standby.
run docker exec "$B" psql -U postgres -At -c "select pg_promote(true, 60)" || fail "promote"
run docker stop -t 60 "$A" || fail "stop a"
run docker network connect "$NET" "$B" || fail "reconnect b"
for i in $(seq 30); do q "$B" "select 1" | grep -qx 1 && break; sleep 1; done
read_new_primary=$(q "$B" "select coalesce(string_agg(k::text, ',' order by k), '') from cp_lim33")
run docker run --rm "${LBL[@]}" --network "$NET" -v "$P-va:$PGDATA" --user postgres "$IMG" \
	pg_rewind --target-pgdata="$PGDATA" --source-server="host=$B port=5432 user=postgres dbname=postgres" -R --progress || fail "pg_rewind"
run docker start "$A" || fail "restart a"
for i in $(seq 60); do q "$A" "select 1" | grep -qx 1 && break; sleep 1; done
for i in $(seq 30); do [ "$(q "$A" "select pg_is_in_recovery()")" = t ] && [ -n "$(q "$B" "select 1 from pg_stat_replication limit 1")" ] && break; sleep 1; done
a_recovery=$(q "$A" "select pg_is_in_recovery()")
final_b=$(q "$B" "select coalesce(string_agg(k::text, ',' order by k), '') from cp_lim33")
final_a=$(q "$A" "select coalesce(string_agg(k::text, ',' order by k), '') from cp_lim33")
outs=$(for k in 1 2 3; do jq -Rs --arg k "$k" '{($k): .}' "$logs/run-$r-client-$k.out"; done | jq -sc add)
jq -cn --argjson r "$r" --arg settings "$settings" --arg sync "$sync" --arg wb "$waiting_before" --arg rb "$read_before" \
	--arg c1 "$cancel1" --arg wa "$waiting_after" --arg ra "$read_after" --arg rn "$read_new_primary" --arg ar "$a_recovery" \
	--arg fb "$final_b" --arg fa "$final_a" --argjson outs "$outs" '{
	command: "prek7-lim33", run: $r, settings: $settings, standby1_sync_state: $sync,
	waiting_in_syncrep_before_cancels: ($wb | tonumber), read_on_a_before_cancels: $rb,
	pg_cancel_backend_k1: $c1, still_waiting_after: $wa, read_on_a_after_cancels: $ra,
	read_on_promoted_b: $rn, a_in_recovery_after_rewind: $ar, final_read_b: $fb, final_read_a: $fa,
	clients: $outs,
	k1_pg_cancel_backend_read_then_vanished: (($ra | split(",") | index("1")) != null and ($fb | split(",") | index("1")) == null and ($fa | split(",") | index("1")) == null),
	k2_cancelrequest_read_then_vanished: (($ra | split(",") | index("2")) != null and ($fb | split(",") | index("2")) == null and ($fa | split(",") | index("2")) == null),
	k3_disconnect_never_read: (($rb + "," + $ra | split(",") | index("3")) == null),
	k3_disconnect_still_waiting: (($wa | split(",") | index("3")) != null),
	exit_code: 0}'
