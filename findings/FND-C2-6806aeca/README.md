**Acknowledged writes lost (G1) in this finding's original run: no.**

# multipooler/multigateway: primary-port register reads return a value that the next failover removes

| Dimension | Value (contract 1.9.0; ADR-0043) | Derivation |
|---|---|---|
| Finding | FND-C2-6806aeca | §11.3: sha256(`SAFETY_VIOLATION\|G6\|non-linearizable\|<minimized plan>`)[:8]; recorded in K6b (`evidence/K6b/findings/FND-C2-6806aeca.json`); K4 had described it inside FND-C2-2a534585's analysis |
| Guarantee | **G6** — single-key autocommit reads and writes through the primary port are linearizable (§3) | C4 `non-linearizable` on registers 1 and 4 |
| Class | SAFETY_VIOLATION | §11.1 |
| Claim level | **DESIGN** (S28: primary-port reads are "leader reads") | G6's row |
| Severity (§11.2, derived) | **high** | SAFETY_VIOLATION ∧ DESIGN |
| Origin | **POSTGRES_INHERITED** (the vanish mechanism, S61; evidence/pre-K7/lim33.md 5 of 5); which cancel acted: UNDETERMINED | the owner's ruling for the mechanism (ADR-0042, ADR-0043) |
| Reproducibility | REPRODUCED (3 of 5 recorded replays) | the minimized plan was minimized by K4 for `incompatible-order`, not for this class |
| Known status | NEW (`evidence/pre-K7/upstream-check.md`) | — |

Cross-reference: same run, same mechanism and same minimized plan as FND-C2-2a534585 (G2b); the mechanism of FND-D1-fb8d0f8c, FND-D1-2e860fd2 and FND-D1-a8eb9857 (`evidence/K6b/reclassification.md`).

## Summary

It is the G2b mechanism seen on registers. While F1c cut leader n1 off from its followers, orchestrators and etcd but not from the gateways, a register `cas` and a register `write` waited at commit for a synchronous standby. Their clients gave up after 10 s (completion `info`). Their values then became visible on the old leader: a cancelled synchronous-replication wait completes the commit locally (S61). `ok` reads through two gateways returned them. After the failover to n2, register 1 no longer held 240 and register 4 returned the older 227. No register order explains an `ok` read of a value followed by `ok` operations that see the state before it: C4 `non-linearizable` (G6). C6 found no `stale-read` (§10.6): no offending read returned a value superseded by a write that completed before it. No acknowledged write was lost (C1 valid); no failed write is visible (C2 valid).

## Version and environment

- Multigres `e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73` built from source with upstream's Dockerfiles (only `FROM` lines changed, §5.2), PostgreSQL 17.7 (`postgres:17.7@sha256:2006493727bd5277eece187319af1ef82b4cf82cf4fc1ed00da0775b646ac2a4`), durability policy `AT_LEAST_2`, cohort {n1, n2, n3}, three cells with one multiorch and one multigateway each (CONTRACT §5.1).
- Profile `baseline` (gateway buffering off, `--enable-slot-based-replication=true` as in upstream's Kubernetes demo); no metrics exporter in the original run (contract 1.0.6; LIM-41).
- Host: Arch Linux, kernel 7.2.9-arch1-1, Docker 29.9.0, one bridge network 10.77.0.0/24, all containers on one host (LIM-02, LIM-03).
- Original run: CommitProof `f4a5a0f` (clean), contract 1.0.6, started 2026-10-11T23:10:36Z; minimized run `20261012T025629Z-C2-3-real-3fb018bf` (CommitProof `f4a5a0f`) and its five recorded replays (K4).
- Client: pgx v5.11.0; P01 deadline 10 s per operation (§6.2).

## Reproduction

From a clone of this repository at tag `v1.0.0` (Linux host meeting `docs/PREREQUISITES.md`):

```sh
make prereq-check                       # 26 checks; exit 0 required
make build
bin/commitproof images build            # pinned images; writes images.lock.json
bin/commitproof cluster up --profile baseline && bin/commitproof cluster verify && bin/commitproof cluster down   # optional: HEALTHY check
mkdir -p runs/k7-pkg/FND-C2-6806aeca
tar --zstd -C runs/k7-pkg/FND-C2-6806aeca -xf evidence/K4/runs/20261012T025629Z-C2-3-real-3fb018bf.tar.zst
bin/commitproof replay runs/k7-pkg/FND-C2-6806aeca/20261012T025629Z-C2-3-real-3fb018bf --times 5 --out runs/k7-pkg/FND-C2-6806aeca --cpcheck bin/cpcheck
```

Each replay brings the cluster up, runs the minimized plan (`plan.json` here; sha256 of its canonical bytes `3fb018bf532f53a631e5e745ba447e260aaa4e0d67457b4577cbd98e5e66127a`), tears the cluster down, seals and checks the run; the last line is a JSON summary with the replays and how many show the class the driver targets. `tar --zstd -xf evidence/K4/runs/20261011T231036Z-C2-3-real-f4505b54.tar.zst` and `bin/cpcheck check <run-dir>` reproduce the original run's `checks.json` here byte for byte.

## Expected vs actual

- **Expected** (G6, DESIGN, S28): single-key autocommit reads and writes through the primary port are linearizable.
- **Actual**: in C2 seed 3 on the real build (`20261011T231036Z-C2-3-real-f4505b54`): register 1 — `cas` 237→240 (op 3258, gw-z2) `info` at 20.73 s; `ok` reads at 30.78 s (op 3307, gw-z3) and 30.83 s (op 3313, gw-z2) return 240; a `cas` expecting 240 at 40.88 s (op 3370) is not applied. Register 4 — `write` 242 (op 3289, gw-z3) `info` at 30.78 s; an `ok` read at 30.88 s (op 3316, gw-z2) returns 242; a read at 41.03 s (op 3412) returns 227, written `ok` at 10.22 s (op 3099). C4 `non-linearizable` (2 registers), C6 valid.

## Analysis

**Design says.** Primary-port reads are leader reads that must not be served by a superseded primary (S28). The gateway cancels the backend on statement timeout (S16), the pooler cancels it when a request context ends (`regular_conn.go`, `handleContextCancellation`), client cancels are forwarded (S17). PostgreSQL ends a cancelled synchronous-replication wait by committing locally (S61).

**System did** (history and observations; `report-excerpt.md`):

| t (s) | Event |
|---|---|
| 10.22 | op 3099 writes 227 to register 4: `ok` |
| 10.65 / 11.88 | F1c injected / confirmed on leader n1 (gateways still reach it) |
| 10.73 → 20.73 | op 3258 `cas` 237→240 on register 1 waits; its client gives up (P01): `info` |
| 20.78 → 30.78 | op 3289 writes 242 to register 4; `info` at P01 |
| 30.78–30.89 | `ok` reads (ops 3307, 3313, 3316) return 240 and 242 |
| 36.3 | rule 2.0 with leader n2 first observed |
| 40.88 | op 3370 `cas` expecting 240 on register 1: not applied (`ok`, applied false) |
| 41.03 | op 3412 reads register 4 = 227: `ok` |
| 42.18 | F1c healed |

The reads that returned 240 and 242 happened before rule 2.0 was observed (pooler status every 1 s), so whether n1 was already superseded when it served them is not shown; what is shown is that the values those reads returned were later absent, which no linearization of the register's history allows. The same run shows the G2b anomaly on list-append keys (FND-C2-2a534585). Plain PostgreSQL 17.7 shows the mechanism in 5 of 5 runs (`evidence/pre-K7/lim33.md`); which cancel reached these backends is not determined (LIM-33).

**Minimization and replays** (§12.2, K4): the minimized plan `3fb018bf532f53a631e5e745ba447e260aaa4e0d67457b4577cbd98e5e66127a` (one F1c step of 30 s, 45 s workload; run `20261012T025629Z-C2-3-real-3fb018bf`) was minimized for `incompatible-order`, ADR-0024's target for this run; its five recorded replays show `non-linearizable` in 3 (§11.1: REPRODUCED).

<!-- K7-RECURRENCES -->

<!-- K7-REPLAY -->

## Suggested direction (optional; not verified)

As for FND-C2-2a534585: whether the pooler's cancel on context end could leave a backend that waits for synchronous replication alone, and whether an isolated leader could stop serving primary-port reads sooner. Both are hypotheses.

## Limitations

- One pinned commit, one host (LIM-01, LIM-02); no metrics exporter in this run (LIM-41); which cancel acted is not determined (LIM-33).
- The minimized plan targeted `incompatible-order`, not this class; G6's parenthetical ("no stale read from a superseded primary") is its motivating case, the checked property is linearizability (C6 found no `stale-read`).
- Porcupine judges the observed history only (LIM-08); register writes whose completion is `info` are included with open-ended return (§10.4).
- The recorded classification comes from K4's runs (contract 1.0.6); K7 re-executed the replay command once with a later build that runs the metrics exporter (section above).
