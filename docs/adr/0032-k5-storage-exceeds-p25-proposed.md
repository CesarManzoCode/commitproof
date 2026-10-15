# ADR-0032 — K5's runs do not fit P25 under ADR-0030's storage (proposed; owner decision)

Status: **Decided by the owner on 2026-10-14: option 1, P25 raised to 250 MB, applied by ADR-0034** (after lossless compression was measured and found insufficient). The text below is the proposal as written; it is kept unchanged.

Original status: **Proposed (K5) — not applied.** K5 stops BLOCKED (with K5.4, ADR-0031). The contract's evidence policy cannot be met as written: the runs that CONTRACT §13 and ADR-0030 require in git **in full** (and that cpgate K5.3 and K5.5 read in full) do not fit in what P25 leaves. Meeting one requirement means relaxing the other — raising P25 (a P-value, §5.7), or committing cited runs as stubs (gate criteria then cannot re-read them from git) — and either is a relaxation that needs the owner (§18). The contract, P25, ADR-0030 and the gate are unchanged. This ADR gives the measurements and the options; the owner decides.

## Context (measurements)

- **P25**: ≤ 200 MB under `evidence/` in git. At K4's close 183.1 MB were used (K4 closure: "about 17 MB left for K5–K7"); measured again on resume: `git ls-files evidence | xargs stat -c %s` = 183 082 177 bytes, so 16.92 MB remain for K5, K6 and K7 together. K6 commits a reference run in full (§15 K6) and K7 its matrix (§13).
- **ADR-0030's full runs**: every C10 primary run (5; K5.3 recomputes the slot's readiness from its observations) and the primary and minimized runs of every real-build FAIL (C9 seeds 1–3 and their three minimized runs; C11 seed 2, its own minimum; K5.5's triage reads their histories): 12 archives, 19.98 MB (zstd -19 tars as `scripts/k5-store-runs.sh` writes them; per run in the table below).
- **Stubs** (manifest, plan, seal) of the other 144 of K5's 156 runs: the stubs of all 156 take 7.71 MB (~49 KB each; the plans dominate), every run's checks.json and the minimization journals 0.58 MB. Non-run K5 evidence (records, logs, corpus results, sabotage results, triage, NOTES, lock and patches copies): 1.06 MB.
- Totals per policy (records `stage-<policy>-2`: `scripts/k5-stage-storage.sh` builds each copy outside `evidence/` from the committed stubs and the local full archives; the first attempts `stage-<policy>` selected nothing — a script defect fixed in `c716bcc` — and stay recorded):

| Policy (record) | Runs not stubs | K5 under `evidence/` | `evidence/` in git | P25 (200 MB) |
|---|---|---|---|---|
| ADR-0030 (`stage-adr0030-2`) | 12 full | 28.49 MB | 211.57 MB | exceeded by 11.57 MB |
| ADR-0030 without the C10 primaries — option 2 (`stage-adr0032-option2-2`) | 7 full | 19.40 MB | 202.48 MB | exceeded by 2.48 MB |
| ADR-0030's set as partial archives without `logs/` — option 4 (`stage-partial-2`) | 12 partial | 25.52 MB | 208.60 MB | exceeded by 8.60 MB |
| every run a stub — option 3 (`store-runs`, committed) | 0 | 9.34 MB (at the store step) | 192.42 MB | fits; 7.58 MB left |

Full archives of ADR-0030's set (bytes; `stage-adr0030-2.stdout`): C9 seeds 1–3 1 807 228, 1 871 499, 1 947 160; their minimized runs 611 286, 292 289, 492 329; C10 seeds 1–5 1 831 507, 1 885 171, 1 964 274, 1 908 542, 1 919 113; C11 seed 2 3 450 920 — 19 981 318 in all. As partial archives the same twelve take 17 004 019 (`stage-partial-2.stdout`): the container logs are only ~15 % of a compressed run; the histories dominate.

Only the all-stub policy fits what is left, and it leaves 7.58 MB for K6 and K7.

## Options

1. **Raise P25** (e.g. to 250 MB) and commit ADR-0030's storage. Keeps every gate criterion re-checkable from git; changes a P-value, which K1's "do not change" list protects. The largest K5 archive is 3.45 MB.
2. **ADR-0030's storage with the C10 primaries as stubs** (`k5-store-runs.sh … adr0032-option2`): the real-build failures stay re-checkable; K5.3 could no longer be recomputed from git (its criterion would need to read a committed extract of the `cdc-slot` observations, a gate change).
3. **All runs as stubs** (`… stubs`; what this commit holds as the interim): fits; K5.3 and K5.5 cannot be re-derived from git (cpgate K5 on the committed evidence fails them), the full archives stay on the owner's machine (`runs/k5-full/`, as LIM-30 for K4's derived runs); a staged copy with ADR-0030's storage is evaluated by the same cpgate to show the result the full archives give.
4. **Partial archives for the cited runs** (history, observations, manifest, plan, seal, commands; container logs kept locally): every file the gate reads stays in git and is verified against the seal's file list, but the runs are no longer "committed sealed" in full (§13), and the container logs that NOTES cites for mechanisms (e.g. the pooler's `pubsub: reader error`) leave git. Size: 17.00 MB for the twelve (25.52 MB for K5).
5. **Full archives outside git** (an LFS store or a release asset): publication, the owner's alone (AGENTS.md §6).

Not options: summarising closed sprints' committed runs (sealed evidence is immutable; K1–K4 are closed). Higher zstd levels than -19 were not measured; they would have to save more than 11.6 MB of 28.5 MB.

Recommendation for the owner's consideration: option 1 — P25 raised to 250 MB, enough for ADR-0030's storage (211.6 MB) plus K6's reference run — because §13 and §17 rest on cited runs being re-checkable from git; with any option, K7's matrix (165 runs) needs §13's summarising policy, since even its stubs (~49 KB each, the plans dominate) take ~8 MB.

## Consequences if an option is approved

The store step is re-run on the local full archives with the approved policy (every run already sealed; nothing is re-run), results.json and the gate re-recorded, and the closure updated; nothing else in K5 changes. Until then the committed evidence is the all-stub interim and LIM-37 states it.

## Sources

CONTRACT §5.7 (P25), §13, §15 K5, §17, §18; ADR-0010, ADR-0030; evidence/K4/closure.yaml (budget at K4's close); evidence/K5/commands.jsonl (`store-sizes`, `store-runs`).
