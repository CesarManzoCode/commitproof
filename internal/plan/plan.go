// Package plan generates plan.json (CONTRACT §6.3): a pure function of
// (campaign, seed, profile, directed schedule id, contract version).
//
// Randomness: math/rand/v2 PCG. Each stream s (one per client, "client-<n>",
// and "nemesis") is rand.NewPCG(seed, fnv1a64(campaign) XOR fnv1a64(s)): the
// first term is the campaign-specific constant, the second the stream
// constant. Operations are generated in rounds (round r: client 0, 1, …, 9)
// so that state shared between clients — list-append key rotation (P20),
// per-run unique values (P21, W4 ids) — is deterministic.
//
// The plan contains no wall-clock times; durations are in milliseconds.
package plan

import (
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"sort"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Workload names (history field wl).
const (
	WLAppend   = "append"
	WLRegister = "register"
	WLBank     = "bank"
	WLLedger   = "ledger"
)

// Gateways in cell order.
var Gateways = []string{"gw-z1", "gw-z2", "gw-z3"}

// Profiles of CONTRACT §5.5.
var Profiles = []string{"baseline", "buffered", "slotless", "replica-port", "h1"}

// Campaign describes what a campaign's plan contains.
type Campaign struct {
	Name     string
	Duration time.Duration
	// Nemesis is false for campaigns without faults.
	Nemesis bool
	steps   []stepSpec
}

// Campaigns implemented so far. K1: smoke (W1–W4, no nemesis, P04 smoke).
// K3: one fault campaign per fault kind (k3-*), a composite, and the CI
// nemesis smoke runs (nsmoke-*), all with W1–W4 (nemesis.go).
func Campaigns() map[string]Campaign {
	cs := map[string]Campaign{
		"smoke": {Name: "smoke", Duration: params.SmokeDuration},
	}
	for n, c := range nemesisCampaigns() {
		cs[n] = c
	}
	return cs
}

// Op is one planned client operation.
type Op struct {
	F     string
	Value any
}

// Client is one workload client's schedule.
type Client struct {
	Client int
	WL     string
	GW     string
	Iso    string // txn workloads only
	Ops    []Op
}

// Plan is plan.json.
type Plan struct {
	ContractVersion string
	Campaign        string
	Seed            uint64
	Profile         string
	Directed        string
	DurationMS      int64
	PacingMS        int64
	ClientTimeoutMS int64
	Clients         []Client
	Steps           []Step // nemesis schedule (§6.3, §8)
}

func fnv1a64(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

func stream(campaign string, seed uint64, name string) *rand.Rand {
	return rand.New(rand.NewPCG(seed, fnv1a64(campaign)^fnv1a64(name)))
}

// Generate builds the plan. It is deterministic in its arguments and
// params.ContractVersion.
func Generate(campaign string, seed uint64, profile, directed string) (*Plan, error) {
	c, ok := Campaigns()[campaign]
	if !ok {
		names := make([]string, 0)
		for n := range Campaigns() {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("unknown campaign %q (implemented: %v)", campaign, names)
	}
	known := false
	for _, p := range Profiles {
		known = known || p == profile
	}
	if !known {
		return nil, fmt.Errorf("unknown profile %q (CONTRACT §5.5: %v)", profile, Profiles)
	}
	if directed != "" {
		return nil, fmt.Errorf("directed schedule %q: none defined before K4", directed)
	}
	p := &Plan{
		ContractVersion: params.ContractVersion,
		Campaign:        c.Name,
		Seed:            seed,
		Profile:         profile,
		Directed:        directed,
		DurationMS:      c.Duration.Milliseconds(),
		PacingMS:        params.Pacing.Milliseconds(),
		ClientTimeoutMS: params.ClientOpTimeout.Milliseconds(),
	}
	if c.Nemesis {
		p.Steps = genNemesis(c, seed)
	}
	// Client layout (P02): append 0–3, register 4–5, bank 6–7, ledger 8–9.
	// Gateway: client i uses Gateways[i mod 3].
	var wls []string
	for range params.ClientsAppend {
		wls = append(wls, WLAppend)
	}
	for range params.ClientsRegister {
		wls = append(wls, WLRegister)
	}
	for range params.ClientsBank {
		wls = append(wls, WLBank)
	}
	for range params.ClientsLedger {
		wls = append(wls, WLLedger)
	}
	rngs := make([]*rand.Rand, len(wls))
	for i, wl := range wls {
		cl := Client{Client: i, WL: wl, GW: Gateways[i%len(Gateways)]}
		if wl == WLAppend || wl == WLBank {
			cl.Iso = "serializable"
		}
		p.Clients = append(p.Clients, cl)
		rngs[i] = stream(c.Name, seed, fmt.Sprintf("client-%d", i))
	}
	g := newGen()
	n := int(c.Duration / params.Pacing)
	for round := 0; round < n; round++ {
		for i := range p.Clients {
			p.Clients[i].Ops = append(p.Clients[i].Ops, g.next(&p.Clients[i], rngs[i]))
		}
	}
	return p, nil
}

// gen is the shared, deterministic generator state.
type gen struct {
	active   []int64         // P20: active list-append keys
	nextKey  int64           // next fresh list-append key
	appended map[int64]int64 // planned appends per key (values are 1..n per key)
	regValue int64           // last unique register value handed out (P21)
	regLast  map[int64]int64 // last planned value written to each register key
	ledgerID int64
}

func newGen() *gen {
	g := &gen{appended: map[int64]int64{}, regLast: map[int64]int64{}}
	for k := int64(0); k < params.AppendActiveKeys; k++ {
		g.active = append(g.active, k)
	}
	g.nextKey = params.AppendActiveKeys
	return g
}

func (g *gen) next(c *Client, r *rand.Rand) Op {
	switch c.WL {
	case WLAppend:
		n := 1 + r.IntN(4)
		mops := make([]any, 0, n)
		for range n {
			slot := r.IntN(len(g.active))
			k := g.active[slot]
			if r.IntN(2) == 0 {
				mops = append(mops, []any{"r", k, nil})
				continue
			}
			g.appended[k]++
			v := g.appended[k]
			mops = append(mops, []any{"append", k, v})
			if v >= params.AppendKeyRetireAt {
				g.active[slot] = g.nextKey
				g.nextKey++
			}
		}
		return Op{F: "txn", Value: mops}
	case WLRegister:
		k := int64(r.IntN(params.RegisterKeys))
		switch x := r.IntN(10); {
		case x < 4:
			return Op{F: "read", Value: map[string]any{"k": k}}
		case x < 8:
			g.regValue++
			g.regLast[k] = g.regValue
			return Op{F: "write", Value: map[string]any{"k": k, "v": g.regValue}}
		default:
			old, ok := g.regLast[k]
			if !ok {
				return Op{F: "read", Value: map[string]any{"k": k}}
			}
			g.regValue++
			g.regLast[k] = g.regValue
			return Op{F: "cas", Value: map[string]any{"k": k, "old": old, "new": g.regValue}}
		}
	case WLBank:
		if r.IntN(10) < 7 {
			from := int64(r.IntN(params.BankAccounts))
			to := int64(r.IntN(params.BankAccounts - 1))
			if to >= from {
				to++
			}
			amt := int64(1 + r.IntN(5))
			return Op{F: "transfer", Value: map[string]any{"from": from, "to": to, "amt": amt}}
		}
		return Op{F: "read-all", Value: map[string]any{}}
	case WLLedger:
		g.ledgerID++
		return Op{F: "insert", Value: map[string]any{"id": g.ledgerID, "payload": fmt.Sprintf("c%d-%016x", c.Client, r.Uint64())}}
	}
	panic("unknown workload " + c.WL)
}

// Map returns the canonical-JSON-ready form of the plan.
func (p *Plan) Map() map[string]any {
	clients := make([]any, len(p.Clients))
	for i, c := range p.Clients {
		ops := make([]any, len(c.Ops))
		for j, o := range c.Ops {
			ops[j] = map[string]any{"f": o.F, "value": o.Value}
		}
		m := map[string]any{"client": c.Client, "wl": c.WL, "gw": c.GW, "ops": ops}
		if c.Iso != "" {
			m["iso"] = c.Iso
		}
		clients[i] = m
	}
	nemesis := make([]any, len(p.Steps))
	for i, s := range p.Steps {
		nemesis[i] = s.Map()
	}
	return map[string]any{
		"campaign":          p.Campaign,
		"client_timeout_ms": p.ClientTimeoutMS,
		"clients":           clients,
		"contract_version":  p.ContractVersion,
		"directed":          p.Directed,
		"duration_ms":       p.DurationMS,
		"nemesis":           nemesis,
		"pacing_ms":         p.PacingMS,
		"profile":           p.Profile,
		"seed":              p.Seed,
		"setup": map[string]any{
			"append_active_keys": params.AppendActiveKeys,
			"append_retire_at":   params.AppendKeyRetireAt,
			"bank_accounts":      params.BankAccounts,
			"bank_initial":       params.BankInitialBalance,
			"register_keys":      params.RegisterKeys,
		},
	}
}

// Canonical returns plan.json bytes (no trailing newline).
func (p *Plan) Canonical() ([]byte, error) {
	return canonjson.Marshal(p.Map())
}
