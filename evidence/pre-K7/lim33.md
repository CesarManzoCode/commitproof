# LIM-33's origin experiment on plain PostgreSQL (pre-K7 readiness, step 4; owner decision D-D)

Command: `scripts/prek7-lim33.sh <n> evidence/pre-K7/lim33` for n = 1…5 (records `lim33-run-1` … `lim33-run-5` and `lim33-summary` in `commands.jsonl`; each run's JSON on the record's stdout, its full command log in `lim33/run-<n>.log`, each client's output in `lim33/run-<n>-client-<k>.out`). The runs used the script as committed in `f5347d3`; its header comment about k = 3 was corrected afterwards in `eebd1f9` (it said CommitProof's client sends no CancelRequest; the code is unchanged, see below).

## Setup (design)

The pinned base image `postgres:17.7@sha256:2006493727bd5277eece187319af1ef82b4cf82cf4fc1ed00da0775b646ac2a4` (CONTRACT §14; pulled by digest from Docker Hub, PREREQUISITES §5), three containers on one Docker network, all labelled `com.docker.compose.project=commitproof` and removed at the end of each run (0 left over):

- A, primary: `wal_level=replica`, `wal_log_hints=on`, `wal_keep_size=1GB` (pg_rewind needs A's WAL back to the checkpoint before the divergence; the first development attempt failed without it), `synchronous_standby_names=standby1`, `synchronous_commit=on`, `client_connection_check_interval=0` (default; recorded per run in `settings`);
- B, synchronous standby from `pg_basebackup -R`, `application_name=standby1`; a run starts only when `pg_stat_replication` on A shows `standby1` with `sync_state = sync`;
- C, the clients' container (psql over TCP to A and B).

Each run: row k = 0 committed and replicated; B disconnected from the network; three autocommit inserts on A, each waiting in synchronous replication (`wait_event = SyncRep`, 3 backends):

| k | Cancel | Models |
|---|---|---|
| 1 | `pg_cancel_backend(pid)` from another session | Multigres' pooler: `regular_conn.go` `execWithContextCancel` → `handleContextCancellation` → `adminPool.CancelBackend` (identical at upstream main, `upstream-check.md`), also the gateway forwarding a client's CancelRequest to the pooler (S17) |
| 2 | the client's own CancelRequest (psql on SIGINT) | CommitProof's pgx client at P01: pgx's context watcher sets a socket deadline and pgconn's `asyncClose` sends a CancelRequest, then Terminate (`pgconn.go`, pgx v5.11.0) |
| 3 | none: the client disconnects (psql killed) | a client that only disconnects; on Multigres the pooler would still cancel when the request context ends |

Then a reader on A; B promoted (`pg_promote`), A shut down (fast), B reconnected, A rewound from B (`pg_rewind -R`) and restarted as B's standby; final reads on B and A.

## Results (system did; 5 of 5 runs identical in every judged item, `lim33-summary`)

- Before any cancel, the reader on A saw only k = 0: a commit waiting for its synchronous standby is not visible (3 backends in `SyncRep`).
- k = 1 and k = 2: the client received `WARNING: canceling wait for synchronous replication due to user request` / `DETAIL: The transaction has already committed locally, but might not have been replicated to the standby.` and `INSERT 0 1` (an acknowledged success, §6.2's `ok` with the S61 notice). The reader on A then saw k = 1 and k = 2.
- k = 3: the backend kept waiting in `SyncRep` (with `client_connection_check_interval = 0` nothing tells it the client is gone); the reader never saw k = 3.
- After the promotion of B and the rewind of A, k = 1, 2, 3 were absent from both final reads; B (never reached by them) held only k = 0.

So on plain PostgreSQL 17.7 streaming replication, **a read observing a write that later vanishes** (the G2 anomaly of FND-D1-fb8d0f8c, FND-D1-2e860fd2, FND-D1-a8eb9857, FND-C2-2a534585) occurs whenever a cancel reaches a backend waiting for synchronous replication — whether `pg_cancel_backend` or the client's CancelRequest — and the old primary is later rewound after a failover to a standby that did not receive the commit. A client that only disconnects does not cause it on plain PostgreSQL.

## What it shows about the origin of the four G2 findings

- **PostgreSQL behaviour suffices, with CommitProof's own client behaviour.** CommitProof's client sends a CancelRequest when P01 expires (pgx `asyncClose`), which is k = 2: on plain PostgreSQL that alone produced the anomaly in 5 of 5 runs. In K4's D1 seed 1 run, gw-z3's log shows `received cancel request` six times during the F1c window, every 10 s from two client connections — the cadence of P01 timeouts (`evidence/K4/runs/20261011T221027Z-D1-1-real-59a95336.tar.zst`, `logs/gw-z3.log`). Matching each one to an operation would need the wall time of the runner's origin, which the run does not record; that alignment is a hypothesis.
- **The pooler's cancel-on-context-end is a second path, not needed for these runs' outcome.** It would produce the same anomaly for a client that only disconnects (k = 3 shows plain PostgreSQL would not). Which of the two cancels reached each backend first in the K4 runs is not determined by this experiment or by the recorded logs (PostgreSQL's own server log was not captured).
- **Classification consequence (proposal, not applied):** §11.1 defines `POSTGRES_INHERITED` as "the behavior reproduces on plain PostgreSQL 17.7 streaming replication or is documented PostgreSQL behavior". With the same client behaviour it reproduces (k = 2, 5/5), and it is documented (S61). The recorded triage files keep `UNDETERMINED` (closed evidence is immutable; `findings-readiness.md` lists the proposal for the owner). What stays undetermined: which cancel path acted in each recorded run, and whether Multigres' failover could have avoided serving reads from a primary whose commits wait for synchronous replication (a design question, not an experiment result).

## Limitations

One host, two PostgreSQL containers, Docker network disconnect as the partition (not iptables), fast shutdown of A before the rewind (Multigres rewinds a running old primary through its pooler). The experiment reproduces the PostgreSQL mechanism, not Multigres' failover timing.
