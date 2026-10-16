# Derivation of `deploy/compose.yaml` from upstream (CONTRACT §5.1, ADR-0001)

Source (S03): `demo/k8s/*.yaml` of Multigres at `e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73`
(paths below are relative to `demo/k8s/` at that commit). Every flag of every
CommitProof process is listed with the upstream line it is copied from, or with
the reason it diverges. Divergences allowed by CONTRACT §5.1: service IDs,
cells, addresses, volume layout, OpenTelemetry/pprof flags removed, `--hostname`
set to the service name, and (contract 1.2.1, ADR-0038) the OpenTelemetry
Prometheus exporter enabled on multipooler, multiorch and multigateway instead of
S03's OTLP push. Anything else that differs is listed under
"Other divergences" with its reason.

Abbreviations: `MP` = `k8s-multipooler-statefulset.yaml`, `MO` =
`k8s-multiorch.yaml`, `MG` = `k8s-multigateway.yaml`, `ET` = `k8s-etcd.yaml`,
`CM` = `k8s-createclustermetadata-job.yaml`, `GC` = `k8s-generate-certs-job.yaml`,
`PW` = `k8s-postgres-password-secret.yaml`.

## pgctld (`pg-n1` … `pg-n4`, image `commitproof/pgctld:real`)

| Flag / setting | Value | Source |
|---|---|---|
| command | `/usr/local/bin/pgctld server` | MP:210, MP:212 |
| `--pooler-dir` | `/data` | MP:213 |
| `--grpc-port` | `16200` | MP:214 |
| `--pg-port` | `5432` | MP:215 |
| `--grpc-socket-file` | `/data/pgctld.sock` | MP:216 |
| `--pg-hba-template` | `/etc/pgctld/pg_hba_template.conf` (file `deploy/pg_hba_template.baseline.conf` = the ConfigMap `pg-hba-template` of MP:9–44 with one line changed, below; `deploy/pg_hba_template.conf` is the ConfigMap verbatim, used by profile h1) | MP:217, MP:241 |
| `--pgbackrest-port` | `8432` | MP:218 |
| `--pgbackrest-cert-dir` | `/certs` | MP:219 |
| `--pprof-http=true` | removed | allowed divergence (pprof) |
| env `PGDATA` | `/data/pg_data` | MP:252 |
| env `POSTGRES_PASSWORD_FILE` | `/var/run/secrets/multigres/password` (file `deploy/postgres-password` = `postgres`, the demo secret's value) | MP:257, PW:27 |
| mounts | `/data-backing` (per-node volume `data-nX`; `/data` is the slowfs pass-through of it), `/certs`, `/backups` (shared volumes) | MP:230–244; volume layout divergence below; slow-disk layer below (K3) |
| entrypoint | `/usr/local/bin/slowfs run --backing=/data-backing --mount=/data --control=/run/slowfs/control --bypass=pg_sockets --uid=999 --gid=999 -- /usr/local/bin/pgctld` (then the flags above) | CONTRACT §8 slowfs (ADR-0006, ADR-0022): PID 1 mounts the slow-disk pass-through on `/data` before pgctld starts, bind-mounts the volume's `pg_sockets` over it, runs pgctld as 999 (the K1 process user) and reaps orphans |
| container user, devices, capabilities | `user: "0:0"` (the launcher; pgctld and PostgreSQL run as 999), `/dev/fuse`, `CAP_SYS_ADMIN`, `apparmor:unconfined` | CONTRACT §8 slowfs; container-scoped (no privileged mode); AppArmor's default profile forbids `mount` (no-op without AppArmor) |

Image (K3): `commitproof/pgctld:real` is `commitproof/pgctld-upstream:real` (the FROM-only derivation of upstream's `Dockerfile.pgctld`, K1.3) plus `fuse3` and the `slowfs` binary in a CommitProof-owned stage (`build/Dockerfile.slowfs`, CONTRACT §5.2, ADR-0022). pgctld's binary and flags are unchanged.

## multipooler (`pooler-n1` … `pooler-n4`, image `commitproof/multigres:real`)

Runs in the network namespace of `pg-nX` (`network_mode: service:pg-nX`), as the
pooler and pgctld containers of one pod share localhost (MP:107–257).

| Flag | Value | Source |
|---|---|---|
| command | `/multigres/bin/multipooler` | MP:110 |
| `--http-port` | `16000` | MP:112 |
| `--grpc-port` | `16100` | MP:113 |
| `--topo-global-server-addresses` | `etcd:2379` | MP:114 |
| `--topo-global-root` | `/multigres/test/global` | MP:115 |
| `--cell` | `zone1` (n1, n4), `zone2` (n2), `zone3` (n3) | MP:116 (`zone1`); allowed divergence (cells, CONTRACT §5.1) |
| `--database` | `postgres` | MP:117 |
| `--table-group` | `default` | MP:118 |
| `--shard` | `0-inf` | MP:119 |
| `--service-id` | `n1` … `n4` | MP:120 (`$(POD_INDEX)`); allowed divergence (service IDs) |
| `--pgctld-addr` | `localhost:16200` | MP:121 |
| `--grpc-socket-file` | `/data/multipooler.sock` | MP:125 |
| `--connpool-global-capacity` | `50` | MP:126 |
| `--service-map` | `grpc-pooler` | MP:127 |
| `--pprof-http=true` | removed | allowed divergence (pprof) |
| `--enable-slot-based-replication` | `true` | MP:132 |
| `--hostname` | `pooler-nX` | not set upstream (the pod name resolves through the headless Service); allowed divergence (`--hostname` set to the service name). `pooler-nX` is a network alias of `pg-nX` on `cpnet` because a container in another container's network namespace has no DNS name of its own |
| env `PGDATA`, `POSTGRES_PASSWORD_FILE` | as pgctld | MP:200, MP:202 |
| OTel env (`OTEL_SERVICE_NAME`, `otel-config`) and `/etc/otel` mount | removed | allowed divergence (OpenTelemetry) |
| env `OTEL_METRICS_EXPORTER=prometheus`, `OTEL_EXPORTER_PROMETHEUS_HOST=0.0.0.0`, `OTEL_EXPORTER_PROMETHEUS_PORT=9464` (anchor `x-otel-env`) | added | allowed divergence (CONTRACT §5.1, P23, ADR-0038): the pinned autoexport's Prometheus exporter serves `/metrics` on 9464 for the runner's scrapes; S03 pushes OTLP to a collector instead (`k8s-observability.yaml` ConfigMap `otel-config`: `OTEL_METRICS_EXPORTER=otlp`, interval 5 s) and no HTTP port of the pinned commit serves `/metrics` (record `metrics-probe-2`, evidence/pre-K7); `OTEL_METRICS_EXPORTER` is `${CP_OTEL_METRICS_EXPORTER:-prometheus}`: `none` (the K1–K6 configuration) only for ADR-0041's exporter measurement, refused by `commitproof campaign` |
| process user | `999:999` (the pgctld image's postgres user) instead of the image's `65532:65532` | allowed divergence (CONTRACT §5.1, ADR-0021): both processes of a node share `/data`, the 0600 pgBackRest key and `/backups` |

## multiorch (`orch-z1` … `orch-z3`)

| Flag | Value | Source |
|---|---|---|
| command | `/multigres/bin/multiorch` | MO:22 |
| `--http-port` | `17000` | MO:24 |
| `--grpc-port` | `17100` | MO:25 |
| `--hostname` | `orch-zX` | MO:29 (`$(POD_IP)`); allowed divergence (service name) |
| `--topo-global-server-addresses` | `etcd:2379` | MO:30 |
| `--topo-global-root` | `/multigres/test/global` | MO:31 |
| `--cell` | `zone1` / `zone2` / `zone3` | MO:32 (`zone1`); allowed divergence (cells) |
| `--watch-targets` | `postgres` | MO:33 |
| `--pooler-health-check-interval` | `500ms` | MO:34 |
| `--recovery-cycle-interval` | `500ms` | MO:35 |
| `--pprof-http=true` | removed | allowed divergence (pprof) |
| OTel env (`OTEL_SERVICE_NAME`, `otel-config`) and `/etc/otel` mount | removed | allowed divergence (OpenTelemetry) |
| env `OTEL_METRICS_EXPORTER=prometheus`, `OTEL_EXPORTER_PROMETHEUS_HOST=0.0.0.0`, `OTEL_EXPORTER_PROMETHEUS_PORT=9464` (anchor `x-otel-env`) | added | allowed divergence (CONTRACT §5.1, P23, ADR-0038): the pinned autoexport's Prometheus exporter serves `/metrics` on 9464 for the runner's scrapes; S03 pushes OTLP to a collector instead (`k8s-observability.yaml` ConfigMap `otel-config`: `OTEL_METRICS_EXPORTER=otlp`, interval 5 s) and no HTTP port of the pinned commit serves `/metrics` (record `metrics-probe-2`, evidence/pre-K7); `OTEL_METRICS_EXPORTER` is `${CP_OTEL_METRICS_EXPORTER:-prometheus}`: `none` (the K1–K6 configuration) only for ADR-0041's exporter measurement, refused by `commitproof campaign` |
| replicas | 3 (one per cell) instead of 1 | CONTRACT §5.1 / ADR-0001 (three cells, S42) |

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
| `--cell` | `zone1` / `zone2` / `zone3` | MG:33 (`zone1`); allowed divergence (cells) |
| `--pprof-http=true` | removed | allowed divergence (pprof) |
| `--enable-slot-based-replication` | `true` | MG:37 |
| OTel env (`OTEL_SERVICE_NAME`, `otel-config`) and `/etc/otel` mount | removed | allowed divergence (OpenTelemetry) |
| env `OTEL_METRICS_EXPORTER=prometheus`, `OTEL_EXPORTER_PROMETHEUS_HOST=0.0.0.0`, `OTEL_EXPORTER_PROMETHEUS_PORT=9464` (anchor `x-otel-env`) | added | allowed divergence (CONTRACT §5.1, P23, ADR-0038): the pinned autoexport's Prometheus exporter serves `/metrics` on 9464 for the runner's scrapes; S03 pushes OTLP to a collector instead (`k8s-observability.yaml` ConfigMap `otel-config`: `OTEL_METRICS_EXPORTER=otlp`, interval 5 s) and no HTTP port of the pinned commit serves `/metrics` (record `metrics-probe-2`, evidence/pre-K7); `OTEL_METRICS_EXPORTER` is `${CP_OTEL_METRICS_EXPORTER:-prometheus}`: `none` (the K1–K6 configuration) only for ADR-0041's exporter measurement, refused by `commitproof campaign` |
| replicas | 3 (one per cell) instead of 5 behind one Service | CONTRACT §5.1 (clients address each gateway) |

## etcd (`etcd`)

| Flag | Value | Source |
|---|---|---|
| image | `quay.io/coreos/etcd:v3.6.4@sha256:885a73b0…` | ET:39 uses `v3.5.17`; CONTRACT §5.1/§14 pins v3.6.4 (the version upstream bundles, S01) by digest |
| command | `/usr/local/bin/etcd` | ET:41 (`etcd`; absolute path in the distroless image) |
| `--name` | `etcd-0` | ET:42 |
| `--data-dir` | `/var/lib/etcd` | ET:43 |
| `--listen-client-urls` | `http://0.0.0.0:2379` | ET:44 |
| `--advertise-client-urls` | `http://etcd:2379` | ET:45 (`etcd-0.etcd`); address divergence (Compose DNS name) |
| `--listen-peer-urls` | `http://0.0.0.0:2380` | ET:46 |
| `--initial-advertise-peer-urls` | `http://etcd:2380` | ET:47; address divergence |
| `--initial-cluster` | `etcd-0=http://etcd:2380` | ET:48; address divergence |
| `--initial-cluster-state` | `new` | ET:49 |
| `--listen-metrics-urls` | `http://0.0.0.0:2381` | ET:50 |
| healthcheck | `etcdctl endpoint health` every 1 s | translation of the readiness probe (ET:64–66, `/readyz`); gates `cluster-metadata` |

## Nemesis helpers (`nem-*`, image `commitproof/nemesis:real`) and runner (K3)

Not in the demo (CommitProof's own fault-injection layer, CONTRACT §5.1, §8, ADR-0005). One `nem-<svc>` per network namespace (`etcd`, `pg-n1…n4`, `orch-z1…z3`, `gw-z1…z3`) with `network_mode: service:<svc>`, `cap_drop: [ALL]`, `cap_add: [NET_ADMIN]`, command `cpnet idle`; `nem-pg-n4` is in the `n4` profile. `nem-runner` is started by the runner in its own namespace at run time (ADR-0023). The runner service adds `group_add: [<gid of /var/run/docker.sock>]` and `DOCKER_CONFIG=/tmp/.docker` so its docker CLI can drive the nemesis through the mounted socket (ADR-0018, ADR-0023).

## Init jobs

| Service | Translation of | Flags / behavior |
|---|---|---|
| `cluster-metadata` | CM | `multigres createclustermetadata --global-topo-address=etcd:2379` (CM:18) `--global-topo-root=/multigres/test/global` (CM:19) `--cells=zone1,zone2,zone3` (CM:20 has `zone1`; cells divergence) `--durability-policy=AT_LEAST_2` (CM:21) `--backup-location=/backups` (CM:22). Runs once after etcd is healthy; poolers, orchestrators and gateways start after it completes. |
| `cluster-init` | GC and the hostPath volume setup of MP:259–297 | Generates the pgBackRest TLS material with Go `crypto/x509` inside the pinned runner image instead of `alpine/openssl:latest` (an unpinned `latest` image is a K1 rejection condition): RSA-4096 self-signed CA `CN=Multigres Root CA`, 3650 days (GC:21–24); RSA-2048 `CN=pgbackrest`, keyUsage critical digitalSignature+keyEncipherment, extKeyUsage serverAuth+clientAuth, 3650 days (GC:37–38, GC:49–59); CA key discarded (GC:70); chain verified (GC:82). SANs: `pgbackrest`, `localhost`, `127.0.0.1` (GC:43–45) plus `pg-n1…4`, `pooler-n1…4` replacing the Kubernetes wildcard `*.multipooler-zone1.default.svc.cluster.local` (GC:42; address divergence). File modes: `ca.crt`, `pgbackrest.crt` 0644 (GC:74); `pgbackrest.key` 0600 (GC:73) owned by the node user 999 (ADR-0021; pgBackRest refuses a key with group or other permissions unless root owns it). Also gives the shared volumes `/backups` and `/data-nX` to uid 999 (mode 0755): in the demo, both containers of a pod write the same hostPath directories (MP:264–282). |

## Other divergences

| Item | Upstream | CommitProof | Reason |
|---|---|---|---|
| Volume layout | hostPath `/data/<pod>` per pod, `/data/certs`, `/data/backups`; pod-local emptyDir `/data/pg_sockets` (MP:157–169, MP:259–282) | Docker named volumes `data-nX` (per node, shared by `pg-nX` and `pooler-nX`), `certs`, `backups` (shared by all nodes) | allowed divergence (volume layout). The `pg_sockets` overlay is a macOS/virtiofs workaround (MP:162–167) not needed on Linux volumes |
| Cert copy init container | `setup-certs` copies a Secret into `/certs` (MP:89–105) | `cluster-init` writes the certificates directly into the shared `certs` volume | same files at the same paths; no Kubernetes Secret in Compose |
| Restart policy | Kubernetes restarts failed containers | `restart: "no"` everywhere | CONTRACT §15 K1 rejection condition; faults must not be healed by the orchestrator (ADR-0001) |
| Probes | liveness/readiness/startup probes (MP:138–155, MO:52–64, MG:58–69, ET:58–69) | none (etcd healthcheck only, above) | a probe-driven restart would heal faults on its own schedule; readiness is checked by the HEALTHY predicate (CONTRACT §5.1) |
| pg_hba local line (K5) | `local all all peer` (MP:28) | `local all all trust` (`deploy/pg_hba_template.baseline.conf`) | allowed divergence (CONTRACT §5.1, ADR-0029): the pooler's per-user pools connect as the client's role over the Unix socket, where `peer` refuses every role but the pooler's OS user, so W8's `cp_storm`/`cp_steady` could not run; every `host` line is already `trust` and the postgres user's line is unchanged |
| Profile `buffered` (K5) | — | `deploy/compose-buffered.yaml` restates each gateway's command with `--buffer-enabled=true` appended | CONTRACT §5.5 (defaults otherwise, S14) |
| Profile `slotless` (K6b) | `--enable-slot-based-replication=true` on poolers and gateways (S03) | `deploy/compose-slotless.yaml` restates the command of `pooler-n1`…`pooler-n4` and `gw-z1`…`gw-z3` with that value `false`, nothing else changed | CONTRACT §5.5, ADR-0041 (K7 cell V2) |
| Image HEALTHCHECKs | the upstream Dockerfiles declare HEALTHCHECKs; Kubernetes ignores them | disabled (`healthcheck: {disable: true}`) | same behaviour as the demo; with `restart: "no"` they would only add periodic processes to every container |
| Images | `docker.io/multigres/*:latest` (MP:108, MP:208, MO:20, MG:20, CM:13) | `commitproof/multigres:real`, `commitproof/pgctld:real` built from the pinned commit (CONTRACT §5.2, ADR-0002); `cluster up --mutant M<n>` runs the Multigres services on `commitproof/multigres:M<n>` (`CP_MULTIGRES_TAG`, K4) | pin by source; mutants are the pinned commit + one patch (§9.1) |
| multiadmin, multiadmin-web, observability | deployed by the demo | not deployed | not part of the §5.1 topology |
