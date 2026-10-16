# ADR-0046 — K7.3: elle-cli on every C3 anomaly of K7, on M1's five D1 runs, and on histories where elle-cli runs out of memory

Status: **Accepted by the owner (K7, contract 1.8.1).** Applies the owner's decision 4 of 2026-10-15. Written and committed before any K7 run and before any oracle run of K7.

## Owner decision (verbatim)

> 4. K7.3 and elle-cli: before any K7 run, write an ADR that fixes how K7.3 handles M1's D1 runs (never checked against elle-cli) and any history on which elle-cli runs out of memory (LIM-31), following K4's accepted approach (validity pass recorded separately; the label if neither completes). Run elle-cli on the local full archives of M1's D1 runs.

## Context

- K7.3: "every C3 anomaly in the matrix confirmed by elle-cli or labelled `oracle-disagreement` with triage"; §10.11: elle-cli double-checks every C3 finding in K7, and a C3 finding the oracle does not confirm is labelled `oracle-disagreement` and triaged before any package is written.
- K4 ran `cpcheck check --elle-oracle` on its real-build failures with C3 anomalies (record `oracle-<run>`). On ~18 000-operation histories elle-cli's process-order cycle search exits 255 with `OutOfMemoryError` (also with a 9 GB heap), so cpcheck could not finish; K4 then recorded elle-cli's **validity pass** on its own (`elle-validity-<run>`: the pass that decides validity and the anomaly classes) and the process pass (`elle-process-<run>`, to show the failure), and cpgate K4 accepted "confirmed" when the validity pass answered with C3's classes (`internal/gate/k4.go` `checkOracle`; LIM-31).
- M1's five D1 runs (contract 1.0.6; committed in full in `evidence/K4/runs/`) FAIL with `lost-write` and `incompatible-order` (C3). K4 cross-checked only real-build failures, so their C3 anomalies were never given to elle-cli (ADR-0039's criteria table).

## Decision

### Which runs

1. Every valid primary run of K7's matrix whose `checks.json` has C3 `valid: false` (repeats and new failures alike).
2. M1's five D1 runs (`evidence/K4/results.json`'s D1/M1 cell): 20261011T215900Z-D1-1-M1-59a95336, 20261011T220118Z-D1-2-M1-8b22bb67, 20261011T220335Z-D1-3-M1-da159fa0, 20261011T220553Z-D1-4-M1-ad3fca18, 20261011T220810Z-D1-5-M1-3319a30c, from their full archives: K4 committed these primaries in full in `evidence/K4/runs/` (`runs/k4-full/` holds only the archives of K4's stubbed runs); each archive is extracted into the git-ignored `runs/.k7/oracle/`, its seal verified (`cpcheck verify-seal`) and its seal root compared with K4's `checks.json`; the K4 evidence is not touched.

A new finding's minimized run and replays are not part of K7.3; the finding's original run is (item 1).

### How (K4's approach, unchanged)

For each run, on its full run directory, with no cluster running (elle-cli's JVM and P10 on the 16 GB host):

1. `cpcheck check <run> --elle-oracle` (recorded `oracle-<run-id>`). If it completes, C3's `stats.elle` decides: `valid` `false` with `agree` (elle-cli's classes equal C3's) → **confirmed**; elle-cli answering `unknown` is not a confirmation (cpcheck's `agree` is then true by K2's convention) and is treated as not completed (step 2); any other answer → **`oracle-disagreement`**, triaged.
2. If it does not complete (elle-cli exits with an error, e.g. `OutOfMemoryError`, or times out): elle-cli's validity pass on the exported history (`elle-validity-<run-id>`: `-m list-append -c <model> --output json --cycle-search-timeout 10000`, model from the run's isolation level as cpcheck chooses it) and the process pass (`elle-process-<run-id>`, the anomalies with `-process` suffixes), each recorded with the pinned image `commitproof/elle-oracle:9c2a18b`, no network, read-only. If the validity pass answers `valid? false` with exactly C3's classes (Elle names mapped by §10.11's table, informational names dropped) → **confirmed** (source: the validity pass); if it answers otherwise → **`oracle-disagreement`**, triaged.
3. If neither cpcheck's oracle nor the validity pass completes, the run is labelled **`oracle-disagreement`** with the triage "elle-cli did not complete (…)": an unanswered oracle is never a confirmation (fail closed, LIM-31).

### Where it is recorded

`evidence/K7/oracle/<run-id>.json`: run id, seal root, source (K7 cell, or the cited M1 D1 cell), C3's classes and totals, `result` (`confirmed` | `oracle-disagreement`), `elle_valid`, `elle_classes`, the records that decided it, and `triage` (required for `oracle-disagreement`). For K7's runs the `checks.json` that cpcheck rewrites with `stats.elle` is the one committed (as a summary or with the full archive); for M1's D1 runs that rewritten file is kept as `evidence/K7/oracle/<run-id>.checks.json` and K4's committed `checks.json` stays as it is. `cpgate K7` checks every record against these files as `checkOracle` does for K4.

### Not changed

The oracle never changes a verdict: C3 decides, elle-cli cross-checks (§10.11). A disagreement is triaged, never resolved by editing C3's result.

## Alternatives rejected

- **A larger heap or a longer cycle search.** LIM-31: 9 GB did not help on K4's histories; the host has 16 GB and K7 runs one cluster at a time.
- **Skipping M1's D1 runs** because they are mutant runs: K7.3 covers every C3 anomaly of the cells K7 reports; the owner's decision asks for them.
- **Re-checking M1's D1 runs in place.** Closed evidence is immutable (AGENTS.md); the copy is checked and the result recorded under K7.

## Consequences

- Contract 1.8.0 → 1.8.1 (PATCH, clarification): §10.11 points here.
- LIM-31 states K7's outcome (which runs elle-cli finished, which were confirmed by the validity pass, any label).

## Sources

CONTRACT §10.3, §10.11, §15 K7 (K7.3), P10; ADR-0004, ADR-0039; LIM-26, LIM-31; scripts/k4-evidence.sh (oracle group); internal/gate/k4.go (checkOracle); evidence/K4/results.json, evidence/K4/triage/.
