# Report excerpt — run `20261016T230045Z-C8-6-real-9601ddce`

From cpreport's page of the original run: `bin/cpreport render evidence/K7/runs/20261016T230045Z-C8-6-real-9601ddce.tar.zst --out <dir>` (record `render-pkg-FND-C8-e6b7ca52` in `evidence/K7/commands.jsonl`) wrote `20261016T230045Z-C8-6-real-9601ddce.html` (2241554 bytes, sha256 `1b172eb634e8b39f4d7cdfe338b716f11da9478154a8d0d77e6dbc7d85af2559`); the page is byte-reproducible (K6.1) and is not committed. Seal root `b0605bbb4c445d34bfd3df15434aba0c768e28e5f4e88ae43cb7230c76430028`; cpcheck's verdict **FAIL**; 20401 history lines, 10185 operations. Faults, rules and leader changes are observations (ADR-0016): they never decide a verdict.

## Faults

| Fault | Kind | Targets | Injected | Confirmed | Heal | Healed | Rule changes | Leader changes |
|---|---|---|---|---|---|---|---|---|
| f0 | F6 | pooler-n1 | 70.273 s | 75.400 s | 80.400 s | 80.572 s | 3 (2.0, 2.1, 2.2) | 1 |
| f1 | F6 | pooler-n2 | 91.313 s | 92.596 s | 97.596 s | 97.762 s | 2 (3.1, 3.2) | 1 |
| f2 | F6 | pooler-n3 | 108.302 s | 110.116 s | 115.116 s | 115.291 s | 3 (4.0, 4.1, 4.2) | 1 |

## Decided rules and leader changes (observations)

| Rule | Leader | Cohort | First seen |
|---|---|---|---|
| 1.0 | n1 | n1, n2, n3 | 0.177 s |
| 2.0 | n2 | n1, n2, n3 | 75.259 s |
| 2.1 | n2 | n2, n3 | 76.258 s |
| 2.2 | n2 | n1, n2, n3 | 89.262 s |
| 3.1 | n3 | n1, n3 | 93.262 s |
| 3.2 | n3 | n1, n2, n3 | 106.264 s |
| 4.0 | n1 | n1, n2, n3 | 110.262 s |
| 4.1 | n1 | n1, n2 | 111.260 s |
| 4.2 | n1 | n1, n2, n3 | 124.264 s |

Leader changes: n1 → n2 at rule 2.0, first seen 75.259 s; n2 → n3 at rule 3.1, first seen 93.262 s; n3 → n1 at rule 4.0, first seen 110.262 s.

## WAL positions and roles (pooler-status observations, 1 s polling)

Each cell: the node's last successful pooler-status sample at or before the instant (role, decided rule it reports, WAL position). Observations only (ADR-0016); the runner reaches every node's pooler except those a fault isolates from it (LIM-28).

Fault f0 (F6 on pooler-n1):

| Node | before inject (70.273 s) | confirmed (75.400 s) | heal (80.400 s) | healed + 10 s (90.572 s) |
|---|---|---|---|---|
| n1 | PRIMARY 1.0 `0/40AA730` (69.260 s) | PRIMARY 1.0 `0/40B3958` (70.275 s) | PRIMARY 1.0 `0/40B3958` (70.275 s) | REPLICA 2.2 `0/50DD5D0` (90.261 s) |
| n2 | REPLICA 1.0 `0/40B3958` (70.266 s) | PRIMARY 2.0 `0/502B3F8` (75.261 s) | PRIMARY 2.1 `0/506A818` (80.258 s) | PRIMARY 2.2 `0/50DD5D0` (90.261 s) |
| n3 | REPLICA 1.0 `0/40AA730` (69.259 s) | REPLICA 2.0 `0/502B3F8` (75.259 s) | REPLICA 2.1 `0/506A818` (80.258 s) | REPLICA 2.2 `0/50DD5D0` (90.261 s) |

Fault f1 (F6 on pooler-n2):

| Node | before inject (91.313 s) | confirmed (92.596 s) | heal (97.596 s) | healed + 10 s (107.762 s) |
|---|---|---|---|---|
| n1 | REPLICA 2.2 `0/50E7D30` (91.267 s) | REPLICA 2.2 `0/60000A0` (92.263 s) | REPLICA 3.1 `0/606BDF0` (97.259 s) | REPLICA 3.2 `0/60E21F0` (107.265 s) |
| n2 | PRIMARY 2.2 `0/50E7D30` (91.268 s) | PRIMARY 2.2 `0/50E7D30` (91.268 s) | PRIMARY 2.2 `0/50E7D30` (91.268 s) | REPLICA 3.2 `0/60E21F0` (107.262 s) |
| n3 | REPLICA 2.2 `0/50E7D30` (91.272 s) | REPLICA 2.2 `0/60000D8` (92.267 s) | PRIMARY 3.1 `0/606BDF0` (97.260 s) | PRIMARY 3.2 `0/60E21F0` (107.266 s) |

Fault f2 (F6 on pooler-n3):

| Node | before inject (108.302 s) | confirmed (110.116 s) | heal (115.116 s) | healed + 10 s (125.291 s) |
|---|---|---|---|---|
| n1 | REPLICA 3.2 `0/60EE578` (108.269 s) | REPLICA 3.2 `0/70000A0` (109.261 s) | PRIMARY 4.1 `0/70497C0` (114.264 s) | PRIMARY 4.2 `0/70DC890` (125.262 s) |
| n2 | REPLICA 3.2 `0/60EE578` (108.268 s) | REPLICA 3.2 `0/70000A0` (109.261 s) | REPLICA 4.1 `0/70493A0` (114.263 s) | REPLICA 4.2 `0/70DC890` (125.263 s) |
| n3 | PRIMARY 3.2 `0/60EEB30` (108.270 s) | PRIMARY 3.2 `0/60EEB30` (108.270 s) | PRIMARY 3.2 `0/60EEB30` (108.270 s) | REPLICA 4.2 `0/70DC890` (125.262 s) |

Role changes: n1 PRIMARY → REPLICA first seen 86.262 s (rule 1.0, WAL `0/50000A0`); n1 REPLICA → PRIMARY first seen 110.262 s (rule 4.0, WAL `0/7018BD0`); n2 REPLICA → PRIMARY first seen 75.261 s (rule 2.0, WAL `0/502B3F8`); n2 PRIMARY → REPLICA first seen 102.209 s (rule —, WAL ``); n3 REPLICA → PRIMARY first seen 93.263 s (rule 3.1, WAL `0/603BA88`); n3 PRIMARY → REPLICA first seen 118.210 s (rule —, WAL ``).

Highest WAL position each node reported: n1 `0/72605C0`; n2 `0/72605C0`; n3 `0/72605C0`.

## Unavailability windows per gateway (ADR-0036)

| Fault | Gateway | Window | Start | End | Length | Non-ok operations |
|---|---|---|---|---|---|---|
| f0 | gw-z1 | window | 74.937 s | 80.587 s | 5.650 s | 122 |
| f0 | gw-z2 | window | 70.296 s | 80.586 s | 10.290 s | 6 |
| f0 | gw-z3 | window | 70.296 s | 80.588 s | 10.292 s | 51 |
| f1 | gw-z1 | window | 91.325 s | 97.773 s | 6.447 s | 186 |
| f1 | gw-z2 | window | 91.354 s | 97.775 s | 6.421 s | 24 |
| f1 | gw-z3 | window | 91.354 s | 97.773 s | 6.419 s | 81 |
| f2 | gw-z1 | window | 108.340 s | 115.293 s | 6.953 s | 205 |
| f2 | gw-z2 | window | 108.341 s | 115.325 s | 6.985 s | 33 |
| f2 | gw-z3 | window | 108.340 s | 115.297 s | 6.957 s | 104 |

## Latency per phase (ms)

| Fault | Phase | Operations | p50 | p95 | p99 |
|---|---|---|---|---|---|
| f0 | before | 1006 | 5.753 | 9.199 | 11.920 |
| f0 | during | 619 | 5.061 | 8.377 | 12.239 |
| f0 | after | 1091 | 5.816 | 9.296 | 11.524 |
| f1 | before | 1089 | 5.879 | 9.311 | 11.581 |
| f1 | during | 587 | 5.892 | 11.233 | 16.369 |
| f1 | after | 1083 | 5.866 | 9.290 | 11.655 |
| f2 | before | 1077 | 5.872 | 9.165 | 11.349 |
| f2 | during | 595 | 5.848 | 11.106 | 14.693 |
| f2 | after | 1085 | 5.936 | 9.943 | 12.978 |

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
| C8 | true | — |
| C9 | false | `planned-failover-error` (1) |
| P | true | — |

First anomalies of C9 (cpcheck lists at most 20 per class):

- `planned-failover-error` (ops 2408): explicit transaction append i=2408 completed info (map[class:57 phase:statement sqlstate:57P03]), not 40001, within switchover f0
