package gate

import (
	"strings"
	"testing"
)

// §11.2 severity, derived from the class, the claim level and (for G7) the
// guarantee; written from the table, not from the code.
func TestK4Severity(t *testing.T) {
	for _, c := range []struct{ class, level, g, want string }{
		{"SAFETY_VIOLATION", "EXPLICIT", "G1", "critical"},
		{"SAFETY_VIOLATION", "INFERRED", "G5", "high"},
		{"SAFETY_VIOLATION", "DESIGN", "G6", "high"},
		{"PROTOCOL_DEVIATION", "DESIGN", "G7", "high"},
		{"PROTOCOL_DEVIATION", "DESIGN", "G8", "medium"},
		{"LIVENESS_OBSERVATION", "EXPLICIT", "G1", "info"},
	} {
		if got := k4Severity(c.class, c.level, c.g); got != c.want {
			t.Errorf("%v: %s", c, got)
		}
	}
}

func TestK4FindingIDAndRows(t *testing.T) {
	id := k4FindingID("C2", "SAFETY_VIOLATION", "G1", "lost-write", strings.Repeat("a", 64))
	if !strings.HasPrefix(id, "FND-C2-") || len(id) != len("FND-C2-")+8 {
		t.Fatalf("finding id %s", id)
	}
	if id == k4FindingID("C2", "SAFETY_VIOLATION", "G1", "lost-write", strings.Repeat("b", 64)) {
		t.Fatal("finding id ignores the minimized plan")
	}
	// §6.2: ok is row 1; fail rows 2–7 and 10; info rows 8, 9, 11, 12.
	for row := 1; row <= 12; row++ {
		want := map[bool]string{true: "ok"}[row == 1]
		if row >= 2 && row <= 7 || row == 10 {
			want = "fail"
		}
		if row == 8 || row == 9 || row == 11 || row == 12 {
			want = "info"
		}
		for _, typ := range []string{"ok", "fail", "info"} {
			if k4RowConsistent(typ, row) != (typ == want) {
				t.Errorf("row %d %s", row, typ)
			}
		}
	}
}

func TestCheckPatch(t *testing.T) {
	good := "Intent: x\nFailure mode modelled: y\nSources: S09.\n\ndiff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1 +1 @@\n-a\n+b\n"
	if p := checkPatch("M9", []byte(good)); len(p) != 0 {
		t.Fatalf("good patch: %v", p)
	}
	two := good + "diff --git a/g.go b/g.go\n@@ -1 +1 @@\n-a\n+b\n"
	if p := checkPatch("M9", []byte(two)); len(p) == 0 {
		t.Fatal("two-file patch accepted")
	}
	if p := checkPatch("M9", []byte(strings.Replace(good, "Sources:", "Refs:", 1))); len(p) == 0 {
		t.Fatal("header without sources accepted")
	}
}

// ADR-0027: M2 is judged on D2′ with D2's class and checker; K4 runs of
// contract 1.0.6 stay evidence except as D2′ runs, which D2′ postdates.
func TestK4D2Prime(t *testing.T) {
	if d := k4Directed["M2"]; d.Campaign != "D2p" || d.Class != "lost-write" || d.Checker != "C1" {
		t.Errorf("M2 directed %+v", d)
	}
	for _, c := range []struct {
		campaign, version string
		want              bool
	}{
		{"D2p", "1.0.7", true}, {"D2p", "1.0.6", false},
		{"D2", "1.0.6", true}, {"C1", "1.0.6", true}, {"D1", "1.0.7", true},
		{"C1", "1.0.5", false}, {"D2", "", false}, {"D1", "1.0.9", false},
	} {
		if got := k4ContractOK(c.campaign, c.version); got != c.want {
			t.Errorf("k4ContractOK(%s, %s) = %v", c.campaign, c.version, got)
		}
	}
}

// ADR-0033: M3 is judged on D3′ with §9.2's two classes; K5 runs of
// contract 1.0.9 stay evidence except as D3′ runs, which D3′ postdates;
// buffering stays confined to C8, D3′ and the superseded D3.
func TestK5D3Prime(t *testing.T) {
	d := k5Directed["M3"]
	if d.Campaign != "D3p" || d.Profile != "buffered" || len(d.Expected) != 2 ||
		d.Expected[0] != [2]string{"C3", "duplicate-elements"} || d.Expected[1] != [2]string{"C2", "g1a"} {
		t.Errorf("M3 directed %+v", d)
	}
	for _, c := range []struct {
		campaign, version string
		want              bool
	}{
		{"D3p", "1.1.0", true}, {"D3p", "1.0.9", false}, {"D3p", "1.0.10", false},
		{"D3", "1.0.9", true}, {"C10", "1.0.9", true}, {"D5", "1.1.0", true},
		{"C9", "1.0.8", false}, {"D3", "", false}, {"C8", "1.0.10", false},
	} {
		if got := k5ContractOK(c.campaign, c.version); got != c.want {
			t.Errorf("k5ContractOK(%s, %s) = %v", c.campaign, c.version, got)
		}
	}
	for c, want := range map[string]bool{"C8": true, "D3": true, "D3p": true, "C9": false, "C10": false, "C11": false, "D5": false} {
		if k5Buffered[c] != want {
			t.Errorf("k5Buffered[%s] = %v", c, k5Buffered[c])
		}
	}
	if k4TargetClass(&k4Run{Manifest: map[string]any{"campaign": "D3p"}, Classes: []string{"lost-write", "g1a"}}) != "g1a" {
		t.Error("D3′ target class")
	}
}
