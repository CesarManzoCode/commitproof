# Report excerpt — run `20261013T232750Z-C11-2-real-1f5423ed`

From cpreport's page of the original run: `bin/cpreport render evidence/K5/runs/20261013T232750Z-C11-2-real-1f5423ed.tar.zst --out <dir>` (record `render-pkg-FND-C11-17a344c1` in `evidence/K7/commands.jsonl`) wrote `20261013T232750Z-C11-2-real-1f5423ed.html` (5427853 bytes, sha256 `b5abdf6c04f834960f457356540937c84a2c2515260415a124a659a33f23033d`); the page is byte-reproducible (K6.1) and is not committed. Seal root `535406abf5f1a525c220924fbfd1c0597003f49eef2fd10560af668b1c932353`; cpcheck's verdict **FAIL**; 37122 history lines, 18551 operations. Faults, rules and leader changes are observations (ADR-0016): they never decide a verdict.

## Faults

| Fault | Kind | Targets | Injected | Confirmed | Heal | Healed | Rule changes | Leader changes |
|---|---|---|---|---|---|---|---|---|
| f0 | F2a | n1 | 10.500 s | 10.594 s | 10.594 s | 14.214 s | 1 (2.0) | 1 |

## Decided rules and leader changes (observations)

| Rule | Leader | Cohort | First seen |
|---|---|---|---|
| 1.0 | n1 | n1, n2, n3 | 0.178 s |
| 2.0 | n2 | n1, n2, n3 | 14.263 s |

Leader changes: n1 → n2 at rule 2.0, first seen 14.263 s.

## WAL positions and roles (pooler-status observations, 1 s polling)

Each cell: the node's last successful pooler-status sample at or before the instant (role, decided rule it reports, WAL position). Observations only (ADR-0016); the runner reaches every node's pooler except those a fault isolates from it (LIM-28).

Fault f0 (F2a on n1):

| Node | before inject (10.500 s) | confirmed (10.594 s) | heal (10.594 s) | healed + 10 s (24.214 s) |
|---|---|---|---|---|
| n1 | PRIMARY 1.0 `0/4053A78` (10.265 s) | PRIMARY 1.0 `0/4053A78` (10.265 s) | PRIMARY 1.0 `0/4053A78` (10.265 s) | REPLICA 2.0 `0/407BEF0` (23.265 s) |
| n2 | REPLICA 1.0 `0/4053A78` (10.265 s) | REPLICA 1.0 `0/4053A78` (10.265 s) | REPLICA 1.0 `0/4053A78` (10.265 s) | PRIMARY 2.0 `0/407BEF0` (23.266 s) |
| n3 | REPLICA 1.0 `0/4053A78` (10.265 s) | REPLICA 1.0 `0/4053A78` (10.265 s) | REPLICA 1.0 `0/4053A78` (10.265 s) | REPLICA 2.0 `0/407BEF0` (23.265 s) |

Role changes: n1 PRIMARY → REPLICA first seen 13.304 s (rule 1.0, WAL ``); n2 REPLICA → PRIMARY first seen 14.264 s (rule 2.0, WAL `0/4069AF0`).

Highest WAL position each node reported: n1 `0/412BE70`; n2 `0/412BE70`; n3 `0/412BE70`.

## Unavailability windows per gateway (ADR-0036)

| Fault | Gateway | Window | Start | End | Length | Non-ok operations |
|---|---|---|---|---|---|---|
| f0 | gw-z1 | window | 10.553 s | 14.236 s | 3.684 s | 4400 |
| f0 | gw-z2 | window | 10.645 s | 14.676 s | 4.031 s | 3880 |
| f0 | gw-z3 | window | 10.552 s | 14.236 s | 3.684 s | 3945 |

## Latency per phase (ms)

| Fault | Phase | Operations | p50 | p95 | p99 |
|---|---|---|---|---|---|
| f0 | before | 591 | 5.431 | 4705.032 | 5039.048 |
| f0 | during | 61 | 1012.546 | 2019.609 | 2020.602 |
| f0 | after | 1141 | 2503.199 | 4076.997 | 4547.407 |

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
| C8 | false | `fair-share-starvation` (1) |
| C9 | true | — |
| P | true | — |

First anomalies of C8 (cpcheck lists at most 20 per class):

- `fair-share-starvation` (ops ): storm phase 1: 766 of 886 cp_steady operations completed ok (< 90%)
