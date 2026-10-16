package plan

import (
	"math/rand/v2"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

// K7 campaign C12 (CONTRACT §15 K7 "C12 mixed"; steps, kinds, targets,
// holds, seeds and clients fixed by ADR-0041 before any run of it).

// C12Kinds are the fault kinds C12 draws from (§15 K7), in the fixed order
// each block's permutation starts from.
var C12Kinds = []string{"F1a", "F1b", "F1c", "F1d", "F2a", "F2b", "F2c", "F2d", "F2e", "F3a", "F3b", "F4a", "F6", "F7"}

// C12 constants (ADR-0041).
const (
	// c12Draws: drawn faults per run, between F5a and F5b.
	c12Draws = 2
	// c12SeedsPerBlock: seeds 1–7, 8–14, … each draw every kind once.
	c12SeedsPerBlock = 7
	// c12KindStream names the block stream of the kind permutation.
	c12KindStream = "nemesis-kinds"
	// c12MemberHold: F5a and F5b are held as C4 holds them.
	c12MemberHold = c4Hold
	// c12NetHold: F4a and F7 alone (C5's F4a hold; K3's network hold).
	c12NetHold = holdNetwork
)

// C12KindsForSeed returns the kinds of seed's two drawn faults, in step
// order: positions 2·((seed−1) mod 7) and +1 of the permutation of C12Kinds
// drawn from the PCG stream of (C12, block (seed−1) div 7, "nemesis-kinds").
// Uniform at every draw, without replacement inside a block (ADR-0041).
func C12KindsForSeed(seed uint64) []string {
	n := seed - 1 // seed 0 wraps: still a pure function of the seed
	r := stream("C12", n/c12SeedsPerBlock, c12KindStream)
	perm := append([]string(nil), C12Kinds...)
	r.Shuffle(len(perm), func(i, j int) { perm[i], perm[j] = perm[j], perm[i] })
	pos := int(n % c12SeedsPerBlock)
	return perm[c12Draws*pos : c12Draws*pos+c12Draws]
}

// c12Step is one drawn fault with the targets and hold of the campaign that
// already exercises its kind (ADR-0041's table), choices from r.
func c12Step(r *rand.Rand, kind string) stepSpec {
	one := func(hold time.Duration, e Element) stepSpec {
		return stepSpec{Duration: hold, Elements: []Element{e}}
	}
	switch kind {
	case "F1a":
		e := el("F1a", pick(r, "leader", "random-follower"))
		return one(pick(r, c2HoldShort, c2HoldLong), e)
	case "F1b":
		e := Element{Kind: "F1b", A: []string{pick(r, "leader", "random-follower"), "random-orch"}, B: []string{"rest"}}
		return one(pick(r, c2HoldShort, c2HoldLong), e)
	case "F1c", "F1d":
		return one(pick(r, c2HoldShort, c2HoldLong), el(kind, "leader"))
	case "F2a":
		return one(0, el("F2a", pick(r, "leader", "random-node")))
	case "F2b":
		return one(killHold, el("F2b", pick(r, "leader", "random-node")))
	case "F2c":
		return one(killHold, el("F2c", "random-orch"))
	case "F2d":
		return one(killHold, el("F2d", "random-gw"))
	case "F2e":
		return one(killHold, el("F2e", "etcd"))
	case "F3a":
		e := el("F3a", pick(r, "leader", "random-follower"))
		return one(pick(r, params.PauseShort, params.PauseLong), e)
	case "F3b":
		e := el("F3b", pick(r, "pooler:leader", "random-orch", "random-gw"))
		return one(pick(r, params.PauseShort, params.PauseLong), e)
	case "F4a":
		return one(c12NetHold, el("F4a", pick(r, "leader", "random-follower")))
	case "F6":
		return one(k5Hold, el("F6", "leader"))
	case "F7":
		return one(c12NetHold, el("F7", pick(r, "leader", "random-follower")))
	}
	panic("C12: unknown kind " + kind)
}

// c12Shape: F5a(n4); the seed's two drawn faults; F5b(n4).
func c12Shape(r *rand.Rand, seed uint64) []stepSpec {
	out := []stepSpec{{Duration: c12MemberHold, Elements: []Element{el("F5a", "n4")}}}
	for _, k := range C12KindsForSeed(seed) {
		out = append(out, c12Step(r, k))
	}
	return append(out, stepSpec{Duration: c12MemberHold, Elements: []Element{el("F5b", "n4")}})
}

// k7Campaigns are the campaigns K7 adds: C12 (W1–W4, P02's clients).
func k7Campaigns() map[string]Campaign {
	return map[string]Campaign{
		"C12": {Name: "C12", Duration: params.CampaignDuration, Nemesis: true, shape: c12Shape},
	}
}
