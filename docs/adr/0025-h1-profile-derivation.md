# ADR-0025 — Profile h1: images from the historical commit, its demo topology, and a HEALTHY predicate it can answer

Status: Accepted (K4, contract 1.0.5). Corrects CONTRACT §5.5 (profile h1), §14 (pins) and, for h1 only, §5.1's HEALTHY predicate.

## Context (defects)

§5.5 defines `h1` as the topology for commit `4bca1d5770a6bd425bde5a61eaad240431242cd5` (issue #778, S51) "with flags taken from that commit's `demo/k8s/*.yaml`"; §9.4 classifies H1 as NOT_DEPLOYABLE only if that topology "cannot reach HEALTHY after deriving its flags". Three things at that commit make the contract's literal requirements unanswerable for reasons unrelated to deployability:

1. **The images do not build FROM-only with the tag's current digest.** The commit's `Dockerfile` and `Dockerfile.pgctld` start `FROM golang:1.25-alpine` and run `apk add --no-cache git=2.52.0-r0 make=4.4.1-r3 bash=5.3.3-r1`. `docker buildx imagetools inspect golang:1.25-alpine` (2026-10-11) → `sha256:1ae0735f…3af7`… (go1.25.14 on Alpine 3.24.1); building with it fails: `ERROR: unable to select packages: bash-5.3.9-r1: breaks: world[bash=5.3.3-r1] git-2.54.0-r0: breaks: world[git=2.52.0-r0] make-4.4.1-r4: breaks: world[make=4.4.1-r3]`. The packages pinned by the commit are Alpine 3.23's: `golang:1.25-alpine3.23@sha256:27b3069b25647beacd3745b0b2c83e2f9b8d3935455a32cd4531236ee1c960e0` (go1.25.14, Alpine 3.23.5) installs them (`apk add … → OK`) and both images build. `go.mod` at that commit says `go 1.25.1`.
2. **No `multigres.version()`.** Through a gateway of the h1 topology: `select multigres.version()` → `ERROR: function multigres.version() does not exist … HINT: No function matches the given name and argument types`; the `multigres` schema holds `heartbeat`, `durability_policy`, `leadership_history`, `tablegroup`, `tablegroup_table`, `shard`. §5.1 (d) cannot hold.
3. **No rule model, a different pooler API.** The commit predates rules (S06): `proto/multipoolermanagerdata.proto` has `Status{pooler_type, primary_status, replication_status, …, postgres_role, wal_position, consensus_term}`; there is no `multigres.current_rule`/`rule_history`. §5.1 (a)/(b) are stated in rule terms and CommitProof's observation code is compiled against the pinned commit's protos; the leader role (§6.3) is resolved the same way.

An exploratory bring-up of the derived topology (images built as above, `docker compose -f deploy/compose-h1.yaml up`) bootstrapped within 75 s: n1 out of recovery with `synchronous_standby_names = ANY 1 ("zone1_n2", "zone1_n3")`, n2 and n3 in recovery, and a write through each of gw-z1, gw-z2, gw-z3 succeeded. Classifying H1 NOT_DEPLOYABLE because CommitProof's predicate speaks a newer API would be a statement about the harness, not about the commit.

## Decision

- **Images.** `build/h1/Dockerfile` and `build/h1/Dockerfile.pgctld` are the commit's files with only their `FROM` lines changed: `golang:1.25-alpine3.23@sha256:27b3069b…60e0` for the builder stages (the Alpine release whose packages the commit pins), and the contract's `debian:bookworm-slim` and `postgres:17.7` digests (§14) for the final stages. `commitproof images build --profile h1` checks the FROM-only derivation against the commit's own files (as K1.3 does for the pinned commit), builds `commitproof/multigres:h1`, `commitproof/pgctld-upstream:h1` and `commitproof/pgctld:h1` (the same slowfs layer as the real node image, `build/h1/Dockerfile.slowfs`) from `.cache/multigres-h1` at that commit, and records them in `images.lock.json` (variant `h1`).
- **Topology** (`deploy/compose-h1.yaml`, derivation `deploy/DERIVATION-h1.md`): the commit's demo is one cell (`--cells=zone1`, `--durability-policy=ANY_2`), three poolers, one multiorch, five gateways behind a Service. CommitProof runs n1–n3 (zone1), orch-z1 (zone1) and gw-z1…gw-z3 (all zone1; three gateways because the workload clients address three), with the baseline's structural divergences (addresses, volumes, slowfs node layer, helpers, runner, `restart: "no"`, pprof/OTel removed, `--hostname` = service name, pooler user 999). No n4.
- **HEALTHY for h1** (evaluated by the runner, `internal/runner/h1.go`): (a') exactly one of n1–n3 is out of recovery (`select pg_is_in_recovery()` through psql in the node container); (b') that primary's `synchronous_standby_names` names exactly the other two nodes and its `pg_stat_replication` shows both `streaming`; (c) unchanged (a write and a read-back through every gateway); (d') every Multigres container runs `commitproof/multigres:h1` and every node `commitproof/pgctld:h1`, whose IDs `cluster up` checked against the lock (the commit is identified by the images built from it, as it has no version function).
- **Leader role for h1**: the node out of recovery, when exactly one is. **Observations for h1**: the same SQL read per node every second (P24) instead of the pooler Status RPC (diagnosis only, ADR-0016).
- CONTRACT §5.5's h1 row, §9.4 and §14 (new pin row for the h1 builder image) are amended (1.0.4 → 1.0.5).

## Alternatives rejected

- **NOT_DEPLOYABLE.** The topology bootstraps and serves writes; the contract requires evidence of a failed derivation, not of an incompatible observer.
- **Keep `golang:1.25-alpine` and unpin the apk packages.** Edits a `RUN` line: not a FROM-only derivation.
- **Pick a Docker Hub digest of `golang:1.25-alpine` from March 2026.** The registry API does not list historical digests of a tag; any choice would be unverifiable. `1.25-alpine3.23` is a published tag whose content provably satisfies the commit's own pins.
- **Generate the commit's protos under another import path and observe through the pooler API.** A second copy of generated Multigres code inside CommitProof for one control; PostgreSQL's own state answers the same questions with less code.
- **Three cells as in S51's local-provisioner setup.** §5.5 requires the commit's `demo/k8s` flags, which are single-cell.

## Consequences

- H1 is classified from DH1's five runs with the ordinary checkers (C1 `lost-write` = DETECTED in ≥ 1 run, §9.4).
- An h1 run's manifest names `multigres_commit` = the historical commit and variant `h1`.
- LIMITATIONS: H1 runs a single-cell topology with one orchestrator; S51's report used three zones.

## Sources

S33, S51, S06; CONTRACT §5.1, §5.5, §9.4, §14; ADR-0002, ADR-0015, ADR-0016, ADR-0022.
