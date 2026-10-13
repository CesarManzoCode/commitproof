package plan

import (
	"fmt"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Fault kinds of CONTRACT §8 (closed catalog, ADR-0005).
var FaultKinds = []string{"F1a", "F1b", "F1c", "F1d", "F2a", "F2b", "F2c", "F2d", "F2e", "F3a", "F3b", "F4a", "F5a", "F5b", "F6", "F7"}

// Confirmation deadlines that §8 states per fault instead of P27: membership
// faults (F5a, F5b) 120 s, graceful switchover (F6) 60 s.
const (
	MembershipConfirmDeadline = 120 * time.Second
	SwitchoverConfirmDeadline = 60 * time.Second
)

// ConfirmDeadline is the confirmation deadline of a fault kind (§8, P27).
func ConfirmDeadline(kind string) time.Duration {
	switch kind {
	case "F5a", "F5b":
		return MembershipConfirmDeadline
	case "F6":
		return SwitchoverConfirmDeadline
	}
	return params.FaultConfirmDeadline
}

// Healable reports whether a fault kind has a heal (F5a and F5b do not:
// n4 stays until F5b, and F5b is itself the removal).
func Healable(kind string) bool { return kind != "F5a" && kind != "F5b" }

// Element is one fault of a step: a simple fault is a step with one element,
// a composite fault a step with several, injected and confirmed in order and
// healed in reverse order (CONTRACT §8).
//
// Targets are roles or names resolved at run time (§6.3): "leader",
// "followers", "random-follower", "random-orch", "random-gw", a node "n1"…"n4",
// a service "orch-z1", "gw-z2", "etcd", "runner", or a container role
// "pooler:<node role>" / "pg:<node role>" (F3b). F1b has two sets A and B
// instead, where "rest" means every other running network namespace. Pick
// selects among candidates for random roles (candidate index = pick mod the
// number of candidates, candidates sorted by name).
//
// K4 roles (ADR-0024): "random-node" (any running node), "base-follower:<k>"
// (the k-th of n1–n3, sorted, without the leader: the "other two followers"
// of D2, the same under every build whatever the cohort records), "same:<j>"
// (the node element j of the same step resolved to), and "followers" in an
// F1b set (every running follower of the leader's cohort).
//
// HoldMS is how long the nemesis waits after this element is confirmed
// before injecting the next element of the step (0 for the last element:
// the step's DurationMS applies).
//
// ConfirmAfterHeals defers the element's confirmation predicate until the
// step's heal sequence has completed (ADR-0026: C4's "F5b during F1a(leader)",
// whose predicate cannot hold while the leader is partitioned).
type Element struct {
	Kind              string
	Targets           []string
	A, B              []string
	Pick              int64
	HoldMS            int64
	ConfirmAfterHeals bool
}

// Heal is one entry of a step's explicit heal sequence: element Element is
// healed DelayMS after the previous entry's heal is confirmed (the first
// entry: after the step's DurationMS).
type Heal struct {
	Element int
	DelayMS int64
}

// Step is one plan step. The first step starts DelayMS after workload start
// (P05: t = 10 s); every later step starts DelayMS (P05: 10 s + seeded jitter
// in [0, 5) s) after the previous step's last heal is confirmed. Each fault
// lasts DurationMS after its last element is confirmed.
//
// Heals, when set, replaces the default heal order (reverse injection
// order, no delays) with an explicit sequence; elements without a heal
// (F5a, F5b) are not listed.
type Step struct {
	Index      int
	After      string // "workload-start", "previous-heal" or "new-leader"
	DelayMS    int64
	DurationMS int64
	Elements   []Element
	Heals      []Heal
}

// Step anchors.
const (
	AfterWorkloadStart = "workload-start"
	AfterPreviousHeal  = "previous-heal"
)

// stepSpec is the fixed shape of a campaign step; jitter and picks are drawn
// from the nemesis stream.
type stepSpec struct {
	Duration time.Duration
	Elements []Element
	After    string // "" or AfterNewLeader (other steps follow P05)
	Heals    []Heal
	// FixedDelay: a later step starts Delay after the previous heal is
	// confirmed, without P05's jitter (D3, ADR-0030).
	FixedDelay bool
	Delay      time.Duration
}

func el(kind string, targets ...string) Element { return Element{Kind: kind, Targets: targets} }

func steps(d time.Duration, es ...Element) []stepSpec {
	out := make([]stepSpec, len(es))
	for i, e := range es {
		out[i] = stepSpec{Duration: d, Elements: []Element{e}}
	}
	return out
}

// K3 hold durations (how long a fault lasts after confirmation). They are
// implementation choices sized so that three applications, their
// confirmations and the P05 gaps fit in a P04 campaign workload; F3a uses
// P16's short pause and F2a none (the postmaster's death is the fault; its
// self-restart is observed by the heal predicate).
const (
	holdNetwork   = 10 * time.Second // F1, F4a, F7
	holdProcess   = 5 * time.Second  // F2b–F2e, F3b, F5a, F6
	holdComposite = 5 * time.Second
)

// nemesisCampaigns are the K3 fault campaigns (one per fault kind of K3.1,
// F5a+F5b together, one composite) and the CI nemesis smoke runs (§13).
func nemesisCampaigns() map[string]Campaign {
	f1b := func() Element { return Element{Kind: "F1b", A: []string{"leader", "orch-z1"}, B: []string{"rest"}} }
	cs := map[string][]stepSpec{
		"k3-f1a": steps(holdNetwork, el("F1a", "leader"), el("F1a", "random-follower"), el("F1a", "leader")),
		"k3-f1b": steps(holdNetwork, f1b(), f1b(), f1b()),
		"k3-f1c": steps(holdNetwork, el("F1c", "leader"), el("F1c", "leader"), el("F1c", "leader")),
		"k3-f1d": steps(holdNetwork, el("F1d", "leader"), el("F1d", "leader"), el("F1d", "leader")),
		"k3-f2a": steps(0, el("F2a", "leader"), el("F2a", "random-follower"), el("F2a", "leader")),
		"k3-f2b": steps(holdProcess, el("F2b", "leader"), el("F2b", "random-follower"), el("F2b", "leader")),
		"k3-f2c": steps(holdProcess, el("F2c", "orch-z1"), el("F2c", "orch-z2"), el("F2c", "orch-z3")),
		"k3-f2d": steps(holdProcess, el("F2d", "gw-z1"), el("F2d", "gw-z2"), el("F2d", "gw-z3")),
		"k3-f2e": steps(holdProcess, el("F2e", "etcd"), el("F2e", "etcd"), el("F2e", "etcd")),
		"k3-f3a": steps(params.PauseShort, el("F3a", "leader"), el("F3a", "random-follower"), el("F3a", "leader")),
		"k3-f3b": steps(holdProcess, el("F3b", "pooler:leader"), el("F3b", "random-orch"), el("F3b", "random-gw")),
		"k3-f4a": steps(holdNetwork, el("F4a", "leader"), el("F4a", "random-follower"), el("F4a", "leader")),
		"k3-f5": {
			{Duration: holdProcess, Elements: []Element{el("F5a", "n4")}},
			{Duration: 0, Elements: []Element{el("F5b", "n4")}},
		},
		"k3-f6": steps(holdProcess, el("F6", "leader"), el("F6", "leader")),
		"k3-f7": steps(holdNetwork, el("F7", "leader"), el("F7", "random-follower"), el("F7", "leader")),
		// §8's example shape F5a → F3a → F2a(leader), one step.
		"k3-composite": {{Duration: holdComposite, Elements: []Element{
			el("F5a", "n4"), el("F3a", "random-follower"), el("F2a", "leader")}}},
	}
	out := map[string]Campaign{}
	for name, s := range cs {
		out[name] = Campaign{Name: name, Duration: params.CampaignDuration, Nemesis: true, steps: s}
	}
	// CI nemesis smoke (§13): one 45 s run per fault kind, one fault each.
	for kind, spec := range map[string]stepSpec{
		"f1a": {Duration: holdNetwork, Elements: []Element{el("F1a", "leader")}},
		"f2a": {Duration: 0, Elements: []Element{el("F2a", "leader")}},
		"f3a": {Duration: params.PauseShort, Elements: []Element{el("F3a", "leader")}},
		"f4a": {Duration: holdNetwork, Elements: []Element{el("F4a", "leader")}},
		"f7":  {Duration: holdNetwork, Elements: []Element{el("F7", "leader")}},
	} {
		name := "nsmoke-" + kind
		out[name] = Campaign{Name: name, Duration: params.SmokeDuration, Nemesis: true, steps: []stepSpec{spec}}
	}
	return out
}

// genNemesis draws the step shapes (K4 campaigns), the P05 jitter and the
// role picks from the nemesis stream.
func genNemesis(c Campaign, seed uint64) []Step {
	r := stream(c.Name, seed, "nemesis")
	specs := c.steps
	if c.shape != nil {
		specs = c.shape(r, seed)
	}
	var out []Step
	for i, s := range specs {
		st := Step{Index: i, DurationMS: s.Duration.Milliseconds(), Heals: s.Heals}
		switch {
		case i == 0:
			st.After, st.DelayMS = AfterWorkloadStart, params.FirstFaultAt.Milliseconds()
		case s.After == AfterNewLeader:
			st.After = AfterNewLeader
			st.DelayMS = NewLeaderDelayMin.Milliseconds() + r.Int64N(NewLeaderDelaySpan.Milliseconds())
		case s.FixedDelay:
			st.After, st.DelayMS = AfterPreviousHeal, s.Delay.Milliseconds()
		default:
			st.After = AfterPreviousHeal
			st.DelayMS = params.FaultGap.Milliseconds() + r.Int64N(params.FaultJitterMax.Milliseconds())
		}
		for _, e := range s.Elements {
			e.Pick = r.Int64N(1 << 31)
			st.Elements = append(st.Elements, e)
		}
		out = append(out, st)
	}
	return out
}

func strList(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// Map is the plan.json form of a step.
func (s Step) Map() map[string]any {
	els := make([]any, len(s.Elements))
	for i, e := range s.Elements {
		m := map[string]any{"kind": e.Kind, "pick": e.Pick}
		if e.Kind == "F1b" {
			m["a"], m["b"] = strList(e.A), strList(e.B)
		} else {
			m["targets"] = strList(e.Targets)
		}
		if e.HoldMS > 0 {
			m["hold_ms"] = e.HoldMS
		}
		if e.ConfirmAfterHeals {
			m["confirm_after_heals"] = true
		}
		els[i] = m
	}
	m := map[string]any{"step": s.Index, "after": s.After, "delay_ms": s.DelayMS, "duration_ms": s.DurationMS, "elements": els}
	if len(s.Heals) > 0 {
		hs := make([]any, len(s.Heals))
		for i, h := range s.Heals {
			hs[i] = map[string]any{"element": int64(h.Element), "delay_ms": h.DelayMS}
		}
		m["heals"] = hs
	}
	return m
}

// HealSequence returns the step's heal sequence: Heals if set, else every
// healable element in reverse injection order with no delay.
func (s Step) HealSequence() []Heal {
	if len(s.Heals) > 0 {
		return s.Heals
	}
	var out []Heal
	for k := len(s.Elements) - 1; k >= 0; k-- {
		if Healable(s.Elements[k].Kind) {
			out = append(out, Heal{Element: k})
		}
	}
	return out
}

// FaultID names element k of step s in history events: "f<s>" for a simple
// fault, "f<s>.<k>" for an element of a composite.
func FaultID(s Step, k int) string {
	if len(s.Elements) == 1 {
		return fmt.Sprintf("f%d", s.Index)
	}
	return fmt.Sprintf("f%d.%d", s.Index, k)
}

// ParseSteps reads the nemesis field of a decoded plan.json.
func ParseSteps(v any) ([]Step, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("plan nemesis: not a list")
	}
	var out []Step
	for i, x := range list {
		m, ok := x.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("plan nemesis[%d]: not an object", i)
		}
		st := Step{}
		var err error
		st.After, _ = m["after"].(string)
		idx, e1 := toInt(m["step"])
		st.DelayMS, err = toInt(m["delay_ms"])
		dur, e2 := toInt(m["duration_ms"])
		st.DurationMS, st.Index = dur, int(idx)
		if err != nil || e1 != nil || e2 != nil || (st.After != AfterWorkloadStart && st.After != AfterPreviousHeal && st.After != AfterNewLeader) || st.Index != i {
			return nil, fmt.Errorf("plan nemesis[%d]: bad step header", i)
		}
		els, _ := m["elements"].([]any)
		if len(els) == 0 {
			return nil, fmt.Errorf("plan nemesis[%d]: no elements", i)
		}
		for k, ev := range els {
			em, _ := ev.(map[string]any)
			e := Element{}
			e.Kind, _ = em["kind"].(string)
			if !knownKind(e.Kind) {
				return nil, fmt.Errorf("plan nemesis[%d].elements[%d]: unknown kind %q", i, k, e.Kind)
			}
			if e.Pick, err = toInt(em["pick"]); err != nil {
				return nil, fmt.Errorf("plan nemesis[%d].elements[%d]: pick: %w", i, k, err)
			}
			e.Targets, e.A, e.B = strs(em["targets"]), strs(em["a"]), strs(em["b"])
			if h, ok := em["hold_ms"]; ok {
				if e.HoldMS, err = toInt(h); err != nil || e.HoldMS < 0 {
					return nil, fmt.Errorf("plan nemesis[%d].elements[%d]: bad hold_ms", i, k)
				}
			}
			if c, ok := em["confirm_after_heals"]; ok {
				if c != true {
					return nil, fmt.Errorf("plan nemesis[%d].elements[%d]: bad confirm_after_heals", i, k)
				}
				e.ConfirmAfterHeals = true
			}
			st.Elements = append(st.Elements, e)
		}
		if hv, ok := m["heals"].([]any); ok {
			seen := map[int]bool{}
			for j, x := range hv {
				hm, _ := x.(map[string]any)
				el, e1 := toInt(hm["element"])
				d, e2 := toInt(hm["delay_ms"])
				if e1 != nil || e2 != nil || el < 0 || int(el) >= len(st.Elements) || d < 0 || seen[int(el)] ||
					!Healable(st.Elements[el].Kind) {
					return nil, fmt.Errorf("plan nemesis[%d].heals[%d]: bad heal", i, j)
				}
				seen[int(el)] = true
				st.Heals = append(st.Heals, Heal{Element: int(el), DelayMS: d})
			}
			for k, e := range st.Elements {
				if Healable(e.Kind) && !seen[k] {
					return nil, fmt.Errorf("plan nemesis[%d]: element %d has no heal in the heal sequence", i, k)
				}
			}
		}
		out = append(out, st)
	}
	return out, nil
}

func knownKind(k string) bool {
	for _, x := range FaultKinds {
		if x == k {
			return true
		}
	}
	return false
}

func strs(v any) []string {
	l, _ := v.([]any)
	out := make([]string, 0, len(l))
	for _, x := range l {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// toInt converts a decoded JSON integer (json.Number from canonjson.Decode,
// or int64/int).
func toInt(v any) (int64, error) {
	switch x := v.(type) {
	case int64:
		return x, nil
	case int:
		return int64(x), nil
	case interface{ Int64() (int64, error) }:
		return x.Int64()
	}
	return 0, fmt.Errorf("not an integer: %v", v)
}

// commandSlack bounds the commands of one inject or heal (docker exec,
// docker start/stop): three P01 client-operation timeouts.
const commandSlack = 3 * params.ClientOpTimeout

// NemesisBound is the longest the nemesis schedule can take before it ends
// or aborts: every delay and hold, every confirmation deadline, two P28 heal
// windows per element (F2a may need a manual start after its self-restart
// window), and command time. The host bounds the runner with it.
func NemesisBound(steps []Step) time.Duration {
	var d time.Duration
	for _, s := range steps {
		d += time.Duration(s.DelayMS+s.DurationMS) * time.Millisecond
		if s.After == AfterNewLeader {
			d += NewLeaderDeadline
		}
		for _, h := range s.Heals {
			d += time.Duration(h.DelayMS) * time.Millisecond
		}
		for _, e := range s.Elements {
			d += ConfirmDeadline(e.Kind) + 2*params.HealConfirmDeadline + 2*commandSlack + time.Duration(e.HoldMS)*time.Millisecond
			if e.Kind == "F5b" {
				d += 30 * time.Second // docker stop -t 30 of pooler-n4 (§8)
			}
		}
	}
	return d
}
