# ADR index

| ADR | Title | Status |
|---|---|---|
| [0001](0001-multi-container-compose-topology.md) | Multi-container Compose topology derived from upstream's Kubernetes demo | Accepted (K0) |
| [0002](0002-pin-by-source-commit.md) | Pin Multigres by source commit; build images; reproducibility on binary hashes | Accepted (K0) |
| [0003](0003-history-format.md) | History as canonical, hash-chained JSONL in sealed run directories | Accepted (K0) |
| [0004](0004-checkers.md) | Reimplement an Elle subset in Go, wrap Porcupine, use elle-cli only as an oracle | Accepted (K0) |
| [0005](0005-fault-catalog-and-injection.md) | Closed fault catalog, injection by netns helpers, no clock skew | Accepted (K0) |
| [0006](0006-slow-disk-slowfs.md) | Slow disk via FUSE pass-through with a pre-decided static-throttle fallback | Accepted (K0) |
| [0007](0007-mutants.md) | Mutants as single-function patches with directed schedules and equivalence analysis | Accepted (K0) |
| [0008](0008-findings-taxonomy.md) | Closed multi-dimensional findings taxonomy with claim levels | Accepted (K0) |
| [0009](0009-verdicts-and-statistics.md) | Run verdicts, reproducibility classes and campaign statistics | Accepted (K0) |
| [0010](0010-ci-vs-local-evidence.md) | What runs in GitHub Actions and what is recorded local evidence | Accepted (K0) |
| [0011](0011-bullet-coverage.md) | Honest coverage of job bullets 1, 3, 6 and 8 | Accepted (K0) |
| [0012](0012-license.md) | License: Apache-2.0 | Accepted (K0) |
| [0013](0013-name.md) | Final name: CommitProof | Accepted (K0) |
| [0014](0014-language-and-toolchain.md) | English documentation; Go 1.26.9 via GOTOOLCHAIN | Accepted (K0); amended by ADR-0019 |
| [0015](0015-historical-control.md) | Historical control H1 (issue #778) in addition to mutants | Accepted (K0) |
| [0016](0016-black-box-verdicts.md) | Verdicts are black-box; system internals only confirm faults and explain | Accepted (K0) |
| [0017](0017-independent-gate.md) | Independent gate binary with evidence-level negative controls | Accepted (K0) |
| [0018](0018-runner-in-container.md) | A single runner process inside the cluster network writes the history | Accepted (K0) |
| [0019](0019-gomod-no-redundant-toolchain-line.md) | go.mod has no `toolchain` line equal to its `go` line | Accepted (K1, contract 1.0.1) |
| [0020](0020-base-image-digests-k1.md) | Base image digests re-resolved in K1; etcd digest locked | Accepted (K1, contract 1.0.2) |
| [0021](0021-node-processes-share-one-uid.md) | The two processes of a node run as one user (uid 999) | Accepted (K1, contract 1.0.3) |
| [0022](0022-slowfs-node-layer.md) | The slowfs node layer: built FROM the local upstream-derived tag, launched as PID 1 | Accepted (K3, contract 1.0.4) |
| [0023](0023-nemesis-lifecycle-from-runner.md) | Nemesis lifecycle commands from the runner: n4 created at run start, nem-runner started by the runner | Accepted (K3, contract 1.0.4) |
| [0024](0024-k4-schedules-fixed-before-runs.md) | K4 schedules, directed-schedule timing, the probe checker and minimization details, fixed before any K4 run | Accepted (K4, contract 1.0.5) |
| [0025](0025-h1-profile-derivation.md) | Profile h1: images from the historical commit, its demo topology, and a HEALTHY predicate it can answer | Accepted (K4, contract 1.0.5) |
| [0026](0026-f5b-during-f1a-confirmed-after-heal.md) | "F5b during F1a(leader)": F5b's confirmation is evaluated after F1a heals | Accepted (K4, contract 1.0.6) |
| [0027](0027-d2-n4-unavailable.md) | D2′: D2 makes n4 unavailable when the failover recruits | Accepted by the owner (K4, contract 1.0.7) |
| [0028](0028-c5-heals-slow-disk-first.md) | C5 heals the slow disk before waiting for PostgreSQL's restart | Accepted (K4; amends ADR-0024's C5 shape) |
| [0029](0029-k5-contract-defects.md) | Three contract defects found before any K5 run: the buffer timing guard (G8), the LSN that W7 confirms (G11), W8's roles under S03's pg_hba (G12) | Accepted (K5, contract 1.0.8) |
| [0030](0030-k5-schedules-fixed-before-runs.md) | K5 schedules, workload details, C8 completions, the slot precondition and evidence storage, fixed before any K5 run | Accepted (K5, contract 1.0.9) |
| [0031](0031-d3-commits-in-flight-proposed.md) | D3′: hold commits in synchronous replication when the leader is killed (amendment of §9.2 D3 for M3) | Proposed (K5) — not applied; owner decision |

New ADRs continue the numbering. Each ADR: Context, Decision, Alternatives rejected, Consequences, Sources.
