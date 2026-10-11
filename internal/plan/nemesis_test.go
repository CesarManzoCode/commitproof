package plan

import (
	"bytes"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

func TestNemesisPlanDeterministicAndShaped(t *testing.T) {
	want := map[string][]string{
		"k3-f1a": {"F1a", "F1a", "F1a"}, "k3-f6": {"F6", "F6"}, "k3-f5": {"F5a", "F5b"},
		"k3-composite": {"F5a+F3a+F2a"}, "nsmoke-f7": {"F7"},
	}
	for name, kinds := range want {
		a, err := Generate(name, 1, "baseline", "")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := Generate(name, 1, "baseline", "")
		ab, _ := a.Canonical()
		bb, _ := b.Canonical()
		if !bytes.Equal(ab, bb) {
			t.Fatalf("%s: same inputs, different plans", name)
		}
		if len(a.Steps) != len(kinds) {
			t.Fatalf("%s: %d steps, want %d", name, len(a.Steps), len(kinds))
		}
		for i, s := range a.Steps {
			k := ""
			for j, e := range s.Elements {
				if j > 0 {
					k += "+"
				}
				k += e.Kind
			}
			if k != kinds[i] {
				t.Errorf("%s step %d: %s, want %s", name, i, k, kinds[i])
			}
			// P05: first fault at 10 s after workload start; later ones
			// 10 s + jitter in [0, 5) s after the previous heal.
			switch {
			case i == 0 && (s.After != AfterWorkloadStart || s.DelayMS != 10_000):
				t.Errorf("%s step 0: %s %d ms", name, s.After, s.DelayMS)
			case i > 0 && (s.After != AfterPreviousHeal || s.DelayMS < 10_000 || s.DelayMS >= 15_000):
				t.Errorf("%s step %d: %s %d ms", name, i, s.After, s.DelayMS)
			}
		}
	}
	// k3-f3a uses P16's short pause; F2a has no hold.
	p, _ := Generate("k3-f3a", 1, "baseline", "")
	if p.Steps[0].DurationMS != params.PauseShort.Milliseconds() {
		t.Errorf("F3a hold %d ms", p.Steps[0].DurationMS)
	}
	if s1, _ := Generate("k3-f1a", 2, "baseline", ""); s1.Steps[1].DelayMS == p.Steps[1].DelayMS && s1.Steps[2].DelayMS == p.Steps[2].DelayMS {
		t.Error("jitter does not depend on the seed")
	}
}

func TestParseStepsRoundTrip(t *testing.T) {
	p, _ := Generate("k3-f1b", 3, "baseline", "")
	b, _ := p.Canonical()
	v, err := canonjson.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseSteps(v.(map[string]any)["nemesis"])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[1].DelayMS != p.Steps[1].DelayMS || got[2].Elements[0].Pick != p.Steps[2].Elements[0].Pick {
		t.Fatalf("round trip %+v", got)
	}
	e := got[0].Elements[0]
	if e.Kind != "F1b" || len(e.A) != 2 || e.A[0] != "leader" || e.A[1] != "orch-z1" || len(e.B) != 1 || e.B[0] != "rest" {
		t.Fatalf("F1b element %+v", e)
	}
	if _, err := ParseSteps([]any{map[string]any{"step": int64(0), "after": "workload-start", "delay_ms": int64(1), "duration_ms": int64(1),
		"elements": []any{map[string]any{"kind": "F9", "pick": int64(0)}}}}); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestFaultIDAndBound(t *testing.T) {
	c, _ := Generate("k3-composite", 1, "baseline", "")
	if FaultID(c.Steps[0], 2) != "f0.2" {
		t.Errorf("composite id %s", FaultID(c.Steps[0], 2))
	}
	s, _ := Generate("k3-f6", 1, "baseline", "")
	if FaultID(s.Steps[1], 0) != "f1" {
		t.Errorf("simple id %s", FaultID(s.Steps[1], 0))
	}
	// Two F6 steps: each at least 60 s confirmation + 2×P28.
	if b := NemesisBound(s.Steps); b < 2*(SwitchoverConfirmDeadline+2*params.HealConfirmDeadline) {
		t.Errorf("bound %s too small", b)
	}
}
