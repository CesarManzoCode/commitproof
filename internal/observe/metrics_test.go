package observe

import (
	"reflect"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

// A trimmed exposition in the format of the pinned exporter (ADR-0038).
const exposition = `# HELP mg_pooler_server_connections Server connections.
# TYPE mg_pooler_server_connections gauge
mg_pooler_server_connections{otel_scope_name="x",routing_role="primary",state="active"} 2
mg_pooler_server_connections{otel_scope_name="x",routing_role="primary",state="idle"} 5
mg_pooler_server_connections{otel_scope_name="x",routing_role="replica",state="active"} 1
mg_pooler_up{otel_scope_name="x",routing_role="primary"} 1
mg_pooler_replication_lag_seconds{otel_scope_name="x"} 0.0125
mg_pooler_server_conn_open_errors_total{pool_type="admin",reason="error"} 198
mg_pooler_server_conn_open_errors_total{pool_type="regular",reason="a \"quoted\", {braced} value\\n"} 2
mg_pooler_query_duration_seconds_count{pool_type="regular",status="ok"} 16
mg_pooler_query_duration_seconds_bucket{pool_type="regular",status="ok",le="+Inf"} 16
process_cpu_time_seconds_total{cpu_mode="system"} 0.14
process_cpu_time_seconds_total{cpu_mode="user"} 0.18
process_memory_usage_bytes 5.6823808e+07
mg_gateway_client_connections{endpoint="primary"} 3
mg_gateway_query_duration_seconds_count{status="ok"} 4
mg_gateway_query_duration_seconds_count{status="error"} 2
mg_gateway_query_duration_seconds_count{status="error"} NaN
`

func TestReduce(t *testing.T) {
	got, err := Reduce(exposition, KindPooler)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"pl_up": int64(1), "pl_server_conns_active": int64(3), "pl_replication_lag_ms": int64(13),
		"pl_conn_open_errors": int64(200), "pl_queries": int64(16), "pl_serving_transitions": int64(0), "cpu_ms": int64(320),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pooler: got %v, want %v", got, want)
	}
	got, err = Reduce(exposition, KindGateway)
	if err != nil {
		t.Fatal(err)
	}
	want = map[string]any{"gw_client_connections": int64(3), "gw_queries": int64(6), "gw_query_errors": int64(2),
		"gw_buffered": int64(0), "cpu_ms": int64(320)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("gateway: got %v, want %v", got, want)
	}
	got, err = Reduce(exposition, KindOrchestrator)
	if err != nil {
		t.Fatal(err)
	}
	want = map[string]any{"or_recovery_actions": int64(0), "or_recovery_errors": int64(0), "cpu_ms": int64(320)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("orchestrator: got %v, want %v", got, want)
	}
}

// The HTTP ports of the pinned commit answer /metrics with an HTML page
// (record metrics-probe-2): never an exposition.
func TestReduceRejectsHTML(t *testing.T) {
	for _, body := range []string{"<!doctype html>\n<html lang=\"en\">\n", "", "# HELP only comments\n"} {
		if _, err := Reduce(body, KindGateway); err == nil {
			t.Errorf("Reduce(%q) accepted a body without samples", body)
		}
	}
}

func TestParseSample(t *testing.T) {
	name, labels, v, err := parseSample(`m{a="x\"y",b="{,}"} -1.5e3 1700000000`)
	if err != nil || name != "m" || labels["a"] != `x"y` || labels["b"] != "{,}" || v != -1500 {
		t.Errorf("parseSample: %q %v %v %v", name, labels, v, err)
	}
	for _, bad := range []string{`m{a="x} 1`, `m{a=x} 1`, `{a="x"} 1`, `m{a="x"}`, `m 1 2 3`, `m abc`} {
		if _, _, _, err := parseSample(bad); err == nil {
			t.Errorf("parseSample(%q) accepted", bad)
		}
	}
}

func TestMetricsTargets(t *testing.T) {
	ts := MetricsTargets("10.77.0", []string{"n1", "n4"})
	got := map[string]string{}
	for _, x := range ts {
		got[x.Name] = x.Kind + " " + x.Addr
	}
	want := map[string]string{
		"gw-z1": "gateway 10.77.0.31:9464", "gw-z2": "gateway 10.77.0.32:9464", "gw-z3": "gateway 10.77.0.33:9464",
		"pooler-n1": "pooler 10.77.0.11:9464", "pooler-n4": "pooler 10.77.0.14:9464",
		"orch-z1": "orchestrator 10.77.0.21:9464", "orch-z2": "orchestrator 10.77.0.22:9464", "orch-z3": "orchestrator 10.77.0.23:9464",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("targets %v, want %v", got, want)
	}
}

// The observer records exactly params.MetricFamilies' keys, with its kinds,
// in ADR-0038's order.
func TestFamiliesMatchParams(t *testing.T) {
	if len(Families) != len(params.MetricFamilies) {
		t.Fatalf("%d families, params has %d", len(Families), len(params.MetricFamilies))
	}
	for i, f := range Families {
		if p := params.MetricFamilies[i]; f.Key != p.Key || f.Counter != p.Counter {
			t.Errorf("family %d: %s counter=%t, params %s counter=%t", i, f.Key, f.Counter, p.Key, p.Counter)
		}
	}
}
