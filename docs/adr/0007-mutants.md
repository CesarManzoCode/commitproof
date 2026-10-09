# ADR-0007 — Mutants as single-function patches with directed schedules and equivalence analysis

Status: Accepted (K0, contract 1.0.0)

## Context

"A verifier that never fails is not evidence." Mutants are the negative control for the whole pipeline (workload → nemesis → history → checkers) against the real system. They are only convincing if each models a plausible, documented failure mode, changes nothing else, and is actually detectable (non-equivalent) under a schedule we can run.

## Decision

- Catalog M1–M5 (CONTRACT §9.2): `sync-standby-cleared` (ack before quorum, S09), `cohort-add-local-view` (the membership race of S41), `gateway-retry-any` (unsafe retry, S14), `guard-removed` (S11), `standby-slots-cleared` (logical decoding ahead of the cohort, S13). Each is one patch touching one function, with a header stating intent and source.
- Each has a directed schedule (D1–D5, or probe P1) designed so the defect, if present, produces a specific checker class; detection = ≥ 4/5 directed runs (P14). The real build runs the same directed schedules and its results are reported.
- Mutant isolation: binaries that do not contain the patched package keep baseline hashes (K4.4).
- Equivalence analysis is part of the catalog: rejected candidates and why (§9.3), including the finding that for `AT_LEAST_2` the two recruitment terms coincide for cohorts of 3 and 4 (I3, verified by §10.8).

## Alternatives rejected

- **Random mutation testing (e.g. go-mutesting) of Multigres.** Thousands of mostly equivalent or irrelevant mutants; each needs a cluster run; no story a reviewer can check.
- **Mutants only via configuration** (e.g. `synchronous_commit=off` set by an operator). The gateway blocks it (S11); configuration changes outside code would not exercise the consensus code paths that matter.
- **Mutants without directed schedules** (rely on random campaigns). Detection becomes a probability with wide intervals; failing to detect would not distinguish a weak verifier from an unlucky schedule.

## Consequences

- M2 changes semantics only on `COHORT_ADD`; F5a's admission predicate is stated in terms observable under both builds (`synchronous_standby_names` names n4).
- Directed schedules cannot be tuned after seeing results without an ADR showing the change does not make detection easier than the real failure mode warrants.

## Sources

S09, S11, S13, S14, S24, S25, S41, S76.
