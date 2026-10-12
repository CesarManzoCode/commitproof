package plan

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

func mkplan(t *testing.T, c string, seed uint64, profile string) *Plan {
	t.Helper()
	p, err := Generate(c, seed, profile, "")
	if err != nil {
		t.Fatalf("%s seed %d: %v", c, seed, err)
	}
	return p
}

func kinds(s Step) string {
	var k []string
	for _, e := range s.Elements {
		k = append(k, e.Kind)
	}
	return strings.Join(k, "+")
}

func TestK4CampaignsDeterministicAndRoundTrip(t *testing.T) {
	for _, c := range []string{"C1", "C2", "C3", "C4", "C5", "C6", "C7", "D1", "D2", "D2p", ProbeSuite} {
		for seed := uint64(1); seed <= 5; seed++ {
			a, b := mkplan(t, c, seed, "baseline"), mkplan(t, c, seed, "baseline")
			ab, _ := a.Canonical()
			bb, _ := b.Canonical()
			if !bytes.Equal(ab, bb) {
				t.Fatalf("%s seed %d: same inputs, different plans", c, seed)
			}
			d, err := Decode(ab)
			if err != nil {
				t.Fatalf("%s seed %d: decode: %v", c, seed, err)
			}
			db, _ := d.Canonical()
			if !bytes.Equal(db, ab) {
				t.Fatalf("%s seed %d: decode round trip differs", c, seed)
			}
		}
	}
	p := mkplan(t, "DH1", 1, "h1")
	b, _ := p.Canonical()
	if _, err := Decode(b); err != nil {
		t.Fatal(err)
	}
}

// Workloads per campaign (CONTRACT §15 K4 table); client numbers keep P02.
func TestK4Workloads(t *testing.T) {
	want := map[string]map[string]int{
		"C1": {WLAppend: 4, WLBank: 2, WLLedger: 2},
		"C2": {WLAppend: 4, WLRegister: 2, WLLedger: 2},
		"C3": {WLAppend: 4, WLRegister: 2, WLLedger: 2},
		"C4": {WLAppend: 4, WLLedger: 2}, "C5": {WLAppend: 4, WLLedger: 2}, "C6": {WLAppend: 4, WLLedger: 2},
		"C7": {WLAppend: 4, WLLedger: 2}, "D1": {WLAppend: 4, WLLedger: 2}, "D2": {WLAppend: 4, WLLedger: 2},
		"D2p":      {WLAppend: 4, WLLedger: 2},
		ProbeSuite: {WLProbe: 3},
	}
	for c, w := range want {
		p := mkplan(t, c, 1, "baseline")
		got := map[string]int{}
		for _, cl := range p.Clients {
			got[cl.WL]++
			if cl.WL == WLLedger && cl.Client < 8 {
				t.Errorf("%s: ledger client numbered %d", c, cl.Client)
			}
		}
		if !reflect.DeepEqual(got, w) {
			t.Errorf("%s: workloads %v, want %v", c, got, w)
		}
	}
}

func TestC3IsolationAlternates(t *testing.T) {
	for seed, iso := range map[uint64]string{1: "serializable", 2: "repeatable-read", 3: "serializable"} {
		for _, c := range mkplan(t, "C3", seed, "baseline").Clients {
			if c.WL == WLAppend && c.Iso != iso {
				t.Errorf("seed %d: W1 iso %s, want %s", seed, c.Iso, iso)
			}
		}
	}
	for _, c := range mkplan(t, "C2", 2, "baseline").Clients {
		if c.WL == WLAppend && c.Iso != "serializable" {
			t.Errorf("C2 W1 iso %s", c.Iso)
		}
	}
}

func TestC7ClientOptions(t *testing.T) {
	for _, c := range mkplan(t, "C7", 1, "baseline").Clients {
		switch c.WL {
		case WLLedger:
			if c.StatementTimeoutMS != params.C7StatementTimeout.Milliseconds() || c.CancelCommitAfterMS != 0 {
				t.Errorf("W4 client %d: %+v", c.Client, c)
			}
		case WLAppend:
			if c.CancelCommitAfterMS != 1000 || c.StatementTimeoutMS != 0 {
				t.Errorf("W1 client %d: %+v", c.Client, c)
			}
		}
	}
	for _, c := range mkplan(t, "C2", 1, "baseline").Clients {
		if c.StatementTimeoutMS != 0 || c.CancelCommitAfterMS != 0 {
			t.Errorf("C2 client with C7 options: %+v", c)
		}
	}
}

// The directed schedules are exactly §9.2 / §9.4 (ADR-0024, ADR-0027).
func TestDirectedSchedules(t *testing.T) {
	d1 := mkplan(t, "D1", 3, "baseline")
	if d1.Directed != "D1" || len(d1.Steps) != 1 || kinds(d1.Steps[0]) != "F1c" || d1.Steps[0].Elements[0].Targets[0] != "leader" ||
		d1.Steps[0].DelayMS != 10_000 || d1.Steps[0].After != AfterWorkloadStart || d1.Steps[0].DurationMS != 30_000 {
		t.Errorf("D1: %+v", d1.Steps)
	}
	d2 := mkplan(t, "D2", 1, "baseline")
	s := d2.Steps
	if len(s) != 1 || kinds(s[0]) != "F5a+F3a+F3a+F2a+F2b" || s[0].DurationMS != 0 {
		t.Fatalf("D2: %+v", s)
	}
	e := s[0].Elements
	if e[0].Targets[0] != "n4" || e[1].Targets[0] != "base-follower:0" || e[2].Targets[0] != "base-follower:1" ||
		e[2].HoldMS != 40_000 || e[3].Targets[0] != "leader" || e[4].Targets[0] != "same:3" {
		t.Errorf("D2 elements %+v", e)
	}
	if want := []Heal{{2, 0}, {1, 0}, {4, 30_000}, {3, 0}}; !reflect.DeepEqual(s[0].Heals, want) {
		t.Errorf("D2 heals %+v, want %+v", s[0].Heals, want)
	}
	// D2′ is D2 plus F2b(n4), healed last; nothing else differs (picks are
	// drawn from a stream keyed by the campaign name and select nothing:
	// none of D2's roles is random).
	noPick := func(es []Element) []Element {
		out := append([]Element{}, es...)
		for i := range out {
			out[i].Pick = 0
		}
		return out
	}
	for seed := uint64(1); seed <= 5; seed++ {
		dp, d := mkplan(t, "D2p", seed, "baseline"), mkplan(t, "D2", seed, "baseline")
		sp, sd := dp.Steps, d.Steps
		if dp.Directed != "D2p" || len(sp) != 1 || kinds(sp[0]) != "F5a+F3a+F3a+F2a+F2b+F2b" || sp[0].DurationMS != 0 {
			t.Fatalf("D2p seed %d: %+v", seed, sp)
		}
		if !reflect.DeepEqual(noPick(sp[0].Elements[:5]), noPick(sd[0].Elements)) || sp[0].Elements[5].Targets[0] != "n4" || sp[0].Elements[5].HoldMS != 0 ||
			sp[0].DelayMS != sd[0].DelayMS || sp[0].After != sd[0].After || dp.DurationMS != d.DurationMS {
			t.Errorf("D2p seed %d differs from D2 beyond F2b(n4): %+v vs %+v", seed, sp[0], sd[0])
		}
		if want := append(append([]Heal{}, sd[0].Heals...), Heal{5, 0}); !reflect.DeepEqual(sp[0].Heals, want) {
			t.Errorf("D2p heals %+v, want %+v", sp[0].Heals, want)
		}
	}
	dh1 := mkplan(t, "DH1", 1, "h1")
	if len(dh1.Steps) != 1 || kinds(dh1.Steps[0]) != "F3a" || dh1.Steps[0].DurationMS != 120_000 || dh1.Steps[0].Elements[0].Targets[0] != "leader" {
		t.Errorf("DH1: %+v", dh1.Steps)
	}
	if _, err := Generate("DH1", 1, "baseline", ""); err == nil {
		t.Error("DH1 accepted on baseline")
	}
	if _, err := Generate("D1", 1, "baseline", "D2"); err == nil {
		t.Error("D1 accepted with --directed D2")
	}
	if p, err := Generate("D2", 1, "baseline", "D2"); err != nil || p.Directed != "D2" {
		t.Errorf("D2 with --directed D2: %v", err)
	}
	if _, err := Generate("D2p", 1, "baseline", "D2"); err == nil {
		t.Error("D2p accepted with --directed D2")
	}
}

func TestK4Shapes(t *testing.T) {
	// C4 covers its four sequences over seeds 1–3.
	want := map[uint64][]string{1: {"F5a", "F5b"}, 2: {"F5a+F2a", "F1a+F5b"}, 3: {"F5a+F3a+F3a+F2a", "F5b"}}
	for seed, w := range want {
		p := mkplan(t, "C4", seed, "baseline")
		var got []string
		for _, s := range p.Steps {
			got = append(got, kinds(s))
		}
		if !reflect.DeepEqual(got, w) {
			t.Errorf("C4 seed %d: %v, want %v", seed, got, w)
		}
	}
	if e := mkplan(t, "C4", 2, "baseline").Steps[1].Elements; !e[1].ConfirmAfterHeals || e[0].ConfirmAfterHeals {
		t.Errorf("C4 seed 2: F5b during F1a must be confirmed after the heals (ADR-0026): %+v", e)
	}
	for _, st := range mkplan(t, "C5", 1, "baseline").Steps {
		if !reflect.DeepEqual(st.Heals, []Heal{{0, 0}, {1, 0}}) {
			t.Errorf("C5 heals %+v: the slow disk heals first (ADR-0028)", st.Heals)
		}
	}
	// C6: F2a pairs, the second anchored on the new leader, 3–8 s.
	for seed := uint64(1); seed <= 5; seed++ {
		p := mkplan(t, "C6", seed, "baseline")
		if len(p.Steps) != 4 {
			t.Fatalf("C6: %d steps", len(p.Steps))
		}
		for i, s := range p.Steps {
			if kinds(s) != "F2a" {
				t.Errorf("C6 step %d: %s", i, kinds(s))
			}
			if i%2 == 1 && (s.After != AfterNewLeader || s.DelayMS < 3000 || s.DelayMS >= 8000) {
				t.Errorf("C6 step %d: %s %d", i, s.After, s.DelayMS)
			}
			if i == 2 && s.After != AfterPreviousHeal {
				t.Errorf("C6 step 2: %s", s.After)
			}
		}
	}
	// C7: F1b(followers | rest) held 15 s, then F2a(leader).
	for _, s := range mkplan(t, "C7", 2, "baseline").Steps {
		e := s.Elements
		if kinds(s) != "F1b+F2a" || e[0].A[0] != "followers" || e[0].B[0] != "rest" || e[0].HoldMS != 15_000 || e[1].Targets[0] != "leader" {
			t.Errorf("C7 step %+v", s)
		}
	}
	// Kinds stay inside each campaign's pool (§15 K4) over many seeds.
	pools := map[string]string{"C1": "F2a F2b F2c F2d", "C2": "F1a F1b F1c F1d", "C3": "F3a F3b", "C5": "F4a F2a"}
	for c, pool := range pools {
		seen := map[string]bool{}
		for seed := uint64(1); seed <= 40; seed++ {
			for _, s := range mkplan(t, c, seed, "baseline").Steps {
				for _, e := range s.Elements {
					if !strings.Contains(pool, e.Kind) {
						t.Errorf("%s seed %d: %s outside the pool", c, seed, e.Kind)
					}
					seen[e.Kind] = true
				}
			}
		}
		if len(seen) != len(strings.Fields(pool)) {
			t.Errorf("%s: kinds seen %v over 40 seeds, pool %s", c, seen, pool)
		}
	}
}

func TestReductions(t *testing.T) {
	p := mkplan(t, "C1", 1, "baseline")
	sum, _ := p.SHA256()
	q, err := p.KeepSteps([]int{2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Steps) != 2 || q.Steps[0].Index != 0 || q.Steps[0].After != AfterWorkloadStart || q.Steps[0].DelayMS != 10_000 ||
		q.Steps[1].DelayMS != p.Steps[3].DelayMS || q.ReducedFrom != sum || len(p.Steps) != 4 {
		t.Errorf("keep-steps: %+v", q.Steps)
	}
	h, _ := q.HalveDuration()
	if h.DurationMS != 45_000 || len(h.Clients[0].Ops) != 900 || len(q.Clients[0].Ops) != 1800 || len(h.Reductions) != 2 {
		t.Errorf("halve: %d ms, %d ops, reductions %v", h.DurationMS, len(h.Clients[0].Ops), h.Reductions)
	}
	k, _ := h.KeepClients([]int{0, 8})
	if len(k.Clients) != 2 || k.Clients[1].Client != 8 {
		t.Errorf("keep-clients: %+v", k.Clients)
	}
	b, _ := k.Canonical()
	d, err := Decode(b)
	if err != nil || !reflect.DeepEqual(d.Reductions, k.Reductions) {
		t.Errorf("decode of a reduced plan: %v %v", err, d)
	}
	if _, err := h.KeepClients([]int{77}); err == nil {
		t.Error("empty client set accepted")
	}
}
