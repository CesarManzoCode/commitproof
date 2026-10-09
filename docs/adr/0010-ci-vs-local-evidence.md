# ADR-0010 — What runs in GitHub Actions and what is recorded local evidence

Status: Accepted (K0, contract 1.0.0)

## Context

Free public-repo runners: 4 CPU, 16 GB RAM, 14 GB SSD, 6 h per job (S88). The full K7 matrix needs many hours. Pushing to GitHub is publication, which only the owner may do.

## Decision

- CI runs `make` targets: unit + independence + parameter tests; checker corpus, sabotage controls and the Elle oracle; image build + cluster smoke; one nemesis smoke per fault kind among F1a, F2a, F3a, F7 (F4a if `/dev/fuse` exists on the runner, else recorded as skipped with reason); M1 directed smoke; report byte-regeneration.
- Full campaigns, all mutants, statistics and H1 are local evidence: sealed run directories committed under `evidence/` (zstd-compressed, ≤ 200 MB total, P25); uncited runs summarized by seal root if the budget is exceeded.
- Every sprint gate requires the CI-equivalent targets to pass locally. A remote CI link is recorded only after the owner pushes.

## Alternatives rejected

- **Everything in CI:** exceeds job limits and would make CI the only evidence (not reproducible offline).
- **Nothing in CI:** loses continuous protection of checkers and determinism, which are cheap.
- **Self-hosted runner on the owner's machine:** exposes the machine to a public repository's workflows.

## Sources

S88.
