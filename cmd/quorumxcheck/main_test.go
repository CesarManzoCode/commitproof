package main

import "testing"

// The table is complete and agrees with upstream; for AT_LEAST_2 the two
// recruitment terms coincide exactly for cohorts of 3 and 4 (CONTRACT I3).
func TestTable(t *testing.T) {
	rows, mm, bc, co, err := Table()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 330 || len(mm) != 0 || bc != 0 {
		t.Fatalf("%d rows, %d mismatches, %d (b)<=>(c) violations", len(rows), len(mm), bc)
	}
	var at2 []int64
	for _, x := range co {
		m := x.(map[string]any)
		if m["N"].(int64) == 2 {
			at2 = append(at2, m["n"].(int64))
		}
	}
	if len(at2) != 2 || at2[0] != 3 || at2[1] != 4 {
		t.Fatalf("AT_LEAST_2 coincidences %v, want [3 4]", at2)
	}
}
