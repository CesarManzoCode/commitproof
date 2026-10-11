# ADR-0021 — The two processes of a node run as one user (uid 999)

Status: Accepted (K1, contract 1.0.3). Amends CONTRACT §5.1 (allowed divergences).

## Context (defect)

CONTRACT §5.1 copies the upstream pod model (S03): `pgctld` and `multipooler` of a node share `/data` (and all nodes share `/certs` and `/backups`). Its list of allowed divergences does not include the process user. The upstream images run as different users: `Dockerfile.pgctld` ends with `USER postgres` (uid 999 in `postgres:17.7`), `Dockerfile` with `USER 65532:65532`. With each image's own user the topology cannot start at the pinned commit:

```
$ commitproof cluster up --profile baseline        # first attempt, 2026-10-11T01:09Z
$ docker logs commitproof-pooler-n1-1 | grep ERROR
{"level":"ERROR","msg":"manager failed to initialize", ... "error":"failed to generate pgbackrest client config:
 failed to create directory /data/pgbackrest/spool: mkdir /data/pgbackrest/spool: permission denied"}
Error: run hooks: manager failed to reach ready state: ...    (multipooler exits 1 on n1, n2, n3)
$ docker exec commitproof-pg-n1-1 ls -la /data
drwxr-xr-x 1 postgres postgres 106 /data/pgbackrest        (created by pgctld, uid 999)
$ docker exec commitproof-pg-n1-1 tail /data/pgbackrest/log/all-server.log
ERROR: [042]: key file '/certs/pgbackrest.key' has group or other permissions
  HINT: file must have permissions u=rw (0600) or less if owned by the 'postgres' user
  HINT: file must have permissions u=rw, g=r (0640) or less if owned by root
```

pgctld (uid 999) creates `/data/pgbackrest` with mode 0755; multipooler (uid 65532) must create `/data/pgbackrest/spool` inside it. pgBackRest also refuses a TLS key readable by anyone but its owner (or root+group), so one key cannot serve two users. The demo's Kubernetes manifests set no `securityContext`, so the source gives no other identity mapping to copy.

## Decision

- `pooler-nX` runs as `user: "999:999"`, the pgctld image's postgres user. Both processes of a node then own `/data`, read the 0600 key (the demo's mode, GC:73) and write `/backups`, as the processes of one pod share the pod's files.
- The `cluster-init` job gives `data-nX` and `backups` to uid 999 (mode 0755) and writes `pgbackrest.key` 0600 owned by 999.
- CONTRACT §5.1 adds "the process user of multipooler (ADR-0021)" to the allowed divergences (contract 1.0.2 → 1.0.3). `deploy/DERIVATION.md` records it.
- Nothing else changes: same binaries, flags, process boundaries and network namespaces.

## Alternatives rejected

- **World-writable shared directories (1777) and a group-readable key.** pgctld and pgBackRest create subdirectories with 0755/0750 and pgBackRest rejects a group-readable key unless root owns it; the processes would still not share files created at run time.
- **Shared group with default POSIX ACLs on the volumes.** Depends on ACL support of the Docker volume filesystem and on an ACL tool in an init image; more moving parts than one user id, with no fidelity gain.
- **Run pgctld as 65532.** PostgreSQL's `initdb` needs a passwd entry for its user; the postgres image has none for 65532.

## Consequences

- `multigres` binaries in the pooler container run with a uid that has no passwd entry in the multigres image; pgBackRest and the pooler start normally (verified: `cluster up` reaches HEALTHY, evidence/K1).
- The orchestrators and gateways keep the image user 65532 (they share no files).

## Sources

S02, S03 (MP:207–257, GC:70–74).
