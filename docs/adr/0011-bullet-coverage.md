# ADR-0011 — Honest coverage of job bullets 1, 3, 6 and 8

Status: Accepted (K0, contract 1.0.0)

## Context

Governing rule: every bullet has visible, verifiable evidence or is declared not covered. Facts: Multigres does not shard yet (S48); the migrator is a proto with no implementation on main (S30); logical replication is unsupported on the platform but implemented behind a flag in the repo (S12, S48); upstream already runs pg_regress, isolation, contrib and external suites with 100% pass-or-patched (S45) and a pgeval differential workflow (`.github/workflows/test-pgeval-differential.yml`).

## Decision

- **Bullet 1 (sharded query routing): PARTIAL, non-sharded only.** Evidence is leader routing under faults (G6, G8, M3). Sharded routing is NOT COVERED.
- **Bullet 3 (materialization for migrations and CDC): PARTIAL.** CDC across failover (G11, C10, M5). Migrations/materialization NOT COVERED: nothing exists to test.
- **Bullet 6 (sharding, relational algebra, Postgres internals): PARTIAL.** Postgres internals exercised through C6 (pg_rewind same-timeline divergence, S44), C7 (sync-rep cancel semantics, S61), C13 (visibility order on standbys, S62), isolation checking; sharding/relational algebra NOT COVERED.
- **Bullet 8 (stream processing, materialization, CDC): PARTIAL.** CDC checker and campaign only.

## Alternatives rejected

- **Differential semantic suite: gateway vs. plain PostgreSQL** to "cover" bullet 1/6. Rejected: it does not test routing for sharded databases (there are none) and duplicates upstream's parity suites (S45). It would be coverage by renaming.
- **Build a toy sharded router to show the skill.** Not evidence about Multigres; outside the governing rule ("what doesn't serve a bullet stays out" — it would serve the bullet only rhetorically).
- **Build a materialization pipeline on top of logical replication.** Same objection; and it would be CommitProof's code under test, not Multigres'.

## Consequences

The README reproduces the §4 matrix verbatim, NOT COVERED rows included.

## Sources

S12, S30, S44, S45, S48, S61, S62.
