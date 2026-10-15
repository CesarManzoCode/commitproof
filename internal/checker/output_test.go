package checker

import (
	"reflect"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/checker/output"
)

// TestOutputDecodesChecksJSON ties the reader types of package output (used
// by cpreport, which must not link the checkers) to the encoder cpcheck
// writes: what ChecksJSON encodes, output.Decode reads back.
func TestOutputDecodesChecksJSON(t *testing.T) {
	rs := []Result{
		{ID: "C1", Valid: True, Stats: map[string]any{"ok_writes_checked": int64(7)}},
		{ID: "C3", Valid: False, Anomalies: []Anomaly{{Class: "incompatible-order", Ops: []int64{3, 9}, Explanation: "x"}},
			Observations: []Anomaly{{Class: "G-single-realtime", Ops: []int64{4}, Explanation: "y"}}, Stats: map[string]any{}},
		{ID: "C4", Valid: Unknown, Stats: map[string]any{}},
	}
	b, err := ChecksJSON("run-1", "abc", rs, VerdictFail)
	if err != nil {
		t.Fatal(err)
	}
	c, err := output.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if c.RunID != "run-1" || c.SealRoot != "abc" || c.Verdict != VerdictFail || len(c.Checkers) != 3 {
		t.Fatalf("decoded %+v", c)
	}
	if got := []string{c.Checkers[0].Valid, c.Checkers[1].Valid, c.Checkers[2].Valid}; !reflect.DeepEqual(got, []string{output.ValidTrue, output.ValidFalse, output.ValidUnknown}) {
		t.Errorf("validities %v", got)
	}
	if a := c.Checkers[1].Anomalies; len(a) != 1 || a[0].Class != "incompatible-order" || !reflect.DeepEqual(a[0].Ops, []int64{3, 9}) || a[0].Explanation != "x" {
		t.Errorf("anomalies %+v", a)
	}
	if o := c.Checkers[1].Observations; len(o) != 1 || o[0].Class != "G-single-realtime" {
		t.Errorf("observations %+v", o)
	}
	if _, ok := c.Checkers[0].Stats["ok_writes_checked"]; !ok {
		t.Errorf("stats %v", c.Checkers[0].Stats)
	}
	if _, err := output.Decode([]byte(`{"run_id":"r","seal_root":"s","verdict":"PASS","checkers":[{"id":"C1","valid":"maybe","anomalies":[],"stats":{}}]}`)); err == nil {
		t.Error("a validity other than true, false, \"unknown\" decoded")
	}
}
