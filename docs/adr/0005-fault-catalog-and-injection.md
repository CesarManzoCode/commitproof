# ADR-0005 — Closed fault catalog, injection by netns helpers, no clock skew

Status: Accepted (K0, contract 1.0.0)

## Context

The brief listed partitions, kill -9 of each component, SIGSTOP pauses, clock skew, slow disk, and membership change during failover, with the requirement that each fault be confirmed as observed, not just sent, and with minimum privilege. Upstream's harness has kill and SIGSTOP but no transport-level partition (S27, S28). Multigres binaries are static Go (`CGO_ENABLED=0`, S02). The heartbeat reader compares wall clocks across processes (S32).

## Decision

- Catalog F1a–F1d (partitions incl. asymmetric and one-way), F2a–F2e (kill), F3a–F3b (pause), F4a (slow disk, ADR-0006), F5a–F5b (membership), F6 (graceful switchover), F7 (network delay). Closed: anything else needs an ADR.
- Partitions and delay: `iptables`/`tc` run inside each network namespace by a helper container (`network_mode: service:<target>`, `cap_add: [NET_ADMIN]`, nothing else). No privileged containers, no host iptables, no host `sudo` in the run path.
- Kill/pause: `docker kill`, `docker pause`, or `kill -9/-STOP/-CONT` inside the container.
- Every fault has a confirmation and a heal predicate evaluated against the live system with deadlines; failure makes the run INVALID (fail-closed). K3.2 proves this by sabotaging injectors.
- **Clock skew is excluded.** Containers share the host kernel's `CLOCK_REALTIME`; Linux time namespaces only offset `CLOCK_MONOTONIC`/`CLOCK_BOOTTIME`; changing the realtime clock needs `CAP_SYS_TIME` on the host and would skew every container at once; `libfaketime` works through `LD_PRELOAD` on libc and cannot affect static Go binaries that read time through the vDSO. Multigres safety relies on logical terms (S06); the wall-clock comparison in heartbeat lag (S32) is recorded as an untested sensitivity.

## Alternatives rejected

- **Toxiproxy** for partitions: requires routing every connection through proxies and changing addresses in Multigres flags (diverges from S03); cannot express namespace-wide drops (gRPC health streams, replication, etcd) uniformly.
- **Pumba / Blockade / Chaos Mesh:** additional dependencies, privileged containers (Pumba) or Kubernetes (Chaos Mesh).
- **libfaketime for PostgreSQL only:** would skew `now()` inside PostgreSQL, but Multigres decisions use Go time in poolers/orchestrators; a partial skew would be misleading.
- **VMs (QEMU/Firecracker) per node for clock skew:** exceeds the 16 GB budget with four nodes plus control plane; CI cannot run nested virtualization reliably.

## Consequences

- Prerequisites: kernel modules for `iptables` (nf_tables) and `sch_netem` available on the host; `/dev/fuse` for slowfs (PREREQUISITES).
- LIMITATIONS lists excluded faults: clock skew, power loss/page-cache loss, disk full, packet corruption, DNS.

## Sources

S02, S06, S27, S28, S32.
