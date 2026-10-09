# ADR-0006 — Slow disk via FUSE pass-through with a pre-decided static-throttle fallback

Status: Accepted (K0, contract 1.0.0)

## Context

Slow disk matters for synchronous replication (flush on standby before ack, S09, S60) and for replay completion during revoke (S10). Host-level tools (`dm-delay`, cgroup `io.max` writes) need root on the host. Docker's `--device-write-bps` works without host root but is fixed at container start and needs the backing block device.

## Decision

- `slowfs`: a small Go FUSE pass-through (`github.com/hanwen/go-fuse/v2 v2.12.0`) mounted over `/data` inside `pg-nX` before pgctld starts. A control file outside the mount sets `delay_ms` for `write` and `fsync`. Container gets `--device /dev/fuse` and `CAP_SYS_ADMIN` (container-scoped). Confirmation: median of 5 probe `write+fsync` of 4 KiB on the mount ≥ 150 ms; heal: < 50 ms (P17).
- **Pre-decided fallback F4b**, applied by ADR without owner approval if K3 shows slowfs at 0 ms breaks the HEALTHY predicate or smoke checkers: static `--device-write-bps <data device>:1mb` on the node for the whole run, confirmed with a 256 KiB probe; runs labelled `static-throttle`; LIMITATIONS updated. This does not relax any gate: the confirmation requirement remains.

## Alternatives rejected

- **dm-delay / host cgroup writes:** host root.
- **tc on the replication link only:** models a slow network, not a slow disk (F7 covers that).
- **SIGSTOP of the WAL writer:** a pause, not a slow disk; F3 covers pauses.

## Consequences

- Baseline nodes run on FUSE too (delay 0) so that F4a runs and baseline runs share the same I/O path; overhead is uniform and recorded in K3 NOTES.
- `CAP_SYS_ADMIN` inside `pg-nX` is broader than other containers' privileges; documented in PREREQUISITES and LIMITATIONS.

## Sources

S09, S10, S60, S89.
