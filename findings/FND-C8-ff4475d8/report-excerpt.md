# Report excerpt — run `20261016T225050Z-C8-3-real-537e02d1`

From cpreport's page of the original run: `bin/cpreport render evidence/K7/runs/20261016T225050Z-C8-3-real-537e02d1.tar.zst --out <dir>` (record `render-pkg-FND-C8-ff4475d8` in `evidence/K7/commands.jsonl`) wrote `20261016T225050Z-C8-3-real-537e02d1.html` (2395207 bytes, sha256 `0483ced4963a33ad3bf5f4ab8cf4d25ca145c5adc8d18cb3ee1ce50bfdbcc4eb`); the page is byte-reproducible (K6.1) and is not committed. Seal root `3638579c2d8a74ab522bc2d32e8508a3384d44359b15f8bc2460e28efdd1ed87`; cpcheck's verdict **FAIL**; 20401 history lines, 10185 operations. Faults, rules and leader changes are observations (ADR-0016): they never decide a verdict.

## Faults

| Fault | Kind | Targets | Injected | Confirmed | Heal | Healed | Rule changes | Leader changes |
|---|---|---|---|---|---|---|---|---|
| f0 | F6 | pooler-n1 | 70.273 s | 75.413 s | 80.414 s | 80.578 s | 2 (2.1, 2.2) | 1 |
| f1 | F6 | pooler-n2 | 93.651 s | 98.222 s | 103.223 s | 103.935 s | 3 (3.0, 3.1, 3.2) | 1 |
| f2 | F6 | pooler-n3 | 117.341 s | 119.154 s | 124.155 s | 124.323 s | 3 (4.0, 4.1, 4.2) | 1 |

## Decided rules and leader changes (observations)

| Rule | Leader | Cohort | First seen |
|---|---|---|---|
| 1.0 | n1 | n1, n2, n3 | 0.182 s |
| 2.1 | n2 | n2, n3 | 75.272 s |
| 2.2 | n2 | n1, n2, n3 | 89.270 s |
| 3.0 | n3 | n1, n2, n3 | 98.268 s |
| 3.1 | n3 | n1, n3 | 99.273 s |
| 3.2 | n3 | n1, n2, n3 | 112.280 s |
| 4.0 | n2 | n1, n2, n3 | 119.265 s |
| 4.1 | n2 | n1, n2 | 120.268 s |
| 4.2 | n2 | n1, n2, n3 | 133.269 s |

Leader changes: n1 → n2 at rule 2.1, first seen 75.272 s; n2 → n3 at rule 3.0, first seen 98.268 s; n3 → n2 at rule 4.0, first seen 119.265 s.

## WAL positions and roles (pooler-status observations, 1 s polling)

Each cell: the node's last successful pooler-status sample at or before the instant (role, decided rule it reports, WAL position). Observations only (ADR-0016); the runner reaches every node's pooler except those a fault isolates from it (LIM-28).

Fault f0 (F6 on pooler-n1):

| Node | before inject (70.273 s) | confirmed (75.413 s) | heal (80.414 s) | healed + 10 s (90.578 s) |
|---|---|---|---|---|
| n1 | PRIMARY 1.0 `0/40B4CC0` (70.271 s) | PRIMARY 1.0 `0/40B4CC0` (70.271 s) | PRIMARY 1.0 `0/40B4CC0` (70.271 s) | REPLICA 2.2 `0/50C0028` (90.267 s) |
| n2 | REPLICA 1.0 `0/40B4CC0` (70.273 s) | PRIMARY 2.1 `0/502CB58` (75.273 s) | PRIMARY 2.1 `0/50633E8` (80.266 s) | PRIMARY 2.2 `0/50C0028` (90.268 s) |
| n3 | REPLICA 1.0 `0/40A9208` (69.269 s) | REPLICA 2.1 `0/502CB58` (75.272 s) | REPLICA 2.1 `0/50633E8` (80.265 s) | REPLICA 2.2 `0/50C0028` (90.267 s) |

Fault f1 (F6 on pooler-n2):

| Node | before inject (93.651 s) | confirmed (98.222 s) | heal (103.223 s) | healed + 10 s (113.935 s) |
|---|---|---|---|---|
| n1 | REPLICA 2.2 `0/50DC6B8` (93.267 s) | REPLICA 2.2 `0/60000A0` (97.269 s) | REPLICA 3.1 `0/60405E8` (102.264 s) | REPLICA 3.2 `0/60B6200` (113.272 s) |
| n2 | PRIMARY 2.2 `0/50DC6B8` (93.269 s) | PRIMARY 2.2 `0/50DC6B8` (93.269 s) | PRIMARY 2.2 `0/50DC6B8` (93.269 s) | REPLICA 3.2 `0/60B6200` (113.272 s) |
| n3 | REPLICA 2.2 `0/50DC6B8` (93.267 s) | REPLICA 2.2 `0/60000A0` (97.269 s) | PRIMARY 3.1 `0/60405E8` (102.264 s) | PRIMARY 3.2 `0/60B6200` (113.273 s) |

Fault f2 (F6 on pooler-n3):

| Node | before inject (117.341 s) | confirmed (119.154 s) | heal (124.155 s) | healed + 10 s (134.323 s) |
|---|---|---|---|---|
| n1 | REPLICA 3.2 `0/60DEAF0` (117.268 s) | REPLICA 3.2 `0/70000A0` (118.270 s) | REPLICA 4.1 `0/7050A28` (123.267 s) | REPLICA 4.2 `0/70BF6D8` (134.269 s) |
| n2 | REPLICA 3.2 `0/60DEAF0` (117.268 s) | REPLICA 3.2 `0/70000A0` (118.271 s) | PRIMARY 4.1 `0/7050A28` (123.268 s) | PRIMARY 4.2 `0/70BF6D8` (134.270 s) |
| n3 | PRIMARY 3.2 `0/60DEAF0` (117.268 s) | PRIMARY 3.2 `0/60DEAF0` (117.268 s) | PRIMARY 3.2 `0/60DEAF0` (117.268 s) | REPLICA 4.2 `0/70BF6D8` (134.269 s) |

Role changes: n1 PRIMARY → REPLICA first seen 82.212 s (rule —, WAL ``); n2 REPLICA → PRIMARY first seen 75.273 s (rule 2.1, WAL `0/502CB58`); n2 PRIMARY → REPLICA first seen 104.216 s (rule —, WAL ``); n2 REPLICA → PRIMARY first seen 119.265 s (rule 4.0, WAL `0/7029E50`); n3 REPLICA → PRIMARY first seen 98.268 s (rule 3.0, WAL `0/6019E10`); n3 PRIMARY → REPLICA first seen 128.215 s (rule —, WAL ``).

Highest WAL position each node reported: n1 `0/71CDEF0`; n2 `0/71CDEF0`; n3 `0/71CDEF0`.

## Unavailability windows per gateway (ADR-0036)

| Fault | Gateway | Window | Start | End | Length | Non-ok operations |
|---|---|---|---|---|---|---|
| f0 | gw-z1 | window | 70.296 s | 80.612 s | 10.316 s | 137 |
| f0 | gw-z2 | window | 70.296 s | 80.612 s | 10.316 s | 69 |
| f0 | gw-z3 | window | 75.150 s | 80.615 s | 5.464 s | 70 |
| f1 | gw-z1 | window | 93.665 s | 103.978 s | 10.313 s | 422 |
| f1 | gw-z2 | window | 93.713 s | 104.031 s | 10.319 s | 150 |
| f1 | gw-z3 | window | 93.673 s | 103.980 s | 10.307 s | 233 |
| f2 | gw-z1 | window | 117.378 s | 124.357 s | 6.979 s | 255 |
| f2 | gw-z2 | window | 117.378 s | 124.362 s | 6.983 s | 107 |
| f2 | gw-z3 | window | 117.378 s | 124.359 s | 6.980 s | 126 |

## Latency per phase (ms)

| Fault | Phase | Operations | p50 | p95 | p99 |
|---|---|---|---|---|---|
| f0 | before | 1002 | 5.754 | 9.588 | 12.605 |
| f0 | during | 616 | 5.688 | 10.072 | 15.261 |
| f0 | after | 1009 | 5.917 | 9.822 | 12.917 |
| f1 | before | 1010 | 6.291 | 9.966 | 13.931 |
| f1 | during | 628 | 6.101 | 10.563 | 16.350 |
| f1 | after | 991 | 6.415 | 9.809 | 14.277 |
| f2 | before | 1012 | 6.662 | 10.300 | 15.793 |
| f2 | during | 545 | 6.301 | 13.845 | 20.577 |
| f2 | after | 1005 | 6.386 | 9.814 | 13.932 |

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
| C9 | false | `planned-failover-error` (2) |
| P | true | — |

First anomalies of C9 (cpcheck lists at most 20 per class):

- `planned-failover-error` (ops 2410): explicit transaction append i=2410 completed info (map[class:57 phase:statement sqlstate:57P03]), not 40001, within switchover f0
- `planned-failover-error` (ops 2411): explicit transaction append i=2411 completed info (map[class:57 phase:statement sqlstate:57P03]), not 40001, within switchover f0
