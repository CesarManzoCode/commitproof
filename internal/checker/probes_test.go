package checker

// Probe checker (G7). Expected results are written from CONTRACT §3 G7 and
// §15 K4 (P1 → 0A000, P2 → rejected), not by running the checker.

import (
	"reflect"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

func probeOp(i int64, id, typ, sqlstate string) *Op {
	o := &Op{Index: i, WL: "probe", F: "probe", GW: "gw-z1", Type: typ, Done: true,
		InValue: map[string]any{"id": id, "sql": "x"}}
	if sqlstate != "" || typ != history.TypeOK {
		o.Err = map[string]any{"sqlstate": sqlstate}
	}
	return o
}

func TestProbeChecker(t *testing.T) {
	plan := map[string]any{"campaign": "claims"}
	cases := []struct {
		name    string
		ops     []*Op
		valid   Validity
		classes []string
	}{
		{"real build: both rejected", []*Op{probeOp(1, "P1", history.TypeFail, "0A000"), probeOp(3, "P1-show", history.TypeOK, ""),
			probeOp(5, "P2", history.TypeFail, "0A000")}, True, nil},
		{"P2 rejected with any SQLSTATE", []*Op{probeOp(1, "P1", history.TypeFail, "0A000"), probeOp(5, "P2", history.TypeInfo, "XX000")}, True, nil},
		{"guard removed: P1 accepted", []*Op{probeOp(1, "P1", history.TypeOK, ""), probeOp(5, "P2", history.TypeFail, "0A000")}, False, []string{"p1-not-rejected"}},
		{"P1 wrong SQLSTATE", []*Op{probeOp(1, "P1", history.TypeFail, "42501"), probeOp(5, "P2", history.TypeFail, "0A000")}, False, []string{"p1-wrong-sqlstate"}},
		{"ALTER SYSTEM accepted", []*Op{probeOp(1, "P1", history.TypeFail, "0A000"), probeOp(5, "P2", history.TypeOK, "")}, False, []string{"p2-not-rejected"}},
		{"P1 outcome unknown", []*Op{probeOp(1, "P1", history.TypeInfo, ""), probeOp(5, "P2", history.TypeFail, "0A000")}, Unknown, nil},
		{"P2 missing", []*Op{probeOp(1, "P1", history.TypeFail, "0A000")}, Unknown, nil},
		{"violation beats unknown", []*Op{probeOp(1, "P1", history.TypeOK, ""), probeOp(5, "P2", history.TypeInfo, "")}, False, []string{"p1-not-rejected"}},
	}
	for _, c := range cases {
		r := CheckProbes(&History{Ops: c.ops, Plan: plan})
		if r.ID != ProbeID || r.Valid != c.valid || !reflect.DeepEqual(Classes(r.Anomalies), c.classes) {
			t.Errorf("%s: %s %v, want %s %v", c.name, r.Valid, Classes(r.Anomalies), c.valid, c.classes)
		}
	}
	// A run that is not the probe suite and has no probes is vacuously valid.
	if r := CheckProbes(&History{Plan: map[string]any{"campaign": "C1"}}); r.Valid != True {
		t.Errorf("no probes: %s", r.Valid)
	}
}
