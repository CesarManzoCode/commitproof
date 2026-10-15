package gate

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/history/historytest"
	"github.com/CesarManzoCode/commitproof/internal/report"
)

const k6ms = int64(1_000_000)

// TestK6ExpectedByHand: cpgate's own computation on historytest.Failover
// gives ADR-0036's values worked out by hand (see the report package's
// TestFailoverMetrics for the arithmetic).
func TestK6ExpectedByHand(t *testing.T) {
	_, evs, obs := historytest.Failover()
	dir := t.TempDir()
	t.Cleanup(func() { removeAll(dir) })
	man, _, _ := historytest.Failover()
	run := filepath.Join(dir, historytest.FailoverRunID)
	if _, err := historytest.WriteRun(run, man, evs, obs); err != nil {
		t.Fatal(err)
	}
	rep := history.VerifySeal(run)
	if !rep.Valid {
		t.Fatal(rep.Problems)
	}
	got := newK6Run(rep.History, rep.Observations).expected()["f0"]
	want := map[string]any{"kind": "F2a", "inject": 10_000 * k6ms, "end": 13_000 * k6ms,
		"gateways": map[string]any{
			"gw-z1": map[string]any{"window": "window", "window_start": 10_200 * k6ms, "window_end": 14_050 * k6ms, "window_ns": 3_850 * k6ms, "gap_ns": 8_950 * k6ms, "non_ok": int64(1)},
			"gw-z2": map[string]any{"window": "window", "window_start": 12_400 * k6ms, "window_end": 13_540 * k6ms, "window_ns": 1_140 * k6ms, "gap_ns": 9_460 * k6ms, "non_ok": int64(1)},
		},
		"latency": map[string]any{
			"before": map[string]any{"n": int64(2), "p50": 10 * k6ms, "p95": 20 * k6ms, "p99": 20 * k6ms},
			"during": map[string]any{"n": int64(1), "p50": 30 * k6ms, "p95": 30 * k6ms, "p99": 30 * k6ms},
			"after":  map[string]any{"n": int64(2), "p50": 40 * k6ms, "p95": 50 * k6ms, "p99": 50 * k6ms},
		},
		"rule_changes": int64(2), "leader_changes": int64(1), "rules": []string{"2.0", "2.1"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

// k6Fixture builds an evidence directory holding the Failover run as the
// reference (archive and checks.json) and its rendered report.
func k6Fixture(t *testing.T) string {
	t.Helper()
	ev := t.TempDir()
	t.Cleanup(func() { removeAll(ev) })
	work := t.TempDir()
	t.Cleanup(func() { removeAll(work) })
	man, evs, obs := historytest.Failover()
	run := filepath.Join(work, historytest.FailoverRunID)
	root, err := historytest.WriteRun(run, man, evs, obs)
	if err != nil {
		t.Fatal(err)
	}
	checks := historytest.FailoverChecks(root)
	if err := os.WriteFile(run+report.ChecksSuffix, checks, 0o644); err != nil {
		t.Fatal(err)
	}
	refDir := filepath.Join(ev, k6RefDir)
	os.MkdirAll(refDir, 0o755)
	if err := ArchiveRun(run, filepath.Join(refDir, historytest.FailoverRunID+ArchiveSuffix)); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(refDir, historytest.FailoverRunID+".checks.json"), checks, 0o644)
	r, err := report.LoadRun(run)
	if err != nil {
		t.Fatal(err)
	}
	m, err := report.Build(r)
	if err != nil {
		t.Fatal(err)
	}
	page, err := report.RenderRun(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := report.WriteFile(filepath.Join(ev, k6ReportDir), historytest.FailoverRunID+".html", page); err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestK6CompletenessAndMetrics(t *testing.T) {
	ev := k6Fixture(t)
	e := LoadEvidence(ev)
	defer e.Close()
	if _, p, d := evalK63(e); len(p) > 0 {
		t.Errorf("K6.3 on a faithful render: %v (%v)", p, d)
	}
	if _, p, d := evalK65(e); len(p) > 0 {
		t.Errorf("K6.5 on a faithful render: %v (%v)", p, d)
	}
	page := filepath.Join(ev, k6ReportDir, historytest.FailoverRunID+".html")
	orig, _ := os.ReadFile(page)
	for _, c := range []struct {
		name     string
		edit     func([]byte) []byte
		k63, k65 bool // which criterion must report a problem
	}{
		{"completion index dropped", func(b []byte) []byte {
			return regexp.MustCompile(`data-ev="1 2"`).ReplaceAll(b, []byte(`data-ev="1"`))
		}, true, false},
		{"leader change dropped", func(b []byte) []byte {
			return bytes.ReplaceAll(b, []byte(`data-leader-change="2.0"`), []byte(`data-x="2.0"`))
		}, true, false},
		{"anomaly link retargeted", func(b []byte) []byte { return bytes.Replace(b, []byte(`href="#e11"`), []byte(`href="#e12"`), 1) }, true, false},
		{"gap altered", func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"gap_ns":8950000000`), []byte(`"gap_ns":8950000001`), 1)
		}, false, true},
		{"p99 altered", func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"p99":50000000`), []byte(`"p99":49000000`), 1)
		}, false, true},
	} {
		nb := c.edit(orig)
		if bytes.Equal(nb, orig) {
			t.Fatalf("%s: the edit changed nothing", c.name)
		}
		os.WriteFile(page, nb, 0o644)
		e := LoadEvidence(ev)
		_, p3, _ := evalK63(e)
		_, p5, _ := evalK65(e)
		e.Close()
		if (len(p3) > 0) != c.k63 || (len(p5) > 0) != c.k65 {
			t.Errorf("%s: K6.3 problems %v, K6.5 problems %v", c.name, p3, p5)
		}
	}
	os.WriteFile(page, orig, 0o644)
	if !strings.Contains(string(orig), `id="cp-data"`) {
		t.Error("no data block")
	}
}
