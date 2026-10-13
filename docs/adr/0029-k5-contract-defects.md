# ADR-0029 — Three contract defects found before any K5 run: the buffer timing guard (G8), the LSN that W7 confirms (G11), and W8's roles under S03's pg_hba (G12)

Status: Accepted (K5, contract 1.0.8). Corrects RESEARCH S14, CONTRACT §3 G8, §5.1, §7 W7, §10.7 and §10.10. Written before any K5 campaign or directed run; the evidence below comes from the pinned sources and from a fault-free development cluster (no K5 plan was run).

## Context

While deriving the K5 workloads and checkers from the pinned commit (CONTRACT §15 K5), three statements of the contract turned out to be wrong for the system it pins. Each would make a K5 campaign report something other than what its guarantee says: a documented behavior counted as a violation (prompt K5 §4 and ADR-0008 forbid that), every correct CDC reconnect reported as a duplicate, or a workload that cannot run at all.

## Defect 1 — S14 omits the buffer timing guard; G8 would count it

Evidence (pinned commit `e3cbdbb`):

- `docs/query_serving/failover_buffering.md` (S14), configuration table: `| --buffer-min-time-between-failovers | 1m | Min gap between failovers per shard |`; safety-rails table: `| Timing guard | (no buffer) | MinTimeBetweenFailovers not elapsed since last failover |`.
- `go/services/multigateway/buffer/config.go:63–64`: default `1 * time.Minute`; `go/services/multigateway/buffer/shard_buffer.go:111–118`: a shard in IDLE whose last buffering ended less than that ago records `skipped "min_time_between_failovers"` and returns without buffering; `shard_buffer.go:201`: `lastEnd` is set when buffering stops (the new primary is seen serving); `MaxFailoverDuration` (`buffer-max-failover-duration`, 20 s, S14) force-stops an episode.
- `internal/runner/healthy.go` `checkGateways`: the HEALTHY predicate writes through every gateway on every evaluation, also while bootstrap has no primary yet, so a buffered cluster typically ends a buffering episode just before it becomes HEALTHY.
- RESEARCH S14 lists window, size and maximum duration, not this flag; §5.5 `buffered` keeps the defaults. C8 applies F6 three times in a 90 s run (P04; P05 gaps of 10–15 s): by design the second and third switchovers are not buffered, and the first one is not either if it comes less than 60 s after bootstrap's episode. Their errors would be counted as `planned-failover-error`. D3's three F2a, 15 s apart, are likewise buffered at most once per run.

Correction: G8 is the claim for the planned failovers that the documented configuration buffers. (a) In profile `buffered` the runner waits `buffer-min-time-between-failovers` (60 s) after the run-start HEALTHY evaluation succeeds before the workload starts (marker `buffer-guard-wait`); a buffering episode of bootstrap ends before HEALTHY can succeed. (b) C9 judges G8 in an F6 window only if its injection is at least 60 s after `max(end, start + 20 s)` of every earlier fault window of the run (a window's end is its last nemesis event, `healed`; the 20 s bound covers an episode that outlived the window). Errors of single autocommit statements and non-40001 errors of explicit transactions in the other F6 windows are listed as the observation `planned-failover-unbuffered` (C9 stats `g8_windows_judged`, `g8_windows_unbuffered`), never as violations. RESEARCH S14 gains the two flags.

Alternatives rejected: counting every F6 window (counts a documented behavior as a violation); changing the gateways' `--buffer-min-time-between-failovers` (§5.5 says defaults otherwise; it would test a configuration nobody documents); fewer or later switchovers (P04 and C8's "F6 ×3" are fixed; ≥ 60 s apart does not fit three in 90 s).

Consequence for M3 (recorded before any D3 run): D3 can reach M3's buffered retry at most once per run, at its first F2a.

## Defect 2 — the LSN W7 confirms and §10.7 compares

§7 W7 says the consumer confirms "the last fully received commit LSN" and §10.7 calls a re-delivery a violation when "the second delivery's commit LSN ≤ the consumer's confirmed flush". pgoutput (proto 1) gives two LSNs per transaction: BEGIN `final_lsn` = COMMIT `commit_lsn` (the start of the commit record) and COMMIT `end_lsn` (its end). PostgreSQL resumes decoding at the first transaction whose commit record starts at or after the confirmed position (logical decoding skips commits that start before `start_decoding_at`). Development cluster, slot `cp_slot` created through gw-z1 with `CREATE_REPLICATION_SLOT cp_slot LOGICAL pgoutput (SNAPSHOT 'nothing', FAILOVER true)`, proto_version '1', publication cp_pub:

```
== round 1: confirm commit START lsn (FinalLSN)
  BEGIN final 0/405AE80 xid 988 ...
  COMMIT commit 0/405AE80 end 0/405AEB0 walstart 0/405AEB0
  -> status flush 0/405AE80
== round 2: reconnect at 0/405AE80 (confirmed = commit start)
  BEGIN final 0/405AE80 xid 988 ...                         <- re-sent
  COMMIT commit 0/405B278 end 0/405B2A8 ...  -> status flush 0/405B2A8
== round 3: reconnect at 0/405B2A8 (confirmed = commit end)  <- nothing re-sent
== round 4: reconnect at 0/0 (server uses the slot's confirmed_flush) <- nothing re-sent
```

Read with the commit's start LSN, W7 plus §10.7's `≤` turn PostgreSQL's correct resume into a `cdc-duplicate` on every reconnect. Correction: the "commit LSN" of §7 and §10.7 is the transaction's commit **end** LSN (COMMIT `end_lsn`; the XLogData start of the COMMIT message): the consumer records it with each received change, confirms it, and reconnects with `START_REPLICATION … <last confirmed>`. With that reading §10.7's `≤` is exact: a confirmed transaction is never re-sent, the next one is (its end LSN exceeds the confirmed one). K2's corpus and checker are unchanged (its synthetic consumer confirms the same LSN it records).

Alternatives rejected: the start LSN with a strict `<` (that is the K2 sabotage `cp_sabotage_cdc_duplicate_bound`, and it needlessly re-sends one transaction per reconnect).

## Defect 3 — W8's roles cannot run under S03's pg_hba

W8 needs roles `cp_storm` and `cp_steady` through the gateway. At the pinned commit the pooler's per-user pools "connect directly as that user using trust/peer authentication" over PostgreSQL's Unix socket (`go/services/multipooler/internal/connpoolmanager/user_pool.go:36`, `:123`). `deploy/pg_hba_template.conf` is S03's ConfigMap verbatim: `local all {{.User}} trust` then `local all all peer`, and the pooler runs as the OS user postgres (ADR-0021), so peer authentication refuses every other role. Development cluster: the gateway accepted the login of `cp_steady` (SCRAM against the role created through gw-z1), and its first statement failed with `FATAL: Peer authentication failed for user "cp_steady" (SQLSTATE 28000)`; `cp_storm` alike. After replacing that line with `local all all trust` on the three nodes and `select pg_reload_conf()`, the same `cp_steady` session inserted into `cp.ledger` and returned `pg_postmaster_start_time()`, and `cp_storm` ran `select pg_sleep(0.5)` in 505 ms.

Correction: the §5.1 topology uses `deploy/pg_hba_template.baseline.conf`, S03's file with that one line changed to `local all all trust`; it is listed as an allowed divergence in §5.1 and `deploy/DERIVATION.md`. Every `host` line of S03's file is already `trust`, and the postgres user's line is unchanged, so nothing K1–K4 exercised changes. `deploy/pg_hba_template.conf` stays verbatim for profile h1 (ADR-0025).

Alternatives rejected: `--socket-file=''` (the pooler would reach PostgreSQL over TCP, whose lines are `trust`, but every backend connection would change transport, a larger divergence); a `pg_ident` map (pgctld does not manage `pg_ident.conf`); a W8-only profile (§5.5 has none, and C11 would no longer run on `baseline`).

## Consequences

- Contract 1.0.7 → 1.0.8: §3 G8 and §10.10 state the guard; §5.1 lists the pg_hba divergence; §7 W7 and §10.7 name the end LSN; RESEARCH S14 lists the timing-guard flags.
- The K1–K4 gates read their recorded evidence and keep accepting the contract versions their runs carry (K4's D2′ runs carry 1.0.7).

## Sources

S03, S12, S14, S15; CONTRACT §3, §5.1, §5.5, §7, §10.7, §10.10; ADR-0008, ADR-0021, ADR-0025.
