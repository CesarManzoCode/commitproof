# ADR-0026 — "F5b during F1a(leader)": F5b's confirmation is evaluated after F1a heals

Status: Accepted (K4, contract 1.0.6). Corrects the interaction of CONTRACT §8 (composite faults) with §15 K4 (C4's "F5b during F1a(leader)").

## Context (defect)

§15 K4 lists, for C4, the sequence "F5b during F1a(leader)": the member n4 is removed while the leader is partitioned. §8 defines composites as "ordered sequences … in one plan step, each element confirmed before the next starts", and F5b's confirmation predicate is "the leader's `synchronous_standby_names` no longer names n4 — deadline 120 s". Removal is leader-led (S24: multiorch issues `UpdateConsensusRule(REMOVE)` on the leader). With the leader isolated by F1a, the predicate can only become true if the system first elects another leader — and a safe system may refuse to.

Evidence (development run `20261011T213422Z-C4-2-real-e1f5552e`, not K4 evidence; `runs/dev/`): F5a admitted n4 (rule 1.1, cohort n1–n4), F2a(leader n1) → new leader n3 at rule 2.0; then F1a isolated n3 (confirmed after 1 s) and F5b stopped pooler-n4 gracefully. For the next 120 s no pooler reported itself leader of a newer rule; the orchestrators logged `initialized poolers (2) satisfy the durability policy but aren't failure-safe; add another pooler to this shard` and `UpdateConsensusRule failed: rpc error: code = Unavailable …` (logs/orch-z*.log). With n3 cut off and n4 stopped, 2 of the 4 cohort members remain; recruitment needs max(⌊4/2⌋+1, 4−2+1) = 3 (§10.8). F5b ended `not-applied: confirmation predicate did not hold within 2m0s` and the run INVALID. The step's design makes every run of this sequence INVALID whenever Multigres behaves safely; three INVALID attempts would be classified as a harness defect (§12.1) and C4 seed 2 could never have a valid run.

## Decision

- An element of a composite may be marked `confirm_after_heals`: it is resolved, prepared and injected in its place in the sequence, without waiting for its confirmation; the step then lasts its duration and runs its heal sequence; after the last heal is confirmed, the element's **unchanged** confirmation predicate is evaluated with its **unchanged** deadline (F5b: 120 s). Not holding within it is `not-applied` and the run INVALID, as before. The history shows `inject` (with a `confirmation-deferred` marker), then the other elements' heals, then `confirmed`/`not-applied`.
- C4's removal variant "F5b during F1a(leader)" is the step [F1a(leader), F5b(n4) `confirm_after_heals`] (ADR-0024's shapes otherwise unchanged): n4 is stopped while the leader is partitioned, exactly as the K4 table asks; whether and when the system removes it is observed once the partition heals.
- CONTRACT §8 (composite paragraph) and §15 K4 (C4 row) are amended (1.0.5 → 1.0.6).

## Alternatives rejected

- **Drop the sequence from C4.** Removing a fault from a campaign is a relaxation (§18).
- **Reverse the order (F5b, then F1a).** That is "F1a after F5b", not "F5b during F1a".
- **Resolve "the leader" for F5b as the isolated old leader and read its standby names.** The isolated leader cannot be told to remove n4; the predicate would still not hold, and reading a node by role instead of by the cluster's state would change the predicate's meaning.
- **Lengthen F5b's deadline.** Thresholds are P-values / §8 values; and no deadline helps while F1a holds the leader.

## Consequences

- C4 seeds whose removal variant is 1 ((seed−1) mod 2 = 1, e.g. seed 2) can produce valid runs; the observed unavailability while the leader is isolated is a liveness observation (§11.1), not a safety result.
- The K3 composite (F5a → F3a → F2a) and every other plan are unchanged (the field is emitted only when set).

## Sources

CONTRACT §8, §10.8, §12.1, §15 K4, §18; S24; ADR-0005, ADR-0024.
