# Does ADR-0038's metrics exporter change anything observable? One measured pair (owner decision 5)

A measurement, not evidence for any K7 cell and not a conclusion (ADR-0041). K7's runs have the OpenTelemetry Prometheus exporter enabled on every multigateway, multipooler and multiorch (ADR-0038); K4's and K5's runs — among them the directed cells K7 cites (ADR-0039) — did not. LIM-41 states the difference and this measurement.

## What was run

Records `exporter-ab-on`, `exporter-ab-off`, `exporter-ab-compare` (`evidence/K6b/commands.jsonl`; `scripts/k6b-exporter-ab.sh`, `scripts/k6b-exporter-compare.sh`). The CI smoke plan (`smoke`, seed 42: W1–W4 with P02's ten clients, 45 s, no fault; contract 1.6.0, identical plan bytes `e2ed36db…`) run twice back to back on the same host, images and binaries (commit `ecef6d3`):

- **on**, as K7 runs it — run `20261016T025924Z-smoke-42-real-e2ed36db`;
- **off**, `env CP_OTEL_METRICS_EXPORTER=none` (the K1–K6 configuration: the pinned code sets `none` when the variable is unset, ADR-0038) — run `20261016T030055Z-smoke-42-real-e2ed36db`.

During each run a sampler read every second, per container, its cgroup's cumulative CPU time (`cpu.stat` `usage_usec`) and peak memory (`memory.peak`), and once its `OTEL_METRICS_EXPORTER` (`docker inspect`). Samples: `exporter-ab-not-evidence/samples-{on,off}.jsonl.zst`; the runs' summaries (stub, `checks.json`): `exporter-ab-not-evidence/`.

## What the runs show (system did)

| | on | off |
|---|---|---|
| Exporter setting of the 9 Multigres containers | `prometheus` | `none` |
| Scrapes of `/metrics` (P23) | 540, all ok | 540, all failed (nothing on 9464) |
| Verdict (cpcheck) | PASS | PASS |
| Run span (first to last command of the run) | 89 s | 89 s |
| Leader (run-start HEALTHY) | n2 | n1 |
| Full §13 archive | 1 074 734 B | 1 046 979 B |

Operations (completions / ok) and latency of ok operations (p50 / p95 / p99, ms; nearest rank):

| Workload | on | off | on latency | off latency |
|---|---|---|---|---|
| append (W1) | 3 153 / 2 508 | 3 086 / 2 495 | 7.27 / 14.22 / 20.36 | 7.27 / 12.67 / 18.57 |
| bank (W3) | 1 554 / 1 367 | 1 406 / 1 209 | 6.01 / 13.68 / 19.76 | 6.02 / 12.74 / 38.67 |
| ledger (W4) | 1 783 / 1 783 | 1 785 / 1 785 | 5.94 / 7.89 / 11.43 | 5.72 / 8.06 / 9.23 |
| register (W2) | 1 781 / 1 781 | 1 782 / 1 782 | 3.94 / 7.55 / 10.43 | 4.47 / 8.18 / 10.85 |

CPU time over the containers' lives (cluster up to the last sample before teardown, ms) and summed peak memory (MiB):

| Component | CPU on | CPU off | on vs off | Peak memory on | off |
|---|---|---|---|---|---|
| gateways (3) | 10 856 | 9 237 | +17.5 % | 113 | 91 |
| orchestrators (3) | 908 | 551 | +64.8 % | 84 | 60 |
| poolers (3) | 12 187 | 11 027 | +10.5 % | 309 | 311 |
| — the leader's pooler | 9 256 | 8 463 | +9.4 % | | |
| PostgreSQL nodes (3) | 50 307 | 49 372 | +1.9 % | 541 | 547 |
| runner (scrapes in "on") | 3 364 | 2 099 | +60.3 % | 46 | 37 |

## Reading it (measurement, not conclusion)

- Measured: with the exporter on, the gateways, orchestrators and poolers used more CPU (+10 % to +65 % of small totals: +1.6 s, +0.36 s and +1.2 s over ~89 s) and gateways and orchestrators about 7–8 MiB more peak memory each; PostgreSQL's CPU and memory and every workload's completions and median latencies are within the differences one would also see between two runs (bank completions differ by 10 % in the exporter's favour, bank p99 by a factor of 2 in its favour).
- Not measured: run-to-run variance (one pair), any effect under faults (no fault in the plan: failover detection and recovery timing are not compared), the slotless and buffered profiles, a loaded host. The two runs had different leaders, so per-node figures compare different roles (the role-matched rows above use each run's leader).
- So nothing here shows a difference in what the checkers judge, and nothing here shows that timing under faults is unchanged. K7 states the difference wherever it compares its cells with the cited K4/K5 cells (LIM-41).

Sources: ADR-0038, ADR-0041; P23; records above.
