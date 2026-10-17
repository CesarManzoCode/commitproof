# K7 NOTES — Full campaign, statistics and release

K7's matrix ran under contract **1.8.1** (every matrix plan carries it). After the matrix, the owner's decision on K7's BLOCKED closure raised P25 to 275 MB (ADR-0047, contract **1.9.0**). K7.4's replays ran under 1.9.0 with their packages' plans unchanged. Evidence produced by `scripts/k7-evidence.sh` (groups setup, m1oracle, m1triage, images, start, wait, oracle, store, results, render, triage, cited, c3audit, pkgreports, pkgreplay, p25, ci, gate, tagcheck), the resume steps of §2 and the fixes of §3. Every command is recorded in `commands.jsonl` (argv, exit code, stdout/stderr sha256), 2026-10-15 22:03 CST – 2026-10-17 UTC, one cluster at a time. Every statement that reports a result carries its `claims.yaml` id. "Design says" cites RESEARCH ids; "system did" cites runs and records.

## 1. Gate

`cpgate K7 --evidence evidence/K7 --negative-controls` (records `cpgate-k7…`): its result is `gate.json` and `negative-controls.json`, and the closure (`closure.yaml`) reports it. `cpgate K7` was written before the matrix (`c017e83`, `1b66d40`). Its K7.4 replay check was corrected in `9807b0c` to the P13 replay command (§13).

## 2. Resume after the host power-off

The owner's machine was powered off during K7's matrix: `last -x` shows "shutdown system down" at 2026-10-15 22:39 CST and `journalctl --list-boots` ends that boot at 2026-10-16T04:39:31Z. State found before anything was touched:

- **Git**: `main`, 67 commits ahead of `origin/main`, nothing pushed; ADR-0045 (`d07fca6`) and ADR-0046 (`6d06b73`, `49c541b`) committed before the matrix started.
- **Campaign**: `runs/k7` held seven sealed runs (V1 C1 seeds 1–7) and one unsealed directory, C1 seed 8's first attempt `20261016T043817Z-C1-8-real-9f75e28e` (no seal; its history ends in `run-end` `{"reason":"nemesis interrupted","status":"invalid"}`).
- **Docker**: 26 exited project containers, the network `commitproof_cpnet` and seven project volumes.

Resume protocol (as in K4, K5 and K6b):

1. `prereq-check-resume` exit 2: 25 of 26 checks, D5 failing on this project's own leftover `commitproof_cpnet`. `docker-cleanup-resume` removed only the resources labelled `com.docker.compose.project=commitproof` (images kept, no sudo). `prereq-check-resume-2`: 26 of 26 (`prereq-resume-2.json`). `evidence/K1/prereq.json` was restored to its committed content both times. [CLM-0031]
2. `precondition-resume`: cpgate K1–K6 on copies of the closed evidence, every criterion PASS; `images-verify-resume`: the local images equal the lock. [CLM-0032]
3. The unsealed attempt was archived unchanged (`harness-defect-runs/`) and re-run with the same plan and seed (`20261016T170819Z-C1-8-real-9f75e28e`, same `plan_sha256`).

The `docker-cleanup-resume` record was first written with raw newlines in its argv; the line was rejoined with escaped newlines and `record-command.sh` fixed (`748e549`).

## 3. Harness defects found in K7 (none is a Multigres finding, §11.1)

1. **The runner's final reads reused a retired process.** After a recovery insert ended `info`, the recovery loop moved to a fresh process (§6.1), but `finalReads` restarted on process 10 + g; `verify-seal` rejected such a history ("proc 10 invokes after an info completion"). V1 C2 seed 3 was INVALID three times for this reason, a persistent HARNESS_DEFECT (§12.1). Fixed in `1f1f60b`, with a regression test that fails on the old behaviour. The driver was stopped at a run boundary, the runner image rebuilt (`build-5`, `images-build-2`), and the three attempts set aside as summaries (`harness-defect-runs/README.md`). The twelve runs sealed before the fix had no `info` recovery insert and were reused. [CLM-0029]
2. **C3 dropped one transaction of every reported cycle.** A reported cycle lacked the anchor's endpoint, so the anomaly's ops and explanation were wrong and the shape step ran on the shortened list. Whether a cycle exists, hence a FAIL under the serializable model, did not depend on it. Fixed in `32b660a` with three regression tests. `make ci-checkers` with the fix is clean (`ci-checkers-c3fix`). The 15 K7 runs sealed before it were re-checked before any was committed (`recheck-c3fix`): no verdict or class set changed, and one `checks.json` (V1 C2 seed 3's cycle explanations) was rewritten. What it changes in the committed evidence is §10. [CLM-0030]

## 4. The stop, the owner's decision and the resumption (ADR-0047)

K7 first closed BLOCKED at its store step (record `store`, exit 4; closure at `fb5d22f`): the P25 projection after storing every K7 run was 251 678 804 bytes, above 250 MB. Nothing stored was dropped. The owner then approved P25 = 275 000 000 bytes, asked that the cited D3′/D5 runs be kept in full, and asked for an audit of the claims the C3 correction affects. ADR-0047 records the decision verbatim (contract 1.9.0, `566d1af`). K7 resumed from the store step: `store-2` stored nothing new and reported a projection of 251 729 547 bytes against 275 000 000 (exit 0). [CLM-0014] [CLM-0033]

## 5. The matrix (§15 K7; `campaigns/k7.json`, `make campaign-k7`)

Records `campaign-start` … `campaign-start-4`, `campaign-wait`, `campaign-wait-2`. The final dry run reported 130 valid and 0 to make. Runs were made with commitproof `8788818` (12 primaries), `1f1f60b` (3 primaries, re-checked with the fixed C3) and `32b660a` (every other run). Every manifest says contract 1.8.1, clean tree, and every run had ADR-0038's metrics exporter (LIM-41). `k7results` wrote `results.json` and `runs-index.json` (215 stored K7 runs with their seal roots); `render-k7` wrote the campaign page with §12.3's statistics. [CLM-0020]

| Cell | Campaign / profile | Valid runs | Failing runs | Classes of the failing runs |
|---|---|---|---|---|
| V1 | C1 / baseline | 10 | 0 | — [CLM-0020] |
| V1 | C2 / baseline | 10 | 4 | seed 3: G-nonadjacent, G-single, G2-item, incompatible-order; seed 7: non-linearizable; seed 8: incompatible-order; seed 9: incompatible-order, non-linearizable [CLM-0020] |
| V1 | C3, C4, C5, C6, C7, C10, C12 / baseline | 10 each | 0 | — [CLM-0020] |
| V1 | C9 / baseline | 10 | 10 | notify-not-resumed (all) [CLM-0020] |
| V2 | C1, C4 / slotless | 5 each | 0 | — [CLM-0020] |
| V2 | C2 / slotless | 5 | 2 | seed 1: non-linearizable; seed 3: incompatible-order, non-linearizable [CLM-0020] |
| V3 | C8 / buffered | 10 | 3 | planned-failover-error (seeds 3, 6, 10) [CLM-0020] |
| V5 | C11 / baseline | 5 | 0 | — [CLM-0020] |

**No acknowledged write was lost in any valid K7 run**: C1 is valid in all 130 valid primaries. [CLM-0025]

INVALID attempts were re-run with the same seed (§12.1) and none is counted; each is stored as a summary with its checks.json, and `results.json` lists each one with its cause [CLM-0020]:

| Cause | Attempts |
|---|---|
| harness defect: final reads on a retired process (§3 item 1) | V1 C2 seed 3 ×3 |
| F6 not confirmed within 60 s (the decided rule's leader did not change) | V1 C9 seeds 2, 4; V1 C10 seed 4 ×2 [CLM-0020] |
| F5b not confirmed within 120 s | V1 C4 seed 2; V1 C12 seed 7; V2 C4 seed 2 ×2 [CLM-0020] |
| no recovery within P06 (final reads not completed; §11) | V1 C9 seed 3 ×2; V1 C9 seed 5 |
| F3a not heal-confirmed within 30 s | V1 C3 seed 8 [CLM-0020] |

Every cell reached a valid run within its three attempts.

## 6. Failures: repeats and new failures (ADR-0045)

ADR-0045's predicates (`internal/repeats`, fixed before any K7 run) classified the 19 failing runs [CLM-0026]:

- **14 repeats**, counted in their cells, stored as summaries and not minimized: V1 C2 seed 7 (R-G6), seed 8 (R-G2b), seed 9 (R-G2b and R-G6); V1 C9 seeds 1–10 (R-G10); V2 C2 seed 3 (R-G2b and R-G6). Each has a triage record (`triage/<run>.json`, `scripts/k7-triage-final.py`: §6.2 review, oracle record for C3, §11 classification), and each is listed with its matched fields as a recurrence in the packages its predicates stand for. [CLM-0026]
- **5 new failures**, minimized and replayed 5× by the driver, original and minimized runs stored in full, triaged by hand (`triage/<run>.json`) and packaged as new findings (§9) [CLM-0026]:

| Run | Finding | Why no predicate matched | Minimized run | Recorded replays with the class |
|---|---|---|---|---|
| V1 C2 seed 3 `20261016T173617Z-C2-3-real-9375b700` | FND-C2-c7475a73 (G5) | its C3 cycle groups have no predicate (its incompatible-order group matches R-G2b) | the original (11 candidates) | 0 of 5 G-nonadjacent [CLM-0046] |
| V2 C2 seed 1 `20261016T221153Z-C2-1-real-c8781715` | FND-C2-8215101c (G6) | R-G6's condition 5: F1c not among the confirmed faults (F1b, F1d) | `20261017T002205Z-C2-1-real-316f3877` (F1d alone, 22.5 s) | 5 of 5 [CLM-0045] |
| V3 C8 seed 3 `20261016T225050Z-C8-3-real-537e02d1` | FND-C8-ff4475d8 (G8) | G8 has no predicate | the original (11 candidates) | 2 of 5 [CLM-0047] |
| V3 C8 seed 6 `20261016T230045Z-C8-6-real-9601ddce` | FND-C8-e6b7ca52 (G8) | as above | the original | 0 of 5 [CLM-0048] |
| V3 C8 seed 10 `20261016T231406Z-C8-10-real-cb16d052` | FND-C8-5254b5cb (G8) | as above | the original | 1 of 5 [CLM-0049] |

The triage, in short:

- **FND-C2-c7475a73.** Every cycle runs through the `info` transaction i=3178, whose key-12 element was read by `ok` transactions and is absent from the final reads after the failover to n3. C3 infers key 12's version order from the transactions' reads (LIM-22). Origin UNDETERMINED: the cycles may be the vanish's effect on that order; the replays reproduced the vanish without cycles. [CLM-0046]
- **FND-C2-8215101c.** FND-C2-6806aeca's mechanism with the leader reachable through F1d instead of F1c: a `cas` whose client timed out became visible, was read by an `ok` read, and was removed by the failover. POSTGRES_INHERITED. [CLM-0045]
- **The three C8 findings.** Explicit transactions that began 15–20 ms after a judged switchover's injection received `57P03` on a statement after `BEGIN`, not `40001`. S14 lists the errors the gateway buffers, and `57P03` is not one of them. Origin hypothesis MULTIGRES; the routing is not traced. [CLM-0047] [CLM-0048] [CLM-0049]

## 7. K7.3: elle-cli on C3 anomalies (ADR-0046)

Group `oracle` (no cluster running). Four matrix runs have C3 anomalies; each is labelled oracle-disagreement with a triage in `oracle/<run-id>.json` [CLM-0027]:

| Run | elle-cli | Result |
|---|---|---|
| V1 C2 seed 3 | validity pass: invalid; incompatible-order and the same G-single as C3 (keys 12 and 13); cycle search timed out, no answer for G-nonadjacent and G2-item | oracle-disagreement [CLM-0027] |
| V1 C2 seed 8 | out of memory (default heap and 9 GB) | oracle-disagreement, no answer (LIM-31) [CLM-0027] |
| V1 C2 seed 9 | invalid; incompatible-order plus a G-single through key 42, whose reads are incompatible | oracle-disagreement [CLM-0027] |
| V2 C2 seed 3 | out of memory (default heap and 9 GB) | oracle-disagreement, no answer (LIM-31) [CLM-0027] |

M1's five D1 runs (records of 2026-10-16 04:12–04:21Z): three oracle-disagreement because elle-cli did not complete (seeds 1, 2, 5), two because elle-cli adds G0 and G-single cycles through incompatible keys (seeds 3, 4). [CLM-0027]

## 8. K7.4: each package's replay command executed once more

Group `pkgreplay`, no other cluster running. Each package's minimized run was extracted from its committed archive (seal verified against `finding.json`), and `commitproof replay <minimized run> --times 5` was executed (record `pkg-replay-<id>`; P13's command, the only count the CLI accepts). CommitProof `e8eca97`, contract 1.9.0, ran with the metrics exporter. The replays are stored as summaries in `package-replays/`, and `k7findings` wrote their results into each `finding.json` (`k7_replay`). The recorded reproducibility stands; this is reported beside it.

<!-- K7.4-TABLE -->

## 9. Findings packages (§11.4)

Fourteen packages under `findings/` (written, not published): the nine recorded findings, with the classifications ADR-0042 and ADR-0043 gave them and the recorded classification beside them, and the five new findings of §6. Each holds README (upstream issue format, S50; first line G1), `finding.json`, `plan.json` (minimized plan), `checks.json` (original run's), `report-excerpt.md` (from cpreport's page of the original run, `render-pkg-<id>`, `excerpt-<id>`) and `claim.yaml`. [CLM-0040] [CLM-0041] [CLM-0042] [CLM-0043] [CLM-0044] [CLM-0045] [CLM-0046] [CLM-0047] [CLM-0048] [CLM-0049] [CLM-0050] [CLM-0051] [CLM-0052] [CLM-0053]

## 10. What the C3 correction changes (`c3-correction-audit.md`)

`scripts/k7-c3-audit.sh` re-checked, in scratch copies, every committed run held in full that has a C3 result, with the corrected cpcheck (record `c3-audit-2`). Of 172 runs, none changed its verdict, C3 validity, anomaly classes, class totals or observation classes; 166 are identical. In 6 K4 runs only the operations and explanations of cycles changed, all of them `-process`, `-realtime` or SI observations that no verdict reads. One of them is FND-D1-a8eb9857's original run, whose package `checks.json` must stay byte-identical; its README now says so. No README, LIMITATIONS row, finding or claim quotes a C3 cycle's operations from before the fix. LIM-44 records it. [CLM-0015]

## 11. Observation: no recovery within P06 in four INVALID attempts

Four attempts ended INVALID because no recovery insert succeeded within P06 (120 s) after the last heal: V1 C9 seed 3 ×2, V1 C9 seed 5, and replay 4 of V1 C2 seed 3. Records `liveness-<run>` (`scripts/k7-liveness.py`) and `liveness-summary` show the same state in all four. One node is PRIMARY of the latest rule with its WAL position moving; every other node reports REPLICA with its WAL position frozen from the last heal to the end; all 33 recovery inserts (11 per gateway) end `info` at the client deadline. With `AT_LEAST_2`, a commit then waits for a standby that does not replicate. §11.1 names this a LIVENESS_OBSERVATION; §12.1 makes the run INVALID, so these attempts count toward nothing and no criterion packages them (LIM-45). Each cell reached a valid run on a later attempt. Why replication stopped is not determined; the full archives are in `runs/k7/` on this machine. [CLM-0016]

## 12. Storage and P25 (ADR-0039, ADR-0045, ADR-0047)

- Store step (`store`): 212 run directories stored once, 6 in full (the five new failures' originals and V2 C2 seed 1's minimized run) and 206 as summaries (stub and `checks.json`). [CLM-0014]
- The cited D3′ and D5 real-build runs in full (`store-cited`): 10 archives from `runs/k5-full/` into `cited/`, each manifest, plan and seal byte-identical to K5's stub, its seal verified and its root equal to K5's `checks.json`. K5's stubs are untouched. Their campaign page (`render-cited-full`) gives both cells exposure from the histories and §12.3's statement. [CLM-0018]
- K7.4's 70 replays are stored as summaries in `package-replays/`. [CLM-0028]

<!-- P25-FINAL -->

## 13. Claims, overclaims, public documents

- `claims.yaml` (§17) holds every claim of README, LIMITATIONS and this file, each with contract section, target, command, artifact, the artifact's sha256 (a run archive: its seal root), result and limitations. Each package's `claim.yaml` is its entry there.
- `overclaim-review.md` lists every statement changed from earlier sprints' drafts, K7's BLOCKED draft included, and why.
- README: the §4 matrix verbatim, results per cell with §12.3's statement for every clean cell (K7's 11 and the cited D2′, P1, DH1, D3′ and D5 real-build cells), mutants beside the real build, H1, the findings ordered by severity after the G1 line, and the limitations summary.
- LIMITATIONS: no row PENDING; LIM-16 OPEN (owner action: the first remote CI run); new rows LIM-44 (C3 correction) and LIM-45 (liveness is recorded, not judged).

## 14. CI and release

`make lint-workflow` and every `make ci-*` target (`ci-unit`, `ci-checkers`, `ci-cluster-smoke`, `ci-nemesis-smoke`, `ci-mutant-smoke`, `ci-report`) ran on the release commit's clean build (record `ci…`). The annotated tag `v1.0.0` is local (`git tag -a`); `scripts/k7-tag-check.sh` records its object type, its commit, that its code equals the CI's, and that no remote-tracking ref contains K7's first commit. Nothing was pushed.

## 15. Deviations from the prompt

- K7 stopped once BLOCKED at the store step (owner decision 1) and resumed after the owner raised P25 to 275 MB (ADR-0047, contract 1.9.0). The version changed after the matrix, not while it ran. K7.4's replays record 1.9.0 in their manifests and replay their packages' plans unchanged.
- K7.4 executes each package's P13 replay command (`--times 5`): the CLI accepts no other count, and cpgate K7's check was corrected to it (`9807b0c`).
- Two harness defects were fixed during the matrix (§3); the driver was stopped twice at a run boundary, and the sealed runs were re-checked, not recomputed.
- `k7findings` counted an INVALID replay attempt as a recorded replay; fixed (`65ff46a`) before any package was written from it.
- The four "no recovery within P06" attempts are reported as an observation (§11, LIM-45): the contract gives no package to INVALID runs.
