package stats

import "testing"

// Expected values are written from the definitions (CONTRACT §12.3), with
// the decimals computed independently in floating point (python3: the
// Wilson formula with z = 1.96, and 1 − 0.05**(1/n)), then rounded down
// (lower ends) or up (upper ends) — never by running this package.
func TestWilson95(t *testing.T) {
	for _, c := range []struct {
		k, n   int64
		lo, hi string
	}{
		{0, 1, "0.000", "0.794"},  // [0, 0.793457]
		{0, 3, "0.000", "0.562"},  // [0, 0.561506]
		{1, 3, "0.061", "0.793"},  // [0.061490, 0.792345]
		{3, 3, "0.438", "1.000"},  // [0.438494, 1]
		{2, 5, "0.117", "0.770"},  // [0.117618, 0.769280]
		{4, 5, "0.375", "0.964"},  // [0.375528, 0.963777]
		{5, 5, "0.565", "1.000"},  // [0.565509, 1]
		{0, 10, "0.000", "0.278"}, // [0, 0.277540]
		{1, 10, "0.017", "0.405"}, // [0.017876, 0.404156]
	} {
		iv, err := Wilson95(c.k, c.n)
		if err != nil {
			t.Fatal(err)
		}
		if Down(iv.Lo) != c.lo || Up(iv.Hi) != c.hi {
			t.Errorf("Wilson95(%d, %d) = [%s, %s], want [%s, %s]", c.k, c.n, Down(iv.Lo), Up(iv.Hi), c.lo, c.hi)
		}
	}
	for _, bad := range [][2]int64{{0, 0}, {-1, 3}, {4, 3}} {
		if _, err := Wilson95(bad[0], bad[1]); err == nil {
			t.Errorf("Wilson95(%d, %d) did not fail", bad[0], bad[1])
		}
	}
}

func TestZeroFailureBound95(t *testing.T) {
	for _, c := range []struct {
		n    int64
		want string
	}{
		{1, "0.950"},  // 0.95 exactly
		{3, "0.632"},  // 0.631597
		{5, "0.451"},  // 0.450720
		{10, "0.259"}, // 0.258866; CONTRACT §12.3: "for N = 10: 0.259"
		{20, "0.140"}, // 0.139108
	} {
		b, err := ZeroFailureBound95(c.n)
		if err != nil {
			t.Fatal(err)
		}
		if Up(b) != c.want {
			t.Errorf("ZeroFailureBound95(%d) = %s, want %s", c.n, Up(b), c.want)
		}
	}
	if _, err := ZeroFailureBound95(0); err == nil {
		t.Error("ZeroFailureBound95(0) did not fail")
	}
}

func TestRounding(t *testing.T) {
	if Down(Fixed(Scale)) != "1.000" || Up(Fixed(Scale)) != "1.000" || Down(0) != "0.000" || Up(1) != "0.001" || Down(Fixed(Scale/1000-1)) != "0.000" {
		t.Error("rounding at the ends")
	}
}
