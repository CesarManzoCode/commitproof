package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

// Op is an invoke with its completion (CONTRACT §6.1).
type Op struct {
	Inv, Comp   int64 // line indices
	TInv, TComp int64 // ns since run start
	Client      int64
	Proc        int64
	GW, WL, F   string
	Type        string // ok, fail, info
	Err         map[string]any
	InValue     any
}

// Latency is the client-observed duration.
func (o *Op) Latency() int64 { return o.TComp - o.TInv }

// Fault is the nemesis events sharing one fault id (§6.1, §8).
type Fault struct {
	ID, Kind string
	Targets  []string
	Events   []history.Event
	// I is the inject time; E the healed time, else the confirmed time
	// (F5a/F5b have no heal), else the last event's (ADR-0036).
	I, E int64
	// Phase times by phase name (the first event of each phase).
	Phase map[string]int64
}

// Rule is a decided rule observed by the poolers (ADR-0036).
type Rule struct {
	Term, Sub  int64
	Leader     string
	Cohort     []string
	FirstSeen  int64
	SeenBy     string
	LeaderPrev string // the previous rule's leader in identity order; "" for the first
}

// Key renders the rule identity as "term.subterm".
func (r Rule) Key() string { return fmt.Sprintf("%d.%d", r.Term, r.Sub) }

// LeaderChange reports whether the rule changed the leader (ADR-0036).
func (r Rule) LeaderChange() bool { return r.LeaderPrev != "" && r.LeaderPrev != r.Leader }

// NodeObs is one observation of a node's state (pooler-status, node-sql).
type NodeObs struct {
	T       int64
	OK      bool
	State   string // primary, standby, error, down
	WAL     int64  // LSN as an integer; -1 unknown
	Latency int64  // probe latency in ms; -1 unknown
	Detail  string
}

// SlotObs is one cdc-slot observation of a node.
type SlotObs struct {
	T      int64
	Ready  bool
	Detail string
}

// ClientObs is a client-side error observation.
type ClientObs struct {
	T       int64
	Op      int64
	Message string
}

// Model is everything a run page shows, derived from the sealed files.
type Model struct {
	Run       *Run
	Ops       []*Op           // by invoke index
	OpOf      map[int64]*Op   // invoke or completion index → op
	Markers   []history.Event // marker events
	Faults    []*Fault        // by inject time, then id
	Rules     []Rule          // by identity
	Nodes     map[string][]NodeObs
	Slots     map[string][]SlotObs
	ClientObs []ClientObs
	ObsBySrc  map[string]int64
	Counts    map[string]int64 // history events by type
	End       int64            // t of the last history event
}

// Build derives the model. It fails on a history that pairs an invoke with
// no completion (VerifySeal's well-formedness already excludes it).
func Build(r *Run) (*Model, error) {
	m := &Model{Run: r, OpOf: map[int64]*Op{}, Nodes: map[string][]NodeObs{}, Slots: map[string][]SlotObs{},
		ObsBySrc: map[string]int64{}, Counts: map[string]int64{}}
	inv := map[int64]history.Event{}
	faults := map[string]*Fault{}
	for _, ev := range r.History {
		m.Counts[ev.Type]++
		m.End = ev.T
		switch ev.Type {
		case history.TypeInvoke:
			inv[ev.I] = ev
		case history.TypeOK, history.TypeFail, history.TypeInfo:
			if ev.Op == nil {
				return nil, fmt.Errorf("i=%d: completion without op", ev.I)
			}
			in, ok := inv[*ev.Op]
			if !ok || in.Client == nil || in.Proc == nil {
				return nil, fmt.Errorf("i=%d: completion of op %d, which is not an invoke with client and proc", ev.I, *ev.Op)
			}
			o := &Op{Inv: in.I, Comp: ev.I, TInv: in.T, TComp: ev.T, Client: *in.Client, Proc: *in.Proc,
				GW: in.GW, WL: in.WL, F: in.F, Type: ev.Type, Err: ev.Err, InValue: in.Value}
			m.Ops = append(m.Ops, o)
			m.OpOf[o.Inv], m.OpOf[o.Comp] = o, o
			delete(inv, in.I)
		case history.TypeMarker:
			m.Markers = append(m.Markers, ev)
		case history.TypeNemesis:
			id := str(ev.Fault, "id")
			f, ok := faults[id]
			if !ok {
				f = &Fault{ID: id, Kind: str(ev.Fault, "kind"), Targets: strs(ev.Fault["targets"]), Phase: map[string]int64{}}
				faults[id] = f
			}
			f.Events = append(f.Events, ev)
			ph := str(ev.Fault, "phase")
			if _, seen := f.Phase[ph]; !seen {
				f.Phase[ph] = ev.T
			}
		}
	}
	if len(inv) > 0 {
		return nil, fmt.Errorf("%d invokes without a completion", len(inv))
	}
	sort.Slice(m.Ops, func(a, b int) bool { return m.Ops[a].Inv < m.Ops[b].Inv })
	for _, id := range sortedKeys(faults) {
		f := faults[id]
		f.I = f.Events[0].T
		if t, ok := f.Phase["inject"]; ok {
			f.I = t
		}
		f.E = f.Events[len(f.Events)-1].T
		if t, ok := f.Phase["confirmed"]; ok {
			f.E = t
		}
		if t, ok := f.Phase["healed"]; ok {
			f.E = t
		}
		m.Faults = append(m.Faults, f)
	}
	sort.SliceStable(m.Faults, func(a, b int) bool { return m.Faults[a].I < m.Faults[b].I })
	m.Rules = observedRules(r.Observations)
	for _, ev := range r.Observations {
		v, _ := ev.Value.(map[string]any)
		src := str(v, "src")
		m.ObsBySrc[src]++
		node := str(v, "node")
		switch src {
		case "pooler-status":
			o := NodeObs{T: ev.T, OK: v["ok"] == true, WAL: -1, Latency: num(v["latency_ms"], -1)}
			switch {
			case !o.OK:
				o.State, o.Detail = "error", str(v, "error")
			case v["postgres_running"] == false:
				o.State, o.Detail = "down", str(v, "postgres_status")
			case str(v, "pooler_type") == "PRIMARY":
				o.State, o.Detail = "primary", str(v, "postgres_status")
			default:
				o.State, o.Detail = "standby", str(v, "postgres_status")
			}
			o.WAL = lsn(str(v, "wal_position"))
			m.Nodes[node] = append(m.Nodes[node], o)
		case "node-sql":
			o := NodeObs{T: ev.T, OK: v["ok"] == true, WAL: lsn(str(v, "lsn")), Latency: -1}
			switch {
			case !o.OK:
				o.State, o.Detail = "error", str(v, "error")
			case v["in_recovery"] == false:
				o.State, o.Detail = "primary", "not in recovery"
			default:
				o.State, o.Detail = "standby", "in recovery"
			}
			m.Nodes[node] = append(m.Nodes[node], o)
		case "cdc-slot":
			ready := v["ok"] == true && v["present"] == true && v["synced"] == true && v["temporary"] == false && v["invalidated"] == false
			if v["in_recovery"] == false {
				ready = v["ok"] == true && v["present"] == true && v["failover"] == true
			}
			m.Slots[node] = append(m.Slots[node], SlotObs{T: ev.T, Ready: ready, Detail: str(v, "raw") + " " + str(v, "error")})
		case "client":
			m.ClientObs = append(m.ClientObs, ClientObs{T: ev.T, Op: num(v["op"], -1), Message: str(v, "message")})
		}
	}
	return m, nil
}

// observedRules: a decided rule is observed by a pooler-status observation
// with ok true and a decision, or by a healthy observation with a rule
// (ADR-0036); identity (coordinator_term, leader_subterm), first_seen the
// least t reporting it, leader and cohort from that first observation.
func observedRules(obs []history.Event) []Rule {
	type key struct{ term, sub int64 }
	seen := map[key]*Rule{}
	add := func(t int64, by string, d map[string]any) {
		if d == nil {
			return
		}
		k := key{num(d["coordinator_term"], -1), num(d["leader_subterm"], -1)}
		if k.term < 0 || k.sub < 0 {
			return
		}
		if r, ok := seen[k]; ok && r.FirstSeen <= t {
			return
		}
		seen[k] = &Rule{Term: k.term, Sub: k.sub, Leader: str(d, "leader"), Cohort: strs(d["cohort"]), FirstSeen: t, SeenBy: by}
	}
	for _, ev := range obs {
		v, _ := ev.Value.(map[string]any)
		switch str(v, "src") {
		case "pooler-status":
			if v["ok"] == true {
				d, _ := v["decision"].(map[string]any)
				add(ev.T, str(v, "node"), d)
			}
		case "healthy":
			d, _ := v["rule"].(map[string]any)
			add(ev.T, "healthy", d)
		}
	}
	keys := make([]key, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		return keys[a].term < keys[b].term || keys[a].term == keys[b].term && keys[a].sub < keys[b].sub
	})
	out := make([]Rule, 0, len(keys))
	for i, k := range keys {
		r := *seen[k]
		if i > 0 {
			r.LeaderPrev = out[i-1].Leader
		}
		out = append(out, r)
	}
	return out
}

// Gateways returns the gateways of every operation except final reads,
// sorted (ADR-0036's set O).
func (m *Model) Gateways() []string {
	seen := map[string]bool{}
	for _, o := range m.Ops {
		if o.F != "final-read" && o.GW != "" {
			seen[o.GW] = true
		}
	}
	return sortedKeys(seen)
}

// Clients returns the clients with operations, ascending.
func (m *Model) Clients() []int64 {
	seen := map[int64]bool{}
	for _, o := range m.Ops {
		seen[o.Client] = true
	}
	out := make([]int64, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}

// lsn parses "X/Y" (hex) as X·2³² + Y; -1 if absent or malformed.
func lsn(s string) int64 {
	hi, lo, ok := strings.Cut(s, "/")
	if !ok {
		return -1
	}
	h, err1 := strconv.ParseUint(hi, 16, 32)
	l, err2 := strconv.ParseUint(lo, 16, 32)
	if err1 != nil || err2 != nil {
		return -1
	}
	return int64(h<<32 | l)
}

func str(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	switch v := m[k].(type) {
	case string:
		return v
	case nil:
		return ""
	case json.Number:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func strs(v any) []string {
	a, _ := v.([]any)
	out := make([]string, 0, len(a))
	for _, x := range a {
		out = append(out, fmt.Sprint(x))
	}
	return out
}

func num(v any, def int64) int64 {
	n, ok := v.(json.Number)
	if !ok {
		return def
	}
	x, err := strconv.ParseInt(string(n), 10, 64)
	if err != nil {
		return def
	}
	return x
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
