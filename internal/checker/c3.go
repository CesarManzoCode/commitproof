package checker

import (
	"fmt"
	"sort"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

// C3 list-append (CONTRACT §10.3, ADR-0004) is reimplemented from the Elle
// paper (Kingsbury & Alvaro, VLDB 2020, S71) and Adya's definitions (S72),
// without reading Elle's or tipocket's code.
//
// Model. Every append of v to key k creates a new version of k: the list of
// elements appended so far. A read returning [e1 … ei] observed the version
// written by the transaction that appended ei. The version order of k is
// the longest read of k; every other read must be a prefix of it.
//
// Edges between transactions (ok ones, and info ones whose appends were
// observed; failed transactions are never nodes):
//   - ww: writer(e_i) → writer(e_{i+1});
//   - wr: writer(e_i) → T if T read [e1 … ei];
//   - rw: T → writer(e_{i+1}) if T read [e1 … ei] (i may be 0: an empty or
//     null read);
//   - appends of committed transactions that no read observed lie after the
//     longest read (versions are prefixes of the final list), in an unknown
//     order: ww from the writer of its last element to each of their
//     writers, rw from each reader of the longest version to each of them;
//   - process: consecutive transactions of one process;
//   - realtime: T1 → T2 if T1 completed before T2 was invoked (transitively
//     reduced; info transactions never complete).
// A process or realtime edge is not added where a dependency edge already
// orders the same pair in the same direction.
//
// Cycle classes, by the edges of a simple cycle: G0 (ww only), G1c (ww and
// wr, at least one wr), G-single (exactly one rw), G-nonadjacent (at least
// two rw, no two consecutive), G2-item (at least two rw, some consecutive).
// The dependency graph is searched alone, then with process edges, then with
// realtime edges; a cycle that needs at least one process (realtime) edge is
// reported with the -process (-realtime) suffix. Searches run per strongly
// connected component with the P12 budget; an exhausted budget makes the
// result unknown.
//
// Non-cycle anomalies: duplicate-elements, incompatible-order, g1a (a read
// observes an element appended by a failed transaction), g1b (a read's last
// element is not the final append of its writer to that key), internal (a
// read inconsistent with the transaction's own earlier reads and appends),
// dirty-update (in the version order, an element of a failed transaction is
// followed by an element of a transaction that did not fail).

// Edge labels.
const (
	lWW uint8 = 1 << iota
	lWR
	lRW
	lPO
	lRT
	lDep = lWW | lWR | lRW
)

var labelName = map[uint8]string{lWW: "ww", lWR: "wr", lRW: "rw", lPO: "process", lRT: "realtime"}

type c3mop struct {
	append    bool
	k, v      int64
	read      []int64
	readKnown bool
}

type c3txn struct {
	op   *Op
	mops []c3mop
	node int
	// appends per key, in order.
	appends map[int64][]int64
}

// Forbidden classes per expected model (CONTRACT §10.3, G5).
var c3Forbidden = map[string]map[string]bool{
	"serializable": set("G0", "g1a", "g1b", "G1c", "G-single", "G-nonadjacent", "G2-item",
		"duplicate-elements", "incompatible-order", "internal", "dirty-update"),
	"repeatable-read": set("G0", "g1a", "g1b", "G1c", "G-single", "G-nonadjacent",
		"duplicate-elements", "incompatible-order", "internal", "dirty-update"),
}

func set(xs ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// c3Model returns the expected model of a W1 history: serializable, or
// repeatable-read if any transaction ran at that level ("" if some
// transaction used an isolation level without a §10.3 model).
func c3Model(txns []*c3txn) string {
	model := "serializable"
	for _, t := range txns {
		switch t.op.Iso {
		case "", "serializable":
		case "repeatable-read":
			model = "repeatable-read"
		default:
			return ""
		}
	}
	return model
}

// W1Txns returns the W1 operations C3 analyses: every append-workload op
// except final reads (CONTRACT §10.3 has no final-read input; C1 and C2
// cover final reads).
func W1Txns(h *History) []*Op { return h.opsOf("append") }

func parseC3(o *Op) (*c3txn, error) {
	src := o.InValue
	if o.Type == history.TypeOK {
		src = o.Value
	}
	mops, ok := src.([]any)
	if !ok {
		return nil, fmt.Errorf("i=%d: value is not a micro-op list", o.Index)
	}
	t := &c3txn{op: o, node: -1, appends: map[int64][]int64{}}
	for _, m := range mops {
		mo, _ := m.([]any)
		if len(mo) != 3 {
			return nil, fmt.Errorf("i=%d: bad micro-op %v", o.Index, m)
		}
		k, ok := Int(mo[1])
		if !ok {
			return nil, fmt.Errorf("i=%d: bad key in %v", o.Index, m)
		}
		switch mo[0] {
		case "append":
			v, ok := Int(mo[2])
			if !ok {
				return nil, fmt.Errorf("i=%d: bad append value in %v", o.Index, m)
			}
			t.mops = append(t.mops, c3mop{append: true, k: k, v: v})
			t.appends[k] = append(t.appends[k], v)
		case "r":
			mp := c3mop{k: k}
			if o.Type == history.TypeOK {
				mp.readKnown = true
				if mo[2] != nil {
					vals, ok := IntList(mo[2])
					if !ok {
						return nil, fmt.Errorf("i=%d: bad read result in %v", o.Index, m)
					}
					mp.read = vals
				}
			}
			t.mops = append(t.mops, mp)
		default:
			return nil, fmt.Errorf("i=%d: unknown micro-op %v", o.Index, mo[0])
		}
	}
	return t, nil
}

// c3graph is the transaction graph.
type c3graph struct {
	txns  []*c3txn // nodes, by node index
	adj   []map[int]uint8
	keys  map[[3]int]int64 // (from, to, label) → key, for explanations
	edges map[uint8]int64
}

func (g *c3graph) add(a, b int, l uint8, k int64) {
	if a == b {
		return
	}
	if g.adj[a][b]&l == 0 {
		g.edges[l]++
	}
	g.adj[a][b] |= l
	if _, ok := g.keys[[3]int{a, b, int(l)}]; !ok {
		g.keys[[3]int{a, b, int(l)}] = k
	}
}

// CheckListAppend is C3.
func CheckListAppend(h *History) Result {
	c := newCollector()
	stats := map[string]any{}
	var txns []*c3txn
	for _, o := range W1Txns(h) {
		t, err := parseC3(o)
		if err != nil {
			return c.result("C3", "malformed W1 operation: "+err.Error(), stats)
		}
		txns = append(txns, t)
	}
	stats["txns"] = int64(len(txns))
	model := c3Model(txns)
	if model == "" {
		return c.result("C3", "a W1 transaction uses an isolation level without a §10.3 model", stats)
	}
	stats["model"] = model
	if len(txns) == 0 {
		return c.result("C3", "", stats)
	}

	// Writers: (k, v) → transaction. Appended values are unique per key (P20);
	// a repeated append makes versions ambiguous.
	writer := map[kv]*c3txn{}
	for _, t := range txns {
		for _, m := range t.mops {
			if !m.append {
				continue
			}
			if w, dup := writer[kv{m.k, m.v}]; dup {
				return c.result("C3", fmt.Sprintf("value %d appended to key %d twice (i=%d and i=%d); versions are ambiguous", m.v, m.k, w.op.Index, t.op.Index), stats)
			}
			writer[kv{m.k, m.v}] = t
		}
	}

	findings := map[string][]Anomaly{}
	note := func(class string, ops []int64, f string, a ...any) {
		findings[class] = append(findings[class], Anomaly{Class: class, Ops: sortedOps(ops), Explanation: fmt.Sprintf(f, a...)})
	}

	// Reads by key (ok transactions only), non-cycle anomalies.
	type rd struct {
		t    *c3txn
		vals []int64
	}
	readsByKey := map[int64][]rd{}
	dupKeys := map[int64]bool{}
	observed := map[*c3txn]bool{}
	for _, t := range txns {
		if t.op.Type != history.TypeOK {
			continue
		}
		c3Internal(t, note)
		for _, m := range t.mops {
			if m.append || !m.readKnown {
				continue
			}
			readsByKey[m.k] = append(readsByKey[m.k], rd{t, m.read})
			seen := map[int64]bool{}
			for _, v := range m.read {
				if seen[v] && !sabotageDuplicateDetection {
					note("duplicate-elements", []int64{t.op.Index}, "read of key %d by i=%d contains %d more than once: %v", m.k, t.op.Index, v, m.read)
					dupKeys[m.k] = true
				}
				seen[v] = true
				w := writer[kv{m.k, v}]
				if w == nil {
					continue
				}
				if w.op.Type == history.TypeFail {
					if !sabotageG1a {
						note("g1a", []int64{w.op.Index, t.op.Index}, "read of key %d by i=%d observes %d, appended by failed i=%d", m.k, t.op.Index, v, w.op.Index)
					}
				} else if w != t {
					observed[w] = true
				}
			}
			if n := len(m.read); n > 0 {
				last := m.read[n-1]
				if w := writer[kv{m.k, last}]; w != nil && w != t && w.op.Type != history.TypeFail {
					aps := w.appends[m.k]
					if aps[len(aps)-1] != last {
						note("g1b", []int64{w.op.Index, t.op.Index}, "read of key %d by i=%d ends at %d, an intermediate append of i=%d (which appended %v)", m.k, t.op.Index, last, w.op.Index, aps)
					}
				}
			}
		}
	}

	// Version orders.
	orders := map[int64][]int64{}
	for _, k := range sortedKeys(readsByKey) {
		if dupKeys[k] {
			continue
		}
		rs := readsByKey[k]
		longest := rs[0]
		for _, r := range rs[1:] {
			if len(r.vals) > len(longest.vals) {
				longest = r
			}
		}
		compatible := true
		if !sabotagePrefixCheck {
			for _, r := range rs {
				if !isPrefix(r.vals, longest.vals) {
					note("incompatible-order", []int64{r.t.op.Index, longest.t.op.Index}, "reads of key %d are not prefixes of each other: %v (i=%d) and %v (i=%d)", k, r.vals, r.t.op.Index, longest.vals, longest.t.op.Index)
					compatible = false
				}
			}
		}
		if compatible {
			orders[k] = longest.vals
		}
	}
	for _, k := range sortedKeys(orders) {
		ord := orders[k]
		for i := 0; i+1 < len(ord); i++ {
			a, b := writer[kv{k, ord[i]}], writer[kv{k, ord[i+1]}]
			if a != nil && b != nil && a != b && a.op.Type == history.TypeFail && b.op.Type != history.TypeFail {
				note("dirty-update", []int64{a.op.Index, b.op.Index}, "in the version order of key %d, %d of i=%d (%s) follows %d of failed i=%d", k, ord[i+1], b.op.Index, b.op.Type, ord[i], a.op.Index)
			}
		}
	}

	// Nodes.
	g := &c3graph{keys: map[[3]int]int64{}, edges: map[uint8]int64{}}
	for _, t := range txns {
		if t.op.Type == history.TypeOK || (t.op.Type == history.TypeInfo && observed[t]) {
			t.node = len(g.txns)
			g.txns = append(g.txns, t)
			g.adj = append(g.adj, map[int]uint8{})
		}
	}
	stats["nodes"] = int64(len(g.txns))
	isNode := func(t *c3txn) bool { return t != nil && t.node >= 0 }

	// Dependency edges.
	for _, k := range sortedKeys(orders) {
		ord := orders[k]
		ws := make([]*c3txn, len(ord))
		for i, v := range ord {
			ws[i] = writer[kv{k, v}]
		}
		for i := 0; i+1 < len(ws); i++ {
			if isNode(ws[i]) && isNode(ws[i+1]) {
				g.add(ws[i].node, ws[i+1].node, lWW, k)
			}
		}
		for _, r := range readsByKey[k] {
			i := len(r.vals)
			if i > 0 && isNode(ws[i-1]) {
				g.add(ws[i-1].node, r.t.node, lWR, k)
			}
			if i < len(ws) && isNode(ws[i]) && !sabotageRWEdge {
				g.add(r.t.node, ws[i].node, lRW, k)
			}
		}
		// Committed appends no read observed lie after the longest read (every
		// version is a prefix of the final list), in an unknown order: they
		// follow the last observed writer (ww) and every reader of the longest
		// version precedes them (rw).
		inOrder := map[int64]bool{}
		for _, v := range ord {
			inOrder[v] = true
		}
		var unobserved []*c3txn
		for _, t := range g.txns {
			for _, v := range t.appends[k] {
				if !inOrder[v] {
					unobserved = append(unobserved, t)
					break
				}
			}
		}
		for _, u := range unobserved {
			if n := len(ws); n > 0 && isNode(ws[n-1]) {
				g.add(ws[n-1].node, u.node, lWW, k)
			}
			for _, r := range readsByKey[k] {
				if len(r.vals) == len(ord) && !sabotageRWEdge {
					g.add(r.t.node, u.node, lRW, k)
				}
			}
		}
	}
	// Process edges: consecutive nodes of each process.
	byProc := map[int64][]int{}
	for n, t := range g.txns {
		byProc[t.op.Proc] = append(byProc[t.op.Proc], n)
	}
	for _, p := range sortedKeys(byProc) {
		ns := byProc[p]
		sort.SliceStable(ns, func(i, j int) bool { return g.txns[ns[i]].op.Invoke < g.txns[ns[j]].op.Invoke })
		for i := 0; i+1 < len(ns); i++ {
			if g.adj[ns[i]][ns[i+1]]&lDep == 0 {
				g.add(ns[i], ns[i+1], lPO, -1)
			}
		}
	}
	// Realtime edges, transitively reduced: an invoked node's predecessors are
	// the completed nodes not yet followed by another completed node.
	type ev struct {
		t    int64
		kind int // 0 invoke, 1 complete: at equal times intervals overlap
		n    int
	}
	var evs []ev
	for n, t := range g.txns {
		evs = append(evs, ev{t.op.Invoke, 0, n})
		if t.op.Type == history.TypeOK {
			evs = append(evs, ev{t.op.Complete, 1, n})
		}
	}
	sort.Slice(evs, func(i, j int) bool {
		if evs[i].t != evs[j].t {
			return evs[i].t < evs[j].t
		}
		if evs[i].kind != evs[j].kind {
			return evs[i].kind < evs[j].kind
		}
		return evs[i].n < evs[j].n
	})
	frontier := map[int]bool{}
	preds := map[int][]int{}
	for _, e := range evs {
		if e.kind == 0 {
			for _, p := range sortedIntKeys(frontier) {
				preds[e.n] = append(preds[e.n], p)
				if g.adj[p][e.n]&lDep == 0 {
					g.add(p, e.n, lRT, -1)
				}
			}
			continue
		}
		for _, p := range preds[e.n] {
			delete(frontier, p)
		}
		frontier[e.n] = true
	}
	edgeStats := map[string]any{}
	for l, n := range g.edges {
		edgeStats[labelName[l]] = n
	}
	stats["edges"] = edgeStats

	unknown := c3Cycles(g, note, stats)

	// Split by the expected model.
	forbidden := c3Forbidden[model]
	classes := make([]string, 0, len(findings))
	for cl := range findings {
		classes = append(classes, cl)
	}
	sort.Strings(classes)
	for _, cl := range classes {
		for _, a := range findings[cl] {
			if forbidden[cl] {
				c.add(cl, a.Ops, "%s", a.Explanation)
			} else {
				c.observe(cl, a.Ops, "%s", a.Explanation)
			}
		}
	}
	return c.result("C3", unknown, stats)
}

// c3Internal checks a transaction's reads against its own earlier reads and
// appends: after a read of k the state of k is known; an append extends the
// known state, or the known suffix if k was not read yet; a later read must
// equal the known state, or end with the known suffix.
func c3Internal(t *c3txn, note func(string, []int64, string, ...any)) {
	type st struct {
		known  bool
		list   []int64
		suffix []int64
	}
	state := map[int64]*st{}
	for _, m := range t.mops {
		s := state[m.k]
		if s == nil {
			s = &st{}
			state[m.k] = s
		}
		if m.append {
			if s.known {
				s.list = append(append([]int64{}, s.list...), m.v)
			} else {
				s.suffix = append(append([]int64{}, s.suffix...), m.v)
			}
			continue
		}
		if !m.readKnown {
			continue
		}
		switch {
		case s.known && !equalInts(m.read, s.list):
			note("internal", []int64{t.op.Index}, "i=%d read key %d as %v after its own operations implied %v", t.op.Index, m.k, m.read, s.list)
		case !s.known && len(s.suffix) > 0 && !hasSuffix(m.read, s.suffix):
			note("internal", []int64{t.op.Index}, "i=%d read key %d as %v, which does not end with its own appends %v", t.op.Index, m.k, m.read, s.suffix)
		}
		s.known, s.list, s.suffix = true, append([]int64{}, m.read...), nil
	}
}

func isPrefix(a, b []int64) bool {
	if len(a) > len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasSuffix(a, s []int64) bool {
	if len(s) > len(a) {
		return false
	}
	off := len(a) - len(s)
	for i := range s {
		if a[off+i] != s[i] {
			return false
		}
	}
	return true
}

func equalInts(a, b []int64) bool { return len(a) == len(b) && isPrefix(a, b) }

func sortedIntKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
