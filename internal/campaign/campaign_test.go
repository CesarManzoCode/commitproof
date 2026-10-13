package campaign

import (
	"fmt"
	"reflect"
	"testing"
)

// ddmin finds a 1-minimal failing subset; the oracle here fails iff the
// subset contains both 1 and 3.
func TestDDMin(t *testing.T) {
	seen := map[string]bool{} // a repeated candidate reuses its sealed run
	test := func(s []int) (bool, error) {
		seen[fmt.Sprint(s)] = true
		has := map[int]bool{}
		for _, x := range s {
			has[x] = true
		}
		return has[1] && has[3], nil
	}
	got, err := ddmin([]int{0, 1, 2, 3, 4}, test)
	if err != nil || !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("ddmin = %v %v", got, err)
	}
	if len(seen) > 20 {
		t.Errorf("%d distinct candidates for 5 steps", len(seen))
	}
	// A single essential step is found directly.
	got, _ = ddmin([]int{0, 1, 2, 3}, func(s []int) (bool, error) {
		for _, x := range s {
			if x == 2 {
				return true, nil
			}
		}
		return false, nil
	})
	if !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("single = %v", got)
	}
}

func TestReproducibility(t *testing.T) {
	for hits, want := range map[int]string{0: "NOT_REPRODUCED", 1: "INTERMITTENT", 2: "INTERMITTENT", 3: "REPRODUCED", 5: "REPRODUCED"} {
		if got := Reproducibility(hits); got != want {
			t.Errorf("%d: %s", hits, got)
		}
	}
}

// ADR-0024 and ADR-0030's minimization targets.
func TestTargetClass(t *testing.T) {
	for _, c := range []struct {
		campaign string
		classes  []string
		want     string
	}{
		{"D1", []string{"incompatible-order", "lost-write"}, "lost-write"},
		{"D3", []string{"g1a", "duplicate-elements"}, "duplicate-elements"},
		{"D3", []string{"lost-write", "g1a"}, "g1a"},
		{"D3", []string{"lost-write"}, "lost-write"},
		{"D5", []string{"cdc-gap", "cdc-phantom"}, "cdc-phantom"},
		{"C10", []string{"incompatible-order", "cdc-gap"}, "incompatible-order"},
	} {
		r := &Run{Manifest: map[string]any{"campaign": c.campaign}, Classes: c.classes}
		if got := TargetClass(r); got != c.want {
			t.Errorf("TargetClass(%s %v) = %q, want %q", c.campaign, c.classes, got, c.want)
		}
	}
}

// The committed matrices decode and their plans generate (§15 K4, K5).
func TestMatricesRead(t *testing.T) {
	for _, f := range []string{"k4.json", "k4-d2p.json", "k5.json"} {
		m, err := ReadMatrix("../../campaigns/" + f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if !m.MinimizeFailures || m.Replays != 5 {
			t.Errorf("%s: minimize %v replays %d", f, m.MinimizeFailures, m.Replays)
		}
	}
}
