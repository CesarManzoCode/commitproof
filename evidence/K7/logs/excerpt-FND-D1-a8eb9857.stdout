# Report excerpt — run `20261011T221728Z-D1-4-real-ad3fca18`

From cpreport's page of the original run: `bin/cpreport render evidence/K4/runs/20261011T221728Z-D1-4-real-ad3fca18.tar.zst --out <dir>` (record `render-pkg-FND-D1-a8eb9857` in `evidence/K7/commands.jsonl`) wrote `20261011T221728Z-D1-4-real-ad3fca18.html` (1407388 bytes, sha256 `8d8c58362465297d2ed91862a53443ff742479cbc68bf814bcfb8794052e3a97`); the page is byte-reproducible (K6.1) and is not committed. Seal root `45a068bff1408e07a3b68ad6f5c536cb12ae01b8d03f6660cdc51f6aca08c2b8`; cpcheck's verdict **FAIL**; 14072 history lines, 7026 operations. Faults, rules and leader changes are observations (ADR-0016): they never decide a verdict.

## Faults

| Fault | Kind | Targets | Injected | Confirmed | Heal | Healed | Rule changes | Leader changes |
|---|---|---|---|---|---|---|---|---|
| f0 | F1c | n1 | 10.666 s | 11.880 s | 41.880 s | 42.169 s | 1 (2.0) | 1 |

## Decided rules and leader changes (observations)

| Rule | Leader | Cohort | First seen |
|---|---|---|---|
| 1.0 | n1 | n1, n2, n3 | 0.279 s |
| 2.0 | n3 | n1, n2, n3 | 35.357 s |

Leader changes: n1 → n3 at rule 2.0, first seen 35.357 s.

## WAL positions and roles (pooler-status observations, 1 s polling)

Each cell: the node's last successful pooler-status sample at or before the instant (role, decided rule it reports, WAL position). Observations only (ADR-0016); the runner reaches every node's pooler except those a fault isolates from it (LIM-28).

Fault f0 (F1c on n1):

| Node | before inject (10.666 s) | confirmed (11.880 s) | heal (41.880 s) | healed + 10 s (52.169 s) |
|---|---|---|---|---|
| n1 | PRIMARY 1.0 `0/40AA8E8` (10.359 s) | PRIMARY 1.0 `0/40AD8F8` (11.358 s) | PRIMARY 1.0 `0/40B0328` (41.361 s) | REPLICA 1.0 `` (51.354 s) |
| n2 | REPLICA 1.0 `0/40AA8E8` (10.359 s) | REPLICA 1.0 `0/40AD380` (11.358 s) | REPLICA 2.0 `0/40D33D0` (41.361 s) | REPLICA 2.0 `0/4147C50` (51.359 s) |
| n3 | REPLICA 1.0 `0/40AA8E8` (10.359 s) | REPLICA 1.0 `0/40AD380` (11.358 s) | PRIMARY 2.0 `0/40D33D0` (41.361 s) | PRIMARY 2.0 `0/4147C50` (51.359 s) |

Role changes: n1 PRIMARY → REPLICA first seen 43.356 s (rule 1.0, WAL ``); n3 REPLICA → PRIMARY first seen 35.357 s (rule 2.0, WAL `0/40B5770`).

Highest WAL position each node reported: n1 `0/40B0328`; n2 `0/4331968`; n3 `0/4331968`.

## Unavailability windows per gateway (ADR-0036)

| Fault | Gateway | Window | Start | End | Length | Non-ok operations |
|---|---|---|---|---|---|---|
| f0 | gw-z1 | window | 11.488 s | 42.212 s | 30.724 s | 59 |
| f0 | gw-z2 | window | 11.487 s | 42.275 s | 30.788 s | 30 |
| f0 | gw-z3 | window | 20.586 s | 42.171 s | 21.585 s | 17 |

## Latency per phase (ms)

| Fault | Phase | Operations | p50 | p95 | p99 |
|---|---|---|---|---|---|
| f0 | before | 1060 | 6.469 | 13.035 | 15.475 |
| f0 | during | 164 | 11.912 | 22.783 | 121.591 |
| f0 | after | 1127 | 5.096 | 36.618 | 63.971 |

## Checkers (cpcheck, `checks.json`)

| Checker | Valid | Anomaly classes (count) |
|---|---|---|
| C1 | true | — |
| C2 | true | — |
| C3 | false | `incompatible-order` (5) |
| C4 | true | — |
| C5 | true | — |
| C6 | true | — |
| C7 | true | — |
| C8 | true | — |
| C9 | true | — |
| P | true | — |

First anomalies of C3 (cpcheck lists at most 20 per class):

- `incompatible-order` (ops 2438, 2506): reads of key 14 are not prefixes of each other: [2 4 5 6 7 8 9 10 11 12 13 14 15 16 18 19 20 21 22 23 26 28 29 30 32 34 35 37 38 39 40 41 43 44 45 46 47 50 51 55] (i=2438) and [2 4 5 6 7 8 9 10 11 12 13 14 15 16 18 19 20 21 22 23 26 28 29 30 32 34 35 37 38 39 40 41 43 44 45 46 47 50 51 61 60 63] (i=2506)
- `incompatible-order` (ops 2442, 2506): reads of key 14 are not prefixes of each other: [2 4 5 6 7 8 9 10 11 12 13 14 15 16 18 19 20 21 22 23 26 28 29 30 32 34 35 37 38 39 40 41 43 44 45 46 47 50 51 55] (i=2442) and [2 4 5 6 7 8 9 10 11 12 13 14 15 16 18 19 20 21 22 23 26 28 29 30 32 34 35 37 38 39 40 41 43 44 45 46 47 50 51 61 60 63] (i=2506)
- `incompatible-order` (ops 2449, 2506): reads of key 14 are not prefixes of each other: [2 4 5 6 7 8 9 10 11 12 13 14 15 16 18 19 20 21 22 23 26 28 29 30 32 34 35 37 38 39 40 41 43 44 45 46 47 50 51 55] (i=2449) and [2 4 5 6 7 8 9 10 11 12 13 14 15 16 18 19 20 21 22 23 26 28 29 30 32 34 35 37 38 39 40 41 43 44 45 46 47 50 51 61 60 63] (i=2506)
