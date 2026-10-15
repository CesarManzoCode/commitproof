package gate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// K6's independent computation (CONTRACT §15 K6.3, K6.5): written from
// ADR-0036's definitions, not from package report, which cpgate does not
// import. Every value is an integer and is compared exactly.

// k6Op is an operation: an invoke paired with its completion.
type k6Op struct {
	inv, comp     int64
	tInv, tComp   int64
	gw, f, result string
}

// k6Fault is one fault id's nemesis events.
type k6Fault struct {
	id, kind string
	phases   map[string]int64 // first t per phase
	idx      map[int64]string // event index → phase
	first    int64
	last     int64
}

// inject and end as ADR-0036 defines I and E.
func (f *k6Fault) inject() int64 {
	if t, ok := f.phases["inject"]; ok {
		return t
	}
	return f.first
}

func (f *k6Fault) end() int64 {
	for _, ph := range []string{"healed", "confirmed"} {
		if t, ok := f.phases[ph]; ok {
			return t
		}
	}
	return f.last
}

type k6Rule struct {
	term, sub int64
	leader    string
	first     int64
}

func (r k6Rule) key() string { return fmt.Sprintf("%d.%d", r.term, r.sub) }

type k6Run struct {
	evs    []history.Event
	ops    []k6Op
	opOf   map[int64]k6Op // invoke or completion index → op
	faults map[string]*k6Fault
	rules  []k6Rule // by identity
}

func k6Num(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	x, err := strconv.ParseInt(string(n), 10, 64)
	return x, err == nil
}

func newK6Run(h, obs []history.Event) *k6Run {
	r := &k6Run{evs: h, opOf: map[int64]k6Op{}, faults: map[string]*k6Fault{}}
	pending := map[int64]history.Event{}
	for _, ev := range h {
		switch ev.Type {
		case history.TypeInvoke:
			pending[ev.I] = ev
		case history.TypeOK, history.TypeFail, history.TypeInfo:
			if ev.Op == nil {
				continue
			}
			in, ok := pending[*ev.Op]
			if !ok {
				continue
			}
			o := k6Op{inv: in.I, comp: ev.I, tInv: in.T, tComp: ev.T, gw: in.GW, f: in.F, result: ev.Type}
			r.ops = append(r.ops, o)
			r.opOf[o.inv], r.opOf[o.comp] = o, o
		case history.TypeNemesis:
			id := fmt.Sprint(ev.Fault["id"])
			f := r.faults[id]
			if f == nil {
				f = &k6Fault{id: id, kind: fmt.Sprint(ev.Fault["kind"]), phases: map[string]int64{}, idx: map[int64]string{}, first: ev.T}
				r.faults[id] = f
			}
			ph := fmt.Sprint(ev.Fault["phase"])
			if _, dup := f.phases[ph]; !dup {
				f.phases[ph] = ev.T
			}
			f.idx[ev.I] = ph
			f.last = ev.T
		}
	}
	// Rules (ADR-0036): pooler-status ok with a decision, or healthy with a
	// rule; first observation wins.
	byKey := map[[2]int64]k6Rule{}
	for _, ev := range obs {
		v, _ := ev.Value.(map[string]any)
		var d map[string]any
		switch v["src"] {
		case "pooler-status":
			if v["ok"] == true {
				d, _ = v["decision"].(map[string]any)
			}
		case "healthy":
			d, _ = v["rule"].(map[string]any)
		}
		if d == nil {
			continue
		}
		term, ok1 := k6Num(d["coordinator_term"])
		sub, ok2 := k6Num(d["leader_subterm"])
		if !ok1 || !ok2 || term < 0 || sub < 0 {
			continue
		}
		k := [2]int64{term, sub}
		if _, seen := byKey[k]; seen {
			continue // observations are in t order: the first is the earliest
		}
		leader, _ := d["leader"].(string)
		byKey[k] = k6Rule{term: term, sub: sub, leader: leader, first: ev.T}
	}
	for _, x := range byKey {
		r.rules = append(r.rules, x)
	}
	sort.Slice(r.rules, func(a, b int) bool {
		if r.rules[a].term != r.rules[b].term {
			return r.rules[a].term < r.rules[b].term
		}
		return r.rules[a].sub < r.rules[b].sub
	})
	return r
}

// leaderChanges: rules whose leader differs from the previous rule's in
// identity order.
func (r *k6Run) leaderChanges() map[string]string {
	out := map[string]string{}
	for i := 1; i < len(r.rules); i++ {
		if r.rules[i].leader != r.rules[i-1].leader {
			out[r.rules[i].key()] = r.rules[i].leader
		}
	}
	return out
}

// k6Expected computes the data block's "faults" entries per ADR-0036.
func (r *k6Run) expected() map[string]map[string]any {
	gwSet := map[string]bool{}
	var ops []k6Op
	for _, o := range r.ops {
		if o.f == "final-read" {
			continue
		}
		ops = append(ops, o)
		if o.gw != "" {
			gwSet[o.gw] = true
		}
	}
	lc := r.leaderChanges()
	tail := int64(params.FaultGap)
	lead := int64(1_000_000_000)
	out := map[string]map[string]any{}
	for id, f := range r.faults {
		I, E := f.inject(), f.end()
		gws := map[string]any{}
		for gw := range gwSet {
			// S: least completion ≥ I of a non-ok op through gw.
			S, R := int64(-1), int64(-1)
			var oks []int64
			nonOK := int64(0)
			for _, o := range ops {
				if o.gw != gw {
					continue
				}
				if o.result != history.TypeOK && o.tComp >= I && (S == -1 || o.tComp < S) {
					S = o.tComp
				}
				if o.tComp >= I-lead && o.tComp <= E+tail {
					if o.result == history.TypeOK {
						oks = append(oks, o.tComp)
					} else {
						nonOK++
					}
				}
			}
			if S != -1 {
				lo := S
				if E > lo {
					lo = E
				}
				for _, o := range ops {
					if o.gw == gw && o.result == history.TypeOK && o.tComp >= lo && (R == -1 || o.tComp < R) {
						R = o.tComp
					}
				}
			}
			bounds := append([]int64{I - lead}, oks...)
			bounds = append(bounds, E+tail)
			sort.Slice(bounds, func(a, b int) bool { return bounds[a] < bounds[b] })
			gap := int64(0)
			for k := 1; k < len(bounds); k++ {
				if d := bounds[k] - bounds[k-1]; d > gap {
					gap = d
				}
			}
			g := map[string]any{"gap_ns": gap, "non_ok": nonOK}
			switch {
			case S == -1:
				g["window"] = "none"
			case R == -1:
				g["window"], g["window_start"] = "open", S
			default:
				g["window"], g["window_start"], g["window_end"], g["window_ns"] = "window", S, R, R-S
			}
			gws[gw] = g
		}
		lat := map[string]any{}
		for _, ph := range []string{"before", "during", "after"} {
			var ls []int64
			for _, o := range ops {
				if o.result != history.TypeOK {
					continue
				}
				in := false
				switch ph {
				case "before":
					in = o.tInv >= I-tail && o.tInv < I
				case "during":
					in = o.tInv >= I && o.tInv <= E
				case "after":
					in = o.tInv > E && o.tInv <= E+tail
				}
				if in {
					ls = append(ls, o.tComp-o.tInv)
				}
			}
			sort.Slice(ls, func(a, b int) bool { return ls[a] < ls[b] })
			n := int64(len(ls))
			x := map[string]any{"n": n}
			if n > 0 {
				for _, q := range []int64{50, 95, 99} {
					rank := (q*n + 99) / 100 // ⌈q·n/100⌉, 1-based
					x["p"+strconv.FormatInt(q, 10)] = ls[rank-1]
				}
			}
			lat[ph] = x
		}
		next := int64(-1)
		for _, g := range r.faults {
			if gi := g.inject(); gi > I && (next == -1 || gi < next) {
				next = gi
			}
		}
		var rules []string
		leaders := int64(0)
		for _, ru := range r.rules {
			if ru.first >= I && (next == -1 || ru.first < next) {
				rules = append(rules, ru.key())
				if _, ok := lc[ru.key()]; ok {
					leaders++
				}
			}
		}
		out[id] = map[string]any{"kind": f.kind, "inject": I, "end": E, "gateways": gws, "latency": lat,
			"rule_changes": int64(len(rules)), "leader_changes": leaders, "rules": rules}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
