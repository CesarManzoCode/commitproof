package report

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/checker/output"
	"github.com/CesarManzoCode/commitproof/internal/history/historytest"
)

func writeFailover(t *testing.T) (dir string) {
	t.Helper()
	base := t.TempDir()
	t.Cleanup(func() { removeTree(base) }) // sealed directories are 0555
	dir = filepath.Join(base, historytest.FailoverRunID)
	man, evs, obs := historytest.Failover()
	root, err := historytest.WriteRun(dir, man, evs, obs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+ChecksSuffix, historytest.FailoverChecks(root), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const ms = int64(1_000_000)

// TestFailoverMetrics: ADR-0036's definitions applied by hand to the
// Failover scenario (historytest), never by running this package:
//
//	f0 F2a: I = 10 000 ms (inject), E = 13 000 ms (healed).
//	gw-z1: S = 10 200 (fail of op 6); R = first ok completion ≥ max(S, E) =
//	  14 050 (op 17) → window 3 850 ms. Gap over [9 000, 23 000] between
//	  9 000, 11 030, 14 050, 23 000 → 8 950 ms; non-ok in it: 1 (op 6).
//	gw-z2: S = 12 400 (info of op 10); R = 13 540 (op 15) → 1 140 ms. Gap
//	  between 9 000, 13 540, 23 000 → 9 460 ms; non-ok 1.
//	Latency of ok ops by invoke time: before [0, 10 000): 10, 20 ms →
//	  p50 rank ⌈1⌉ = 10, p95 rank ⌈1.9⌉ = 20, p99 20; during [10 000,
//	  13 000]: 30 ms only; after (13 000, 23 000]: 40, 50 → p50 40, p95
//	  50, p99 50. The final read (30 000) is excluded.
//	Rules: 1.0 n1 (500), 2.0 n2 (10 500), 2.1 n2 (20 000); first seen in
//	  [I, ∞): 2.0 and 2.1 → 2 rule changes, 1 leader change (2.0).
func TestFailoverMetrics(t *testing.T) {
	r, err := LoadRun(writeFailover(t))
	if err != nil {
		t.Fatal(err)
	}
	m, err := Build(r)
	if err != nil {
		t.Fatal(err)
	}
	met := m.Metrics()
	if len(met) != 1 {
		t.Fatalf("%d faults", len(met))
	}
	f := met[0]
	if f.Fault.I != 10_000*ms || f.Fault.E != 13_000*ms {
		t.Errorf("I, E = %d, %d", f.Fault.I, f.Fault.E)
	}
	want := []GatewayMetric{
		{GW: "gw-z1", State: WindowClosed, S: 10_200 * ms, R: 14_050 * ms, Gap: 8_950 * ms, NonOK: 1},
		{GW: "gw-z2", State: WindowClosed, S: 12_400 * ms, R: 13_540 * ms, Gap: 9_460 * ms, NonOK: 1},
	}
	if !reflect.DeepEqual(f.Gateways, want) {
		t.Errorf("gateways\n got %+v\nwant %+v", f.Gateways, want)
	}
	if got, w := f.Before, (Phase{N: 2, P50: 10 * ms, P95: 20 * ms, P99: 20 * ms}); got != w {
		t.Errorf("before %+v, want %+v", got, w)
	}
	if got, w := f.During, (Phase{N: 1, P50: 30 * ms, P95: 30 * ms, P99: 30 * ms}); got != w {
		t.Errorf("during %+v, want %+v", got, w)
	}
	if got, w := f.After, (Phase{N: 2, P50: 40 * ms, P95: 50 * ms, P99: 50 * ms}); got != w {
		t.Errorf("after %+v, want %+v", got, w)
	}
	var keys []string
	for _, ru := range f.Rules {
		keys = append(keys, ru.Key())
	}
	if !reflect.DeepEqual(keys, []string{"2.0", "2.1"}) || f.LeaderChanges != 1 {
		t.Errorf("rules %v, leader changes %d", keys, f.LeaderChanges)
	}
	if len(m.Rules) != 3 || !m.Rules[1].LeaderChange() || m.Rules[2].LeaderChange() || m.Rules[0].LeaderChange() {
		t.Errorf("rules %+v", m.Rules)
	}
}

// TestPhaseNearestRank: ⌈q·n/100⌉ on 1..100 is q itself; on 1..3 the
// ranks of 50, 95, 99 are 2, 3, 3.
func TestPhaseNearestRank(t *testing.T) {
	var ls []int64
	for i := int64(100); i >= 1; i-- {
		ls = append(ls, i)
	}
	if p := phase(ls); p != (Phase{N: 100, P50: 50, P95: 95, P99: 99}) {
		t.Errorf("1..100: %+v", p)
	}
	if p := phase([]int64{3, 1, 2}); p != (Phase{N: 3, P50: 2, P95: 3, P99: 3}) {
		t.Errorf("1..3: %+v", p)
	}
	if p := phase(nil); p != (Phase{}) {
		t.Errorf("empty: %+v", p)
	}
}

// TestRenderDeterministicAndComplete: two renders are byte-identical; every
// history index is in exactly one element; anomaly ops link to the
// operation's invoke (12 is the completion of the op invoked at 11).
func TestRenderDeterministicAndComplete(t *testing.T) {
	dir := writeFailover(t)
	render := func() []byte {
		r, err := LoadRun(dir)
		if err != nil {
			t.Fatal(err)
		}
		m, err := Build(r)
		if err != nil {
			t.Fatal(err)
		}
		b, err := RenderRun(m)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	a, b := render(), render()
	if !bytes.Equal(a, b) {
		t.Fatal("two renders differ")
	}
	seen := map[string]int{}
	for _, x := range regexp.MustCompile(`data-ev="([0-9 ]+)"`).FindAllSubmatch(a, -1) {
		for _, f := range bytes.Fields(x[1]) {
			seen[string(f)]++
		}
	}
	if len(seen) != 22 {
		t.Errorf("%d distinct indices in elements, want 22", len(seen))
	}
	for k, n := range seen {
		if n != 1 {
			t.Errorf("index %s in %d elements", k, n)
		}
	}
	if !bytes.Contains(a, []byte(`data-checker="C1" data-class="lost-write" data-ops="1 12">`)) ||
		!bytes.Contains(a, []byte(`href="#e1"`)) || !bytes.Contains(a, []byte(`href="#e11"`)) {
		t.Error("anomaly element or its links missing")
	}
	if !bytes.Contains(a, []byte(`data-leader-change="2.0" data-leader="n2"`)) || bytes.Contains(a, []byte(`data-leader-change="2.1"`)) {
		t.Error("leader changes")
	}
	// No path of the rendering and no external reference (the SVG namespace
	// URI is a name, not a fetch).
	for _, bad := range [][]byte{[]byte(dir), []byte(" src="), []byte(`href="http`), []byte("url("), []byte("@import"), []byte("<link")} {
		if bytes.Contains(a, bad) {
			t.Errorf("the page contains %q", bad)
		}
	}
}

// TestFailClosed: a flipped byte in history.jsonl, a checks file for
// another seal root, a missing checks file and a stub are refused.
func TestFailClosed(t *testing.T) {
	dir := writeFailover(t)
	p := filepath.Join(dir, "history.jsonl")
	b, _ := os.ReadFile(p)
	os.Chmod(p, 0o644)
	b[len(b)/2] ^= 1
	os.WriteFile(p, b, 0o444)
	if _, err := LoadRun(dir); err == nil {
		t.Error("a run whose seal does not verify loaded")
	}
	dir2 := writeFailover(t)
	os.WriteFile(dir2+ChecksSuffix, historytest.FailoverChecks("0000"), 0o644)
	if _, err := LoadRun(dir2); err == nil {
		t.Error("a checks file of another seal root was accepted")
	}
	os.Remove(dir2 + ChecksSuffix)
	if _, err := LoadRun(dir2); err == nil {
		t.Error("a run without checks file loaded")
	}
	if _, err := LoadRun(filepath.Join(t.TempDir(), "x"+StubSuffix)); err == nil {
		t.Error("a stub loaded")
	}
}

// ADR-0037: a cell whose runs carry C8 storm windows reports both G12
// fractions side by side; cells without them render nothing.
func TestRenderG12(t *testing.T) {
	var none bytes.Buffer
	if got := renderG12(&none, []*Cell{{Campaign: "C2"}}); len(got) != 0 || none.Len() != 0 {
		t.Fatalf("a cell without storm windows rendered %q", none.String())
	}
	cl := &Cell{Campaign: "C11", Profile: "baseline", Mutant: "real",
		G12: G12Fractions{Windows: 4, LiteralTotal: 3516, LiteralOK: 3396, LiteralBelow: 1, JudgedTotal: 3396, JudgedOK: 3396, NotJudged: 0}}
	var b bytes.Buffer
	got := renderG12(&b, []*Cell{{Campaign: "C2"}, cl})
	if _, ok := got[1]; !ok || len(got) != 1 {
		t.Fatalf("data for cells %v, want cell 1 only", got)
	}
	for _, want := range []string{"<td class=\"n\">3396/3516</td><td class=\"n\">1</td><td class=\"n\">3396/3396</td><td class=\"n\">0</td><td class=\"n\">0</td>", "ADR-0037"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("rendered G12 table lacks %q:\n%s", want, b.String())
		}
	}
	if got[1]["judged_ok"] != int64(3396) || got[1]["literal_below_90"] != int64(1) {
		t.Errorf("data %v", got[1])
	}
}

// ADR-0038: the metrics data block, with expectations written by hand
// (the same as the gate's TestMetricsExpected).
func TestMetricsData(t *testing.T) {
	m := &Model{Scrapes: map[string][]MetricObs{
		"gw-z1": {
			{T: 2, OK: true, V: map[string]int64{"gw_queries": 10, "gw_client_connections": 2, "cpu_ms": 100}},
			{T: 3},
			{T: 4, OK: true, V: map[string]int64{"gw_queries": 15, "gw_client_connections": 5, "cpu_ms": 150}},
			{T: 5, OK: true, V: map[string]int64{"gw_queries": 3, "gw_client_connections": 1, "cpu_ms": 20}},
		},
		"pooler-n1": {{T: 6, OK: true, V: map[string]int64{"pl_up": 1, "not_in_table": 7}}},
	}}
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
	if got := m.metricsData(); !reflect.DeepEqual(got, want) && !sabotageMetrics {
		t.Fatalf("metricsData:\n got %v\nwant %v", got, want)
	}
	if got := m.metricKeys(); !reflect.DeepEqual(got, []string{"gw_client_connections", "gw_queries", "pl_up", "cpu_ms", "not_in_table"}) {
		t.Errorf("metricKeys %v", got)
	}
}

// ADR-0044: a stub whose checks.json carries C9's exposure contributes it to
// its cell, and a cell whose runs all do gets the §12.3 clean statement; a
// stub without it (plans before 1.7.0) still counts only by its verdict.
// Expected numbers written by hand from the two checks files below.
func TestChecksExposure(t *testing.T) {
	cj := func(id string, exposure string) []byte {
		c9 := `{"anomalies":[],"id":"C9","stats":{"faults":2},"valid":true}`
		if exposure != "" {
			c9 = `{"anomalies":[],"id":"C9","stats":{"exposure":` + exposure + `,"faults":2},"valid":true}`
		}
		return []byte(`{"checkers":[{"anomalies":[],"id":"C1","stats":{},"valid":true},` + c9 + `],"run_id":"` + id + `","seal_root":"r","verdict":"PASS"}`)
	}
	man := func(campaign string, seed string) map[string]any {
		return map[string]any{"campaign": campaign, "profile": "baseline", "mutant": "real", "seed": seed, "multigres_commit": "e3cbdbbb0b94"}
	}
	var runs []*CampaignRun
	for i, e := range []string{
		`{"confirmed_faults":{"F1a":1,"F5a":1},"fault_time_ns":10000000000,"ok_writes":100,"operations":150}`,
		`{"confirmed_faults":{"F2a":1,"F5a":1},"fault_time_ns":2500000000,"ok_writes":200,"operations":250}`,
	} {
		id := []string{"a", "b"}[i]
		checks, err := outputDecode(cj(id, e))
		if err != nil {
			t.Fatal(err)
		}
		r := summarize(id, "r", man("C12", id), checks)
		r.Stub = true
		runs = append(runs, r)
	}
	old, err := outputDecode(cj("c", ""))
	if err != nil {
		t.Fatal(err)
	}
	oldRun := summarize("c", "r", man("C2", "1"), old)
	oldRun.Stub = true
	runs = append(runs, oldRun)
	if runs[0].ChecksExposure == nil || oldRun.ChecksExposure != nil {
		t.Fatalf("exposure parsed %v / %v", runs[0].ChecksExposure, oldRun.ChecksExposure)
	}
	cells := (&Campaign{Runs: runs}).Cells()
	if len(cells) != 2 {
		t.Fatalf("%d cells", len(cells))
	}
	c12 := cells[0] // sorted by name: C12 < C2
	if c12.Campaign != "C12" || c12.ChecksRuns != 2 || c12.FullRuns != 0 || c12.Ops != 400 || c12.OKWrites != 300 ||
		c12.FaultTime != 12_500_000_000 || !reflect.DeepEqual(c12.Confirmed, map[string]int64{"F1a": 1, "F2a": 1, "F5a": 2}) {
		t.Fatalf("C12 cell %+v", c12)
	}
	if c2 := cells[1]; c2.Campaign != "C2" || c2.ChecksRuns != 0 || c2.Ops != 0 {
		t.Fatalf("C2 cell %+v", c2)
	}
	page, err := RenderCampaign(&Campaign{Runs: runs})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"2 of 2 runs (0 full, 2 from checks.json)",
		"<li>In 2 valid runs of campaign C12 (profile baseline, Multigres e3cbdbb), exposing 300 ok writes to 4 confirmed faults of kinds {F1a, F2a, F5a}",
		"0 of 1 runs",
		"ADR-0044",
	} {
		if !strings.Contains(string(page), want) {
			t.Errorf("campaign page lacks %q", want)
		}
	}
	if strings.Contains(string(page), "<li>In 1 valid runs of campaign C2") {
		t.Error("a cell without exposure got a clean statement")
	}
}

func outputDecode(b []byte) (*output.Checks, error) { return output.Decode(b) }
