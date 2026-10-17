**Acknowledged writes lost (G1) in this finding's original run: no.**

# multipooler/multigateway: after a cancelled synchronous-replication wait, other sessions read a commit that the next failover removes

| Dimension | From contract 1.5.0 (ADR-0042; this package) | Recorded in K4 (contract 1.0.7) |
|---|---|---|
| Finding | FND-D1-2e860fd2 | FND-D1-2e860fd2 |
| Guarantee | **G2b** — a value an `ok` read returned is in the history that survives (§3) | G2 (contract ≤ 1.4.1: G2a's and G2b's observables under one row) |
| Class | SAFETY_VIOLATION | SAFETY_VIOLATION |
| Claim level | **INFERRED** (I2; no public source states G2b) | EXPLICIT |
| Severity (§11.2, derived) | **high** | critical |
| Origin | **POSTGRES_INHERITED** (S61; evidence/pre-K7/lim33.md 5 of 5); which cancel acted in this run: UNDETERMINED | UNDETERMINED |
| Reproducibility | REPRODUCED (4 of 5 recorded replays) | REPRODUCED |
| Known status | NEW (`evidence/pre-K7/upstream-check.md`) | NEW |

Reclassified by the owner's decisions of 2026-10-15 (ADR-0042, `evidence/K6b/reclassification.md`); the K4 records are not edited.

## Summary

This is PostgreSQL behaviour that Multigres' cancel-on-timeout path exposes to other readers. While F1c cut the leader n1 off from its followers, orchestrators and etcd but not from the gateways, a client's COMMIT waited for a synchronous standby that could not answer. When the client gave up after its 10 s deadline, its write became visible on the old leader: a backend cancel ends PostgreSQL's synchronous-replication wait and completes the commit locally (S61). Another client then read the value with an `ok` read. Multiorch failed over to a follower that had never received the write, the old leader was rewound, and the value disappeared. Two `ok` reads of one key are not prefixes of each other (C3 `incompatible-order`). No write the cluster acknowledged was lost (C1 valid) and no failed write became visible (C2 valid): the value that vanished was never acknowledged to its writer. That is why the finding is G2b (INFERRED, from PostgreSQL compatibility), not G1/G2a (EXPLICIT).

## Version and environment

- Multigres `e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73` built from source with upstream's Dockerfiles (only `FROM` lines changed, §5.2), PostgreSQL 17.7 (`postgres:17.7@sha256:2006493727bd5277eece187319af1ef82b4cf82cf4fc1ed00da0775b646ac2a4`), durability policy `AT_LEAST_2`, cohort {n1, n2, n3}, three cells with one multiorch and one multigateway each (CONTRACT §5.1).
- Profile `baseline` (gateway buffering off, `--enable-slot-based-replication=true` as in upstream's Kubernetes demo); no metrics exporter in the original run (contract 1.0.6; LIM-41).
- Host: Arch Linux, kernel 7.2.9-arch1-1, Docker 29.9.0, one bridge network 10.77.0.0/24, all containers on one host (LIM-02, LIM-03).
- Original run: CommitProof `f4a5a0f` (clean), contract 1.0.6, started 2026-10-11T22:15:06Z; minimized run and its five recorded replays: same build (K4).
- Client: pgx v5.11.0; every operation has the P01 deadline of 10 s, after which the client gives up (and pgx sends a CancelRequest) and records the completion `info` (§6.2 row 9).

## Reproduction

From a clone of this repository at tag `v1.0.0` (Linux host meeting `docs/PREREQUISITES.md`; nothing is pulled at run time except the pinned base images):

```sh
make prereq-check                       # 26 checks; exit 0 required
make build                              # bin/commitproof, bin/cpcheck, …
bin/commitproof images build            # pinned images; writes images.lock.json
bin/commitproof cluster up --profile baseline && bin/commitproof cluster verify && bin/commitproof cluster down   # optional: HEALTHY check
mkdir -p runs/k7-pkg/FND-D1-2e860fd2
tar --zstd -C runs/k7-pkg/FND-D1-2e860fd2 -xf evidence/K4/runs/20261012T020127Z-D1-3-real-fb2e3778.tar.zst
bin/commitproof replay runs/k7-pkg/FND-D1-2e860fd2/20261012T020127Z-D1-3-real-fb2e3778 --times 5 --out runs/k7-pkg/FND-D1-2e860fd2 --cpcheck bin/cpcheck
```

Each replay brings the cluster up, runs the minimized plan (`plan.json` here, sha256 of its canonical bytes `fb2e37782d44c0539e00490762563debdea8e2b1730289d0f8c9690e96dc733f`), tears the cluster down, seals the run and checks it; the last line is a JSON summary with the replays, how many show `incompatible-order` and the §11.1 class. The original run: `tar --zstd -xf evidence/K4/runs/20261011T221506Z-D1-3-real-da159fa0.tar.zst` and `bin/cpcheck check <run-dir>` reproduce `checks.json` here byte for byte.

## Expected vs actual

- **Expected** (G2b, §3, INFERRED from I2, PostgreSQL compatibility): a value an `ok` read returned is in the history that survives; no list-append `incompatible-order` from a value that later vanished.
- **Actual**: in D1 seed 3 on the real build (`20261011T221506Z-D1-3-real-da159fa0`), key 13 was read with value 35 at its end by an `ok` transaction (op 2142, gw-z2, 20.75 s); the key's longest read (op 2523, gw-z3, 42.23 s) lacks 35. Value 35 was appended by op 2132 (gw-z2), invoked at 10.74 s and recorded `info` at 20.74 s when its client gave up (P01). C3 `incompatible-order` (1 anomaly).
- Not violated in this run: G1 (C1 `valid: true` — every `ok` append and ledger insert is in the stable final reads through every gateway) and G4 (C2 `valid: true`).

## Analysis

**Design says.** Multigres acknowledges a write only after a quorum (S48, S49); HA runs `synchronous_commit=on` (S09). The gateway enforces timeouts by cancelling the backend query (S16) and forwards client cancels (S17). The pooler cancels a backend's statement when the request context ends (`go/services/multipooler/internal/pools/regular/regular_conn.go`, `execWithContextCancel` → `handleContextCancellation`, at e3cbdbb). PostgreSQL 17.7 ends a cancelled synchronous-replication wait with `WARNING: canceling wait for synchronous replication` and "the transaction has already committed locally, but might not have been replicated to the standby" (S61). On primary-port reads the repository's own test expects "leader reads" (S28).

**System did** (history and observations of the original run; `report-excerpt.md`):

| t (s) | Event |
|---|---|
| ~10.7 | F1c injected on leader n1 (gateways still reach it); see `report-excerpt.md` for the exact phases |
| 10.744 | op 2132 (client 1, gw-z2) appends 35 to key 13; its COMMIT waits for a synchronous standby |
| 20.744 | op 2132's client gives up (P01): completion `info` (§6.2 row 9) |
| 20.744–20.754 | op 2142 (client 1, gw-z2) reads key 13 ending in 35: `ok` |
| — | a follower becomes leader of rule 2.0 (`report-excerpt.md`: rules and WAL positions); the old leader is rewound |
| 42.218–42.226 | op 2523 (client 2, gw-z3) reads key 13 without 35: `ok` |

**Plain PostgreSQL** (`evidence/pre-K7/lim33.md`, records `lim33-run-1`…`5`): with one synchronous standby cut off, a commit waiting in SyncRep that is cancelled — by `pg_cancel_backend` or by the client's own CancelRequest — becomes visible to other sessions in 5 of 5 runs, and a failover to the standby with `pg_rewind` removes it; a client that only disconnects leaves the backend waiting and nothing is read. Which cancel reached the backend in this Multigres run — the pgx CancelRequest sent when P01 expired or the pooler's cancel on context end — is not determined: PostgreSQL's server log was not captured (LIM-33).

**Oracle** (K4, §10.11): **oracle-disagreement** (triaged) — cpcheck's three elle-cli passes completed: elle-cli also judges the history invalid and reports `incompatible-order`, and additionally a `G-single` cycle (and its realtime variant) whose anti-dependency edge starts at the read that observed the vanished value. Adya's cycle classes presuppose one version order per key (S72); these reads admit none, and §10.3 reports non-prefix reads as `incompatible-order` and derives no cycle through them (LIM-31). Triage `evidence/K4/triage/20261011T221506Z-D1-3-real-da159fa0.json`; checker unchanged.

**Minimization** (§12.2, K4): the empty schedule PASSes; the workload halved once (90 → 45 s) keeps the class, with all clients. Minimized run `20261012T020127Z-D1-3-real-fb2e3778` (45 s). The five recorded replays of the minimized plan show `incompatible-order` in 4 (§11.1: REPRODUCED).

<!-- K7-RECURRENCES -->
## Recurrences in K7 (ADR-0045)

3 valid run(s) of K7's matrix repeat this finding's mechanism: every guarantee group of their anomalies matches ADR-0045's predicate R-G2b, which reads only `checks.json`. Each is counted in its cell's k/N and interval (§12.3), triaged (`evidence/K7/triage/`), committed as a summary (stub and `checks.json`, `evidence/K7/runs/`), and not minimized. Its full archive stays on the machine that ran it (`runs/k7/`). `cpgate K7` re-evaluates the predicate for every one.

| Run | Cell | Seed | Seal root | Predicate | Matched fields |
|---|---|---|---|---|---|
| `20261016T175837Z-C2-8-real-65087015` | V1 C2/baseline | 8 | `48d4079baeecf6ca…` | R-G2b | classes incompatible-order (incompatible-order 5); valid(C1) true; valid(C2) true; timeouts 35; F {F1b, F1c}; vanished: key 23: [59] read at i=5913, absent from i=6041; key 24: [35] read at i=5919, absent from i=6542; key 25: [23] read at i=5919, absent from i=7016; key 26: [27] read at i=5909, absent from i=6920; … (5 listed) |
| `20261016T180118Z-C2-9-real-4bafa50d` | V1 C2/baseline | 9 | `75c53226ee75f4ac…` | R-G2b | classes incompatible-order (incompatible-order 3); valid(C1) true; valid(C2) true; timeouts 35; F {F1c, F1d}; vanished: key 41: [56] read at i=10126, absent from i=10318; key 42: [44] read at i=10088, absent from i=10430; key 42: [44] read at i=10113, absent from i=10430 |
| `20261016T221713Z-C2-3-real-273a1b0c` | V2 C2/slotless | 3 | `4c6dfdf2109b9990…` | R-G2b | classes incompatible-order (incompatible-order 2); valid(C1) true; valid(C2) true; timeouts 45; F {F1c}; vanished: key 14: [40] read at i=3114, absent from i=3854; key 36: [6] read at i=7481, absent from i=8647 |
<!-- /K7-RECURRENCES -->

<!-- K7-REPLAY -->
## K7 replay (K7.4)

The package's replay command was executed once more in K7 (record `pkg-replay-FND-D1-2e860fd2` in `evidence/K7/commands.jsonl`; CommitProof `e8eca97`, contract 1.9.0 (ADR-0047), with ADR-0038's metrics exporter on every Multigres component, LIM-41). The plan replayed is this package's minimized plan, unchanged. Its five replays are stored as summaries in `evidence/K7/package-replays/`:

| Replay | Verdict | Classes | Seal root |
|---|---|---|---|
| `20261017T200331Z-D1-3-real-fb2e3778` | FAIL | `incompatible-order` | `9d8b12f66156ce93…` |
| `20261017T200504Z-D1-3-real-fb2e3778` | FAIL | `incompatible-order` | `6b9bf2e23da4bf36…` |
| `20261017T200635Z-D1-3-real-fb2e3778` | FAIL | `incompatible-order` | `3b8866e4a24d7e4d…` |
| `20261017T200806Z-D1-3-real-fb2e3778` | FAIL | `incompatible-order` | `0d8740b593468468…` |
| `20261017T200949Z-D1-3-real-fb2e3778` | PASS | — | `81d55428131164a4…` |

**4 of 5** show `incompatible-order` (§11.1 on these replays: REPRODUCED). The recorded classification stays REPRODUCED (4 of 5 recorded replays): this re-execution is reported beside it, not instead of it.
<!-- /K7-REPLAY -->

## Suggested direction (optional; not verified)

Not tested: whether the pooler's cancel on context end could leave a backend that is waiting for synchronous replication alone (a client's CancelRequest would still reach it, S17); whether the isolated leader could stop serving reads sooner once it cannot reach a quorum. Either is a hypothesis about where Multigres could narrow the window; the PostgreSQL behaviour itself (S61) is unchanged by both.

## Limitations

- One pinned commit, one host, containers on one bridge network (LIM-01, LIM-02); no metrics exporter in this run, unlike K7's matrix (LIM-41).
- Which cancel acted is not determined (LIM-33); the origin is the owner's classification, supported by the plain-PostgreSQL experiment and S61.
- The reads that returned the vanished value were served by the old leader before the new rule was observed (pooler status every 1 s, P24); whether it was already superseded when it served them is not shown.
- elle-cli does not confirm this run's classes exactly (it adds a G-single cycle; oracle-disagreement, triaged above).
- The recorded classification and reproducibility come from K4's runs (contract 1.0.6); K7 re-executed the replay command once with a later build that runs the metrics exporter (section above).
