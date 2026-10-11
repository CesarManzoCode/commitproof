# ADR-0022 — The slowfs node layer: built FROM the local upstream-derived tag, launched as PID 1

Status: Accepted (K3, contract 1.0.4). Corrects CONTRACT §5.2 (how the slowfs stage names its base); records the runtime design of §8's slowfs (ADR-0006) in `pg-nX`.

## Context (defect)

CONTRACT §5.2 says `commitproof/pgctld` layers slowfs and fuse3 "in a separate final stage of a CommitProof-owned Dockerfile `FROM commitproof/pgctld-upstream@<local image ID>`". BuildKit (the builder `commitproof images build` uses, `docker buildx build`) cannot resolve a local image by ID; it treats the reference as a registry name:

```
$ ID=$(docker image inspect --format '{{.Id}}' commitproof/pgctld:real)   # sha256:c5c53fbe…d88f
$ printf 'ARG BASE\nFROM ${BASE}\nRUN id\n' > Dockerfile
$ docker buildx build --build-arg BASE=$ID -t commitproof/fromtest:x .
ERROR: failed to build: failed to solve: sha256:c5c53fbe…d88f: failed to resolve source metadata for
docker.io/library/sha256:c5c53fbe…d88f: pull access denied, repository does not exist or may require authorization
```

`FROM commitproof/pgctld:real` (a local tag) does build, with the docker driver's local image store (K3 development, 2026-10-11).

Running slowfs "over `/data` inside `pg-nX` before pgctld starts" (§8, ADR-0006) also needs choices the contract leaves open, and two of them were found by running the baseline cluster on slowfs in K3 development.

## Decision

Build:

- `build/Dockerfile.pgctld` (FROM-only derivation of upstream's `Dockerfile.pgctld`, K1.3) is tagged `commitproof/pgctld-upstream:real`. `build/Dockerfile.slowfs` (CommitProof-owned) is `FROM commitproof/pgctld-upstream:real`, installs `fuse3` from the base's Debian repository (unpinned like upstream's pgBackRest; `dpkg-query -W fuse3` is locked) and copies the `slowfs` binary (CommitProof's `cmd/slowfs`, go-fuse v2.12.0).
- `commitproof images build` builds `pgctld-upstream` first, then the slowfs image, and fails unless the base tag still resolves to the image ID just built **and** that image's layers are a prefix of the slowfs image's layers. `images.lock.json` records both image IDs, `extends: {tag, id, layers_prefix: true}`, and the slowfs binary's sha256. This gives the binding `@<local image ID>` intended.
- CONTRACT §5.2 is corrected accordingly (1.0.3 → 1.0.4).

Runtime (implementation of ADR-0006; no contract text changes):

- `pg-nX` runs `slowfs run` as PID 1 (container user root, `/dev/fuse`, `CAP_SYS_ADMIN`, `apparmor:unconfined` because AppArmor's default profile denies `mount`; a no-op where AppArmor is absent). The node volume is mounted at `/data-backing`; slowfs mounts its pass-through on `/data` (`allow_other`, `default_permissions`, no writeback cache), resets the control file `/run/slowfs/control` (outside the mount) to `delay_ms=0`, then runs `pgctld server …` exactly as in K1, as uid/gid 999.
- **`pg_sockets` is bind-mounted from the backing volume over the pass-through.** PostgreSQL binds its Unix socket in `/data/pg_sockets`; multipooler, in its own container and mount namespace, dials it through the plain volume. A socket bound through FUSE belongs to the FUSE inode, so (by the kernel's Unix-socket lookup, not observed) it would not be reachable from the plain volume. With the bind, both containers see the same directory of the volume. Socket files are not data I/O; nothing that slowfs must slow passes through it.
- **No FUSE entry, attribute or negative caching.** multipooler writes files of the same volume through the plain mount (e.g. `archive_mode` in `postgresql.auto.conf` before asking pgctld to restart PostgreSQL). With go-fuse's one-second caches the first K3 cluster never became HEALTHY: PostgreSQL restarted on a stale view of `postgresql.auto.conf`, and pgBackRest's `check` failed on every node (`ERROR: [087]: archive_mode must be enabled`, pooler log 2026-10-11T16:18:51Z), so bootstrap looped (`initdb` → first backup fails → data directory removed) until P09 expired. With caches off, the same cluster reached HEALTHY in 27.5 s.
- **The launcher reaps orphans.** `pgctld` starts PostgreSQL with `pg_ctl start`, so the postmaster is re-parented to PID 1. In K1, PID 1 was `pgctld`, which does not reap foreign children; a killed postmaster would remain a zombie whose `/proc/<pid>` entry exists, which contradicts F2a's confirmation predicate ("postmaster PID absent from `/proc`"). The launcher waits for every child (as a pod's init or `docker run --init` would) and forwards termination signals to pgctld.
- slowfs's file handles do not expose go-fuse's passthrough file descriptor or `Writev`, so every file `write` and `fsync` request reaches the daemon and sleeps `delay_ms` (kernel FUSE passthrough would bypass the daemon).

## Alternatives rejected

- **`--build-context pgctld-upstream=docker-image://…` with `FROM pgctld-upstream`.** Still names the base by tag; the layer check is what binds it to the ID either way, and the plain tag is simpler.
- **Classic builder (`DOCKER_BUILDKIT=0`), which accepts `FROM <image ID>`.** Deprecated, and a second builder for one image.
- **slowfs mounted only on `/data/pg_data`.** Leaves WAL archiving spool and other pgctld files on the plain volume while §8 says "over `/data`"; the bypass is limited to the socket directory instead.
- **Keep FUSE caches and invalidate them from slowfs.** The daemon cannot see writes made through the plain volume in another container.
- **F4b now.** The pre-decided fallback applies only if slowfs at 0 ms breaks HEALTHY or the smoke checkers; after the two fixes above it does not (evidence/K3).

## Consequences

- `pg-nX` holds `CAP_SYS_ADMIN`, `/dev/fuse`, AppArmor unconfined, and a root PID 1 (pgctld and PostgreSQL still run as 999): LIMITATIONS LIM-11, LIM-27.
- Every PostgreSQL file operation crosses FUSE without kernel caching even at 0 ms; the overhead is measured in K3 NOTES (LIM-05).
- `deploy/DERIVATION.md` records the `pg-nX` divergences (entrypoint, user, volume mount point, devices).

## Sources

S02, S03, S89; CONTRACT §5.2, §8; ADR-0006.
