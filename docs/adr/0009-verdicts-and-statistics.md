# ADR-0009 — Run verdicts, reproducibility classes and campaign statistics

Status: Accepted (K0, contract 1.0.0)

## Context

Distributed failures are nondeterministic: the plan is deterministic, the system is not. The project must neither discard a real violation because a replay did not reproduce it, nor present a lucky clean campaign as proof.

## Decision

- Run verdicts PASS/FAIL/UNKNOWN/INVALID (§12.1). INVALID (fault not confirmed, seal broken, no final read, cluster not healthy) never counts and is re-run with the same seed.
- A FAIL stands on its recorded history. Minimization + 5 replays only add a reproducibility class (§11.1, P13).
- Campaign statistics: k/N with Wilson 95% interval; for k = 0 the one-sided bound `1 − 0.05^(1/N)` plus exposure counts (ok writes, confirmed faults by kind, fault-time). The clean-result sentence template (§12.3) is mandatory.
- Mutant detection threshold ≥ 4/5 (P14). Repetitions: 10 per baseline cell, 5 per variant (P15), sized to fit about one night on the target machine.

## Alternatives rejected

- **Require a FAIL to replay before counting it:** would hide rare real violations; Jepsen practice treats the history as evidence.
- **Fixed-duration soak instead of repetitions:** one long run gives one sample of startup/teardown and recovery paths; repetitions with fresh clusters sample bootstrap and recovery many times.
- **Larger N (e.g. 100 per cell):** ~4 minutes per run makes 100×10 cells ≈ 67 hours; not feasible on one machine within the project's pace. The bound is reported honestly instead.

## Sources

Jepsen methodology (S62, S63), statistics standard practice.
