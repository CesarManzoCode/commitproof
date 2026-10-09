# ADR-0001 — Multi-container Compose topology derived from upstream's Kubernetes demo

Status: Accepted (K0, contract 1.0.0)

## Context

CommitProof needs per-node network partitions, per-process kills and pauses, a fourth node for 3→4→3 membership changes, and several orchestrators that can hold different views (the race of S41). It must run on one 16 GB Arch Linux host and on 16 GB GitHub runners.

The upstream `docker-compose.yml` (S01) runs everything — etcd, multiadmin, and per cell PostgreSQL+pgctld, multipooler, multiorch, multigateway — inside **one** container via the `local` provisioner, clamps cells to 2–3, and only publishes zone1's gateway. Inside one container all components talk over loopback in one network namespace, so a partition between nodes would need loopback port filtering, and a fourth node is impossible.

The upstream Kubernetes demo (S03) runs each component as its own process with explicit flags, pooler and pgctld sharing localhost and `/data` in a pod, shared `/backups` and `/certs`.

## Decision

Run a Docker Compose project `commitproof` with one container per process group, flags copied from S03: `etcd`; `pg-nX` (pgctld+PostgreSQL) with `pooler-nX` sharing its network namespace (`network_mode: service:pg-nX`) and `/data` volume, for n1–n4 (n4 started only by fault F5a); `orch-z1..z3`; `gw-z1..z3`; one `nem-*` helper per network namespace; one `runner`. Three cells (zone1–zone3) mirror the production shape described in S42 (three AZs, two or three orchestrators). Static addresses on `10.77.0.0/24`. Every flag's provenance is recorded in `deploy/DERIVATION.md`.

## Alternatives rejected

- **Upstream single-container compose.** No per-node network faults, max 3 cells, no fourth node. Would have tested a topology no one runs in production.
- **Kind with upstream manifests.** Closest to production, but: a control plane plus three kubelet nodes costs several GiB before Multigres starts; kubelet restarts killed containers on its own schedule, making fault timing nondeterministic; NetworkPolicy-based partitions depend on the CNI (kindnet) and cannot express one-way or asymmetric drops; cluster bring-up takes minutes, which multiplies across hundreds of runs; the demo hostPath backups assume a single node directory. The derived Compose keeps the same flags and process boundaries without these costs.
- **Bare processes on the host via the `local` provisioner.** Requires host PostgreSQL binaries and host-level iptables (root); pollutes the host; not reproducible in CI without the same setup.
- **One container per component including separate pgctld and multipooler network namespaces.** Diverges from the upstream pod model where they share localhost (`--pgctld-addr=localhost:16200`).

## Consequences

- Faults act per network namespace and per container, as on separate machines, except for the shared kernel (page cache, clock) — recorded in LIMITATIONS.
- K1 must translate the demo's TLS/cert job and cluster-metadata job into Compose init services.
- The topology is not the Supabase platform's operator deployment; claims name the profile.

## Sources

S01, S03, S41, S42.
