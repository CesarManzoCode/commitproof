# Report excerpt — run `20261013T230909Z-C9-3-real-bb8a276f`

From cpreport's page of the original run: `bin/cpreport render evidence/K5/runs/20261013T230909Z-C9-3-real-bb8a276f.tar.zst --out <dir>` (record `render-pkg-FND-C9-90da5532` in `evidence/K7/commands.jsonl`) wrote `20261013T230909Z-C9-3-real-bb8a276f.html` (3219313 bytes, sha256 `ff90e40daafac1bf1ab76b3484351a815e8107cac86c677ee9ffdf196763a15c`); the page is byte-reproducible (K6.1) and is not committed. Seal root `b2d189b9510ccab37b20788a3bf43ce21d22669b2ebb39bd7b4bc9faa41a24b9`; cpcheck's verdict **FAIL**; 28767 history lines, 14368 operations. Faults, rules and leader changes are observations (ADR-0016): they never decide a verdict.

## Faults

| Fault | Kind | Targets | Injected | Confirmed | Heal | Healed | Rule changes | Leader changes |
|---|---|---|---|---|---|---|---|---|
| f0 | F6 | pooler-n2 | 10.430 s | 12.239 s | 17.240 s | 17.393 s | 3 (2.0, 2.1, 2.2) | 1 |
| f1 | F2a | n1 | 30.231 s | 30.327 s | 30.327 s | 34.299 s | 1 (3.0) | 1 |
| f2 | F2b | pooler-n3 | 45.950 s | 46.104 s | 51.104 s | 51.810 s | 0 | 0 |

## Decided rules and leader changes (observations)

| Rule | Leader | Cohort | First seen |
|---|---|---|---|
| 1.0 | n2 | n1, n2, n3 | 0.173 s |
| 2.0 | n1 | n1, n2, n3 | 12.284 s |
| 2.1 | n1 | n1, n3 | 13.252 s |
| 2.2 | n1 | n1, n2, n3 | 26.253 s |
| 3.0 | n3 | n1, n2, n3 | 32.256 s |

Leader changes: n2 → n1 at rule 2.0, first seen 12.284 s; n1 → n3 at rule 3.0, first seen 32.256 s.

## WAL positions and roles (pooler-status observations, 1 s polling)

Each cell: the node's last successful pooler-status sample at or before the instant (role, decided rule it reports, WAL position). Observations only (ADR-0016); the runner reaches every node's pooler except those a fault isolates from it (LIM-28).

Fault f0 (F6 on pooler-n2):

| Node | before inject (10.430 s) | confirmed (12.239 s) | heal (17.240 s) | healed + 10 s (27.393 s) |
|---|---|---|---|---|
| n1 | REPLICA 1.0 `0/406F9D8` (10.254 s) | REPLICA 1.0 `0/50000A0` (11.251 s) | PRIMARY 2.1 `0/5023610` (16.254 s) | PRIMARY 2.2 `0/5059F60` (27.255 s) |
| n2 | PRIMARY 1.0 `0/406F9D8` (10.255 s) | PRIMARY 1.0 `0/406F9D8` (10.255 s) | PRIMARY 1.0 `0/406F9D8` (10.255 s) | REPLICA 2.2 `0/5059F60` (27.254 s) |
| n3 | REPLICA 1.0 `0/406F9D8` (10.254 s) | REPLICA 1.0 `0/50000A0` (11.251 s) | REPLICA 2.1 `0/5023610` (16.253 s) | REPLICA 2.2 `0/5059F60` (27.254 s) |

Fault f1 (F2a on n1):

| Node | before inject (30.231 s) | confirmed (30.327 s) | heal (30.327 s) | healed + 10 s (44.299 s) |
|---|---|---|---|---|
| n1 | PRIMARY 2.2 `0/50640C8` (29.255 s) | PRIMARY 2.2 `0/5068FE8` (30.257 s) | PRIMARY 2.2 `0/5068FE8` (30.257 s) | REPLICA 3.0 `0/50B3A80` (44.255 s) |
| n2 | REPLICA 2.2 `0/50640C8` (29.254 s) | REPLICA 2.2 `0/5068FE8` (30.257 s) | REPLICA 2.2 `0/5068FE8` (30.257 s) | REPLICA 3.0 `0/50B3A80` (44.255 s) |
| n3 | REPLICA 2.2 `0/50640C8` (29.254 s) | REPLICA 2.2 `0/5068FE8` (30.257 s) | REPLICA 2.2 `0/5068FE8` (30.257 s) | PRIMARY 3.0 `0/50B3CA8` (44.255 s) |

Fault f2 (F2b on pooler-n3):

| Node | before inject (45.950 s) | confirmed (46.104 s) | heal (51.104 s) | healed + 10 s (61.810 s) |
|---|---|---|---|---|
| n1 | REPLICA 3.0 `0/50B8B50` (45.252 s) | REPLICA 3.0 `0/50B8B50` (45.252 s) | REPLICA 3.0 `0/50BC338` (50.251 s) | REPLICA 3.0 `0/50BE860` (61.252 s) |
| n2 | REPLICA 3.0 `0/50B8B50` (45.252 s) | REPLICA 3.0 `0/50B8B50` (45.252 s) | REPLICA 3.0 `0/50BC338` (50.251 s) | REPLICA 3.0 `0/50BE860` (61.252 s) |
| n3 | PRIMARY 3.0 `0/50B8B50` (45.253 s) | PRIMARY 3.0 `0/50B8B50` (45.253 s) | PRIMARY 3.0 `0/50B8B50` (45.253 s) | PRIMARY 3.0 `0/50BE860` (61.253 s) |

Role changes: n1 REPLICA → PRIMARY first seen 13.253 s (rule 2.1, WAL `0/50127D8`); n1 PRIMARY → REPLICA first seen 32.247 s (rule 2.2, WAL ``); n2 PRIMARY → REPLICA first seen 20.228 s (rule —, WAL ``); n3 REPLICA → PRIMARY first seen 32.257 s (rule 3.0, WAL `0/50765C0`); n3 PRIMARY → REPLICA first seen 52.721 s (rule 3.0, WAL `0/50BE210`); n3 REPLICA → PRIMARY first seen 57.252 s (rule 3.0, WAL `0/50BE210`).

Highest WAL position each node reported: n1 `0/5170568`; n2 `0/5170568`; n3 `0/5170568`.

## Unavailability windows per gateway (ADR-0036)

| Fault | Gateway | Window | Start | End | Length | Non-ok operations |
|---|---|---|---|---|---|---|
| f0 | gw-z1 | window | 10.466 s | 17.400 s | 6.934 s | 105 |
| f0 | gw-z2 | window | 10.467 s | 17.400 s | 6.933 s | 70 |
| f0 | gw-z3 | window | 10.467 s | 17.400 s | 6.933 s | 105 |
| f1 | gw-z1 | window | 30.292 s | 34.299 s | 4.008 s | 7 |
| f1 | gw-z2 | window | 30.291 s | 34.306 s | 4.015 s | 4 |
| f1 | gw-z3 | window | 30.292 s | 34.309 s | 4.018 s | 6 |
| f2 | gw-z1 | window | 45.974 s | 52.395 s | 6.421 s | 942 |
| f2 | gw-z2 | window | 45.975 s | 52.395 s | 6.420 s | 628 |
| f2 | gw-z3 | window | 45.974 s | 52.395 s | 6.421 s | 903 |

## Latency per phase (ms)

| Fault | Phase | Operations | p50 | p95 | p99 |
|---|---|---|---|---|---|
| f0 | before | 1614 | 5.133 | 14.249 | 1000.770 |
| f0 | during | 837 | 5.509 | 13.310 | 1000.745 |
| f0 | after | 1614 | 5.805 | 13.694 | 1000.614 |
| f1 | before | 1606 | 5.819 | 13.950 | 1000.617 |
| f1 | during | 410 | 4.615 | 11.706 | 1000.815 |
| f1 | after | 1598 | 4.767 | 12.002 | 1000.668 |
| f2 | before | 1610 | 5.156 | 12.228 | 1000.643 |
| f2 | during | 18 | 1000.326 | 1000.980 | 1000.980 |
| f2 | after | 69 | 5.704 | 1000.874 | 1001.059 |

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
| C8 | false | `notify-not-resumed` (3922) |
| C9 | true | — |
| P | true | — |

First anomalies of C8 (cpcheck lists at most 20 per class):

- `notify-not-resumed` (ops 10, 6213): NOTIFY 1890 (i=6213) was not received within 10s by listener client 26, whose session (LISTEN i=10) was alive and past its first successful op after recovery
- `notify-not-resumed` (ops 13, 6213): NOTIFY 1890 (i=6213) was not received within 10s by listener client 27, whose session (LISTEN i=13) was alive and past its first successful op after recovery
- `notify-not-resumed` (ops 12, 6213): NOTIFY 1890 (i=6213) was not received within 10s by listener client 28, whose session (LISTEN i=12) was alive and past its first successful op after recovery
