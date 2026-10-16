package report

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

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
