package gate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// The report's metrics data block (CONTRACT P23, §15 K6.5; ADR-0038),
// recomputed from observations.jsonl with gate code written from
// ADR-0038's definitions — cpgate imports no report code.

// MetricsExpected returns the data block's "metrics" object for the run's
// observations, or nil when the run holds no metrics observation: per
// target, its scrapes and failed scrapes, and per family key present in
// its ok scrapes (observation order): kind (counter if ADR-0038's table
// says so, else gauge), n, first, last, min, max and, for counters,
// increase = Σ (vᵢ − vᵢ₋₁ if vᵢ ≥ vᵢ₋₁, else vᵢ).
func MetricsExpected(obs []history.Event) map[string]any {
	counter := map[string]bool{}
	for _, f := range params.MetricFamilies {
		counter[f.Key] = f.Counter
	}
	scrapes, failed := map[string]int64{}, map[string]int64{}
	series := map[string]map[string][]int64{} // target → key → values
	for _, ev := range obs {
		v, _ := ev.Value.(map[string]any)
		if v["src"] != "metrics" {
			continue
		}
		t, _ := v["target"].(string)
		scrapes[t]++
		if series[t] == nil {
			series[t] = map[string][]int64{}
		}
		if v["ok"] != true {
			failed[t]++
			continue
		}
		vals, _ := v["v"].(map[string]any)
		for k, x := range vals {
			if n, ok := k6Num(x); ok {
				series[t][k] = append(series[t][k], n)
			}
		}
	}
	if len(scrapes) == 0 {
		return nil
	}
	out := map[string]any{}
	for t, n := range scrapes {
		fams := map[string]any{}
		for k, vs := range series[t] {
			lo, hi := vs[0], vs[0]
			var inc int64
			for i, x := range vs {
				lo, hi = min(lo, x), max(hi, x)
				if i > 0 {
					if x >= vs[i-1] {
						inc += x - vs[i-1]
					} else {
						inc += x
					}
				}
			}
			f := map[string]any{"kind": "gauge", "n": int64(len(vs)), "first": vs[0], "last": vs[len(vs)-1], "min": lo, "max": hi}
			if counter[k] {
				f["kind"], f["increase"] = "counter", inc
			}
			fams[k] = f
		}
		out[t] = map[string]any{"scrapes": n, "failed": failed[t], "families": fams}
	}
	return out
}

// CheckMetrics compares a report's data block with MetricsExpected: the
// "metrics" key exists exactly when the run holds metrics observations,
// and every integer equals the recomputation. It returns the problems and
// a summary line.
func CheckMetrics(data map[string]any, obs []history.Event) ([]string, string) {
	want := MetricsExpected(obs)
	got, has := data["metrics"]
	switch {
	case want == nil && !has:
		return nil, "metrics: no /metrics scrape in the run, none in the report"
	case want == nil:
		return []string{"the report has a metrics block; the run holds no metrics observation"}, ""
	case !has:
		return []string{"the run holds metrics observations; the report has no metrics block"}, ""
	}
	p := k6Diff("metrics", want, got)
	fams := 0
	targets := make([]string, 0, len(want))
	for t, x := range want {
		targets = append(targets, t)
		fams += len(x.(map[string]any)["families"].(map[string]any))
	}
	sort.Strings(targets)
	return p, fmt.Sprintf("metrics: %d targets %v, %d target families compared (scrapes, failed; n, first, last, min, max, increase)", len(targets), targets, fams)
}

// ReportDataBlock decodes a rendered report's cp-data block (numbers as
// json.Number).
func ReportDataBlock(page []byte) (map[string]any, error) {
	m := k6DataRe.FindSubmatch(page)
	if m == nil {
		return nil, fmt.Errorf("no cp-data block in the report")
	}
	var data map[string]any
	dec := json.NewDecoder(bytes.NewReader(m[1]))
	dec.UseNumber()
	if err := dec.Decode(&data); err != nil {
		return nil, fmt.Errorf("cp-data: %w", err)
	}
	return data, nil
}
