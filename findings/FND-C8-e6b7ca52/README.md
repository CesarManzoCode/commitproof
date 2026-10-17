**Acknowledged writes lost (G1) in this finding's original run: no.**

# multigateway: an explicit transaction started during a buffered planned switchover receives 57P03 instead of 40001

| Dimension | Value (contract 1.9.0) | Derivation |
|---|---|---|
| Finding | FND-C8-e6b7ca52 | §11.3: sha256(`PROTOCOL_DEVIATION\|G8\|planned-failover-error\|<minimized plan>`)[:8]; a K7 matrix failure that matches no ADR-0045 predicate |
| Guarantee | **G8** — with buffering enabled, a planned failover that the documented configuration buffers produces zero client-visible errors for single autocommit statements; explicit transactions open across the switch receive `40001` and nothing else (§3) | C9 `planned-failover-error` in a judged F6 window |
| Class | PROTOCOL_DEVIATION | §11.1 |
| Claim level | **DESIGN** (S14, S15) | G8's row |
| Severity (§11.2, derived) | **medium** | PROTOCOL_DEVIATION ∧ DESIGN |
| Origin | **MULTIGRES** (the SQLSTATE is PostgreSQL's; returning it to the client inside a buffered planned failover is the gateway's behavior, which plain PostgreSQL has no counterpart for) | hypothesis (§11.1); see Analysis |
| Reproducibility | NOT_REPRODUCED (0 of 5 recorded replays show `planned-failover-error`) | §11.1, P13 |
| Known status | NEW | — |

Cross-reference: the same mechanism in K7's V3 C8 seeds 3, 10 (FND-C8-ff4475d8, FND-C8-5254b5cb): one package per failing run (§11.3 gives each minimized plan its own id). K5's three C8 runs had none (`evidence/K5/NOTES.md`).

## Summary

In K7's V3 C8 seed 6 (profile `buffered`) on the real build, the run's first F6 gracefully switched the leader n1 over to n2. The explicit transaction i=2408 (gw-z3) began 15 ms after the switchover began and received SQLSTATE `57P03` (cannot_connect_now) on a statement after `BEGIN`, before any of its reads returned a result, not `40001`. G8 allows an explicit transaction in a buffered switchover `40001` and nothing else (§10.10: C9 `planned-failover-error`). In the same window 64 other explicit transactions failed with `40001`, and no single autocommit statement ended `fail` or `info`. No acknowledged write was lost (C1 valid). The class appeared in 0 of 5 replays of the run's plan.

## Version and environment

- Multigres `e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73` built from source with upstream's Dockerfiles (only `FROM` lines changed, §5.2), PostgreSQL 17.7 (`postgres:17.7@sha256:2006493727bd5277eece187319af1ef82b4cf82cf4fc1ed00da0775b646ac2a4`), durability policy `AT_LEAST_2`, cohort {n1, n2, n3}, three cells with one multiorch and one multigateway each (CONTRACT §5.1).
- Profile `buffered` (gateways add `--buffer-enabled=true`, S14 defaults; the runner waits 60 s after HEALTHY so that bootstrap's buffering episode cannot suppress the first failover's, ADR-0029); ADR-0038's OpenTelemetry Prometheus exporter on every multigateway, multipooler and multiorch (K7's runs, LIM-41).
- Host: Arch Linux, kernel 7.2.9-arch1-1, Docker 29.9.0, one bridge network 10.77.0.0/24, all containers on one host (LIM-02, LIM-03).
- Original run `20261016T230045Z-C8-6-real-9601ddce`: CommitProof `32b660a` (clean), contract 1.8.1 (K7's matrix, `campaigns/k7.json`, `make campaign-k7`). The minimized plan is the original's (11 candidates, none showed the class); its five recorded replays were made by CommitProof `32b660a`.
- Client: pgx v5.11.0; P01 deadline 10 s per operation (§6.2).

## Reproduction

From a clone of this repository at tag `v1.0.0` (Linux host meeting `docs/PREREQUISITES.md`):

```sh
make prereq-check                       # 26 checks; exit 0 required
make build
bin/commitproof images build            # pinned images; writes images.lock.json
mkdir -p runs/k7-pkg/FND-C8-e6b7ca52
tar --zstd -C runs/k7-pkg/FND-C8-e6b7ca52 -xf evidence/K7/runs/20261016T230045Z-C8-6-real-9601ddce.tar.zst
bin/commitproof replay runs/k7-pkg/FND-C8-e6b7ca52/20261016T230045Z-C8-6-real-9601ddce --times 5 --out runs/k7-pkg/FND-C8-e6b7ca52 --cpcheck bin/cpcheck
```

Each replay brings the cluster up, runs the minimized plan (`plan.json` here; sha256 of its canonical bytes `9601ddce3ac130d9d9572c9fba4fb97f3f6135e972a75d0f433be72f9ca1e43e`), tears the cluster down, seals and checks the run; the last line is a JSON summary with the replays and how many show the class the driver targets. `tar --zstd -xf` of the original run's archive and `bin/cpcheck check <run-dir>` reproduce the original run's `checks.json` (here) byte for byte.

## Expected vs actual

- **Expected** (G8, DESIGN, S14, S15): in a buffered planned failover, zero client-visible errors for single autocommit statements; an explicit transaction open across the switch receives `40001` and nothing else.
- **Actual** (`20261016T230045Z-C8-6-real-9601ddce`): judged switchover f0 (F6 on the leader n1, injected at 70.273 s; heal confirmed at 80.572 s): the explicit transaction i=2408 (gw-z3) completed `info` with `57P03` at 70.288–70.296 s (phase `statement`: after `BEGIN`, before `COMMIT`), invoked 15 ms after the injection. C9 `planned-failover-error`; every other checker valid.

## Analysis

**Design says** (S14, at `e3cbdbb`): with buffering enabled the gateway buffers or retries only replay-safe errors — no writable primary, `MTF01`, pre-execution unavailable, and `25006` or recovery `0A000` only for a single autocommit statement or the first statement of a deferred read-write transaction. S15: during a planned failover an open transaction is aborted with `40001` ("connection terminated during a planned failover"). PostgreSQL returns `57P03` to new sessions while it shuts down or starts; it is not among the errors S14 buffers.

**System did** (history and observations; `report-excerpt.md`): the F6 injection sent TERM to the leader's pooler container (graceful drain, S15) at 70.273 s; the explicit transaction i=2408 (gw-z3) began 15 ms later and failed with `57P03` on a statement after `BEGIN`; no read of theirs returned a result (§6.2 row 11: a SQLSTATE not listed, `info`; whether a write statement had executed is unknown, which `info` expresses). The old leader stopped answering pooler-status about 1 s after the injection; n2 was primary about 5 s after it; the old leader restarted as a standby. Between the injection and the heal's confirmation 64 other explicit transactions failed with `40001` (SQLSTATE does not tell the drain's aborts from serialization failures), and no single autocommit statement ended `fail` or `info`. The anomaly's operations are `info` with an error, each in its §6.2 row: no completion is misclassified (`evidence/K7/triage/20261016T230045Z-C8-6-real-9601ddce.json`).

The hypothesis is that, within the first milliseconds of the drain, the gateway opened the new transaction on the draining PostgreSQL (S14 mentions deferred read-write transactions, which suggests the backend transaction starts with the first statement) and passed its `57P03` to the client, because that SQLSTATE is not one it buffers (S14). It is not verified: CommitProof records SQLSTATEs at the client and does not trace the gateway's routing.

**Minimization and replays** (§12.2, P13): none of the 11 candidates showed the class, so the minimized plan is the original's (`9601ddce3ac130d9d9572c9fba4fb97f3f6135e972a75d0f433be72f9ca1e43e`). The class appeared in 0 of 5 of its recorded replays (§11.1: NOT_REPRODUCED). The window is narrow (tens of milliseconds after the injection), which fits a timing-dependent outcome.

<!-- K7-RECURRENCES -->

<!-- K7-REPLAY -->

## Suggested direction (optional; not verified)

Whether `classifyError` (S14) should treat `57P03` from a pooler that is draining for a planned failover as replay-safe for the first statement of a deferred transaction, or the gateway should stop routing new transactions to a draining pooler before PostgreSQL starts refusing them. Both are hypotheses.

## Limitations

- One pinned commit, one host (LIM-01, LIM-02); K7's runs run the metrics exporter, a deployment divergence (LIM-40, LIM-41).
- The transactions began during the switchover, not before it: C9 (§10.10) applies G8's "explicit transactions … receive 40001 and nothing else" to every explicit transaction in a judged window, the literal rule (LIM-25). Under a reading of S15 that covers only transactions open before the drain, these errors would not be a deviation.
- Where the `57P03` came from (which pooler, which connection) is inferred from timing, not traced.
- 0 of 5 replays show the class; the evidence is the original history (§11.1).
