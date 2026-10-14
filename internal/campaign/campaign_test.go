package campaign

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/plan"
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

// The clients of an anomaly map to the plan's clients: K5's own client
// numbers (listeners, notifiers, the consumer) and the storm's connections
// (to the storm client); the runner's clients are not plan clients.
func TestMapInvolved(t *testing.T) {
	i := func(n int64) *int64 { return &n }
	evs := []history.Event{
		{I: 1, Client: i(26)}, {I: 2, Client: i(23)}, {I: 3, Client: i(10)}, {I: 4, Client: i(157)},
		{I: 5, Client: i(29)}, {I: 6, Client: i(14)}, {I: 7, Client: i(8)},
	}
	clients := []plan.Client{{Client: 8}, {Client: 23}, {Client: 26}, {Client: 29, Mode: plan.ModeConsumer},
		{Client: 100, Mode: plan.ModeStorm, Connections: 200}}
	anoms := []Anomaly{{Class: "notify-not-resumed", Ops: []int64{1, 2, 3}}, {Class: "x", Ops: []int64{4, 5, 6, 7}}}
	if got := mapInvolved(evs, anoms, "notify-not-resumed", clients); !reflect.DeepEqual(got, []int{23, 26}) {
		t.Errorf("notify-not-resumed clients %v, want [23 26]", got)
	}
	if got := mapInvolved(evs, anoms, "x", clients); !reflect.DeepEqual(got, []int{8, 29, 100}) {
		t.Errorf("x clients %v, want [8 29 100]", got)
	}
}
