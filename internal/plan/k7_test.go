package plan

import (
	"bytes"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

func TestC12DeterministicAndRoundTrip(t *testing.T) {
	for seed := uint64(0); seed <= 15; seed++ {
		a, b := mkplan(t, "C12", seed, "baseline"), mkplan(t, "C12", seed, "baseline")
		ab, _ := a.Canonical()
		bb, _ := b.Canonical()
		if !bytes.Equal(ab, bb) {
			t.Fatalf("seed %d: same inputs, different plans", seed)
		}
		d, err := Decode(ab)
		if err != nil {
			t.Fatalf("seed %d: decode: %v", seed, err)
		}
		if db, _ := d.Canonical(); !bytes.Equal(db, ab) {
			t.Fatalf("seed %d: decode round trip differs", seed)
		}
	}
}

// ADR-0041: seeds 1–7, 8–14, … each draw the 14 kinds of §15 K7 exactly
// once, two per run.
func TestC12KindsWithoutReplacementPerBlock(t *testing.T) {
	want := append([]string(nil), C12Kinds...)
	sort.Strings(want)
	if len(want) != 14 || strings.Join(want, ",") != "F1a,F1b,F1c,F1d,F2a,F2b,F2c,F2d,F2e,F3a,F3b,F4a,F6,F7" {
		t.Fatalf("C12 kinds %v, want §15 K7's {F1a–F1d, F2a–F2e, F3a, F3b, F4a, F6, F7}", want)
	}
	for first := uint64(1); first <= 64; first += 7 {
		var got []string
		for s := first; s < first+7; s++ {
			k := C12KindsForSeed(s)
			if len(k) != 2 || k[0] == k[1] {
				t.Fatalf("seed %d: kinds %v", s, k)
			}
			got = append(got, k...)
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("seeds %d–%d draw %v, want each kind once", first, first+6, got)
		}
	}
	// Different blocks draw different permutations (not one fixed order).
	if reflect.DeepEqual(C12KindsForSeed(1), C12KindsForSeed(8)) && reflect.DeepEqual(C12KindsForSeed(2), C12KindsForSeed(9)) &&
		reflect.DeepEqual(C12KindsForSeed(3), C12KindsForSeed(10)) {
		t.Fatal("blocks 0 and 1 draw the same kinds for their first three seeds")
	}
}

// ADR-0041's table: F5a(n4) 5 s, two drawn faults with their campaign's
// targets and holds, F5b(n4) 5 s; W1–W4 with P02's clients.
func TestC12Shape(t *testing.T) {
	ms := func(d time.Duration) int64 { return d.Milliseconds() }
	targets := map[string][]string{
		"F1a": {"leader", "random-follower"}, "F1c": {"leader"}, "F1d": {"leader"},
		"F2a": {"leader", "random-node"}, "F2b": {"leader", "random-node"}, "F2c": {"random-orch"}, "F2d": {"random-gw"},
		"F2e": {"etcd"}, "F3a": {"leader", "random-follower"}, "F3b": {"pooler:leader", "random-orch", "random-gw"},
		"F4a": {"leader", "random-follower"}, "F6": {"leader"}, "F7": {"leader", "random-follower"},
	}
	holds := map[string][]int64{
		"F1a": {10000, 30000}, "F1b": {10000, 30000}, "F1c": {10000, 30000}, "F1d": {10000, 30000},
		"F2a": {0}, "F2b": {5000}, "F2c": {5000}, "F2d": {5000}, "F2e": {5000},
		"F3a": {ms(params.PauseShort), ms(params.PauseLong)}, "F3b": {ms(params.PauseShort), ms(params.PauseLong)},
		"F4a": {10000}, "F6": {5000}, "F7": {10000},
	}
	in := func(x string, xs []string) bool {
		for _, y := range xs {
			if x == y {
				return true
			}
		}
		return false
	}
	seen := map[string]bool{}
	for seed := uint64(1); seed <= 14; seed++ {
		p := mkplan(t, "C12", seed, "baseline")
		if len(p.Steps) != 4 {
			t.Fatalf("seed %d: %d steps, want 4", seed, len(p.Steps))
		}
		first, last := p.Steps[0], p.Steps[3]
		if kinds(first) != "F5a" || first.Elements[0].Targets[0] != "n4" || first.DurationMS != 5000 ||
			first.After != AfterWorkloadStart || first.DelayMS != params.FirstFaultAt.Milliseconds() {
			t.Errorf("seed %d: step 0 %+v, want F5a(n4) 5 s at t = 10 s", seed, first)
		}
		if kinds(last) != "F5b" || last.Elements[0].Targets[0] != "n4" || last.DurationMS != 5000 {
			t.Errorf("seed %d: step 3 %+v, want F5b(n4) 5 s", seed, last)
		}
		drawn := C12KindsForSeed(seed)
		for i, s := range p.Steps[1:3] {
			if len(s.Elements) != 1 || s.Elements[0].Kind != drawn[i] {
				t.Fatalf("seed %d: step %d is %s, want %s", seed, i+1, kinds(s), drawn[i])
			}
			e := s.Elements[0]
			seen[e.Kind] = true
			if e.Kind == "F1b" {
				if len(e.A) != 2 || !in(e.A[0], []string{"leader", "random-follower"}) || e.A[1] != "random-orch" ||
					!reflect.DeepEqual(e.B, []string{"rest"}) {
					t.Errorf("seed %d: F1b sets %v / %v", seed, e.A, e.B)
				}
			} else if len(e.Targets) != 1 || !in(e.Targets[0], targets[e.Kind]) {
				t.Errorf("seed %d: %s targets %v, want one of %v", seed, e.Kind, e.Targets, targets[e.Kind])
			}
			ok := false
			for _, h := range holds[e.Kind] {
				ok = ok || s.DurationMS == h
			}
			if !ok {
				t.Errorf("seed %d: %s held %d ms, want one of %v", seed, e.Kind, s.DurationMS, holds[e.Kind])
			}
			if s.After != AfterPreviousHeal || s.DelayMS < params.FaultGap.Milliseconds() ||
				s.DelayMS >= (params.FaultGap+params.FaultJitterMax).Milliseconds() {
				t.Errorf("seed %d: step %d anchor %s delay %d, want P05", seed, i+1, s.After, s.DelayMS)
			}
		}
		got := map[string]int{}
		for _, c := range p.Clients {
			got[c.WL]++
			if c.GW != Gateways[c.Client%3] {
				t.Errorf("seed %d: client %d through %s", seed, c.Client, c.GW)
			}
		}
		if !reflect.DeepEqual(got, map[string]int{WLAppend: 4, WLRegister: 2, WLBank: 2, WLLedger: 2}) {
			t.Errorf("seed %d: workloads %v, want W1–W4 with P02's clients", seed, got)
		}
		if p.DurationMS != params.CampaignDuration.Milliseconds() {
			t.Errorf("seed %d: duration %d", seed, p.DurationMS)
		}
	}
	if len(seen) != 14 {
		t.Errorf("seeds 1–14 drew %d kinds, want 14", len(seen))
	}
}

// ADR-0041: a plan on slotless differs from the same campaign's baseline
// plan only in its profile field (V2 runs V1's schedules).
func TestSlotlessPlansDifferOnlyInProfile(t *testing.T) {
	for _, c := range []string{"C1", "C2", "C4"} {
		for seed := uint64(1); seed <= 5; seed++ {
			b, _ := mkplan(t, c, seed, "baseline").Canonical()
			s, _ := mkplan(t, c, seed, "slotless").Canonical()
			if bytes.Equal(b, s) {
				t.Fatalf("%s seed %d: slotless plan equals baseline plan", c, seed)
			}
			s = bytes.Replace(s, []byte(`"profile":"slotless"`), []byte(`"profile":"baseline"`), 1)
			if !bytes.Equal(b, s) {
				t.Fatalf("%s seed %d: slotless plan differs from baseline beyond the profile", c, seed)
			}
		}
	}
}
