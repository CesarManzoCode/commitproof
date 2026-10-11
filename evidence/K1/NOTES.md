# K1 NOTES — Reproducible cluster and history recording

Contract **1.0.3** (1.0.0 at sprint start; corrections ADR-0019, ADR-0020, ADR-0021).
Every evidence command ran on commit `5af2749` with a clean tree (`commitproof_dirty:
false` and `runner_image_dirty: false` in the smoke run's `manifest.json`; `dirty:
false` in both build locks). Evidence produced by `scripts/k1-evidence.sh all` on
2026-10-11 between 01:43 and 01:54 UTC. No `claims.yaml` exists yet (K7
deliverable): every statement below names its command, artifact and sha256
directly, or is marked as not demonstrated.

## 1. Gate

`bin/cpgate K1 --evidence evidence/K1 --negative-controls` → exit 0 (record
`cpgate-k1` in `commands.jsonl`, stdout sha256
`16f1c0d163a4e5484aabbd413d05a985b362d529facf04033e63859459df3798`).
`gate.json` (sha256 `b2bb191649dd65560d307061b91f5fed7182866a5a410850e6f64ffb399a2d25`):
K1.1–K1.8 PASS. `negative-controls.json` (sha256
`53d8f75efa993bc160c063cf1dffea98ce4eab5dd17cfd73f2bcb72e1a1339e4`): each of the
eight sabotages failed exactly its own criterion. The sabotages that edit a
recorded command output (K1.4, K1.6) also rewrite that record's stdout hash, so
the criterion has to catch them by its own logic, not by the hash check.

## 2. Design says / system did

### K1.1 Lock
- **Design says:** §5.2 lists the lock fields; §14 pins base images by digest
  and requires K1 to re-resolve them (ADR-0002, ADR-0020).
- **System did:** `commitproof images build` (record `images-build-1`, exit 0)
  wrote `images.lock.build1.json` (sha256 `329e5a01…d707`): Multigres commit and
  date, sha256 and upstream git blob of each Dockerfile, five base digests
  (golang, debian, postgres, alpine, etcd), three local image IDs, sha256 of
  every built binary plus `/usr/bin/pgbackrest`, `pgbackrest version` =
  `pgBackRest 2.59.3` in both Multigres images, `mutant` and `patch_sha256`
  null. Every base digest equals the re-resolution recorded as
  `resolve-digests-4` (`docker buildx imagetools inspect`).
- Re-resolution found `golang:1.26.9-alpine` and `debian:bookworm-slim` moved
  since 2026-10-09; contract corrected by ADR-0020. Three earlier resolution
  attempts stay in `commands.jsonl` (`resolve-digests`, `-2`, `-3`: a quay.io
  502, an error in my script, the Docker Hub anonymous rate limit).

### K1.2 Binary reproducibility
- **Design says:** the six binaries are byte-identical after a forced rebuild;
  image digests are not expected to match (ADR-0002).
- **System did:** `docker builder prune -af` (record `builder-prune`) then
  `commitproof images build --no-cache` (record `images-build-2`, 5 min 24 s;
  sub-commands in `cmdlog/images-build-2/`). `images.lock.build2.json` (sha256
  `de30b2bf…5959`) has the same sha256 as build 1 for all six: multigres
  `6646dec2…d673`, multiadmin `567ce523…c821`, multigateway `bb6da287…6fef`,
  multiorch `5dd6ec6e…112d`, multipooler `6080a6aa…1532`, pgctld `8f718fe3…9784`.
  The image IDs differ (multigres `2c153095…` cached vs `6b2a6b1c…` rebuilt), as
  ADR-0002 anticipated (apt layers).
- In that no-cache build the **runner** image (CommitProof's own, not one of the
  six) failed twice in `go mod download` with DNS timeouts against the host's
  resolver (`lookup proxy.golang.org on 10.166.176.20:53: i/o timeout`) and was
  built on the third recorded attempt (`cmdlog/images-build-2/commands.jsonl`,
  `build-runner-attempt-1..3`). The Multigres images built in one attempt each.

### K1.3 Derivation
- **Design says:** `build/` Dockerfiles differ from upstream only in FROM lines (§5.2).
- **System did:** `make check-dockerfile-derivation` (record, exit 0) wrote
  `derivation/` with both sides; `derivation.json` sha256 `0accd93d…c07d`. Two
  FROM lines changed per file; the upstream copies hash to the pinned git blobs
  (`Dockerfile` `037648c3…`, `Dockerfile.pgctld` `83babd5d…`); `cpgate`
  recomputes the line diff itself.

### K1.4 Up/down from zero
- **Design says:** HEALTHY (§5.1 a–d) within P09 = 300 s,
  `multigres.version()` reports `e3cbdbb`, no project resources after `down`.
- **System did:** three attempts (records `updown-{1,2,3}-{up,verify,down}`),
  each starting from zero project containers, volumes and networks: HEALTHY
  after **23.7 s, 24.3 s, 25.4 s** (the predicate is evaluated every second);
  leader `n1`, decided rule (coordinator term 1, subterm 0), cohort {n1, n2,
  n3}, policy AT_LEAST_2; `multigres.version()` = `Multigres 0.1.0-SNAPSHOT
  (e3cbdbbb0b94, 2026-10-09) built with go1.26.9`; write + read-back through
  gw-z1, gw-z2, gw-z3; `cluster verify` HEALTHY with every running container on
  its locked image; after each `down`: 0 containers, 0 volumes, 0 networks. One
  `cluster up` per attempt, never retried.

### K1.5 Memory
- **Design says:** peak sum of container memory ≤ P10 = 6 GiB; INFERENCE I1
  estimated 2–3 GiB.
- **System did:** `commitproof __memsample` ran `docker stats --no-stream` every
  1 s around the smoke run (`memory-samples.jsonl`, sha256 `26f1afba…c764`; 92
  samples, 83 valid inside the run window, 9 failed overall: `docker stats`
  exits 1 when a container stops between listing and sampling). **Peak sum
  482.9 MiB.** Per-component peaks: pg-n1 98.3 MiB (leader), pg-n2 63.0, pg-n3
  60.3; pooler-n1 42.8, n2 42.1, n3 49.5; gw-z1 30.3, z2 29.0, z3 28.7; orch-z1
  18.4, z2 20.8, z3 18.6; etcd 34.3; the two runner containers of the run (HEALTHY check and workload) 29.2 and 15.7.
  This measures the 45 s smoke workload only; I1's estimate for campaigns
  (90 s, faults) remains an inference.

### K1.6 Plan determinism
- **Design says:** same inputs → byte-identical plan (§6.3).
- **System did:** `commitproof plan --campaign smoke --seed 42` three times,
  `go clean -cache`, `make build`, once more: four sha256
  `8437a28083c66135c86fda9721fe59584cbe0ceaee424ac1f05308f4eb3558b5`; seed 43:
  `8ef3b2a6…8409`. `cpgate` recomputes each hash from the printed plan. The plan
  embeds the contract version (§6.3), so it changed when the contract went to
  1.0.3 during the sprint.

### K1.7 History integrity
- **Design says:** sealed run directory (§5.4), canonical hash-chained history
  (§6.1), Jepsen completion discipline, W1–W4 all exercised.
- **System did:** `commitproof run --campaign smoke --seed 42 --out
  evidence/K1/runs` (record `smoke-run`, 88 s wall: cluster up 25.2 s, 45 s
  workload, recovery, 10 s quiescence, final reads, teardown) produced
  `runs/20261011T015033Z-smoke-42-real-8437a280/`, seal root
  `77cc860c96ef88c85d26623f86a34d4fd02371591cb5f811dbaaa1e5dd92b690`;
  `cpcheck verify-seal` exit 0. 16 178 history lines, 944 observation lines, t
  non-decreasing, every invoke answered exactly once, run-end `complete`.
- Completions of planned operations: append 2 355 ok / 568 fail (512 × 40001
  at a statement, 47 × 40001 at COMMIT, 9 × 40P01); bank 1 361 ok (881
  transfers, 480 read-alls) / 195 fail (189 × 40001, 6 × 40P01); register 1 782
  ok (745 reads, 706 writes, 331 cas); ledger 1 782 ok; **0 info**. Recovery
  (P06) succeeded with the first write through each gateway; final reads were
  stable after 3 rounds on every gateway (P08), 4 s after quiescence; the final
  bank read sums to 500.
- These counts describe what the client observed. No checker verdict exists
  before K2 and none is claimed here.

### K1.8 CI-equivalent
- `make ci-unit ci-cluster-smoke` (record `ci`, exit 0, 2 min 13 s): gofmt, go
  vet, staticcheck v0.8.1, all unit tests (including the §5.3 independence test
  and the §5.7 table test), image build (cached), cluster up/verify/down, a
  second smoke run (into the git-ignored `runs/`), `cpcheck verify-seal`.
- `.github/workflows/ci.yml` passes `make lint-workflow` (actionlint 1.7.12 by
  digest, record `lint-workflow`). It has not run on GitHub (nothing pushed).

## 3. Divergences from upstream flags

Complete list in `deploy/DERIVATION.md` (every flag with its
`demo/k8s/*.yaml` line). Summary:
- allowed by §5.1: service ids `n1…n4`, cells zone1–zone3 (n4 in zone1),
  addresses, Docker volumes instead of hostPath, OTel/pprof flags removed,
  `--hostname` = service name (`pooler-nX` is a network alias of `pg-nX`);
- ADR-0021 (added to §5.1): multipooler runs as uid 999. **Observed** at the
  first `cluster up` with each image's own user: multipooler exited 1 with
  `mkdir /data/pgbackrest/spool: permission denied`, and pgBackRest rejected a
  group-readable key;
- other: etcd v3.6.4 (contract) instead of the demo's v3.5.17; certificates
  generated by the CommitProof init job (Go crypto/x509) instead of
  `alpine/openssl:latest`, SANs `pg-nX`/`pooler-nX` instead of the Kubernetes
  wildcard; image HEALTHCHECKs disabled and no probes (Kubernetes ignores
  HEALTHCHECK; probe-driven restarts would heal faults); `restart: "no"`.
- Every flag exists in the binary's `--help` (`scripts/check-flags.sh`, record
  `check-flags`: 144 service × flag checks over 44 distinct flags, 0 missing).
  Defaults the contract relies on match the help text: gateway
  `--statement-timeout` 30s (S16), `--buffer-enabled` off (S14), multiorch
  `--leader-postgres-response-threshold` 30s, grace 4s + jitter 8s (S21).

## 4. Demonstrated / not demonstrated

Demonstrated (artifacts above): a pinned build with reproducible binaries; a
FROM-only derivation; deterministic up/down of the §5.1 baseline topology with
HEALTHY in about 25 s; seed-deterministic plans; a 45 s W1–W4 run recorded as a
sealed, hash-chained, well-formed history; a gate computed from evidence files
alone whose eight negative controls each fail only their criterion.

Not demonstrated in K1:
- any checker verdict on a history (K2);
- nemesis helpers `nem-*`, any fault, the slowfs layer of the pgctld image (K3;
  the K1 topology has no helper containers);
- `/metrics` scraping (P23) in the observation stream: K1 observes pooler
  Status RPCs only (P24); metric timelines are K6;
- memory and HEALTHY behaviour under 90 s campaigns, faults or with n4 started;
- the CI workflow on GitHub-hosted runners (needs a push by the owner, LIM-16);
- profiles other than `baseline` (`cluster up --profile buffered` exits 3).

## 5. Provenance notes

- **A first complete evidence run was discarded.** It passed the same gate with
  the same negative-control behaviour (gate run 01:38 UTC), but its records held
  the repository's absolute host path in build, compose and bind-mount argv,
  including inside its sealed smoke run, which AGENTS.md §4 forbids committing.
  Commit `5af2749` makes every recorded path repository-relative; the evidence
  was reset to the records that preceded that run (`prereq-check*`,
  `resolve-digests*`) and regenerated. The discarded run's numbers were within
  the ranges reported here (HEALTHY 23.7–25.6 s, peak 477.5 MiB, identical six
  binary hashes and plan hashes).
- `commands.jsonl` keeps the K1 BLOCKED attempt (`prereq-check`, 14 FAIL before
  the owner installed the prerequisites) and the passing re-check
  (`prereq-check-2`, 26/26).
- Docker Hub's anonymous limit (100 manifest requests per hour) was exhausted
  twice by digest inspection during the sprint; `resolve-digests.sh` now reads
  only each tag's index.
- During development a probe of Compose's relative bind mounts made Docker
  create an empty root-owned `~/runs/mounttest` outside the repository; both
  empty directories were removed with `rmdir`.
