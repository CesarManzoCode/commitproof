package plan

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

// K5 schedules (CONTRACT §15 K5 campaign table, §9.2 D3′/D5, §7 W5–W8;
// clients, shapes and timing fixed by ADR-0030 before any K5 run; D3′ by
// ADR-0033 before any D3′ run).

// K5 workload names (history field wl, CONTRACT §6.1).
const (
	WLSession  = "session"
	WLNotify   = "notify"
	WLCDC      = "cdc"
	WLOverload = "overload"
)

// Modes of clients without planned operations.
const (
	ModeListener = "listener" // W6 listener: LISTEN, then 1 s waits
	ModeConsumer = "consumer" // W7 logical replication consumer
	ModeStorm    = "storm"    // W8 cp_storm connections
)

// W8 roles and W7 names (CONTRACT §7).
const (
	RoleStorm   = "cp_storm"
	RoleSteady  = "cp_steady"
	Publication = "cp_pub"
	Slot        = "cp_slot"
	Channel     = "cp_chan"
)

// K5.3 slot precondition modes (plan setup "cdc_slot").
const (
	CDCSlotRequired = "required" // C10: not failover-ready before the workload → INVALID
	CDCSlotRecorded = "recorded" // D5: observed and recorded only
)

// Client numbers (ADR-0030).
const (
	SessionClient0  = 20 // W5 20–22
	NotifierClient0 = 23 // W6 notifiers 23–25
	ListenerClient0 = 26 // W6 listeners 26–28
	ConsumerClient  = 29 // W7
	StormClient0    = 100
	WarmupClient    = 14 // W7 slot warm-up writes (runner)
)

// K5 constants (ADR-0030, ADR-0029).
const (
	k5Hold        = 5 * time.Second  // F6, F2b holds (K3's holdProcess)
	c10F1cHold    = 10 * time.Second // F1c in C10 (K3's holdNetwork)
	d3Gap         = 15 * time.Second // D3: "15 s apart", after each heal
	d5Partition   = 20 * time.Second // D5: F1b "for 20 s"
	d5LeaderDown  = 30 * time.Second // D5: old leader down after F1b heals
	d3W4PacingMS  = 25               // D3: W4 "at 2× pacing"
	ConnpoolCap   = 50               // S03 --connpool-global-capacity
	StormPerPhase = 4 * ConnpoolCap  // W8: "up to 4× capacity"
	// ListenerTail: listeners keep waiting this long after the workload
	// deadline, so every NOTIFY has its full 10 s window (§10.9).
	ListenerTail = 11 * time.Second
	// ListenerWait is one W6 wait.
	ListenerWait = 1 * time.Second
	// SlotReadyDeadline bounds K5.3's wait (two 30 s slot-sync naptimes).
	SlotReadyDeadline = 60 * time.Second
	// BufferGuardWait: profile buffered waits buffer-min-time-between-
	// failovers after HEALTHY before the workload (ADR-0029, S14).
	BufferGuardWait = 60 * time.Second
	// CatchUpDeadline bounds W7's final catch-up (P08's 60 s).
	CatchUpDeadline = params.FinalReadDeadline
)

// StormPhases are W8's storm windows (ms from workload start).
var StormPhases = [][2]int64{{5000, 25000}, {45000, 65000}}

// W5's tracked GUCs in SET order and their value pools (ADR-0030).
var (
	SessionGUCs   = []string{"application_name", "search_path", "work_mem", "DateStyle"}
	SessionValues = map[string][]string{
		"application_name": {"cp-a", "cp-b", "cp-c", "cp-d"},
		"search_path":      {"cp, public", "public, cp", "public"},
		"work_mem":         {"4MB", "8MB", "16MB"},
		"DateStyle":        {"ISO, MDY", "ISO, DMY"},
	}
)

type k5Setup struct {
	publication string
	roles       []string
	cdcSlot     string
}

// Workload sets of the K5 campaigns' P02 clients.
func w1Clients(iso string, lockOrdered, autocommit bool) []Client {
	var out []Client
	for i := range params.ClientsAppend {
		c := Client{Client: i, WL: WLAppend, GW: Gateways[i%3], Iso: iso, lockOrdered: lockOrdered, autocommitAppend: autocommit}
		out = append(out, c)
	}
	return out
}

func w4Clients(pacingMS int64, role string) []Client {
	var out []Client
	for i := 8; i < 8+params.ClientsLedger; i++ {
		out = append(out, Client{Client: i, WL: WLLedger, GW: Gateways[i%3], PacingMS: pacingMS, Role: role})
	}
	return out
}

func perGateway(first int, wl, mode string) []Client {
	var out []Client
	for g, gw := range Gateways {
		out = append(out, Client{Client: first + g, WL: wl, GW: gw, Mode: mode})
	}
	return out
}

func consumer() Client {
	return Client{Client: ConsumerClient, WL: WLCDC, GW: Gateways[0], Mode: ModeConsumer}
}

// k5Campaigns are C8–C11 and the directed schedules D3, D5.
func k5Campaigns() map[string]Campaign {
	camp := func(name string, profile string, shape func(*rand.Rand, uint64) []stepSpec, clients func(uint64) []Client, setup k5Setup) Campaign {
		return Campaign{Name: name, Duration: params.CampaignDuration, Nemesis: true, shape: shape, Profiles: []string{profile},
			k5Clients: clients, setup: setup}
	}
	cdc := func(mode string) k5Setup { return k5Setup{publication: Publication, cdcSlot: mode} }
	cs := map[string]Campaign{
		"C8": camp("C8", "buffered", c8Shape, func(uint64) []Client {
			return append(w1Clients("serializable", true, false), w4Clients(0, "")...)
		}, k5Setup{}),
		"C9": camp("C9", "baseline", c9Shape, func(uint64) []Client {
			out := w4Clients(0, "")
			out = append(out, perGateway(SessionClient0, WLSession, "")...)
			out = append(out, perGateway(NotifierClient0, WLNotify, "")...)
			return append(out, perGateway(ListenerClient0, WLNotify, ModeListener)...)
		}, k5Setup{}),
		"C10": camp("C10", "baseline", c10Shape, func(uint64) []Client {
			out := append(w1Clients("serializable", false, false), w4Clients(0, "")...)
			return append(out, consumer())
		}, cdc(CDCSlotRequired)),
		"C11": camp("C11", "baseline", c11Shape, func(uint64) []Client {
			return append(w4Clients(0, RoleSteady), Client{Client: StormClient0, WL: WLOverload, Mode: ModeStorm, Role: RoleStorm,
				Connections: StormPerPhase, PhasesMS: append([][2]int64{}, StormPhases...)})
		}, k5Setup{roles: []string{RoleSteady, RoleStorm}}),
		"D3":  camp("D3", "buffered", d3Shape, d3Clients, k5Setup{}),
		"D3p": camp("D3p", "buffered", d3pShape, d3Clients, k5Setup{}),
		"D5": camp("D5", "baseline", d5Shape, func(uint64) []Client {
			return append(w4Clients(0, ""), consumer())
		}, cdc(CDCSlotRecorded)),
	}
	for _, d := range []string{"D3", "D3p", "D5"} {
		c := cs[d]
		c.Directed = d
		cs[d] = c
	}
	return cs
}

// c8Shape: F6 on the leader three times, held 5 s.
func c8Shape(_ *rand.Rand, _ uint64) []stepSpec {
	return steps(k5Hold, el("F6", "leader"), el("F6", "leader"), el("F6", "leader"))
}

// once returns one step per spec, in an order drawn from the nemesis stream.
func once(r *rand.Rand, specs []stepSpec) []stepSpec {
	out := make([]stepSpec, len(specs))
	for i, j := range r.Perm(len(specs)) {
		out[i] = specs[j]
	}
	return out
}

// c9Shape: F2a(leader), F6(leader), F2b(leader), once each.
func c9Shape(r *rand.Rand, _ uint64) []stepSpec {
	return once(r, []stepSpec{
		{Duration: 0, Elements: []Element{el("F2a", "leader")}},
		{Duration: k5Hold, Elements: []Element{el("F6", "leader")}},
		{Duration: k5Hold, Elements: []Element{el("F2b", "leader")}},
	})
}

// c10Shape: F2a(leader), F2b(leader), F1c(leader), F6(leader), once each.
func c10Shape(r *rand.Rand, _ uint64) []stepSpec {
	return once(r, []stepSpec{
		{Duration: 0, Elements: []Element{el("F2a", "leader")}},
		{Duration: k5Hold, Elements: []Element{el("F2b", "leader")}},
		{Duration: c10F1cHold, Elements: []Element{el("F1c", "leader")}},
		{Duration: k5Hold, Elements: []Element{el("F6", "leader")}},
	})
}

// c11Shape: no nemesis on odd seeds; F2a(leader) at t = 10 s, inside the
// first storm phase, on even seeds.
func c11Shape(_ *rand.Rand, seed uint64) []stepSpec {
	if seed%2 == 1 {
		return nil
	}
	return []stepSpec{{Duration: 0, Elements: []Element{el("F2a", "leader")}}}
}

// d3Clients are D3's and D3′'s: W1 autocommit appends at P03 and W4 at
// 2× pacing (ADR-0030).
func d3Clients(uint64) []Client {
	return append(w1Clients("", false, true), w4Clients(d3W4PacingMS, "")...)
}

// D3 (contract ≤ 1.0.9, ADR-0030), superseded by D3′ (ADR-0033) and kept
// for the superseded matrix campaigns/k5.json: F2a on the leader three
// times; the first at t = 10 s, each later one 15 s after the previous heal
// is confirmed (no jitter).
func d3Shape(_ *rand.Rand, _ uint64) []stepSpec {
	kill := func() stepSpec {
		return stepSpec{Duration: 0, Elements: []Element{el("F2a", "leader")}, FixedDelay: true, Delay: d3Gap}
	}
	return []stepSpec{kill(), kill(), kill()}
}

// D3′ (§9.2, ADR-0033; campaign "D3p"): D3's three steps and gaps, each
// step F7 on both followers, then F2a on the leader as soon as the second
// F7 is confirmed, so that every commit waits for its delayed standby
// acknowledgement, its WAL already on both followers, when the leader
// dies. Heals: both F7s at once, then F2a (PostgreSQL restarted by its
// pooler).
func d3pShape(_ *rand.Rand, _ uint64) []stepSpec {
	kill := func() stepSpec {
		return stepSpec{
			Duration:   0,
			Elements:   []Element{el("F7", "base-follower:0"), el("F7", "base-follower:1"), el("F2a", "leader")},
			Heals:      []Heal{{Element: 0}, {Element: 1}, {Element: 2}},
			FixedDelay: true, Delay: d3Gap,
		}
	}
	return []stepSpec{kill(), kill(), kill()}
}

// D5 (§9.2): F1b(A = followers, B = rest) held 20 s, then F2a and F2b on
// the leader; heal F1b, keep the old leader down 30 s, then F2b, then F2a.
func d5Shape(_ *rand.Rand, _ uint64) []stepSpec {
	return []stepSpec{{
		Duration: 0,
		Elements: []Element{
			held(Element{Kind: "F1b", A: []string{"followers"}, B: []string{"rest"}}, d5Partition),
			el("F2a", "leader"),
			el("F2b", "same:1"),
		},
		Heals: []Heal{{Element: 0}, {Element: 2, DelayMS: d5LeaderDown.Milliseconds()}, {Element: 1}},
	}}
}

// sessionSpec draws a W5 client's GUC values from its stream; statement
// values are unique per client.
func sessionSpec(client int, r *rand.Rand) *SessionSpec {
	s := &SessionSpec{}
	for _, g := range SessionGUCs {
		vs := SessionValues[g]
		s.GUCs = append(s.GUCs, [2]string{g, vs[r.IntN(len(vs))]})
	}
	for j := 1; j <= 4; j++ {
		v := int64(client*10 + j)
		st := Statement{Name: fmt.Sprintf("cp_s%d", v), Kind: "sql", Value: v}
		if j > 2 {
			st = Statement{Name: fmt.Sprintf("cp_p%d", v), Kind: "protocol", Value: v}
		}
		s.Statements = append(s.Statements, st)
	}
	return s
}

// genK5Ops fills a K5 campaign's clients and their operations, generated in
// rounds like P02's (round r: every client in order; a client at 2× pacing
// takes two ops per round), with the shared generator state.
func genK5Ops(p *Plan, c Campaign, seed uint64) {
	p.Clients = c.k5Clients(seed)
	rngs := make([]*rand.Rand, len(p.Clients))
	for i := range p.Clients {
		rngs[i] = stream(c.Name, seed, fmt.Sprintf("client-%d", p.Clients[i].Client))
		if p.Clients[i].WL == WLSession {
			p.Clients[i].Session = sessionSpec(p.Clients[i].Client, rngs[i])
		}
	}
	g := newGen()
	n := int(c.Duration / params.Pacing)
	for round := 0; round < n; round++ {
		for i := range p.Clients {
			cl := &p.Clients[i]
			if cl.Mode != "" {
				continue
			}
			per := int64(1)
			if cl.PacingMS > 0 {
				per = params.Pacing.Milliseconds() / cl.PacingMS
			}
			for range per {
				cl.Ops = append(cl.Ops, g.nextK5(cl, rngs[i]))
			}
		}
	}
}

// nextK5 is gen.next for the K5 client kinds.
func (g *gen) nextK5(c *Client, r *rand.Rand) Op {
	switch c.WL {
	case WLAppend:
		if c.autocommitAppend {
			slot := r.IntN(len(g.active))
			k := g.active[slot]
			g.appended[k]++
			v := g.appended[k]
			if v >= params.AppendKeyRetireAt {
				g.active[slot] = g.nextKey
				g.nextKey++
			}
			return Op{F: "append", Value: []any{[]any{"append", k, v}}}
		}
		op := g.next(c, r)
		if c.lockOrdered {
			mops := op.Value.([]any)
			sort.SliceStable(mops, func(i, j int) bool { return mops[i].([]any)[1].(int64) < mops[j].([]any)[1].(int64) })
		}
		return op
	case WLLedger:
		op := g.next(c, r)
		if c.Role != "" {
			v := op.Value.(map[string]any)
			v["role"] = c.Role
		}
		return op
	case WLNotify:
		g.ledgerID++
		return Op{F: "notify", Value: map[string]any{"id": g.ledgerID, "payload": fmt.Sprintf("n%d-%016x", c.Client, r.Uint64())}}
	case WLSession:
		x := r.IntN(100)
		switch {
		case x < 45:
			return Op{F: "show", Value: map[string]any{"guc": SessionGUCs[r.IntN(len(SessionGUCs))]}}
		case x < 90:
			st := c.Session.Statements[r.IntN(len(c.Session.Statements))]
			return Op{F: "execute", Value: map[string]any{"name": st.Name}}
		default:
			guc := SessionGUCs[r.IntN(len(SessionGUCs))]
			vs := SessionValues[guc]
			return Op{F: "set", Value: map[string]any{"guc": guc, "value": vs[r.IntN(len(vs))]}}
		}
	}
	return g.next(c, r)
}

// mapK5 adds a client's K5 fields to its plan.json form (only when set, so
// K1–K4 plans are unchanged).
func (c Client) mapK5(m map[string]any) {
	if c.PacingMS > 0 {
		m["pacing_ms"] = c.PacingMS
	}
	if c.Mode != "" {
		m["mode"] = c.Mode
	}
	if c.Role != "" {
		m["role"] = c.Role
	}
	if c.Connections > 0 {
		m["connections"] = c.Connections
	}
	if len(c.PhasesMS) > 0 {
		ph := make([]any, len(c.PhasesMS))
		for i, x := range c.PhasesMS {
			ph[i] = []any{x[0], x[1]}
		}
		m["phases_ms"] = ph
	}
	if c.Session != nil {
		gucs := make([]any, len(c.Session.GUCs))
		for i, x := range c.Session.GUCs {
			gucs[i] = []any{x[0], x[1]}
		}
		sts := make([]any, len(c.Session.Statements))
		for i, x := range c.Session.Statements {
			sts[i] = map[string]any{"name": x.Name, "kind": x.Kind, "value": x.Value}
		}
		m["session"] = map[string]any{"gucs": gucs, "statements": sts}
	}
}

// decodeK5 reads the K5 fields of a decoded plan client.
func (c *Client) decodeK5(cm map[string]any) error {
	var err error
	for k, dst := range map[string]*int64{"pacing_ms": &c.PacingMS, "connections": &c.Connections} {
		if v, ok := cm[k]; ok {
			if *dst, err = toInt(v); err != nil || *dst <= 0 {
				return fmt.Errorf("bad %s", k)
			}
		}
	}
	c.Mode, _ = cm["mode"].(string)
	c.Role, _ = cm["role"].(string)
	if v, ok := cm["phases_ms"]; ok {
		l, _ := v.([]any)
		for _, x := range l {
			pr, _ := x.([]any)
			if len(pr) != 2 {
				return fmt.Errorf("bad phases_ms")
			}
			a, e1 := toInt(pr[0])
			b, e2 := toInt(pr[1])
			if e1 != nil || e2 != nil || a < 0 || b <= a {
				return fmt.Errorf("bad phases_ms")
			}
			c.PhasesMS = append(c.PhasesMS, [2]int64{a, b})
		}
	}
	if v, ok := cm["session"]; ok {
		sm, _ := v.(map[string]any)
		s := &SessionSpec{}
		gl, _ := sm["gucs"].([]any)
		for _, x := range gl {
			pr := strs(x)
			if len(pr) != 2 {
				return fmt.Errorf("bad session gucs")
			}
			s.GUCs = append(s.GUCs, [2]string{pr[0], pr[1]})
		}
		sl, _ := sm["statements"].([]any)
		for _, x := range sl {
			xm, _ := x.(map[string]any)
			st := Statement{}
			st.Name, _ = xm["name"].(string)
			st.Kind, _ = xm["kind"].(string)
			if st.Value, err = toInt(xm["value"]); err != nil || st.Name == "" || (st.Kind != "sql" && st.Kind != "protocol") {
				return fmt.Errorf("bad session statement")
			}
			s.Statements = append(s.Statements, st)
		}
		c.Session = s
	}
	return nil
}
