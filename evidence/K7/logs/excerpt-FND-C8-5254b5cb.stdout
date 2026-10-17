# Report excerpt — run `20261016T231406Z-C8-10-real-cb16d052`

From cpreport's page of the original run: `bin/cpreport render evidence/K7/runs/20261016T231406Z-C8-10-real-cb16d052.tar.zst --out <dir>` (record `render-pkg-FND-C8-5254b5cb` in `evidence/K7/commands.jsonl`) wrote `20261016T231406Z-C8-10-real-cb16d052.html` (2383542 bytes, sha256 `29982ca226305a405d78e472600472cb3adb9df542a89dd23621863b2875d371`); the page is byte-reproducible (K6.1) and is not committed. Seal root `2232bb76e0f354ee3bf3055e4d6b770bd5a8cd4f34c510a75134877dde04ee80`; cpcheck's verdict **FAIL**; 20391 history lines, 10180 operations. Faults, rules and leader changes are observations (ADR-0016): they never decide a verdict.

## Faults

| Fault | Kind | Targets | Injected | Confirmed | Heal | Healed | Rule changes | Leader changes |
|---|---|---|---|---|---|---|---|---|
| f0 | F6 | pooler-n2 | 70.289 s | 75.440 s | 80.441 s | 81.164 s | 1 (2.1) | 1 |
| f1 | F6 | pooler-n1 | 91.936 s | 96.542 s | 101.543 s | 102.287 s | 4 (2.2, 3.0, 3.1, 3.2) | 1 |
| f2 | F6 | pooler-n3 | 117.087 s | 120.017 s | 125.018 s | 125.740 s | 2 (4.1, 4.2) | 1 |

## Decided rules and leader changes (observations)

| Rule | Leader | Cohort | First seen |
|---|---|---|---|
| 1.0 | n2 | n1, n2, n3 | 0.187 s |
| 2.1 | n1 | n1, n3 | 75.288 s |
| 2.2 | n1 | n1, n2, n3 | 92.274 s |
| 3.0 | n3 | n1, n2, n3 | 96.314 s |
| 3.1 | n3 | n2, n3 | 97.276 s |
| 3.2 | n3 | n1, n2, n3 | 111.281 s |
| 4.1 | n1 | n1, n2 | 120.274 s |
| 4.2 | n1 | n1, n2, n3 | 134.283 s |

Leader changes: n2 → n1 at rule 2.1, first seen 75.288 s; n1 → n3 at rule 3.0, first seen 96.314 s; n3 → n1 at rule 4.1, first seen 120.274 s.

## WAL positions and roles (pooler-status observations, 1 s polling)

Each cell: the node's last successful pooler-status sample at or before the instant (role, decided rule it reports, WAL position). Observations only (ADR-0016); the runner reaches every node's pooler except those a fault isolates from it (LIM-28).

Fault f0 (F6 on pooler-n2):

| Node | before inject (70.289 s) | confirmed (75.440 s) | heal (80.441 s) | healed + 10 s (91.164 s) |
|---|---|---|---|---|
| n1 | REPLICA 1.0 `0/40AD388` (69.276 s) | PRIMARY 2.1 `0/5036258` (75.288 s) | PRIMARY 2.1 `0/5079318` (80.278 s) | PRIMARY 2.1 `0/50E1E90` (90.287 s) |
| n2 | PRIMARY 1.0 `0/40AD388` (69.276 s) | PRIMARY 1.0 `0/40B8610` (70.290 s) | PRIMARY 1.0 `0/40B8610` (70.290 s) | REPLICA 1.0 `` (90.331 s) |
| n3 | REPLICA 1.0 `0/40AD388` (69.276 s) | REPLICA 2.1 `0/5035E40` (75.288 s) | REPLICA 2.1 `0/5078B50` (80.278 s) | REPLICA 2.1 `0/50E1DC8` (90.285 s) |

Fault f1 (F6 on pooler-n1):

| Node | before inject (91.936 s) | confirmed (96.542 s) | heal (101.543 s) | healed + 10 s (112.287 s) |
|---|---|---|---|---|
| n1 | PRIMARY 2.1 `0/50EB400` (91.295 s) | PRIMARY 2.1 `0/50EB400` (91.295 s) | PRIMARY 2.1 `0/50EB400` (91.295 s) | REPLICA 3.2 `0/60DF5F8` (112.277 s) |
| n2 | REPLICA 2.1 `0/50EB590` (91.311 s) | REPLICA 3.0 `0/60181F0` (96.314 s) | REPLICA 3.1 `0/6054660` (101.276 s) | REPLICA 3.2 `0/60DF5F8` (112.277 s) |
| n3 | REPLICA 2.1 `0/50EB400` (91.295 s) | PRIMARY 3.0 `0/601EC60` (96.322 s) | PRIMARY 3.1 `0/6054660` (101.277 s) | PRIMARY 3.2 `0/60DF5F8` (112.277 s) |

Fault f2 (F6 on pooler-n3):

| Node | before inject (117.087 s) | confirmed (120.017 s) | heal (125.018 s) | healed + 10 s (135.740 s) |
|---|---|---|---|---|
| n1 | REPLICA 3.2 `0/6104BC0` (116.277 s) | REPLICA 3.2 `0/70076D0` (119.275 s) | PRIMARY 4.1 `0/704B278` (124.277 s) | PRIMARY 4.2 `0/70C5490` (135.281 s) |
| n2 | REPLICA 3.2 `0/6104BC0` (116.277 s) | REPLICA 3.2 `0/70000A0` (119.274 s) | REPLICA 4.1 `0/704B278` (124.277 s) | REPLICA 4.2 `0/70C5490` (135.280 s) |
| n3 | PRIMARY 3.2 `0/6104BC0` (116.277 s) | PRIMARY 3.2 `0/6104BC0` (116.277 s) | PRIMARY 3.2 `0/6104BC0` (116.277 s) | REPLICA 4.2 `0/70C5490` (135.279 s) |

Role changes: n1 REPLICA → PRIMARY first seen 75.288 s (rule 2.1, WAL `0/5036258`); n1 PRIMARY → REPLICA first seen 109.277 s (rule 2.2, WAL ``); n1 REPLICA → PRIMARY first seen 120.275 s (rule 4.1, WAL `0/70202B8`); n2 PRIMARY → REPLICA first seen 87.328 s (rule 1.0, WAL `0/50000A0`); n3 REPLICA → PRIMARY first seen 96.322 s (rule 3.0, WAL `0/601EC60`); n3 PRIMARY → REPLICA first seen 126.222 s (rule —, WAL ``).

Highest WAL position each node reported: n1 `0/71E9070`; n2 `0/71E9070`; n3 `0/71E9070`.

## Unavailability windows per gateway (ADR-0036)

| Fault | Gateway | Window | Start | End | Length | Non-ok operations |
|---|---|---|---|---|---|---|
| f0 | gw-z1 | window | 70.312 s | 81.184 s | 10.873 s | 118 |
| f0 | gw-z2 | window | 75.530 s | 81.185 s | 5.655 s | 64 |
| f0 | gw-z3 | window | 70.312 s | 81.188 s | 10.876 s | 61 |
| f1 | gw-z1 | window | 92.001 s | 102.325 s | 10.324 s | 386 |
| f1 | gw-z2 | window | 92.007 s | 102.333 s | 10.326 s | 144 |
| f1 | gw-z3 | window | 91.974 s | 102.292 s | 10.318 s | 229 |
| f2 | gw-z1 | window | 117.096 s | 125.788 s | 8.692 s | 258 |
| f2 | gw-z2 | window | 117.148 s | 125.744 s | 8.595 s | 99 |
| f2 | gw-z3 | window | 117.148 s | 125.742 s | 8.594 s | 163 |

## Latency per phase (ms)

| Fault | Phase | Operations | p50 | p95 | p99 |
|---|---|---|---|---|---|
| f0 | before | 1007 | 5.764 | 9.683 | 13.846 |
| f0 | during | 691 | 4.921 | 8.484 | 15.963 |
| f0 | after | 1033 | 4.638 | 9.210 | 10.637 |
| f1 | before | 1024 | 4.726 | 9.545 | 12.584 |
| f1 | during | 661 | 5.951 | 13.581 | 24.455 |
| f1 | after | 1011 | 5.700 | 12.523 | 16.141 |
| f2 | before | 1004 | 5.224 | 9.866 | 14.332 |
| f2 | during | 695 | 5.808 | 11.144 | 14.847 |
| f2 | after | 1008 | 6.915 | 13.254 | 16.292 |

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
| C9 | false | `planned-failover-error` (3) |
| P | true | — |

First anomalies of C9 (cpcheck lists at most 20 per class):

- `planned-failover-error` (ops 2409): explicit transaction append i=2409 completed info (map[class:57 phase:statement sqlstate:57P03]), not 40001, within switchover f0
- `planned-failover-error` (ops 2410): explicit transaction append i=2410 completed info (map[class:57 phase:statement sqlstate:57P03]), not 40001, within switchover f0
- `planned-failover-error` (ops 2411): explicit transaction append i=2411 completed info (map[class:57 phase:statement sqlstate:57P03]), not 40001, within switchover f0
