# ADR-0044 — A run's §12.3 exposure travels in its checks.json (C9's stats) from contract 1.7.0, so that runs kept as summaries still give a cell its clean statement

Status: **Accepted (K6b, contract 1.7.0).** Resolves a contradiction (CONTRACT §18) that ADR-0039 exposed. Written and committed before the code and before any K7 run. Amends ADR-0036 (exposure) and ADR-0039 (what makes a run cited).

## Context: the contradiction

- §12.3: a cell with k = 0 reports the zero-failure bound **and the exact exposure** (operations, ok writes, confirmed faults by kind, fault-time), and a clean statement must take the §12.3 form, which names the exposure. K7.2: every cell's statistics — k/N, interval or bound, **exposure counts** — are "recomputed by `cpgate` from `checks.json` files". K7's README uses the §12.3 sentence for clean cells (K7.6).
- ADR-0036 (K6) computes exposure from the histories of the cell's runs **held in full** ("otherwise 'm of N runs': stubs are not re-checkable") and renders the clean statement only when the exposure is complete. `checks.json` holds no exposure: C1's `ok_writes_checked` counts ledger and append writes only, C9's `faults` counts fault windows.
- ADR-0039 (the owner's decision 1) commits every K7 run that only contributes to a cell as a summary (stub and `checks.json`), and defined as *cited* — in full — "a run whose history a stated number is computed from (for example the exposure of a §12.3 clean statement)".

Together: either no K7 cell can carry the clean statement (its runs are summaries, exposure "0 of N"), or every clean cell's runs are cited in full — about 120 runs of 1.1–2.2 MB (`evidence/K6b/k7-estimate.md`), far beyond P25. K7.2's own wording (exposure from `checks.json`) is the way out: the exposure must be in `checks.json`.

## Decision

- **cpcheck.** For a run whose plan carries contract 1.7.0 or later, C9's `stats` carry `exposure`: `{"operations": n, "ok_writes": n, "confirmed_faults": {kind: n}, "fault_time_ns": n}`, computed from the sealed history with ADR-0036's definitions — operations = invokes except `final-read`; ok writes = `ok` ledger inserts + the `append` micro-ops of `ok` append operations + `ok` register `write` and `cas` + `ok` bank `transfer`; confirmed faults = `confirmed` nemesis events by kind; fault-time = Σ (heal t − confirmed t) over the faults with both. C9 is the availability **observation** checker (§10.10): exposure is stats, never part of a verdict, and C9's validity is unchanged. Plans before 1.7.0 get no field, so every recorded run re-checks byte-identically (records `cited-recheck-2`, `g12-recheck`) and the K2, K5 and K7 corpora (pinned versions) are unchanged.
- **cpreport.** A cell's exposure counts every valid run: from its history when the run is held in full (as before), from its `checks.json`'s C9 exposure when it is a summary that carries one. The "exposure over" column states how many runs were counted from histories and how many from `checks.json`; the clean statement needs every valid run counted one way or the other. A summary without the field (plans before 1.7.0) counts as before ("m of N runs held in full"). Pages of recorded runs render byte-identically (the K6 example, `make ci-report`).
- **cpgate K7** (K7 implements it): recomputes each cell's exposure from `checks.json`, and for every run held in full cross-checks its `checks.json` exposure against an independent recomputation from the history, as K6.5 does for failover metrics.
- **ADR-0039, amended:** the exposure of a clean statement no longer makes a run cited; its artifact is the runs' `checks.json` files and seal roots, like the cell's k/N. Runs a claim, a finding package or a report names as their artifact are still cited in full.

A summarized run's exposure, like its verdict, rests on cpcheck's output tied to its seal root and cannot be re-derived from git without re-running; K7's LIMITATIONS says so for its summarized runs (§13).

## Alternatives rejected

- **Cite every clean cell's runs in full.** ~120 runs, about 150–250 MB.
- **No clean statement for summarized cells.** Drops §12.3's mandatory form from almost every K7 cell (K7.6, R8).
- **A new top-level field of `checks.json`.** The K2 interface fixes `{run_id, seal_root, checkers, verdict}`; a checker's `stats` are free-form.
- **Exposure for every plan version.** Recorded runs' `checks.json` would no longer re-check byte for byte.

## Consequences

- Contract 1.6.0 → 1.7.0 (MINOR: a new output requirement): §10.10 (C9's exposure), §12.3 (exposure of summarized runs).
- K7's matrix plans carry 1.7.0; the K6b dry run's plan hashes change with the version (the dry run is repeated).

## Sources

CONTRACT §5.3, §10.10, §12.3, §13, §15 K6 (K6.5), §15 K7 (K7.2, K7.6), §18, §20 R8; ADR-0030, ADR-0036, ADR-0037, ADR-0039; LIM-30, LIM-37; evidence/K6b/k7-estimate.md.
