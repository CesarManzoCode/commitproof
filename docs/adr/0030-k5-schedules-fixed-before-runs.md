# ADR-0030 — K5 schedules, workload details, C8 completions, the slot precondition and evidence storage, fixed before any K5 run

Status: Accepted (K5, contract 1.0.9). Clarifies CONTRACT §6.1 (client numbers), §7 W5–W8, §9.2 D3/D5, §10.9, §13 and the §15 K5 campaign table and K5.3. Written and committed before any K5 campaign or directed run, as ADR-0024 was for K4 (ADR-0007: a directed schedule changed after results needs an ADR). ADR-0029's corrections apply.

## Context (gaps)

§15 K5 fixes the campaigns' profiles, workloads and fault pools, §9.2 the directed schedules' fault sequences and §7 the workloads' operations; the rest is the implementer's choice (§0). These points must not look chosen after results: fault counts, holds and order per run; D3's "15 s apart" and D5's heal timing; the clients of each campaign; what a W5–W8 client does on reconnect; how C8 finds "the listener's first successful op post-recovery" (§10.9); when and how K5.3's slot observation is made; which class a K5 failure is minimized for; and how K5's runs fit P25 (§13: 183.1 MB of 200 MB were used at K4's close).

## Decision

### Clients (history `client` numbers; §6.1's 0–9 are P02's)

| Campaign | Clients |
|---|---|
| C8 (`buffered`) | W1 0–3 (serializable, explicit; micro-ops in ascending key order, below), W4 8–9 |
| C9 | W4 8–9; W5 20–22 (gw-z1, gw-z2, gw-z3); W6 notifiers 23–25 and listeners 26–28 (each gw-z1, gw-z2, gw-z3) |
| C10 | W1 0–3 (serializable), W4 8–9, W7 consumer 29 (gw-z1) |
| C11 | W8: `cp_steady` 8–9 (W4 inserts at P03), `cp_storm` connections 100–299 (connection c through gateway (c − 100) mod 3) |
| D3 (`buffered`) | W1 autocommit-append 0–3 (P03), W4 8–9 at 25 ms ("2× pacing") |
| D5 | W4 8–9, W7 consumer 29 (gw-z1) |

P02's gateway rule holds (client i → `Gateways[i mod 3]`) unless the table names one. The runner's own clients keep their numbers (recovery and final reads 10–12) and W7's slot warm-up writes are client 14.

### Workloads (§7)

- **W5.** On every new connection a session client SETs the four tracked GUCs (`application_name` ∈ {cp-a, cp-b, cp-c, cp-d}, `search_path` ∈ {`cp, public`, `public, cp`, `public`}, `work_mem` ∈ {4MB, 8MB, 16MB}, `DateStyle` ∈ {`ISO, MDY`, `ISO, DMY`}; values from the client's stream; PostgreSQL echoes these forms in SHOW) and prepares four named statements — two SQL-level (`PREPARE cp_s<j> AS SELECT <j>::bigint`) and two protocol-level (Parse of `cp_p<j>`, `SELECT <j>::bigint`), j unique per client — each recorded as an op of that connection. Then it issues the plan's probes at P03: SHOW of a tracked GUC (45 %), EXECUTE of one of the four statements (45 %), or a re-SET of a GUC to another value (10 %; later SHOWs must return it). A transport failure ends the connection; the next op reconnects, and the connection setup is repeated with the current values. History forms as K2 (§10.9 comment in `internal/checker/c8c9.go`); the SQL text and kind of a prepared statement are added to the prepare op's value.
- **W6.** Each notifier runs `BEGIN; INSERT INTO cp.ledger …; NOTIFY cp_chan, '<id>'; COMMIT` at P03 (wl `notify`, f `notify`, ids from the W4 id sequence); one per gateway, so that every listener's gateway has writes that witness its recovery (below) and notifications cross gateways. A listener, on every (re)connect, runs `LISTEN cp_chan` (op `listen`) and then contiguous 1 s waits (op `wait`, ok with the payloads received during it); a failed wait ends the session and the next op reconnects. Listeners keep waiting until 11 s after the workload deadline, so every NOTIFY has its full 10 s window.
- **W7.** The consumer connects with `replication=database`; its first connection creates `cp_slot` with `CREATE_REPLICATION_SLOT cp_slot LOGICAL pgoutput (SNAPSHOT 'nothing', FAILOVER true)`; every connection runs `START_REPLICATION SLOT cp_slot LOGICAL <last confirmed flush> (proto_version '1', publication_names 'cp_pub')` (op `connect`). A transaction's changes are recorded when its COMMIT arrives (op `recv` per change, `lsn` = the commit end LSN, ADR-0029; `flushed` = the confirmed flush before this transaction). A standby status update (write = flush = apply = the last fully received commit end LSN) is sent every 1 s and whenever the server requests a reply (op `ack`). An error ends the stream (op `stream-end`, info); the consumer reconnects 1 s later with the same slot. After the final reads it reads `pg_current_wal_lsn()` through gw-z1 and streams, asking for a reply every 1 s, until the received position (commit end LSNs and the server WAL end of keepalives and XLogData) reaches it, within P08's 60 s: op `caught-up` (ok; info if the deadline passes). Setup adds `create publication cp_pub for table cp.ledger, cp.append` when a campaign has W7.
- **W8.** Storm phases are [5 s, 25 s) and [45 s, 65 s) of the workload ("storm phases of 20 s"). In each phase 200 `cp_storm` connections (4 × `--connpool-global-capacity` 50, S03) each run `select pg_sleep(0.5)` back to back until the phase ends (op `storm` {role, phase, conn}; a failed connect is a fail op and is retried after 500 ms). `cp_steady` runs W4 inserts at P03 for the whole workload with `returning pg_postmaster_start_time()::text` as `server` (the pooler reaches PostgreSQL over its Unix socket, so `inet_server_addr()` is null; a leader change moves execution to another node, whose postmaster start time differs). Setup adds the two login roles (password = name), `grant usage on schema cp` and `grant insert, select on cp.ledger` to `cp_steady` when a campaign has W8.
- **C8's W1** issues each transaction's micro-ops in ascending key order (stable). K4's W1 runs had 13 deadlocks (40P01) in 90 s (`20261011T232310Z-C4-1-real-4d9e8fd0`: 1 150 × 40001, 13 × 40P01): with row locks taken in key order W1 cannot deadlock, so every non-40001 error inside a judged switchover window is G8's business. C10's W1 is unchanged.

### Campaign shapes (§15 K5; P04 90 s; first fault at 10 s, then P05 gaps)

| Campaign | Steps (order from the nemesis stream) | Holds |
|---|---|---|
| C8 | F6(leader) ×3 | 5 s |
| C9 | F2a(leader), F6(leader), F2b(leader), once each, in a seeded order | 0, 5 s, 5 s |
| C10 | F2a(leader), F2b(leader), F1c(leader), F6(leader), once each, in a seeded order | 0, 5 s, 10 s, 5 s |
| C11 | odd seeds: none; even seeds: F2a(leader) at t = 10 s (inside the first storm phase) | 0 |

Every run sees every kind of its pool; with K3's timings (F6 confirmed ≈ 2 s, F2a healed ≈ 2–3 s) all steps fit in P04.

### Directed schedules (§9.2; seeds 1–5 for mutant and real build, P14)

- **D3** (`buffered`): three steps F2a(leader), hold 0; the first at t = 10 s (P05), each later one **15 s after the previous heal is confirmed** (no jitter): the plan's previous-heal anchor with a 15 s delay.
- **D5**: one step: F1b(A = followers, B = rest) held 20 s → F2a(leader) → F2b(same:1). Heals: F1b, then **30 s later** F2b (the old leader's pooler), then F2a (its PostgreSQL restarts once its pooler runs; §8's manual start after P28 otherwise). As for D2 (ADR-0024), the 30 s keep the crashed leader down while multiorch fails over (S21: 4 s + up to 8 s jitter; K3: new leaders 2.7–12.9 s after a kill); otherwise the old leader returns with the WAL the consumer already received and may be elected again, which is not the failure M5 models (logical decoding ahead of the cohort, then a failover to a node without that WAL, S13).
- **Detection** (§12.4): M3 via D3 — a run detects iff it is FAIL with `duplicate-elements` (C3) or `g1a` (C2), the two expected classes of §9.2; M5 via D5 — FAIL with `cdc-phantom` (C7).
- **Minimization target** (§12.2, ADR-0024): D3 → `duplicate-elements` if present, else `g1a`; D5 → `cdc-phantom`; every other K5 failure → the first class of the first invalid checker. A halved plan drops the W8 storm time past its deadline; KeepClients keeps the storm with plan client 100.

### C8 completions (§10.9), validated on the K5 corpus before any K5 run

- **G10.** A listener session lasts from its LISTEN's completion to the completion of its last op (its first non-ok op, or its last wait). An ok NOTIFY n is judged for listener client L iff (i) one session s of L was live over [n's invoke, n's completion + 10 s]; (ii) no fault window (first to last nemesis event of a fault id) overlaps that interval; and (iii) n was invoked after s's **first successful op post-recovery**: the first ok op of s invoked after the recovery of the latest fault window that ended before n was invoked, recovery being the completion of the first ok write (W4 insert or NOTIFY transaction) through L's gateway invoked after that window's end (P06's notion: "a successful write through each gateway"); without an earlier fault, the LISTEN itself. A judged NOTIFY that L never received within 10 s is `notify-not-resumed`; every other miss is the observation `notify-window-loss` (S19's reconnect windows). On K2's corpus (no faults; one connection break) the classification is unchanged.
- **G9 and G12** keep K2's rules: G9 is checked at every ok SHOW and EXECUTE (stricter than "after the failover window"); G12's ok fraction counts every storm window, also one a fault overlaps (LIM-25: a refinement needs an ADR, not a silent change).

### K5.3: the slot precondition

"Synced on standbys" is read as failover-ready, upstream's own readiness test (`synced AND NOT temporary AND invalidation_reason IS NULL`, `docs/ha/logical_slot_failover_design.md` §3.1.2, S12): a synced copy that is still temporary is dropped at promotion. On a development cluster (no faults) both standbys showed `cp_slot` `synced = true, temporary = true` 17 s after its creation and turned `temporary = false` only after the consumer confirmed advancing positions. Therefore, after setup and slot creation and before the workload starts, the runner polls every node every 1 s (`docker exec` psql on the node's socket; observation `src: "cdc-slot"`) and writes one warm-up row per second through gw-z1 (client 14, `cp.ledger` ids 8 000 000 000 + n, recorded like any W4 insert and checked like one) until every node in recovery among n1–n3 holds `cp_slot` failover-ready and the primary's `cp_slot` has `failover = true`, within 60 s (two of PostgreSQL's maximum slot-sync naptimes, 30 s); marker `cdc-slot-ready` {ready, waited_ms}. **C10:** not ready → the run ends INVALID (K5.3). **D5:** the marker is recorded and the run proceeds (K5.3 names C10; under M5 the standbys may never persist the slot, ADR-0029's source comment on `setSynchronizedStandbySlots`). The polling continues through the run for diagnosis (slot sync after each failover). cpgate K5.3 recomputes readiness from the observations before the first nemesis event of every C10 run.

### Evidence storage (§13, P25)

Committed in full: every C10 primary run (K5.3 reads its observations), and the primary and minimized runs of every real-build FAIL (triage, K5.5). Every other K5 run — the other primary runs, minimization candidates and replays — is committed as a stub (manifest, plan, seal; its checks.json beside it, verified against the seal by cpgate), its full archive kept under `runs/k5-full/` on the machine that ran it (LIM-30). The store step refuses to write evidence that would make `evidence/` exceed P25.

## Alternatives rejected

- **Fault kinds drawn with replacement (K4's C1 style).** With 3 (C9) or 5 (C10) runs some kind could miss a campaign; once each guarantees every kind of the pool in every run.
- **W5 state set once per run.** A reconnect would leave the rest of the run unjudged.
- **Judging G10 from the LISTEN on.** Counts S19's documented reconnect-window loss as a violation (K5 rejection condition).
- **A storm phase per fault.** C11 says one run without nemesis and one with F2a during a storm; seeds alternate.
- **Waiting for slot readiness after the workload starts.** The first fault at 10 s (P05) would often precede it, making most C10 runs INVALID.

## Consequences

- Contract 1.0.8 → 1.0.9: §6.1, §7, §9.2, §10.9, §13 and §15 K5 point here; K1–K4 plans are byte-identical apart from their contract version (new plan fields are emitted only when used).
- A real-build G10 or G12 failure is triaged against these definitions (K5.5); a C8 rule that misattributes is a checker defect, fixed with a corpus case, never by editing a run.

## Sources

CONTRACT §6–§10, §12, §13, §15 K5; S03, S12, S13, S14, S15, S18, S19, S21; evidence/K3/NOTES.md; evidence/K4/NOTES.md; ADR-0007, ADR-0024, ADR-0029.
