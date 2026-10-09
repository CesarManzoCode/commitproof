# ADR-0017 — Independent gate binary with evidence-level negative controls

Status: Accepted (K0, contract 1.0.0)

## Context

Method inherited from Thalyx-Kernel: each sprint closes on a gate decided by an independent checker that reads what the system recorded, validated by sabotaging each criterion and confirming that exactly that criterion fails.

## Decision

- `cpgate K<n>` reads only files under `evidence/K<n>/` (and referenced sealed runs), recomputes what it can (hashes, counts, statistics), and emits `gate.json`.
- `--negative-controls` copies evidence to a temp directory, applies one deterministic sabotage per criterion, and requires that criterion and only that criterion to fail. Checker-level sabotage uses build tags `cp_sabotage_*` excluded from product builds.
- `cpgate` does not import runner, nemesis, cluster, workload or observation packages, nor Multigres (§5.3).

## Alternatives rejected

- **Gate logic inside `commitproof`:** the system would grade itself.
- **Negative controls only at the code level:** proves checkers can fail, not that the gate reads the evidence it claims to read.

## Sources

Method reference (brief), S27.
