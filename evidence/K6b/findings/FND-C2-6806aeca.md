# FND-C2-6806aeca — G6 (non-linearizable registers) in K4's C2 seed 3 run

Recorded in K6b by the owner's decision 7 of 2026-10-15 (ADR-0043): the G6 anomaly of run `20261011T231036Z-C2-3-real-f4505b54` is its own finding, cross-referenced to the mechanism it shares with the G2b findings. K4's records are not edited (K4 had described it inside FND-C2-2a534585's analysis). Machine-readable record, with the completion review: `FND-C2-6806aeca.json`. K7 writes its §11.4 package.

| Dimension | Value | Derivation |
|---|---|---|
| Guarantee | **G6** — single-key autocommit reads and writes through the primary port are linearizable | C4 `non-linearizable` on registers 1 and 4 (`checks.json` sha256 `2a520b5d…d066`) |
| Class | **SAFETY_VIOLATION** | §11.1: G6 is among G1–G6 |
| Claim level | **DESIGN** (S28) | G6's row |
| Severity | **high** | §11.2: SAFETY_VIOLATION ∧ DESIGN (critical needs EXPLICIT) |
| Origin | **POSTGRES_INHERITED** | the vanish mechanism, as the owner decided for it (ADR-0042, R15; `evidence/pre-K7/lim33.md` 5/5; S61); which cancel acted: UNDETERMINED |
| Reproducibility | **REPRODUCED** | the minimized plan `3fb018bf…127a` (run `20261012T025629Z-C2-3-real-3fb018bf`) shows `non-linearizable` in 3 of its 5 recorded replays (`030115Z`, `030441Z`, `030809Z`); that plan was minimized by K4 for `incompatible-order` (ADR-0024's target), not for this class |
| Known status | **NEW** | nothing upstream (`evidence/pre-K7/upstream-check.md`) |
| Id | **FND-C2-6806aeca** | §11.3: sha256(`SAFETY_VIOLATION\|G6\|non-linearizable\|3fb018bf…127a`)[:8]; the same formula reproduces the recorded FND-C2-2a534585 |

**What the history shows** (system did): with F1c on leader n1 (10.65–42.18 s, gateways still reaching it), a cas 237→240 on register 1 (op 3258) and a write of 242 on register 4 (op 3289) completed `info` when their clients gave up at P01 (20.73 s, 30.78 s). `ok` reads through gw-z3 and gw-z2 at 30.78–30.88 s returned 240 and 242. Rule 2.0 with leader n2 was first observed at 36.3 s. Afterwards register 1 no longer held 240 (a cas expecting 240 at 40.88 s was not applied) and register 4 returned the older 227 (read at 41.03 s). No register order explains an `ok` read of a value followed by `ok` operations that see the state before it.

**Shared mechanism** (cross-reference FND-C2-2a534585, FND-D1-fb8d0f8c, FND-D1-2e860fd2, FND-D1-a8eb9857; `evidence/K6b/reclassification.md`): a cancelled synchronous-replication wait completes the commit locally (S61), other sessions read it, and the failover to a standby that never received it removes it. On list-append keys it is G2b (C3 `incompatible-order`); on registers it is G6 (C4).

**Limitations:** the minimized plan targeted another class; the reads happened before the new rule was observed (1 s polling), so whether n1 was already superseded when it served them is not shown; G6's parenthetical ("no stale read from a superseded primary") is its motivating case, the checked property is linearizability (C6 found no `stale-read`).
