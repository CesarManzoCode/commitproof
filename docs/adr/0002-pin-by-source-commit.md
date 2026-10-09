# ADR-0002 — Pin Multigres by source commit; build images; reproducibility on binary hashes

Status: Accepted (K0, contract 1.0.0)

## Context

The brief asked for Multigres "fixed by commit or image digest, brought up from zero with the same digest". Facts: the only release, v0.1.0 (2026-05-30), is months behind the code the alpha runs (S04, S48); published images live on GHCR with nightly tags, but the registry could not be inspected from the research environment (S04); mutants need builds from source anyway (ADR-0007). Upstream Dockerfiles use a floating builder tag and install pgBackRest from an unpinned apt repository (S02), so image digests from two builds will differ even when the code is identical. `.dockerignore` excludes `.git`, so Go binaries carry no VCS stamp (S02), and `make build-release` builds with `CGO_ENABLED=0` and explicit ldflags.

## Decision

- Pin source commit `e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73` (main HEAD on 2026-10-09).
- Build `commitproof/multigres` and `commitproof/pgctld` from upstream's Dockerfiles copied into `build/` with only `FROM` lines changed to digest-pinned references (§14). A test enforces "only FROM lines differ".
- `images.lock.json` records commit, Dockerfile hashes, base digests, local image IDs, sha256 of every binary, `pgbackrest version`, mutant patch hashes. `cluster up` refuses to run if local images differ from the lock.
- Reproducibility gate (K1.2): a forced rebuild (`docker builder prune -af`, `--no-cache`) yields byte-identical binaries. Image digest equality is **not** required.

## Alternatives rejected

- **Pull published nightly by digest.** Could not verify the registry or that a nightly exists for the exact commit; mutants would still need a source build, giving two provenance paths.
- **Pin v0.1.0.** Tests code that the alpha no longer runs; misses the membership and slot-failover work of June–October 2026.
- **Require identical image digests.** Impossible with unpinned apt packages without rewriting upstream Dockerfiles substantially; a gate that cannot pass is a contract defect (lesson from AgentTruth K1).
- **Own Dockerfiles written from scratch.** More reproducible, but diverges from how upstream builds its images; reviewers would have to trust our packaging.

## Consequences

- Claims say "Multigres e3cbdbb built from source with upstream Dockerfiles".
- pgBackRest version is recorded, not pinned; a drift appears as a lock difference, not silently.
- K1 resolves etcd's digest (not loadable during research, S86) and re-resolves all others.

## Sources

S02, S04, S48, S81–S86.
