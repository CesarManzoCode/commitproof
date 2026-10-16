# ADR-0042 — G2 is split into G2a (acknowledged write lost under split brain, EXPLICIT) and G2b (a read observes a write that later disappears, INFERRED), prospectively

Status: **Accepted by the owner (K6b, contract 1.5.0).** Applies the owner's decision 6 of 2026-10-15. Written and committed before any K7 run.

## Owner decision (verbatim)

> 6. Split G2 prospectively: G2a (acknowledged write lost, split brain; EXPLICIT; S48, S42) and G2b (a read observes a write that later disappears; INFERRED, no public source says it cannot happen). ADR plus contract bump committed before any K7 run. Recorded K4 findings and records are not edited: add a reclassification note that references the ADR. Origin of the vanish findings is POSTGRES_INHERITED (R15), supported by the 5/5 plain-PostgreSQL runs; keep "which cancel acted in each Multigres run" as UNDETERMINED.

## Context

- G2 (contract ≤ 1.4.1) read: "No split brain: the history admits a single total order of committed transactions; no two leaders commit divergent durable histories. Observable as: no acknowledged write lost (G1) **and** no list-append incompatible order / no read observing a write that later disappears", level EXPLICIT (S48, S42), checkers C1, C2, C3.
- The four G2 findings of K4 (FND-D1-fb8d0f8c, FND-D1-2e860fd2, FND-D1-a8eb9857, FND-C2-2a534585) are of the second observable only: no acknowledged write was lost (C1 valid in all four runs); `ok` reads on the isolated old leader returned values whose writes completed `info` and that a later failover and rewind removed (C3 `incompatible-order`). By §11.2 they were `critical` because the row was EXPLICIT (`evidence/pre-K7/findings-readiness.md` §"The G2 findings", proposal 1).
- The public statements (S48: acknowledged commits survive "failovers and split-brain scenarios"; S42: "the cluster does not split") concern acknowledged commits. No public source states that a read can never observe a write that later disappears; the expectation comes from PostgreSQL compatibility (I2): on one PostgreSQL server, a value another session has read is committed and stays.
- The anomaly reproduces on plain PostgreSQL 17.7 streaming replication (`evidence/pre-K7/lim33.md`, 5 of 5 runs: a cancel of a commit waiting for its synchronous standby makes it visible; a failover with `pg_rewind` removes it; documented, S61).

## Decision

### §3: two rows replace G2

| ID | Guarantee (as tested) | Level | Source | Checker(s) | Campaigns |
|---|---|---|---|---|---|
| G2a | No split brain for acknowledged writes: no two leaders commit divergent durable histories in which an acknowledged write is lost. Observable as: no acknowledged write lost while the cluster was split or failing over (C1 `lost-write`) and no divergence of the stable final reads between gateways (C1 `final-read-divergence`). | EXPLICIT | S48, S42 | C1 | C2, C3, C4, C6, C12 |
| G2b | No read observes a write that later disappears: a value an `ok` read returned is in the history that survives, whether or not its write was acknowledged. Observable as: no list-append `incompatible-order` (C3) — two reads of a key that are not prefixes of each other — from a value that later vanished. | INFERRED | I2 (no public source states it; S61 documents PostgreSQL's side) | C3 | C2, C3, C4, C6, C12 |

- A `lost-write` is G1 unless the triage shows two leaders that could commit (a split or a failover in progress), then G2a: one finding cites one guarantee (§3), chosen by that triage as before.
- A C3 `incompatible-order` whose vanished value was written by an `ok` operation is also a C1 `lost-write` and is triaged as G2a (or G1); one whose vanished values were all written by `info` operations is G2b.
- §11.1's class: both are `SAFETY_VIOLATION`. §11.2 therefore derives **G2a → `critical`** (EXPLICIT) and **G2b → `high`** (INFERRED).

### Prospective: recorded findings keep their records

- The K4 triage records and closure (`evidence/K4/`) are not edited. Their finding ids stay the recorded ones (§11.3 hashed the guarantee as recorded, "G2"): an id names a recorded finding; it is not recomputed.
- `evidence/K6b/reclassification.md` is the reclassification note: under contract 1.5.0 or later the four findings are **G2b, `SAFETY_VIOLATION`, `INFERRED`, `high`**, origin **`POSTGRES_INHERITED`** (R15; `lim33.md`, 5 of 5; S61), and which cancel acted in each Multigres run (the client's CancelRequest at P01 or the pooler's cancel on context end) stays **UNDETERMINED**. K7's finding packages state that classification and cite this ADR and the note; they show the recorded classification beside it.
- Findings recorded from contract 1.5.0 on hash `G2a` or `G2b` (§11.3).

## Alternatives rejected

- **Keep G2 whole and EXPLICIT.** Rates a read of a never-acknowledged value as a violation of a public statement about acknowledged commits; the owner's decision.
- **Edit the K4 records.** Closed evidence is immutable (AGENTS.md §2); the note references this ADR instead.
- **Recompute the recorded ids with "G2b".** Two ids for one recorded finding; the recorded id stays the reference.

## Consequences

- Contract 1.4.1 → 1.5.0 (MINOR: changed requirement): §3 rows G2a and G2b; §11.1 lists them among `SAFETY_VIOLATION`'s guarantees; LIM-33 states the origin.
- cpgate K4 keeps judging K4's records against the rows they were written under (`internal/gate/k4.go`); K7's gate uses G2a/G2b.

## Sources

CONTRACT §3, §11.1–§11.3, §20 R15; S42, S48, S61, I2; ADR-0008; evidence/K4/triage/, evidence/K4/NOTES.md §7; evidence/pre-K7/findings-readiness.md, lim33.md, upstream-check.md.
