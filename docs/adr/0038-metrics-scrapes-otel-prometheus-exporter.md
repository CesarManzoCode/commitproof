# ADR-0038 — /metrics scrapes (P23) from the OpenTelemetry Prometheus exporter; the families recorded, the report's metric timelines and the gate's cross-check

Status: **Accepted (pre-K7, contract 1.2.1); the exporter and the corrected P23 text kept by the owner's decision 5 of 2026-10-15 (K6b; LIM-41 states the difference from K4/K5 runs, measured in evidence/K6b/exporter-ab.md).** Implements the owner's decision D-A of 2026-10-15 ("LIM-40: add the /metrics scraper to the observer (do not amend §5.4/P23)") and corrects a research defect in P23 that the decision could not have known. Written and committed before the scraper, the report change and the gate change.

## Owner decision (verbatim)

> D-A. LIM-40: add the /metrics scraper to the observer (do not amend §5.4/P23).

The task adds: record only the metric families the report draws, as integers, named in a new ADR before implementing; measure bytes per run-minute on a real smoke run; draw metric timelines when scrapes exist, a run without scrapes rendering byte-identically to the committed K6 example; cross-check the new values in cpgate's independent recomputation; a negative control for the metrics section.

## Context: what serves /metrics at e3cbdbb

P23 reads "1 s (`/metrics` on HTTP ports 15000, 16000, 17000)". At the pinned commit no component serves metrics there:

- Record `metrics-probe-2` (evidence/pre-K7, `scripts/prek7-metrics-probe.sh`, the committed baseline compose): on all nine components, `GET /metrics` on the HTTP port returns `200` with the component's HTML status page, byte-identical (same sha256) to `GET /cp-no-such-path` — the servenv mux's catch-all — and no Prometheus `# TYPE` line; nothing listens on 9464.
- Source: `go/common/servenv` registers `/live`, `/ready`, `/version`, `/config`, `/css/`, `/favicon.ico`, `/debug/pprof/*` and the status page; `go/tools/telemetry/telemetry.go` `initMetrics` builds the meter provider with one reader from `autoexport.NewMetricReader`, setting `OTEL_METRICS_EXPORTER=none` when unset, with the comment `TODO(dweitzman): Add an additional prometheus exporter that's always at /metrics for debugging`.
- The pinned autoexport (`go.opentelemetry.io/contrib/exporters/autoexport v0.69.0`, `metrics.go`) supports `OTEL_METRICS_EXPORTER=prometheus`: an exporter whose own HTTP server serves `/metrics` on `OTEL_EXPORTER_PROMETHEUS_HOST` (default `localhost`) : `OTEL_EXPORTER_PROMETHEUS_PORT` (default `9464`). Upstream's own end-to-end harness enables exactly that for multipooler and multigateway (`go/test/endtoend/shardsetup/setup.go`: `OTEL_METRICS_EXPORTER=prometheus`, `OTEL_EXPORTER_PROMETHEUS_PORT=<port>`). Upstream's Kubernetes demo (S03) instead pushes OTLP every 5 s to a collector (`k8s-observability.yaml`, ConfigMap `otel-config`), which `deploy/DERIVATION.md` removed as an allowed OpenTelemetry divergence.
- A development cluster with the exporter enabled served Prometheus text on 9464 from every multigateway, multipooler and multiorch: 70–120 KB per scrape (22–50 families, 217–388 series), i.e. about 1 MB per second for the ten components — far beyond P25 if stored raw.

So P23's parenthetical is a research defect: the HTTP ports serve no metrics at the pinned commit (CONTRACT §18: "a default value read wrongly … an outdated source"). The owner's decision keeps the requirement — scrape `/metrics` every second into `observations.jsonl` (§5.4) — and that is only possible with an exporter enabled.

## Decision

### Deployment

Every `multigateway`, `multipooler` and `multiorch` container gets the environment `OTEL_METRICS_EXPORTER=prometheus`, `OTEL_EXPORTER_PROMETHEUS_HOST=0.0.0.0`, `OTEL_EXPORTER_PROMETHEUS_PORT=9464` (`deploy/compose.yaml`, so every profile derived from it: baseline, buffered, and K7's slotless and replica-port). pgctld gets none (multipooler shares its network namespace; one exporter per namespace). Profile h1 (the historical commit's images and demo, ADR-0025) is unchanged and not scraped. This is a new allowed divergence of §5.1, recorded in `deploy/DERIVATION.md`: a pull exporter instead of S03's OTLP push to a collector. Traces and logs stay unexported (their exporters stay unset, as before).

### P23, corrected

P23's value (1 s) and §5.4 are unchanged. P23's parenthetical becomes: `/metrics` of the OpenTelemetry Prometheus exporter on port 9464 of every multigateway, multipooler and multiorch (the HTTP ports 15000, 16000, 17000 serve no `/metrics` at the pinned commit; ADR-0038). This corrects where the endpoint is, not what is required; the deviation from the decision's words "do not amend P23" is recorded in the readiness report.

### The scraper

- Targets: `gw-z1`…`gw-z3` (cpnet .31–.33), `pooler-n1`…`pooler-n3` (the `pg-nX` addresses .11–.13; `pooler-n4`, .14, from F5a's start, as its status poller), `orch-z1`…`orch-z3` (.21–.23); port 9464, path `/metrics`, by address (no DNS lookup).
- One goroutine per target, every P23 (1 s), from the start of the observation pollers (after the setup transaction) until the observers stop after the final reads, as P24's pollers. Each scrape is bounded by P23 (HTTP client timeout 1 s) and reads at most 4 MiB.
- One observation per scrape: `{"src":"metrics","target":<t>,"ok":true,"latency_ms":<ms>,"bytes":<body bytes>,"v":{<family>:<integer>…}}`, or `{"src":"metrics","target":<t>,"ok":false,"latency_ms":<ms>,"error":<≤ 256 bytes>}`. The raw exposition text is not stored (P25).
- Observations never decide a verdict (ADR-0016): checkers never read them; the report and the gate only display and cross-check them.

### The recorded families (the ones the report draws)

Each family is the sum, over the series of that name whose labels match the filter, of the sample values (finite values only), times the scale, rounded half away from zero to an integer. A counter family absent from a scrape is 0 (OpenTelemetry exposes a counter after its first increment); a gauge family absent is not recorded.

| Key | Series (filter) | Kind | Scale (unit) | Components |
|---|---|---|---|---|
| `gw_client_connections` | `mg_gateway_client_connections` | gauge | 1 | gateways |
| `gw_queries` | `mg_gateway_query_duration_seconds_count` | counter | 1 | gateways |
| `gw_query_errors` | `mg_gateway_query_duration_seconds_count` {`status` ≠ `ok`} | counter | 1 | gateways |
| `gw_buffered` | `multigateway_buffer_requests_buffered_total` | counter | 1 | gateways |
| `pl_up` | `mg_pooler_up` | gauge | 1 | poolers |
| `pl_serving_transitions` | `mg_pooler_serving_transitions_total` | counter | 1 | poolers |
| `pl_server_conns_active` | `mg_pooler_server_connections` {`state` = `active`} | gauge | 1 | poolers |
| `pl_client_waiting` | `mg_pooler_client_waiting_connections` | gauge | 1 | poolers |
| `pl_conn_open_errors` | `mg_pooler_server_conn_open_errors_total` | counter | 1 | poolers |
| `pl_queries` | `mg_pooler_query_duration_seconds_count` | counter | 1 | poolers |
| `pl_replication_lag_ms` | `mg_pooler_replication_lag_seconds` | gauge | 1000 (ms) | poolers |
| `or_recovery_actions` | `multiorch_recovery_action_duration_milliseconds_count` | counter | 1 | orchestrators |
| `or_recovery_errors` | `multiorch_recovery_errors_total` | counter | 1 | orchestrators |
| `or_detected_problems` | `multiorch_recovery_detected_problems` | gauge | 1 | orchestrators |
| `or_streams_connected` | `multiorch_recovery_stream_connected` | gauge | 1 | orchestrators |
| `cpu_ms` | `process_cpu_time_seconds_total` | counter | 1000 (ms) | all |

Every other family (Go runtime, histograms' buckets and sums, auth, TLS, pgBackRest, RPC, process memory) is not recorded.

### The report (amends ADR-0036's "Metrics from observations")

When a run holds `metrics` observations, the run page adds a section **Metrics** (and its link in the navigation): one chart per family, one line per target, on the timeline's time scale with the fault windows shaded — gauges as values, counters as the per-second increase between consecutive samples (a decrease is a reset: the increase is the new value) — and a table per target. The data block gains `metrics`:

`{<target>: {"scrapes": <n>, "failed": <n>, "families": {<key>: {"kind": "counter"|"gauge", "n": <samples>, "first", "last", "min", "max"[, "increase"]}}}}`

over the target's `ok` scrapes in observation order, `increase` (counters only) = Σ over consecutive samples of (vᵢ − vᵢ₋₁ if vᵢ ≥ vᵢ₋₁, else vᵢ). Keys not in the table above are summarized as gauges. A run without `metrics` observations renders exactly as before: no section, no link, no data-block key — the committed K6 example stays byte-identical (`make ci-report`).

### The gate's cross-check and the controls

- `cpgate`'s K6.5 recomputes the `metrics` data block from `observations.jsonl` with gate code written from this ADR (it imports no report code) and compares every integer exactly; a report without the block for a run with scrapes, or with it for a run without, fails. On the K6 reference run (no scrape) the check is vacuous, so K6's gate and its negative controls are unchanged.
- `internal/tools/metricscheck` runs the same gate function on any rendered run (report + sealed run), for evidence outside K6 (pre-K7: the smoke run with scrapes).
- Negative controls, recorded in evidence/pre-K7: (1) one integer of the rendered smoke report's `metrics` block altered in a copy → the cross-check fails; (2) checker-level: `cpreport` built with `cp_sabotage_report_metrics` (each target's first scrape skipped) renders the smoke run → the cross-check fails, and renders the K6 reference run byte-identically to the committed example (the sabotage reaches the metrics section only).

### Cost

Measured on a real 45 s smoke run (P04) before K7: bytes of `metrics` observation lines per run-minute, uncompressed and as stored (zstd -19), and the run archive's size; the K7 feasibility note uses the measurement.

## Alternatives rejected

- **Scrape the HTTP ports as P23 states**: they return an HTML page; nothing to record.
- **Amend §5.4/P23 to drop scrapes**: the owner's decision rejects it.
- **OTLP push to a receiver in the runner** (S03's mechanism, interval 1 s): closer to S03's transport, but P23 and §5.4 speak of `/metrics` scrapes, it needs an OTLP protobuf receiver in the runner, and upstream's own harness uses the Prometheus exporter for the same purpose.
- **Record whole scrapes, or every family**: ≈ 1 MB/s raw for ten components; P25 could not hold K7's cited runs.
- **Delta-encode samples**: smaller, but the report and the gate would reconstruct values; integers per scrape are simpler to verify.

## Consequences

- Contract 1.2.1 (PATCH: correction of P23's location, a new §5.1 divergence, and the clarification of §15 K6's metrics). `params.ContractVersion` = 1.2.1.
- K7 runs carry metric scrapes; K1–K6 runs do not (LIM-40 is resolved for K7, with this measurement). The exporter adds an HTTP server and metric aggregation inside every Multigres process (upstream's demo aggregates and exports every 5 s as well); K7's timing is therefore measured with it, K1–K6's without it (a new LIMITATIONS row).
- F1 partitions and F3b pauses make scrapes of their targets fail; such failures are observations.

## Sources

CONTRACT §5.1, §5.3, §5.4, §5.7 (P23, P24, P25), §15 K6, §18; ADR-0001, ADR-0016, ADR-0025, ADR-0036; LIM-40; record `metrics-probe-2` (evidence/pre-K7/commands.jsonl); Multigres e3cbdbb `go/tools/telemetry/telemetry.go` (`initMetrics`), `go/common/servenv/{http,pages}.go`, `go/test/endtoend/shardsetup/setup.go`, `go/observability/metriccatalog/catalog_generated.go`, `demo/k8s/k8s-observability.yaml` (S03); `go.opentelemetry.io/contrib/exporters/autoexport@v0.69.0/metrics.go`.
