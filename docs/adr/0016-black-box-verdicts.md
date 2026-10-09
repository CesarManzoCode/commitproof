# ADR-0016 — Verdicts are black-box; system internals only confirm faults and explain

Status: Accepted (K0, contract 1.0.0)

## Context

Multigres exposes rich internals: pooler status RPCs, `multigres.current_rule`/`rule_history`, `synchronous_standby_names`, `/metrics`. Using them for verdicts would make the verifier trust the system under test. Not using them at all would make fault confirmation and incident timelines impossible.

## Decision

- Safety and protocol verdicts are computed only from client-observed history (`history.jsonl`) and the plan.
- Internals are read through a separate observation stream, used for: fault confirmation predicates (§8), the HEALTHY predicate (§5.1), and diagnosis/timelines (K6) — never to turn a FAIL into a PASS.
- Only observation code and the separate `quorumxcheck` binary may import Multigres packages; checker, gate and report binaries may not (§5.3, enforced by test).

## Alternatives rejected

- **White-box verdicts** (e.g. compare `rule_history` across nodes): verifies the system's account of itself, which is the thing in question.
- **No internals at all:** cannot confirm F5a admission or F6 switchover; timelines lose leader changes.

## Sources

S06, S25, S27.
