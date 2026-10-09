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
| [0014](0014-language-and-toolchain.md) | English documentation; Go 1.26.9 via GOTOOLCHAIN | Accepted (K0) |
| [0015](0015-historical-control.md) | Historical control H1 (issue #778) in addition to mutants | Accepted (K0) |
| [0016](0016-black-box-verdicts.md) | Verdicts are black-box; system internals only confirm faults and explain | Accepted (K0) |
| [0017](0017-independent-gate.md) | Independent gate binary with evidence-level negative controls | Accepted (K0) |
| [0018](0018-runner-in-container.md) | A single runner process inside the cluster network writes the history | Accepted (K0) |

New ADRs continue the numbering. Each ADR: Context, Decision, Alternatives rejected, Consequences, Sources.
