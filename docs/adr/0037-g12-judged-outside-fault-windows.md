# ADR-0037 — G12 is judged outside fault windows from contract 1.2.0 on; both fractions are reported (owner decision on ADR-0035)

Status: **Accepted by the owner (pre-K7, contract 1.2.0); the version number 1.2.0 (rather than 1.1.2) confirmed by the owner's decision 5 of 2026-10-15 (K6b).** Applies ADR-0035 prospectively. Written and committed before any K7 run, as the owner requires, and before the checker code that implements it.

## Owner decision (verbatim, 2026-10-15)

> D-B. ADR-0035: ACCEPTED prospectively, for K7 only. The recorded K5 verdicts and findings (including FND-C11-17a344c1) stay exactly as recorded under the literal rule. From K7 on, every C11 cell reports BOTH fractions side by side (literal, and excluding operations invoked inside fault windows), and the verdict rule is the one ADR-0035 defines, written down and committed before any K7 run. No recorded run changes verdict.

## Context

- ADR-0035 (proposed in K5) showed that C11 seed 2's FAIL comes from the F2a the plan injects inside a storm: all 120 non-ok `cp_steady` operations were invoked inside the F2a window, and 744 of 744 invoked outside it completed ok (record `c11-window-check`). G10's judgement already excludes fault windows (ADR-0030); G12's literal rule (§10.9, LIM-25) does not.
- The owner accepted ADR-0035 for K7 only. "No recorded run changes verdict" means the checker must keep the literal rule for every run already recorded (K1–K5 runs, the K2 and K5 corpora), including when `cpcheck` re-checks one of them, while every K7 run is judged by ADR-0035's rule.
- A plan is a pure function of `(campaign, seed, profile, directed schedule id, contract version)` and carries `contract_version` (§6.3). It is the one input of a checker (§10: "pure functions of a sealed run (history + plan)") that tells a recorded run from a K7 run.
- Versioning (the paragraph under the contract header): changed requirements bump MINOR. A verdict rule that stops counting some failures is a changed requirement, as ADR-0034's change of P25 was; only the owner may make it (§18), and the owner did. The owner's decision names "1.1.2 (or the next number)"; the contract's own rule gives 1.2.0, so the contract becomes **1.2.0**.

## Decision

### Which rule a run gets

`C8`'s G12 rules read the plan's `contract_version`. A plan carrying **1.2.0 or later** (compared as three integers; every K7 run) is judged by the rule below. A plan carrying an earlier version, or none, is judged by the literal rule of contract ≤ 1.1.1, unchanged, and gets none of the statistics or observations below: re-checking any recorded run reproduces its recorded `checks.json`, and no recorded verdict or finding changes (FND-C11-17a344c1 stays `PROTOCOL_DEVIATION`, `medium`, as recorded).

### The rule from 1.2.0 (ADR-0035's proposal, made precise)

For each storm phase p:

1. **Storm window** W = [first invoke, last completion] of the phase's `cp_storm` operations (unchanged).
2. **Fault windows**: per fault id, its first to its last nemesis event; a fault with neither `healed` nor `not-healed` (other than F5a/F5b) lasts to the end of the history. These are the windows G10's judgement uses (§10.9, ADR-0030).
3. **L** = the `cp_steady` operations invoked in W (the literal set). **O** = those of L whose invoke time lies inside a fault window (closed intervals). **J** = L \ O, the judged set. An operation invoked before a fault window and completing inside it is in J, as ADR-0035 words it ("invoked outside every fault window").
4. **Outside time** S = the length of W minus the length of the union of W's intersections with the fault windows, in ns.
5. **Judgement** (integers only):
   - `fair-share-starvation` if J is not empty and `ok(J)·10 < |J|·9` (fewer than 90 % of J completed ok);
   - otherwise **not judged** if J is empty or `|J| · 1 s < S` (fewer than one judged operation per second of storm outside fault windows): observation `fair-share-not-judged`, and C8 is `unknown` with the reason — never PASS (§12.1; a window with no `cp_steady` operation is already `unknown` under the literal rule);
   - otherwise the window passes.
   Starvation is judged before the minimum rate, so a window the literal rule fails without any fault (J = L) still fails: the clarification relaxes nothing outside fault windows (ADR-0035). A fault-free window that the literal rule passes with fewer than one operation per second becomes `unknown`, which is stricter.
6. **Observation `overload-fault-overlap`** when O is not empty: its operations (their invoke indices) with their completions counted by type and the overlapping fault ids — measured, not judged, as `notify-window-loss` is for G10.
7. **Both fractions** are written into C8's statistics for every storm window, `stats.storm_windows[]` = `{phase, start, end, literal_total, literal_ok, judged_total, judged_ok, inside_fault_total, outside_fault_ns, result}` with `result` ∈ `pass`, `fair-share-starvation`, `not-judged`.
8. `overload-failover` is unchanged (it already judges only storm windows that no fault window overlaps).

### Reporting in K7

Every C11 cell (V5) reports, side by side with k/N (§12.3): Σ`literal_ok`/Σ`literal_total` and Σ`judged_ok`/Σ`judged_total` over the cell's storm windows, and the number of storm windows below 90 % under each, read from `checks.json` (C8 `stats.storm_windows`). The cell's k counts FAIL runs under the rule its runs carry (this ADR's, for K7 runs); the literal fraction is reported, not judged.

### Classification

A G12 failure under this rule stays `PROTOCOL_DEVIATION` (§11.1), severity `medium` (§11.2, G12 is DESIGN). Runs judged under the literal rule are not re-classified (the owner's decision).

### Validation before any K7 run

- A new synthetic corpus `testdata/corpus-k7` (family `w8g`), expectations written from this rule and the plant, never from running the checker: fault-free clean storms; fault-overlapped storms that the literal rule fails and this rule passes (`overload-fault-overlap`); the same histories carrying contract 1.0.9 (`g12-literal`: the literal rule still fails them); starvation outside fault windows that both rules fail (`fair-share-starvation`); windows not judged (`fair-share-not-judged`: no judged operation, or one judged operation in 2 s of storm outside a fault).
- Sabotage rules (build tags, CONTRACT §15 K5.1): `cp_sabotage_g12_fault_exclusion` (J = L: no exclusion) must fail exactly the labels `overload-fault-overlap` and `fair-share-not-judged`; `cp_sabotage_g12_judged_minimum` (no minimum rate) exactly `fair-share-not-judged`; the existing `cp_sabotage_fair_share` exactly `fair-share-starvation` and `g12-literal`. `make ci-checkers` runs them (`k2check --set k7`).

## Alternatives rejected

- **Apply the rule to every run, re-checking K5**: contradicts the owner's decision ("No recorded run changes verdict").
- **Choose the rule by campaign or by a command-line flag**: a flag makes the verdict depend on how `cpcheck` was invoked, not on the sealed run; the campaign does not tell a K5 C11 run from a K7 one. The plan's contract version does, and is sealed.
- **Not judged ⇒ PASS** (ADR-0035 forbids it) or **not judged ⇒ FAIL** (would report unfairness that was not observed).
- **The minimum rate before the starvation test**: a fault-free starving window with operations hanging until P01 would then be `unknown` instead of FAIL — a relaxation outside fault windows.
- **Contract 1.1.2**: the owner allowed "the next number"; the contract's versioning paragraph makes a changed verdict rule MINOR.

## Consequences

- Contract 1.2.0: §10.9's G12 item and §15 K7's V5 row point here. `params.ContractVersion` = 1.2.0; recorded runs keep the versions they carry, and the gates of K4 and K5 keep reading theirs (`params.K4…`, `params.K5…`).
- ADR-0035: status "Accepted by the owner (pre-K7), prospectively for K7 only; applied by ADR-0037".
- LIMITATIONS LIM-25 records that K5's G12 verdicts used the literal rule and K7's use this one, and that a not-judged storm window makes a run UNKNOWN.

## Sources

CONTRACT header (versioning), §3 (G12), §6.3, §10, §10.9, §11.1, §11.2, §12.1, §12.3, §15 K5, §15 K7, §18; ADR-0030, ADR-0034, ADR-0035; LIM-25; record `c11-window-check` (evidence/K5/commands.jsonl); S22, S40.
