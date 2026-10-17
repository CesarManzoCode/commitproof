# Report excerpt — run `20261011T221506Z-D1-3-real-da159fa0`

From cpreport's page of the original run: `bin/cpreport render evidence/K4/runs/20261011T221506Z-D1-3-real-da159fa0.tar.zst --out <dir>` (record `render-pkg-FND-D1-2e860fd2` in `evidence/K7/commands.jsonl`) wrote `20261011T221506Z-D1-3-real-da159fa0.html` (1315131 bytes, sha256 `9dca294ad54c18340a1b02189f94f0e86745b332aabb335e5b9e14f59fcad26c`); the page is byte-reproducible (K6.1) and is not committed. Seal root `7926f6cb4aeea04a8732c8e40aeba5d3671f083604b5db00b87bde490f3c5a96`; cpcheck's verdict **FAIL**; 12942 history lines, 6461 operations. Faults, rules and leader changes are observations (ADR-0016): they never decide a verdict.

## Faults

| Fault | Kind | Targets | Injected | Confirmed | Heal | Healed | Rule changes | Leader changes |
|---|---|---|---|---|---|---|---|---|
| f0 | F1c | n2 | 10.655 s | 11.877 s | 41.877 s | 42.189 s | 1 (2.0) | 1 |

## Decided rules and leader changes (observations)

| Rule | Leader | Cohort | First seen |
|---|---|---|---|
| 1.0 | n2 | n1, n2, n3 | 0.348 s |
| 2.0 | n3 | n1, n2, n3 | 39.428 s |

Leader changes: n2 → n3 at rule 2.0, first seen 39.428 s.

## WAL positions and roles (pooler-status observations, 1 s polling)

Each cell: the node's last successful pooler-status sample at or before the instant (role, decided rule it reports, WAL position). Observations only (ADR-0016); the runner reaches every node's pooler except those a fault isolates from it (LIM-28).

Fault f0 (F1c on n2):

| Node | before inject (10.655 s) | confirmed (11.877 s) | heal (41.877 s) | healed + 10 s (52.189 s) |
|---|---|---|---|---|
| n1 | REPLICA 1.0 `0/4095C00` (10.428 s) | REPLICA 1.0 `0/4099130` (11.425 s) | REPLICA 2.0 `0/40BB1F0` (41.428 s) | REPLICA 2.0 `0/4123BB8` (51.427 s) |
| n2 | PRIMARY 1.0 `0/4095C00` (10.428 s) | PRIMARY 1.0 `0/4099A18` (11.426 s) | PRIMARY 1.0 `0/409B640` (41.428 s) | REPLICA 1.0 `` (51.420 s) |
| n3 | REPLICA 1.0 `0/4095C00` (10.428 s) | REPLICA 1.0 `0/4099130` (11.426 s) | PRIMARY 2.0 `0/40BB1F0` (41.428 s) | PRIMARY 2.0 `0/4123BB8` (51.427 s) |

Role changes: n2 PRIMARY → REPLICA first seen 46.073 s (rule 1.0, WAL ``); n3 REPLICA → PRIMARY first seen 39.429 s (rule 2.0, WAL `0/40A17F0`).

Highest WAL position each node reported: n1 `0/42BE540`; n2 `0/409B640`; n3 `0/42BE540`.

## Unavailability windows per gateway (ADR-0036)

| Fault | Gateway | Window | Start | End | Length | Non-ok operations |
|---|---|---|---|---|---|---|
| f0 | gw-z1 | window | 20.751 s | 42.190 s | 21.439 s | 86 |
| f0 | gw-z2 | window | 20.745 s | 42.229 s | 21.484 s | 43 |
| f0 | gw-z3 | window | 20.751 s | 42.224 s | 21.472 s | 42 |

## Latency per phase (ms)

| Fault | Phase | Operations | p50 | p95 | p99 |
|---|---|---|---|---|---|
| f0 | before | 947 | 6.270 | 13.957 | 16.784 |
| f0 | during | 160 | 7.275 | 21.142 | 25.772 |
| f0 | after | 1007 | 6.090 | 12.762 | 15.232 |

## Checkers (cpcheck, `checks.json`)

| Checker | Valid | Anomaly classes (count) |
|---|---|---|
| C1 | true | — |
| C2 | true | — |
| C3 | false | `incompatible-order` (1) |
| C4 | true | — |
| C5 | true | — |
| C6 | true | — |
| C7 | true | — |
| C8 | true | — |
| C9 | true | — |
| P | true | — |

First anomalies of C3 (cpcheck lists at most 20 per class):

- `incompatible-order` (ops 2142, 2523): reads of key 13 are not prefixes of each other: [1 2 3 4 6 7 8 9 10 11 12 13 15 16 17 18 20 22 21 23 25 28 26 29 30 31 32 33 36 34 35] (i=2142) and [1 2 3 4 6 7 8 9 10 11 12 13 15 16 17 18 20 22 21 23 25 28 26 29 30 31 32 33 36 34 37 38 41 43 42 44 45 49 50 51 54 55 52 53 59 58 60 62 63] (i=2523)
