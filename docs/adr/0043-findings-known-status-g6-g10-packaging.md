# ADR-0043 — Known status `KNOWN` (an open upstream fix), the G6 finding of C2 seed 3, and how G10 is packaged

Status: **Accepted by the owner (K6b, contract 1.6.0).** Applies the owner's decision 7 of 2026-10-15. Written and committed before any K7 run and before any finding package.

## Owner decision (verbatim)

> 7. Package the G6 anomaly of run C2 seed 3 as its own finding, claim_level DESIGN (S28), severity derived by §11.2 and shown, cross-referenced to the shared mechanism. G10 is packaged as an independent reproduction that supports upstream PR #1550, known_status KNOWN (open PR), not as a new issue.

## Context

- **G6.** K4's C2 seed 3 run (`20261011T231036Z-C2-3-real-f4505b54`) failed C3 (`incompatible-order`, G2) and C4 (`non-linearizable` on registers 1 and 4, G6). K4 recorded one finding, FND-C2-2a534585 (the class its minimization targeted), and described the C4 anomaly as the same mechanism. §3 gives every finding exactly one guarantee; §11.3 hashes guarantee and anomaly class (`evidence/pre-K7/findings-readiness.md`, proposal 6).
- **G10.** The three G10 findings of K5 (FND-C9-40b6f05a, FND-C9-2fe9a9fc, FND-C9-90da5532) share one mechanism: the gateway re-establishes a LISTEN notification stream only on the client's next LISTEN/UNLISTEN. Upstream pull request #1550 (open, created 2026-10-15T01:06:33Z, after K5's runs; not merged at upstream main `b2137f9`) fixes that mechanism (`evidence/pre-K7/upstream-check.md`; RESEARCH S53). §11.1's known-status values are `NEW`, `KNOWN_GAP` (an `alpha-known-gap` issue or a documented limitation) and `KNOWN_FIXED` (historical control): none names an open upstream fix. §11.4 writes every package "in upstream issue format".

## Decision

### §11.1: known status `KNOWN`

`KNOWN` = an open upstream issue or an unmerged upstream pull request describes or fixes the same mechanism; it must be cited (a RESEARCH source). The three G10 findings are **`KNOWN`** (PR #1550, S53). Their records in `evidence/K5/` are not edited (they say `NEW`, true when recorded); K7's packages state `KNOWN` and cite this ADR. Severity does not depend on known status (§11.2): they stay `PROTOCOL_DEVIATION` / `DESIGN` / `medium`.

### §11.4: a `KNOWN` finding's package

A `KNOWN` finding is packaged as an **independent reproduction supporting the cited upstream item**, not as a new issue: its README keeps §11.4's sections (summary, version/environment, reproduction, expected vs actual, analysis, limitations) and opens by naming the upstream item it supports and what the reproduction adds (a black-box, seed-replayable history at the pinned commit). Each finding id keeps its package directory (`findings/<id>/`, §11.4: every finding has a complete package); the three G10 packages may share one README text and cross-reference each other. Publishing stays the owner's decision (AGENTS.md §6).

### The G6 finding

**FND-C2-6806aeca**: G6, `SAFETY_VIOLATION`, claim level `DESIGN` (S28), severity **`high`** (§11.2: SAFETY_VIOLATION ∧ DESIGN), origin `POSTGRES_INHERITED` (the vanish mechanism, as decided for it by ADR-0042), reproducibility `REPRODUCED` (3 of 5 recorded replays of the run's minimized plan show `non-linearizable`), known status `NEW`. Id by §11.3 over K4's minimized plan `3fb018bf…127a` (the same formula reproduces the recorded FND-C2-2a534585). Its record, with the completion review and the timeline, is `evidence/K6b/findings/FND-C2-6806aeca.{md,json}`; its package (K7) cross-references the G2b findings of the same mechanism. The minimized plan was minimized for `incompatible-order`; a minimization for `non-linearizable` is not required by §12.2 for a finding identified after the fact and is not made (the plan reproduces the class, P13).

## Alternatives rejected

- **`KNOWN_GAP` for G10.** That value means an accepted gap (`alpha-known-gap`) or a documented limitation; S19 documents only reconnect-window loss, which G10 does not judge.
- **One finding id for the three G10 runs.** §11.3 derives ids from minimized plans; the recorded ids stay.
- **Fold G6 into FND-C2-2a534585 (K4's choice).** The owner's decision; and §3's one-guarantee rule.

## Consequences

- Contract 1.5.0 → 1.6.0 (MINOR: a value added to a closed taxonomy, a package form): §11.1 adds `KNOWN`; §11.4 states the `KNOWN` package form; §20 R19 names it. RESEARCH adds S53 (PR #1550).
- K7's finding list (prompts/K7.md): the four G2b findings, FND-C2-6806aeca, the three G10 findings (`KNOWN`), FND-C11-17a344c1, and whatever K7 finds.

## Sources

CONTRACT §3, §11.1–§11.4, §12.2, §20 R19; S19, S28, S53, S61; ADR-0024, ADR-0042; evidence/K4/triage/20261011T231036Z-C2-3-real-f4505b54.json, evidence/K4/runs/ (the run, its minimized run and replays); evidence/K5/triage/; evidence/pre-K7/findings-readiness.md, upstream-check.md, upstream/lookups.jsonl (lookups 21–23).
