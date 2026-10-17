**Acknowledged writes lost (G1) in this finding's original run: no.**

# multipooler/multigateway: under a one-way leader-to-follower drop, a register read returns a value that the next failover removes

| Dimension | Value (contract 1.9.0) | Derivation |
|---|---|---|
| Finding | FND-C2-8215101c | §11.3: sha256(`SAFETY_VIOLATION\|G6\|non-linearizable\|<minimized plan>`)[:8]; a K7 matrix failure that matches no ADR-0045 predicate |
| Guarantee | **G6** — single-key autocommit reads and writes through the primary port are linearizable (no stale read from a superseded primary) (§3) | C4 `non-linearizable` on register 4 |
| Class | SAFETY_VIOLATION | §11.1 |
| Claim level | **DESIGN** (S28: primary-port reads are "leader reads") | G6's row |
| Severity (§11.2, derived) | **high** | SAFETY_VIOLATION ∧ DESIGN |
| Origin | **POSTGRES_INHERITED** (the vanish mechanism, S61; plain PostgreSQL 17.7 shows it in 5 of 5 runs, evidence/pre-K7/lim33.md; which cancel acted: UNDETERMINED) | hypothesis (§11.1); see Analysis |
| Reproducibility | REPRODUCED (5 of 5 recorded replays show `non-linearizable`) | §11.1, P13 |
| Known status | NEW | — |

Cross-reference: the mechanism of FND-C2-6806aeca (G6) and of the G2b findings FND-D1-fb8d0f8c, FND-D1-2e860fd2, FND-D1-a8eb9857, FND-C2-2a534585. It is a new failure only because ADR-0045's R-G6 requires F1c among the run's confirmed faults (its condition 5); this run's faults are F1b and F1d, and the minimized plan keeps F1d alone.

## Summary

In K7's V2 C2 seed 1 (profile `slotless`) on the real build, F1d dropped the leader n2's traffic to its followers. A register `cas` 1127→1130 waited at commit for a synchronous standby; its client gave up after 10 s (`info`). An `ok` read through a gateway then returned 1130. After n1 was promoted, the final read of register 4 returned 1127: the value 1130 had vanished. No register order explains an `ok` read of 1130 followed by reads of the state before it: C4 `non-linearizable` (G6). C6 found no `stale-read`; no acknowledged write was lost (C1 valid); no failed write is visible (C2 valid). The minimized plan (F1d on the leader alone, a 22.5 s workload) shows the class in 5 of 5 replays.

## Version and environment

- Multigres `e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73` built from source with upstream's Dockerfiles (only `FROM` lines changed, §5.2), PostgreSQL 17.7 (`postgres:17.7@sha256:2006493727bd5277eece187319af1ef82b4cf82cf4fc1ed00da0775b646ac2a4`), durability policy `AT_LEAST_2`, cohort {n1, n2, n3}, three cells with one multiorch and one multigateway each (CONTRACT §5.1).
- Profile `slotless` (pooler and gateway `--enable-slot-based-replication=false`; K7 cell V2); ADR-0038's OpenTelemetry Prometheus exporter on every multigateway, multipooler and multiorch (K7's runs, LIM-41).
- Host: Arch Linux, kernel 7.2.9-arch1-1, Docker 29.9.0, one bridge network 10.77.0.0/24, all containers on one host (LIM-02, LIM-03).
- Original run `20261016T221153Z-C2-1-real-c8781715`: CommitProof `32b660a` (clean), contract 1.8.1 (K7's matrix, `campaigns/k7.json`, `make campaign-k7`). Minimized run `20261017T002205Z-C2-1-real-316f3877` (CommitProof `32b660a`, 7 candidates) and its five recorded replays.
- Client: pgx v5.11.0; P01 deadline 10 s per operation (§6.2).

## Reproduction

From a clone of this repository at tag `v1.0.0` (Linux host meeting `docs/PREREQUISITES.md`):

```sh
make prereq-check                       # 26 checks; exit 0 required
make build
bin/commitproof images build            # pinned images; writes images.lock.json
mkdir -p runs/k7-pkg/FND-C2-8215101c
tar --zstd -C runs/k7-pkg/FND-C2-8215101c -xf evidence/K7/runs/20261017T002205Z-C2-1-real-316f3877.tar.zst
bin/commitproof replay runs/k7-pkg/FND-C2-8215101c/20261017T002205Z-C2-1-real-316f3877 --times 5 --out runs/k7-pkg/FND-C2-8215101c --cpcheck bin/cpcheck
```

Each replay brings the cluster up, runs the minimized plan (`plan.json` here; sha256 of its canonical bytes `316f38777476090cac8a900822d1037967cda210f8e22c907527251554d51701`), tears the cluster down, seals and checks the run; the last line is a JSON summary with the replays and how many show the class the driver targets. `tar --zstd -xf` of the original run's archive and `bin/cpcheck check <run-dir>` reproduce the original run's `checks.json` (here) byte for byte.

## Expected vs actual

- **Expected** (G6, DESIGN, S28): single-key autocommit reads and writes through the primary port are linearizable.
- **Actual** (`20261016T221153Z-C2-1-real-c8781715`): register 4 — `write` 1127 `ok` at 77.598 s (gw-z2); `cas` 1127→1130 (gw-z3) `info` at 87.645 s (P01); `ok` read at 87.696 s (i=13733, gw-z3) returns 1130; the final read i=13777 returns 1127. C4 `non-linearizable` (earliest offending operation: the final read), C6 valid, C1 and C2 valid.

## Analysis

**Design says.** Primary-port reads are leader reads (S28). The gateway cancels the backend on statement timeout (S16) and forwards client cancels (S17); PostgreSQL ends a cancelled synchronous-replication wait by committing locally (S61).

**System did** (history and observations; `report-excerpt.md`):

| t (s) | Event |
|---|---|
| 10.459–42.226 | F1b: {leader n1, an orchestrator} vs the rest; n2 becomes primary (rule 2.0, first observed 35.26) |
| 52.456–64.218 | F1b: {a follower, an orchestrator} vs the rest; no leader change |
| 77.595 / 78.734 | F1d injected / confirmed on the leader n2 (drop n2 → followers only) |
| 77.598 | `write` 1127 on register 4: `ok` |
| 77.644 → 87.645 | `cas` 1127→1130 (gw-z3) waits at commit; its client gives up (P01): `info` |
| 87.696 | `ok` read (i=13733, gw-z3) returns 1130 |
| 97.28 | n1 promoting; rule 3.0 with leader n1 first observed at 98.26 (then 4.0 at 102.27) |
| 108.942 | F1d healed |
| 111.30 → 116.27 | n2's WAL position restarts at `0/50000A0`; n2 rejoins as a standby of n1 (rule 4.0) |
| final read | register 4 = 1127 |

With F1d the leader cannot send WAL to its synchronous standbys, so the `cas`'s commit waits; when its client's deadline expires the wait is cancelled and the commit becomes visible locally (S61); an `ok` read sees it; n1, which never received it, is promoted and n2 is rewound, removing it. That is FND-C2-6806aeca's sequence with the leader reachable from the gateways because F1d leaves every other path open, not because of F1c. Which cancel reached the backend is not determined (LIM-33). Every operation the anomaly names other than the `cas` is `ok` (§6.2 row 1); the `cas` is `info` (row 9): no completion is misclassified (`evidence/K7/triage/20261016T221153Z-C2-1-real-c8781715.json`).

**Minimization and replays** (§12.2, P13): 7 candidates; the minimized plan `316f38777476090cac8a900822d1037967cda210f8e22c907527251554d51701` keeps step 2 (F1d on the leader, 30 s) and halves the workload twice (22.5 s), 8 clients (run `20261017T002205Z-C2-1-real-316f3877`, FAIL `non-linearizable`). Its five recorded replays show `non-linearizable` in 5 of 5 (§11.1: REPRODUCED).

<!-- K7-RECURRENCES -->
## Recurrences in K7 (ADR-0045)

None. No valid run of K7's matrix (130 cells, `evidence/K7/results.json`) failed with a guarantee group that matches the predicate standing for this finding; nothing of K7 is cited here. Every K7 failure and how ADR-0045's predicates classified it is listed in `evidence/K7/results.json` and the K7 closure.
<!-- /K7-RECURRENCES -->

<!-- K7-REPLAY -->
## K7 replay (K7.4)

The package's replay command was executed once more in K7 (record `pkg-replay-FND-C2-8215101c` in `evidence/K7/commands.jsonl`; CommitProof `e8eca97`, contract 1.9.0 (ADR-0047), with ADR-0038's metrics exporter on every Multigres component, LIM-41). The plan replayed is this package's minimized plan, unchanged. Its five replays are stored as summaries in `evidence/K7/package-replays/`:

| Replay | Verdict | Classes | Seal root |
|---|---|---|---|
| `20261017T041149Z-C2-1-real-316f3877` | FAIL | `non-linearizable` | `234087ba34d38619…` |
| `20261017T041317Z-C2-1-real-316f3877` | FAIL | `non-linearizable` | `db21f8d72f0a971e…` |
| `20261017T041445Z-C2-1-real-316f3877` | FAIL | `non-linearizable` | `1da2f0051786dd4e…` |
| `20261017T041620Z-C2-1-real-316f3877` | FAIL | `non-linearizable` | `35a44df73b36a18f…` |
| `20261017T041748Z-C2-1-real-316f3877` | FAIL | `non-linearizable` | `305fa2765802d2f4…` |

**5 of 5** show `non-linearizable` (§11.1 on these replays: REPRODUCED). The recorded classification stays REPRODUCED (5 of 5 recorded replays): this re-execution is reported beside it, not instead of it.
<!-- /K7-REPLAY -->

## Suggested direction (optional; not verified)

As for FND-C2-6806aeca: whether the pooler's cancel on context end can leave a backend that waits for synchronous replication alone, and whether a leader that cannot reach its synchronous standbys should stop serving primary-port reads before the failover. Both are hypotheses.

## Limitations

- One pinned commit, one host (LIM-01, LIM-02); K7's runs run the metrics exporter, a deployment divergence (LIM-40, LIM-41).
- Which cancel acted is not determined (LIM-33).
- Porcupine judges the observed history only (LIM-08); register writes whose completion is `info` are included with open-ended return (§10.4).
- ADR-0045's predicates were fixed before K7's runs; R-G6 names F1c because K4's recorded run had it. This finding records that the mechanism does not need F1c; the predicates were not changed after the results (ADR-0045 §2).
