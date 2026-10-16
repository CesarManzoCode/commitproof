# Findings readiness (pre-K7 readiness, step 5) — proposals only; no finding record changed

Recorded findings: evidence/K4/closure.yaml (4) and evidence/K5/closure.yaml (4), with their triage records under `evidence/K4/triage/` and `evidence/K5/triage/` (closed evidence, unchanged). Rules applied as written: CONTRACT §11.1 (class from the guarantee: G1–G6, G11, G13 → `SAFETY_VIOLATION`; G7–G10, G12 → `PROTOCOL_DEVIATION`; claim level "from the guarantee row"), §11.2 (severity derived: `critical` = SAFETY_VIOLATION ∧ EXPLICIT; `high` = SAFETY_VIOLATION ∧ (DESIGN ∨ INFERRED), or PROTOCOL_DEVIATION ∧ DESIGN on durability (G7); `medium` = other PROTOCOL_DEVIATION; `info` = LIVENESS_OBSERVATION). New inputs since the records: `lim33.md` (D-D), `upstream-check.md` (D-C), ADR-0037 (D-B).

## Re-check

| Finding | Guarantee (row level) | Recorded class / level / severity | By §11.1–§11.2 | Origin recorded → supported now | Known status recorded → upstream now | Reproducibility |
|---|---|---|---|---|---|---|
| FND-D1-fb8d0f8c | G2 (EXPLICIT: S48, S42) | SAFETY_VIOLATION / EXPLICIT / critical | **same** | UNDETERMINED → POSTGRES_INHERITED supported (proposal 2) | NEW → NEW (nothing upstream) | REPRODUCED |
| FND-D1-2e860fd2 | G2 | SAFETY_VIOLATION / EXPLICIT / critical | **same** | as above | NEW → NEW | REPRODUCED |
| FND-D1-a8eb9857 | G2 | SAFETY_VIOLATION / EXPLICIT / critical | **same** | as above | NEW → NEW | REPRODUCED |
| FND-C2-2a534585 | G2 | SAFETY_VIOLATION / EXPLICIT / critical | **same** | as above | NEW → NEW | INTERMITTENT |
| FND-C9-40b6f05a | G10 (DESIGN: S19) | PROTOCOL_DEVIATION / DESIGN / medium | **same** (G10 is not durability, so not `high`) | MULTIGRES → MULTIGRES | NEW → open upstream PR #1550 (proposal 3) | REPRODUCED |
| FND-C9-2fe9a9fc | G10 | PROTOCOL_DEVIATION / DESIGN / medium | **same** | MULTIGRES | NEW → PR #1550 | REPRODUCED |
| FND-C9-90da5532 | G10 | PROTOCOL_DEVIATION / DESIGN / medium | **same** | MULTIGRES | NEW → PR #1550 | INTERMITTENT |
| FND-C11-17a344c1 | G12 (DESIGN: S40, S22) | PROTOCOL_DEVIATION / DESIGN / medium | **same** (kept by the owner's decisions of 2026-10-14 and D-B) | MULTIGRES | NEW → NEW | INTERMITTENT |

**No severity or claim level of a recorded finding changes under the contract's rules.** Every recorded severity is the one §11.2 derives from the recorded class and claim level, and every claim level is the one its guarantee row states. One anomaly has no finding of its own (proposal 6).

## The G2 findings, stated without inflation or deflation

What the evidence supports, for the K7 packages' wording (§11.4 "Expected vs actual", "Limitations"):

- No acknowledged write was lost: C1 `valid: true` in all four runs; every value that later vanished was written by an operation that completed `info` (the client's P01 expired after its COMMIT was sent, §6.2 row 9), never `ok`.
- What was violated is G2's own observable, "no read observing a write that later disappears" (§3 G2): `ok` reads on the isolated old leader returned values that a later failover and rewind removed (C3 `incompatible-order`; confirmed by elle-cli for D1-1 and C2-3, `oracle-disagreement` for D1-3 and D1-4, LIM-31).
- Mechanism: PostgreSQL makes a commit whose synchronous-replication wait is cancelled visible locally (S61); the cancel came from the client's CancelRequest at P01 and/or Multigres' pooler cancelling on context end (`lim33.md`).

## Proposals for the owner (none applied)

1. **G2's claim level (a contract question, not a re-classification).** The G2 row gives the whole guarantee EXPLICIT (S48: acknowledged commits survive "failovers and split-brain scenarios"; S42: "the cluster does not split"). Those public statements concern acknowledged commits and split brain; the observable these four findings violate (a read of a write that was never acknowledged and later vanished) is the contract's operationalization of "no split brain" (two primaries' histories diverging and one being discarded), not a sentence of S48/S42. By the rules the four stay `critical`. If the owner judges that EXPLICIT overstates the public claim for this observable, the change is to the G2 row (split the read-visibility observable into a DESIGN- or INFERRED-level guarantee) by ADR before K7 packages them, which would make them `high`. No change is proposed here.
2. **G2 origin: `UNDETERMINED` → `POSTGRES_INHERITED`.** §11.1 defines `POSTGRES_INHERITED` as "the behavior reproduces on plain PostgreSQL 17.7 streaming replication or is documented PostgreSQL behavior". `lim33.md`: with the same client behaviour as CommitProof's (a CancelRequest when the client gives up), the read-then-vanish reproduced on plain PostgreSQL 17.7 in 5 of 5 runs, and it is documented (S61). The recorded triage keeps `UNDETERMINED`; K7's packages could state `POSTGRES_INHERITED` with that evidence, plus the Multigres-specific part as analysis: the pooler also cancels when a request context ends, so on Multigres even a client that only disconnects triggers it (plain PostgreSQL would keep waiting, k = 3). Which cancel acted in each recorded run stays undetermined.
3. **G10 known status.** PR #1550 (open, created 2026-10-15T01:06Z, after K5's runs of 2026-10-13) fixes the same mechanism and is not merged at upstream main `b2137f9`. §11.1's values have no "fix proposed upstream": `KNOWN_GAP` requires an `alpha-known-gap` issue or a documented limitation (S19 documents only reconnect-window loss), so by the rules the three stay `NEW`. Options: keep `NEW` and cite #1550 in each package's Analysis (fits the rules as written), or add a value (e.g. `KNOWN_OPEN_FIX`) by ADR (a MINOR change). The task's vocabulary: **KNOWN**, not FIXED-UPSTREAM.
4. **Three G10 ids, one mechanism.** §11.3 hashes each minimized plan, so the three C9 findings have three ids (as K5 recorded). K7 may package them under one README that lists the three ids, or as three packages that cross-reference; §11.4 requires a complete package "for every finding".
5. **G12 under ADR-0037.** FND-C11-17a344c1 stays `PROTOCOL_DEVIATION` / `medium` as recorded (D-B). Its package should state both fractions (literal 766/886 in storm window 1; outside fault windows 744/744) and that K7's C11 runs are judged by ADR-0037's rule, under which this storm window would pass.

6. **An unrecorded G6 finding in C2 seed 3.** Run `20261011T231036Z-C2-3-real-f4505b54` failed C3 (`incompatible-order` ×2, G2) **and** C4 (`non-linearizable` ×2 on registers 1 and 4, G6: "no stale read from a superseded primary", DESIGN, S28). K4 recorded one finding (FND-C2-2a534585, anomaly class `incompatible-order`, the class its minimization targeted) and described the C4 anomalies as the same mechanism (evidence/K4/NOTES.md §7, §10). §3 says every finding cites exactly one guarantee, and §11.3's id hashes the guarantee and the anomaly class, so the G6 anomaly has no finding of its own. Replays of the minimized plan `20261012T025629Z-C2-3-real-3fb018bf` (`evidence/K4/runs/*.checks.json`): `non-linearizable` in 3 of 5 (`030115Z`, `030441Z`, `030809Z`), `incompatible-order` in 2 of 5. Under §11.1–§11.2 a G6 finding would be `SAFETY_VIOLATION` / `DESIGN` / **`high`**, `REPRODUCED` on that plan (or minimized for `non-linearizable` first, §12.2). The owner decides whether K7 packages it as a ninth finding or confirms K4's folding.

## Package inputs available (§11.4)

Original and minimized runs of all eight findings are committed in full: K4's under `evidence/K4/runs/` (LIM-30), K5's four failing primaries and three minimized runs under `evidence/K5/runs/` (LIM-37). Their replays are full only in `runs/k4-full/` and `runs/k5-full/` (stubs in git; see the backup manifest). No `findings/` package exists yet (K7.4).
