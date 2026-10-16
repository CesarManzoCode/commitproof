# Reclassification note: the four K4 G2 findings under ADR-0042 (contract 1.5.0)

The recorded findings and their records are **not edited** (`evidence/K4/triage/*.json`, `evidence/K4/closure.yaml`). This note states how they are classified from contract 1.5.0 on, by ADR-0042 (the owner's decision 6 of 2026-10-15: G2 split into G2a and G2b, prospectively). Their ids stay the recorded ones.

| Finding (recorded id) | Run | Recorded (contract 1.0.7) | From contract 1.5.0 (ADR-0042) |
|---|---|---|---|
| FND-D1-fb8d0f8c | `20261011T221027Z-D1-1-real-59a95336` | G2 · SAFETY_VIOLATION · EXPLICIT · **critical** · origin UNDETERMINED · REPRODUCED · NEW | **G2b** · SAFETY_VIOLATION · **INFERRED** · **high** · origin **POSTGRES_INHERITED** · REPRODUCED · NEW |
| FND-D1-2e860fd2 | `20261011T221506Z-D1-3-real-da159fa0` | G2 · SAFETY_VIOLATION · EXPLICIT · **critical** · UNDETERMINED · REPRODUCED · NEW | **G2b** · SAFETY_VIOLATION · **INFERRED** · **high** · **POSTGRES_INHERITED** · REPRODUCED · NEW |
| FND-D1-a8eb9857 | `20261011T221728Z-D1-4-real-ad3fca18` | G2 · SAFETY_VIOLATION · EXPLICIT · **critical** · UNDETERMINED · REPRODUCED · NEW | **G2b** · SAFETY_VIOLATION · **INFERRED** · **high** · **POSTGRES_INHERITED** · REPRODUCED · NEW |
| FND-C2-2a534585 | `20261011T231036Z-C2-3-real-f4505b54` | G2 · SAFETY_VIOLATION · EXPLICIT · **critical** · UNDETERMINED · INTERMITTENT · NEW | **G2b** · SAFETY_VIOLATION · **INFERRED** · **high** · **POSTGRES_INHERITED** · INTERMITTENT · NEW |

## Why G2b, not G2a

In all four runs no acknowledged write was lost: C1 is `valid: true` (each run's `checks.json` beside its archive in `evidence/K4/runs/`). Every value that later vanished was written by an operation that completed `info` (the client's P01 expired after its COMMIT was sent, §6.2 row 9; the triage records' `completion_review`), never `ok`. What the checkers found is C3 `incompatible-order`: `ok` reads on the isolated old leader returned those values, and a failover and rewind removed them. That is G2b's observable ("a read observes a write that later disappears"), not G2a's (an acknowledged write lost). Severity follows §11.2: SAFETY_VIOLATION ∧ INFERRED → `high`.

## Origin

**POSTGRES_INHERITED** (§11.1: "the behavior reproduces on plain PostgreSQL 17.7 streaming replication or is documented PostgreSQL behavior"; R15), as the owner decided, supported by:

- `evidence/pre-K7/lim33.md` (records `lim33-run-1` … `lim33-run-5`, `lim33-summary` in `evidence/pre-K7/commands.jsonl`): on plain PostgreSQL 17.7 with one synchronous standby, in 5 of 5 runs a commit waiting in SyncRep that is cancelled — by `pg_cancel_backend` or by the client's own CancelRequest — is completed locally and read by another session, and a failover to the standby with `pg_rewind` of the old primary removes it; a client that only disconnects does not cause it.
- S61: PostgreSQL documents that a cancelled synchronous-replication wait leaves the transaction "committed locally, but might not have been replicated to the standby".

**Which cancel acted in each Multigres run stays UNDETERMINED**: CommitProof's pgx client sends a CancelRequest when P01 expires (pgconn `asyncClose`), and Multigres' pooler cancels the backend when a request context ends (`regular_conn.go`, `handleContextCancellation`); either produces the anomaly, and the recorded runs do not show which reached each backend first (PostgreSQL's server log was not captured; `lim33.md`).

## Unchanged

Ids, runs, minimized plans, replays, reproducibility and known status (`NEW`: no upstream report or fix, `evidence/pre-K7/upstream-check.md`) are as recorded. The G6 anomaly of the C2 seed 3 run is a finding of its own, FND-C2-6806aeca (`evidence/K6b/findings/FND-C2-6806aeca.md`, ADR-0043).

Sources: ADR-0042, ADR-0043; CONTRACT §3, §6.2, §11.1–§11.3; S61; evidence/K4/triage/, evidence/K4/NOTES.md §7; evidence/pre-K7/findings-readiness.md, lim33.md, upstream-check.md.
