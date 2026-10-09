# ADR-0012 — License: Apache-2.0

Status: Accepted (K0, contract 1.0.0)

## Context

The project is free and open source by the owner's choice. Mutant patches are derived from Multigres (Apache-2.0). Dependencies: Porcupine (MIT), pgx (MIT), pglogrepl (MIT), go-fuse (BSD-3-Clause). Elle is EPL-2.0/GPL-2.0+CPE and is only executed as an external tool (ADR-0004).

## Decision

Apache-2.0 for the whole repository, with a `NOTICE` file crediting Multigres for the derived Dockerfiles and patches. Commits are DCO-signed (`git commit -s`) so mutant patches and fixes can be offered upstream without rework (S31).

## Alternatives rejected

- **MIT:** compatible, but patches derived from Apache-2.0 code must carry Apache terms anyway; one license is simpler for upstream.
- **GPL/AGPL:** incompatible with offering patches upstream under Apache-2.0.

## Sources

S01, S31, S70, S74, S89.
