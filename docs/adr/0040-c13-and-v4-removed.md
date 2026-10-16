# ADR-0040 — Campaign C13 and K7 cell V4 (replica reads) are removed

Status: **Accepted by the owner (K6b, contract 1.4.0).** Applies the owner's decision 4 of 2026-10-15: an owner-approved removal of a campaign (CONTRACT §18 forbids it without the owner). Written and committed before any K7 run and before the K7 matrix file.

## Owner decision (verbatim)

> 4. Cut C13 and V4 (replica reads): ADR plus a LIMITATIONS entry; amend the coverage table's bullet 6 and the matrix, because LIM-19 says replica-port reads carry no claim. This is an owner-approved removal of a campaign.

## Context

- §15 K7's cell V4 ran C13 ("W1 read-only transactions through the replica port mixed with primary-port writes, no nemesis and with F2a") on profile `replica-port` (gateways `--pg-replica-port=15433`, §5.5) five times. §4's bullet 6 listed "visibility order and snapshot isolation on replicas (C13, S62)" as evidence of PostgreSQL internals exercised.
- LIM-19 (K0) already says replica-port reads carry no claim: anomalies there would be compared with PostgreSQL's visibility-order behaviour on standbys (S62) and reported with origin analysis, never as Multigres violations; ADR-0008 says the same. No guarantee of §3 names C13.
- Neither C13, its replica-port read client nor the `replica-port` profile was implemented (`evidence/pre-K7/k7-feasibility.md` §1), and V4's run count was ambiguous (5 in total or 5 per variant).

## Decision

- C13 and V4 are removed from the contract. Profile `replica-port` (§5.5) is removed with them: no other campaign used it. K7's matrix is V1, V2, V3 and V5 (and the directed cells ADR-0039 cites).
- §4 bullet 6 keeps **PARTIAL (internals only)**; its evidence no longer lists C13, and its "not claimed" column adds visibility order and snapshot isolation on replicas.
- P15 reads "10 per cell of V1 and V3; 5 per cell of V2, V5; mutants and H1 per P14 (5)".
- R15 names C7 only.
- LIMITATIONS: a new entry (LIM-42) states the removal and what is therefore not tested; LIM-19 points to it.

## Alternatives rejected

- **Implement C13 and report its anomalies as observations.** The owner's decision; and with no claim to test, V4 would add runs and storage (P25) without adding a judged result.
- **Keep the `replica-port` profile for later.** Nothing in the contract would use it; it can return with a campaign that needs it, through an ADR.

## Consequences

- Contract 1.3.0 → 1.4.0 (MINOR: a requirement removed with the owner's approval): §4 bullet 6, §5.5, P15, §15 K7, §20 R15; the code's profile list and P15 constant follow.
- ADR-0008's and ADR-0011's sentences about C13 describe the K0 plan; this ADR supersedes them.
- K7 runs 130 runs (V1 100, V2 15, V3 10, V5 5).

## Sources

CONTRACT §3, §4, §5.5, §15 K7, §18, §20, P15; ADR-0008, ADR-0011, ADR-0039; LIM-19; S20, S62; evidence/pre-K7/k7-feasibility.md.
