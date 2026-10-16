package checker

import "testing"

// ADR-0037: the judged G12 rule applies to plans of contract 1.2.0 or
// later; every other plan keeps the literal rule.
func TestG12Judged(t *testing.T) {
	for v, want := range map[string]bool{
		"": false, "1.0.9": false, "1.1.0": false, "1.1.1": false, "1.1.2": false,
		"1.2.0": true, "1.2.1": true, "1.10.0": true, "2.0.0": true,
		"1.2": false, "1.2.0.1": false, "1.x.0": false, "-1.3.0": false,
	} {
		if got := g12Judged(map[string]any{"contract_version": v}); got != want {
			t.Errorf("g12Judged(%q) = %t, want %t", v, got, want)
		}
	}
	if g12Judged(nil) {
		t.Error("g12Judged(nil plan) = true, want false")
	}
}

func TestCoveredNS(t *testing.T) {
	fw := func(s, e int64) faultWindow { return faultWindow{Start: s, End: e} }
	for _, c := range []struct {
		name       string
		start, end int64
		faults     []faultWindow
		want       int64
	}{
		{"none", 0, 100, nil, 0},
		{"outside", 0, 100, []faultWindow{fw(200, 300)}, 0},
		{"inside", 0, 100, []faultWindow{fw(10, 30)}, 20},
		{"clipped", 0, 100, []faultWindow{fw(-50, 10), fw(90, 500)}, 20},
		{"overlapping", 0, 100, []faultWindow{fw(10, 40), fw(30, 60), fw(70, 80)}, 60},
		{"nested", 0, 100, []faultWindow{fw(10, 90), fw(20, 30)}, 80},
		{"whole", 0, 100, []faultWindow{fw(-10, 110)}, 100},
	} {
		if got := coveredNS(c.start, c.end, c.faults); got != c.want {
			t.Errorf("%s: coveredNS = %d, want %d", c.name, got, c.want)
		}
	}
}
