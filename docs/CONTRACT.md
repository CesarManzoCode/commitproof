# CommitProof — Normative Contract

| Field | Value |
|---|---|
| Contract version | **1.4.1** (K0; K1 corrections: ADR-0019, ADR-0020, ADR-0021; K3 corrections: ADR-0022, ADR-0023; K4 clarifications and corrections: ADR-0024, ADR-0025, ADR-0026; K4 amendment approved by the owner: ADR-0027; K5 corrections and clarifications: ADR-0029, ADR-0030; K5 amendment approved by the owner: ADR-0033; K5 change approved by the owner: ADR-0034, P25; K6 clarifications: ADR-0036; pre-K7 change approved by the owner: ADR-0037, G12; pre-K7 correction: ADR-0038, P23; K6b changes approved by the owner: ADR-0039, K7 storage and cited cells; ADR-0040, C13 and V4 removed; K6b clarification: ADR-0041, C12, slotless and the K7 campaign command) |
| Date | 2026-10-09 |
| Owner | César Manzo (GitHub `CesarManzoCode`) |
| System under test | Multigres, `github.com/multigres/multigres` @ `e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73` |
| Go module | `github.com/CesarManzoCode/commitproof` |
| License | Apache-2.0 (ADR-0012) |
| Language | English for all repository content (ADR-0014) |
| Companion documents | `docs/RESEARCH.md` (source IDs `Sxx`), `docs/adr/` (ADR-xxxx), `docs/LIMITATIONS.md`, `docs/PREREQUISITES.md`, `AGENTS.md`, `prompts/K1.md`…`K7.md` |

Normative words: **MUST**, **MUST NOT**, **SHOULD** follow RFC 2119. Anything this contract does not specify (internal package layout, file names inside packages, helper functions) is the implementer's choice, provided the interfaces in §5 and the gates in §15 hold.

Versioning: the contract changes only through an ADR (§18). Clarifications and corrections bump PATCH, new or changed requirements bump MINOR. No change may relax a gate, a criterion, a negative control or a threshold without the owner's explicit approval (§18).

---

## 1. Technical verdict

**Build it, with five corrections to the brief.** The core idea holds against the evidence: Multigres publicly claims that acknowledged commits survive failovers and split-brain (S48, S49, S42), its safety argument is a FlexPaxos-style quorum construction implemented on unmodified PostgreSQL synchronous replication (S05–S07, S76), and its own test suite is deterministic white-box scenarios without history checking, randomized fault schedules or a transport-level partition primitive (S27, S28). Upstream explicitly names one scenario its harness cannot reach — an old primary still reachable by a gateway after its orchestrator link is cut (S28). A public, history-checked, mutation-validated black-box verifier fills a real gap and speaks the project's language (Go, its protos, its issue format).

The corrections, each recorded in an ADR:

1. **Topology.** The upstream `docker-compose.yml` runs the whole cluster in one container with 2–3 cells (S01); that admits neither per-node network faults nor a fourth node. CommitProof runs a multi-container Compose topology whose component flags are copied from upstream's own Kubernetes demo (S03): one etcd, four pooler nodes (pgctld+PostgreSQL and multipooler sharing a network namespace, as in the upstream pod), three multiorch and three multigateway across three cells (ADR-0001). Kind was rejected (ADR-0001).
2. **Pinning by source, not by pulled digest.** Mutants require building from source, and the published registry was not reachable for verification (S04). Multigres is built from the pinned commit with digest-pinned base images; reproducibility is asserted on **binary** hashes, not image digests, because the upstream images install pgBackRest from an unpinned apt repository (S02) (ADR-0002).
3. **Clock skew is dropped.** Containers share the host kernel's realtime clock; Linux time namespaces do not virtualize `CLOCK_REALTIME`; Multigres binaries are static Go (`CGO_ENABLED=0`, S02), so `libfaketime` cannot affect them. Per-node skew is impossible on one host without VMs. Multigres safety uses logical terms (S06); its one cross-process wall-clock comparison (heartbeat lag, S32) is recorded as an untested sensitivity in LIMITATIONS (ADR-0005).
4. **CDC is in scope only as the upstream feature-flagged path.** The Supabase alpha states logical replication is not supported (S48); the open-source repo implements failover-capable logical slots behind `--enable-slot-based-replication` (S12, S13), which upstream's own Kubernetes demo enables (S03). CDC findings therefore carry claim level DESIGN, never EXPLICIT. The migration/materialization pipeline does not exist yet (S30) and is declared not covered (ADR-0011).
5. **Sharding/query routing for sharded databases cannot be tested** (no sharding, S48). It is declared not covered; the only routing tested is leader routing under faults. A differential "gateway vs. plain Postgres" suite was rejected because upstream already runs pg_regress/isolation/contrib parity (S45) and it would duplicate rather than complement (ADR-0011).

Plus three additions the research justified:

- **Commit-cancel campaign (C7).** PostgreSQL 17.7 acknowledges a commit whose synchronous-replication wait is cancelled, with only a WARNING (S61). Multigres enforces `statement_timeout` (default 30 s) by cancelling backend queries (S16) and forwards client cancels (S17). Whether an acknowledged-but-unreplicated commit can surface through the gateway and be lost on failover is an open, testable question against the EXPLICIT durability claim.
- **Same-timeline rewind campaign (C6)** targeting the pg_rewind divergence Multigres itself found with TLA+ (S44).
- **Historical control H1**: the real data-loss bug of issue #778 at commit `4bca1d5…` (S51) as a positive control alongside synthetic mutants (ADR-0015).

What CommitProof will **not** be able to say: anything about sharding, cross-region or cross-cloud behavior, power-loss durability (a container kill keeps the page cache; S09's `remote_write` hazard is unreachable here), clock-skew behavior, production Kubernetes operator behavior, or the Supabase-hosted service. A clean result means "no violation observed in the exact matrix of §12, with the stated statistical bound", nothing more.

---

## 2. Research findings

Each item is labelled **FACT** (directly supported by a cited source), **INFERENCE** (reasoned from facts; not verified) or **DESIGN DECISION** (a choice this project makes; ADR cited). A gate may depend on a FACT or a DESIGN DECISION, never on an INFERENCE.

### 2.1 Local deployment

- **FACT F1.** Upstream compose = single container, `local` provisioner, `MULTIGRES_NUM_CELLS` clamped to 2–3, `AT_LEAST_2` bootstrap, gateway on 15432, credentials `postgres`/`postgres` (S01).
- **FACT F2.** Upstream Kubernetes demo runs each component as a separate process with explicit flags (S03); pooler and pgctld share `/data` and localhost; backups (`/backups`) and pgBackRest certs (`/certs`) are shared; slot-based replication is enabled on pooler and gateway.
- **FACT F3.** Images: builder `golang:${GO_VERSION}-alpine`, finals `debian:bookworm-slim` and `postgres:17.7`; pgBackRest installed from apt unpinned; `.git` excluded from the build context; `go.mod` = `go 1.26` (S02).
- **FACT F4.** The only release tag `v0.1.0` (2026-05-30) is ~4 months behind `main`; the alpha announced 2026-10-01 runs current code (S04, S48). Nightly images exist on GHCR (S04) but could not be inspected.
- **FACT F5.** pgctld defaults `shared_buffers=64MB` (S26).
- **INFERENCE I1.** Peak memory of the 4-node topology under the §7 workload is ≈2–3 GiB (4×PostgreSQL ≈250 MiB, 4×pooler, 3×orch, 3×gateway ≈100 MiB each, etcd, helpers). Not measured; K1 measures and gates at P10.
- **DESIGN DECISION D1.** Multi-container Compose derived from S03 (ADR-0001); build from source at the pinned commit (ADR-0002).

### 2.2 Guarantees Multigres states

- **FACT F6.** EXPLICIT public claims: writes are acknowledged only after the cluster agrees they are durable via quorum-based consensus; acknowledged commits survive failovers and split-brain; failover promotes another node "within seconds" (S48, S49); "every committed write survives the failover", "the cluster does not split" (S42); "a Multigres cluster scales without losing data" (S41).
- **FACT F7.** No public statement defines read consistency or isolation (S43, S48, S49). The repository's own test treats primary-port reads as "leader reads" that must not be served by a superseded primary (S28) — a DESIGN-level expectation.
- **FACT F8.** DESIGN-level statements: user assignments to `synchronous_commit` are rejected with `0A000` (S11); planned failovers with buffering produce zero application-visible errors for single statements, while open transactions get `40001` (S14, S15); the gateway is authoritative for logical session state (S18); LISTEN/NOTIFY can lose notifications during reconnect windows (S19, documented limitation); logical-slot failover aims for "no loss of data during a failover" (S12).
- **INFERENCE I2.** Because Multigres targets "full compatibility with standard Postgres" (S48), a client may expect single-node PostgreSQL isolation semantics through the primary port. This is labelled INFERRED in findings, never EXPLICIT.

### 2.3 Consensus as implemented

- **FACT F9.** Rule = (rule number, leader, cohort, durability policy); stored in `multigres.current_rule` + `multigres.rule_history` and changed by WAL-replicated transactions; rule number `(coordinator_term, leader_subterm)`; positions totally ordered by `(rule, LSN)` (S06).
- **FACT F10.** Failover: Recruit (stop replication participation, persist `TermRevocation` to disk, report position) → first safe proposal → Promote (`pg_promote`, rule write that blocks until a synchronous standby acks) / SetPrimary (repoint, `pg_rewind` if diverged). Recruitment threshold `max(⌊n/2⌋+1, n−N+1)` over the outgoing cohort (S07).
- **FACT F11.** Membership: leader-led `UpdateConsensusRule(ADD|REMOVE)` with compare-and-swap on the expected outgoing rule; ADD for any healthy, replicating, eligible pooler not in the cohort; REMOVE for a member missing from the live cache (tombstone after graceful SHUTDOWN, or gated by `IsCohortMemberRemovalSafe`) (S24, S25). On ADD/REMOVE the primary also recomputes `synchronized_standby_slots` (S13).
- **FACT F12.** Supported policies: `AT_LEAST_2`, `MULTI_CELL_AT_LEAST_2` (S23). HA runs `synchronous_commit=on` (S09).
- **FACT F13.** Interrupted rule changes ("propagation") are documented as not implemented (S07); `UpdateRule` fails closed on an undecided proposal for non-promotion writes (S25).
- **INFERENCE I3.** For `AT_LEAST_2`, the majority term and the revocation term of the recruitment threshold coincide for cohorts of 3 and 4 (both 2 and 3 respectively). A mutant that drops either term is therefore *equivalent* in every configuration CommitProof can run; such mutants are rejected (§9.3). K4's quorum-table check (§10.8) turns this inference into a measured fact.

### 2.4 Fault injection

- **FACT F14.** Upstream's harness has process kill and SIGSTOP but no transport-level partition (S27, S28).
- **FACT F15.** Static Go binaries (`CGO_ENABLED=0`) (S02); heartbeat lag compares two processes' wall clocks (S32).
- **DESIGN DECISION D2.** Partitions and latency via `iptables`/`tc` executed in each component's network namespace by an unprivileged-host helper container holding only `NET_ADMIN`; kill and pause via Docker and in-container signals; slow disk via a FUSE pass-through filesystem with injectable latency (`slowfs`), with a pre-decided fallback (ADR-0005, ADR-0006). No clock skew (ADR-0005).

### 2.5 Checkers

- **FACT F16.** Elle is Clojure, EPL-2.0/GPL-2.0+CPE (S70); elle-cli 0.1.11 wraps Elle 0.2.7 and needs Java 21 (S73); Porcupine v1.3.1 is Go/MIT with timeouts and partitioning (S74); a Go port of Elle exists inside PingCAP tipocket, derived from Elle's code (S75).
- **DESIGN DECISION D3.** Reimplement the needed Elle subset in Go from the paper and Adya's definitions (S71, S72), wrap Porcupine as a library, and use elle-cli (in a pinned container, never redistributed) as a differential oracle in K2 (ADR-0004).

### 2.6 CDC, pooler, gateway

- **FACT F17.** Without the flag, only temporary slots are allowed and failover tears the stream down; with it, failover slots are synced and `synchronized_standby_slots` gates logical decoding on the cohort (S12, S13).
- **FACT F18.** Gateway defaults: `--buffer-enabled=false`, `--statement-timeout=30s`; with buffering enabled, a failover less than `--buffer-min-time-between-failovers` (1 min) after the previous buffering episode ended is not buffered (S14, S16; ADR-0029).
- **FACT F19.** PostgreSQL acknowledges a commit whose sync-rep wait was cancelled, with a WARNING only (S61).

### 2.7 Existing verification and conventions

- **FACT F20.** Upstream verification: pg_regress-family suites (S45), deterministic end-to-end scenarios (S27), TLA+ design models outside the repo (S44), an internal "Multiquake" chaos test of unknown scope (S29), and external manual fault testing that found data loss in March 2026 (S51).
- **FACT F21.** Issue conventions: `component: description` titles; summary, version/environment, reproduction, expected vs actual, analysis, suggested fix; `alpha-known-gap` label for accepted gaps; DCO sign-off for commits (S31, S50).
- **DESIGN DECISION D4.** CommitProof's contribution claim is limited to: public, reproducible, history-checked, mutation-validated, seed-replayable fault testing of the implementation. It does **not** claim to be the first fault testing of Multigres (S29, S51).

---

## 3. Guarantees under test

Claim levels: **EXPLICIT** (public product statement), **DESIGN** (repository design doc, decision log or test intent), **INFERRED** (expectation derived from PostgreSQL compatibility; I2). Every finding cites exactly one guarantee ID.

| ID | Guarantee (as tested) | Level | Source | Checker(s) | Campaigns |
|---|---|---|---|---|---|
| G1 | A write acknowledged as committed to a client is present after any sequence of failovers, partitions, pauses and membership changes, once the cluster has recovered. | EXPLICIT | S48, S49, S42 | C1 lost-acked-writes | C1–C7, C12 |
| G2 | No split brain: the history admits a single total order of committed transactions; no two leaders commit divergent durable histories. Observable as: no acknowledged write lost (G1) **and** no list-append incompatible order / no read observing a write that later disappears. | EXPLICIT | S48, S42 | C1, C2, C3 | C2, C3, C4, C6, C12 |
| G3 | Membership changes (3→4→3) do not lose acknowledged writes, including a membership change concurrent with leader failure. | EXPLICIT | S41 | C1, C3 | C4 |
| G4 | A write the client saw definitively fail never becomes visible (no G1a / aborted read). | INFERRED | I2, S72 | C2, C3 | all |
| G5 | Transactions at SERIALIZABLE through the primary port are serializable; at REPEATABLE READ they satisfy snapshot isolation; across failovers. | INFERRED | I2, S63 | C3, C5 | C1–C6, C12 |
| G6 | Single-key autocommit reads and writes through the primary port are linearizable (no stale read from a superseded primary). | DESIGN | S28 | C4, C6 | C2, C3, C12 |
| G7 | User assignment to `synchronous_commit` is rejected with SQLSTATE `0A000`; `ALTER SYSTEM` is rejected. | DESIGN | S11 | P (probes P1, P2; §10.12) | K4 probe suite |
| G8 | With buffering enabled, a planned failover that the documented configuration buffers (not within `--buffer-min-time-between-failovers`, 1 min, of the previous buffering episode; S14, ADR-0029) produces zero client-visible errors for single autocommit statements; explicit transactions open across the switch receive `40001` and nothing else. | DESIGN | S14, S15 | C9 availability | C8 |
| G9 | Gateway-tracked session settings and named prepared statements remain effective for the same client session after a failover. | DESIGN | S18 | C8 session-state | C9 |
| G10 | LISTEN subscriptions resume after a failover: notifications sent after recovery are delivered. Loss during reconnect windows is expected and is measured, not judged. | DESIGN | S19 | C8 session-state | C9 |
| G11 | A logical-replication consumer of a failover slot, reconnecting through the gateway after failover, observes every acknowledged committed change (no gap), no change that is absent from the final committed state (no phantom), and no duplicate beyond its last confirmed flush. | DESIGN | S12, S13 | C7 CDC | C10 |
| G12 | Per-user pools share fairly: under a connection storm by one role, another role with steady demand still obtains connections and completes operations; overload alone does not trigger a failover. | DESIGN | S40 ("Per-user pools that share fairly"), S22 | C9, C8 | C11 |
| G13 | Bank invariant: the sum of balances is constant and no balance is negative in any committed state or consistent read. | INFERRED | I2 | C5 | C1–C6 |

Measured, not judged (reported as observations): failover duration and unavailability windows (vs. "within seconds", S48), LISTEN notification loss windows (S19), latency under injected network delay, number of rule changes per fault (cascading re-election check, S50 #1068).

---

## 4. Job bullet → evidence matrix

Coverage values: **COVERED** (direct executable evidence), **PARTIAL** (some aspect evidenced, the rest declared), **NOT COVERED** (declared, with reason). The K7 README reproduces this table verbatim with links to evidence.

| # | Bullet (S47) | Coverage | Evidence (gate / artifact) | What is explicitly not claimed |
|---|---|---|---|---|
| 1 | Query routing logic for sharded databases | **PARTIAL** (non-sharded only) | Leader routing under faults: G6 (no stale reads from superseded primary), G8 (buffered routing during switchover), mutant M3 (unsafe retry in the gateway's error classifier) — gates K5.4, K4.3 | Any sharded routing, query splitting, scatter/gather. Multigres has no sharding at the pinned commit (S48). |
| 2 | Consensus and replication for distributed durability | **COVERED** | G1–G3 campaigns C1–C7 (K4), mutants M1/M2 detection, historical control H1, minimized replayable findings — gates K4.2–K4.6, K7.2 | Proofs. Results are empirical over the §12 matrix. |
| 3 | Materialization pipelines for migrations and CDC | **PARTIAL** (CDC failover only) | G11 campaign C10, mutant M5 (logical decoding not gated by standby slots) — gate K5.3 | Migrations and materialization: the migrator exists only as a proto (S30). No pipeline is built or tested. |
| 4 | Connection pooling and workload isolation | **COVERED** (as fault behavior) | G8–G10, G12 campaigns C8, C9, C11; mutant M3 — gates K5.1, K5.2, K5.4 | Pool performance optimization; pooler internals beyond black-box behavior. |
| 5 | Open-source collaboration | **PARTIAL** | Upstream-format evidence packages for every finding (§11.4), DCO-ready patches for mutants, issue drafts — gate K7.4 | Any actual upstream interaction: publication is the owner's decision (AGENTS.md). |
| 6 | Sharding, relational algebra, Postgres internals | **PARTIAL** (internals only) | Internals exercised and explained: sync-rep cancel semantics (C7, S61), pg_rewind same-timeline divergence (C6, S44), WAL/LSN and timeline reasoning in findings; isolation checking (C3) | Sharding and relational algebra: nothing to test (S48). Visibility order and snapshot isolation on replicas: not tested (C13 removed, ADR-0040; LIM-42). |
| 7 | Consensus protocols (Raft, Paxos, FlexPaxos), distributed transactions | **PARTIAL** | FlexPaxos quorum-intersection table cross-checked against upstream `CheckSufficientRecruitment` (§10.8, gate K4.1); equivalence analysis of recruitment mutants (§9.3); consensus campaigns (K4) | Distributed transactions: single shard, no 2PC in scope; Raft/Paxos implementations are not built. |
| 8 | Stream processing, materialization, CDC | **PARTIAL** | CDC consumer with gap/phantom/duplicate checking across failover (C10, C7 checker) — gate K5.3 | Stream processing frameworks, materialized views. |
| 9 | Robust, high-performance distributed systems with strong observability | **COVERED** (for the harness) | The harness itself: fail-closed fault confirmation (K3), sealed hash-chained histories (K1), byte-reproducible incident timeline/report with metrics (K6), statistics (K7) | Production deployment of CommitProof; performance tuning of Multigres. |
| 10 | Low-latency infrastructure and network protocol optimization | **PARTIAL** (measurement only) | Wire-protocol-level outcome capture (SQLSTATE, NOTICE/WARNING), commit-latency vs injected RTT (F7, C1/C12), failover latency distributions (K6, K7) | Any optimization of Multigres or of a network protocol. |
| D78 | Discussion #78 scale expectations | **PARTIAL** | Load spikes/overload (C11), timeouts and retries (M3, C7, G8), cascading failures (rule changes per fault, observations), minimal-disruption deploys (C8 graceful switchover), observability (K6) | Cross-cluster and cross-cloud interaction: single host. |

---

## 5. Architecture and boundaries

### 5.1 Topology (profile `baseline`)

Compose project name **`commitproof`**. One user-defined bridge network `cpnet`, subnet **`10.77.0.0/24`** (P26). Static addresses:

| Service | Image | Address | Cell | Process(es) / key flags |
|---|---|---|---|---|
| `etcd` | `quay.io/coreos/etcd:v3.6.4` (digest locked in K1, §14) | 10.77.0.10 | — | client port 2379 |
| `pg-n1` … `pg-n4` | `commitproof/pgctld` (built, §5.2) | .11 … .14 | n1 zone1, n2 zone2, n3 zone3, n4 zone1 | `pgctld server` with S03 flags (`--pooler-dir=/data --grpc-port=16200 --pg-port=5432 --pgbackrest-port=8432 --pgbackrest-cert-dir=/certs`) |
| `pooler-n1` … `pooler-n4` | `commitproof/multigres` | shares `pg-nX` netns (`network_mode: service:pg-nX`) | as node | `multipooler` with S03 flags (`--http-port=16000 --grpc-port=16100 --pgctld-addr=localhost:16200 --connpool-global-capacity=50 --enable-slot-based-replication=true`), `--service-id=nX`, `--cell=<zone>` |
| `orch-z1` … `orch-z3` | `commitproof/multigres` | .21 … .23 | zone1…zone3 | `multiorch --http-port=17000 --grpc-port=17100 --watch-targets=postgres --pooler-health-check-interval=500ms --recovery-cycle-interval=500ms` (S03 values) |
| `gw-z1` … `gw-z3` | `commitproof/multigres` | .31 … .33 | zone1…zone3 | `multigateway --http-port=15000 --grpc-port=15100 --pg-port=15432 --enable-slot-based-replication=true` |
| `nem-<svc>` (one per network namespace: etcd, pg-n1..n4, orch-z1..z3, gw-z1..z3, runner) | `commitproof/nemesis` (alpine 3.24.2 + iptables + iproute2 + CommitProof's `cpnet` probe tool) | shares target netns | — | idle; executes nemesis commands; capability `NET_ADMIN` only. `nem-runner` is started by the runner in its own namespace at run time (ADR-0023) |
| `runner` | `commitproof/runner` | .40 | — | the `commitproof` binary; Docker socket mounted |

Common flags for all Multigres components: `--topo-global-server-addresses=etcd:2379 --topo-global-root=/multigres/test/global` (S03). Cluster metadata: `multigres createclustermetadata --global-topo-address=etcd:2379 --global-topo-root=/multigres/test/global --cells=zone1,zone2,zone3 --durability-policy=AT_LEAST_2 --backup-location=/backups`. Shared volumes: `backups` (all `pg-*` and `pooler-*`), `certs` (generated once per cluster by an init service), per-node `data-nX`. `n4` is **not started** at cluster up; it exists only for fault F5.

Bootstrap HEALTHY predicate (used by K1.4 and by every run): (a) exactly one pooler reports itself leader of a decided rule and all of n1–n3 report that rule; (b) the rule's cohort is exactly {n1, n2, n3}; (c) a write and a read-back succeed through each of gw-z1, gw-z2, gw-z3; (d) `SELECT multigres.version()` through gw-z1 reports the pinned commit. Observation of (a)/(b) uses pooler status RPCs (§5.6, observations only).

The derivation of every flag from S03 is recorded in `deploy/DERIVATION.md` (created in K1): one line per flag with its S03 file:line, or the reason for divergence. Divergences allowed: service IDs, cells, addresses, volume layout, OpenTelemetry/pprof flags removed, `--hostname` set to the service name, the process user of multipooler (it runs as the pgctld image's postgres user, uid 999, so both processes of a node share `/data`, `/certs` and `/backups`; ADR-0021), the pg_hba template's `local all all` line set to `trust` instead of `peer` (`deploy/pg_hba_template.baseline.conf`: the pooler's per-user pools connect as the client's role over the Unix socket, which `peer` refuses for every role but the pooler's OS user; ADR-0029), and the OpenTelemetry Prometheus exporter enabled on every multigateway, multipooler and multiorch (`OTEL_METRICS_EXPORTER=prometheus`, `OTEL_EXPORTER_PROMETHEUS_HOST=0.0.0.0`, `OTEL_EXPORTER_PROMETHEUS_PORT=9464`) instead of S03's OTLP push to a collector, so that P23's scrapes exist (ADR-0038).

### 5.2 Images

- `commitproof/multigres` and `commitproof/pgctld` are built from upstream's `Dockerfile` and `Dockerfile.pgctld` at the pinned commit, copied into `build/` with **only `FROM` lines changed** to digest-pinned references (§14). A test enforces that the diff against upstream contains nothing but `FROM` lines (K1.3).
- Build args: `GIT_COMMIT=e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73`, `COMMIT_DATE=2026-10-09T14:17:55+02:00`.
- `commitproof/pgctld` additionally layers the `slowfs` binary and `fuse3` in a separate final stage of a CommitProof-owned Dockerfile `FROM commitproof/pgctld-upstream:real`; the build fails unless that tag is the upstream-derived image just built and its layers are a prefix of the result, and the lock records both image IDs (BuildKit cannot resolve `FROM <local image ID>`; ADR-0022). The upstream-derived part stays byte-identical except `FROM`.
- Mutant images: same Dockerfiles, source tree = pinned commit + exactly one patch from `mutants/` (§9). Tag `commitproof/multigres:M<n>`.
- `images.lock.json` records: source commit, sha256 of each Dockerfile, base image digests, local image IDs, sha256 of every binary inside every image, `pgbackrest version` output, patch sha256 for mutants. `commitproof cluster up` MUST refuse to start if a local image ID differs from the lock (fail-closed).

### 5.3 Binaries and their boundaries

| Binary | Responsibility | May import |
|---|---|---|
| `commitproof` | build/lock images, cluster lifecycle, plan generation, workload clients, nemesis, observations, minimization driver, replay driver, campaign driver | anything |
| `cpcheck` | read sealed run directories; verify seals; run checkers; write `checks.json` | `history`, `checker` packages, Porcupine |
| `cpgate` | evaluate sprint gates from evidence directories; run negative controls on evidence copies | `history`, `gate`, `stats` packages |
| `cpreport` | render the static report from sealed runs | `history`, `checker` output types, `stats`, `report` (and the utility packages `canonjson`, `params`, as `cpgate`; ADR-0036) |

Independence rule (enforced by a test that runs `go list -deps` per binary): `cpcheck`, `cpgate` and `cpreport` MUST NOT depend on runner, nemesis, cluster, workload or observation packages, and MUST NOT import any Multigres package. The verdict on a run is produced by `cpcheck` from the sealed history alone; the gate verdict is produced by `cpgate` from files on disk alone. Neither ever trusts a status reported by `commitproof` itself.

Black-box boundary (ADR-0016): safety verdicts use only client-observed history (§6). Multigres internals (pooler status RPCs, `multigres.rule_history`, `synchronous_standby_names`, `/metrics`) are used only for (i) confirming that a fault happened (§8), (ii) the HEALTHY predicate, (iii) diagnosis and timelines (K6). An observation never turns a FAIL into a PASS.

### 5.4 Run directory (sealed, immutable)

`runs/<run-id>/` where `run-id = <UTC yyyymmddThhmmssZ>-<campaign>-<seed>-<mutant|real>-<first 8 hex of plan sha256>`. Contents:

| File | Content |
|---|---|
| `manifest.json` | contract version, run id, campaign, profile, seed, mutant, sha256 of `images.lock.json`, sha256 of `plan.json`, commitproof git commit and dirty flag, host kernel, Docker version, start wall time (RFC 3339, informational only) |
| `plan.json` | the deterministic schedule (§6.3), canonical JSON |
| `history.jsonl` | client and nemesis events (§6.1), hash-chained |
| `observations.jsonl` | system observations and metric scrapes, hash-chained |
| `logs/` | `docker compose logs --timestamps` per service, captured at teardown |
| `commands.jsonl` | every external command executed (argv, start/end monotonic ns, exit code, sha256 of stdout/stderr) |
| `seal.json` | sha256 of every other file, and the Merkle root over the sorted list |

After sealing, every file is chmod `0444` and the directory `0555`. `cpcheck verify-seal` recomputes all hashes and the chains. Any mismatch makes every consumer refuse the run.

### 5.5 Profiles

| Profile | Difference from `baseline` | Used by |
|---|---|---|
| `baseline` | — | default everywhere |
| `buffered` | gateways add `--buffer-enabled=true` (defaults otherwise, S14: window 10 s, size 1000, max failover duration 20 s, min time between failovers 1 min); the runner waits 60 s after the run-start HEALTHY evaluation before the workload starts, so that bootstrap's buffering episode cannot suppress the first failover's (ADR-0029) | C8, mutant M3 |
| `slotless` | pooler and gateway `--enable-slot-based-replication=false` | K7 variant V2 |
| `h1` | topology for historical commit `4bca1d5770a6…` with flags taken from that commit's `demo/k8s/*.yaml` (derivation file `deploy/DERIVATION-h1.md`); images from that commit's Dockerfiles, FROM-only; HEALTHY evaluated as (a') one node out of recovery, (b') its `synchronous_standby_names` names exactly the other two, both streaming, (c) as §5.1, (d') the containers run the locked h1 images — the commit has no rule model and no `multigres.version()` (ADR-0025) | H1 |

### 5.6 Command-line interfaces (fixed)

Exit code 0 = success, 1 = verdict FAIL, 2 = INVALID/UNKNOWN, 3 = usage or prerequisite error. All commands print one JSON summary line on stdout last.

```
commitproof images build [--mutant M<n> | --profile h1] [--no-cache]  # builds, writes/updates images.lock.json
commitproof images verify                                    # local image IDs == lock
commitproof cluster up|down|verify --profile <p> [--mutant M<n>]
commitproof plan --campaign <C> --seed <u64> [--profile <p>] # prints plan.json to stdout
commitproof run  --campaign <C> --seed <u64> [--profile <p>] [--mutant M<n>] [--directed D<n>] --out runs/
commitproof probe --suite claims [--mutant M<n>] [--seed <u64>] --out runs/
commitproof minimize <run-dir> --out runs/ [--cpcheck <bin>]  # writes a new run dir for the minimal plan
commitproof replay <run-dir> --times <k> --out runs/ [--cpcheck <bin>]
commitproof campaign --matrix <file> --out runs/ [--cpcheck <bin>] [--dry-run]  # resumable: skips sealed runs already present; --dry-run prints the cells and the runs still to make (ADR-0041)
cpcheck verify-seal <run-dir>
cpcheck check <run-dir> [--elle-oracle]                      # writes <run-dir>.checks.json next to the run (not inside the seal)
cpcheck corpus <dir>                                         # K2 synthetic corpus
quorumxcheck                                                 # §10.8 (separate binary; imports Multigres consensus package)
cpgate K<n> --evidence evidence/K<n> [--negative-controls]
cpreport render <run-dir|campaign-dir> --out <dir>           # a run dir may be given as its §13 archive <run-id>.tar.zst (ADR-0036)
```

`commitproof cluster up` with a prerequisite missing (Docker unreachable, `/dev/fuse` absent, `sch_netem` unavailable, image lock mismatch) exits 3 with the missing item named. Nothing is skipped silently.

### 5.7 Fixed parameters

All numbers used by gates live here. Implementations read them from one Go constants file mirrored from this table; a test compares the constants file to this table (parsed from the Markdown) so that the contract and the code cannot drift.

| ID | Parameter | Value | Constraint it must satisfy (checked in K0) |
|---|---|---|---|
| P01 | Client operation timeout | 10 s | < gateway `--statement-timeout` default 30 s (S16) |
| P02 | Clients per run | 10: list-append 4, register 2, bank 2, ledger 2 | — |
| P03 | Pacing per client | one operation every 50 ms; if the previous op is still running, the next starts on completion (closed loop) | — |
| P04 | Workload duration | 90 s (campaigns), 45 s (CI smoke, K1 smoke) | — |
| P05 | Fault cadence | first fault at t = 10 s; each fault lasts the plan's duration; next fault starts 10 s + seeded jitter in [0, 5) s after the previous heal is confirmed | — |
| P06 | Recovery deadline | 120 s from final heal to a successful write through each gateway | > P16 long pause + multiorch grace 4 s + jitter 8 s (S21) |
| P07 | Quiescence | 10 s after recovery before final reads | — |
| P08 | Final read stability | per gateway, 3 consecutive identical final reads 2 s apart, within 60 s | — |
| P09 | Cluster HEALTHY deadline | 300 s | — |
| P10 | Memory budget | peak sum of container memory usage ≤ 6 GiB | ≤ 16 GiB host, ≤ 16 GB CI runner (S88) |
| P11 | Porcupine timeout per partition | 60 s → UNKNOWN | — |
| P12 | Cycle search budget per strongly connected component | 10 s → UNKNOWN | — |
| P13 | Replays per minimized failure | 5; REPRODUCED if ≥ 3 | — |
| P14 | Directed runs per mutant | 5; DETECTED if ≥ 4 | — |
| P15 | K7 repetitions | 10 per cell of V1 and V3; 5 per cell of V2, V5; mutants and H1 per P14 (5) | — |
| P16 | Pause durations | short 3 s; long 60 s | long > `leader-postgres-response-threshold` 30 s (S21) |
| P17 | slowfs injected latency | 200 ms per `write`/`fsync`; confirmed if median probe ≥ 150 ms and baseline median < 50 ms | — |
| P18 | netem delay | 50 ms one-way on egress of the target netns; confirmed if measured TCP connect RTT increase ≥ 40 ms | — |
| P19 | C7 session `statement_timeout` | 2000 ms | < P01; gateway-managed (S16) |
| P20 | List-append keys | 8 active keys; a key retires at 64 elements and a fresh key replaces it | — |
| P21 | Register keys | 5 keys; written values unique per run | — |
| P22 | Bank | 5 accounts × 100 | — |
| P23 | Metrics scrape interval | 1 s (`/metrics` of the OpenTelemetry Prometheus exporter on port 9464 of every multigateway, multipooler and multiorch; the HTTP ports 15000, 16000, 17000 serve no `/metrics` at the pinned commit — ADR-0038) | — |
| P24 | Observation interval | 1 s | — |
| P25 | Evidence size budget in git | ≤ 250 MB total under `evidence/` and the run archives under `findings/` (ADR-0039) | — |
| P26 | Docker subnet | 10.77.0.0/24 | configurable via `CP_SUBNET`; gates use the default |
| P27 | Fault confirmation deadline | 15 s after the inject command returns | — |
| P28 | Heal confirmation deadline | 30 s | — |

---

## 6. History model

### 6.1 Event schema

`history.jsonl`: one JSON object per line, UTF-8, **canonical JSON** (object keys sorted lexicographically, no insignificant whitespace, integers only — no floats, strings NFC). Fields:

| Field | Type | Meaning |
|---|---|---|
| `i` | int | 0-based line index |
| `prev` | hex string | sha256 of the previous line's bytes (without newline); `"0"×64` for line 0 |
| `t` | int | nanoseconds since run start, from the runner's monotonic clock |
| `type` | enum | `invoke`, `ok`, `fail`, `info`, `nemesis`, `marker` |
| `proc` | int | logical process; a process issues one op at a time; after an `info` completion the process is retired and the client continues as a new `proc` (Jepsen semantics) |
| `client` | int | physical client index (0–9 for P02's clients; the runner's recovery and final-read clients are 10–12; K5's workloads use the numbers of ADR-0030) |
| `gw` | string | gateway used (`gw-z1`…) |
| `wl` | enum | `append`, `register`, `bank`, `ledger`, `session`, `cdc`, `notify`, `probe`, `overload` |
| `f` | string | operation (`txn`, `read`, `write`, `cas`, `transfer`, `read-all`, `insert`, …) |
| `value` | object/array | operation payload; for completions, the observed result |
| `iso` | enum | `serializable`, `repeatable-read`, `read-committed` (txn ops only) |
| `err` | object | `{sqlstate, class, phase}`; `phase` ∈ `connect`, `begin`, `statement`, `commit`, `client-timeout`, `transport` |
| `notices` | array | server notices/warnings received during the op: `{severity, sqlstate, message}` (message truncated to 256 bytes) |
| `op` | int | id of the invoke this completion answers (completions only) |
| `fault` | object | nemesis events only: `{id, kind, phase, targets, evidence}`; `phase` ∈ `inject`, `confirmed`, `heal`, `healed`, `not-applied`, `not-healed` |

`observations.jsonl` uses the same chaining and canonicalization with `type: "obs"` and source-specific payloads.

### 6.2 Completion classification (closed table)

A completion is `ok` only if the client received an explicit success for the whole operation; `fail` only if the error proves the operation did not take effect; otherwise `info`.

| Situation | Classification |
|---|---|
| Success response (`CommandComplete` for every statement, `COMMIT` acknowledged) | `ok` (any notices recorded) |
| SQLSTATE class `40` (serialization failure, deadlock) at any statement **before** `COMMIT` returns | `fail` |
| SQLSTATE `40001` returned **by `COMMIT`** | `fail` (PostgreSQL aborts on serialization failure at commit) |
| SQLSTATE class `23` (integrity constraint) before commit | `fail` |
| SQLSTATE `0A000`, `42xxx`, `22xxx` before any write statement executed | `fail` |
| SQLSTATE `25006` (read-only) on a statement, before commit | `fail` |
| SQLSTATE `MTF01`/"no writable primary" before the first statement executed | `fail` |
| Any error, timeout or transport failure during or after sending `COMMIT` (including `57014`, `57P01`, `08xxx`, gRPC/transport errors surfaced by the gateway) | `info` |
| Client-side timeout (P01) at any phase after the first write statement was sent | `info` |
| Connection error before any statement was sent | `fail` |
| Any SQLSTATE not listed above | `info` |
| Autocommit single statement: transport error or timeout after sending | `info` |

`COMMIT` returning success **with** the WARNING text `canceling wait for synchronous replication` (S61) is `ok` and the notice is recorded; C1 treats it exactly like any other `ok`. K2 tests every row of this table.

### 6.3 Plan

`plan.json` is a pure function of `(campaign, seed, profile, directed schedule id, contract version)`. Randomness: Go `math/rand/v2` PCG source `rand.NewPCG(seed, <campaign-specific constant>)`, one independent stream per client and one for the nemesis (stream constant = FNV-1a 64 of the stream name). The plan lists, per client, the ordered operations with their payloads and target gateway, and, for the nemesis, the ordered faults with kind, targets (by role: `leader`, `random-follower`, by name, …; roles are resolved at runtime and the resolution is recorded in the history), durations and gaps. The plan contains no wall-clock times. Same inputs → byte-identical `plan.json` (K1.6).

### 6.4 Final state

After recovery (P06) and quiescence (P07), the runner performs final reads through every gateway (P08): all list-append keys, all registers, all bank accounts, the full ledger. These are recorded as ordinary `invoke`/`ok` events with `f: "final-read"`. A run whose final reads cannot complete is `INVALID` (§12.1), never PASS.

---

## 7. Workloads

All tables live in database `postgres`, schema `cp`, created by the runner at run start through gw-z1 in a single setup transaction (recorded).

| ID | Workload | Schema / operations | Feeds checkers |
|---|---|---|---|
| W1 | list-append | `cp.append(k int primary key, vals bigint[] not null)`; transactions of 1–4 micro-ops, each `[append k v]` (`insert … on conflict (k) do update set vals = cp.append.vals || excluded.vals`) or `[r k]` (`select vals`); `v` unique per key; isolation per campaign (default `serializable`; C3 also `repeatable-read`) | C1, C2, C3 |
| W2 | register | `cp.reg(k int primary key, v bigint)`; autocommit `read`, `write`, `cas` (`update … where v = $old returning v`) | C4, C6 |
| W3 | bank | `cp.acct(id int primary key, bal bigint not null)`; `transfer(from,to,amt)` and `read-all` in one transaction; `serializable` | C5 |
| W4 | ledger | `cp.ledger(id bigint primary key, payload text not null)`; autocommit inserts with unique ids | C1, C2 |
| W5 | session | per client session: `SET` of tracked GUCs (`application_name`, `search_path`, `work_mem`, `DateStyle`), `PREPARE`/protocol-level named statements, `SHOW`/`EXECUTE` probes before and after failover; the session state is set up again on every new connection (ADR-0030) | C8 |
| W6 | notify | one listener session per gateway, `LISTEN cp_chan`; notifier inserts a ledger row and `NOTIFY cp_chan, '<id>'` in the same transaction (one notifier per gateway; listeners wait in 1 s steps until 11 s after the workload; ADR-0030) | C8 |
| W7 | cdc | consumer connects through gw-z1 in logical replication mode, creates a non-temporary `pgoutput` slot with `failover=true` on publication `cp_pub` (tables `cp.ledger`, `cp.append`), streams, sends standby status updates every 1 s with `confirmed_flush` = last fully received commit LSN — the transaction's commit **end** LSN (pgoutput COMMIT `end_lsn`), from which PostgreSQL resumes without re-sending it (ADR-0029) — and reconnects after any error with the same slot name, starting at its last confirmed flush | C7 |
| W8 | overload | role `cp_storm` opens connections at up to 4× `--connpool-global-capacity` (50, S03) and runs `select pg_sleep(0.5)`; role `cp_steady` runs W4 at P03 pacing, each insert returning `pg_postmaster_start_time()` as the server identity (storm phases and roles: ADR-0030) | C9, C8 |

---

## 8. Fault catalog (closed)

Every fault has: an injection mechanism, a **confirmation predicate** evaluated against the live system after injection (deadline P27), a heal mechanism and a **heal predicate** (deadline P28). If a confirmation or heal predicate is not satisfied in time, the runner records `not-applied`/`not-healed`, aborts the workload, tears down, and the run is `INVALID` (fail-closed). Nemesis commands run through `docker exec` into the `nem-*` helper of the target namespace, or `docker` lifecycle commands, all recorded in `commands.jsonl`.

| ID | Fault | Injection | Confirmation predicate | Heal / heal predicate |
|---|---|---|---|---|
| F1a | Isolate node | iptables `DROP` in both directions between the target node netns and every other `cpnet` address, installed in the target's netns and in every peer's netns | TCP connect from target to each peer and from each peer to target fails within 1 s (probed from both helpers); iptables packet counters > 0 | delete rules; all probes connect |
| F1b | Majority/minority split | sets {A} and {B} given by plan (e.g. leader + orch-z1 vs. rest) | as F1a for every A–B pair; A–A and B–B connect | as F1a |
| F1c | Asymmetric: isolate leader from orchestrators, followers and etcd, **not** from gateways | rules on the leader netns and peers excluding gw-* | leader↔{orch,followers,etcd} fail; leader↔gw-* connect | delete; all connect |
| F1d | One-way drop leader → followers | `OUTPUT` drop on leader towards followers only | leader→follower connect fails; follower→leader TCP SYN reaches leader (counter) | delete; connect |
| F2a | Kill PostgreSQL | `kill -9` of the postmaster PID inside `pg-nX` | postmaster PID absent from `/proc` | multipooler/pgctld restarts PostgreSQL by itself; heal predicate: a new postmaster PID exists and accepts `select 1` on the node, within P28. If it does not restart by itself, the runner records that observation and starts it via pgctld; the run stays valid |
| F2b | Kill multipooler | `docker kill -s KILL pooler-nX` | container state `exited`, exit code 137 | `docker start`; container `running` and `/ready` on 16000 = 200 |
| F2c | Kill multiorch | `docker kill -s KILL orch-zX` | as F2b | as F2b (`/ready` on 17000) |
| F2d | Kill multigateway | `docker kill -s KILL gw-zX` | as F2b | as F2b (PG port accepts) |
| F2e | Kill etcd | `docker kill -s KILL etcd` | as F2b | `docker start`; `etcdctl endpoint health` ok |
| F3a | Pause PostgreSQL | `kill -STOP` all `postgres` processes of node (postmaster and children) | every targeted PID state `T` in `/proc/<pid>/stat`; `psql -c 'select 1'` on the node times out after 3 s | `kill -CONT`; states not `T`; `select 1` succeeds |
| F3b | Pause component container | `docker pause <svc>` (cgroup freezer) | `State.Paused = true` | `docker unpause`; `Paused = false` |
| F4a | Slow disk | write `delay_ms=200` to the slowfs control file of node X | median of 5 probe `write+fsync` of 4 KiB on the data mount ≥ 150 ms (P17) | write `delay_ms=0`; median < 50 ms |
| F5a | Add node n4 | `pg-n4`, `pooler-n4`, `nem-pg-n4` created from the Compose definitions when the run starts (`docker compose --profile n4 create`), then started by the runner with `docker start` in that order (ADR-0023) | the leader's `current_setting('synchronous_standby_names')` names n4's application name **and** n4 replicates (`pg_stat_replication` row on the leader) — deadline 120 s (not P27, membership is slower) | — (n4 stays until F5b) |
| F5b | Remove node n4 | `docker stop -t 30 pooler-n4` (graceful SHUTDOWN → tombstone, S24), then stop `pg-n4` | the leader's `synchronous_standby_names` no longer names n4 — deadline 120 s | — |
| F6 | Graceful switchover | `docker kill -s TERM pooler-<leader>` (graceful drain, S15) then `docker start` after the plan's duration | the decided rule's leader changes (rule number increases) within 60 s | container running, `/ready` = 200 |
| F7 | Network delay | `tc qdisc add dev eth0 root netem delay 50ms` in the target netns | TCP connect RTT from a peer helper to the target increases by ≥ 40 ms vs. pre-fault probe (P18) | `tc qdisc del`; RTT within 10 ms of baseline |

**Composite faults** are ordered sequences of the above in one plan step, each element confirmed before the next starts (e.g. F5a → F3a(n2,n3) → F2a(leader)) — except an element marked `confirm_after_heals`, which is injected in its place and whose unchanged predicate is evaluated, with its unchanged deadline, after the step's heals (ADR-0026: F5b during F1a(leader)).

**Excluded faults** (ADR-0005): clock skew (impossible per node on one host for static Go binaries), power loss / page-cache loss (containers share the host page cache), disk full, packet corruption, DNS faults. Each is listed in LIMITATIONS.

**slowfs** (ADR-0006): a CommitProof Go binary (`github.com/hanwen/go-fuse/v2` v2.12.0) mounted as a pass-through over `/data` inside `pg-nX` before pgctld starts, with a control file outside the mount. Container needs `--device /dev/fuse` and `CAP_SYS_ADMIN` (container-scoped). Pre-decided fallback, applied by ADR without owner approval because it does not relax any gate: if K3 shows that the baseline cluster with slowfs at 0 ms fails the HEALTHY predicate or the smoke checkers, F4a is replaced by **F4b**: the node container is started with `--device-write-bps <data device>:1mb` (static throttle for the whole run), confirmed by the same probe with threshold "median ≥ 150 ms for a 256 KiB write+fsync"; F4b runs are labelled `static-throttle` and LIMITATIONS records that slow-disk onset is not dynamic.

---

## 9. Mutant catalog

### 9.1 Rules

- A mutant is exactly one patch file `mutants/M<n>.patch` against the pinned commit, touching one function, plus a header comment stating intent, the real failure mode it models, and its source. Mutant builds use the same Dockerfiles (§5.2).
- After building a mutant, the sha256 of every binary **not** containing the patched package MUST equal the baseline lock (proves the build changed only what the patch touches).
- Each mutant has a **directed schedule** `D<n>`: a fixed plan designed so the mutant's defect, if present, produces a checker-detectable violation. Detection gate: violation of the expected class in ≥ 4 of 5 directed runs (P14). The **real** build runs the same directed schedule 5 times and its result is reported whatever it is.
- A mutant that cannot be detected by its directed schedule is not "removed": it is reported as UNDETECTED with its runs, the gate K4.3/K5.4 fails, and the owner decides (§18).

### 9.2 Catalog

| ID | Name | Patch (semantics) | Models | Source | Directed schedule | Expected detection |
|---|---|---|---|---|---|---|
| M1 | `sync-standby-cleared` | `postgresqlSyncStandbyManager.setStandbyNames` (sync_standby_manager.go:101) always writes `synchronous_standby_names = ''`: commits are acknowledged after local flush only ("ack before quorum") | durability not tied to quorum | S09, S25 | D1: steady W1+W4 load; at t=10 s apply F1c on the leader (gateways still reach it) for 30 s; heal; recovery; final reads | C1 lost-acked-writes (writes acked by the isolated old leader are rewound away) |
| M2 | `cohort-add-local-view` | in `UpdateConsensusRule` (rpc_manager.go:405), for `COHORT_ADD` only: apply the incoming synchronous-standby policy (new member counted for acks) and return success **without** writing the rule (`DoUpdateRule` skipped) | membership admitted from a local view, not by consensus | S41, S24 | D2′ (ADR-0027; identifier `D2p`): F5a (wait for n4 admission predicate) → F3a(base-follower:0), F3a(base-follower:1) held 40 s (W4/W1 load) → F2a(leader), F2b(same:3) (F2b on the node F2a killed) and F2b(n4) → heal both F3a → the old leader and n4 stay down 30 s → heals: F2b and F2a of the old leader, then F2b(n4) → recovery → final reads. Supersedes D2 (contract ≤ 1.0.6: the same without F2b(n4)), which multiorch defeats by recruiting every pooler of the shard and electing n4 (ADR-0027) | C1 (writes acked only by n4 are lost when a failover that cannot reach n4 recruits from the old cohort) |
| M3 | `gateway-retry-any` | `classifyError` (pooler_gateway.go:96) returns buffer/retry for every error on PRIMARY targets, including errors after execution began | unsafe retry → duplicate execution | S14 | D3′ (ADR-0033; identifier `D3p`; profile `buffered`): W1 autocommit-append variant and W4 at 2× pacing; three steps, the first at t = 10 s, each 15 s after the previous step's last heal is confirmed; each step F7(base-follower:0), F7(base-follower:1) → F2a(leader) as soon as the second F7 is confirmed (every commit waits ≥ 50 ms for its standby acknowledgement, its WAL already on both followers, when the leader dies) → heal both F7, then F2a (PostgreSQL restarted by its pooler). The buffer's timing guard buffers at most the first kill (ADR-0029). Supersedes D3 (contract ≤ 1.0.9: F2a on the leader three times, 15 s apart, ADR-0030), which reaches the defect only when a kill catches a 3–8 ms commit (ADR-0031) | C3 `duplicate-elements` (an autocommit append executed twice); C2 `g1a` (a ledger insert reported to the client as failed with `23505` on the retry while the first execution's row is present) — either class detects (ADR-0030) |
| M4 | `guard-removed` | delete the `synchronous_commit` entry from the restricted-GUC map (`go/common/pgsettings/restricted.go:46`) | durability guard bypass | S11 | probe P1 | P1 expects `0A000`, mutant returns success |
| M5 | `standby-slots-cleared` | `setSynchronizedStandbySlots` (pg_replication.go:600) always sets `synchronized_standby_slots = ''` | logical decoding runs ahead of the cohort | S13, S12 | D5: W7 consumer streaming; W4 load; F1b with A = {all followers' nodes}, B = everything else, for 20 s (commits block, become `info`); then F2a+F2b on the leader; heal: F1b first, the old leader's pooler 30 s later, then its PostgreSQL (ADR-0030); recovery | C7 phantom (consumer received changes that are absent from the final state) |

### 9.3 Rejected mutants (recorded so reviewers see the reasoning)

| Candidate | Reason rejected |
|---|---|
| `synchronous_commit = remote_write` | Equivalent here: data reaches the standby's page cache, which survives a container kill (shared host kernel). Only an OS crash loses it (S09). |
| Drop the majority term or the revocation term in `CheckSufficientRecruitment` | Equivalent for `AT_LEAST_2` with cohorts of 3 and 4 (I3; verified by §10.8). |
| `ComparePosition` ignores rule number | Needs a multi-failover divergence pattern with no directed schedule that reliably reaches it; detection would be probabilistic; kept as a candidate for future work in LIMITATIONS. |
| Revocation kept in memory only (`AcceptRevocation` not persisted) | Requires two concurrent coordinators plus a pooler restart in a precise window; no reliable directed schedule. |
| Revert gateway stale-primary retraction (PR #1353) | Detectable only after the default health-stream staleness window (~90 s, S28); detection is availability (hang) rather than safety; low value per run-minute. |

### 9.4 Historical control H1 (ADR-0015)

Build the `h1` profile from commit `4bca1d5770a6bd425bde5a61eaad240431242cd5` (issue #778, S51; images, topology and HEALTHY predicate per ADR-0025). Schedule DH1: W4+W1 load, F3a on the leader's PostgreSQL for 120 s (the duration used in S51; deliberately longer than P16), `kill -CONT`, recovery, final reads; 5 runs. Outcome is recorded as DETECTED (C1 violation in ≥ 1 run), NOT_DETECTED, or NOT_DEPLOYABLE (topology for that commit cannot reach HEALTHY after deriving its flags per §5.5). The gate requires the attempt and honest classification, not a particular outcome; LIMITATIONS and the README state the outcome.

---

## 10. Checkers

All checkers are pure functions of a sealed run (history + plan; never observations). Each returns `valid: true|false|unknown` and a list of anomalies `{class, ops[], explanation}`.

### 10.1 C1 lost-acked-writes

Inputs: W4 ledger ops, W1 appends, final reads. Violation `lost-write`: an `ok` insert/append absent from the stable final read through **every** gateway. Violation `final-read-divergence`: stable final reads differ between gateways. `info` writes may be present or absent.

### 10.2 C2 aborted-visible

Violation `g1a`: a value whose write completed `fail` is observed by any read (including final reads).

### 10.3 C3 list-append (Elle subset, reimplemented — ADR-0004)

- Version order per key inferred from the longest read; every read must be a prefix of it, else `incompatible-order`.
- Non-cycle anomalies: `duplicate-elements`, `incompatible-order`, `g1a`, `g1b` (read of an intermediate value), `internal` (a transaction's read inconsistent with its own prior appends), `dirty-update`.
- Dependency graph edges: `ww`, `wr`, `rw` from version order; optional `process` and `realtime` edges.
- Cycle classes (Adya/Elle names): `G0` (ww only), `G1c` (ww+wr), `G-single` (exactly one rw), `G-nonadjacent` (≥2 non-adjacent rw), `G2-item` (≥1 rw, otherwise). Realtime/process variants reported separately as `-realtime` / `-process` suffixes.
- Expected model per isolation level (G5): `serializable` forbids G0, G1a, G1b, G1c, G-single, G-nonadjacent, G2-item; `repeatable-read` (snapshot isolation) forbids G0, G1a, G1b, G1c, G-single and `G-nonadjacent`, and allows G2-item; realtime variants are reported as observations unless the guarantee row says otherwise.
- Cycle search per SCC bounded by P12 → `unknown`.

### 10.4 C4 linearizability (Porcupine v1.3.1, wrapped)

Model: register with read/write/cas, partitioned by key. `info` ops are included with open-ended return (Porcupine's convention for unknown completion). `Illegal` → violation `non-linearizable`; timeout (P11) → `unknown`.

### 10.5 C5 bank

Every `ok` `read-all` must sum to 500 with no negative balance (`bank-sum`, `bank-negative`); final read likewise.

### 10.6 C6 stale-read (diagnostic refinement of C4)

For a `non-linearizable` register key, classify the earliest offending read as `stale-read` if its value equals a value superseded by a write that completed before the read was invoked.

### 10.7 C7 CDC

Inputs: W7 stream events (each change carries table, key, commit LSN — the transaction's commit end LSN, the position the consumer confirms, §7 W7, ADR-0029 —, xid, and the consumer's confirmed-flush LSN at receipt), W4/W1 histories, final reads.
- `cdc-gap`: an `ok` committed change present in the final state never appears in the stream after the consumer has caught up to the final LSN.
- `cdc-phantom`: a streamed change whose row/value is absent from the final state.
- `cdc-duplicate`: a change delivered twice where the second delivery's commit LSN ≤ the consumer's confirmed flush at the time of the first delivery's acknowledgement (duplicates after the last confirmed flush are allowed: at-least-once).
- `cdc-order`: per-key changes delivered out of commit-LSN order within one connection.

### 10.8 Quorum-table cross-check (FlexPaxos)

`quorumxcheck` computes, independently of Multigres code, for every cohort size n ∈ [1,9] and `AT_LEAST_N` with N ∈ [1,n] and every recruited-set size r ∈ [0,n]: (a) **term uniqueness** holds iff any two recruited sets of size r intersect (r > n/2); (b) **no rogue quorum** holds iff the unrecruited set cannot contain N members (n − r < N); (c) the FlexPaxos condition that every recruited set intersects every durability quorum (r + N > n). It asserts (b) ⇔ (c), then compares "sufficient" = (a) ∧ (b) with upstream `consensus.CheckSufficientRecruitment` imported from the pinned commit (this is the only Multigres package imported outside observation code; it lives in the separate `quorumxcheck` binary so the independence rule of §5.3 still holds for `cpcheck`, `cpgate` and `cpreport`). Output: the full table, mismatches (must be 0), and the list of (n, N) where the majority and revocation terms coincide.

### 10.9 C8 session-state

- G9: after the failover window (first successful op after a recorded leader change), `SHOW` of each tracked GUC equals the value set before; `EXECUTE` of each named prepared statement succeeds and returns the expected row. Violations `session-setting-lost`, `prepared-statement-lost`.
- G10: notifications whose `NOTIFY` transaction committed `ok` **after** the listener's first successful op post-recovery must be delivered within 10 s (`notify-not-resumed`). Notifications in the reconnect window are counted (`notify-window-loss`, observation only). Recovery from a fault window is the first `ok` write through the listener's gateway invoked after the window's last nemesis event (P06's notion); a NOTIFY whose 10 s window overlaps a fault window, or that no listener session was live for, is in a reconnect window (ADR-0030).
- G12: during W8 storm, `cp_steady` ops: fraction `ok` ≥ 0.9 and no rule change attributable to the storm (no leader change during a storm window without a nemesis fault) — violations `fair-share-starvation`, `overload-failover`. From contract 1.2.0 on — runs whose plan carries 1.2.0 or later; recorded runs keep the literal rule and their verdicts — the fraction counts only the `cp_steady` operations invoked outside every fault window (first to last nemesis event of a fault id, as for G10); both fractions, literal and outside fault windows, are reported side by side; the operations invoked inside fault windows are the observation `overload-fault-overlap`; a storm window without starvation that has no such operation, or fewer than one per second of storm outside fault windows, is not judged (observation `fair-share-not-judged`, C8 `unknown`, never PASS) (ADR-0037, the owner's decision on ADR-0035).

### 10.10 C9 availability (observation + G8)

Computes unavailability windows per gateway (first failed/unknown op after a fault to first `ok` after heal: by completion times, from the fault's injection, the `ok` op completing at or after the heal's confirmation; final reads excluded — ADR-0036), latency percentiles per phase, error class counts. K6's per-fault metrics (phases, percentiles, rule changes) are defined by ADR-0036. G8 check: in C8 (profile `buffered`, F6), any `fail`/`info` on single autocommit statements within a judged switchover window is `planned-failover-error`; explicit transactions may fail only with `40001`. A switchover window (F6 inject to healed) is judged if its injection is at least 60 s (`--buffer-min-time-between-failovers`, S14) after `max(end, start + 20 s)` (`--buffer-max-failover-duration`) of every earlier fault window of the run; the same errors in the other F6 windows are the observation `planned-failover-unbuffered` (documented timing guard, ADR-0029).

### 10.12 P probe checker (G7)

Input: the probe suite's `wl: probe` operations (§15 K4; ADR-0024). P1 (`SET synchronous_commit = off`) must be rejected with `0A000`: completed `ok` → `p1-not-rejected`; rejected with another SQLSTATE → `p1-wrong-sqlstate`. P2 (`ALTER SYSTEM SET work_mem = '8MB'`) must be rejected (any SQLSTATE): completed `ok` → `p2-not-rejected`. A probe outcome without SQLSTATE (timeout, transport) or a planned probe that did not run → `unknown`. Runs without probe operations are vacuously valid. P follows C1–C9 in `checks.json`; the K2 corpus checkers are C1–C9.

### 10.11 Elle oracle (K2 only)

`cpcheck check --elle-oracle` exports W1 histories to elle-cli JSON and runs elle-cli 0.1.11 in the pinned `clojure` container (§14). Used to cross-validate C3 in K2 and to double-check every C3 finding in K7 (a C3 finding the oracle does not confirm is labelled `oracle-disagreement` and triaged before any finding package is written).

---

## 11. Findings taxonomy (closed)

### 11.1 Dimensions

| Dimension | Values |
|---|---|
| Class | `SAFETY_VIOLATION` (G1–G6, G11, G13 violations); `PROTOCOL_DEVIATION` (G7–G10, G12); `LIVENESS_OBSERVATION` (no recovery within P06, unavailability windows); `HARNESS_DEFECT` (CommitProof bug: fault not applied/healed, checker error, misclassified completion) |
| Claim level | `EXPLICIT`, `DESIGN`, `INFERRED` (from the guarantee row) |
| Origin hypothesis | `MULTIGRES`, `POSTGRES_INHERITED` (the behavior reproduces on plain PostgreSQL 17.7 streaming replication or is documented PostgreSQL behavior, e.g. S61, S62), `UNDETERMINED` |
| Reproducibility | `REPRODUCED` (minimized plan reproduces in ≥ 3/5 replays), `INTERMITTENT` (1–2/5), `NOT_REPRODUCED` (0/5; the original history is still the evidence) |
| Known status | `NEW`, `KNOWN_GAP` (matches an `alpha-known-gap` issue or a documented limitation such as S19; must cite it), `KNOWN_FIXED` (historical control) |

`HARNESS_DEFECT` is never reported as a Multigres finding; every one is fixed and the affected runs are re-run.

### 11.2 Severity (derived, not chosen)

`critical` = SAFETY_VIOLATION ∧ EXPLICIT; `high` = SAFETY_VIOLATION ∧ (DESIGN ∨ INFERRED) or PROTOCOL_DEVIATION ∧ DESIGN on durability (G7); `medium` = other PROTOCOL_DEVIATION; `info` = LIVENESS_OBSERVATION.

### 11.3 Finding identity

`finding id = FND-<campaign>-<sha256(class|guarantee|anomaly class|minimized plan hash)[:8]>`. The same anomaly from replays maps to the same id.

### 11.4 Evidence package (per finding, K7)

`findings/<id>/`: `README.md` in upstream issue format (S50): title `component: description`; Summary; Version/environment (pinned commit, profile, PostgreSQL 17.7, host); Reproduction (exact commands: build, cluster up, `commitproof replay`); Expected vs actual (cite the guarantee and its source); Analysis (design says / system did, timeline excerpt, LSN/rule evidence from observations); Suggested direction (optional); Limitations. Plus: minimized `plan.json`, the original and minimized run directories (sealed), replay results, `checks.json`, report excerpt, and `claim.yaml` (§17). Packages are written; nothing is published (AGENTS.md).

---

## 12. Verdict rules and statistics

### 12.1 Run verdict (by `cpcheck`)

- `INVALID`: seal mismatch; any fault `not-applied`/`not-healed`; cluster not HEALTHY at start; final reads not completed; runner crash. INVALID runs never count toward any matrix cell; they are re-run with the same seed (and if INVALID persists 3 times, it is a `HARNESS_DEFECT` to fix).
- `FAIL`: at least one checker `valid: false`.
- `UNKNOWN`: no `false`, at least one `unknown`.
- `PASS`: every checker `valid: true`.

### 12.2 Minimization (by `commitproof minimize`)

Delta debugging over the plan: first over nemesis steps (remove faults while the violation of the same anomaly class persists), then over workload length (halve duration, keep ≥ 10 s), then over client count (down to the clients involved in the anomaly + 1). Each candidate is evaluated by one run; a candidate "keeps" the failure if the same class appears. The final minimized plan is replayed P13 = 5 times; reproducibility per §11.1.

### 12.3 Campaign statistics (by `cpgate`/`cpreport`)

For a matrix cell with N valid runs and k FAIL runs of a given anomaly class (cells, exposure items and the integer arithmetic of the bounds: ADR-0036):
- report k/N and the Wilson 95% interval for the per-run violation probability;
- if k = 0: report the one-sided 95% upper bound `1 − 0.05^(1/N)` (for N = 10: 0.259) and the exact exposure: total operations, total `ok` writes, number of confirmed faults by kind, total fault-time;
- a clean statement MUST take the form: *"In N valid runs of campaign C (profile p, Multigres e3cbdbb), exposing W ok writes to F confirmed faults of kinds {…}, no violation of {checkers} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below X. Not tested: {list from LIMITATIONS}."*

### 12.4 Mutant detection

DETECTED iff ≥ 4/5 directed runs FAIL with the expected anomaly class (P14). The report shows mutant and real-build results side by side for the same directed schedule.

---

## 13. CI versus local evidence

GitHub Actions (public repo, `ubuntu-24.04`, 4 CPU/16 GB/14 GB SSD, 6 h/job, S88). Workflows call `make` targets only, so the same targets run locally. Remote CI runs are a **user action** (pushing is publication; AGENTS.md). Each sprint gate requires the CI-equivalent `make ci-*` targets to pass locally; K7 records a remote run link only if the owner has pushed.

| Target | Contents | Where |
|---|---|---|
| `make ci-unit` | `go vet`, `staticcheck`, unit tests, independence test (§5.3), parameters-table test (§5.7) | CI + local |
| `make ci-checkers` | K2 synthetic corpus, checker sabotage controls, Elle oracle differential (docker `clojure` image) | CI + local |
| `make ci-cluster-smoke` | build pinned images (buildx cache), cluster up/verify/down, 45 s smoke run, `cpcheck` | CI + local |
| `make ci-nemesis-smoke` | one 45 s run per fault kind F1a, F2a, F3a, F7 (F4a requires `/dev/fuse`, included if present on the runner, else the job records SKIPPED-WITH-REASON and LIMITATIONS says so) | CI + local |
| `make ci-mutant-smoke` | build M1, run D1 once, expect C1 violation | CI + local |
| `make ci-report` | regenerate the K6 reference report from the committed reference run and compare bytes | CI + local |
| `make campaign-k4` / `campaign-k5` / `campaign-k7` | full matrices | **local only**, evidence recorded |

Local evidence policy: every run cited by a claim is committed sealed under `evidence/` (compressed with zstd, still sealed by hash of the uncompressed files); total ≤ P25. K5 commits in full only the runs a gate criterion reads in full (C10 primary runs for K5.3; the primary and minimized runs of every real-build failure), the others as stubs (ADR-0030). K7 (ADR-0039, the owner's decision): a run that a claim, a finding package or a K7 report names as its artifact is cited and committed in full, once; every other run — one that only contributes its verdict and classes to a cell's statistics — is committed as a summary (its stub: manifest, plan, seal; its `checks.json` beside it) and listed with its seal root in `evidence/K7/runs-index.json`, its full archive kept on the machine that ran it, and LIMITATIONS says such runs are not independently re-checkable without re-running. P25 counts every byte under `evidence/` and every run archive under `findings/`; the text files of `findings/` do not count.

---

## 14. Toolchain and pins

Versions observed 2026-10-09 (RESEARCH §5). K1 re-resolves every image digest with `docker buildx imagetools inspect`, writes the resolved values into `images.lock.json`, and if any differs from this table records the difference in an ADR (research staleness, §18) — not BLOCKED.

| Item | Pin | Source |
|---|---|---|
| Multigres source | `e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73` | S01 |
| Historical control source | `4bca1d5770a6bd425bde5a61eaad240431242cd5` | S33, S51 |
| Historical control builder image | `golang:1.25-alpine3.23@sha256:27b3069b25647beacd3745b0b2c83e2f9b8d3935455a32cd4531236ee1c960e0` (go1.25.14; the Alpine release whose packages the historical Dockerfiles pin; resolved in K4, ADR-0025) | S33 |
| Go (CommitProof and Multigres builds) | `go1.26.9` via `GOTOOLCHAIN=go1.26.9` (host `go` ≥ 1.21 downloads it); `go.mod`: `go 1.26.9` with no `toolchain` line (the go command rejects a `toolchain` line equal to the `go` line; ADR-0019) | S80, S02 |
| Builder image | `golang:1.26.9-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade` (re-resolved in K1; the 2026-10-09 digest `cdfd4fe2…` is superseded, ADR-0020) | S81 |
| Multigres runtime base | `debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587` (re-resolved in K1; the 2026-10-09 digest `abd67ffc…` is superseded, ADR-0020) | S83 |
| PostgreSQL base | `postgres:17.7@sha256:2006493727bd5277eece187319af1ef82b4cf82cf4fc1ed00da0775b646ac2a4` | S82 |
| etcd | `quay.io/coreos/etcd:v3.6.4@sha256:885a73b09cde0293c84adad3d589cc5d091f03e61d390b94413c0a866ccfe232` (resolved in K1, ADR-0020) | S01, S86 |
| Nemesis/helper base | `alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6` | S84 |
| Elle oracle runtime | `clojure:temurin-21-lein-2.13.0-bookworm@sha256:724a7f5840394bf9e659730e5039bf7498f31185e747e3586dbab626b717c34f` | S85 |
| elle-cli | commit `9c2a18b3719f573dab06ae05c955c9c9d5225673` (0.1.11; Elle 0.2.7) built inside the oracle image | S73 |
| Porcupine | `github.com/anishathalye/porcupine v1.3.1` | S74 |
| pgx | `github.com/jackc/pgx/v5 v5.11.0` | S89 |
| pglogrepl | `github.com/jackc/pglogrepl` at commit `662581bb6bb6de4da82805879643f1dc39e41fad` (pseudo-version resolved by `go get`) | S89 |
| go-fuse | `github.com/hanwen/go-fuse/v2 v2.12.0` | S89 |
| Multigres Go packages (observations, quorumxcheck) | `github.com/multigres/multigres` at the pinned commit (pseudo-version) | S01 |
| Host | Arch Linux; `docker 1:29.9.0-1`, `docker-compose 5.6.0-1`, `docker-buildx 0.38.0-1` or newer | S87 |
| CI runner | `ubuntu-24.04` | S88 |
| Lint | `staticcheck` `honnef.co/go/tools v0.8.1` pinned in `tools/go.mod` (latest release at K1 time) | — |
| Workflow lint | `rhysd/actionlint:1.7.12@sha256:b1934ee5f1c509618f2508e6eb47ee0d3520686341fec936f3b79331f9315667` (latest release, resolved in K1, ADR-0020) | — |

---

## 15. Sprint specifications

Every sprint follows the same protocol: read AGENTS.md and the cited contract sections; implement; run the gate with `cpgate K<n> --evidence evidence/K<n> --negative-controls`; write `evidence/K<n>/NOTES.md` (design says / system did; demonstrated / not demonstrated); write the closure report (§19); commit. Gate verdict = `cpgate` exit 0 **and** every negative control behaving as specified.

**Negative controls (all sprints).** For each gate criterion K<n>.<m>, `cpgate --negative-controls` copies the evidence to a temporary directory, applies a deterministic sabotage specific to that criterion, re-evaluates, and requires that criterion — and **only** that criterion — to FAIL. Results go to `evidence/K<n>/negative-controls.json`. A criterion whose sabotage does not fail it, or fails another criterion, fails the gate. Checker-level sabotage (K2, K5, K6) uses Go build tags `cp_sabotage_<rule>` that compile a deliberately broken rule; product builds never include these tags (a test asserts the release binaries contain no sabotage symbols).

### K1 — Reproducible cluster and history recording

**Objective.** A Go repository that builds Multigres images from the pinned commit reproducibly, brings the §5.1 topology up and down deterministically, generates seed-deterministic plans, and records sealed, hash-chained histories from real workload clients.

**Deliverables.** Repository foundation (K0 documents committed first, unchanged, as their own commit); Go module; `build/` derived Dockerfiles; `images.lock.json`; `deploy/` Compose files for `baseline` and `DERIVATION.md`; `commitproof` (images, cluster, plan, run with workloads W1–W4 and no nemesis), `cpcheck verify-seal`; history and observation writers; parameters constants + table test; independence test; Makefile `ci-unit`, `ci-cluster-smoke`; `.github/workflows/ci.yml`; `evidence/K1/`.

**Interfaces fixed.** §5.4 run directory, §5.6 CLI (subcommands implemented in K1: `images build|verify`, `cluster up|down|verify`, `plan`, `run` without nemesis, `verify-seal`), §6.1 event schema, §6.2 classification, §6.3 plan.

**Gate K1** (`cpgate K1`):

| Criterion | Command(s) recorded in evidence | Pass condition |
|---|---|---|
| K1.1 Lock | `commitproof images build` | `images.lock.json` contains every §5.2 field; base digests resolved |
| K1.2 Binary reproducibility | `commitproof images build --no-cache` (second build, cache pruned with `docker builder prune -af` first) | sha256 of all six binaries (`multigres`, `multiadmin`, `multigateway`, `multiorch`, `multipooler`, `pgctld`) identical between the two builds |
| K1.3 Derivation | `make check-dockerfile-derivation` | diff between upstream Dockerfiles at the pinned commit and `build/` contains only `FROM` lines |
| K1.4 Up/down from zero | 3× `commitproof cluster up --profile baseline && commitproof cluster verify && commitproof cluster down` | each `up` reaches HEALTHY (§5.1) within P09; `multigres.version()` reports `e3cbdbb`; after each `down`, zero containers, volumes and networks with label `com.docker.compose.project=commitproof` |
| K1.5 Memory | smoke run with sampling of `docker stats --no-stream` every 1 s | peak sum ≤ P10 |
| K1.6 Plan determinism | `commitproof plan --campaign smoke --seed 42` ×3; `go clean -cache` and rebuild; ×1; seed 43 ×1 | four identical sha256 for seed 42; seed 43 differs |
| K1.7 History integrity | `commitproof run --campaign smoke --seed 42` (P04 smoke = 45 s); `cpcheck verify-seal` | seal valid; hash chains valid; `t` non-decreasing; every `invoke` has exactly one completion (open ops closed as `info` at end); every `ok` count > 0 for each of W1–W4 |
| K1.8 CI-equivalent | `make ci-unit ci-cluster-smoke` | exit 0 |

Negative controls: K1.1 remove a field from the lock copy; K1.2 alter one binary hash in the second-build record; K1.3 add a `RUN` line to a derived Dockerfile copy; K1.4 add a leftover volume to one teardown record; K1.5 set a recorded sample above P10; K1.6 alter one plan hash; K1.7 flip one byte in a copied `history.jsonl`; K1.8 replace the recorded exit code with 1.

**Rejection conditions.** Any image pulled by tag without digest; any `latest`; Compose `restart:` policies other than `"no"`; a smoke run that silently skips a workload; HEALTHY reached by retrying `cluster up` inside the measured attempt.

**Do not change.** K0 documents except through §18; the pinned commit; P-table values.

**DONE.** Gate PASS, negative controls behave, closure report written, commits made.

### K2 — Checkers validated before touching Multigres

**Objective.** C1–C7 and C9's G8 logic implemented and validated on synthetic histories with exact expected classifications, cross-validated against Elle for C3.

**Deliverables.** `cpcheck check`, `cpcheck corpus`; synthetic history generator (an in-memory reference database executing transactions serially under random interleavings, which is serializable by construction, plus a fault-planting layer); corpus under `testdata/corpus/` with `expected.json` per history; Elle oracle container and exporter; sabotage build tags; Makefile `ci-checkers`; `evidence/K2/`.

**Interfaces fixed.** `checks.json` schema: `{run_id, seal_root, checkers: [{id, valid, anomalies:[{class, ops, explanation}], stats}], verdict}`; anomaly class names exactly as in §10.

**Gate K2:**

| Criterion | Pass condition |
|---|---|
| K2.1 Planted anomalies | for every anomaly class in §10.1–§10.7 and §10.9–§10.10: ≥ 3 planted histories; `cpcheck` reports exactly the expected set of classes for each (set equality) |
| K2.2 Clean histories | ≥ 50 clean histories per workload (W1–W4, W5–W7 synthetic forms) are `valid: true` for all checkers |
| K2.3 Unknown is not pass | a pathological register history that exceeds P11 yields `unknown`, verdict `UNKNOWN`, exit code 2 |
| K2.4 Elle agreement | ≥ 300 W1 histories (≥ 100 clean, ≥ 200 with planted anomalies across classes, both isolation models): C3 validity and the mapped anomaly-class set agree with elle-cli on every history where elle-cli does not return `unknown`; disagreements: 0, or each recorded in `evidence/K2/divergences.json` with a minimal history, the Adya/Elle definition that decides it, and which tool is wrong; any disagreement where CommitProof is wrong is fixed before the gate |
| K2.5 Classification table | a test per row of §6.2 drives a fake PostgreSQL wire server (or pgx-level error injection) and asserts the classification |
| K2.6 Checker sabotage | for every rule tagged `cp_sabotage_<rule>` (at least: rw-edge inference, prefix check, duplicate detection, lost-write set difference, g1a detection, Porcupine model CAS, bank sum, CDC phantom, CDC duplicate bound), the corpus run fails exactly the classes that depend on that rule and no others |
| K2.7 CI-equivalent | `make ci-unit ci-checkers` exit 0 |

Negative controls: standard evidence sabotage per criterion (K2.6 is itself a checker-level negative control).

**Rejection conditions.** Expected outputs generated by running the checker under test; any test that asserts only "some anomaly"; Elle disagreement resolved by editing the oracle's input.

**Do not change.** §6 schema, §10 class names, K1 interfaces.

**DONE.** Gate PASS; NOTES lists every checker's demonstrated coverage and known blind spots.

### K3 — Nemesis

**Objective.** Every fault of §8 injected, confirmed, healed and heal-confirmed against the real baseline cluster, fail-closed.

**Deliverables.** Nemesis helper image; slowfs (or F4b per fallback); plan support for faults and composites; `commitproof run` with nemesis; confirmation/heal predicates; `make ci-nemesis-smoke`; `evidence/K3/`.

**Gate K3:**

| Criterion | Pass condition |
|---|---|
| K3.1 Each fault confirmed | for each of F1a, F1b, F1c, F1d, F2a–F2e, F3a, F3b, F4a (or F4b), F5a, F5b, F6, F7: one run applying it 3 times (F5a/F5b once each; F6 twice) → every injection `confirmed`, every heal `healed`, run not INVALID |
| K3.2 Fail-closed | for each fault kind, a run with the injector sabotaged to a no-op (build tag `cp_sabotage_inject_<kind>`) ends INVALID with `not-applied` for that fault, exit code 2 |
| K3.3 Heal fail-closed | for F1a and F3a, heal sabotaged to no-op → INVALID with `not-healed` |
| K3.4 Observability of confirmation | each confirmation event's `evidence` field contains the raw probe outputs (hashes of stdout/stderr in `commands.jsonl` match) |
| K3.5 Checkers on real runs | `cpcheck` runs on all K3.1 runs; verdicts recorded (any value) |
| K3.6 CI-equivalent | `make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke` exit 0 |

**Rejection conditions.** A confirmation predicate that only checks the injection command's exit code; any `--privileged` container; any host-level `sudo` in the run path.

**Do not change.** §8 predicates' meaning (thresholds are P-values); K1–K2 interfaces.

**DONE.** Gate PASS; NOTES records each fault's mechanism and the observed system reaction (design says / system did).

### K4 — Consensus and failover campaigns, minimization, mutants

**Objective.** Run campaigns C1–C7 against real Multigres, minimize and replay failures, detect mutants M1, M2, M4, run probes and the historical control, and run the quorum-table cross-check.

**Campaign definitions:**

| Campaign | Workloads | Faults (nemesis pool, seeded) |
|---|---|---|
| C1 failover-kill | W1, W3, W4 | F2a, F2b on leader or random node; F2c, F2d random |
| C2 partition | W1, W2, W4 | F1a (leader/follower), F1b, F1c, F1d |
| C3 pause | W1 (both isolation levels across runs), W2, W4 | F3a short/long on leader and follower; F3b on pooler/orch/gateway |
| C4 membership | W1, W4 | F5a/F5b sequences; composite F5a→F2a(leader); composite F5a→F3a(followers)→F2a(leader) (the original D2's shape); F5b during F1a(leader) (F5b confirmed after F1a heals, ADR-0026) |
| C5 slow-disk | W1, W4 | F4a on leader or follower, combined with F2a |
| C6 rewind | W1, W4 | F2a(leader) then, 3–8 s after the new leader is observed, F2a(new leader); repeated twice per run |
| C7 commit-cancel | W4 with session `statement_timeout` P19, W1 | F1b with A = {all followers' nodes}, B = everything else (commits block) for 15 s, then F2a on the leader; also client `CancelRequest` 1 s after `COMMIT` is sent |

Fault counts, holds, role names and the directed schedules' seeds and heal timing are fixed by ADR-0024 (before any K4 run).

**Deliverables.** Campaign driver; `minimize`, `replay`; mutant patches M1, M2, M4 and directed schedules D1, D2′ (ADR-0027); probe suite P1 (`SET synchronous_commit = off` → expect `0A000`) and P2 (`ALTER SYSTEM SET work_mem = '8MB'` → expect rejection); `h1` profile and DH1; `quorumxcheck`; `evidence/K4/`.

**Gate K4:**

| Criterion | Pass condition |
|---|---|
| K4.1 Quorum table | 0 mismatches; table and coincidence list recorded |
| K4.2 Campaign runs | each of C1–C7: ≥ 3 valid runs on `baseline`, seeds 1–3; every run checked; results recorded verbatim in `evidence/K4/results.json` whatever they are |
| K4.3 Mutant detection | M1 via D1 and M2 via D2′: DETECTED (≥ 4/5) with the expected class; M4 via P1: probe fails on M4 in 5/5 and passes on real in 5/5; real build results for D1/D2′ (5 runs each) recorded; the superseded D2's runs (contract 1.0.6) stay recorded (ADR-0027) |
| K4.4 Mutant isolation | for each mutant image, binaries not containing the patched package have baseline hashes |
| K4.5 Minimize and replay | every FAIL run of K4 (mutant or real) has a minimized plan and 5 replays with reproducibility classified (§11.1) |
| K4.6 Historical control | H1 attempted with 5 runs of DH1 (or NOT_DEPLOYABLE with evidence of the derivation attempt); outcome classified |
| K4.7 Findings triage | every FAIL on the real build has a triage record: checker output, oracle cross-check (C3), classification per §11, and a check that it is not a HARNESS_DEFECT (completion classification reviewed against §6.2) |
| K4.8 CI-equivalent | `make ci-*` (all CI targets including `ci-mutant-smoke`) exit 0 |

**Rejection conditions.** Changing a directed schedule after seeing mutant results without an ADR that preserves detection difficulty; dropping a failing real-build run; classifying a real-build FAIL as harness defect without the §6.2 review evidence.

**Do not change.** Checker logic (bug fixes allowed only with a new K2 corpus case proving the bug and re-running K2's gate); fault predicates.

**DONE.** Gate PASS; NOTES states what K4 demonstrated about each of G1–G7 and what it did not.

### K5 — Pooler, gateway and CDC under faults

**Objective.** Campaigns C8–C11, mutants M3 and M5, with checkers C7–C9 validated synthetically first.

**Campaign definitions:**

| Campaign | Profile | Workloads | Faults |
|---|---|---|---|
| C8 switchover | `buffered` | W4 autocommit, W1 explicit txns | F6 ×3 per run |
| C9 pooler-state | `baseline` | W5, W6, W4 | F2a(leader), F6, F2b(leader) |
| C10 cdc | `baseline` | W7, W4, W1 | F2a(leader), F2b(leader), F1c, F6 |
| C11 overload | `baseline` | W8 | storm phases of 20 s; one run with no nemesis, one with F2a during a storm |

Clients, fault counts, holds and order per run, the directed schedules' timing, the C8 completions, K5.3's observation and the storage of K5 runs are fixed by ADR-0030 (before any K5 run).

**Deliverables.** W5–W8; C7–C9 checker completions; synthetic corpus additions with planted session/notify/CDC/availability anomalies; M3, M5 patches and D3′ (ADR-0033), D5; `evidence/K5/`.

**Gate K5:**

| Criterion | Pass condition |
|---|---|
| K5.1 Synthetic validation | planted `session-setting-lost`, `prepared-statement-lost`, `notify-not-resumed`, `fair-share-starvation`, `overload-failover`, `planned-failover-error`, `cdc-gap`, `cdc-phantom`, `cdc-duplicate`, `cdc-order`: exact classification; clean histories valid; sabotage controls per rule |
| K5.2 Campaign runs | C8, C9, C11: ≥ 3 valid runs each; C10: ≥ 5 valid runs; results recorded whatever they are |
| K5.3 CDC slot failover precondition | in every C10 run, the slot is observed `failover = true` and synced on standbys (pg_replication_slots on followers, observation) before the first fault; otherwise the run is INVALID. "Synced" means failover-ready: `synced` and not `temporary` and not invalidated (S12; ADR-0030) |
| K5.4 Mutant detection | M3 via D3′ (`buffered`) and M5 via D5: DETECTED (≥ 4/5) with the expected class; real-build results (5 runs each) recorded; the superseded D3's runs (contract 1.0.9) stay recorded (ADR-0033) |
| K5.5 Minimize/replay/triage | as K4.5 and K4.7 for every K5 FAIL |
| K5.6 CI-equivalent | all `make ci-*` exit 0 |

**Rejection conditions.** Counting reconnect-window notification loss as a violation (it is a documented limitation, S19); treating `40001` on explicit transactions in C8 as a violation; enabling buffering in any campaign other than C8/D3′ (and the superseded D3).

**DONE.** Gate PASS; NOTES per guarantee G8–G12.

### K6 — Observability and reproducible report

**Objective.** A static, self-contained report that reconstructs an incident from a sealed run: timeline of operations, faults (inject/confirmed/heal/healed), leader/rule changes and metrics; latency and availability during failovers; checker results; regenerated byte-for-byte.

**Deliverables.** `cpreport render`; single-file HTML with inline SVG (no external fetches, no JavaScript dependencies from CDNs; any script inline and deterministic); per-campaign summary page; reference run committed under `evidence/K6/reference-run/`; `make ci-report`; `evidence/K6/`.

**Determinism rules.** No generation timestamps, hostnames or absolute paths in output; maps iterated in sorted order; floating-point values rendered from integer arithmetic with fixed decimal places; SVG coordinates integers.

The reference run, the report's inputs and fail-closed behaviour, the event elements K6.3 counts, observed rules and leader changes, and the per-fault metrics K6.5 cross-checks are fixed by ADR-0036 (before any K6 render). Metric scrapes (P23) are drawn when a run holds them, and K6.5's cross-check covers their summary; the recorded families are fixed by ADR-0038. No real K4 run holds both F2a and F1c; the reference is K4's C2 seed 3 (three F1c, one failover).

**Gate K6:**

| Criterion | Pass condition |
|---|---|
| K6.1 Byte reproducibility | three renders of the reference run → identical sha256 |
| K6.2 Forced rebuild | `git clone` of the repo at HEAD into a temp dir, `go clean -cache -modcache`, build `cpreport`, render → same sha256 as K6.1 |
| K6.3 Completeness | the report contains every history event (count check), every fault phase, every leader change in observations, every checker anomaly with links to the ops involved |
| K6.4 Seal enforcement | rendering a run whose seal does not verify exits 2 and writes nothing |
| K6.5 Failover metrics | per fault: unavailability window per gateway, p50/p95/p99 latency before/during/after, rule changes count (cascade indicator); values cross-checked against an independent computation in `cpgate` from the raw history |
| K6.6 CI-equivalent | all `make ci-*` exit 0 |

Checker-level sabotage: build tag `cp_sabotage_report_nondeterminism` (inserts map iteration without sorting) must fail K6.1 only.

**DONE.** Gate PASS; the report of one real K4 failover is committed as an example.

### K7 — Full campaign, statistics and release

**Objective.** Execute the final matrix, compute statistics, produce findings packages, correct overclaims, finalize documentation, tag v1.0.0 locally.

**Matrix (P15):**

| Cell | Campaigns | Profile | Runs |
|---|---|---|---|
| V1 baseline | C1, C2, C3, C4, C5, C6, C7, C9, C10, C12 | `baseline` | 10 each |
| V2 slotless | C1, C2, C4 | `slotless` | 5 each |
| V3 buffered | C8 | `buffered` | 10 |
| V5 overload | C11 (both G12 fractions reported per cell, ADR-0037) | `baseline` | 5 |
| Mutants | D1, D2′, D3′, D5 and P1 for M4, each with the real build on the same schedule (§9.1, §12.4) | as defined | cited from K4/K5, not re-run: 5 each, valid, with the contract versions their plans carry (ADR-0039) |
| H1 | DH1 | `h1` | cited from K4: 5 runs (ADR-0039) |

K7 runs the cells V1, V2, V3 and V5 (130 runs; V4 and its campaign C13 were removed by the owner, ADR-0040); the measured wall time per run in K4 and K5 is about 2.3 minutes (cluster up, 90 s workload, recovery, final reads, teardown; `evidence/pre-K7/k7-feasibility.md`), plus re-runs of INVALID runs and the minimization and replays of failures; the campaign driver is resumable.

C12 mixed: all workloads W1–W4, nemesis drawing uniformly from {F1a–F1d, F2a–F2e, F3a, F3b, F4a, F6, F7}, plus one F5a/F5b pair per run. Its steps, targets, holds, seeds and clients, profile `slotless`, and `make campaign-k7` (resumable, background-safe, with a dry run) are fixed by ADR-0041 (before any run of them).

**Deliverables.** `make campaign-k7` (resumable, background-safe); statistics per §12.3; `findings/` packages for every finding (§11.4); `claims.yaml` (§17) covering every claim in README, LIMITATIONS and NOTES; public `README.md` (what it is, how to reproduce, results table with statistics, the §4 matrix, limitations summary, how to read findings); final `docs/LIMITATIONS.md`; `CHANGELOG.md`; `evidence/K7/`; annotated local tag `v1.0.0`.

**Gate K7:**

| Criterion | Pass condition |
|---|---|
| K7.1 Matrix complete | every cell has the required number of valid runs; INVALID runs listed with causes; none counted |
| K7.2 Statistics | every cell has k/N, Wilson interval or zero-failure bound, exposure counts, recomputed by `cpgate` from `checks.json` files and matching `cpreport` output |
| K7.3 Oracle on findings | every C3 anomaly in the matrix confirmed by elle-cli or labelled `oracle-disagreement` with triage |
| K7.4 Findings packages | every finding has a complete §11.4 package; replay commands in each package executed once more and the result recorded |
| K7.5 Claims audit | every claim in README/LIMITATIONS/NOTES of K7 appears in `claims.yaml` with all §17 fields; every referenced artifact hash matches; claims lacking a field are worded as hypotheses (checked by `cpgate` against a list of hypothesis markers) |
| K7.6 Overclaim correction | `evidence/K7/overclaim-review.md` lists each statement changed from earlier sprints' NOTES/README drafts and why; README contains the §4 matrix verbatim and the §12.3 clean-result template where applicable |
| K7.7 Release | annotated tag `v1.0.0` exists locally on a commit where all `make ci-*` targets exit 0; nothing pushed |

**DONE.** Gate PASS; closure report lists pending user actions (push, enable Actions, publish findings or not).

---

## 16. Final acceptance matrix (GATE / EVIDENCE)

| Gate | Evidence location | Verified by |
|---|---|---|
| K1.1–K1.8 | `evidence/K1/` (`gate.json`, `negative-controls.json`, lock, rebuild hashes, up/down records, smoke run) | `cpgate K1` |
| K2.1–K2.7 | `evidence/K2/` (corpus results, Elle comparison, divergences, sabotage matrix) | `cpgate K2` |
| K3.1–K3.6 | `evidence/K3/` (per-fault runs, fail-closed runs) | `cpgate K3` |
| K4.1–K4.8 | `evidence/K4/` (quorum table, campaign results, mutant runs, minimized plans, replays, H1, triage) | `cpgate K4` |
| K5.1–K5.6 | `evidence/K5/` | `cpgate K5` |
| K6.1–K6.6 | `evidence/K6/` (reference run, render hashes, fresh-clone record) | `cpgate K6` |
| K7.1–K7.7 | `evidence/K7/`, `findings/`, `claims.yaml`, README | `cpgate K7` |
| Bullet matrix §4 | each COVERED/PARTIAL row → the gates listed in its row | `cpgate K7` claims audit |

---

## 17. Evidence rule

Every claim made anywhere in the repository (README, NOTES, LIMITATIONS, findings, closure reports) is either a **supported claim** or a **hypothesis**. A supported claim has all of:

`claim text → contract section → target (system, commit, profile, mutant) → command → immutable artifact (sealed run dir or evidence file) → sha256 → result → known limitations`

These are recorded in `claims.yaml`:

```yaml
- id: CLM-0001
  text: "..."
  contract: "§3 G1"
  target: {system: multigres, commit: e3cbdbb..., profile: baseline, mutant: none}
  command: "commitproof run --campaign C1 --seed 1 --out runs/"
  artifact: "evidence/K4/runs/<run-id>/"
  sha256: "<seal root>"
  result: "PASS (C1 valid, ...)"
  limitations: ["LIM-03", "LIM-07"]
```

If any field is missing, the statement must be phrased as a hypothesis ("we expect", "not verified") and is listed under `hypotheses:` in `claims.yaml`. Notes always separate **design says** (with source ID) from **system did** (with artifact).

---

## 18. BLOCKED protocol

If Claude Code is blocked by an error **in this contract, an ADR or the research** — a contradiction between sections, a default value read wrongly, an outdated source, a flag that does not exist at the pinned commit, a digest that changed — it resolves it itself:

1. writes a new ADR (`docs/adr/NNNN-<slug>.md`) stating the defect, the evidence (command + output), the correction and the alternatives considered;
2. bumps the contract version (PATCH for corrections, MINOR for new requirements) and updates the affected sections and the ADR index;
3. commits the fix separately (`docs(contract): <summary> (ADR-NNNN)`);
4. continues the sprint.

It never resolves by itself, and instead stops with **BLOCKED** and evidence:

- prerequisites missing on the owner's machine (packages, permissions, kernel modules, disk, memory);
- irreversible actions (deleting data outside the repo and its own Docker resources, rewriting published history, force-pushing);
- publishing anything outside the repository (push, issues, PRs, comments, releases, uploads);
- any change that relaxes a gate, a criterion, a threshold, a test or a negative control — including lowering repetitions, widening tolerances, removing a fault from a campaign, reclassifying a failure as expected, or skipping a criterion.

A BLOCKED stop prints the closure report (§19) with `status: BLOCKED`, the exact blocking condition, the command and output that show it, what was completed, and the single action needed from the owner.

---

## 19. Closure report format

Printed at the end of every sprint and saved to `evidence/K<n>/closure.yaml`:

```yaml
sprint: K<n>
status: DONE | BLOCKED
contract_version: "1.x.y"
commits:
  - {sha: "...", message: "..."}
gate:
  command: "cpgate K<n> --evidence evidence/K<n> --negative-controls"
  exit_code: 0
  criteria:
    - {id: "K<n>.1", result: PASS, evidence: "evidence/K<n>/...", sha256: "..."}
  negative_controls:
    - {criterion: "K<n>.1", sabotage: "...", failed_only_target: true}
demonstrated:
  - "..."            # each line backed by a claims.yaml id
not_demonstrated:
  - "..."
findings:            # K3+ only
  - {id: FND-..., class: ..., guarantee: G?, claim_level: ..., reproducibility: ..., severity: ...}
adrs_added: ["ADR-NNNN: ..."]
deviations_from_prompt: []   # anything done differently from the prompt, with reason
blocked:             # only when status: BLOCKED
  condition: "..."
  evidence: {command: "...", output_excerpt: "..."}
  owner_action: "..."
pending_user_actions: []
next_sprint_ready: true | false
```

---

## 20. Risk register

| ID | Risk | Detection / mitigation | Residual (LIMITATIONS) |
|---|---|---|---|
| R1 | Multigres `main` moves daily; results age | pinned commit; every claim names it | LIM-01 |
| R2 | Topology differs from production (operator on Kubernetes, three AZs) | flags derived from upstream demo (DERIVATION.md); three cells, three orchestrators like S42 | LIM-02 |
| R3 | slowfs disturbs PostgreSQL | K3 smoke with 0 ms; pre-decided F4b fallback | LIM-05 if fallback used |
| R4 | iptables backend or `sch_netem` unavailable | `cluster up` prerequisite checks (exit 3); PREREQUISITES; K3.1 | — |
| R5 | Flaky results inflate or hide findings | §12 reproducibility classes; INVALID never counted; replay ×5 | — |
| R6 | Client misclassifies ambiguous outcomes → false loss reports | closed §6.2 table, K2.5 tests, K4.7 triage before packaging | — |
| R7 | Checker bug | K2 corpus, sabotage controls, Elle oracle on every C3 finding | — |
| R8 | "No violations" read as "correct" | §12.3 mandatory wording with bound and exposure; LIMITATIONS | LIM-* |
| R9 | Mutants are strawmen or equivalent | mutants map to documented failure modes (S09, S41, S11, S14, S13); equivalence analysis §9.3; mutant isolation K4.4; real build run on the same directed schedules | — |
| R10 | Overclaiming bullet coverage | §4 matrix with NOT COVERED/PARTIAL; K7.5 claims audit; K7.6 overclaim review | — |
| R11 | Upstream already does this internally (Multiquake, S29) | positioning D4: public, history-checked, mutation-validated; never "first" | LIM-10 |
| R12 | Resource exhaustion on 16 GB | P10 gate; one cluster at a time; K7 resumable | — |
| R13 | Runtime of K7 exceeds a session | resumable `campaign`, background-safe, idempotent skip of sealed runs | — |
| R14 | Docker socket in runner is root-equivalent | documented in PREREQUISITES; runner image built locally only | LIM-11 |
| R15 | Findings in C7 are PostgreSQL behavior, not Multigres | origin dimension `POSTGRES_INHERITED`; reproduce on plain PostgreSQL streaming replication when claimed | — |
| R16 | Licensing contamination from Elle | no Elle code copied; elle-cli only executed in a container, never vendored (ADR-0004) | — |
| R17 | Publication by mistake | AGENTS.md hard rule; BLOCKED protocol | — |
| R18 | Single etcd is a SPOF not present in production | etcd faults only F2e; LIMITATIONS | LIM-12 |
| R19 | A finding reported by CommitProof is already known | taxonomy `KNOWN_GAP`/`KNOWN_FIXED`; search of S50 before packaging | — |
| R20 | Wall-clock dependence in heartbeat lag (S32) untested | stated as untested sensitivity | LIM-04 |
