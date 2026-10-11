# ADR-0020 — Base image digests re-resolved in K1; etcd digest locked

Status: Accepted (K1, contract 1.0.2). Amends CONTRACT §14 (research staleness, §18 step 1–4).

## Context (defect)

CONTRACT §14 pins base images by the digests observed on 2026-10-09 (RESEARCH S81–S85) and leaves etcd's digest "resolved in K1" (S86 did not load). K1 re-resolved every pin with `docker buildx imagetools inspect` (`scripts/resolve-digests.sh`, recorded in `evidence/K1/commands.jsonl` as `resolve-digests-4`; earlier attempts `resolve-digests`, `-2`, `-3` are recorded too and failed on a quay.io 502, a script error and the Docker Hub anonymous rate limit). Two tags no longer point at the contract's digests:

| Tag | Contract digest (2026-10-09) | Resolved 2026-10-10/11 | linux/amd64 image created |
|---|---|---|---|
| `golang:1.26.9-alpine` | `sha256:cdfd4fe2…84e0` | `sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade` | 2026-10-08T17:12:34Z |
| `debian:bookworm-slim` | `sha256:abd67ffc…9241` | `sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587` | 2026-10-05 (contract digest's image: 2026-08-03) |
| `postgres:17.7` | `sha256:20064937…c2a4` | unchanged | 2026-02-03 |
| `alpine:3.24.2` | `sha256:294b683c…77e6` | unchanged | 2026-09-17 |
| `clojure:temurin-21-lein-2.13.0-bookworm` | `sha256:724a7f58…c34f` | unchanged | — |
| `quay.io/coreos/etcd:v3.6.4` | resolved in K1 | `sha256:885a73b09cde0293c84adad3d589cc5d091f03e61d390b94413c0a866ccfe232` | 2025-07-25 |
| `rhysd/actionlint:1.7.12` (latest release, `git ls-remote --tags` 2026-10-10) | resolved in K1 | `sha256:b1934ee5f1c509618f2508e6eb47ee0d3520686341fec936f3b79331f9315667` | — |

Both old digests still resolve in the registry (the tags moved; the old indexes were not deleted). `debian:bookworm-slim` moved to a newer point build. For `golang:1.26.9-alpine` the linux/amd64 image of the old and new index carries the same creation time, so the index was re-published; whether the amd64 image itself changed was not determined (the per-platform comparison hit the Docker Hub rate limit).

## Decision

- `build/Dockerfile`, `build/Dockerfile.pgctld`, `build/Dockerfile.runner` and `deploy/compose.yaml` use the re-resolved digests; `images.lock.json` records them (K1.1 checks lock digests equal the recorded re-resolution).
- CONTRACT §14 rows for golang, debian and etcd are updated (contract 1.0.1 → 1.0.2); RESEARCH S81, S83 and S86 note the re-resolution.
- `scripts/resolve-digests.sh` fetches only each tag's index (one manifest GET per tag): per-platform lookups exhaust the anonymous Docker Hub limit (100 manifest GETs per hour per IP).

## Alternatives rejected

- **Keep the contract's digests (they still resolve).** §14 instructs K1 to lock the re-resolved values and record differences; keeping stale pins would also keep the Debian base two months behind its security updates.
- **Pin by tag.** Rejected by §15 K1 (any image by tag without digest).

## Consequences

- Reproducibility (K1.2) is asserted on binary hashes built from these bases; image digests of the built images are not expected to be stable (ADR-0002).
- The same staleness can recur; every sprint that rebuilds from scratch uses the locked digests, never the tags.

## Sources

S81, S83, S86; `evidence/K1/logs/resolve-digests*.stdout`.
