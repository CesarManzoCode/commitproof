# Changelog

All notable changes to CommitProof. The normative specification is `docs/CONTRACT.md` (version in its header); every change to it is an ADR (`docs/adr/`). Each sprint's results, gate and closure report are under `evidence/K<n>/`; this file lists what each sprint added, not its results.

## v1.0.0 — 2026-10-17 (sprint K7; contracts 1.8.0, 1.8.1, 1.9.0)

Full campaign, statistics and release; the annotated tag `v1.0.0` is local, nothing is pushed. Results, gate and closure: `evidence/K7/` and its `closure.yaml`.

- The K7 matrix (`campaigns/k7.json`, `make campaign-k7`, resumable, detached, with a dry run; ADR-0041): cells V1 (C1–C7, C9, C10, C12 on `baseline`, 10 runs each), V2 (C1, C2, C4 on `slotless`, 5 each), V3 (C8 on `buffered`, 10) and V5 (C11 on `baseline`, 5), 130 runs on the real build with ADR-0038's metrics exporter; the directed cells of K4 and K5 cited, not re-run (ADR-0039).
- Storage under P25: runs a claim or a finding names are committed in full, every other run as a summary (stub and `checks.json`) listed with its seal root in `evidence/K7/runs-index.json` (ADR-0039); a real-build failure that repeats a recorded finding (ADR-0045's predicates over `checks.json`) is counted, triaged and cited as a recurrence, kept as a summary (ADR-0045, contract 1.8.0). P25 raised to 275 MB by the owner after K7's first store step, with the cited D3′/D5 real-build runs committed in full under `evidence/K7/cited/` (ADR-0047, contract 1.9.0).
- elle-cli on every C3 anomaly of the matrix and on M1's five D1 runs, with a triage for each disagreement (ADR-0046, contract 1.8.1).
- Fourteen findings packages under `findings/` (§11.4; nine recorded, five new), each with its replay command executed once more (K7.4); `claims.yaml` (§17) with a `claim.yaml` per package; the overclaim review (`evidence/K7/overclaim-review.md`); the C3-correction audit (`evidence/K7/c3-correction-audit.md`); the public `README.md`; final `docs/LIMITATIONS.md` (LIM-44, LIM-45, LIM-46 added).
- `cpgate K7` (criteria K7.1–K7.7 with negative controls); tools `k7results`, `k7findings`, `k7triage`, `k7oracle`; scripts `k7-evidence.sh` (with the groups `liveness`, `index` and `packages` for K7.4's results), `k7-store.sh`, `k7-store-cited.sh`, `k7-c3-audit.sh`, `k7-p25-count.sh`, `k7-tag-check.sh`, `k7-liveness.py`, `k7-package-k7-sections.py`; `k7results --pkg-replays` lists K7.4's replays in `runs-index.json`.
- Fixes: the runner's final reads continue on the process recovery ended with (`1f1f60b`); C3 reports every transaction of a cycle (`32b660a`; verdicts and classes unchanged, `c3-correction-audit.md`); `record-command.sh` escapes newlines in argv (`748e549`); K7.4 executes the package's P13 replay command (`9807b0c`); `k7findings` does not count an INVALID replay attempt (`65ff46a`); `cpgate K7` holds K7.4's replays to the contract version they carry, 1.9.0, and the matrix's runs to 1.8.1 (`9bf9005`).

## Sprint K6b and pre-K7 — 2026-10-15 (contract 1.2.0 → 1.7.0)

Readiness work for K7 (`evidence/pre-K7/`, `evidence/K6b/`): G12 judged outside fault windows (ADR-0037), the metrics exporter (ADR-0038), K7 storage and cited cells (ADR-0039), C13 and V4 removed (ADR-0040), C12 and the `slotless` profile (ADR-0041), G2 split into G2a and G2b (ADR-0042), known status `KNOWN` and the G6 finding (ADR-0043), the run's exposure in `checks.json` (ADR-0044); the plain-PostgreSQL experiment behind LIM-33.

## Sprint K6 — 2026-10-15 (contract 1.1.1)

`cpreport render`: a static, self-contained, byte-reproducible incident report of a sealed run and a per-campaign summary page with §12.3 statistics in integer arithmetic; `make ci-report` (ADR-0036). Closure: `evidence/K6/closure.yaml`.

## Sprint K5 — 2026-10-14 (contracts 1.0.9, 1.1.0)

Workloads W5–W8 (session state, LISTEN/NOTIFY, CDC, overload); checkers C7–C9; campaigns C8–C11; mutants M3 and M5 with directed schedules D3′ and D5 (ADR-0030, ADR-0033); P25 at 250 MB (ADR-0034). Closure: `evidence/K5/closure.yaml`.

## Sprint K4 — 2026-10-12 (contract 1.0.7)

Campaign driver, `minimize` and `replay`; campaigns C1–C7; mutants M1, M2, M4 with D1, D2′ and probe P1 (ADR-0024, ADR-0027); the historical control H1 (ADR-0025); `quorumxcheck` (§10.8). Closure: `evidence/K4/closure.yaml`.

## Sprint K3 — 2026-10-11 (contract 1.0.4)

The nemesis: every fault of §8 with confirmation and heal predicates, fail-closed; slowfs (ADR-0006, ADR-0022); `make ci-nemesis-smoke`. Closure: `evidence/K3/closure.yaml`.

## Sprint K2 — 2026-10-10 (contract 1.0.3)

`cpcheck check` with checkers C1–C6 and C9's G8 logic, the synthetic corpus, the elle-cli differential oracle (ADR-0004), checker sabotage controls; `make ci-checkers`. Closure: `evidence/K2/closure.yaml`.

## Sprint K1 — 2026-10-10 (contract 1.0.3)

Reproducible cluster and history recording: images built from Multigres `e3cbdbb` with upstream's Dockerfiles (FROM lines only), `images.lock.json`, the Compose topology, seed-deterministic plans, sealed hash-chained histories, `cpcheck verify-seal`; `make ci-unit`, `make ci-cluster-smoke`. Closure: `evidence/K1/closure.yaml`.

## K0 — 2026-10-09 (contract 1.0.0)

The contract, ADRs 0001–0018, the research register and the sprint prompts.
