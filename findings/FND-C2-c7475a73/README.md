**Acknowledged writes lost (G1) in this finding's original run: no.**

# multipooler/multigateway: list-append cycles through a transaction whose commit the next failover removed

| Dimension | Value (contract 1.9.0) | Derivation |
|---|---|---|
| Finding | FND-C2-c7475a73 | §11.3: sha256(`SAFETY_VIOLATION\|G5\|G-nonadjacent\|<minimized plan>`)[:8]; a K7 matrix failure that matches no ADR-0045 predicate |
| Guarantee | **G5** — transactions at SERIALIZABLE through the primary port are serializable, across failovers (§3) | C3 `G-nonadjacent`, `G-single`, `G2-item` |
| Class | SAFETY_VIOLATION | §11.1 |
| Claim level | **INFERRED** (I2, S63) | G5's row |
| Severity (§11.2, derived) | **high** | SAFETY_VIOLATION ∧ INFERRED |
| Origin | **UNDETERMINED** (every cycle runs through a write the failover removed, the G2b mechanism; whether the cycles are more than that mechanism's effect on the inferred version order is not shown) | hypothesis (§11.1); see Analysis |
| Reproducibility | NOT_REPRODUCED (0 of 5 recorded replays show `G-nonadjacent`) | §11.1, P13 |
| Known status | NEW | — |

Cross-reference: the same run's `incompatible-order` (key 14) matches ADR-0045's R-G2b, the mechanism of FND-D1-fb8d0f8c, FND-D1-2e860fd2, FND-D1-a8eb9857 and FND-C2-2a534585; the cycle classes have no predicate, so the run is a new failure (ADR-0045 §1). The K7.3 oracle record of this run: `evidence/K7/oracle/20261016T173617Z-C2-3-real-9375b700.json`.

## Summary

In K7's V1 C2 seed 3 on the real build, C3 reports three cycles (a `G-nonadjacent`, a `G-single` and a `G2-item`) and one incompatible order. All three cycles run through one transaction, i=3178, whose completion is `info`: its client gave up after 10 s (P01) while F1c had cut the leader n1 off from its followers, orchestrators and etcd but not from the gateways. The element that transaction appended to key 12 (62) was read by later `ok` transactions and is absent from the final reads, after the failover to n3. C3 infers key 12's version order from the transactions' reads, so the order contains that element, and the cycles' `ww` and `wr` edges through i=3178 come from it. elle-cli agrees on validity (false), on the incompatible order and on the same `G-single`; its cycle search timed out on the component, so it gave no answer for `G-nonadjacent` and `G2-item` (oracle-disagreement, triaged). The minimizer found no smaller plan with `G-nonadjacent`, and 0 of 5 replays of the original plan show it; all 5 show the incompatible order (the vanish) without cycles. No acknowledged write was lost (C1 valid); no failed write is visible (C2 valid).

## Version and environment

- Multigres `e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73` built from source with upstream's Dockerfiles (only `FROM` lines changed, §5.2), PostgreSQL 17.7 (`postgres:17.7@sha256:2006493727bd5277eece187319af1ef82b4cf82cf4fc1ed00da0775b646ac2a4`), durability policy `AT_LEAST_2`, cohort {n1, n2, n3}, three cells with one multiorch and one multigateway each (CONTRACT §5.1).
- Profile `baseline` (gateway buffering off, `--enable-slot-based-replication=true` as in upstream's Kubernetes demo); ADR-0038's OpenTelemetry Prometheus exporter on every multigateway, multipooler and multiorch (K7's runs, LIM-41).
- Host: Arch Linux, kernel 7.2.9-arch1-1, Docker 29.9.0, one bridge network 10.77.0.0/24, all containers on one host (LIM-02, LIM-03).
- Original run `20261016T173617Z-C2-3-real-9375b700`: CommitProof `1f1f60b` (clean), contract 1.8.1 (K7's matrix, `campaigns/k7.json`, `make campaign-k7`). Its `checks.json` was rewritten by the C3 correction of `32b660a` before it was committed (record `recheck-c3fix`: the verdict and classes are unchanged; the cycles' operations and explanations are the corrected ones). The minimized plan is the original's (11 candidates, none showed the class); its five recorded replays were made by CommitProof `32b660a`.
- Client: pgx v5.11.0; P01 deadline 10 s per operation (§6.2).

## Reproduction

From a clone of this repository at tag `v1.0.0` (Linux host meeting `docs/PREREQUISITES.md`):

```sh
make prereq-check                       # 26 checks; exit 0 required
make build
bin/commitproof images build            # pinned images; writes images.lock.json
mkdir -p runs/k7-pkg/FND-C2-c7475a73
tar --zstd -C runs/k7-pkg/FND-C2-c7475a73 -xf evidence/K7/runs/20261016T173617Z-C2-3-real-9375b700.tar.zst
bin/commitproof replay runs/k7-pkg/FND-C2-c7475a73/20261016T173617Z-C2-3-real-9375b700 --times 5 --out runs/k7-pkg/FND-C2-c7475a73 --cpcheck bin/cpcheck
```

Each replay brings the cluster up, runs the minimized plan (`plan.json` here; sha256 of its canonical bytes `9375b700cc2bc8dfb50914a958c230f2c0256a8dee205696ea5f82e1b002d8b9`), tears the cluster down, seals and checks the run; the last line is a JSON summary with the replays and how many show the class the driver targets. `tar --zstd -xf` of the original run's archive and `bin/cpcheck check <run-dir>` reproduce the original run's `checks.json` (here) byte for byte.

## Expected vs actual

- **Expected** (G5, INFERRED, I2, S63): transactions at SERIALIZABLE through the primary port are serializable across failovers; C3 (§10.3) reports no `G0`, `G1c`, `G-single`, `G-nonadjacent` or `G2-item` cycle.
- **Actual** (`20261016T173617Z-C2-3-real-9375b700`): `G-nonadjacent` cycle i=3214 -rw(k=9)-> i=3226 -ww(k=9)-> i=3230 -rw(k=13)-> i=3178 -wr(k=12)-> i=3214; `G-single` cycle i=3230 -rw(k=13)-> i=3178 -ww(k=12)-> i=3230; `G2-item` cycle i=3214 -rw(k=12)-> i=3230 -rw(k=13)-> i=3178 -wr(k=12)-> i=3214; and `incompatible-order` on key 14 (value 48, appended by the `info` transaction i=3093, read by `ok` i=3214, absent from the key's longest read i=3813). C1 and C2 valid.

## Analysis

**Design says.** Transactions at SERIALIZABLE through the primary port are serializable across failovers (G5, INFERRED from I2 and S63). The gateway cancels the backend on statement timeout (S16) and forwards client cancels (S17); PostgreSQL ends a cancelled synchronous-replication wait by committing locally (S61). C3 infers each key's version order from the longest transactional read, not from final reads (§10.3, LIM-22).

**System did** (history and observations; `report-excerpt.md`):

| t (s) | Event |
|---|---|
| 10.126 → 20.127 | i=3093 appends 48 to key 14; its client gives up (P01): `info` |
| 10.442 / 11.673 | F1c injected / confirmed on leader n1 (gateways still reach it) |
| 20.227 → 30.227 | i=3178 appends to keys 12 (62) and 13; its client gives up (P01): `info` |
| 31.185–31.191 | i=3214 (`ok`, gw-z2) reads key 12 ending in 62 and key 14 with 48 |
| 35.26 | rule 2.0 with leader n3 first observed (pooler status, every 1 s) |
| 40.230–40.286 | i=3226 and i=3230 (`ok`) on the new primary; i=3230 reads key 13 without i=3178's element and appends 64 to key 12 |
| 41.992 | F1c healed; n1 does not rejoin before the run ends |
| final reads | key 12 ends in 58 64 (62 absent); key 14 lacks 48 |

Every operation the cycles name other than i=3178 is `ok` (§6.2 row 1), and i=3178 is `info` (row 9): no completion is misclassified (`evidence/K7/triage/20261016T173617Z-C2-3-real-9375b700.json`). The vanished elements were written by `info` transactions, exactly as in the G2b findings; the cycles are what C3 derives from a version order that contains one of them. Whether, beyond that, the history holds an isolation anomaly of its own is not shown: the minimizer's 11 candidates and the 5 recorded replays reproduced the vanish (`incompatible-order` in 5 of 5) but never a cycle, and elle-cli confirmed only the `G-single`, the cycle that runs through key 12's vanished element. Origin is therefore UNDETERMINED: POSTGRES_INHERITED if the cycles are the vanish's effect (LIM-33), not determined otherwise.

**Minimization and replays** (§12.2, P13): the driver targeted `G-nonadjacent`; none of the 11 candidates showed it, so the minimized plan is the original's (`9375b700cc2bc8dfb50914a958c230f2c0256a8dee205696ea5f82e1b002d8b9`). Its five recorded replays: FAIL with `incompatible-order` only, 5 of 5; `G-nonadjacent` 0 of 5 (§11.1: NOT_REPRODUCED; the original history is the evidence). A further replay attempt ended INVALID (recovery deadline P06 exceeded; `evidence/K7/NOTES.md` §7) and was re-run with the same plan.

<!-- K7-RECURRENCES -->

<!-- K7-REPLAY -->

## Suggested direction (optional; not verified)

As for the G2b findings: whether a backend whose synchronous-replication wait was cancelled should keep serving reads on an isolated leader. A checker-side hypothesis: excluding from C3's version order the elements absent from every final read would show whether the cycles survive without the vanished element; CommitProof does not do this (LIM-22).

## Limitations

- One pinned commit, one host (LIM-01, LIM-02); K7's runs run the metrics exporter, a deployment divergence (LIM-40, LIM-41).
- C3 infers version orders from transactional reads only (LIM-22); its cycle classes presuppose one version order per key, which a vanished element breaks (the K4 triage of FND-D1-2e860fd2 states the same for elle-cli).
- elle-cli did not answer for `G-nonadjacent` and `G2-item` (cycle search timeout; LIM-31's family); an unanswered class is not a confirmation (ADR-0046).
- 0 of 5 replays show the class; the evidence is the original history (§11.1).
