package plan

import (
	"bytes"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

var k5Profiles = map[string]string{"C8": "buffered", "C9": "baseline", "C10": "baseline", "C11": "baseline", "D3": "buffered", "D5": "baseline"}

func TestK5CampaignsDeterministicAndRoundTrip(t *testing.T) {
	for c, prof := range k5Profiles {
		for seed := uint64(1); seed <= 5; seed++ {
			a, b := mkplan(t, c, seed, prof), mkplan(t, c, seed, prof)
			ab, _ := a.Canonical()
			bb, _ := b.Canonical()
			if !bytes.Equal(ab, bb) {
				t.Fatalf("%s seed %d: same inputs, different plans", c, seed)
			}
			d, err := Decode(ab)
			if err != nil {
				t.Fatalf("%s seed %d: decode: %v", c, seed, err)
			}
			for _, red := range []func(*Plan) (*Plan, error){(*Plan).HalveDuration, func(p *Plan) (*Plan, error) { return p.KeepSteps(nil) }} {
				q, err := red(d)
				if err != nil {
					t.Fatal(err)
				}
				qb, _ := q.Canonical()
				if _, err := Decode(qb); err != nil {
					t.Fatalf("%s seed %d: reduced plan does not decode: %v", c, seed, err)
				}
			}
		}
		other := map[string]string{"buffered": "baseline", "baseline": "buffered"}[prof]
		if _, err := Generate(c, 1, other, ""); err == nil {
			t.Errorf("%s accepted profile %s (§15 K5: %s only; buffering only in C8 and D3)", c, other, prof)
		}
	}
}

// ADR-0030's client table.
func TestK5Clients(t *testing.T) {
	want := map[string][]int{
		"C8":  {0, 1, 2, 3, 8, 9},
		"C9":  {8, 9, 20, 21, 22, 23, 24, 25, 26, 27, 28},
		"C10": {0, 1, 2, 3, 8, 9, 29},
		"C11": {8, 9, 100},
		"D3":  {0, 1, 2, 3, 8, 9},
		"D5":  {8, 9, 29},
	}
	for c, nums := range want {
		p := mkplan(t, c, 1, k5Profiles[c])
		var got []int
		for _, cl := range p.Clients {
			got = append(got, cl.Client)
			n := int(params.CampaignDuration / params.Pacing)
			switch {
			case cl.Mode != "":
				if len(cl.Ops) != 0 {
					t.Errorf("%s client %d (%s) has planned ops", c, cl.Client, cl.Mode)
				}
			case cl.PacingMS == 25:
				if len(cl.Ops) != 2*n {
					t.Errorf("%s client %d: %d ops at 25 ms", c, cl.Client, len(cl.Ops))
				}
			default:
				if len(cl.Ops) != n {
					t.Errorf("%s client %d: %d ops", c, cl.Client, len(cl.Ops))
				}
			}
		}
		if len(got) != len(nums) {
			t.Fatalf("%s clients %v, want %v", c, got, nums)
		}
		for i := range got {
			if got[i] != nums[i] {
				t.Fatalf("%s clients %v, want %v", c, got, nums)
			}
		}
	}
	p := mkplan(t, "C9", 1, "baseline")
	for _, cl := range p.Clients {
		if cl.Client >= SessionClient0 && cl.Client < SessionClient0+3 && (cl.GW != Gateways[cl.Client-SessionClient0] || cl.Session == nil || len(cl.Session.Statements) != 4) {
			t.Errorf("W5 client %+v", cl)
		}
	}
}

// C8's W1 takes row locks in key order (ADR-0030); D3's W1 appends once per
// autocommit op; C11's steady inserts carry the role.
func TestK5WorkloadForms(t *testing.T) {
	for _, cl := range mkplan(t, "C8", 3, "buffered").Clients {
		if cl.WL != WLAppend {
			continue
		}
		for _, o := range cl.Ops {
			prev := int64(-1)
			for _, m := range o.Value.([]any) {
				k := m.([]any)[1].(int64)
				if k < prev {
					t.Fatalf("C8 txn %v not in key order", o.Value)
				}
				prev = k
			}
		}
	}
	for _, cl := range mkplan(t, "D3", 3, "buffered").Clients {
		for _, o := range cl.Ops {
			if cl.WL == WLAppend && (o.F != "append" || len(o.Value.([]any)) != 1 || cl.Iso != "") {
				t.Fatalf("D3 W1 op %v (iso %q)", o, cl.Iso)
			}
		}
	}
	for _, cl := range mkplan(t, "C11", 2, "baseline").Clients {
		if cl.WL == WLLedger && cl.Ops[0].Value.(map[string]any)["role"] != RoleSteady {
			t.Fatalf("C11 steady op %v", cl.Ops[0])
		}
	}
}

func TestK5Shapes(t *testing.T) {
	for seed := uint64(1); seed <= 5; seed++ {
		for c, kindsWant := range map[string]map[string]bool{
			"C9":  {"F2a": true, "F6": true, "F2b": true},
			"C10": {"F2a": true, "F2b": true, "F1c": true, "F6": true},
		} {
			p := mkplan(t, c, seed, "baseline")
			got := map[string]bool{}
			for _, s := range p.Steps {
				got[kinds(s)] = true
			}
			if len(p.Steps) != len(kindsWant) || len(got) != len(kindsWant) {
				t.Fatalf("%s seed %d: steps %v", c, seed, got)
			}
		}
		if n := len(mkplan(t, "C11", seed, "baseline").Steps); (seed%2 == 1 && n != 0) || (seed%2 == 0 && n != 1) {
			t.Fatalf("C11 seed %d: %d steps", seed, n)
		}
		d3 := mkplan(t, "D3", seed, "buffered")
		for i, s := range d3.Steps {
			if kinds(s) != "F2a" || (i > 0 && (s.After != AfterPreviousHeal || s.DelayMS != 15000)) {
				t.Fatalf("D3 step %d: %+v", i, s)
			}
		}
		d5 := mkplan(t, "D5", seed, "baseline")
		if len(d5.Steps) != 1 || kinds(d5.Steps[0]) != "F1b+F2a+F2b" || d5.Steps[0].Elements[0].HoldMS != 20000 ||
			d5.Steps[0].Heals[1].DelayMS != 30000 || d5.Steps[0].Heals[1].Element != 2 {
			t.Fatalf("D5 %+v", d5.Steps)
		}
	}
}
