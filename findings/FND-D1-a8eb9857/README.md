**Acknowledged writes lost (G1) in this finding's original run: no.**

# multipooler/multigateway: after a cancelled synchronous-replication wait, other sessions read a commit that the next failover removes

| Dimension | From contract 1.5.0 (ADR-0042; this package) | Recorded in K4 (contract 1.0.7) |
|---|---|---|
| Finding | FND-D1-a8eb9857 | FND-D1-a8eb9857 |
| Guarantee | **G2b** — a value an `ok` read returned is in the history that survives (§3) | G2 (contract ≤ 1.4.1: G2a's and G2b's observables under one row) |
| Class | SAFETY_VIOLATION | SAFETY_VIOLATION |
| Claim level | **INFERRED** (I2; no public source states G2b) | EXPLICIT |
| Severity (§11.2, derived) | **high** | critical |
| Origin | **POSTGRES_INHERITED** (S61; evidence/pre-K7/lim33.md 5 of 5); which cancel acted in this run: UNDETERMINED | UNDETERMINED |
| Reproducibility | REPRODUCED (5 of 5 recorded replays) | REPRODUCED |
| Known status | NEW (`evidence/pre-K7/upstream-check.md`) | NEW |

Reclassified by the owner's decisions of 2026-10-15 (ADR-0042, `evidence/K6b/reclassification.md`); the K4 records are not edited.

## Summary

This is PostgreSQL behaviour that Multigres' cancel-on-timeout path exposes to other readers. While F1c cut the leader n1 off from its followers, orchestrators and etcd but not from the gateways, a client's COMMIT waited for a synchronous standby that could not answer. When the client gave up after its 10 s deadline, its write became visible on the old leader: a backend cancel ends PostgreSQL's synchronous-replication wait and completes the commit locally (S61). Another client then read the value with an `ok` read. Multiorch failed over to a follower that had never received the write, the old leader was rewound, and the value disappeared. Two `ok` reads of one key are not prefixes of each other (C3 `incompatible-order`). No write the cluster acknowledged was lost (C1 valid) and no failed write became visible (C2 valid): the value that vanished was never acknowledged to its writer. That is why the finding is G2b (INFERRED, from PostgreSQL compatibility), not G1/G2a (EXPLICIT).

## Version and environment

- Multigres `e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73` built from source with upstream's Dockerfiles (only `FROM` lines changed, §5.2), PostgreSQL 17.7 (`postgres:17.7@sha256:2006493727bd5277eece187319af1ef82b4cf82cf4fc1ed00da0775b646ac2a4`), durability policy `AT_LEAST_2`, cohort {n1, n2, n3}, three cells with one multiorch and one multigateway each (CONTRACT §5.1).
- Profile `baseline` (gateway buffering off, `--enable-slot-based-replication=true` as in upstream's Kubernetes demo); no metrics exporter in the original run (contract 1.0.6; LIM-41).
- Host: Arch Linux, kernel 7.2.9-arch1-1, Docker 29.9.0, one bridge network 10.77.0.0/24, all containers on one host (LIM-02, LIM-03).
- Original run: CommitProof `f4a5a0f` (clean), contract 1.0.6, started 2026-10-11T22:17:28Z; minimized run and its five recorded replays: same build (K4).
- Client: pgx v5.11.0; every operation has the P01 deadline of 10 s, after which the client gives up (and pgx sends a CancelRequest) and records the completion `info` (§6.2 row 9).

## Reproduction

From a clone of this repository at tag `v1.0.0` (Linux host meeting `docs/PREREQUISITES.md`; nothing is pulled at run time except the pinned base images):

```sh
make prereq-check                       # 26 checks; exit 0 required
make build                              # bin/commitproof, bin/cpcheck, …
bin/commitproof images build            # pinned images; writes images.lock.json
bin/commitproof cluster up --profile baseline && bin/commitproof cluster verify && bin/commitproof cluster down   # optional: HEALTHY check
mkdir -p runs/k7-pkg/FND-D1-a8eb9857
tar --zstd -C runs/k7-pkg/FND-D1-a8eb9857 -xf evidence/K4/runs/20261012T021947Z-D1-4-real-eb537f55.tar.zst
bin/commitproof replay runs/k7-pkg/FND-D1-a8eb9857/20261012T021947Z-D1-4-real-eb537f55 --times 5 --out runs/k7-pkg/FND-D1-a8eb9857 --cpcheck bin/cpcheck
```

Each replay brings the cluster up, runs the minimized plan (`plan.json` here, sha256 of its canonical bytes `eb537f55cca0bde22afe45afee17a8c3e06e75de06b9ed3c84d717da4866acb1`), tears the cluster down, seals the run and checks it; the last line is a JSON summary with the replays, how many show `incompatible-order` and the §11.1 class. The original run: `tar --zstd -xf evidence/K4/runs/20261011T221728Z-D1-4-real-ad3fca18.tar.zst` and `bin/cpcheck check <run-dir>` reproduce `checks.json` here byte for byte.

## Expected vs actual

- **Expected** (G2b, §3, INFERRED from I2, PostgreSQL compatibility): a value an `ok` read returned is in the history that survives; no list-append `incompatible-order` from a value that later vanished.
- **Actual**: in D1 seed 4 on the real build (`20261011T221728Z-D1-4-real-ad3fca18`), five `ok` reads observed values that later vanished: key 14 read with 55 (ops 2438, 2442, 2449 at 21.49–21.59 s) and with 55 and 58 (op 2456, 30.59 s), while its longest read (op 2506, 40.76 s) lacks both; key 18 read as `[1]` (op 2468, 31.64 s) while its longest read (op 3517, 45.18 s) lacks 1. Values 55, 58 and 1 were appended by ops 2400, 2430 and 2452, each recorded `info` when its client gave up after P01 (20.59 s, 30.59 s, 31.64 s). C3 `incompatible-order` (5 anomalies).
- Not violated in this run: G1 (C1 `valid: true` — every `ok` append and ledger insert is in the stable final reads through every gateway) and G4 (C2 `valid: true`).

## Analysis

**Design says.** Multigres acknowledges a write only after a quorum (S48, S49); HA runs `synchronous_commit=on` (S09). The gateway enforces timeouts by cancelling the backend query (S16) and forwards client cancels (S17). The pooler cancels a backend's statement when the request context ends (`go/services/multipooler/internal/pools/regular/regular_conn.go`, `execWithContextCancel` → `handleContextCancellation`, at e3cbdbb). PostgreSQL 17.7 ends a cancelled synchronous-replication wait with `WARNING: canceling wait for synchronous replication` and "the transaction has already committed locally, but might not have been replicated to the standby" (S61). On primary-port reads the repository's own test expects "leader reads" (S28).

**System did** (history and observations of the original run; `report-excerpt.md`):

| t (s) | Event |
|---|---|
| ~10.6 | F1c injected on leader n1 (gateways still reach it); see `report-excerpt.md` for the exact phases |
| 10.584 / 20.585 | op 2400 (client 2, gw-z3) appends 55 to key 14; its client gives up (P01): `info` |
| 20.588 / 30.588 | op 2430 (client 0, gw-z1) appends 58 to key 14; `info` at P01 |
| 21.487–21.590 | ops 2438, 2442, 2449 read key 14 ending in 55: `ok` |
| 21.637 / 31.638 | op 2452 (client 1, gw-z2) appends 1 to key 18; `info` at P01 |
| 30.591 | op 2456 reads key 14 with 55 and 58: `ok` |
| 31.638–31.644 | op 2468 reads key 18 = `[1]`: `ok` |
| — | a follower becomes leader of rule 2.0 (`report-excerpt.md`); the old leader is rewound |
| 40.745–40.761 | op 2506 reads key 14 without 55 and 58: `ok` |
| 45.173–45.180 | op 3517 reads key 18 without 1: `ok` |

**Plain PostgreSQL** (`evidence/pre-K7/lim33.md`, records `lim33-run-1`…`5`): with one synchronous standby cut off, a commit waiting in SyncRep that is cancelled — by `pg_cancel_backend` or by the client's own CancelRequest — becomes visible to other sessions in 5 of 5 runs, and a failover to the standby with `pg_rewind` removes it; a client that only disconnects leaves the backend waiting and nothing is read. Which cancel reached the backend in this Multigres run — the pgx CancelRequest sent when P01 expired or the pooler's cancel on context end — is not determined: PostgreSQL's server log was not captured (LIM-33).

**Oracle** (K4, §10.11): **oracle-disagreement** (triaged) — elle-cli did not complete on this history: its validity pass and its process pass exited 255 with `java.lang.OutOfMemoryError`, the validity pass also with `-Xmx9g` (records `elle-validity-…`, `elle-validity-xmx9g-…`, `elle-process-20261011T221728Z-D1-4-real-ad3fca18`); an unanswered oracle is never a confirmation (LIM-31). Triage `evidence/K4/triage/20261011T221728Z-D1-4-real-ad3fca18.json`.

**Minimization** (§12.2, K4): the empty schedule PASSes; the workload halved once (90 → 45 s) and the clients reduced to 0–3 keep the class. Minimized run `20261012T021947Z-D1-4-real-eb537f55` (45 s, clients 0–3). The five recorded replays of the minimized plan show `incompatible-order` in 5 (§11.1: REPRODUCED).

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

The package's replay command was executed once more in K7 (record `pkg-replay-FND-D1-a8eb9857` in `evidence/K7/commands.jsonl`; CommitProof `e8eca97`, contract 1.9.0 (ADR-0047), with ADR-0038's metrics exporter on every Multigres component, LIM-41). The plan replayed is this package's minimized plan, unchanged. Its five replays are stored as summaries in `evidence/K7/package-replays/`:

| Replay | Verdict | Classes | Seal root |
|---|---|---|---|
| `20261017T201121Z-D1-4-real-eb537f55` | PASS | — | `c4b12fd812a1cdf2…` |
| `20261017T201305Z-D1-4-real-eb537f55` | FAIL | `incompatible-order` | `e0c8f12e41e562e2…` |
| `20261017T201449Z-D1-4-real-eb537f55` | FAIL | `incompatible-order` | `ec56e661cc644595…` |
| `20261017T201620Z-D1-4-real-eb537f55` | FAIL | `incompatible-order` | `f858dbdbd3c135c7…` |
| `20261017T201755Z-D1-4-real-eb537f55` | FAIL | `G-nonadjacent`, `G-single`, `G0`, `G1c`, `G2-item` | `548389ac81d64870…` |

**3 of 5** show `incompatible-order` (§11.1 on these replays: REPRODUCED). The recorded classification stays REPRODUCED (5 of 5 recorded replays): this re-execution is reported beside it, not instead of it.
<!-- /K7-REPLAY -->

## Suggested direction (optional; not verified)

Not tested: whether the pooler's cancel on context end could leave a backend that is waiting for synchronous replication alone (a client's CancelRequest would still reach it, S17); whether the isolated leader could stop serving reads sooner once it cannot reach a quorum. Either is a hypothesis about where Multigres could narrow the window; the PostgreSQL behaviour itself (S61) is unchanged by both.

## Limitations

- One pinned commit, one host, containers on one bridge network (LIM-01, LIM-02); no metrics exporter in this run, unlike K7's matrix (LIM-41).
- Which cancel acted is not determined (LIM-33); the origin is the owner's classification, supported by the plain-PostgreSQL experiment and S61.
- The reads that returned the vanished value were served by the old leader before the new rule was observed (pooler status every 1 s, P24); whether it was already superseded when it served them is not shown.
- elle-cli could not check this history (out of memory, LIM-31); the anomaly rests on C3 alone, validated against elle-cli on the K2 corpus (K2.4).
- `checks.json` (the original run's, byte for byte, as §11.4 requires) was written before the C3 correction of `32b660a`: its two realtime cycle observations (`G-single-realtime`, `G2-item-realtime`; no verdict reads them) each name one transaction too few. Re-checked with the corrected C3, the verdict, the `incompatible-order` anomalies and every class are unchanged; only those two observations' operations and explanations differ (`evidence/K7/c3-correction-audit.md`, record `c3-audit-2`; LIM-44).
- The recorded classification and reproducibility come from K4's runs (contract 1.0.6); K7 re-executed the replay command once with a later build that runs the metrics exporter (section above).
