# Report excerpt — run `20261011T231036Z-C2-3-real-f4505b54`

From cpreport's page of the original run: `bin/cpreport render evidence/K4/runs/20261011T231036Z-C2-3-real-f4505b54.tar.zst --out <dir>` (record `render-pkg-FND-C2-6806aeca` in `evidence/K7/commands.jsonl`) wrote `20261011T231036Z-C2-3-real-f4505b54.html` (841275 bytes, sha256 `35dc3e484d12349a16546967d953001065287c0f015c5ac0a878c1c67118c94c`); the page is byte-reproducible (K6.1) and is not committed. Seal root `a5472bd9c4212b63ab9397b7ff513a05ba7efe81aeeed54c7da492d6b20c4969`; cpcheck's verdict **FAIL**; 7862 history lines, 3916 operations. Faults, rules and leader changes are observations (ADR-0016): they never decide a verdict.

## Faults

| Fault | Kind | Targets | Injected | Confirmed | Heal | Healed | Rule changes | Leader changes |
|---|---|---|---|---|---|---|---|---|
| f0 | F1c | n1 | 10.652 s | 11.881 s | 41.882 s | 42.184 s | 1 (2.0) | 1 |
| f1 | F1c | n2 | 55.714 s | 56.953 s | 86.954 s | 87.221 s | 1 (4.0) | 0 |
| f2 | F1c | n2 | 105.320 s | 106.556 s | 116.557 s | 116.886 s | 0 | 0 |

## Decided rules and leader changes (observations)

| Rule | Leader | Cohort | First seen |
|---|---|---|---|
| 1.0 | n1 | n1, n2, n3 | 0.272 s |
| 2.0 | n2 | n1, n2, n3 | 36.349 s |
| 4.0 | n2 | n1, n2, n3 | 102.350 s |

Leader changes: n1 → n2 at rule 2.0, first seen 36.349 s.

## WAL positions and roles (pooler-status observations, 1 s polling)

Each cell: the node's last successful pooler-status sample at or before the instant (role, decided rule it reports, WAL position). Observations only (ADR-0016); the runner reaches every node's pooler except those a fault isolates from it (LIM-28).

Fault f0 (F1c on n1):

| Node | before inject (10.652 s) | confirmed (11.881 s) | heal (41.882 s) | healed + 10 s (52.184 s) |
|---|---|---|---|---|
| n1 | PRIMARY 1.0 `0/40ACE90` (10.352 s) | PRIMARY 1.0 `0/40B2198` (11.351 s) | PRIMARY 1.0 `0/40B3808` (41.354 s) | REPLICA 1.0 `` (51.342 s) |
| n2 | REPLICA 1.0 `0/40ACE90` (10.352 s) | REPLICA 1.0 `0/40B1768` (11.351 s) | PRIMARY 2.0 `0/40D7548` (41.352 s) | PRIMARY 2.0 `0/4148D58` (51.352 s) |
| n3 | REPLICA 1.0 `0/40ACE90` (10.352 s) | REPLICA 1.0 `0/40B1768` (11.351 s) | REPLICA 2.0 `0/40D7548` (41.351 s) | REPLICA 2.0 `0/4148CD0` (51.351 s) |

Fault f1 (F1c on n2):

| Node | before inject (55.714 s) | confirmed (56.953 s) | heal (86.954 s) | healed + 10 s (97.221 s) |
|---|---|---|---|---|
| n1 | REPLICA 1.0 `` (55.344 s) | REPLICA 1.0 `` (56.342 s) | REPLICA 1.0 `` (86.347 s) | REPLICA 1.0 `` (96.348 s) |
| n2 | PRIMARY 2.0 `0/41685B8` (55.351 s) | PRIMARY 2.0 `0/416BD58` (56.348 s) | PRIMARY 2.0 `0/416E070` (86.352 s) | PRIMARY 2.0 `0/4197A88` (96.354 s) |
| n3 | REPLICA 2.0 `0/41685B8` (55.351 s) | REPLICA 2.0 `0/416B4A0` (56.348 s) | REPLICA 2.0 `0/416B4A0` (86.352 s) | REPLICA 2.0 `0/416B4A0` (96.353 s) |

Fault f2 (F1c on n2):

| Node | before inject (105.320 s) | confirmed (106.556 s) | heal (116.557 s) | healed + 10 s (126.886 s) |
|---|---|---|---|---|
| n1 | REPLICA 1.0 `` (104.347 s) | REPLICA 1.0 `` (106.347 s) | REPLICA 1.0 `` (116.345 s) | REPLICA 1.0 `` (126.347 s) |
| n2 | PRIMARY 4.0 `0/5003CB0` (104.350 s) | PRIMARY 4.0 `0/5003E50` (106.352 s) | PRIMARY 4.0 `0/5003E88` (116.349 s) | PRIMARY 4.0 `0/5007BF8` (126.350 s) |
| n3 | REPLICA 4.0 `0/5003CB0` (104.350 s) | REPLICA 4.0 `0/5003D80` (106.352 s) | REPLICA 4.0 `0/5003D80` (116.348 s) | REPLICA 4.0 `0/5007BF8` (126.350 s) |

Role changes: n1 PRIMARY → REPLICA first seen 45.854 s (rule 1.0, WAL ``); n2 REPLICA → PRIMARY first seen 36.349 s (rule 2.0, WAL `0/40B9B58`); n2 PRIMARY → REPLICA first seen 101.386 s (rule 2.0, WAL `0/50022A0`); n2 REPLICA → PRIMARY first seen 102.351 s (rule 4.0, WAL `0/50032C0`).

Highest WAL position each node reported: n1 `0/40B3808`; n2 `0/500DCD8`; n3 `0/500DCD8`.

## Unavailability windows per gateway (ADR-0036)

| Fault | Gateway | Window | Start | End | Length | Non-ok operations |
|---|---|---|---|---|---|---|
| f0 | gw-z1 | window | 10.692 s | 42.205 s | 31.512 s | 77 |
| f0 | gw-z2 | window | 20.731 s | 42.199 s | 21.468 s | 48 |
| f0 | gw-z3 | window | 10.692 s | 42.198 s | 31.506 s | 60 |
| f1 | gw-z1 | window | 55.744 s | 116.905 s | 61.161 s | 21 |
| f1 | gw-z2 | window | 55.745 s | 116.906 s | 61.161 s | 12 |
| f1 | gw-z3 | window | 65.777 s | 116.906 s | 51.129 s | 15 |
| f2 | gw-z1 | none | — | — | — | 0 |
| f2 | gw-z2 | none | — | — | — | 0 |
| f2 | gw-z3 | none | — | — | — | 0 |

## Latency per phase (ms)

| Fault | Phase | Operations | p50 | p95 | p99 |
|---|---|---|---|---|---|
| f0 | before | 1445 | 6.027 | 11.928 | 14.053 |
| f0 | during | 233 | 6.811 | 16.022 | 26.095 |
| f0 | after | 1381 | 6.281 | 11.746 | 15.018 |
| f1 | before | 1241 | 6.267 | 9.709 | 14.193 |
| f1 | during | 25 | 2.559 | 10.348 | 11.755 |
| f1 | after | 0 | — | — | — |
| f2 | before | 0 | — | — | — |
| f2 | during | 0 | — | — | — |
| f2 | after | 3 | 20.325 | 20.441 | 20.441 |

## Checkers (cpcheck, `checks.json`)

| Checker | Valid | Anomaly classes (count) |
|---|---|---|
| C1 | true | — |
| C2 | true | — |
| C3 | false | `incompatible-order` (2) |
| C4 | false | `non-linearizable` (2) |
| C5 | true | — |
| C6 | true | — |
| C7 | true | — |
| C8 | true | — |
| C9 | true | — |
| P | true | — |

First anomalies of C3 (cpcheck lists at most 20 per class):

- `incompatible-order` (ops 3298, 3591): reads of key 8 are not prefixes of each other: [3 4 5 6 7 8 9 10 11 12 13 14 15 16 18 21 22 23 25 26 28 29 36 37 40 41 42 43 44 46 47 49 51] (i=3298) and [3 4 5 6 7 8 9 10 11 12 13 14 15 16 18 21 22 23 25 26 28 29 36 37 40 41 42 43 44 46 47 49 54 55 56 57 58 59 61 62 63] (i=3591)
- `incompatible-order` (ops 3298, 3541): reads of key 14 are not prefixes of each other: [1 3 4 5 7 8 9 11 12 13 14 15 17 18 19 20 21 22 23 26 27 29 30 33 34 35 36 38 40 41 43 44 45 46 48 50 51 53] (i=3298) and [1 3 4 5 7 8 9 11 12 13 14 15 17 18 19 20 21 22 23 26 27 29 30 33 34 35 36 38 40 41 43 44 45 46 48 50 51 59 61 62 63] (i=3541)

First anomalies of C4 (cpcheck lists at most 20 per class):

- `non-linearizable` (ops 3370): register 1: no linearization; the earliest offending operation is cas i=3370 ({f:cas v:0 old:240 new_:245} -> {known:true set:false v:0 applied:false})
- `non-linearizable` (ops 3412): register 4: no linearization; the earliest offending operation is read i=3412 ({f:read v:0 old:0 new_:0} -> {known:true set:true v:227 applied:false})
