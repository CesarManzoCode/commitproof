# ADR-0015 — Historical control H1 (issue #778) in addition to mutants

Status: Accepted (K0, contract 1.0.0)

## Context

Synthetic mutants show the pipeline detects defects we planted. A real bug that upstream fixed is a stronger control: issue #778 reported that a SIGSTOP-frozen primary was not detected and that thawing led to loss of 1000 synchronously committed rows, at commit `4bca1d5770a6` (S51), using manual `kill -STOP/-CONT`.

## Decision

Add profile `h1` built from commit `4bca1d5770a6bd425bde5a61eaad240431242cd5` with flags derived from that commit's own `demo/k8s` manifests, and schedule DH1 (long PostgreSQL pause on the leader, thaw, recovery, final reads), 5 runs. The gate requires the attempt and an honest outcome: DETECTED, NOT_DETECTED or NOT_DEPLOYABLE. It does not require detection, because the old commit's topology may not be reproducible with today's harness (six months of flag changes), and making the gate depend on that would be a contract defect.

## Alternatives rejected

- **Gate on H1 detection:** could be impossible for reasons unrelated to CommitProof's power (deployability), which would block the sprint on a non-defect.
- **Skip historical controls:** loses the strongest form of external validity available for free.

## Consequences

The README states H1's outcome in one line, whatever it is.

## Sources

S33, S51.
