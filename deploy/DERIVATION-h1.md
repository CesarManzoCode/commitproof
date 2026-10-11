# Derivation of `deploy/compose-h1.yaml` (CONTRACT §5.5 profile `h1`, §9.4; ADR-0015, ADR-0025)

Source: `demo/k8s/*.yaml` of Multigres at the historical commit
`4bca1d5770a6bd425bde5a61eaad240431242cd5` (issue #778, S51; S33), read from
`.cache/multigres-h1` (fetched by `commitproof images build --profile h1`).
Paths below are relative to `demo/k8s/` at that commit. Every flag of every
process is listed with the line it is copied from, or with the reason it
diverges. The structural divergences are those of the baseline
(`deploy/DERIVATION.md`): addresses, volume layout, the slowfs node layer,
OpenTelemetry/pprof flags removed, `--hostname` set to the service name, the
pooler's process user, `restart: "no"`, no probes, nemesis helpers and the
runner.

Abbreviations: `MP` = `k8s-multipooler-statefulset.yaml`, `MO` =
`k8s-multiorch.yaml`, `MG` = `k8s-multigateway.yaml`, `ET` = `k8s-etcd.yaml`,
`CM` = `k8s-createclustermetadata-job.yaml`, `GC` = `k8s-generate-certs-job.yaml`.

## Images

| Image | Built from | Notes |
|---|---|---|
| `commitproof/multigres:h1` | `build/h1/Dockerfile` = the commit's `Dockerfile` with only its `FROM` lines changed | builder `golang:1.25-alpine3.23@sha256:27b3069b…60e0`, final `debian:bookworm-slim@sha256:7c7b2c96…c587` (ADR-0025: the commit pins Alpine 3.23 packages, `git=2.52.0-r0 make=4.4.1-r3 bash=5.3.3-r1`; today's `golang:1.25-alpine` is Alpine 3.24.1 and cannot satisfy them) |
| `commitproof/pgctld-upstream:h1` | `build/h1/Dockerfile.pgctld` = the commit's `Dockerfile.pgctld`, `FROM` lines only | builder as above, final `postgres:17.7@sha256:20064937…a2c4` (CONTRACT §14) |
| `commitproof/pgctld:h1` | `build/h1/Dockerfile.slowfs` = `build/Dockerfile.slowfs` on `commitproof/pgctld-upstream:h1` | the same CommitProof slowfs layer as the real node image (ADR-0022); F3a lists PostgreSQL processes with `slowfs pgprocs` |

The commit's `go.mod` declares `go 1.25.1`; the builder is go1.25.14. The commit's Makefile `build-release` takes no `GIT_COMMIT` build argument.

## pgctld (`pg-n1` … `pg-n3`)

| Flag / setting | Value | Source |
|---|---|---|
| command | `/usr/local/bin/pgctld server` | MP:184, MP:186 |
| `--pooler-dir` | `/data` | MP:187 |
| `--grpc-port` | `16200` | MP:188 |
| `--pg-port` | `5432` | MP:189 |
| `--grpc-socket-file` | `/data/pgctld.sock` | MP:190 |
| `--pg-hba-template` | `/etc/pgctld/pg_hba_template.conf` (`deploy/pg_hba_template.conf`; identical to the commit's ConfigMap `pg-hba-template`, MP:5–46) | MP:191 |
| `--pgbackrest-port` | `8432` | MP:192 |
| `--pgbackrest-cert-dir` | `/certs` | MP:193 |
| `--pprof-http=true` | removed | allowed divergence (pprof) |
| env `PGDATA` | `/data/pg_data` | MP:218–219 |
| env `POSTGRES_PASSWORD` | `postgres` (the demo sets the value inline; the pinned-commit demo uses a secret file) | MP:220–221 |
| entrypoint, user, devices, capabilities | as baseline (slowfs launcher as PID 1, pgctld as 999, `/dev/fuse`, `CAP_SYS_ADMIN`, `apparmor:unconfined`) | CONTRACT §8 slowfs (ADR-0006, ADR-0022) |

## multipooler (`pooler-n1` … `pooler-n3`, in the namespace of `pg-nX`)

| Flag | Value | Source |
|---|---|---|
| command | `/multigres/bin/multipooler` | MP:110 |
| `--http-port` | `16000` | MP:112 |
| `--grpc-port` | `16100` | MP:113 |
| `--topo-global-server-addresses` | `etcd:2379` | MP:114 |
| `--topo-global-root` | `/multigres/test/global` | MP:115 |
| `--cell` | `zone1` | MP:116 |
| `--database` | `postgres` | MP:117 |
| `--table-group` | `default` | MP:118 |
| `--shard` | `0-inf` | MP:119 |
| `--service-id` | `n1` … `n3` | MP:120 (`$(POD_INDEX)`); allowed divergence (service IDs) |
| `--pooler-dir` | `/data` | MP:121 |
| `--pgctld-addr` | `localhost:16200` | MP:122 |
| `--pg-port` | `5432` | MP:123 |
| `--socket-file` | `/data/pg_sockets/.s.PGSQL.5432` | MP:124 |
| `--grpc-socket-file` | `/data/multipooler.sock` | MP:125 |
| `--connpool-global-capacity` | `50` | MP:126 |
| `--service-map` | `grpc-pooler` | MP:127 |
| `--pgbackrest-cert-file` | `/certs/pgbackrest.crt` | MP:128 |
| `--pgbackrest-key-file` | `/certs/pgbackrest.key` | MP:129 |
| `--pgbackrest-ca-file` | `/certs/ca.crt` | MP:130 |
| `--pgbackrest-port` | `8432` | MP:131 |
| `--pprof-http=true` | removed | allowed divergence (pprof) |
| `--hostname` | `pooler-nX` | not set in the demo; allowed divergence (service name), as baseline |
| env `PGDATA`, `POSTGRES_PASSWORD` | `/data/pg_data`, `postgres` | MP:173–176 |
| OTel env and `/etc/otel` mount | removed | allowed divergence (OpenTelemetry) |
| process user | `999:999` | as baseline (ADR-0021) |
| replicas | 3 (n1, n2, n3) | MP:78 (`replicas: 3`) |

There is no `--enable-slot-based-replication` at this commit's demo, and no n4 (the historical control needs no membership change).

## multiorch (`orch-z1`)

| Flag | Value | Source |
|---|---|---|
| command | `/multigres/bin/multiorch` | MO:22 |
| `--http-port` | `17000` | MO:24 |
| `--grpc-port` | `17100` | MO:25 |
| `--hostname` | `orch-z1` | MO:29 (`$(POD_IP)`); allowed divergence (service name) |
| `--topo-global-server-addresses` | `etcd:2379` | MO:30 |
| `--topo-global-root` | `/multigres/test/global` | MO:31 |
| `--cell` | `zone1` | MO:32 |
| `--watch-targets` | `postgres` | MO:33 |
| `--pooler-health-check-interval` | `500ms` | MO:34 |
| `--recovery-cycle-interval` | `500ms` | MO:35 |
| `--pprof-http=true` | removed | allowed divergence (pprof) |
| replicas | 1 | MO:9 |

## multigateway (`gw-z1` … `gw-z3`)

| Flag | Value | Source |
|---|---|---|
| command | `/multigres/bin/multigateway` | MG:22 |
| `--http-port` | `15000` | MG:24 |
| `--grpc-port` | `15100` | MG:25 |
| `--pg-port` | `15432` | MG:26 |
| `--hostname` | `gw-zX` | MG:30 (`$(POD_IP)`); allowed divergence (service name) |
| `--topo-global-server-addresses` | `etcd:2379` | MG:31 |
| `--topo-global-root` | `/multigres/test/global` | MG:32 |
| `--cell` | `zone1` (all three) | MG:33 |
| `--pprof-http=true` | removed | allowed divergence (pprof) |
| replicas | 3 instead of 5 behind one Service | MG:9; the workload addresses three gateways (CONTRACT §5.1, §7); the service names keep the baseline's `gw-z1…3` |

## etcd, init jobs

| Item | Value | Source |
|---|---|---|
| etcd | as baseline (`quay.io/coreos/etcd:v3.6.4@sha256:885a73b0…`, same flags) | ET:36 uses `v3.5.17`; same divergence and reason as baseline |
| `cluster-metadata` | `multigres createclustermetadata --global-topo-address=etcd:2379 --global-topo-root=/multigres/test/global --cells=zone1 --durability-policy=ANY_2 --backup-location=/backups` | CM:15–22 |
| `cluster-init` | as baseline, volumes of n1–n3 only | GC (the certificate layout `ca.crt`, `pgbackrest.crt`, `pgbackrest.key` is the same at both commits) |

## HEALTHY and observation (ADR-0025)

The commit has neither rules nor `multigres.version()`; the runner evaluates
h1's predicate from PostgreSQL itself: (a') exactly one of n1–n3 out of
recovery; (b') its `synchronous_standby_names` names exactly the other two
nodes (`zone1_nX`) and its `pg_stat_replication` shows both streaming; (c) a
write and read-back through each gateway; (d') the containers run the locked
h1 images. The leader role resolves to the node out of recovery;
observations read the same SQL state every second.
