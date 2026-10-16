# K7 feasibility (pre-K7 readiness, step 2) — no K7 run made

Sources: `prompts/K7.md`; CONTRACT 1.2.1 §4, §9.1, §12.3, §12.4, §13, §15 K7, P15, P25; records in `evidence/pre-K7/commands.jsonl`: `durations` (wall time per run, K4/K5), `k7bounds` (§12.3 bounds), `evidence-size` (P25), `scrape-measure` (scraper cost). Design says / system did are kept apart: the matrix is what the contract says; durations and sizes are what K4–K6 and the pre-K7 smoke run measured.

## 1. The matrix (CONTRACT §15 K7, P15)

| Cell | Campaigns | Profile | N per campaign | Runs | Implemented at `0c45a28`? |
|---|---|---|---|---|---|
| V1 baseline | C1, C2, C3, C4, C5, C6, C7, C9, C10, C12 | baseline | 10 | 100 | C1–C7, C9, C10 yes; **C12 no** (not in `internal/plan`) |
| V2 slotless | C1, C2, C4 | slotless | 5 | 15 | **no** (`slotless` has no compose file in `internal/cluster`) |
| V3 buffered | C8 | buffered | 10 | 10 | yes |
| V4 replica reads | C13 (W1 read-only transactions through the replica port, primary-port writes; no nemesis and with F2a) | replica-port | 5 | 5 | **no** (neither C13 nor the `replica-port` compose file exist) |
| V5 overload | C11 | baseline | 5 | 5 | yes (judged by ADR-0037's rule from contract 1.2.0) |
| Mutants | D1 (M1), D2′ (M2), D3′ (M3), P1 (M4), D5 (M5) | as defined | 5 | 25 | yes (images built and locked) |
| Real build on the same directed schedules (§9.1, §12.4) | D1, D2′, D3′, P1, D5 | as defined | 5 | 25 | yes |
| H1 | DH1 | h1 | 5 | 5 | yes |
| **Total** | | | | **190** | |

Two points the contract leaves open (owner, or K7's own ADR before any run):

- §15 K7 counts "165 runs"; that total omits the 25 real-build runs that §9.1 ("the real build runs the same directed schedule 5 times") and §12.4 ("side by side") require. With them the matrix is **190 runs**.
- V4 names "no nemesis and with F2a" with 5 runs: either 5 runs in total (how many of each?) or 5 per variant (10).

Not implemented, and needed before the matrix can run (each with its schedule fixed by an ADR before any run, as ADR-0024/0030 did): campaign **C12** (W1–W4; faults drawn uniformly from F1a–F1d, F2a–F2e, F3a, F3b, F4a, F6, F7, plus one F5a/F5b pair per run: counts, holds and gaps are undefined); campaign **C13** and its replica-port read client (and how C3/C4 judge replica reads, LIM-19); compose profiles **`slotless`** (pooler and gateway `--enable-slot-based-replication=false`) and **`replica-port`** (gateways `--pg-replica-port=15433`); the K7 matrix file and `make campaign-k7`; `cpgate K7` (a K7 deliverable). `prompts/K7.md` assumes the campaigns exist.

## 2. Expected hours (measured wall time per run)

Record `durations` (`scripts/prek7-durations.sh`): a run's wall time is the time from its start to the next run's start in K4/K5 (cluster up, workload, recovery, final reads, teardown, cpcheck), gaps over 15 min dropped; 428 runs, overall mean 136 s. Medians used below (s): C1 137, C2 140, C3 156, C4 141, C5 155, C6 138, C7 150, C8 198, C10 156, C11 138, D1 94, D2′ 177, D3′ 136, P1 46, D5 149, DH1 171. C9's median (87 s) is dominated by short minimization candidates; 150 s is used for its primaries (its mean is 100 s, its maximum 186 s). C12 and C13 have no measurement: 180 s and 140 s are assumed (C12 adds n4's start and removal).

| Cell | Runs | Seconds | Hours |
|---|---|---|---|
| V1 | 100 | 15 030 | 4.18 |
| V2 | 15 | 2 090 | 0.58 |
| V3 | 10 | 1 980 | 0.55 |
| V4 | 5 (10) | 700 (1 400) | 0.19 (0.39) |
| V5 | 5 | 690 | 0.19 |
| Mutants | 25 | 3 010 | 0.84 |
| Real build, directed | 25 | 3 010 | 0.84 |
| H1 | 5 | 855 | 0.24 |
| **Matrix** | **190** | **27 365** | **7.6** |

Plus re-runs of INVALID runs (K4/K5: 12 INVALID primaries, mostly C5 before ADR-0028; +10 % → about 8.4 h) and minimization and replay of real-build failures (§11.4 packages need a minimized plan and replays; a K4 failure took about 12–25 runs of ~100 s). If every expected real-build FAIL primary (section 4) is minimized, add 5–11 h: **about 8–19 h in total**, on one cluster at a time, with the resumable campaign driver in the background. No deadline is stated; time fits. (The contract's own estimate was ~4 min per run, 11 h for 165 runs; the measured runs are shorter.)

## 3. §12.3 bounds per cell

Record `k7bounds` (`internal/stats`, ADR-0036 integer arithmetic). Zero-failure one-sided 95 % bound `1 − 0.05^(1/N)`: **N = 10 → 0.259**, **N = 5 → 0.451**. Wilson 95 % intervals for k violations:

| k | N = 5 | N = 10 |
|---|---|---|
| 0 | [0.000, 0.435] | [0.000, 0.278] |
| 1 | [0.036, 0.625] | [0.017, 0.405] |
| 2 | [0.117, 0.770] | [0.056, 0.510] |
| 3 | [0.230, 0.883] | [0.107, 0.604] |
| 4 | [0.375, 0.964] | [0.168, 0.688] |
| 5 | [0.565, 1.000] | [0.236, 0.764] |
| 6–10 | — | [0.312, 0.832], [0.396, 0.893], [0.490, 0.944], [0.595, 0.983], [0.722, 1.000] |

V1 and V3 cells get N = 10 (bound 0.259 if clean); V2, V4, V5, every mutant and real-build directed cell, and H1 get N = 5 (0.451). Mutant detection is ≥ 4 of 5 (P14), not a bound.

## 4. Evidence under P25 = 250 MB

Record `evidence-size`: `evidence/` holds **219 778 748 bytes** (K1 9.66, K2 13.85, K3 47.50, K4 112.07, K5 32.26, K6 1.68, pre-K7 2.75 MB): **30.22 MB remain**, of which about 1 MB is reserved for the rest of this readiness work. Largest file in the repository: 4.1 MB (no file near 50 MB).

Measured costs per run (K4/K5 committed evidence; record `scrape-measure`):

- full archive of a 90 s campaign run: 1.16–1.90 MB (K4 C1–C7 means 1.16–1.70 MB, K5 C10 1.90 MB, C11 3.45 MB; D1 0.57 MB, DH1 0.44 MB);
- the scraper adds 27 759 bytes per run-minute at zstd -19 (174 919 uncompressed), about 70 KB for a K7 run (~2.5 min from setup to final reads);
- a stub (manifest, plan, seal): 27 KB (K4) to 49 KB (K5); `checks.json`: 3.5 KB.

Expected real-build FAIL primaries in K7, from K4/K5 rates: C2 1 of 3 → ~3 of 10; D1 3 of 5 → ~3 of 5; C9 3 of 3 (G10, a deterministic mechanism) → ~10 of 10; C11 under ADR-0037 ~0 (the literal rule would give ~2 of 5); C12, C13 unknown (C13's replica reads are expected to show PostgreSQL's visibility anomalies, LIM-19). **About 16, plus C12/C13.** Mutant runs fail by design (~22 of 25).

| Storage policy | evidence/K7 estimate | Fits the ~29 MB left? |
|---|---|---|
| **A. §13 literally:** every run's `checks.json` (190 × 3.5 KB = 0.67 MB) and its seal root in `runs-index.json`; in full only the runs a K7 claim cites in full (e.g. one report reference, ~1.5 MB); the original and minimized runs of findings in `findings/<id>/` (§11.4), which is not under `evidence/` | 3–5 MB | **yes**, if the owner confirms that `findings/` is outside P25 and that uncited runs need no stub |
| **B. K4/K5 practice in evidence/K7:** every run at least a stub (190 × ~40 KB = 7.6 MB) and checks (0.67 MB), every real-build FAIL primary in full (~16 × ~1.6 MB = 25.6 MB) and the minimized run of each new finding (~4 × 0.8 MB = 3.2 MB), reports and notes (~1 MB) | ~38 MB (more with C12/C13 failures) | **no** |
| B, with `findings/` counted toward P25 | B + the finding runs again | **no** |

## 5. Result: owner decision (not chosen here)

**Time fits; P25 fits only under policy A, which needs the owner's reading of §13/§11.4.** Per the task, the choice is the owner's; none is made here. Options:

1. **Policy A** (§13 as written): summarize uncited runs (checks + seal roots), keep failing runs' full copies only in `findings/`, and state that `findings/` is outside P25. Cost: K7's gate re-checks statistics from `checks.json` only; stubs and full histories of uncited runs stay on the machine (LIM-30/37-style row).
2. **Fewer cells** (e.g. drop V2 or V4): saves stubs (~0.2 MB per 5 runs) but not the failing-run storage, which C9, C2 and D1 dominate.
3. **Smaller N** (e.g. V1 at N = 5): about halves the expected FAIL primaries (B ≈ 25 MB, marginal); clean V1 cells would get 0.451 instead of 0.259.
4. **Raise P25** (e.g. to 300 MB, as ADR-0034 did to 250 MB): policy B fits with margin.

Independently of the storage decision, K7 needs C12, C13, the two profiles, the matrix file and `make campaign-k7` implemented and their schedules fixed by ADR before its first run, and a decision on the two open counts (190 vs 165 runs; V4's 5 vs 10).
