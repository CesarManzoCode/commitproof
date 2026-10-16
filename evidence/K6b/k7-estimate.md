# K7 re-estimate (K6b, item 9) — runs, hours, evidence under decision 1, bounds

Contract 1.7.0. Inputs are measurements, each a record in `evidence/K6b/commands.jsonl`: `k7-dry-run-2` (the matrix, `make campaign-k7 DRY_RUN=1`), `k7-estimate-4` (`scripts/k6b-k7-estimate.sh`: every K7 plan's summary size, durations, bytes per K4/K5 finding, evidence size), `k7bounds-2` (§12.3 bounds), `smoke-store` and `exporter-ab-store-*` (archive sizes of runs with the exporter), and pre-K7's `durations`. What is projected (FAIL counts) is labelled as such.

## 1. Runs

`campaigns/k7.json` (ADR-0039–ADR-0041), dry run `k7-dry-run-2`: **130 runs** — V1 100 (C1–C7, C9, C10, C12 × seeds 1–10, `baseline`), V2 15 (C1, C2, C4 × 1–5, `slotless`), V3 10 (C8 × 1–10, `buffered`), V5 5 (C11 × 1–5). The directed cells are cited from K4/K5 (55 valid primaries, ADR-0039), V4 is removed (ADR-0040). Plans carry contract 1.7.0: a contract bump during the matrix would change every remaining plan, and sealed runs of the older plans would no longer match (K7 must not bump the contract while the matrix runs).

## 2. Hours (measured per-run wall time)

Seconds per run: pre-K7's medians of K4/K5 (record `durations`, start to next start): C1 137, C2 140, C3 156, C4 141, C5 155, C6 138, C7 150, C8 198, C10 156, C11 138; C9 150 (its primaries; the 87 s median mixes in short minimization candidates); **C12 193 s** and **C4 on `slotless` 139 s** from the K6b smoke runs (first to last command; one run each; C12 seed 1001 drew a 60 s pause).

| Cell | Runs | Seconds | Hours |
|---|---|---|---|
| V1 | 100 | 10 × (137 + 140 + 156 + 141 + 155 + 138 + 150 + 150 + 156 + 193) = 15 160 | 4.21 |
| V2 | 15 | 5 × (137 + 140 + 139) = 2 080 | 0.58 |
| V3 | 10 | 10 × 198 = 1 980 | 0.55 |
| V5 | 5 | 5 × 138 = 690 | 0.19 |
| **Matrix** | **130** | **19 910** | **5.5** |

Plus INVALID re-runs (K4/K5: about 10 % → ~6.1 h) and the minimization and 5 replays of every real-build failure that is minimized: 15–22 min each in K4/K5 (median ~19 min; 3–7 candidates + 5 replays). With the failures projected in §4, scenario S1 adds ~5.1 h (16 × 19 min; **~11 h in all**); scenario S2 adds ~0.3 h per new failure (**~6.4 h**).

## 3. Bound per cell (§12.3, record `k7bounds-2`)

| Cell | N | If clean (k = 0) | Projected k (K4/K5 rates) → Wilson 95 % |
|---|---|---|---|
| V1: C1, C3, C4, C5, C6, C7, C10, C12 (each) | 10 | bound **0.259** | C12: no earlier rate |
| V1: C2 | 10 | 0.259 | ~3 (K4: 1 of 3) → [0.107, 0.604] |
| V1: C9 | 10 | 0.259 | ~10 (K5: 3 of 3, G10) → [0.722, 1.000] |
| V2: C1, C4 (each) on `slotless` | 5 | bound **0.451** | — |
| V2: C2 on `slotless` | 5 | 0.451 | ~2 → [0.117, 0.770] |
| V3: C8 on `buffered` | 10 | 0.259 | (K5: 0 of 3) |
| V5: C11 | 5 | 0.451 | (K5: 1 of 3 under the literal rule; ADR-0037's rule would not have failed it) |
| Cited, real build: D2′, D3′, D5, P1 (each), DH1 | 5 | **0.451** (0 of 5 each) | recorded |
| Cited, real build: D1 | 5 | — | recorded 3 of 5 → [0.230, 0.883] |
| Cited, mutants: M1 (D1), M4 (P1), M5 (D5) | 5 | — | recorded 5 of 5 → [0.565, 1.000], DETECTED |
| Cited, mutants: M2 (D2′), M3 (D3′) | 5 | — | recorded 4 of 5 → [0.375, 0.964], DETECTED |

A clean cell's statement also needs its exposure: from contract 1.7.0 every K7 run's `checks.json` carries it (ADR-0044), so summarized cells qualify. The cited D3′ and D5 real-build cells are K5 stubs whose `checks.json` predates ADR-0044: their exposure needs their full archives (§4, item D) or is reported as "0 of 5 runs held in full" without the clean template.

## 4. Evidence under decision 1 (ADR-0039, ADR-0044)

`evidence/` now: **220 850 611 B** (`k7-estimate-4`); the rest of K6b's records add a few hundred KB. P25 = 250 MB: **~29 MB left for K7**.

| Item | Size | Basis |
|---|---|---|
| A. Summaries of the 130 runs (stub + `checks.json`) | **11.66 MB** | every K7 plan generated and compressed (zstd -19): 75–95 KB per plan (C12 92 KB, C9 95 KB), + 6.2 KB manifest/seal (smoke stubs) + 3.4 KB `checks.json` (K4/K5 mean) |
| B. K7's own records, notes, gate, campaign page | ~1.5–5 MB | K5 1.7 MB, K4 5.1 MB outside their runs |
| C. Cited runs of **new findings** (original and minimized in full; candidates and replays as summaries) | ~1.7 MB (C2-like), ~2.9 MB (C9-like), ~3.2 MB (C12-like) per finding | K4/K5 findings: originals 0.67–1.95 MB, minimized 0.19–0.61 MB, 3–7 candidates + 5 replays; with the exporter a C12 run's full archive is 2.19 MB (smoke) |
| D. The cited D3′ and D5 real-build cells in full (only for their clean statements) | 11.83 MB | their archives in `runs/k5-full/` (8.28 + 3.55 MB) |
| The 9 recorded findings' packages | 0 MB | their runs are already committed (K4, K5); packages refer to them; text is outside P25 |

Projected real-build failures, from K4's and K5's rates on the same plans' generators (a projection, not a measurement): C9 ~10 of 10 (G10's deterministic mechanism), C2 ~3 of 10 on `baseline` and ~2 of 5 on `slotless` (the G2b/G6 vanish mechanism under F1c), C12 unknown (one assumed), C11 ~0 under ADR-0037, every other cell 0 (K4/K5: 0 of 3–5). **About 16 failures.**

By §11.3 every minimized failure is a finding with its own id (K5 recorded three G10 ids for three C9 failures), and by decision 1 its original and minimized runs stay in full:

| Scenario | evidence/ at K7's end | Fits 250 MB? |
|---|---|---|
| **S1 — every real-build failure minimized and packaged (K4/K5 practice)**, without D | 220.9 + 0.3 + 11.66 + ~3 + (10 × 2.9 + 5 × 1.7 + 1 × 3.2 = 40.7) ≈ **276.6 MB** | **no** (+26.6 MB) |
| S1 with D | ≈ **288.4 MB** | **no** (+38.4 MB) |
| S2 — failures that repeat a recorded finding's guarantee and anomaly class (triaged as the same mechanism) are counted and triaged but kept as summaries; only new kinds are minimized and packaged; without D | 220.9 + 0.3 + 11.66 + ~3 + 1 × 3.2 ≈ **239.1 MB** | yes (~11 MB margin: ~3 more new findings) |
| S2 with D | ≈ **250.9 MB** | no (marginal) |

**Result: under decision 1 the projection does not fit P25 unless failures of already-recorded mechanisms are kept as summaries (S2).** Per decision 1 this is the owner's decision; none is chosen here.

Options for the owner:

1. **S2 by ADR** (a K7 rule the contract leaves open: K7.4 packages "every finding", §11.3 gives each minimized plan its own id): a real-build failure whose guarantee and anomaly class match a recorded finding, and whose triage shows the same mechanism, is reported in its cell's k/N and triage and cited in that finding's package, but not minimized and not committed in full; new kinds are minimized, replayed and packaged in full. ~239 MB; ~6.4 h. Cost: no minimized plan or reproducibility class for the recurrences (they are not new findings).
2. **Raise P25** to about 300 MB (as ADR-0034 did from 200 to 250 MB): S1 with or without D fits; ~11 h.
3. **S1 with fewer failing runs held in full** (e.g. the minimized run in full, the original as a summary): changes decision 1's "runs that a finding cites stay in full"; S1 becomes ~253 MB — still over.
4. Fewer C9 runs (C9 dominates S1: ~29 MB) — a change of P15 the owner would have to approve.

Separately (any option): whether the cited D3′ and D5 real-build cells get their clean statements (item D, 11.83 MB) or report "0 of 5 runs held in full".

Not counted above: re-runs of INVALID runs as summaries (~0.1 MB each), any K7 harness-defect fixes' runs.

Sources: CONTRACT §11.3, §11.4, §12.2, §12.3, §13, §15 K7, P15, P25; ADR-0034, ADR-0036, ADR-0039, ADR-0040, ADR-0041, ADR-0044; evidence/pre-K7/k7-feasibility.md; evidence/K4/results.json, evidence/K5/results.json.
