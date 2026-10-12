# ADR-0028 — C5 heals the slow disk before waiting for PostgreSQL's restart

Status: Accepted (K4). Amends ADR-0024's C5 shape (the contract delegates K4 shapes to ADR-0024; CONTRACT §8's predicates and §5.7's deadlines are unchanged, so the contract version is unchanged).

## Context (harness defect)

ADR-0024 made each C5 step the composite F4a(leader or a random follower) → F2a(leader), healed in the default reverse order: first F2a (whose heal predicate is "PostgreSQL restarts by itself and answers `select 1` within P28; otherwise a manual start through pgctld and another P28", §8), then F4a (slowfs delay back to 0).

When F4a and F2a hit the same node, PostgreSQL's crash recovery runs on the slowed disk (P17: 200 ms per `write` and `fsync`). In the K4 evidence all nine C5 attempts (seeds 1–3, three attempts each, `evidence/K4/runs/*-C5-*`) ended INVALID with `fault f… (F2a) not-healed: heal predicate did not hold within 30s`: in each, a new postmaster existed and the manual start returned 0, but `select 1` did not succeed within the deadline (e.g. run `20261011T233347Z-C5-1-real-d0654ac1`: F4a on n2, then F2a on the leader n2; heal 48.1 s → not-healed 110.4 s). Steps whose F4a and F2a hit different nodes healed normally (same run, step 0: F4a n2, F2a n1, healed in 0.1 s). Three INVALID attempts per seed make a harness defect to fix (§12.1).

## Decision

Each C5 step's heal sequence is F4a, then F2a: the slow disk is removed (F4a's heal predicate: median write+fsync < 50 ms), then F2a's unchanged predicate waits for PostgreSQL's restart with its unchanged deadline. The faults, their targets, their confirmation and the 10 s hold during which the killed PostgreSQL attempts its restart on the slow disk are unchanged. C5 plans change (their `heals` field); no other plan does.

## Alternatives rejected

- **Longer heal deadline for F2a in C5.** P28 is a contract value; widening it is a relaxation.
- **Never put F4a and F2a on the same node.** Removes the "F4a on leader … combined with F2a" case of §15 K4's C5 row.
- **Treat the INVALID runs as results.** INVALID runs never count (§12.1); the cause is the order the harness heals in.

## Consequences

- C5 is re-run with the amended shape; the nine INVALID attempts stay in the evidence with their reasons.

## Sources

CONTRACT §8 (F2a, F4a), §12.1, §15 K4 (C5); P17, P28; ADR-0024.
