# ADR-0039 — K7's evidence storage under P25 (§13 literally) and the directed cells K7 cites from K4 and K5

Status: **Accepted by the owner (K6b, contract 1.3.0).** Applies the owner's decisions 1 and 2 of 2026-10-15. Written and committed before any K7 run, before the K7 matrix file and before `make campaign-k7`.

## Owner decisions (verbatim)

> 1. P25: apply CONTRACT §13 literally: runs no claim, finding or cited cell uses are committed as summaries (manifest, plan, seal, checks.json); runs that a finding or a claim cites stay in full. The text files of findings/ do not count toward P25; any run archive inside a finding package does. Measure and report the projected evidence size; if it does not fit 250 MB, stop with options, do not choose.
>
> 2. K7's matrix: do not re-run the directed cells (mutants D1–D5, real directed runs, claims/P1, H1): K4 and K5 hold them. Check every K7.x criterion in §15 and cpgate: if one needs those cells under the final contract version, stop and report which; otherwise K7 cites the K4/K5 runs (with their contract versions) and runs only the new cells.

## Context

- §13 says every run cited by a claim is committed sealed under `evidence/`, runs not cited are summarized in `evidence/K7/runs-index.json` with their seal roots, and P25 bounds `evidence/` at 250 MB. K4 and K5 committed stubs (manifest, plan, seal) with `checks.json` beside them for the runs no gate criterion reads in full (LIM-30, LIM-37; ADR-0030, ADR-0034). §11.4 puts each finding's package, with "the original and minimized run directories (sealed)", under `findings/`, outside `evidence/`, and P25 did not say whether those count.
- `evidence/` held 220 343 926 bytes when the pre-K7 readiness work closed (`evidence/pre-K7/readiness.md`): 29.66 MB left.
- §15 K7's matrix lists "Mutants D1–D5 (and P1 for M4)" and "H1" with 5 runs each, and the pre-K7 feasibility note (`evidence/pre-K7/k7-feasibility.md` §1) added the 25 real-build runs of the same directed schedules that §9.1 and §12.4 require. K4 and K5 already hold all of them, valid and checked (below).

## Decision

### 1. Storage (§13, P25)

- A K7 run is **cited** when a claim in `claims.yaml`, a finding package or a K7 report names that run itself as its artifact: a finding's original and minimized runs (§11.4), a report's reference run, a run whose history a stated number is computed from (for example the exposure of a §12.3 clean statement, ADR-0036). A run that only contributes its verdict and classes to a cell's statistics is **used by the cell**: the cell's artifact is its runs' `checks.json` files and seal roots (K7.2 recomputes the statistics from `checks.json`).
- Cited runs are committed **in full**, once, in the §13 archive form (`<run-id>.tar.zst`, sealed by the hashes of the uncompressed files). A finding package refers to the committed archive by path and sha256; a copy of a run archive inside `findings/` is allowed and counts toward P25 like any archive.
- Every other K7 run is committed as a **summary**: its stub archive (`manifest.json`, `plan.json`, `seal.json`; the form of ADR-0030) with its `checks.json` beside it, and a row in `evidence/K7/runs-index.json` with its seal root. Its full archive stays on the machine that ran it, as LIM-30 and LIM-37 state for K4 and K5.
- **P25 counts** every byte under `evidence/` and every run archive under `findings/`. The text files of `findings/` (README, `claim.yaml`, `plan.json`, `checks.json`, report excerpts) do not count.
- The cells K7 cites (below) stay as K4 and K5 committed them. A K7 claim that needs the history of a cited cell's stub run (the exposure of a clean statement for D3′ or D5 on the real build, whose runs K5 committed as stubs) either commits that run's full archive from `runs/k5-full/` — byte-identical to the sealed run, its seal checked against the stub's — as a cited run, or reports the exposure as "m of N runs held in full" (ADR-0036) without the clean template.
- The K7 store step refuses to write evidence that would exceed P25, as K5's did. The projected size is measured before K7 (`evidence/K6b/k7-estimate.md`); a projection above P25 is the owner's decision, not chosen here (decision 1).

### 2. The directed cells K7 cites

K7 runs only the new cells (V1, V2, V3, V5; V4 is removed by ADR-0040). Its directed cells are the K4 and K5 runs, with the contract versions their plans carry:

| Cell | Build | Runs (valid) | Recorded result | Plans' contract version | Committed |
|---|---|---|---|---|---|
| D1 | M1 | 5 | 5 FAIL `lost-write` (and `incompatible-order`): DETECTED | 1.0.6 | full (K4) |
| D1 | real | 5 | 3 FAIL `incompatible-order` (FND-D1-…), 2 PASS | 1.0.6 | full (K4) |
| D2′ | M2 | 5 | 4 FAIL `lost-write`, 1 PASS: DETECTED | 1.0.7 | full (K4) |
| D2′ | real | 5 | 5 PASS | 1.0.7 | full (K4) |
| D3′ | M3 | 5 | 4 FAIL `g1a`, 1 PASS: DETECTED | 1.1.0 | stubs (K5; full in `runs/k5-full/`) |
| D3′ | real | 5 | 5 PASS | 1.1.0 | stubs (K5; full in `runs/k5-full/`) |
| P1 (probe suite) | M4 | 5 | 5 FAIL `p1-not-rejected`: DETECTED | 1.0.6 | full (K4) |
| P1 (probe suite) | real | 5 | 5 PASS | 1.0.6 | full (K4) |
| D5 | M5 | 5 | 5 FAIL `cdc-phantom`: DETECTED | 1.0.9 | stubs (K5; full in `runs/k5-full/`) |
| D5 | real | 5 | 5 PASS | 1.0.9 | stubs (K5; full in `runs/k5-full/`) |
| DH1 | h1 | 5 | 5 PASS: NOT_DETECTED (LIM-17) | 1.0.6 | full (K4) |

The superseded D2 (contract ≤ 1.0.6) and D3 (≤ 1.0.9) runs stay recorded (ADR-0027, ADR-0033) and are not K7 cells.

**Re-check.** Record `cited-recheck-2` (evidence/K6b, `scripts/k6b-cited-recheck.sh`): every one of the 55 valid primary runs above, re-checked with the cpcheck of this sprint (with `--elle-oracle` where the recorded `checks.json` holds K4's elle-cli pass, the same oracle image), gives a `checks.json` byte-identical to the recorded one. The first attempt (record `cited-recheck`, kept) differed only on D1 seed 3, whose recorded file carries that oracle pass. So the current checkers give the cited runs exactly their recorded verdicts and classes; no checker change since K4 and K5 (ADR-0036, ADR-0037, ADR-0038) applies to their plans.

**Criteria (§15 K7 and cpgate).** None needs the cited cells under the final contract version:

| Criterion | What it needs from the cited cells | Holds without re-running? |
|---|---|---|
| K7.1 Matrix complete | 5 valid runs per directed cell, INVALID runs listed with causes | yes: 5 valid each (table); K4's and K5's INVALID attempts are recorded in their `results.json` |
| K7.2 Statistics | k/N, Wilson interval or zero-failure bound from `checks.json`; exposure for k = 0 cells | yes from `checks.json` (committed beside every run). Exposure needs histories (ADR-0036): D2′, P1 and DH1 on the real build are held in full; D3′ and D5 on the real build are stubs — storage rule above (commit them in full as cited runs, or report "0 of 5 runs held in full") |
| K7.3 Oracle on findings | elle-cli on every C3 anomaly | yes: K4 cross-checked the real-build ones (triage); the `incompatible-order` anomalies of M1's five D1 runs were not cross-checked and are held in full, so K7 runs the oracle on them (or labels and triages `oracle-disagreement`, LIM-31). No other cited cell has a C3 anomaly |
| K7.4 Findings packages | each finding's replay command executed once more | yes: a replay runs the recorded plan (`commitproof replay`), whatever contract version it carries |
| K7.5 Claims audit | artifact hashes of cited runs | yes: committed archives, stubs and `checks.json` with their seal roots |
| K7.6 Overclaim review, K7.7 Release | — | not about runs |
| §12.4 side by side | mutant and real build on the same directed schedule | yes: each pair ran the same plan generator and schedule |
| cpgate | nothing in cpgate K1–K6 or in the K7 dependencies written so far (`internal/gate/k7deps.go`) selects runs by contract version; G12's rule follows the plan's own version (ADR-0037) | yes |

Two differences between cited and new cells are stated, not hidden: the cited runs ran without ADR-0038's metrics exporter (LIM-41), and under the contract versions in the table.

### Contract changes (1.2.1 → 1.3.0, MINOR: changed requirements)

- §13's local evidence policy states the cited/used/summary rule and what P25 counts; P25's value reads "≤ 250 MB total under `evidence/` and the run archives under `findings/` (ADR-0039)".
- §15 K7's matrix: the mutant, real-build directed and H1 rows are cited from K4/K5 with their contract versions; the run count and the time estimate describe only the cells K7 runs.

## Alternatives rejected

- **Re-run the directed cells under the final version.** The owner's decision 2; and the re-check shows the checkers give the recorded results.
- **Every K7 run in full.** About 1.2–3.5 MB per run (`evidence/pre-K7/k7-feasibility.md` §4) times 130 runs: far beyond the 29.66 MB left.
- **Uncited runs only in `runs-index.json`, without stubs.** The owner's decision names the summary: manifest, plan, seal and `checks.json`; the stub lets cpgate tie each `checks.json` to a sealed plan.
- **Counting `findings/` text toward P25.** The owner's decision.

## Consequences

- K7's matrix file lists only V1, V2, V3 and V5; its statistics cite the directed cells above.
- Whether the projected K7 evidence fits P25 depends on how many runs K7 cites, above all the failing real-build runs that §11.4 packages; the projection is in `evidence/K6b/k7-estimate.md`.

## Sources

CONTRACT §9.1, §11.4, §12.3, §12.4, §13, §15 K7, P15, P25; ADR-0030, ADR-0034, ADR-0036, ADR-0037, ADR-0038; LIM-17, LIM-30, LIM-31, LIM-37; evidence/K4/results.json, evidence/K5/results.json; evidence/pre-K7/k7-feasibility.md, readiness.md; evidence/K6b records `cited-recheck`, `cited-recheck-2`.
