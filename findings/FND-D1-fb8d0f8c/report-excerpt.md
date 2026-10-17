# Report excerpt — run `20261011T221027Z-D1-1-real-59a95336`

From cpreport's page of the original run: `bin/cpreport render evidence/K4/runs/20261011T221027Z-D1-1-real-59a95336.tar.zst --out <dir>` (record `render-pkg-FND-D1-fb8d0f8c` in `evidence/K7/commands.jsonl`) wrote `20261011T221027Z-D1-1-real-59a95336.html` (1303771 bytes, sha256 `af8bb0b919cc90ab153f599237f1ef6487b5da48cfe032031491030995ff8796`); the page is byte-reproducible (K6.1) and is not committed. Seal root `c8e958a5776b5d0a8e1627b0ca35ec9fd79def2ac6592a9f4cdcfc97773181cc`; cpcheck's verdict **FAIL**; 12752 history lines, 6366 operations. Faults, rules and leader changes are observations (ADR-0016): they never decide a verdict.

## Faults

| Fault | Kind | Targets | Injected | Confirmed | Heal | Healed | Rule changes | Leader changes |
|---|---|---|---|---|---|---|---|---|
| f0 | F1c | n1 | 10.687 s | 11.912 s | 41.913 s | 42.226 s | 1 (2.0) | 1 |

## Decided rules and leader changes (observations)

| Rule | Leader | Cohort | First seen |
|---|---|---|---|
| 1.0 | n1 | n1, n2, n3 | 0.396 s |
| 2.0 | n3 | n1, n2, n3 | 35.474 s |

Leader changes: n1 → n3 at rule 2.0, first seen 35.474 s.

## WAL positions and roles (pooler-status observations, 1 s polling)

Each cell: the node's last successful pooler-status sample at or before the instant (role, decided rule it reports, WAL position). Observations only (ADR-0016); the runner reaches every node's pooler except those a fault isolates from it (LIM-28).

Fault f0 (F1c on n1):

| Node | before inject (10.687 s) | confirmed (11.912 s) | heal (41.913 s) | healed + 10 s (52.226 s) |
|---|---|---|---|---|
| n1 | PRIMARY 1.0 `0/409DB38` (10.475 s) | PRIMARY 1.0 `0/409E770` (11.474 s) | PRIMARY 1.0 `0/40A0160` (41.478 s) | REPLICA 1.0 `` (51.466 s) |
| n2 | REPLICA 1.0 `0/409DAC8` (10.475 s) | REPLICA 1.0 `0/409E3F0` (11.473 s) | REPLICA 2.0 `0/40CA9D8` (41.476 s) | REPLICA 2.0 `0/41269D8` (51.472 s) |
| n3 | REPLICA 1.0 `0/409DAC8` (10.475 s) | REPLICA 1.0 `0/409E3F0` (11.473 s) | PRIMARY 2.0 `0/40CA9D8` (41.476 s) | PRIMARY 2.0 `0/41269D8` (51.473 s) |

Role changes: n1 PRIMARY → REPLICA first seen 45.392 s (rule 1.0, WAL ``); n3 REPLICA → PRIMARY first seen 35.474 s (rule 2.0, WAL `0/40A67E0`).

Highest WAL position each node reported: n1 `0/40A0160`; n2 `0/42B59D8`; n3 `0/42B59D8`.

## Unavailability windows per gateway (ADR-0036)

| Fault | Gateway | Window | Start | End | Length | Non-ok operations |
|---|---|---|---|---|---|---|
| f0 | gw-z1 | window | 20.218 s | 42.250 s | 22.031 s | 86 |
| f0 | gw-z2 | window | 11.218 s | 42.252 s | 31.034 s | 35 |
| f0 | gw-z3 | window | 20.216 s | 42.252 s | 22.036 s | 29 |

## Latency per phase (ms)

| Fault | Phase | Operations | p50 | p95 | p99 |
|---|---|---|---|---|---|
| f0 | before | 971 | 6.268 | 13.361 | 15.550 |
| f0 | during | 206 | 7.189 | 18.403 | 21.511 |
| f0 | after | 943 | 6.185 | 14.011 | 37.170 |

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

- `incompatible-order` (ops 2220, 3292): reads of key 16 are not prefixes of each other: [1] (i=2220) and [7 5 6 9 8 10 12 13 11 16 18 14 19 15 17 20 23 25 22 27 26 24 33 34 32 28 37 35 36 30 39 40 42 41 43 44 49 47 50 48 54 55 56 59 53 61 58 60 62] (i=3292)
