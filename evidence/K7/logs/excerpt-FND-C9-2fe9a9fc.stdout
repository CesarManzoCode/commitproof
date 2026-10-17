# Report excerpt — run `20261013T230358Z-C9-2-real-b0861f34`

From cpreport's page of the original run: `bin/cpreport render evidence/K5/runs/20261013T230358Z-C9-2-real-b0861f34.tar.zst --out <dir>` (record `render-pkg-FND-C9-2fe9a9fc` in `evidence/K7/commands.jsonl`) wrote `20261013T230358Z-C9-2-real-b0861f34.html` (3077632 bytes, sha256 `5d572fa62e099ce07510a31d606e245e1944a908481fcd7abde00f01cdc8d8f5`); the page is byte-reproducible (K6.1) and is not committed. Seal root `392e548c0fc99ca55ca84a4b2a487a5402cb108c87c306ed52fea0684d1e6700`; cpcheck's verdict **FAIL**; 28711 history lines, 14340 operations. Faults, rules and leader changes are observations (ADR-0016): they never decide a verdict.

## Faults

| Fault | Kind | Targets | Injected | Confirmed | Heal | Healed | Rule changes | Leader changes |
|---|---|---|---|---|---|---|---|---|
| f0 | F2a | n1 | 10.478 s | 10.571 s | 10.571 s | 12.887 s | 1 (2.0) | 1 |
| f1 | F2b | pooler-n3 | 25.449 s | 25.735 s | 30.735 s | 31.432 s | 0 | 0 |
| f2 | F6 | pooler-n3 | 43.996 s | 45.785 s | 50.785 s | 55.388 s | 2 (3.1, 3.2) | 1 |

## Decided rules and leader changes (observations)

| Rule | Leader | Cohort | First seen |
|---|---|---|---|
| 1.0 | n1 | n1, n2, n3 | 0.178 s |
| 2.0 | n3 | n1, n2, n3 | 14.260 s |
| 3.1 | n2 | n1, n2 | 46.258 s |
| 3.2 | n2 | n1, n2, n3 | 65.258 s |

Leader changes: n1 → n3 at rule 2.0, first seen 14.260 s; n3 → n2 at rule 3.1, first seen 46.258 s.

## WAL positions and roles (pooler-status observations, 1 s polling)

Each cell: the node's last successful pooler-status sample at or before the instant (role, decided rule it reports, WAL position). Observations only (ADR-0016); the runner reaches every node's pooler except those a fault isolates from it (LIM-28).

Fault f0 (F2a on n1):

| Node | before inject (10.478 s) | confirmed (10.571 s) | heal (10.571 s) | healed + 10 s (22.887 s) |
|---|---|---|---|---|
| n1 | PRIMARY 1.0 `0/406F6C8` (10.258 s) | PRIMARY 1.0 `0/406F6C8` (10.258 s) | PRIMARY 1.0 `0/406F6C8` (10.258 s) | REPLICA 2.0 `0/40ADEC8` (22.259 s) |
| n2 | REPLICA 1.0 `0/406F6C8` (10.258 s) | REPLICA 1.0 `0/406F6C8` (10.258 s) | REPLICA 1.0 `0/406F6C8` (10.258 s) | REPLICA 2.0 `0/40ADEC8` (22.259 s) |
| n3 | REPLICA 1.0 `0/406F6C8` (10.257 s) | REPLICA 1.0 `0/406F6C8` (10.257 s) | REPLICA 1.0 `0/406F6C8` (10.257 s) | PRIMARY 2.0 `0/40ADEC8` (22.259 s) |

Fault f1 (F2b on pooler-n3):

| Node | before inject (25.449 s) | confirmed (25.735 s) | heal (30.735 s) | healed + 10 s (41.432 s) |
|---|---|---|---|---|
| n1 | REPLICA 2.0 `0/40BC870` (25.258 s) | REPLICA 2.0 `0/40BC870` (25.258 s) | REPLICA 2.0 `0/40BD848` (30.256 s) | REPLICA 2.0 `0/40C9558` (41.259 s) |
| n2 | REPLICA 2.0 `0/40BC870` (25.258 s) | REPLICA 2.0 `0/40BC870` (25.258 s) | REPLICA 2.0 `0/40BD848` (30.256 s) | REPLICA 2.0 `0/40C9558` (41.259 s) |
| n3 | PRIMARY 2.0 `0/40BC870` (25.259 s) | PRIMARY 2.0 `0/40BC870` (25.259 s) | PRIMARY 2.0 `0/40BC870` (25.259 s) | PRIMARY 2.0 `0/40C9558` (41.259 s) |

Fault f2 (F6 on pooler-n3):

| Node | before inject (43.996 s) | confirmed (45.785 s) | heal (50.785 s) | healed + 10 s (65.388 s) |
|---|---|---|---|---|
| n1 | REPLICA 2.0 `0/40CF538` (43.258 s) | REPLICA 2.0 `0/50000D8` (45.265 s) | REPLICA 3.1 `0/502C118` (50.257 s) | REPLICA 3.2 `0/5076AE0` (65.258 s) |
| n2 | REPLICA 2.0 `0/40CF538` (43.258 s) | REPLICA 2.0 `0/50000D8` (45.268 s) | PRIMARY 3.1 `0/502C118` (50.258 s) | PRIMARY 3.2 `0/5076AE0` (65.259 s) |
| n3 | PRIMARY 2.0 `0/40CF538` (43.259 s) | PRIMARY 2.0 `0/40CF538` (43.259 s) | PRIMARY 2.0 `0/40CF538` (43.259 s) | REPLICA 3.2 `0/5076AE0` (65.258 s) |

Role changes: n1 PRIMARY → REPLICA first seen 13.294 s (rule 1.0, WAL `0/4070F20`); n2 REPLICA → PRIMARY first seen 46.258 s (rule 3.1, WAL `0/5015DA0`); n3 REPLICA → PRIMARY first seen 14.260 s (rule 2.0, WAL `0/4081F68`); n3 PRIMARY → REPLICA first seen 32.309 s (rule 2.0, WAL `0/40BF720`); n3 REPLICA → PRIMARY first seen 36.259 s (rule 2.0, WAL `0/40BFB08`); n3 PRIMARY → REPLICA first seen 60.292 s (rule —, WAL ``).

Highest WAL position each node reported: n1 `0/51174C0`; n2 `0/51174C0`; n3 `0/51174C0`.

## Unavailability windows per gateway (ADR-0036)

| Fault | Gateway | Window | Start | End | Length | Non-ok operations |
|---|---|---|---|---|---|---|
| f0 | gw-z1 | window | 10.562 s | 13.372 s | 2.810 s | 72 |
| f0 | gw-z2 | window | 10.532 s | 13.372 s | 2.840 s | 51 |
| f0 | gw-z3 | window | 10.562 s | 13.372 s | 2.810 s | 73 |
| f1 | gw-z1 | window | 25.474 s | 32.381 s | 6.907 s | 948 |
| f1 | gw-z2 | window | 25.476 s | 32.381 s | 6.905 s | 418 |
| f1 | gw-z3 | window | 25.474 s | 32.381 s | 6.907 s | 801 |
| f2 | gw-z1 | window | 44.047 s | 55.390 s | 11.343 s | 147 |
| f2 | gw-z2 | window | 44.048 s | 55.395 s | 11.348 s | 58 |
| f2 | gw-z3 | window | 44.047 s | 55.395 s | 11.348 s | 87 |

## Latency per phase (ms)

| Fault | Phase | Operations | p50 | p95 | p99 |
|---|---|---|---|---|---|
| f0 | before | 1614 | 5.156 | 14.418 | 1000.299 |
| f0 | during | 13 | 9.098 | 1001.049 | 1001.049 |
| f0 | after | 1518 | 5.076 | 11.440 | 1000.349 |
| f1 | before | 1614 | 5.086 | 12.098 | 1000.302 |
| f1 | during | 18 | 1000.561 | 1000.945 | 1000.945 |
| f1 | after | 391 | 4.327 | 1000.300 | 1001.031 |
| f2 | before | 646 | 4.121 | 10.268 | 1001.031 |
| f2 | during | 1601 | 5.342 | 12.403 | 1000.457 |
| f2 | after | 1606 | 5.645 | 13.740 | 1000.724 |

## Checkers (cpcheck, `checks.json`)

| Checker | Valid | Anomaly classes (count) |
|---|---|---|
| C1 | true | — |
| C2 | true | — |
| C3 | true | — |
| C4 | true | — |
| C5 | true | — |
| C6 | true | — |
| C7 | true | — |
| C8 | false | `notify-not-resumed` (5877) |
| C9 | true | — |
| P | true | — |

First anomalies of C8 (cpcheck lists at most 20 per class):

- `notify-not-resumed` (ops 8, 4388): NOTIFY 1329 (i=4388) was not received within 10s by listener client 26, whose session (LISTEN i=8) was alive and past its first successful op after recovery
- `notify-not-resumed` (ops 4, 4388): NOTIFY 1329 (i=4388) was not received within 10s by listener client 27, whose session (LISTEN i=4) was alive and past its first successful op after recovery
- `notify-not-resumed` (ops 12, 4388): NOTIFY 1329 (i=4388) was not received within 10s by listener client 28, whose session (LISTEN i=12) was alive and past its first successful op after recovery
