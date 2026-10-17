# CommitProof

> **Draft.** Sprint K7 stopped BLOCKED at its store step (P25; `evidence/K7/closure.yaml`). This README is unfinished: its results sections are placeholders, and `claims.yaml` does not exist yet.

CommitProof searches for counterexamples to the durability and consistency guarantees that Multigres states publicly — an acknowledged commit survives failovers, split brain and membership changes — and reports bounded evidence, not proofs: every result is a count of valid runs of a stated fault schedule on one pinned commit (Multigres `e3cbdbb`), with a 95 % bound and the exact exposure, and every violation it found comes with a sealed, replayable history. It is an independent, Jepsen-style verifier written in Go: a reproducible multi-container cluster, seeded fault schedules with confirmed faults, history checkers validated on synthetic histories and against elle-cli, and deliberately broken builds (mutants) that show the checkers catch what they claim to catch. The normative specification is [`docs/CONTRACT.md`](docs/CONTRACT.md) (version 1.8.1); every number below comes from the evidence under [`evidence/`](evidence/) and is listed with its artifact and hash in [`claims.yaml`](claims.yaml).

<!-- README-FINDINGS -->

<!-- README-RESULTS -->

<!-- README-MUTANTS -->

<!-- README-H1 -->

## Job bullet → evidence matrix (CONTRACT §4)

Reproduced verbatim from the contract:

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

Evidence for each row: the gates named in it are evaluated by `cpgate` over [`evidence/K3/`](evidence/K3/), [`evidence/K4/`](evidence/K4/), [`evidence/K5/`](evidence/K5/), [`evidence/K6/`](evidence/K6/) and [`evidence/K7/`](evidence/K7/) (each with `gate.json`, `negative-controls.json` and `closure.yaml`); the quorum table (K4.1) is [`evidence/K4/quorum-table.json`](evidence/K4/quorum-table.json); the findings packages are under [`findings/`](findings/); K7.2's statistics are in [`evidence/K7/results.json`](evidence/K7/results.json) and the campaign pages under [`evidence/K7/report/`](evidence/K7/report/).

## How to reproduce

Prerequisites: a Linux host as [`docs/PREREQUISITES.md`](docs/PREREQUISITES.md) lists (Docker 29.9 with buildx and Compose, cgroup v2, the kernel modules `nf_tables`, `nft_compat`, `sch_netem` and `/dev/fuse`, ≥ 12 GiB free RAM, the bridge subnet 10.77.0.0/24 free, Go ≥ 1.21 to bootstrap go1.26.9). Every entry point is a `make` target:

```sh
make prereq-check          # the 26 checks of docs/PREREQUISITES.md
make build                 # bin/commitproof, bin/cpcheck, bin/cpgate, bin/cpreport, bin/quorumxcheck
bin/commitproof images build   # Multigres e3cbdbb and the helpers from source; images.lock.json
make ci-unit               # gofmt, go vet, staticcheck, unit tests, independence and parameters tests
make ci-checkers           # synthetic corpora, elle-cli differential, checker sabotage controls
make ci-cluster-smoke      # cluster up / verify / down, a 45 s smoke run, cpcheck
make ci-nemesis-smoke      # one run per fault kind F1a, F2a, F3a, F7 (F4a with /dev/fuse)
make ci-mutant-smoke       # mutant M1, directed schedule D1, expects C1 lost-write
make ci-report             # re-renders the K6 reference report and compares the bytes
make campaign-k7 DRY_RUN=1 # K7's 130 runs with their plans; starts nothing
make campaign-k7           # the K7 matrix, detached and resumable (several hours, local only)
```

One run by hand: `bin/commitproof run --campaign C2 --seed 3 --out runs/` writes a sealed run directory; `bin/cpcheck check runs/<run-id>` writes its verdict next to it; `bin/cpreport render runs/<run-id> --out report/` renders its incident report. Every finding package gives the exact commands that replay its minimized plan. A sprint's gate: `bin/cpgate K7 --evidence evidence/K7 --negative-controls`.

## How to read the findings

Each package under [`findings/`](findings/) is written in the upstream issue format (S50) and has not been posted anywhere. Its first line states whether any acknowledged write was lost in its original run. Then come the classification (CONTRACT §11) and the sections Summary, Version and environment, Reproduction, Expected vs actual, Analysis, Recurrences in K7, K7 replay, Suggested direction (optional) and Limitations. Beside them: `finding.json` (runs, seal roots, replays), `plan.json` (the minimized plan), `checks.json` (the original run's checker output), `report-excerpt.md` (faults, rules, WAL positions, windows) and `claim.yaml`. The dimensions:

- **Guarantee and claim level** (§3): EXPLICIT is a public product statement (S48, S49, S42); DESIGN is a repository design document or test intent; INFERRED is an expectation derived from PostgreSQL compatibility that no Multigres source states.
- **Class and severity** (§11.1, §11.2, derived): `critical` needs a SAFETY_VIOLATION of an EXPLICIT guarantee; `high` is a SAFETY_VIOLATION of a DESIGN or INFERRED one.
- **Origin**: MULTIGRES, POSTGRES_INHERITED (the behaviour reproduces on plain PostgreSQL streaming replication or is documented PostgreSQL behaviour) or UNDETERMINED.
- **Reproducibility** (§11.1): from 5 replays of the minimized plan — REPRODUCED (≥ 3), INTERMITTENT (1–2), NOT_REPRODUCED (0).
- **Known status**: NEW, or KNOWN when an open upstream issue or unmerged pull request describes the same mechanism. A KNOWN package is an independent reproduction supporting that item, not a new issue.

## Limitations

The full list, with every status resolved, is [`docs/LIMITATIONS.md`](docs/LIMITATIONS.md). In short:

- One pinned commit of an alpha system, on one host, in a Compose topology derived from upstream's Kubernetes demo, not the operator or real zones (LIM-01, LIM-02).
- No power-loss or page-cache-loss faults (shared kernel), no clock skew, and a narrow, excluded fault list (LIM-03, LIM-04, LIM-20).
- Empirical, bounded evidence only: a clean cell says nothing about schedules outside the matrix (LIM-06). Narrow workloads (LIM-07) and a checker subset (LIM-08, LIM-22).
- Not covered: sharding, migrations, distributed transactions, replica-port reads (LIM-09, LIM-19, LIM-42); one durability policy (LIM-14); five mutants (LIM-15).
- Most runs are committed as summaries whose full archives stay on the machine that ran them; their exposure is cpcheck-reported from the sealed history and re-derivable only from that archive (LIM-30, LIM-37, LIM-43).
- K7's runs have the metrics exporter, the cited K4/K5 runs do not (LIM-41). There has been no remote CI run before the owner pushes (LIM-16).

## Repository

`cmd/` — the binaries (`commitproof`, `cpcheck`, `cpgate`, `cpreport`, `quorumxcheck`, and the helpers `slowfs`, `cpnet`). `internal/` — their packages. `build/`, `deploy/` — images and the Compose topology. `campaigns/` — the K7 matrix. `mutants/` — the five mutant patches. `docs/` — contract, ADRs, research register, limitations, prerequisites. `evidence/` — every sprint's records, runs and gate. `findings/` — the findings packages. `claims.yaml` — every claim of this README, LIMITATIONS and K7's NOTES with its artifact and hash. License: Apache-2.0.
