# Report excerpt — run `20261016T221153Z-C2-1-real-c8781715`

From cpreport's page of the original run: `bin/cpreport render evidence/K7/runs/20261016T221153Z-C2-1-real-c8781715.tar.zst --out <dir>` (record `render-pkg-FND-C2-8215101c` in `evidence/K7/commands.jsonl`) wrote `20261016T221153Z-C2-1-real-c8781715.html` (1492483 bytes, sha256 `452be955fdd69d8c0f6b3c0b80b07fce97f3bbd194ddf77a17abfc34c67849eb`); the page is byte-reproducible (K6.1) and is not committed. Seal root `e0783b2feba2e17571e389340cc9cace5672c3bef7e0f8782bfb1750b0efe5a8`; cpcheck's verdict **FAIL**; 13846 history lines, 6908 operations. Faults, rules and leader changes are observations (ADR-0016): they never decide a verdict.

## Faults

| Fault | Kind | Targets | Injected | Confirmed | Heal | Healed | Rule changes | Leader changes |
|---|---|---|---|---|---|---|---|---|
| f0 | F1b | n1, orch-z2, etcd, gw-z1, gw-z2, gw-z3, n2, n3, orch-z1, orch-z3, runner | 10.459 s | 11.795 s | 41.796 s | 42.226 s | 1 (2.0) | 1 |
| f1 | F1b | n1, orch-z2, etcd, gw-z1, gw-z2, gw-z3, n2, n3, orch-z1, orch-z3, runner | 52.456 s | 53.790 s | 63.791 s | 64.218 s | 0 | 0 |
| f2 | F1d | n2 | 77.595 s | 78.734 s | 108.734 s | 108.942 s | 2 (3.0, 4.0) | 1 |

## Decided rules and leader changes (observations)

| Rule | Leader | Cohort | First seen |
|---|---|---|---|
| 1.0 | n1 | n1, n2, n3 | 0.176 s |
| 2.0 | n2 | n1, n2, n3 | 35.255 s |
| 3.0 | n1 | n1, n2, n3 | 98.260 s |
| 4.0 | n1 | n1, n2, n3 | 102.268 s |

Leader changes: n1 → n2 at rule 2.0, first seen 35.255 s; n2 → n1 at rule 3.0, first seen 98.260 s.

## WAL positions and roles (pooler-status observations, 1 s polling)

Each cell: the node's last successful pooler-status sample at or before the instant (role, decided rule it reports, WAL position). Observations only (ADR-0016); the runner reaches every node's pooler except those a fault isolates from it (LIM-28).

Fault f0 (F1b on n1, orch-z2, etcd, gw-z1, gw-z2, gw-z3, n2, n3, orch-z1, orch-z3, runner):

| Node | before inject (10.459 s) | confirmed (11.795 s) | heal (41.796 s) | healed + 10 s (52.226 s) |
|---|---|---|---|---|
| n1 | PRIMARY 1.0 `0/40A7248` (10.260 s) | PRIMARY 1.0 `0/40A7248` (10.260 s) | PRIMARY 1.0 `0/40A7248` (10.260 s) | PRIMARY 1.0 `0/40A7248` (10.260 s) |
| n2 | REPLICA 1.0 `0/40A7248` (10.260 s) | REPLICA 1.0 `0/40AA300` (11.254 s) | PRIMARY 2.0 `0/40D3530` (41.259 s) | PRIMARY 2.0 `0/4138B18` (51.258 s) |
| n3 | REPLICA 1.0 `0/40A7248` (10.260 s) | REPLICA 1.0 `0/40AA300` (11.254 s) | REPLICA 2.0 `0/40D3530` (41.257 s) | REPLICA 2.0 `0/4138B18` (51.258 s) |

Fault f1 (F1b on n1, orch-z2, etcd, gw-z1, gw-z2, gw-z3, n2, n3, orch-z1, orch-z3, runner):

| Node | before inject (52.456 s) | confirmed (53.790 s) | heal (63.791 s) | healed + 10 s (74.218 s) |
|---|---|---|---|---|
| n1 | REPLICA 2.0 `0/4141FF0` (52.260 s) | REPLICA 2.0 `0/4141FF0` (52.260 s) | REPLICA 2.0 `0/4141FF0` (52.260 s) | REPLICA 2.0 `0/42019A8` (73.262 s) |
| n2 | PRIMARY 2.0 `0/4141FF0` (52.261 s) | PRIMARY 2.0 `0/414C7C0` (53.258 s) | PRIMARY 2.0 `0/41A1B38` (63.259 s) | PRIMARY 2.0 `0/42019A8` (73.262 s) |
| n3 | REPLICA 2.0 `0/4141FF0` (52.260 s) | REPLICA 2.0 `0/414C7C0` (53.258 s) | REPLICA 2.0 `0/41A1B38` (63.259 s) | REPLICA 2.0 `0/42019A8` (73.262 s) |

Fault f2 (F1d on n2):

| Node | before inject (77.595 s) | confirmed (78.734 s) | heal (108.734 s) | healed + 10 s (118.942 s) |
|---|---|---|---|---|
| n1 | REPLICA 2.0 `0/4222B80` (77.261 s) | REPLICA 2.0 `0/4225FD8` (78.257 s) | PRIMARY 4.0 `0/5004808` (108.258 s) | PRIMARY 4.0 `0/500A1B0` (118.257 s) |
| n2 | PRIMARY 2.0 `0/4222B80` (77.261 s) | PRIMARY 2.0 `0/4226F60` (78.258 s) | PRIMARY 2.0 `` (108.253 s) | REPLICA 4.0 `0/500A1B0` (118.256 s) |
| n3 | REPLICA 2.0 `0/4222B80` (77.262 s) | REPLICA 2.0 `0/4225FD8` (78.257 s) | REPLICA 4.0 `0/5004808` (108.258 s) | REPLICA 4.0 `0/500A1B0` (118.256 s) |

Role changes: n1 PRIMARY → REPLICA first seen 52.260 s (rule 2.0, WAL `0/4141FF0`); n1 REPLICA → PRIMARY first seen 98.261 s (rule 3.0, WAL `0/422B268`); n1 PRIMARY → REPLICA first seen 102.270 s (rule 4.0, WAL `0/50036F0`); n1 REPLICA → PRIMARY first seen 103.258 s (rule 4.0, WAL `0/50044C8`); n2 REPLICA → PRIMARY first seen 35.255 s (rule 2.0, WAL `0/40B26F0`); n2 PRIMARY → REPLICA first seen 112.268 s (rule 2.0, WAL `0/50000A0`).

Highest WAL position each node reported: n1 `0/511B3D0`; n2 `0/511B3D0`; n3 `0/511B3D0`.

## Unavailability windows per gateway (ADR-0036)

| Fault | Gateway | Window | Start | End | Length | Non-ok operations |
|---|---|---|---|---|---|---|
| f0 | gw-z1 | window | 10.556 s | 42.231 s | 31.675 s | 102 |
| f0 | gw-z2 | window | 10.503 s | 42.227 s | 31.724 s | 52 |
| f0 | gw-z3 | window | 20.586 s | 42.260 s | 21.674 s | 38 |
| f1 | gw-z1 | window | 52.550 s | 64.236 s | 11.686 s | 143 |
| f1 | gw-z2 | window | 52.496 s | 64.239 s | 11.743 s | 76 |
| f1 | gw-z3 | window | 52.590 s | 64.243 s | 11.653 s | 81 |
| f2 | gw-z1 | window | 77.606 s | 108.958 s | 31.352 s | 17 |
| f2 | gw-z2 | window | 87.645 s | 108.958 s | 21.313 s | 6 |
| f2 | gw-z3 | window | 77.610 s | 108.956 s | 31.347 s | 12 |

## Latency per phase (ms)

| Fault | Phase | Operations | p50 | p95 | p99 |
|---|---|---|---|---|---|
| f0 | before | 1384 | 5.679 | 11.194 | 14.768 |
| f0 | during | 262 | 7.341 | 19.483 | 32.226 |
| f0 | after | 1307 | 6.289 | 12.157 | 15.538 |
| f1 | before | 1305 | 6.302 | 12.030 | 15.538 |
| f1 | during | 1492 | 5.938 | 10.500 | 16.580 |
| f1 | after | 1307 | 4.576 | 8.358 | 10.981 |
| f2 | before | 1253 | 4.480 | 8.302 | 10.634 |
| f2 | during | 11 | 1.174 | 8.188 | 8.188 |
| f2 | after | 3 | 16.040 | 16.132 | 16.132 |

## Checkers (cpcheck, `checks.json`)

| Checker | Valid | Anomaly classes (count) |
|---|---|---|
| C1 | true | — |
| C2 | true | — |
| C3 | true | — |
| C4 | false | `non-linearizable` (1) |
| C5 | true | — |
| C6 | true | — |
| C7 | true | — |
| C8 | true | — |
| C9 | true | — |
| P | true | — |

First anomalies of C4 (cpcheck lists at most 20 per class):

- `non-linearizable` (ops 13777): register 4: no linearization; the earliest offending operation is final-read i=13777 ({f:read v:0 old:0 new_:0} -> {known:true set:true v:1127 applied:false})
