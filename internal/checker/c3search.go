package checker

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

// cycleBudget is P12, the cycle search budget per strongly connected
// component.
var cycleBudget = params.CycleSearchBudget

// Cycle shapes.
const (
	shG0 = iota
	shG1c
	shGSingle
	shGNonadjacent
	shG2Item
)

var shapeName = []string{"G0", "G1c", "G-single", "G-nonadjacent", "G2-item"}

// variant of the graph searched: the dependency graph alone, or with process
// or realtime edges (the order label must then appear in the cycle).
type variant struct {
	suffix string
	order  uint8 // 0, lPO or lRT
}

var variants = []variant{{"", 0}, {"-process", lPO}, {"-realtime", lRT}}

// search state bits.
const (
	stUsedO  uint8 = 1
	stUsedR  uint8 = 2
	stLastRW uint8 = 4
	stRW1    uint8 = 8  // at least one rw
	stRW2    uint8 = 16 // at least two rw
)

// c3Cycles searches every variant and shape, recording one witness per shape
// per SCC. It returns a non-empty reason if some SCC exhausted its budget.
func c3Cycles(g *c3graph, note func(string, []int64, string, ...any), stats map[string]any) string {
	var unknown []string
	sccStats := map[string]any{}
	for _, v := range variants {
		allowed := lDep | v.order
		sccs := tarjan(len(g.adj), func(n int, f func(int)) {
			for _, to := range sortedAdj(g.adj[n]) {
				if g.adj[n][to]&allowed != 0 {
					f(to)
				}
			}
		})
		nontrivial := 0
		for _, scc := range sccs {
			if len(scc) < 2 {
				continue
			}
			nontrivial++
			s := &searcher{g: g, v: v, allowed: allowed, in: map[int]bool{}, deadline: time.Now().Add(cycleBudget)}
			for _, n := range scc {
				s.in[n] = true
			}
			nodes := append([]int{}, scc...)
			sort.Ints(nodes)
			s.nodes = nodes
			for sh := range shapeName {
				cyc, labels, err := s.find(sh)
				if err != nil {
					unknown = append(unknown, fmt.Sprintf("%s search in an SCC of %d transactions: %v", shapeName[sh]+v.suffix, len(scc), err))
					break
				}
				if cyc != nil {
					var ops []int64
					for _, n := range cyc {
						ops = append(ops, g.txns[n].op.Index)
					}
					note(shapeName[sh]+v.suffix, ops, "cycle %s", g.explain(cyc, labels))
				}
			}
		}
		name := "dependency"
		if v.suffix != "" {
			name = v.suffix[1:]
		}
		sccStats[name] = int64(nontrivial)
	}
	stats["nontrivial_sccs"] = sccStats
	return strings.Join(unknown, "; ")
}

func (g *c3graph) explain(cyc []int, labels []uint8) string {
	var b strings.Builder
	for i, n := range cyc {
		next := cyc[(i+1)%len(cyc)]
		fmt.Fprintf(&b, "i=%d -%s", g.txns[n].op.Index, labelName[labels[i]])
		if k, ok := g.keys[[3]int{n, next, int(labels[i])}]; ok && k >= 0 {
			fmt.Fprintf(&b, "(k=%d)", k)
		}
		b.WriteString("-> ")
	}
	fmt.Fprintf(&b, "i=%d", g.txns[cyc[0]].op.Index)
	return b.String()
}

func sortedAdj(m map[int]uint8) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// tarjan returns the strongly connected components (iterative).
func tarjan(n int, succ func(int, func(int))) [][]int {
	index := make([]int, n)
	low := make([]int, n)
	on := make([]bool, n)
	for i := range index {
		index[i] = -1
	}
	var out [][]int
	var st []int
	next := 0
	type frame struct {
		v    int
		succ []int
		i    int
	}
	for root := 0; root < n; root++ {
		if index[root] >= 0 {
			continue
		}
		var call []frame
		push := func(v int) {
			index[v], low[v] = next, next
			next++
			st = append(st, v)
			on[v] = true
			var ss []int
			succ(v, func(w int) { ss = append(ss, w) })
			call = append(call, frame{v: v, succ: ss})
		}
		push(root)
		for len(call) > 0 {
			f := &call[len(call)-1]
			if f.i < len(f.succ) {
				w := f.succ[f.i]
				f.i++
				if index[w] < 0 {
					push(w)
				} else if on[w] {
					low[f.v] = min(low[f.v], index[w])
				}
				continue
			}
			v := f.v
			call = call[:len(call)-1]
			if len(call) > 0 {
				p := call[len(call)-1].v
				low[p] = min(low[p], low[v])
			}
			if low[v] == index[v] {
				var comp []int
				for {
					w := st[len(st)-1]
					st = st[:len(st)-1]
					on[w] = false
					comp = append(comp, w)
					if w == v {
						break
					}
				}
				out = append(out, comp)
			}
		}
	}
	return out
}

// searcher finds cycles of one shape within one SCC of one variant.
type searcher struct {
	g        *c3graph
	v        variant
	allowed  uint8
	in       map[int]bool
	nodes    []int
	deadline time.Time
	ticks    int
}

var errBudget = fmt.Errorf("cycle search budget P12 (%s) exhausted", params.CycleSearchBudget)

func (s *searcher) expired() bool {
	s.ticks++
	return s.ticks%1024 == 0 && time.Now().After(s.deadline)
}

// stepLabels returns the labels a cycle of shape sh may use after its
// anchor.
func (s *searcher) stepLabels(sh int) uint8 {
	switch sh {
	case shG0:
		return lWW | s.v.order
	case shG1c, shGSingle:
		return lWW | lWR | s.v.order
	}
	return lDep | s.v.order
}

// anchor is the first edge(s) of a cycle: a path u → … → last.
type anchor struct {
	path   []int
	labels []uint8
}

// anchors lists the anchor paths of shape sh in the SCC. The anchor carries
// the label the shape needs at least once, so the search only has to close
// the cycle: G0 starts with ww (base) or the order edge; G1c with wr (base)
// or the order edge; G-single and G-nonadjacent with rw; G2-item with two
// consecutive rw edges.
func (s *searcher) anchors(sh int) []anchor {
	var out []anchor
	first := lRW
	switch sh {
	case shG0:
		first = lWW
	case shG1c:
		first = lWR
	}
	if s.v.order != 0 && (sh == shG0 || sh == shG1c) {
		first = s.v.order
	}
	for _, u := range s.nodes {
		for _, v := range sortedAdj(s.g.adj[u]) {
			if !s.in[v] || s.g.adj[u][v]&first == 0 {
				continue
			}
			if sh != shG2Item {
				out = append(out, anchor{[]int{u, v}, []uint8{first}})
				continue
			}
			for _, w := range sortedAdj(s.g.adj[v]) {
				if s.in[w] && s.g.adj[v][w]&lRW != 0 && w != v {
					out = append(out, anchor{[]int{u, v, w}, []uint8{lRW, lRW}})
				}
			}
		}
	}
	return out
}

func (s *searcher) initState(a anchor) uint8 {
	var st uint8
	for _, l := range a.labels {
		st = s.trans(st, l)
	}
	return st
}

func (s *searcher) trans(st, l uint8) uint8 {
	if l == s.v.order && l != 0 {
		st |= stUsedO
	}
	if l == lWR {
		st |= stUsedR
	}
	if l == lRW {
		if st&stRW1 != 0 {
			st |= stRW2
		}
		st |= stRW1 | stLastRW
	} else {
		st &^= stLastRW
	}
	return st
}

// legal reports whether label l may follow state st for shape sh.
func (s *searcher) legal(sh int, st, l uint8) bool {
	if sh == shGNonadjacent && l == lRW && st&stLastRW != 0 {
		return false
	}
	return true
}

// accept reports whether a walk closing at the anchor's start in state st
// (after the closing step) forms a cycle of shape sh. The closing step is
// followed by the anchor's first edge, so for G-nonadjacent the closing step
// must not be rw.
func (s *searcher) accept(sh int, st uint8) bool {
	if s.v.order != 0 && st&stUsedO == 0 {
		return false
	}
	switch sh {
	case shG1c:
		return st&stUsedR != 0
	case shGNonadjacent:
		return st&stRW2 != 0 && st&stLastRW == 0
	}
	return true
}

// find returns a simple cycle of shape sh (nodes and the label of each edge,
// starting at the anchor's first node), nil if none exists, or errBudget.
func (s *searcher) find(sh int) ([]int, []uint8, error) {
	anchors := s.anchors(sh)
	nonSimple := false
	labels := s.stepLabels(sh)
	for _, a := range anchors {
		if s.expired() {
			return nil, nil, errBudget
		}
		u, last := a.path[0], a.path[len(a.path)-1]
		st0 := s.initState(a)
		if last == u { // G2-item 2-cycle: u -rw-> v -rw-> u
			if s.accept(sh, st0) {
				return a.path[:len(a.path)-1], a.labels, nil
			}
			continue
		}
		avoid := map[int]bool{}
		for _, n := range a.path[1 : len(a.path)-1] {
			avoid[n] = true
		}
		path, pl, err := s.bfs(sh, last, u, st0, labels, avoid)
		if err != nil {
			return nil, nil, err
		}
		if path == nil {
			continue
		}
		cyc := append(append([]int{}, a.path[:len(a.path)-1]...), path[:len(path)-1]...)
		cl := append(append([]uint8{}, a.labels...), pl...)
		if simple(cyc) {
			return cyc, cl, nil
		}
		nonSimple = true
	}
	if !nonSimple {
		return nil, nil, nil
	}
	// Only non-simple closed walks were found: enumerate simple paths.
	for _, a := range anchors {
		u, last := a.path[0], a.path[len(a.path)-1]
		if last == u {
			continue
		}
		visited := map[int]bool{}
		for _, n := range a.path {
			visited[n] = true
		}
		visited[u] = false
		path, pl, err := s.dfs(sh, last, u, s.initState(a), labels, visited)
		if err != nil {
			return nil, nil, err
		}
		if path != nil {
			cyc := append(append([]int{}, a.path[:len(a.path)-1]...), path[:len(path)-1]...)
			return cyc, append(append([]uint8{}, a.labels...), pl...), nil
		}
	}
	return nil, nil, nil
}

// bfs searches the product of the SCC and the shape's state for a walk from
// start to target; it returns the nodes after start (ending with target)
// and their labels.
func (s *searcher) bfs(sh, start, target int, st0, labels uint8, avoid map[int]bool) ([]int, []uint8, error) {
	type key struct {
		n  int
		st uint8
	}
	type par struct {
		prev key
		l    uint8
	}
	parent := map[key]par{}
	k0 := key{start, st0}
	parent[k0] = par{prev: key{-1, 0}}
	queue := []key{k0}
	for len(queue) > 0 {
		if s.expired() {
			return nil, nil, errBudget
		}
		cur := queue[0]
		queue = queue[1:]
		for _, to := range sortedAdj(s.g.adj[cur.n]) {
			if !s.in[to] || avoid[to] {
				continue
			}
			ls := s.g.adj[cur.n][to] & labels
			for _, l := range []uint8{lWW, lWR, lRW, lPO, lRT} {
				if ls&l == 0 || !s.legal(sh, cur.st, l) {
					continue
				}
				nst := s.trans(cur.st, l)
				nk := key{to, nst}
				if to == target {
					if !s.accept(sh, nst) {
						continue
					}
					parent[nk] = par{cur, l}
					var nodes []int
					var ls []uint8
					for k := nk; k != k0; k = parent[k].prev {
						nodes = append([]int{k.n}, nodes...)
						ls = append([]uint8{parent[k].l}, ls...)
					}
					return nodes, ls, nil
				}
				if _, seen := parent[nk]; seen {
					continue
				}
				parent[nk] = par{cur, l}
				queue = append(queue, nk)
			}
		}
	}
	return nil, nil, nil
}

// dfs enumerates simple paths from cur to target (exponential; bounded by
// the budget).
func (s *searcher) dfs(sh, cur, target int, st, labels uint8, visited map[int]bool) ([]int, []uint8, error) {
	if s.expired() {
		return nil, nil, errBudget
	}
	for _, to := range sortedAdj(s.g.adj[cur]) {
		if !s.in[to] || (visited[to] && to != target) {
			continue
		}
		ls := s.g.adj[cur][to] & labels
		for _, l := range []uint8{lWW, lWR, lRW, lPO, lRT} {
			if ls&l == 0 || !s.legal(sh, st, l) {
				continue
			}
			nst := s.trans(st, l)
			if to == target {
				if s.accept(sh, nst) {
					return []int{to}, []uint8{l}, nil
				}
				continue
			}
			visited[to] = true
			p, pl, err := s.dfs(sh, to, target, nst, labels, visited)
			visited[to] = false
			if err != nil || p != nil {
				return append([]int{to}, p...), append([]uint8{l}, pl...), err
			}
		}
	}
	return nil, nil, nil
}

func simple(cyc []int) bool {
	seen := map[int]bool{}
	for _, n := range cyc {
		if seen[n] {
			return false
		}
		seen[n] = true
	}
	return true
}
