package gate

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

func metricsObs(t int64, target string, ok bool, v map[string]int64) history.Event {
	m := map[string]any{"src": "metrics", "target": target, "ok": ok}
	if ok {
		vv := map[string]any{}
		for k, x := range v {
			vv[k] = json.Number(itoa(x))
		}
		m["v"] = vv
	}
	return history.Event{T: t, Type: history.TypeObs, Value: m}
}

func itoa(x int64) string { b, _ := json.Marshal(x); return string(b) }

// Expectations written from ADR-0038's definitions by hand.
func TestMetricsExpected(t *testing.T) {
	obs := []history.Event{
		{T: 1, Type: history.TypeObs, Value: map[string]any{"src": "pooler-status", "node": "n1"}},
		metricsObs(2, "gw-z1", true, map[string]int64{"gw_queries": 10, "gw_client_connections": 2, "cpu_ms": 100}),
		metricsObs(3, "gw-z1", false, nil),
		metricsObs(4, "gw-z1", true, map[string]int64{"gw_queries": 15, "gw_client_connections": 5, "cpu_ms": 150}),
		metricsObs(5, "gw-z1", true, map[string]int64{"gw_queries": 3, "gw_client_connections": 1, "cpu_ms": 20}),
		metricsObs(6, "pooler-n1", true, map[string]int64{"pl_up": 1, "not_in_table": 7}),
	}
	want := map[string]any{
		"gw-z1": map[string]any{"scrapes": int64(4), "failed": int64(1), "families": map[string]any{
			"gw_queries":            map[string]any{"kind": "counter", "n": int64(3), "first": int64(10), "last": int64(3), "min": int64(3), "max": int64(15), "increase": int64(8)},
			"gw_client_connections": map[string]any{"kind": "gauge", "n": int64(3), "first": int64(2), "last": int64(1), "min": int64(1), "max": int64(5)},
			"cpu_ms":                map[string]any{"kind": "counter", "n": int64(3), "first": int64(100), "last": int64(20), "min": int64(20), "max": int64(150), "increase": int64(70)},
		}},
		"pooler-n1": map[string]any{"scrapes": int64(1), "failed": int64(0), "families": map[string]any{
			"pl_up":        map[string]any{"kind": "gauge", "n": int64(1), "first": int64(1), "last": int64(1), "min": int64(1), "max": int64(1)},
			"not_in_table": map[string]any{"kind": "gauge", "n": int64(1), "first": int64(7), "last": int64(7), "min": int64(7), "max": int64(7)},
		}},
	}
	got := MetricsExpected(obs)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MetricsExpected:\n got %v\nwant %v", got, want)
	}
	if MetricsExpected(obs[:1]) != nil {
		t.Error("a run without metrics observations has a metrics block")
	}
	// CheckMetrics: presence must match, values must be equal.
	enc := func(v any) map[string]any {
		b, _ := json.Marshal(v)
		var m map[string]any
		d := json.NewDecoder(bytesReader(b))
		d.UseNumber()
		d.Decode(&m)
		return m
	}
	if p, _ := CheckMetrics(enc(map[string]any{"metrics": want}), obs); len(p) != 0 {
		t.Errorf("equal blocks: %v", p)
	}
	bad := enc(map[string]any{"metrics": want})
	bad["metrics"].(map[string]any)["gw-z1"].(map[string]any)["families"].(map[string]any)["gw_queries"].(map[string]any)["increase"] = json.Number("12")
	if p, _ := CheckMetrics(bad, obs); len(p) != 1 {
		t.Errorf("an altered increase: %v", p)
	}
	if p, _ := CheckMetrics(map[string]any{}, obs); len(p) != 1 {
		t.Errorf("a missing block: %v", p)
	}
	if p, _ := CheckMetrics(enc(map[string]any{"metrics": want}), obs[:1]); len(p) != 1 {
		t.Errorf("a block for a run without scrapes: %v", p)
	}
	if p, note := CheckMetrics(map[string]any{}, obs[:1]); len(p) != 0 || note == "" {
		t.Errorf("no scrapes, no block: %v %q", p, note)
	}
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
