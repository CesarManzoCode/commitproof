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
