# K6b readiness report

Date 2026-10-15 (owner's time zone; records in UTC on 2026-10-16). Contract **1.7.0**. Nothing was pushed or published; the K7 matrix was not run. Every command below is a record in `evidence/K6b/commands.jsonl` (argv, exit code, stdout/stderr sha256), produced by `scripts/k6b-evidence.sh`.

## READY FOR K7: **no**

One owner decision stands between this tree and K7 — the one decision 1 asked me to stop on:

**Under decision 1 the projected evidence does not fit P25.** K7's 130 runs as summaries cost 11.7 MB (measured on the real plans), and §11.3 makes every minimized real-build failure a new finding whose original and minimized runs stay in full. With K4's and K5's failure rates (~16 failures, ten of them C9's deterministic G10 mechanism) that ends at **~277 MB** (~288 MB if the cited D3′/D5 real-build cells get their clean statements). If failures of already-recorded mechanisms stay summaries, **~239 MB** (`k7-estimate.md` §4). Options below; none chosen.

Everything else asked for is done.

## What was done

| # | Item | Result | Evidence |
|---|---|---|---|
| — | Precondition | cpgate K1–K6 PASS on copies of the committed evidence, at the start and at the end | records `precondition`, `precondition-2` |
| 1 | P25, §13 literally | ADR-0039 (1.3.0): cited runs in full once, every other run a summary (stub + `checks.json`, `scripts/store-summaries.sh`); P25 counts `evidence/` and run archives under `findings/`, not `findings/` text. Projection: **does not fit** (above) | ADR-0039; `k7-estimate.md` |
| 2 | Cite the directed cells | ADR-0039: D1, D2′, D3′, D5, P1 (mutant and real) and DH1 cited with their plans' versions (1.0.6, 1.0.7, 1.1.0, 1.0.9). **No K7.x criterion needs them under the final version** (table in ADR-0039): all 55 valid primaries re-check byte-identically with this cpcheck; K7.3 must still run elle-cli on M1's D1 runs (K4 cross-checked only real-build failures) | records `cited-recheck` (kept: missed K4's oracle pass on D1 seed 3), `cited-recheck-2` (55/55) |
| 3 | C12 and `slotless` | ADR-0041 (1.4.1) committed before any run: C12 = F5a(n4), two kinds drawn uniformly without replacement in blocks of seven seeds (seeds 1–7 cover all 14), F5b(n4); per-kind targets/holds of the campaigns that already use them; V1 seeds 1–10, V2 1–5. `slotless`: rendered configs differ in exactly the seven flags. **Smoke (not evidence)**: C12 seed 1001 and C4 seed 1001 on `slotless`, both valid PASS by cpcheck; every pooler and gateway of the slotless run ran `=false` | records `slotless-config`, `smoke-wait`, `smoke-cpcheck-1/2`, `smoke-store`; `smoke-not-evidence/` |
| 4 | Cut C13 and V4 | ADR-0040 (1.4.0): §4 bullet 6, §5.5 (`replica-port` gone), P15, §15 K7, R15; LIM-42; LIM-19 points to it | ADR-0040 |
| 5 | Keep the exporter | ADR-0037/0038 status lines record the confirmation; **LIM-41**; one measured pair (smoke seed 42, exporter on/off): gateways, orchestrators, poolers +10–65 % CPU, gateways/orchestrators ~7–8 MiB more peak memory; PostgreSQL, counts, median latencies, verdicts within run-to-run differences; timing under faults not measured | `exporter-ab.md`; records `exporter-ab-on/off/compare` |
| 6 | Split G2 | ADR-0042 (1.5.0): G2a (EXPLICIT; S48, S42), G2b (INFERRED; no public source). K4 records untouched; `reclassification.md`: the four G2 findings → **G2b / INFERRED / high, POSTGRES_INHERITED**, which cancel acted UNDETERMINED; LIM-33 | ADR-0042; `reclassification.md` |
| 7 | G6 finding; G10 | ADR-0043 (1.6.0): known status **`KNOWN`** (open upstream fix, cited; RESEARCH S53 = PR #1550); `KNOWN` packages are independent reproductions. **FND-C2-6806aeca** (G6, SAFETY_VIOLATION, DESIGN, **high** — §11.2 shown, POSTGRES_INHERITED, REPRODUCED 3/5, NEW), with completion review and timeline | ADR-0043; `findings/FND-C2-6806aeca.{md,json}` |
| 8 | Matrix file, `make campaign-k7` | `campaigns/k7.json` (130 runs); `make campaign-k7` (detached, log, pid, resumable), `campaign-k7-status`, `DRY_RUN=1` (`commitproof campaign --dry-run`: plans and run count, read-only). Dry run: 130 runs (V1 100, V2 15, V3 10, V5 5). Two real smoke runs through it; a second invocation started **0** runs. `prompts/K7.md` updated (diff below) | records `k7-dry-run-2`, `smoke-start`, `smoke-resume-wait`; `k7-prompt.diff` |
| 9 | Re-estimate | 130 runs; **~5.5 h** of matrix at measured durations (C12 193 s), ~6.4 h (S2) to ~11 h (S1) with re-runs and minimizations; bounds **0.259** (N = 10) / **0.451** (N = 5) per clean cell; evidence: above | `k7-estimate.md`; records `k7-estimate-4`, `k7bounds-2` |
| 10 | Hygiene | see "Hygiene" | records `lint-workflow`, `ci`, `precondition-2`, `exposure-check`, `evidence-size` |

**Found and resolved while projecting (ADR-0044, 1.7.0):** ADR-0036 counts a cell's §12.3 exposure only from runs held in full, so under decision 1 no summarized K7 cell could carry the clean statement — while K7.2 says exposure is recomputed "from `checks.json`". From 1.7.0, C9's stats carry the run's exposure (ADR-0036's definitions; never a verdict), cpreport counts it for summaries, and cpgate K7 is to cross-check it on runs held in full. Plans before 1.7.0 are unchanged: the recorded runs and corpora re-check byte for byte, and K6's campaign page re-renders to the same bytes (f4763186…).

## Hygiene (item 10)

- **CI-equivalent:** `make lint-workflow` and `make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke ci-mutant-smoke ci-report` exit 0 on the clean `43d6882` (records `lint-workflow`, `ci`, 03:15–03:31Z; runs carry `commitproof_dirty: false`, contract 1.7.0). k2check: 0 problems for the K2, K5 and K7 corpora (807, 157, 31 entries; 9, 12, 3 sabotage rules; elle-cli 350 compared, 0 disagreements); nemesis smoke F1a, F2a, F3a, F7, F4a; M1 via D1 FAIL with C1 `lost-write`; `ci-report` identical to the K6 example (35dc3e48…c94c). Images rebuilt by the CI run are locked (`build(images): lock the images of the K6b CI build`).
- **cpgate K1–K6:** PASS on copies at the start (`precondition`) and the end (`precondition-2`).
- **ADR-0044 on real runs:** C9's exposure equals an independent jq computation from the history on all 6 checked CI runs of contract 1.7.0 (`exposure-check`).
- **Contract version:** header 1.7.0 = `params.ContractVersion` (`TestContractVersion`, in `ci-unit`) = `images.lock.json`.
- **Sizes:** largest file 4 110 156 bytes; `evidence/` 220 896 126 bytes before this report (`evidence-size`), 29.1 MB left under P25.
- **git status:** clean after the final commit; nothing pushed.

## Findings K7 will package

| Finding | Guarantee | Class / level / severity | Origin | Reproducibility | Known status |
|---|---|---|---|---|---|
| FND-D1-fb8d0f8c, FND-D1-2e860fd2, FND-D1-a8eb9857 | G2b (recorded G2) | SAFETY_VIOLATION / INFERRED / **high** (recorded critical) | POSTGRES_INHERITED | REPRODUCED | NEW |
| FND-C2-2a534585 | G2b (recorded G2) | SAFETY_VIOLATION / INFERRED / **high** (recorded critical) | POSTGRES_INHERITED | INTERMITTENT | NEW |
| **FND-C2-6806aeca** (new) | G6 | SAFETY_VIOLATION / DESIGN / **high** | POSTGRES_INHERITED | REPRODUCED | NEW |
| FND-C9-40b6f05a, FND-C9-2fe9a9fc, FND-C9-90da5532 | G10 | PROTOCOL_DEVIATION / DESIGN / medium | MULTIGRES | REPRODUCED, REPRODUCED, INTERMITTENT | **KNOWN** (PR #1550) |
| FND-C11-17a344c1 | G12 | PROTOCOL_DEVIATION / DESIGN / medium | MULTIGRES | INTERMITTENT | NEW |

## Owner decision still open: P25 (decision 1's stop)

1. **Failures of recorded mechanisms stay summaries (S2), by ADR** — a K7 failure whose guarantee and anomaly class match a recorded finding and whose triage shows the same mechanism is counted, triaged and cited by that finding's package, not minimized or kept in full; new kinds are minimized and packaged in full. ~239 MB, ~6.4 h.
2. **Raise P25 to ~300 MB** — every failure minimized and packaged in full (K4/K5 practice): ~277–288 MB, ~11 h.
3. Keep only the minimized run of each new finding in full — changes decision 1's "runs a finding cites stay in full": ~253 MB, still over.
4. Fewer C9 runs (C9's failures are ~29 MB of S1) — a change of P15.

And, with any option: commit the cited D3′ and D5 real-build runs in full (11.8 MB) so they get clean statements, or report their exposure as "0 of 5 runs held in full".

## BLOCKED items

None under AGENTS.md §3 (no missing prerequisite, no irreversible or publishing action). The P25 projection is decision 1's own stop condition.

## Deviations

- **ADR-0044 was not asked for.** It resolves a contradiction the storage decision exposed (§18 lets me fix contradictions that relax nothing); review it with the P25 decision.
- **Four real runs, not two:** the two smoke runs of item 8 (made through `make campaign-k7`, they are also decision 3's smoke runs), plus the exporter measurement's pair (decision 5). All four are labelled not evidence; their summaries are under `smoke-not-evidence/` and `exporter-ab-not-evidence/`.
- The smoke runs' plans carry 1.6.0 (ADR-0044 came after them), so their `checks.json` has no exposure; the CI run's 1.7.0 runs show it (record `exposure-check`).
- The smoke capture's name filter also matched the `nem-gw-*` helpers (no slot flag, shown as `absent`).
- Kept failed records: `cited-recheck` (one checks.json held K4's oracle pass), `k7-estimate` (exit 126, not executable), `k7-estimate-2` (exit 141, SIGPIPE).
- FND-C2-6806aeca's minimized plan was minimized by K4 for `incompatible-order`; it reproduces `non-linearizable` in 3 of 5 recorded replays (§11.1's REPRODUCED). Its origin follows decision 6's ruling for the vanish mechanism.
- The pre-K7 feasibility note estimated 3–5 MB for summaries under policy A; the real plans make them 11.7 MB.

## Pending user actions

- Decide P25 (above); K7's prompt makes that decision, recorded as an ADR, its precondition.
- Keep `runs/k4-full/` and `runs/k5-full/` (the only full copies of the stubbed K4/K5 runs, among them the cited D3′ and D5 runs).
- Push, enable Actions and publish findings when you choose (nothing was pushed).

## prompts/K7.md diff

```diff
diff --git a/prompts/K7.md b/prompts/K7.md
index 5e45275..fa758b3 100644
--- a/prompts/K7.md
+++ b/prompts/K7.md
@@ -1,28 +1,35 @@
 # Sprint K7 — Full campaign, statistics and release
 
-You are Claude Code in the CommitProof repository. K1–K6 are DONE (verify closure files; re-run `cpgate K1`–`K6`; any failure → BLOCKED). Execute sprint K7 end to end in this session.
+You are Claude Code in the CommitProof repository. K1–K6 are DONE and the K6b readiness work is committed (verify the closure files and `evidence/K6b/readiness.md`; re-run `cpgate K1`–`K6` on copies of the committed evidence; any failure → BLOCKED). Execute sprint K7 end to end in this session.
 
 ## 0. Read first
 
 1. `AGENTS.md`.
-2. `docs/CONTRACT.md` — all of it (this sprint audits every claim against it), with emphasis on §1, §3, §4, §9.4, §11, §12, §13, §15 "K7", §16, §17, §19, §20.
-3. All ADRs; `docs/LIMITATIONS.md`; `docs/RESEARCH.md`.
-4. `evidence/K1/` … `evidence/K6/` NOTES and closure files.
+2. `docs/CONTRACT.md` (1.7.0) — all of it (this sprint audits every claim against it), with emphasis on §1, §3 (G2a/G2b), §4, §9.4, §10.10 (C9's exposure), §11 (known status `KNOWN`), §12, §13 (ADR-0039's storage), §15 "K7", §16, §17, §19, §20.
+3. All ADRs — in particular ADR-0037 (G12 outside fault windows), ADR-0038 (metrics exporter), ADR-0039 (K7 storage, cited cells), ADR-0040 (C13 and V4 removed), ADR-0041 (C12, `slotless`, `make campaign-k7`), ADR-0042 (G2a/G2b), ADR-0043 (`KNOWN`, the G6 finding, G10 packaging), ADR-0044 (exposure in `checks.json`); `docs/LIMITATIONS.md`; `docs/RESEARCH.md`.
+4. `evidence/K1/` … `evidence/K6/` NOTES and closure files; `evidence/pre-K7/` and `evidence/K6b/` (readiness, `reclassification.md`, `findings/`, `k7-estimate.md`, `exporter-ab.md`).
 
 ## 1. Objective
 
-Execute the final matrix of §15 K7 (V1–V5, mutants D1–D5 and P1, H1 if deployable), compute statistics, produce findings packages, audit every claim, correct overclaims, finalize public documentation, and create the local annotated tag `v1.0.0`.
+Run the cells of §15 K7 that K7 runs — V1 (C1–C7, C9, C10, C12 on `baseline`, 10 each), V2 (C1, C2, C4 on `slotless`, 5 each), V3 (C8 on `buffered`, 10), V5 (C11 on `baseline`, 5): 130 runs, `campaigns/k7.json` — and cite the directed cells K4 and K5 hold (ADR-0039: D1, D2′, D3′, D5 and P1, each on its mutant and on the real build, and DH1; with the contract versions their plans carry; **not re-run**). C13 and V4 do not exist (ADR-0040). Compute statistics, produce findings packages, audit every claim, correct overclaims, finalize public documentation, and create the local annotated tag `v1.0.0`.
 
 Requirements:
 
-- Write the matrix file and run it with `make campaign-k7` (wrapping `commitproof campaign --matrix …`), in the background with a log, polling progress; it is resumable — if the session is interrupted, re-invoking skips sealed runs. INVALID runs are re-run with the same seed; persistent INVALID (3×) is a HARNESS_DEFECT to fix (fix, add a regression test, re-run affected runs).
-- Statistics per §12.3 for every cell (k/N, Wilson 95% interval or zero-failure bound, exposure), computed by `cpgate` from `checks.json` and matching `cpreport`.
-- Every C3 anomaly confirmed with elle-cli or labelled `oracle-disagreement` and triaged (K7.3).
-- `findings/<id>/` package for every finding per §11.4, in upstream issue format (S50) — written, not published. Re-execute each package's replay command once and record the result.
+- Before the first run: the owner's decision on the projected evidence size (`evidence/K6b/k7-estimate.md` §4, `readiness.md`: under decision 1 the projection exceeds P25 unless failing runs of recorded mechanisms are kept as summaries, or P25 is raised) must be recorded as an ADR, with the matrix file's `minimize_failures` and the store step following it. If it is not, stop BLOCKED with that as the owner action. Do not bump the contract version while the matrix runs: plans carry it, and sealed runs of older plans no longer match.
+- Run the matrix with `make campaign-k7` (it wraps `commitproof campaign --matrix campaigns/k7.json --out runs/k7`, detached, log `runs/k7/campaign-k7.log`); check it first with `make campaign-k7 DRY_RUN=1` (130 runs); poll with `make campaign-k7-status`; it is resumable — if the session is interrupted, re-invoking skips sealed runs. INVALID runs are re-run with the same seed; persistent INVALID (3×) is a HARNESS_DEFECT to fix (fix, add a regression test, re-run affected runs). The driver refuses to run while `CP_OTEL_METRICS_EXPORTER` is set: K7's runs keep ADR-0038's exporter (LIM-41: K4/K5 runs did not have it).
+- Storage per ADR-0039: runs a claim, a finding package or a report names as its artifact are committed in full, once; every other run as a summary (stub archive and `checks.json`, `scripts/store-summaries.sh`) listed with its seal root in `evidence/K7/runs-index.json`; P25 counts `evidence/` and the run archives under `findings/`; the store step refuses to exceed P25.
+- Statistics per §12.3 for every cell, the cited ones included (k/N, Wilson 95% interval or zero-failure bound, exposure), computed by `cpgate` from `checks.json` and matching `cpreport`. K7's runs carry their exposure in C9's stats (ADR-0044); `cpgate K7` cross-checks it against the history of every run held in full. The cited K5 stubs (D3′, D5) predate ADR-0044: their exposure is "0 of 5 runs held in full" unless the owner's decision commits their full archives. V5 reports both G12 fractions (ADR-0037). State each cited cell's contract version and that it ran without the exporter (LIM-41).
+- Every C3 anomaly confirmed with elle-cli or labelled `oracle-disagreement` and triaged (K7.3) — including the `incompatible-order` anomalies of M1's five cited D1 runs, which K4 did not cross-check.
+- Every real-build FAIL triaged (checker output, §6.2 completion review, oracle for C3, classification by §11 with G2a/G2b per ADR-0042).
+- `findings/<id>/` package for every finding per §11.4 — written, not published — and each package's replay command executed once more with the result recorded. The findings to package (plus any K7 finds):
+  - FND-D1-fb8d0f8c, FND-D1-2e860fd2, FND-D1-a8eb9857, FND-C2-2a534585 — **G2b**, SAFETY_VIOLATION, INFERRED, **high**, origin POSTGRES_INHERITED (which cancel acted: UNDETERMINED), as `evidence/K6b/reclassification.md` states (ADR-0042); each package shows the recorded classification (G2, EXPLICIT, critical) beside it.
+  - FND-C2-6806aeca — **G6**, SAFETY_VIOLATION, DESIGN, **high** (§11.2 shown), POSTGRES_INHERITED, REPRODUCED, NEW; cross-referenced to the G2b findings' shared mechanism (`evidence/K6b/findings/FND-C2-6806aeca.md`, ADR-0043).
+  - FND-C9-40b6f05a, FND-C9-2fe9a9fc, FND-C9-90da5532 — G10, PROTOCOL_DEVIATION, DESIGN, medium, known status **KNOWN** (upstream PR #1550, S53), each packaged as an independent reproduction supporting that pull request, not as a new issue (ADR-0043).
+  - FND-C11-17a344c1 — G12, PROTOCOL_DEVIATION, DESIGN, medium, as recorded under the literal rule (owner decision D-B), with both fractions stated (literal 766/886 in storm window 1; outside fault windows 744/744).
 - `claims.yaml` per §17 covering every claim in README, LIMITATIONS, NOTES (K7) and findings; statements lacking a field are rewritten as hypotheses.
 - `evidence/K7/overclaim-review.md`: every statement changed from earlier drafts and why.
-- Public `README.md`: first paragraph says CommitProof searches for counterexamples and reports bounded evidence (not proofs); how to reproduce (prerequisites, `make` targets); the §4 matrix verbatim with links to evidence; results per cell using the §12.3 sentence for clean cells; mutant results side by side with real-build results; H1 outcome in one line; findings list with §11 dimensions; limitations summary linking `docs/LIMITATIONS.md`. No claim without a `claims.yaml` id.
-- Final `docs/LIMITATIONS.md` (resolve every PENDING row), `CHANGELOG.md`, `evidence/K7/runs-index.json` (seal roots of all runs; P25 budget policy of §13).
+- Public `README.md`: first paragraph says CommitProof searches for counterexamples and reports bounded evidence (not proofs); how to reproduce (prerequisites, `make` targets); the §4 matrix verbatim with links to evidence; results per cell using the §12.3 sentence for clean cells; mutant results side by side with real-build results (the cited cells, with their contract versions); H1 outcome in one line; findings list with §11 dimensions; limitations summary linking `docs/LIMITATIONS.md`. No claim without a `claims.yaml` id.
+- Final `docs/LIMITATIONS.md` (resolve every PENDING row), `CHANGELOG.md`, `evidence/K7/runs-index.json` (seal roots of all K7 runs; ADR-0039's storage).
 - Extend `cpgate` with `K7`; run all `make ci-*` targets; create annotated tag `v1.0.0` locally on the final commit. Do not push.
 
 ## 2. Gate
@@ -35,12 +42,12 @@ Pass requires K7.1–K7.7 and negative controls failing exactly their targets. R
 
 ## 3. Closure specifics
 
-In the closure report, `pending_user_actions` must list: pushing the repository and tags (owner's decision), enabling GitHub Actions and recording the first remote CI run in LIMITATIONS LIM-16, and deciding whether to publish each finding upstream.
+In the closure report, `pending_user_actions` must list: pushing the repository and tags (owner's decision), enabling GitHub Actions and recording the first remote CI run in LIMITATIONS LIM-16, and deciding whether to publish each finding upstream (for G10: whether to offer the reproduction on PR #1550).
 
 ## 4. Stops
 
 - Allowed: DONE or BLOCKED per AGENTS.md §3.
-- Forbidden: stopping midway to ask or offer (campaign waiting is done by polling, not by stopping); reducing repetitions or dropping cells to save time; counting INVALID runs; publishing, pushing, opening issues, or contacting upstream; wording any result more strongly than its evidence.
+- Forbidden: stopping midway to ask or offer (campaign waiting is done by polling, not by stopping); reducing repetitions or dropping cells to save time; re-running the cited directed cells (ADR-0039); counting INVALID runs or the K6b smoke runs; publishing, pushing, opening issues, commenting on upstream pull requests, or contacting upstream; wording any result more strongly than its evidence.
 
 ## 5. Finish
 
```
