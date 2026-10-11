# K3 NOTES — Nemesis

Contract **1.0.4** (1.0.3 at sprint start; corrections ADR-0022, ADR-0023).
Evidence produced by `scripts/k3-evidence.sh` on commit `090a366` with a
clean tree (`commitproof_dirty: false`, `runner_image_dirty: false` in every
run manifest; cpgate K3.1 checks it), 2026-10-11 17:54–19:20 UTC, one
cluster at a time. No `claims.yaml` exists yet (K7): every statement below
names its record (in `commands.jsonl`), artifact and sha256, or is marked as
not demonstrated. "Design says" cites RESEARCH source ids; "system did" cites
run ids (all under `evidence/K3/runs/`, each a zstd tar of the sealed run,
§13) and the analysis record `k3obs` (`logs/k3obs.stdout`, diagnosis from
observations, ADR-0016: never a verdict).

## 1. Gate

`bin/cpgate K3 --evidence evidence/K3 --negative-controls` → exit 0 (record
`cpgate-k3`, 19:16:33–19:20:11 UTC, stdout sha256 `373a90be…dfafcbe`).
`gate.json` (sha256 `5cdddbf1…044704`): K3.1–K3.6 PASS.
`negative-controls.json` (sha256 `e1b16fcd…167f96`): each of the six
sabotages failed exactly its own criterion — K3.1: the f1a run record points
at the heal-sabotaged F1a run ("run built with sabotage
[cp_sabotage_heal_f1a]", not-healed); K3.2: the F2a inject-sabotage record
points at the real F2a run (no sabotage tag, no not-applied); K3.3: likewise
for heal-F3a; K3.4: one confirmation probe's recorded stdout sha256 in the
F7 run's `commands.jsonl` replaced by zeros, the run re-sealed and
re-archived, its checks.json and check record updated consistently — only
the probe-to-record comparison notices; K3.5: the F7 run's checks.json
deleted; K3.6: the CI record's exit code set to 1. `commands.jsonl` as cpgate
read it (before the `cpgate-k3` record was appended): sha256
`7be7d485…0355`.

Criteria, as cpgate recomputed them from the files:
- **K3.1** each fault applied the required times with `inject → confirmed →
  heal → healed` (F5a/F5b: `inject → confirmed`), no `not-applied` /
  `not-healed`, run-end `complete`, stable final reads recomputed from the
  history (P08), real build, clean tree: F1a–F1d, F2a–F2e, F3a, F3b, F4a,
  F7 three times each, F6 twice, F5a and F5b once; the composite's three
  elements injected and confirmed in order and healed in reverse.
- **K3.2** sixteen inject-sabotaged runs, **K3.3** two heal-sabotaged runs: §6.
- **K3.4** 46 confirmation events and 756 probe/command entries of
  confirmation and heal events verified against their `commands.jsonl`
  records (argv, exit code, stdout/stderr sha256) and the output files; no
  confirmation consists only of injection commands.
- **K3.5** cpcheck on all 16 K3.1 runs: §7.
- **K3.6** §8.

Precondition: `scripts/k3-precondition.sh` (record `k3-precondition`, exit 0)
ran `cpgate K1` and `cpgate K2` on copies of the closed evidence under the
git-ignored `runs/`: K1.1–K1.8 PASS, K2.1–K2.7 PASS.

## 2. What K3 added (design says / system did)

- **Helpers** (§5.1, ADR-0005, ADR-0023). Design: one helper per network
  namespace, `NET_ADMIN` only, no privileged container, no host `sudo`.
  System: `nem-etcd`, `nem-pg-n1…n4`, `nem-orch-z1…z3`, `nem-gw-z1…z3` in
  `deploy/compose.yaml` with `cap_drop: [ALL]`, `cap_add: [NET_ADMIN]`;
  `nem-runner` started by the runner (`docker run --cap-drop ALL --cap-add
  NET_ADMIN`, records `runner-nem-runner-start` in each run's
  `commands.jsonl`). Image `commitproof/nemesis:real` = pinned alpine 3.24.2
  + iptables (`iptables v1.8.13 (nf_tables)`) + iproute2 (`tc utility,
  iproute2-v7.0.0`) + `cpnet` (versions in `images.lock.json`). Every rule
  and qdisc was installed from inside a helper with only `NET_ADMIN`.
- **slowfs** (§8, ADR-0006, ADR-0022). Design: FUSE pass-through over `/data`
  in every `pg-nX` before pgctld starts, control file outside the mount.
  System: `pg-nX`'s PID 1 is `slowfs run`, which mounts `/data-backing` on
  `/data` (`fuse.slowfs`, `allow_other`, `default_permissions`, no caches),
  bind-mounts the volume's `pg_sockets`, then runs pgctld as 999; control
  file `/run/slowfs/control` (`delay_ms=<n>`), applied value echoed to
  `/run/slowfs/control.status`. Image `commitproof/pgctld:real` extends the
  FROM-only `commitproof/pgctld-upstream:real` (whose image ID equals K1's
  pgctld image, `sha256:c5c53fbe…`), with `fuse3 3.17.2-3` and the slowfs
  binary; the build checks the layer prefix (lock field `extends`).
- **Runner** (ADR-0018, ADR-0023): docker CLI in the runner image, the
  socket's group, a probe listener on 7999; the nemesis runs as a goroutine
  beside the workload; a not-applied/not-healed fault aborts the workload,
  the run ends INVALID without recovery or final reads.

## 3. slowfs at 0 ms and the F4b decision

Design (§8, ADR-0006): if the baseline cluster with slowfs at 0 ms fails the
HEALTHY predicate or the smoke checkers, F4b replaces F4a.

System:
- `slowfs-overhead` (record, exit 0, stdout sha256 `b6211c83…`): `cluster
  up` HEALTHY after 30.2 s; 20 write+fsync probes of 4 KiB per node, same
  container, same moment: median **0.997 / 0.996 / 1.001 ms** through
  slowfs (n1/n2/n3) against **0.914 / 0.926 / 0.925 ms** on the plain
  volume — about 70–83 µs (8–9 %) per write+fsync at delay 0. Metadata
  operations also cross FUSE without kernel caching; their cost was not
  measured separately.
- `slowfs-smoke-run` (the K1 smoke campaign, seed 42, on slowfs; run
  `20261011T175902Z-smoke-42-real-914ed937`, seal root `2653ee7f…`):
  complete, HEALTHY after 26.4 s (K1 without slowfs: 23.7–25.4 s), ok counts
  append 2582, bank 1368, ledger 1782, register 1782 (K1: 2355, 1361, 1782,
  1782); `slowfs-smoke-check`: verdict **PASS**, all nine checkers valid.
- Every K3.1 run reached HEALTHY on slowfs, in 26.7–29.4 s (`cluster_up`
  in each `k3-run-*` summary).
- So the F4b condition did not occur and **F4a is used**; no ADR for F4b.
  LIM-05 now states the emulation's scope.
- It took two fixes to get there (§9): FUSE caches off (stale
  `postgresql.auto.conf`, cluster never HEALTHY) and close-on-exec on
  `/dev/fuse` (teardown deadlock).

## 4. Per fault: mechanism and confirmation

| Fault | Injection (commands in the run's commands.jsonl) | Confirmation predicate (raw outputs in the `confirmed` event) | Heal / heal predicate |
|---|---|---|---|
| F1a isolate | `iptables` chains `CPN-<id>-O/-I` in the target's namespace and in every other running namespace (incl. the runner's, via nem-runner): DROP to/from each peer address | `cpnet connect --timeout 1s` from the target's helper to every peer and from every peer's helper to the target: every attempt `timeout`; `iptables-save -c`: every OUTPUT drop rule counted ≥ 1 packet | chains removed; every pair connects; no `CPN-<id>` chain left |
| F1b split | as F1a for every A–B pair (A = leader node + orch-z1, B = every other running namespace) | every A–B pair times out both ways; every A–A and B–B pair connects; drop counters ≥ 1 | as F1a |
| F1c asymmetric | rules on the leader and on orch-z1..3, the other nodes, etcd; gateways and the runner untouched | leader↔{orch, nodes, etcd} time out both ways; leader↔gw-z1..3 connect both ways; counters ≥ 1 | as F1a |
| F1d one-way | leader only: OUTPUT DROP towards each follower; INPUT rules that only count SYNs from each follower | leader→follower times out; follower→leader times out (its SYN-ACK is dropped) and the leader's SYN counter for that follower ≥ 1; drop counters ≥ 1 | rules removed; both directions connect |
| F2a kill PostgreSQL | `docker exec pg-nX kill -9 <postmaster.pid>` | `slowfs pgprocs --pid <pid>`: `absent` (the launcher, PID 1, reaps it) | none: PostgreSQL must restart by itself — new postmaster PID alive and `psql select 1` = 1 within P28; otherwise recorded and `pgctld start` (another P28) |
| F2b kill multipooler | `docker kill -s KILL pooler-nX` | `docker inspect` State: `exited`, exit code 137 | `docker start`; running and `cpnet http 127.0.0.1:16000/ready` = 200 from the node's helper |
| F2c kill multiorch | `docker kill -s KILL orch-zX` | as F2b | `docker start`, then `docker restart` of nem-orch-zX (it still held the old namespace); running and `/ready` on 17000 = 200 |
| F2d kill multigateway | `docker kill -s KILL gw-zX` | as F2b | `docker start` + helper restart; running and 127.0.0.1:15432 accepts |
| F2e kill etcd | `docker kill -s KILL etcd` | as F2b | `docker start` + helper restart; `etcdctl endpoint health` "is healthy" |
| F3a pause PostgreSQL | `kill -STOP` of the postmaster, then of every other postgres process, re-listed up to 5 rounds | postmaster and every postgres process in state `T`; `timeout 3 psql … select 1` exits 124 | `kill -CONT`; no postgres process in `T`; `select 1` = 1 |
| F3b pause container | `docker pause` (pooler of the leader, a random orchestrator, a random gateway) | `State.Paused = true` | `docker unpause`; `Paused = false`, running |
| F4a slow disk | `slowfs control --set-ms 200` in pg-nX | baseline probe before injection: median of 5 write+fsync of 4 KiB on `/data` < 50 ms; after: median ≥ 150 ms (P17) | `--set-ms 0`; median < 50 ms |
| F5a add n4 | `docker start` pg-n4, pooler-n4, nem-pg-n4 (created at run start, ADR-0023) | on the leader (resolved each round): `synchronous_standby_names` names `zone1_n4` and `pg_stat_replication` has its row; 120 s | none |
| F5b remove n4 | `docker stop -t 30 pooler-n4`, `docker stop pg-n4`, `docker stop nem-pg-n4` | the leader's `synchronous_standby_names` no longer names `zone1_n4`; 120 s | none |
| F6 switchover | `docker kill -s TERM pooler-<leader>` | another pooler reports itself leader of a decided rule with a greater rule number; 60 s | `docker start`; running and `/ready` = 200 |
| F7 delay | `tc qdisc add dev eth0 root netem delay 50ms` in the target's helper | median of 10 TCP connects from a peer helper (etcd's) rose by ≥ 40 ms over the pre-fault median (P18) | `tc qdisc del`; median within 10 ms of baseline and no netem qdisc left |

Confirmation and heal deadlines: P27 = 15 s after the inject commands
return (F5a/F5b 120 s, F6 60 s), P28 = 30 s; predicates polled every 0.5 s.
Every `confirmed`/`healed` event carries the raw outputs of its probes and
injection/heal commands with their `commands.jsonl` name, seq and sha256
(K3.4). Examples from the K3.1 runs (details field of the first events):
F1a: 22 probe outputs (11 connect sources, 11 `iptables-save -c`), every
drop rule counted, no problem; F3a: 29 postgres PIDs all `T`, `select 1`
exit 124; F4a: median 407 179 µs after injection (threshold 150 000),
1 158 µs after heal; F5a: leader n1's `synchronous_standby_names` = `ANY 1
("zone2_n2", "zone1_n1", "zone3_n3", "zone1_n4")`, n4 `streaming`/`quorum`;
F6: old leader n1 rule 1.0 → n3 rule 2.0; F7: connect RTT 55 µs → 50 130
µs, back to 60 µs.

## 5. Observed reaction of Multigres (system did)

From `k3obs` (observations every 1 s, P24, and the client history; times
from the `inject` event, which precedes the injection command; "client gap"
= longest interval without an ok completion through a gateway between 1 s
before the fault and 10 s after its heal):

| Run (`evidence/K3/runs/…`) | Fault | Target | Confirmed after | Healed after heal | Leader / rule at inject → new leader observed (after) | Client gap per gateway z1/z2/z3 (ms) |
|---|---|---|---|---|---|---|
| `20261011T183506Z-k3-composite-1-real-1c2d4fbc` | f0.0 F5a | n4 | 12445 ms | — (no heal) | — → n1@1.1 (11.9 s) | 3497/3497/3495 |
| `20261011T183506Z-k3-composite-1-real-1c2d4fbc` | f0.1 F3a | n4 | 3242 ms | 206 ms | n1@1.1 → no change | 12658/12642/12640 |
| `20261011T183506Z-k3-composite-1-real-1c2d4fbc` | f0.2 F2a | n1 | 94 ms | 113 ms (restarted by itself: yes) | n1@1.1 → n3@3.0 (12.9 s) | 12658/12642/12640 |
| | decided rules observed in the run: 0.1, 1.0, 1.1, 3.0 | | | | | |
| `20261011T180033Z-k3-f1a-1-real-54451565` | f0 F1a | n1 | 1295 ms | 341 ms | n1@1.0 → no change | 16409/14030/17175 |
| `20261011T180033Z-k3-f1a-1-real-54451565` | f1 F1a | n3 | 1310 ms | 397 ms | n1@1.0 → no change | 61/58/63 |
| `20261011T180033Z-k3-f1a-1-real-54451565` | f2 F1a | n1 | 1308 ms | 365 ms | n1@1.0 → no change | 17442/14971/16350 |
| | decided rules observed in the run: 1.0 | | | | | |
| `20261011T180252Z-k3-f1b-1-real-9ac4fcb0` | f0 F1b | n1, orch-z1 +rest | 1304 ms | 368 ms | n1@1.0 → no change | 15625/17253/14461 |
| `20261011T180252Z-k3-f1b-1-real-9ac4fcb0` | f1 F1b | n1, orch-z1 +rest | 1289 ms | 381 ms | n1@1.0 → no change | 16287/17493/17997 |
| `20261011T180252Z-k3-f1b-1-real-9ac4fcb0` | f2 F1b | n1, orch-z1 +rest | 1289 ms | 347 ms | n1@1.0 → no change | 14266/14424/14571 |
| | decided rules observed in the run: 1.0 | | | | | |
| `20261011T180511Z-k3-f1c-1-real-d27a6b82` | f0 F1c | n2 | 1234 ms | 290 ms | n2@1.0 → no change | 9003/11863/11904 |
| `20261011T180511Z-k3-f1c-1-real-d27a6b82` | f1 F1c | n2 | 1224 ms | 318 ms | n2@1.0 → no change | 10045/10012/9852 |
| `20261011T180511Z-k3-f1c-1-real-d27a6b82` | f2 F1c | n2 | 1240 ms | 285 ms | n2@1.0 → no change | 12853/12813/9943 |
| | decided rules observed in the run: 1.0 | | | | | |
| `20261011T180728Z-k3-f1d-1-real-4a8c61a9` | f0 F1d | n1 | 1136 ms | 221 ms | n1@1.0 → no change | 10054/9188/10038 |
| `20261011T180728Z-k3-f1d-1-real-4a8c61a9` | f1 F1d | n1 | 1141 ms | 219 ms | n1@1.0 → no change | 10056/11146/10076 |
| `20261011T180728Z-k3-f1d-1-real-4a8c61a9` | f2 F1d | n1 | 1140 ms | 210 ms | n1@1.0 → no change | 12918/9953/12874 |
| | decided rules observed in the run: 1.0 | | | | | |
| `20261011T180945Z-k3-f2a-1-real-f3b8f225` | f0 F2a | n2 | 92 ms | 1829 ms (restarted by itself: yes) | n2@1.0 → n2@2.0 (6.7 s) | 6744/6695/6738 |
| `20261011T180945Z-k3-f2a-1-real-f3b8f225` | f1 F2a | n3 | 95 ms | 1823 ms (restarted by itself: yes) | n2@2.0 → no change | 59/59/57 |
| `20261011T180945Z-k3-f2a-1-real-f3b8f225` | f2 F2a | n2 | 95 ms | 2920 ms (restarted by itself: yes) | n2@2.0 → n3@3.0 (2.7 s) | 1882/1886/1921 |
| | decided rules observed in the run: 1.0, 2.0, 3.0 | | | | | |
| `20261011T181206Z-k3-f2b-1-real-fd00f96f` | f0 F2b | pooler-n2 | 150 ms | 711 ms | n2@1.0 → no change | 15864/10418/15875 |
| `20261011T181206Z-k3-f2b-1-real-fd00f96f` | f1 F2b | pooler-n1 | 133 ms | 708 ms | n2@1.0 → no change | 53/56/54 |
| `20261011T181206Z-k3-f2b-1-real-fd00f96f` | f2 F2b | pooler-n2 | 149 ms | 699 ms | n2@1.0 → no change | 10426/10436/10441 |
| | decided rules observed in the run: 1.0 | | | | | |
| `20261011T181425Z-k3-f2c-1-real-17d5a8f6` | f0 F2c | orch-z1 | 210 ms | 316 ms | — → no change | 52/55/54 |
| `20261011T181425Z-k3-f2c-1-real-17d5a8f6` | f1 F2c | orch-z2 | 209 ms | 318 ms | — → no change | 75/56/52 |
| `20261011T181425Z-k3-f2c-1-real-17d5a8f6` | f2 F2c | orch-z3 | 242 ms | 325 ms | — → no change | 53/58/78 |
| | decided rules observed in the run: 1.0 | | | | | |
| `20261011T181646Z-k3-f2d-1-real-aef4ff11` | f0 F2d | gw-z1 | 206 ms | 321 ms | — → no change | 5452/55/53 |
| `20261011T181646Z-k3-f2d-1-real-aef4ff11` | f1 F2d | gw-z2 | 216 ms | 319 ms | — → no change | 52/5453/64 |
| `20261011T181646Z-k3-f2d-1-real-aef4ff11` | f2 F2d | gw-z3 | 239 ms | 334 ms | — → no change | 52/59/5497 |
| | decided rules observed in the run: 1.0 | | | | | |
| `20261011T181904Z-k3-f2e-1-real-d9ee78e2` | f0 F2e | etcd | 213 ms | 1070 ms | — → no change | 59/57/54 |
| `20261011T181904Z-k3-f2e-1-real-d9ee78e2` | f1 F2e | etcd | 212 ms | 667 ms | — → no change | 58/56/54 |
| `20261011T181904Z-k3-f2e-1-real-d9ee78e2` | f2 F2e | etcd | 249 ms | 1054 ms | — → no change | 53/57/70 |
| | decided rules observed in the run: 1.0 | | | | | |
| `20261011T182122Z-k3-f3a-1-real-ea0b08a2` | f0 F3a | n2 | 3219 ms | 183 ms | n2@1.0 → no change | 6219/6229/6211 |
| `20261011T182122Z-k3-f3a-1-real-ea0b08a2` | f1 F3a | n3 | 3228 ms | 174 ms | n2@1.0 → no change | 52/56/79 |
| `20261011T182122Z-k3-f3a-1-real-ea0b08a2` | f2 F3a | n2 | 3230 ms | 176 ms | n2@1.0 → no change | 6189/6181/6219 |
| | decided rules observed in the run: 1.0 | | | | | |
| `20261011T182338Z-k3-f3b-1-real-a17ba17c` | f0 F3b | pooler-n2 | 30 ms | 46 ms | n2@1.0 → no change | 5076/5084/5069 |
| `20261011T182338Z-k3-f3b-1-real-a17ba17c` | f1 F3b | orch-z1 | 34 ms | 36 ms | — → no change | 53/58/53 |
| `20261011T182338Z-k3-f3b-1-real-a17ba17c` | f2 F3b | gw-z1 | 39 ms | 37 ms | — → no change | 5048/55/51 |
| | decided rules observed in the run: 1.0 | | | | | |
| `20261011T182556Z-k3-f4a-1-real-c43147f1` | f0 F4a | n1 | 2476 ms | 95 ms | n1@1.0 → no change | 816/815/815 |
| `20261011T182556Z-k3-f4a-1-real-c43147f1` | f1 F4a | n2 | 2670 ms | 102 ms | n1@1.0 → no change | 55/56/54 |
| `20261011T182556Z-k3-f4a-1-real-c43147f1` | f2 F4a | n1 | 2652 ms | 96 ms | n1@1.0 → no change | 856/810/405 |
| | decided rules observed in the run: 1.0 | | | | | |
| `20261011T182813Z-k3-f5-1-real-021a2161` | f0 F5a | n4 | 12283 ms | — (no heal) | — → n1@1.1 (11.8 s) | 53/59/53 |
| `20261011T182813Z-k3-f5-1-real-021a2161` | f1 F5b | n4 | 1111 ms | — (no heal) | — → n1@1.2 (1.6 s) | 52/58/53 |
| | decided rules observed in the run: 0.1, 1.0, 1.1, 1.2 | | | | | |
| `20261011T183032Z-k3-f6-1-real-2a13a090` | f0 F6 | pooler-n1 | 1916 ms | 154 ms | n1@1.0 → n3@2.0 (1.8 s) | 1517/1522/1515 |
| `20261011T183032Z-k3-f6-1-real-2a13a090` | f1 F6 | pooler-n3 | 1975 ms | 151 ms | n3@2.2 → n2@3.1 (2.4 s) | 1772/1769/1774 |
| | decided rules observed in the run: 1.0, 2.0, 2.1, 2.2, 3.1, 3.2 | | | | | |
| `20261011T183249Z-k3-f7-1-real-fc484e0f` | f0 F7 | n2 | 598 ms | 127 ms | n2@1.0 → no change | 149/156/147 |
| `20261011T183249Z-k3-f7-1-real-fc484e0f` | f1 F7 | n1 | 595 ms | 139 ms | n2@1.0 → no change | 71/59/54 |
| `20261011T183249Z-k3-f7-1-real-fc484e0f` | f2 F7 | n2 | 585 ms | 129 ms | n2@1.0 → no change | 149/149/202 |
| | decided rules observed in the run: 1.0 | | | | | |

## 6. Fail-closed (K3.2, K3.3)

Design (§8, §15 K3.2–K3.3, ADR-0005): a fault whose confirmation (heal)
predicate does not hold within its deadline is `not-applied` (`not-healed`);
the workload is aborted, the run torn down and INVALID. Method: for every
kind, `scripts/sabotage-nemesis.sh` built a host binary with
`cp_sabotage_inject_<kind>` (and `cp_sabotage_heal_f1a`, `_f3a`) and the
runner image `commitproof/runner:sab-<rule>` with the same tag and commit
(labels checked by `commitproof run`, which refuses a mismatch); the
injector (heal) of that kind is skipped and says so in the `inject` (`heal`)
event; predicates are untouched. Product binaries carry no sabotage symbol
(`TestProductBinariesHaveNoSabotage`, part of `ci`). F5b's sabotaged run
first applies a real F5a. System did (records `k3-sab-*` and
`k3-sabcheck-*`):

| Record | Run (`evidence/K3/runs/…`) | Fault end (rounds, time after the inject commands) | Last probe showed | run / cpcheck |
|---|---|---|---|---|
| `k3-sab-inject-f1a` | `20261011T183725Z-k3-f1a-1-real-54451565` | f0 F1a not-applied after 21 rounds, 15088 ms | connects to and from the target still succeed (40 problems) | 2 / INVALID 2 |
| `k3-sab-inject-f1b` | `20261011T183828Z-k3-f1b-1-real-9ac4fcb0` | f0 F1b not-applied after 21 rounds, 15205 ms | A–B connects succeed (72 problems) | 2 / INVALID 2 |
| `k3-sab-inject-f1c` | `20261011T183925Z-k3-f1c-1-real-d27a6b82` | f0 F1c not-applied after 23 rounds, 15131 ms | leader↔orch/nodes/etcd connect (24 problems) | 2 / INVALID 2 |
| `k3-sab-inject-f1d` | `20261011T184022Z-k3-f1d-1-real-4a8c61a9` | f0 F1d not-applied after 27 rounds, 15538 ms | leader→followers connect (8 problems) | 2 / INVALID 2 |
| `k3-sab-inject-f2a` | `20261011T184121Z-k3-f2a-1-real-f3b8f225` | f0 F2a not-applied after 29 rounds, 15393 ms | postmaster 114 still in state S | 2 / INVALID 2 |
| `k3-sab-inject-f2b` | `20261011T184218Z-k3-f2b-1-real-fd00f96f` | f0 F2b not-applied after 31 rounds, 15461 ms | pooler-n2 running | 2 / INVALID 2 |
| `k3-sab-inject-f2c` | `20261011T184324Z-k3-f2c-1-real-17d5a8f6` | f0 F2c not-applied after 31 rounds, 15467 ms | orch-z1 running | 2 / INVALID 2 |
| `k3-sab-inject-f2d` | `20261011T184422Z-k3-f2d-1-real-aef4ff11` | f0 F2d not-applied after 31 rounds, 15467 ms | gw-z1 running | 2 / INVALID 2 |
| `k3-sab-inject-f2e` | `20261011T184520Z-k3-f2e-1-real-d9ee78e2` | f0 F2e not-applied after 31 rounds, 15469 ms | etcd running | 2 / INVALID 2 |
| `k3-sab-inject-f3a` | `20261011T184626Z-k3-f3a-1-real-ea0b08a2` | f0 F3a not-applied after 26 rounds, 15576 ms | every postgres process running (none in T) | 2 / INVALID 2 |
| `k3-sab-inject-f3b` | `20261011T184724Z-k3-f3b-1-real-a17ba17c` | f0 F3b not-applied after 31 rounds, 15468 ms | pooler running, not paused | 2 / INVALID 2 |
| `k3-sab-inject-f4a` | `20261011T184822Z-k3-f4a-1-real-c43147f1` | f0 F4a not-applied after 28 rounds, 15023 ms | median 1 133 µs (threshold 150 000) | 2 / INVALID 2 |
| `k3-sab-inject-f5a` | `20261011T184920Z-k3-f5-1-real-021a2161` | f0 F5a not-applied after 144 rounds, 120121 ms | standby names without zone1_n4 | 2 / INVALID 2 |
| `k3-sab-inject-f5b` | `20261011T185203Z-k3-f5-1-real-021a2161` | f1 F5b not-applied after 135 rounds, 120190 ms | standby names still name zone1_n4 (F5a confirmed first) | 2 / INVALID 2 |
| `k3-sab-inject-f6` | `20261011T185523Z-k3-f6-1-real-2a13a090` | f0 F6 not-applied after 94 rounds, 60392 ms | no newer rule than n2@1.0 | 2 / INVALID 2 |
| `k3-sab-inject-f7` | `20261011T185710Z-k3-f7-1-real-fc484e0f` | f0 F7 not-applied after 29 rounds, 15327 ms | RTT 48 µs vs baseline 49 µs | 2 / INVALID 2 |
| `k3-sab-heal-f1a` | `20261011T185812Z-k3-f1a-1-real-54451565` | f0 F1a not-healed after 18 rounds, 30018 ms | target still cut from every peer | 2 / INVALID 2 |
| `k3-sab-heal-f3a` | `20261011T185951Z-k3-f3a-1-real-ea0b08a2` | f0 F3a not-healed after 6 rounds, 33053 ms | postgres processes still in T, select 1 fails | 2 / INVALID 2 |

Each of these runs is sealed; its manifest and run-start marker carry the
sabotage tag; its history has the sabotaged fault's `inject` followed by
`not-applied` (heal: `inject`, `confirmed`, `heal`, `not-healed`); run-end is
`invalid`; cpcheck lists the reason (`fault f0 not-applied` / `not-healed`)
and, since the workload was aborted, the missing final reads. cpgate K3.2
and K3.3 recompute all of it from the sealed runs.

## 7. Checkers on real runs (K3.5)

`cpcheck check` on every K3.1 run (records `k3-check-*`): **PASS** on all 16
runs, all nine checkers `valid: true`, verdict and seal root matching the
run (also on `slowfs-smoke-check`). There is no FAIL verdict on the real
build to triage. What PASS means here: no checker found an anomaly in these
16 histories (each ≤ 90 s of W1–W4 with three or fewer faults); it is not a
campaign result (K4) and carries no statistical bound.

## 8. CI-equivalent (K3.6)

`make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke` (record `ci`,
exit 0, 19:01:34–19:14:19 UTC, stdout sha256 `74236090…dfb01c`): gofmt, vet,
staticcheck, all unit tests (independence and no-sabotage-symbol tests
included); the K2 corpus regenerated byte for byte and classified as
planted, the Elle oracle and the checker sabotage controls; images, cluster
up/verify/down and the smoke run; `ci-nemesis-smoke` printed `nsmoke-f1a`,
`f2a`, `f3a`, `f7` and `f4a` (`/dev/fuse` present) `verdict PASS` and
`ci-nemesis-smoke: OK (f1a f2a f3a f7 f4a)`. `make lint-workflow` (record,
exit 0) passes on the workflow with the new `ci-nemesis-smoke` job. Not run
on GitHub (nothing pushed, LIM-16).

## 9. Harness defects found and fixed in K3 (none is a Multigres finding, §11.1)

1. **FUSE caches** (development): with go-fuse's 1 s attribute cache the
   baseline cluster never became HEALTHY — PostgreSQL restarted on a stale
   `postgresql.auto.conf` written by multipooler through the plain volume,
   pgBackRest `check` failed (`archive_mode must be enabled`) and bootstrap
   looped. Fixed by disabling entry/attribute/negative caching (ADR-0022).
2. **Workload client nil dereference** (development, first F2a run): a
   statement failing with a SQLSTATE followed by a rollback that lost the
   connection made `Session.Do` dereference a nil connection; the runner
   panicked. Fixed in `a3bbce1` with a regression test against the fake
   server; §6.2 classification unchanged.
3. **go-fuse direct mount leaves `/dev/fuse` inheritable** (development):
   PostgreSQL processes held the FUSE device, so when Docker killed the
   launcher at teardown the connection stayed alive and exiting backends
   waited forever for FLUSH replies (two teardowns: a heal-F3a sabotage run
   and an F2a smoke run; 32 processes in D state, `docker stop` failed).
   Fixed in `b686376` (close-on-exec, kill-and-reap before unmount;
   ADR-0022); the same heal-F3a scenario then tore down cleanly.
   **Recovery of the host:** the two dead connections (`/sys/fs/fuse/
   connections/75` and `/85`, root-owned, created at 17:18:26Z and
   17:08:18Z by the pg containers of those runs; connection 71, the
   desktop's document portal, untouched) were aborted by writing `1` to
   their `abort` files from a project-labelled container with every
   capability dropped and the fusectl directory bind-mounted
   (`waiting` was 4 and 28, the number of stuck processes); the stuck
   processes exited and `cluster down` left nothing. No `sudo` was used; it
   did rely on Docker's root-equivalent access (LIM-11).
4. **Leader resolution too strict** (evidence attempt 1): after F2a on the
   leader n1, n1's pooler kept reporting itself leader of its old rule 1.0
   (n2 led rule 2.0) for at least 27 s, so "exactly one self-reported
   leader" never held and the second F2a ended `not-applied` (run INVALID,
   exit 2). Fixed in `090a366`: the leader is the self-reported leader of
   the highest rule number (S06), a tie stays unresolved. The INVALID run
   is kept, sealed, as `attempt-1/20261011T175147Z-k3-f2a-1-real-f3b8f225.tar.zst`
   with its record; the rest of attempt 1 (smoke, f1a–f1d runs) was
   discarded and everything re-run from scratch as attempt 2.

## 10. Observations for K4 (diagnosis only, not verdicts)

Design says (S21): multiorch fails over after `leader-failover-grace-period-base` 4 s plus up to 8 s of jitter, and treats a leader whose PostgreSQL does not respond for `leader-postgres-response-threshold` 30 s as failed; S42/S48: promotion "within seconds". What the K3 runs showed (one run each, no statistics):

- **Partitions of 10 s around the leader produced no failover.** F1a on the leader (`…k3-f1a…`, f0, f2), F1b leader+orch-z1 | rest (all three), F1c (leader cut from orchestrators, followers and etcd but not gateways; all three) and F1d (leader → followers dropped; all three): no new decided rule observed before the next fault; client gaps of 9–18 s on every gateway (writes cannot reach a synchronous standby, and in F1a/F1b the gateways cannot reach the leader). Whether 10 s is inside multiorch's grace window or failover is suppressed in these shapes is a K4 question (C2 partition campaign; S28's asymmetric case is F1c).
- **F2a on the leader**: PostgreSQL restarted by itself in 1.8–2.9 s every time; the cluster moved to a new decided rule in 2.7–6.7 s (once re-electing the same node, n2@2.0, once electing n3@3.0), with client gaps of 1.9–6.7 s. In evidence attempt 1 (`attempt-1/…k3-f2a…`), the killed leader n1 was `pg_rewind`-ed ("servers diverged at WAL location 0/40BC0C0 on timeline 2"), restarted as standby, then its pooler logged `admin pool is closed` on every status call and kept reporting itself leader of rule 1.0 for at least 27 s while n2 led rule 2.0 — a stale self-reported leadership the K4 C6/C1 campaigns should watch. That run is INVALID (harness defect 4, §9) and no checker judged it; whether a gateway routed any operation to n1 then is not known.
- **F2b on the leader's pooler** (killed 5 s): no failover; client gaps 10–16 s. **F2c, F2e** (orchestrator, etcd): no client impact (gaps ≤ 78 ms). **F2d** (a gateway): its own clients saw ~5.5 s gaps, the others none.
- **F3a** (3 s SIGSTOP of the leader's PostgreSQL): ~6.2 s client gaps (3 s confirmation + 3 s hold), no failover, as expected below S21's 30 s threshold. **F3b** of the leader's pooler (5 s): ~5 s gaps; of a gateway: its clients only.
- **F4a** (200 ms per write/fsync on the leader): client gaps of 0.4–0.9 s, no failover; on a follower, none.
- **F5a**: n4 admitted (named in `synchronous_standby_names`, streaming, `quorum`) 12.0 s after its three `docker start` commands returned; rule 1.1 (n1 leader, cohort with n4). **F5b**: the graceful `docker stop -t 30 pooler-n4` took 0.63 s, and 0.2 s after the stop commands returned the leader's standby names no longer named n4; rule 1.2. No client impact in either.
- **F6** (SIGTERM to the leader's pooler): new leader 1.8 s and 2.4 s after the signal; client gaps ~1.5–1.8 s (no buffering: profile baseline). Each switchover produced two or three new decided rules in the observations (2.0, 2.1, 2.2 after the first; 3.1, 3.2 after the second, whose confirmation probe saw 3.0) before the next fault — the rule-changes-per-fault observation of §3 ("cascading re-election check").
- **F7** (50 ms on the leader's egress): client gaps ~150 ms, no failover.
- **Composite** F5a → F3a(n4, the random follower) → F2a(leader n1): with n4 paused, the failover after the leader's postmaster kill took 12.9 s (n3@3.0) even though n1's PostgreSQL was back within 0.2 s; client gaps ~12.6 s.

## 11. Demonstrated / not demonstrated

Demonstrated (artifacts above):
- every fault of §8 — F1a, F1b, F1c, F1d, F2a–F2e, F3a, F3b, F4a, F5a,
  F5b, F6, F7 — and one composite were injected on the real baseline
  cluster, confirmed by predicates over raw probe outputs within their
  deadlines, healed and heal-confirmed (16 runs, gate K3.1, K3.4);
- every injector and the F1a/F3a heals, sabotaged to no-ops, end the run
  INVALID with `not-applied` / `not-healed` and exit 2 (K3.2, K3.3);
- slowfs at 0 ms keeps the baseline HEALTHY and the smoke checkers PASS,
  so F4a (not the F4b fallback) is the slow-disk fault; its overhead per
  4 KiB write+fsync is about 70–83 µs;
- cpcheck PASS on all 16 faulted runs of the real build (one run each,
  ≤ 90 s, no statistics);
- `make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke` locally.

Not demonstrated in K3:
- any campaign result or statistical statement about Multigres (K4/K7);
  the reactions in §5 and §10 are single observations from observation
  data, not verdicts;
- repeatability of the confirmation and heal latencies (one run per kind);
- F1–F7 on the `buffered`, `slotless`, `replica-port` or `h1` profiles;
- n4 re-admission after F5b and membership with an empty n4 volume (LIM-29);
- composites other than F5a → F3a → F2a;
- the CI workflow on GitHub-hosted runners (LIM-16), including whether
  their runner exposes `/dev/fuse`.
