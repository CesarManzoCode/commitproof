# Pre-K7 readiness report

Date 2026-10-15 (owner's time zone; records in UTC on 2026-10-16). Contract **1.2.1**. Nothing was pushed or published; K7 was not started. Every command below is a record in `evidence/pre-K7/commands.jsonl` (argv, exit code, stdout/stderr sha256) produced by `scripts/prek7-evidence.sh`.

## READY FOR K7: **no**

Two things stand between this tree and a K7 that can run and close without surprises, both outside what this task may decide:

1. **P25 (owner decision, feasibility STOP).** The 190-run matrix fits the remaining ~29 MB only if §13's summarizing policy is applied strictly and `findings/` is outside P25 (policy A); under K4/K5's storage practice it needs ~38 MB (`k7-feasibility.md` §4–§5). Options there; none chosen.
2. **K7's matrix is not fully implemented.** Campaigns C12 and C13, the profiles `slotless` and `replica-port`, the matrix file and `make campaign-k7` do not exist, and `prompts/K7.md` assumes them (`k7-feasibility.md` §1). Their schedules need ADRs before any run.

Everything else asked for is done (below).

## What was done

| Step | Result | Evidence |
|---|---|---|
| Precondition | cpgate K1–K6 PASS on copies of the committed evidence, at the start and again at the end | records `precondition`, `precondition-2` |
| 1a. ADR-0035 (D-B) | Accepted prospectively; applied by **ADR-0037**, contract **1.2.0** (a changed verdict rule is MINOR; the decision allowed "the next number"). Plans carrying ≥ 1.2.0 judge G12 outside fault windows, report both fractions (C8 `stats.storm_windows`, campaign page), and a window with fewer than one judged operation per second is not judged (C8 unknown). Corpus `testdata/corpus-k7` (31 histories) matches its hand-written expectations; sabotage `fair_share`, `g12_fault_exclusion`, `g12_judged_minimum` fail exactly their labels. All 11 recorded K5 C11 runs re-check byte-identically: no recorded verdict changes. | ADR-0037; record `g12-recheck`; `make ci-checkers` |
| 1b. Scraper (D-A) | No HTTP port serves `/metrics` at e3cbdbb (status page, byte-identical to a nonexistent path, on all nine components). **ADR-0038**, contract **1.2.1**: the pinned OpenTelemetry Prometheus exporter on 9464 (as upstream's e2e harness does), 16 integer families every second. Smoke run: 540 scrapes, 0 failed, **27 759 bytes per run-minute stored** (174 919 uncompressed). The report draws metric timelines; a run without scrapes renders byte-identically to the K6 example (35dc3e48…c94c). cpgate's K6.5 cross-checks the metrics block; controls: forged value → fail, `cp_sabotage_report_metrics` → fail, and that sabotaged build renders the K6 reference unchanged. LIM-40 resolved for K7. | ADR-0038; records `metrics-probe-2`, `scrape-*`, `metrics-controls` |
| 2. K7 feasibility | 190 runs (165 + 25 real-build directed runs §9.1/§12.4 require), ~7.6 h measured (8–19 h with re-runs and minimizations), bounds 0.259 (N = 10) / 0.451 (N = 5), P25: see above | `k7-feasibility.md`; records `durations`, `k7bounds`, `evidence-size` |
| 3. Upstream (D-C) | 39 read-only lookups, logged. G10: **KNOWN** — open PR #1550 fixes the same mechanism, not merged. G2: **NEW**. G12: **NEW**. Both named files are identical at upstream main `b2137f9`. | `upstream-check.md`, `upstream/lookups.jsonl` |
| 4. LIM-33 (D-D) | On plain PostgreSQL 17.7, 5 of 5: `pg_cancel_backend` or a client CancelRequest on a SyncRep-waiting commit makes it visible; a failover with `pg_rewind` removes it (the G2 anomaly); a client that only disconnects does not cause it. CommitProof's pgx client sends a CancelRequest at P01. LIM-33 updated; origin stays UNDETERMINED in the records (proposal below). | `lim33.md`; records `lim33-run-1`…`5`, `lim33-summary` |
| 5. Findings | Every recorded severity and claim level is what §11.1–§11.2 derive; six proposals, none applied | `findings-readiness.md` |
| 6. Hygiene | all green; see "Hygiene" | records `lint-workflow`, `ci`, `precondition-2`, `evidence-size-2`, `backup-manifest`, `fault-metrics` |

## Hygiene

- **CI-equivalent:** `make lint-workflow` and `make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke ci-mutant-smoke ci-report` exit 0 on the clean `64397d5` (records `lint-workflow`, `ci`: 01:02:37–01:18:37Z, binaries built `Dirty=false`). The K2, K5 and new K7 corpora regenerate byte for byte and k2check reports 0 problems for each (807, 157, 31 entries; 9, 12, 3 sabotage rules); F4a ran (`/dev/fuse` present); `ci-mutant-smoke`: M1 via D1 FAIL with C1 lost-write; `ci-report`: identical to the committed K6 example (35dc3e48…c94c). The images rebuilt by the CI run are locked in `build(images): lock the images of the pre-K7 CI build`.
- **cpgate K1–K6:** PASS on copies of the committed evidence at the start (`precondition`) and at the end (`precondition-2`, with K6.5's new metrics check vacuous on K6's reference run).
- **ADR-0038 on runs with faults:** the CI run's five nemesis smoke runs and its D1/M1 run, rendered and cross-checked by metricscheck: all agree (`fault-metrics`); F1a's isolation shows as 11 of 60 failed scrapes of `pooler-n1`. No Multigres process was killed in them, so counter resets are covered by unit tests only.
- **Contract version:** header 1.2.1 = `params.ContractVersion` (the parameters-table test in `ci-unit`).
- **File sizes:** largest file in the repository 4 110 156 bytes (`evidence/K1/runs/…/history.jsonl`), none near 50 MB.
- **Evidence size:** 220 247 115 bytes under `evidence/` before this report's commit (`evidence-size-2`); P25 = 250 MB; 29.75 MB left.
- **Backup manifest** (`backup-manifest.tsv`, printed by record `backup-manifest`, sha256 `e928b67c3b2cb1e8640e1dd98ee8fdadcc78e0d821f464044d09b276a3785067`): 320 archives in `runs/k4-full/` and `runs/k5-full/`, 253 764 025 bytes; **308 exist only on this machine (233 782 707 bytes)**, 12 are byte-identical to committed archives. Columns: path, bytes, sha256, copy (`local-only`/`in-git`).
- **git status:** clean after the final commit (nothing pushed).

## Owner decisions still open

1. **K7 storage under P25** — (1) §13 strictly + `findings/` outside P25; (2) fewer cells; (3) smaller N; (4) raise P25 (`k7-feasibility.md` §5).
2. **Implementing C12, C13, `slotless`, `replica-port`, the matrix file and `make campaign-k7`** — (a) amend `prompts/K7.md` so K7 implements them, with ADRs fixing their schedules before any run; (b) a separate implementation task before K7.
3. **Matrix counts** — 190 runs (with the real-build directed runs) or the 165 §15 K7 states; V4: 5 runs in total or 5 per variant (no nemesis, F2a).
4. **ADR-0038's exporter** — confirm the deployment divergence and P23's corrected parenthetical (your decision said "do not amend §5.4/P23"; the value and §5.4 are unchanged, the location was wrong). The only alternative without an exporter is to drop scrapes, which D-A rejected.
5. **Contract 1.2.0 for ADR-0037** instead of 1.1.2 (contract versioning: a changed verdict rule is MINOR).
6. **Findings** (`findings-readiness.md`): (1) whether G2's EXPLICIT level should cover reads of never-acknowledged writes (a G2-row change by ADR would make the four `high`; by the rules they stay `critical`); (2) G2 origin → `POSTGRES_INHERITED` in K7's packages; (3) G10 known status: keep `NEW` citing PR #1550, or add a value by ADR; (4) one package or three for the G10 ids; (5) FND-C11-17a344c1's package states both fractions; (6) **an unrecorded G6 finding** in C2 seed 3 (`non-linearizable`, `high`, 3 of 5 replays) — package it in K7 or confirm K4's folding.

## BLOCKED items

None under AGENTS.md §3 (no missing prerequisite, no irreversible or publishing action needed). The feasibility STOP (owner decision 1) is the task's own stop condition, recorded as an open decision.

## Deviations from the task

- P23's parenthetical was corrected (ADR-0038) although the decision said "do not amend §5.4/P23": scraping is impossible at the pinned commit without an exporter, and the old text named endpoints that do not exist. P23's value and §5.4 are unchanged.
- ADR-0035 is accepted by updating its status and applying it in a new ADR-0037 (the pattern of ADR-0032/0034 and ADR-0031/0033), at contract 1.2.0 rather than 1.1.2.
- The K6 gate gained a metrics cross-check in K6.5 (vacuous on K6's reference run; K6 still PASS on its evidence); the scraped run's cross-check and controls live in pre-K7 evidence through `internal/tools/metricscheck`, because K6's closed evidence holds no scraped run.
- The LIM-33 script's header comment first said CommitProof's client sends no CancelRequest; the runs' code was right, the comment was corrected (`eebd1f9`) and `lim33.md` states it.
- The upstream lookups' raw responses are stored compressed (`raw-responses.tar.zst`); the logged sha256s are of the uncompressed files.

## Pending user actions

- Decide items 1–6 above.
- Keep `runs/k4-full/` and `runs/k5-full/` (the only full copies of the K4/K5 stub runs; manifest `backup-manifest.tsv`).
- Push, enable Actions and publish findings when you choose (nothing was pushed).
