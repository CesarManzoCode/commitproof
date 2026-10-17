# Report excerpt — run `20261016T173617Z-C2-3-real-9375b700`

From cpreport's page of the original run: `bin/cpreport render evidence/K7/runs/20261016T173617Z-C2-3-real-9375b700.tar.zst --out <dir>` (record `render-pkg-FND-C2-c7475a73` in `evidence/K7/commands.jsonl`) wrote `20261016T173617Z-C2-3-real-9375b700.html` (994298 bytes, sha256 `513a30da17d8c4ad96b5cd6b8d33657f396c259d0dd93908e2f83399c49cb3da`); the page is byte-reproducible (K6.1) and is not committed. Seal root `d59932aa948d9f940bf882fce51765a339104025c746af9aeb80fdbfa748ea02`; cpcheck's verdict **FAIL**; 7968 history lines, 3969 operations. Faults, rules and leader changes are observations (ADR-0016): they never decide a verdict.

## Faults

| Fault | Kind | Targets | Injected | Confirmed | Heal | Healed | Rule changes | Leader changes |
|---|---|---|---|---|---|---|---|---|
| f0 | F1c | n1 | 10.442 s | 11.673 s | 41.674 s | 41.992 s | 1 (2.0) | 1 |
| f1 | F1c | n3 | 55.483 s | 56.721 s | 86.722 s | 87.030 s | 0 | 0 |
| f2 | F1c | n3 | 101.172 s | 102.419 s | 112.420 s | 112.734 s | 1 (5.0) | 0 |

## Decided rules and leader changes (observations)

| Rule | Leader | Cohort | First seen |
|---|---|---|---|
| 1.0 | n1 | n1, n2, n3 | 0.175 s |
| 2.0 | n3 | n1, n2, n3 | 35.260 s |
| 5.0 | n3 | n1, n2, n3 | 152.262 s |

Leader changes: n1 → n3 at rule 2.0, first seen 35.260 s.

## WAL positions and roles (pooler-status observations, 1 s polling)

Each cell: the node's last successful pooler-status sample at or before the instant (role, decided rule it reports, WAL position). Observations only (ADR-0016); the runner reaches every node's pooler except those a fault isolates from it (LIM-28).

Fault f0 (F1c on n1):

| Node | before inject (10.442 s) | confirmed (11.673 s) | heal (41.674 s) | healed + 10 s (51.992 s) |
|---|---|---|---|---|
| n1 | PRIMARY 1.0 `0/40AF838` (10.259 s) | PRIMARY 1.0 `0/40B0928` (11.258 s) | PRIMARY 1.0 `0/40B31E0` (41.261 s) | REPLICA 1.0 `` (51.251 s) |
| n2 | REPLICA 1.0 `0/40AF838` (10.259 s) | REPLICA 1.0 `0/40B0370` (11.258 s) | REPLICA 2.0 `0/40DA2C8` (41.260 s) | REPLICA 2.0 `0/4143248` (51.259 s) |
| n3 | REPLICA 1.0 `0/40AF838` (10.258 s) | REPLICA 1.0 `0/40B0370` (11.258 s) | PRIMARY 2.0 `0/40DA2C8` (41.260 s) | PRIMARY 2.0 `0/4143248` (51.261 s) |

Fault f1 (F1c on n3):

| Node | before inject (55.483 s) | confirmed (56.721 s) | heal (86.722 s) | healed + 10 s (97.030 s) |
|---|---|---|---|---|
| n1 | REPLICA 1.0 `` (55.252 s) | REPLICA 1.0 `` (56.250 s) | REPLICA 1.0 `` (86.254 s) | REPLICA 1.0 `` (96.253 s) |
| n2 | REPLICA 2.0 `0/4169168` (55.258 s) | REPLICA 2.0 `0/416C348` (56.257 s) | REPLICA 2.0 `0/416C348` (86.258 s) | REPLICA 2.0 `0/416C348` (96.257 s) |
| n3 | PRIMARY 2.0 `0/4169168` (55.259 s) | PRIMARY 2.0 `0/416CAE0` (56.257 s) | PRIMARY 2.0 `0/416E910` (86.258 s) | PRIMARY 2.0 `0/419BB00` (96.259 s) |

Fault f2 (F1c on n3):

| Node | before inject (101.172 s) | confirmed (102.419 s) | heal (112.420 s) | healed + 10 s (122.734 s) |
|---|---|---|---|---|
| n1 | REPLICA 1.0 `` (100.257 s) | REPLICA 1.0 `` (102.255 s) | REPLICA 1.0 `` (112.255 s) | REPLICA 1.0 `` (122.255 s) |
| n2 | REPLICA 2.0 `0/416C348` (100.260 s) | REPLICA 2.0 `0/416C348` (102.257 s) | REPLICA 2.0 `0/416C348` (112.258 s) | REPLICA 2.0 `0/416C348` (122.259 s) |
| n3 | PRIMARY 2.0 `0/419BB00` (100.260 s) | PRIMARY 2.0 `0/419BB00` (102.258 s) | PRIMARY 2.0 `0/419BB00` (112.262 s) | PRIMARY 2.0 `0/419BD88` (122.261 s) |

Role changes: n1 PRIMARY → REPLICA first seen 43.292 s (rule 1.0, WAL ``); n3 REPLICA → PRIMARY first seen 35.260 s (rule 2.0, WAL `0/40B8760`); n3 PRIMARY → REPLICA first seen 151.298 s (rule 2.0, WAL `0/50000A0`); n3 REPLICA → PRIMARY first seen 152.263 s (rule 5.0, WAL `0/50032C0`).

Highest WAL position each node reported: n1 `0/40B31E0`; n2 `0/50112D8`; n3 `0/50112D8`.

## Unavailability windows per gateway (ADR-0036)

| Fault | Gateway | Window | Start | End | Length | Non-ok operations |
|---|---|---|---|---|---|---|
| f0 | gw-z1 | window | 11.130 s | 42.005 s | 30.875 s | 86 |
| f0 | gw-z2 | window | 20.127 s | 41.997 s | 21.871 s | 39 |
| f0 | gw-z3 | window | 20.227 s | 42.002 s | 21.775 s | 47 |
| f1 | gw-z1 | window | 65.538 s | 152.769 s | 87.231 s | 21 |
| f1 | gw-z2 | window | 65.540 s | 152.770 s | 87.229 s | 8 |
| f1 | gw-z3 | window | 65.591 s | 152.770 s | 87.179 s | 18 |
| f2 | gw-z1 | window | 122.736 s | 152.769 s | 30.033 s | 0 |
| f2 | gw-z2 | window | 122.736 s | 152.770 s | 30.034 s | 0 |
| f2 | gw-z3 | window | 122.736 s | 152.770 s | 30.034 s | 0 |

## Latency per phase (ms)

| Fault | Phase | Operations | p50 | p95 | p99 |
|---|---|---|---|---|---|
| f0 | before | 1439 | 5.882 | 12.209 | 14.319 |
| f0 | during | 230 | 5.862 | 14.962 | 19.557 |
| f0 | after | 1317 | 6.091 | 11.281 | 15.820 |
| f1 | before | 1361 | 6.128 | 9.196 | 13.434 |
| f1 | during | 29 | 5.314 | 15.783 | 18.834 |
| f1 | after | 0 | — | — | — |
| f2 | before | 0 | — | — | — |
| f2 | during | 0 | — | — | — |
| f2 | after | 0 | — | — | — |

## Checkers (cpcheck, `checks.json`)

| Checker | Valid | Anomaly classes (count) |
|---|---|---|
| C1 | true | — |
| C2 | true | — |
| C3 | false | `G-nonadjacent` (1), `G-single` (1), `G2-item` (1), `incompatible-order` (1) |
| C4 | true | — |
| C5 | true | — |
| C6 | true | — |
| C7 | true | — |
| C8 | true | — |
| C9 | true | — |
| P | true | — |

First anomalies of C3 (cpcheck lists at most 20 per class):

- `G-nonadjacent` (ops 3178, 3214, 3226, 3230): cycle i=3214 -rw(k=9)-> i=3226 -ww(k=9)-> i=3230 -rw(k=13)-> i=3178 -wr(k=12)-> i=3214
- `G-single` (ops 3178, 3230): cycle i=3230 -rw(k=13)-> i=3178 -ww(k=12)-> i=3230
- `G2-item` (ops 3178, 3214, 3230): cycle i=3214 -rw(k=12)-> i=3230 -rw(k=13)-> i=3178 -wr(k=12)-> i=3214
- `incompatible-order` (ops 3214, 3813): reads of key 14 are not prefixes of each other: [1 2 4 5 7 9 11 12 13 14 15 17 18 19 20 21 22 23 26 27 28 29 30 33 34 35 36 38 40 41 43 44 45 46 48] (i=3214) and [1 2 4 5 7 9 11 12 13 14 15 17 18 19 20 21 22 23 26 27 28 29 30 33 34 35 36 38 40 41 43 44 45 46 50 51 53 55 57 59 58 64 54 56 61 62 63] (i=3813)
