# Report excerpt — run `20261013T230136Z-C9-1-real-21458975`

From cpreport's page of the original run: `bin/cpreport render evidence/K5/runs/20261013T230136Z-C9-1-real-21458975.tar.zst --out <dir>` (record `render-pkg-FND-C9-40b6f05a` in `evidence/K7/commands.jsonl`) wrote `20261013T230136Z-C9-1-real-21458975.html` (2971164 bytes, sha256 `48b29f7570e4680f8b5101a6f11af91e96bd02b3e3f5d6a2353955d056f6935b`); the page is byte-reproducible (K6.1) and is not committed. Seal root `9ccee7319cfa049663012ecec692ec0295e46286a7bdf4ca46cc99285917db7e`; cpcheck's verdict **FAIL**; 28367 history lines, 14168 operations. Faults, rules and leader changes are observations (ADR-0016): they never decide a verdict.

## Faults

| Fault | Kind | Targets | Injected | Confirmed | Heal | Healed | Rule changes | Leader changes |
|---|---|---|---|---|---|---|---|---|
| f0 | F2b | pooler-n1 | 10.444 s | 10.600 s | 15.601 s | 16.306 s | 0 | 0 |
| f1 | F2a | n1 | 27.535 s | 27.631 s | 27.631 s | 31.586 s | 1 (2.0) | 1 |
| f2 | F6 | pooler-n3 | 45.776 s | 47.575 s | 52.575 s | 53.280 s | 2 (3.1, 3.2) | 1 |

## Decided rules and leader changes (observations)

| Rule | Leader | Cohort | First seen |
|---|---|---|---|
| 1.0 | n1 | n1, n2, n3 | 0.178 s |
| 2.0 | n3 | n1, n2, n3 | 32.256 s |
| 3.1 | n2 | n1, n2 | 48.258 s |
| 3.2 | n2 | n1, n2, n3 | 61.258 s |

Leader changes: n1 → n3 at rule 2.0, first seen 32.256 s; n3 → n2 at rule 3.1, first seen 48.258 s.

## WAL positions and roles (pooler-status observations, 1 s polling)

Each cell: the node's last successful pooler-status sample at or before the instant (role, decided rule it reports, WAL position). Observations only (ADR-0016); the runner reaches every node's pooler except those a fault isolates from it (LIM-28).

Fault f0 (F2b on pooler-n1):

| Node | before inject (10.444 s) | confirmed (10.600 s) | heal (15.601 s) | healed + 10 s (26.306 s) |
|---|---|---|---|---|
| n1 | PRIMARY 1.0 `0/406F9D8` (10.259 s) | PRIMARY 1.0 `0/406F9D8` (10.259 s) | PRIMARY 1.0 `0/406F9D8` (10.259 s) | PRIMARY 1.0 `0/4086068` (26.259 s) |
| n2 | REPLICA 1.0 `0/406F9D8` (10.258 s) | REPLICA 1.0 `0/406F9D8` (10.258 s) | REPLICA 1.0 `0/4070990` (15.256 s) | REPLICA 1.0 `0/4086068` (26.258 s) |
| n3 | REPLICA 1.0 `0/406F9D8` (10.258 s) | REPLICA 1.0 `0/406F9D8` (10.258 s) | REPLICA 1.0 `0/4070990` (15.256 s) | REPLICA 1.0 `0/4086068` (26.258 s) |

Fault f1 (F2a on n1):

| Node | before inject (27.535 s) | confirmed (27.631 s) | heal (27.631 s) | healed + 10 s (41.586 s) |
|---|---|---|---|---|
| n1 | PRIMARY 1.0 `0/408B280` (27.259 s) | PRIMARY 1.0 `0/408B280` (27.259 s) | PRIMARY 1.0 `0/408B280` (27.259 s) | REPLICA 2.0 `0/40CF440` (41.259 s) |
| n2 | REPLICA 1.0 `0/408B280` (27.258 s) | REPLICA 1.0 `0/408B280` (27.258 s) | REPLICA 1.0 `0/408B280` (27.258 s) | REPLICA 2.0 `0/40CF440` (41.258 s) |
| n3 | REPLICA 1.0 `0/408B280` (27.258 s) | REPLICA 1.0 `0/408B280` (27.258 s) | REPLICA 1.0 `0/408B280` (27.258 s) | PRIMARY 2.0 `0/40CF440` (41.259 s) |

Fault f2 (F6 on pooler-n3):

| Node | before inject (45.776 s) | confirmed (47.575 s) | heal (52.575 s) | healed + 10 s (63.280 s) |
|---|---|---|---|---|
| n1 | REPLICA 2.0 `0/40E3D40` (45.259 s) | REPLICA 2.0 `0/50000D8` (47.258 s) | REPLICA 3.1 `0/5029CC0` (52.259 s) | REPLICA 3.2 `0/5062290` (63.259 s) |
| n2 | REPLICA 2.0 `0/40E3D40` (45.259 s) | REPLICA 2.0 `0/50000D8` (47.261 s) | PRIMARY 3.1 `0/5029CC0` (52.260 s) | PRIMARY 3.2 `0/5062290` (63.259 s) |
| n3 | PRIMARY 2.0 `0/40E3D40` (45.259 s) | PRIMARY 2.0 `0/40E3D40` (45.259 s) | PRIMARY 2.0 `0/40E3D40` (45.259 s) | REPLICA 3.2 `0/5062290` (63.258 s) |

Role changes: n1 PRIMARY → REPLICA first seen 16.445 s (rule 1.0, WAL `0/4070E80`); n1 REPLICA → PRIMARY first seen 21.258 s (rule 1.0, WAL `0/4071EE8`); n1 PRIMARY → REPLICA first seen 31.265 s (rule 1.0, WAL ``); n2 REPLICA → PRIMARY first seen 48.258 s (rule 3.1, WAL `0/500EA48`); n3 REPLICA → PRIMARY first seen 32.256 s (rule 2.0, WAL `0/40A0E30`); n3 PRIMARY → REPLICA first seen 56.236 s (rule —, WAL ``).

Highest WAL position each node reported: n1 `0/511E178`; n2 `0/511E178`; n3 `0/511E178`.

## Unavailability windows per gateway (ADR-0036)

| Fault | Gateway | Window | Start | End | Length | Non-ok operations |
|---|---|---|---|---|---|---|
| f0 | gw-z1 | window | 10.468 s | 16.375 s | 5.908 s | 783 |
| f0 | gw-z2 | window | 10.469 s | 16.375 s | 5.906 s | 414 |
| f0 | gw-z3 | window | 10.468 s | 16.375 s | 5.908 s | 621 |
| f1 | gw-z1 | window | 27.588 s | 31.790 s | 4.202 s | 58 |
| f1 | gw-z2 | window | 27.689 s | 31.787 s | 4.098 s | 37 |
| f1 | gw-z3 | window | 27.589 s | 31.790 s | 4.200 s | 120 |
| f2 | gw-z1 | window | 45.836 s | 53.323 s | 7.488 s | 93 |
| f2 | gw-z2 | window | 45.836 s | 53.283 s | 7.447 s | 62 |
| f2 | gw-z3 | window | 45.836 s | 53.323 s | 7.487 s | 93 |

## Latency per phase (ms)

| Fault | Phase | Operations | p50 | p95 | p99 |
|---|---|---|---|---|---|
| f0 | before | 1614 | 5.200 | 14.582 | 1000.778 |
| f0 | during | 15 | 1000.850 | 1001.038 | 1001.038 |
| f0 | after | 724 | 4.597 | 12.854 | 1000.722 |
| f1 | before | 923 | 4.743 | 12.497 | 1000.711 |
| f1 | during | 13 | 1000.687 | 1001.106 | 1001.106 |
| f1 | after | 1588 | 5.427 | 13.686 | 1000.568 |
| f2 | before | 1606 | 4.460 | 12.867 | 1000.548 |
| f2 | during | 965 | 5.378 | 12.333 | 1000.685 |
| f2 | after | 1606 | 5.435 | 13.112 | 1000.513 |

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
| C8 | false | `notify-not-resumed` (6822) |
| C9 | true | — |
| P | true | — |

First anomalies of C8 (cpcheck lists at most 20 per class):

- `notify-not-resumed` (ops 13, 9835): NOTIFY 2949 (i=9835) was not received within 10s by listener client 26, whose session (LISTEN i=13) was alive and past its first successful op after recovery
- `notify-not-resumed` (ops 8, 9835): NOTIFY 2949 (i=9835) was not received within 10s by listener client 27, whose session (LISTEN i=8) was alive and past its first successful op after recovery
- `notify-not-resumed` (ops 10, 9835): NOTIFY 2949 (i=9835) was not received within 10s by listener client 28, whose session (LISTEN i=10) was alive and past its first successful op after recovery
