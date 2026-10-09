# ADR-0018 — A single runner process inside the cluster network writes the history

Status: Accepted (K0, contract 1.0.0)

## Context

Client operations and nemesis events must be totally ordered with one clock. Clients on the host would reach gateways through Docker's port publishing (userland proxy/DNAT), adding a path that partitions do not model and making host-port collisions possible.

## Decision

The `runner` container (address 10.77.0.40 on `cpnet`) runs one `commitproof` process that hosts all workload clients, the nemesis controller (via the mounted Docker socket and `docker exec` into helpers) and the observation pollers; it is the only writer of `history.jsonl` and `observations.jsonl`, timestamping with its monotonic clock. The run directory is a bind mount. Host-side `make` targets only build images and launch the runner.

## Alternatives rejected

- **Clients on the host via published ports:** extra network path, collisions, asymmetric with CI.
- **Separate client and nemesis processes:** two writers need clock reconciliation; the shared kernel monotonic clock would work, but merging streams adds failure modes for no benefit.

## Consequences

The runner's Docker socket is root-equivalent on the host (LIMITATIONS, PREREQUISITES). The runner itself can be isolated by F1-style rules only for client↔gateway partitions, which no campaign currently uses.

## Sources

S03.
