package plan

import (
	"math/rand/v2"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

// K4 schedules (CONTRACT §15 K4 campaign table, §9.2 D1/D2, §9.4 DH1;
// parameters the contract leaves open are fixed by ADR-0024 before any K4
// run). Shapes are drawn from the plan's nemesis stream, so a plan stays a
// pure function of (campaign, seed, profile, directed, contract version).

// Workload sets per campaign (§15 K4 "Workloads" column).
var (
	wlC1   = []string{WLAppend, WLBank, WLLedger}     // W1, W3, W4
	wlC2C3 = []string{WLAppend, WLRegister, WLLedger} // W1, W2, W4
	wlW1W4 = []string{WLAppend, WLLedger}             // W1, W4
)

// Step anchor of C6's second fault of each pair: the step starts DelayMS
// after a leader of a decided rule newer than the one the previous step
// resolved its targets against is observed (deadline NewLeaderDeadline).
const AfterNewLeader = "new-leader"

// C6: "3–8 s after the new leader is observed"; the observation deadline
// is the F6 one (a leader change, §8).
const (
	NewLeaderDelayMin  = 3 * time.Second
	NewLeaderDelaySpan = 5 * time.Second
	NewLeaderDeadline  = SwitchoverConfirmDeadline
)

// C7CancelAfter: the client sends a CancelRequest 1 s after COMMIT is sent
// (§15 K4, C7).
const C7CancelAfter = 1 * time.Second

// Directed-schedule constants (§9.2, §9.4; ADR-0024).
const (
	d1Hold        = 30 * time.Second  // D1: F1c "for 30 s"
	d2Load        = 40 * time.Second  // D2: "40 s of W4/W1 load" with both followers paused
	d2LeaderDown  = 30 * time.Second  // D2: leader kept down after the F3a heals (ADR-0024)
	dh1Pause      = 120 * time.Second // DH1: "F3a on the leader's PostgreSQL for 120 s"
	c2HoldShort   = 10 * time.Second
	c2HoldLong    = 30 * time.Second
	c4Hold        = 5 * time.Second
	c5Hold        = 10 * time.Second
	c7Partition   = 15 * time.Second // C7: F1b "for 15 s, then F2a on the leader"
	killHold      = 5 * time.Second  // F2b–F2d: as K3's holdProcess
	probeDuration = 10 * time.Second
)

func pick[T any](r *rand.Rand, xs ...T) T { return xs[r.IntN(len(xs))] }

func held(e Element, d time.Duration) Element { e.HoldMS = d.Milliseconds(); return e }

// c1Shape: four kill faults, kind uniform over {F2a, F2b, F2c, F2d}; F2a and
// F2b on the leader or a random node.
func c1Shape(r *rand.Rand, _ uint64) []stepSpec {
	var out []stepSpec
	for range 4 {
		switch pick(r, "F2a", "F2b", "F2c", "F2d") {
		case "F2a":
			out = append(out, stepSpec{Duration: 0, Elements: []Element{el("F2a", pick(r, "leader", "random-node"))}})
		case "F2b":
			out = append(out, stepSpec{Duration: killHold, Elements: []Element{el("F2b", pick(r, "leader", "random-node"))}})
		case "F2c":
			out = append(out, stepSpec{Duration: killHold, Elements: []Element{el("F2c", "random-orch")}})
		default:
			out = append(out, stepSpec{Duration: killHold, Elements: []Element{el("F2d", "random-gw")}})
		}
	}
	return out
}

// c2Shape: three partitions, kind uniform over {F1a, F1b, F1c, F1d}, hold
// 10 s or 30 s.
func c2Shape(r *rand.Rand, _ uint64) []stepSpec {
	var out []stepSpec
	for range 3 {
		var e Element
		switch pick(r, "F1a", "F1b", "F1c", "F1d") {
		case "F1a":
			e = el("F1a", pick(r, "leader", "random-follower"))
		case "F1b":
			e = Element{Kind: "F1b", A: []string{pick(r, "leader", "random-follower"), "random-orch"}, B: []string{"rest"}}
		case "F1c":
			e = el("F1c", "leader")
		default:
			e = el("F1d", "leader")
		}
		out = append(out, stepSpec{Duration: pick(r, c2HoldShort, c2HoldLong), Elements: []Element{e}})
	}
	return out
}

// c3Shape: three pauses: F3a (leader or follower) or F3b (the leader's
// pooler, an orchestrator or a gateway), each for P16 short or long.
func c3Shape(r *rand.Rand, _ uint64) []stepSpec {
	var out []stepSpec
	for range 3 {
		var e Element
		if pick(r, "F3a", "F3b") == "F3a" {
			e = el("F3a", pick(r, "leader", "random-follower"))
		} else {
			e = el("F3b", pick(r, "pooler:leader", "random-orch", "random-gw"))
		}
		out = append(out, stepSpec{Duration: pick(r, params.PauseShort, params.PauseLong), Elements: []Element{e}})
	}
	return out
}

// c4Shape: one admission and one removal of n4 per run. The variants rotate
// with the seed so that seeds 1–3 cover every sequence of the K4 table:
// admission (seed−1) mod 3 = plain F5a, F5a→F2a(leader), F5a→F3a(the two
// original followers)→F2a(leader) (D2's shape); removal (seed−1) mod 2 =
// plain F5b, F5b during F1a(leader) (F5b confirmed after F1a heals,
// ADR-0026).
func c4Shape(_ *rand.Rand, seed uint64) []stepSpec {
	v := seed - 1
	var add []Element
	switch v % 3 {
	case 0:
		add = []Element{el("F5a", "n4")}
	case 1:
		add = []Element{el("F5a", "n4"), el("F2a", "leader")}
	default:
		add = []Element{el("F5a", "n4"), el("F3a", "base-follower:0"), el("F3a", "base-follower:1"), el("F2a", "leader")}
	}
	rm := []Element{el("F5b", "n4")}
	if v%2 == 1 {
		// F5b is injected while F1a holds; its predicate is evaluated after
		// F1a heals (ADR-0026).
		f5b := el("F5b", "n4")
		f5b.ConfirmAfterHeals = true
		rm = []Element{el("F1a", "leader"), f5b}
	}
	return []stepSpec{{Duration: c4Hold, Elements: add}, {Duration: c4Hold, Elements: rm}}
}

// c5Shape: three composites F4a(leader or follower) → F2a(leader). The slow
// disk is healed before F2a's heal predicate waits for PostgreSQL's restart
// (ADR-0028): with both on one node, crash recovery at 200 ms per write and
// fsync cannot finish within P28.
func c5Shape(r *rand.Rand, _ uint64) []stepSpec {
	var out []stepSpec
	for range 3 {
		out = append(out, stepSpec{Duration: c5Hold, Elements: []Element{el("F4a", pick(r, "leader", "random-follower")), el("F2a", "leader")},
			Heals: []Heal{{Element: 0}, {Element: 1}}})
	}
	return out
}

// c6Shape: F2a(leader), then F2a(new leader) 3–8 s after the new leader is
// observed; twice.
func c6Shape(_ *rand.Rand, _ uint64) []stepSpec {
	pair := []stepSpec{
		{Duration: 0, Elements: []Element{el("F2a", "leader")}},
		{Duration: 0, Elements: []Element{el("F2a", "leader")}, After: AfterNewLeader},
	}
	return append(append([]stepSpec{}, pair...), pair...)
}

// c7Shape: twice, F1b with A = every follower node, B = everything else
// (commits block) for 15 s, then F2a on the leader.
func c7Shape(_ *rand.Rand, _ uint64) []stepSpec {
	step := func() stepSpec {
		return stepSpec{Duration: 0, Elements: []Element{
			held(Element{Kind: "F1b", A: []string{"followers"}, B: []string{"rest"}}, c7Partition),
			el("F2a", "leader"),
		}}
	}
	return []stepSpec{step(), step()}
}

// D1 (§9.2): at t = 10 s, F1c on the leader for 30 s; heal.
func d1Shape(_ *rand.Rand, _ uint64) []stepSpec {
	return []stepSpec{{Duration: d1Hold, Elements: []Element{el("F1c", "leader")}}}
}

// D2 (§9.2, ADR-0024): F5a (admission predicate) → F3a on the other two
// followers → 40 s of load → F2a + F2b on the leader → heal F3a; the
// leader's pooler and PostgreSQL stay down for d2LeaderDown, then F2b and F2a
// are healed (F2a: PostgreSQL restarts by itself once its pooler runs).
func d2Shape(_ *rand.Rand, _ uint64) []stepSpec {
	return []stepSpec{{
		Duration: 0,
		Elements: []Element{
			el("F5a", "n4"),
			el("F3a", "base-follower:0"),
			held(el("F3a", "base-follower:1"), d2Load),
			el("F2a", "leader"),
			el("F2b", "same:3"),
		},
		Heals: []Heal{{Element: 2}, {Element: 1}, {Element: 4, DelayMS: d2LeaderDown.Milliseconds()}, {Element: 3}},
	}}
}

// DH1 (§9.4): F3a on the leader's PostgreSQL for 120 s, kill -CONT.
func dh1Shape(_ *rand.Rand, _ uint64) []stepSpec {
	return []stepSpec{{Duration: dh1Pause, Elements: []Element{el("F3a", "leader")}}}
}

// k4Campaigns are the K4 campaigns, directed schedules and the probe suite.
func k4Campaigns() map[string]Campaign {
	camp := func(name string, wls []string, shape func(*rand.Rand, uint64) []stepSpec) Campaign {
		return Campaign{Name: name, Duration: params.CampaignDuration, Nemesis: true, Workloads: wls, shape: shape}
	}
	cs := map[string]Campaign{
		"C1":  camp("C1", wlC1, c1Shape),
		"C2":  camp("C2", wlC2C3, c2Shape),
		"C3":  camp("C3", wlC2C3, c3Shape),
		"C4":  camp("C4", wlW1W4, c4Shape),
		"C5":  camp("C5", wlW1W4, c5Shape),
		"C6":  camp("C6", wlW1W4, c6Shape),
		"C7":  camp("C7", wlW1W4, c7Shape),
		"D1":  camp("D1", wlW1W4, d1Shape),
		"D2":  camp("D2", wlW1W4, d2Shape),
		"DH1": camp("DH1", wlW1W4, dh1Shape),
	}
	c3 := cs["C3"]
	c3.isoBySeed = true
	cs["C3"] = c3
	c7 := cs["C7"]
	c7.c7 = true
	cs["C7"] = c7
	for _, d := range []string{"D1", "D2", "DH1"} {
		c := cs[d]
		c.Directed = d
		cs[d] = c
	}
	dh1 := cs["DH1"]
	dh1.Profiles = []string{"h1"}
	cs["DH1"] = dh1
	cs[ProbeSuite] = Campaign{Name: ProbeSuite, Duration: probeDuration, Workloads: []string{WLProbe}, Probes: Probes()}
	return cs
}

// ProbeSuite is the campaign name of `commitproof probe --suite claims`.
const ProbeSuite = "claims"

// Probe is one claims probe (§3 G7, §15 K4): one statement through a
// gateway. What the response must be is the checker's business (§10, P),
// not the plan's.
type Probe struct {
	ID  string
	SQL string
}

// Probes are P1 and P2 of §15 K4; P1-show reads the setting back after P1
// (diagnosis: whether an accepted assignment took effect in the session).
func Probes() []Probe {
	return []Probe{
		{ID: "P1", SQL: "SET synchronous_commit = off"},
		{ID: "P1-show", SQL: "SHOW synchronous_commit"},
		{ID: "P2", SQL: "ALTER SYSTEM SET work_mem = '8MB'"},
	}
}
