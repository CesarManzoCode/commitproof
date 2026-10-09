# ADR-0008 — Closed multi-dimensional findings taxonomy with claim levels

Status: Accepted (K0, contract 1.0.0)

## Context

A Multigres reviewer will ask, for any reported problem: is it a broken promise or a broken assumption? Is it Multigres or PostgreSQL? Does it reproduce? Is it already known? Is it a bug in the harness? A single severity label cannot answer that, and an ad hoc label invites inflation.

## Decision

Five dimensions with closed values (CONTRACT §11.1): Class (`SAFETY_VIOLATION`, `PROTOCOL_DEVIATION`, `LIVENESS_OBSERVATION`, `HARNESS_DEFECT`), Claim level (`EXPLICIT`, `DESIGN`, `INFERRED`, taken from the guarantee row in §3), Origin (`MULTIGRES`, `POSTGRES_INHERITED`, `UNDETERMINED`), Reproducibility (`REPRODUCED`, `INTERMITTENT`, `NOT_REPRODUCED`), Known status (`NEW`, `KNOWN_GAP`, `KNOWN_FIXED`). Severity is derived (§11.2), never chosen. Finding ids are content-derived (§11.3).

Specific consequences recorded here so they are not re-decided later:
- CDC findings are at most `DESIGN` (logical replication is not a supported platform feature, S48).
- LISTEN/NOTIFY loss inside reconnect windows is not a finding (documented, S19).
- Unavailability, slow failover, or refusal to fail over when unsafe (#1059/#1060, S50) are `LIVENESS_OBSERVATION` or `KNOWN_GAP`, never safety findings.
- Stale reads on the replica port are not findings (no claim); anomalies there are reported in C13 with origin analysis against S62.

## Alternatives rejected

- **CVSS-like scoring:** designed for security, meaningless here.
- **Free-text severity:** invites inflation (R10).

## Sources

S19, S48, S50, S62.
