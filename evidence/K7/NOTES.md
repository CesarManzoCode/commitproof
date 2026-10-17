# K7 NOTES — Full campaign, statistics and release

Contract **1.8.1** (unchanged during K7: every K7 plan carries it). Evidence produced by `scripts/k7-evidence.sh` (groups setup, m1oracle, m1triage, images, start, wait, oracle, store), the resume steps of §2 and the fixes of §3, every command recorded in `commands.jsonl` (argv, exit code, stdout/stderr sha256), 2026-10-15 22:03 CST – 2026-10-17 03:05 UTC, one cluster at a time. No `claims.yaml` exists yet (K7.5 comes after the stop): every statement names its record, run id or file; "design says" cites RESEARCH ids, "system did" cites runs and records.

**Status: BLOCKED** at the store step, by the owner's decision 1 (ADR-0045 §5): after storing every K7 run, the P25 count is 246 678 804 bytes and the projection 251 678 804 bytes, above 250 MB (§8). Nothing stored was dropped. The matrix is complete (130 valid runs), K7.3's oracle records are written; the work that follows the store step (§9) waits for the owner's decision.

## 1. Gate

Not run: K7 stopped BLOCKED before its statistics, packages, claims audit and release (§9). `cpgate K7` exists (`c017e83`, `1b66d40`, K7.4 fixed in `9807b0c`).

## 2. Resume after the host power-off

The owner's machine was powered off during K7's matrix: `last -x` shows "shutdown system down" at 2026-10-15 22:39 CST and `journalctl --list-boots` ends that boot at 2026-10-16T04:39:31Z; the next boot started at 16:56:25Z. State found before anything was touched:

- **Git**: `main`, 67 commits ahead of `origin/main`, nothing pushed. The P25/repeats ADR (ADR-0045, `d07fca6`) and the K7.3/elle-cli ADR (ADR-0046, `6d06b73`, `49c541b`) had been committed before the matrix started. The matrix file and tooling had been committed in K6b (`09355f3`), the repeat-skipping driver in `a9d9a5d`, elle-cli on M1's D1 runs in `8788818`, the image lock in `d40cce9`, and cpgate K7 in `c017e83` and `1b66d40`. Not committed: the two records of the matrix start (`k7-dry-run`, `campaign-start`) and their logs. Nothing of `findings/`, `claims.yaml`, README, the overclaim review or CHANGELOG existed.
- **Campaign**: `runs/k7` held seven sealed runs, V1 C1 seeds 1–7 (`cpcheck verify-seal` valid, cpcheck PASS), and one unsealed directory, C1 seed 8's first attempt `20261016T043817Z-C1-8-real-9f75e28e` (no seal, `verify-seal` exit 2, its history ending in `run-end` `{"reason":"nemesis interrupted","status":"invalid"}`). The dry run showed 123 of 130 runs still to make.
- **Docker**: 26 exited project containers, the network `commitproof_cpnet` and seven project volumes; no stale FUSE mount of our containers, no stuck process.
- **Closure**: none.

Resume protocol (as in K4, K5 and K6b), every step recorded in `commands.jsonl`:

1. `prereq-check-resume` exit 2: 25/26, D5 FAIL — the only occupant of 10.77.0.0/24 was this project's `commitproof_cpnet` (`prereq-resume-1.json`). `docker-cleanup-resume` removed only the resources labelled `com.docker.compose.project=commitproof` (26 containers, the network, 7 volumes; images kept, no sudo). `prereq-check-resume-2`: 26/26 PASS (`prereq-resume-2.json`). `make prereq-check` writes `evidence/K1/prereq.json`; that file was restored to its committed content both times. The `docker-cleanup-resume` record was first written with raw newlines in its argv (`record-command.sh` did not escape them); the line was rejoined with escaped newlines, and `record-command.sh` was fixed (`748e549`).
2. `precondition-resume`: cpgate K1–K6 on copies of the closed evidence, every criterion PASS. `images-verify-resume`: the local images equal the lock.
3. The unsealed C1 seed 8 attempt was archived unchanged (`harness-defect-runs/`, README there), moved out of `runs/k7`, and re-run from scratch with the same plan and seed by the resumed campaign (`campaign-start-2`): `20261016T170819Z-C1-8-real-9f75e28e`, same `plan_sha256` `9f75e28e…`.

## 3. Harness defects found in K7 (none is a Multigres finding, §11.1)

1. **The runner's final reads reused a retired process.** After a recovery insert ended `info`, the recovery loop moved to a fresh process (§6.1), but `finalReads` restarted on process 10 + g. `verify-seal` then rejected the history ("proc 10 invokes after an info completion"). V1 C2 seed 3 was INVALID three times for this reason, a persistent HARNESS_DEFECT (§12.1). Fixed in `1f1f60b` (recovery and final reads share each gateway's process), with a regression test that fails on the old behaviour with the same message. The driver was stopped with SIGTERM right after the third attempt's cpcheck event; the next run stopped in its preflight before creating a run directory. Binaries and the runner image were rebuilt (`build-5`, `images-build-2`; only the runner image changed), the three attempts were set aside and stored as summaries (`harness-defect-runs/README.md`), and the campaign resumed (`campaign-start-3`). A run without an `info` recovery insert is unchanged by the fix, so the twelve runs sealed before it were reused.
2. **C3 dropped one transaction of every reported cycle.** `find()` built a reported cycle from the anchor path without its last node and from the closing walk without its final node, so the anchor's endpoint was in neither. A two-transaction G-single read "cycle i=53 -rw-> i=53" with ops [53], on the K2 corpus too: K2.1 compares class sets only, so this had gone unnoticed since K2. Its consequences: the anomaly's ops and explanation were wrong, and `simple()` ran on the shortened list, so a closed walk that revisits that node could be reported with the walk's shape instead of a simple cycle's. Whether a closed walk exists, hence the FAIL under the serializable model, did not depend on it. It was found in the triage of the re-run C2 seed 3, whose G-single read "cycle i=3230 -rw-> i=3230". Fixed in `32b660a`. Regression tests: a two- and a four-transaction cycle, with expected ops and explanations written from §10.3, and every cycle of the K2 W1 corpus naming one transaction per edge; all three fail on the old code. `make ci-checkers` with the fix is clean (record `ci-checkers-c3fix`: K2 corpus 807 entries, 350 compared with elle-cli, 0 disagreements; the K5 and G12 corpora too), so no expected classification changed. The driver was stopped again at a run boundary, binaries and the runner image were rebuilt (`build-6`, `images-build-3`), and the 15 sealed K7 runs were re-checked (`recheck-c3fix`): 0 verdicts and 0 class sets changed, and one checks.json changed (C2 seed 3's cycle explanations). The campaign then resumed (`campaign-start-4`). The checks.json committed by K2 and K4, and K7's copies of M1's D1 checks, keep their recorded explanations (closed evidence). The K4 runs' cycles are observations (`-process`, `-realtime` and SI `G2-item` cycles), which no verdict reads.

## 4. The matrix (§15 K7; `campaigns/k7.json`, `make campaign-k7`)

Records `campaign-start` … `campaign-start-4`, `campaign-wait`, `campaign-wait-2` (exit 1: the matrix has FAILs). The driver's summary: 130 cells complete, 19 FAIL, 111 PASS; the final dry run (`make campaign-k7 DRY_RUN=1`): 130 valid, 0 to make. Runs made with commitproof `8788818` (12 primaries: V1 C1 seeds 1–10, C2 seeds 1–2), `1f1f60b` (3 primaries: V1 C2 seeds 3–5, re-checked with the fixed C3, §3) and `32b660a` (every other run: 127 primaries and INVALID attempts, 43 minimization candidates, 27 replays); every manifest says contract 1.8.1, clean tree. K7's runs ran with ADR-0038's metrics exporter (LIM-41).

| Cell | Campaign / profile | Valid runs | FAIL | Classes of the FAIL runs |
|---|---|---|---|---|
| V1 | C1 / baseline | 10 | 0 | — |
| V1 | C2 / baseline | 10 | 4 | seed 3: G-nonadjacent, G-single, G2-item, incompatible-order; seed 7: non-linearizable; seed 8: incompatible-order; seed 9: incompatible-order, non-linearizable |
| V1 | C3, C4, C5, C6, C7, C10, C12 / baseline | 10 each | 0 | — |
| V1 | C9 / baseline | 10 | 10 | notify-not-resumed (all) |
| V2 | C1, C4 / slotless | 5 each | 0 | — |
| V2 | C2 / slotless | 5 | 2 | seed 1: non-linearizable; seed 3: incompatible-order, non-linearizable |
| V3 | C8 / buffered | 10 | 3 | planned-failover-error (seeds 3, 6, 10) |
| V5 | C11 / baseline | 5 | 0 | — |

**No acknowledged write was lost in any valid K7 run**: C1 (lost-acked-writes, final-read divergence) is `valid: true` in all 130 valid primaries' checks.json (`evidence/K7/runs/`).

INVALID attempts (re-run with the same seed, §12.1; none counted; each stored as a summary with its checks.json; causes from cpcheck's summaries in `runs/k7/campaign-k7.log`):

| Cause | Attempts |
|---|---|
| harness defect: final reads on a retired process (§3 item 1) | V1 C2 seed 3 ×3 (set aside, `harness-defect-runs/README.md`) |
| F6 not confirmed within 60 s (the decided rule's leader did not change) | V1 C9 seeds 2, 4; V1 C10 seed 4 ×2; V3 C8 seed 10 |
| F5b not confirmed within 120 s (the leader's `synchronous_standby_names` still named n4) | V1 C4 seed 2; V1 C12 seed 7; V2 C4 seed 2 ×2 |
| no recovery within P06 (final reads not completed) | V1 C9 seed 3 ×2; V1 C9 seed 5; one replay attempt of V1 C2 seed 3 (§7) |
| F3a not heal-confirmed within 30 s | V1 C3 seed 8 |

Every cell reached a valid run within its three attempts.

## 5. Failures: repeats and new failures (ADR-0045)

ADR-0045's predicates (`internal/repeats`, fixed before any K7 run) classified the 19 FAIL runs:

- **14 repeats**, counted in their cells and stored as summaries, not minimized: V1 C2 seed 7 (R-G6), seed 8 (R-G2b), seed 9 (R-G2b and R-G6); V1 C9 seeds 1–10 (R-G10); V2 C2 seed 3 (R-G2b and R-G6). They are to be cited as recurrences by the packages of FND-D1-fb8d0f8c, FND-D1-2e860fd2, FND-D1-a8eb9857, FND-C2-2a534585 (R-G2b), FND-C2-6806aeca (R-G6) and FND-C9-40b6f05a, FND-C9-2fe9a9fc, FND-C9-90da5532 (R-G10).
- **5 new failures**, minimized and replayed 5× by the driver (journals `<run>.minimize.json`, all `done: true`), their original and minimized runs stored in full. Not yet triaged or packaged (§9); what the checkers and the driver recorded:

| Run | Target class | Why no predicate matched | Minimized run | Replays with the class |
|---|---|---|---|---|
| V1 C2 seed 3 `20261016T173617Z-C2-3-real-9375b700` | G-nonadjacent | three C3 cycle groups (G-nonadjacent, G-single, G2-item) have no predicate; its incompatible-order group matches R-G2b | the original (no candidate kept G-nonadjacent; 11 candidates) | 0 of 5 |
| V2 C2 seed 1 `20261016T221153Z-C2-1-real-c8781715` | non-linearizable | R-G6 condition 5: F1c not among the confirmed faults (F1b, F1d) | `20261017T002205Z-C2-1-real-316f3877` | 5 of 5 |
| V3 C8 seed 3 `20261016T225050Z-C8-3-real-537e02d1` | planned-failover-error | G8 has no predicate | the original (11 candidates) | 2 of 5 |
| V3 C8 seed 6 `20261016T230045Z-C8-6-real-9601ddce` | planned-failover-error | as above | the original | 0 of 5 |
| V3 C8 seed 10 `20261016T231406Z-C8-10-real-cb16d052` | planned-failover-error | as above | the original | 1 of 5 |

Preliminary reading (system did; not yet a triage). C2 seed 3: an `info` transaction's appends (i=3178, 62 to key 12 and 63 to key 13) were read by `ok` transactions and later absent from the final reads; C3 infers key 12's version order from the transactions' reads only (LIM-22), so the vanish appears as dependency cycles; elle-cli finds the same G-single on keys 12 and 13 (§6). C8 seed 3: two explicit transactions invoked 19 ms after a judged switchover's injection got `57P03` on their first statement (`info`), not `40001`. Their classification by §11 (guarantee, origin, known status) is the triage still to do.

## 6. K7.3: elle-cli on the matrix's C3 anomalies (ADR-0046)

Group `oracle` (records `oracle-…`, `check-…`, `elle-validity-…`, `elle-validity-xmx9g-…`, `elle-process-…`, `k7oracle-…`), no cluster running. Four matrix runs have C3 anomalies; none is a plain confirmation, each is triaged in its record (`evidence/K7/oracle/<run-id>.json`):

| Run | elle-cli | Result |
|---|---|---|
| V1 C2 seed 3 | validity pass: invalid; incompatible-order, G-single (same keys 12 and 13 as C3's G-single, record `cycle-keys-…`), cycle-search-timeout; no answer for G-nonadjacent, G2-item | oracle-disagreement |
| V1 C2 seed 8 | out of memory (default heap and 9 GB) | oracle-disagreement (no answer, LIM-31) |
| V1 C2 seed 9 | invalid; incompatible-order, plus a G-single through key 42, whose reads are incompatible (record `cycle-keys-…`), as K4's D1 seed 3 | oracle-disagreement |
| V2 C2 seed 3 | out of memory (default heap and 9 GB) | oracle-disagreement (no answer, LIM-31) |

M1's five D1 runs (records of 2026-10-16 04:12–04:21Z, `evidence/K7/oracle/20261011T*-D1-*-M1-*.json`): three labelled oracle-disagreement because elle-cli did not complete (seeds 1, 2, 5), two because elle-cli adds G0 and G-single cycles through incompatible keys (seeds 3, 4).

## 7. Observation: no recovery within P06 in four INVALID attempts

Four attempts ended INVALID because no recovery insert succeeded within P06 (120 s) after the final heal (§4). Records `liveness-<run>` (`scripts/k7-liveness.py` over each sealed run's history and observations) show the same state in all four: one node is (or becomes) PRIMARY of the latest rule with its WAL position moving, every other node reports REPLICA with its WAL position frozen from the final heal to the end, and all 33 recovery inserts (11 per gateway) end `info` at the client deadline. In three of them the decided cohort had shrunk to two members. With `AT_LEAST_2` a commit on the primary then waits for a standby that does not replicate. In C9 the same plans produced valid runs on a later attempt. §11.1 names "no recovery within P06" a LIVENESS_OBSERVATION; §12.1 makes such a run INVALID (its final reads cannot complete), so these attempts count toward nothing and no K7 criterion packages them. They are reported here for the owner; their full archives are in `runs/k7/` on this machine.

## 8. Storage and P25: the stop

Record `store` (`scripts/k7-store.sh runs/k7 evidence/K7/runs --pending 0`, exit 4): 212 run directories stored once — **6 in full** (the five new failures' originals, V2 C2 seed 1's minimized run; ADR-0045) and **206 summaries** (stub and checks.json: the other matrix runs, every repeat, every INVALID attempt, every minimization candidate and replay). Measured:

| Quantity | Bytes |
|---|---|
| P25 count after the store step (every byte under `evidence/`; no run archive under `findings/`) | 246 678 804 |
| ADR-0045 §5 projection: count + 0 pending runs × 85 420 (mean summary) + 5 000 000 (K7's remaining records and text) | **251 678 804** |
| Not in that projection: K7.4's re-execution of the replay command of 14 packages (9 recorded + 5 new) × 5 replays as summaries ≈ 70 × 85 420 | ≈ 5 979 400 (→ ≈ 257 658 204) |
| The cited D3′ and D5 real-build runs in full (decision 2, only if P25 holds) | ≈ 11 830 000 (→ ≈ 269 488 204) |

The projection exceeds 250 MB, so K7 stops BLOCKED (the owner's decision 1). Before K7, `evidence/` held 222 011 297 bytes (2026-10-16 11:00 CST), 222 399 038 before the store step; the store added 24 279 766. The five new failures add about 9.4 MB in full. Options for the owner are in the closure.

## 9. Not done (after the store step), and drafts

Not done, in prompts/K7.md's order: `results.json` and `runs-index.json` (`k7results`), the campaign pages (`render-k7`, `render-cited-k4`, `render-cited-k5`), the triage records of the 19 FAIL runs (`scripts/k7-triage-final.py` for repeats; the five new failures by hand), the packages of the five new findings, K7.4's replays, `claims.yaml`, the final README, LIMITATIONS and CHANGELOG, `cpgate K7`, the CI targets, the tag `v1.0.0`, and the decision on D3′/D5. Drafts committed for the resumed K7, not final: `findings/` (the nine recorded findings' packages without their K7 sections, recurrences and K7 replays), `README.md`, `CHANGELOG.md`, `evidence/K7/overclaim-review.md`, and LIMITATIONS' LIM-16 and LIM-43.

## 10. Deviations from the prompt

- K7.4 executes each package's P13 replay command (`--times 5`): the CLI accepts no other count, and cpgate K7's check is corrected accordingly (`9807b0c`). That adds about 6 MB of replay summaries to finish K7 (§8).
- Two harness defects were fixed during the matrix (§3); the driver was stopped twice at a run boundary to rebuild, and the sealed runs were re-checked, not recomputed.
- The four "no recovery within P06" attempts are reported as an observation (§7); the contract gives no package for INVALID runs.

