package synth

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

// K7 corpus (ADR-0037, the owner's decision on ADR-0035):
// testdata/corpus-k7. G12's fair share judged outside fault windows, for
// plans of contract 1.2.0 or later, in the forms the K5 clients write (W8
// storms, cp_steady ledger inserts with their role and server). Every
// expectation is written per plant from ADR-0037's rule — never from
// running the checkers. The same fault-overlapped storms also carry the K5
// corpus's contract version (label g12-literal), which the literal rule
// of contract ≤ 1.1.1 judges.

// K7CorpusContractVersion is the contract version of the judged entries.
const K7CorpusContractVersion = "1.2.0"

// K7 corpus labels besides LabelClean and the anomaly classes.
const (
	LabelFaultOverlap = "overload-fault-overlap" // judged rule: valid, observation only
	LabelG12Literal   = "g12-literal"            // the same storms under the literal rule
	LabelNotJudged    = "fair-share-not-judged"  // C8 unknown
)

// K7Corpus returns every entry of testdata/corpus-k7, in id order.
func K7Corpus() []Entry {
	var es []Entry
	add := func(id string, f func() *Run) { es = append(es, Entry{id, f}) }
	for n := 1; n <= 15; n++ {
		id := fmt.Sprintf("w8g-clean-%03d", n)
		add(id, func() *Run {
			return w8gRun(id, LabelClean, []string{"none", "fault-between"}[n%2], K7CorpusContractVersion)
		})
	}
	for n := 1; n <= 4; n++ {
		id := fmt.Sprintf("w8g-fault-overlap-%02d", n)
		add(id, func() *Run { return w8gRun(id, LabelFaultOverlap, "fault-overlap", K7CorpusContractVersion) })
		idl := fmt.Sprintf("w8g-literal-%02d", n)
		add(idl, func() *Run { return w8gRun(idl, LabelG12Literal, "fault-overlap", K5CorpusContractVersion) })
		v := []string{"starvation", "starvation-fault-same", "starvation-fault-other", "starvation"}[n-1]
		ids := fmt.Sprintf("w8g-%s-%02d", v, n)
		add(ids, func() *Run { return w8gRun(ids, "fair-share-starvation", v, K7CorpusContractVersion) })
	}
	for n := 1; n <= 4; n++ {
		v := []string{"not-judged-empty", "not-judged-sparse"}[n%2]
		id := fmt.Sprintf("w8g-%s-%02d", v, n)
		add(id, func() *Run { return w8gRun(id, LabelNotJudged, v, K7CorpusContractVersion) })
	}
	sort.Slice(es, func(i, j int) bool { return es[i].ID < es[j].ID })
	return es
}

// w8gRun builds one W8 history: twelve cp_storm clients in storm phases,
// two cp_steady clients (8, 9) inserting W4 rows that return the executing
// server's identity, and at most one F2a fault window. Inside a fault
// window every cp_steady insert invoked fails (the connection is refused
// while PostgreSQL restarts), as in C11 seed 2 (ADR-0035).
//
// Variants (storm windows are first storm invoke to last storm completion,
// ⊂ [phase start, phase end + 150 ms]):
//   - none: no fault.
//   - fault-between: a fault between the two storm windows.
//   - fault-overlap: a 300 ms fault inside storm window 1; about a quarter
//     of its cp_steady operations fail inside it (literal fraction < 90 %),
//     all outside it complete ok.
//   - starvation: in storm window 1 the pool refuses every other cp_steady
//     operation invoked in a 600 ms span, with no fault.
//   - starvation-fault-same: starvation as above and a fault inside the
//     same storm window, after the refusals.
//   - starvation-fault-other: starvation in window 1 and a fault inside
//     storm window 2.
//   - not-judged-empty: a fault spanning all of storm window 1.
//   - not-judged-sparse: one long storm phase (5 s) and a 2.6 s fault in
//     it; outside the fault only two cp_steady operations are invoked, both
//     ok and slow (fewer than one per second of the ≈ 2.5 s outside it).
func w8gRun(id, label, variant, version string) *Run {
	r := NewRun(id, "w8g", label)
	r.Version = version
	r.Workloads = []string{"W8"}
	r.Plant = variant
	phases := [][2]int64{{r.T0 + 100*ms, r.T0 + 1100*ms}, {r.T0 + 1600*ms, r.T0 + 2600*ms}}
	end := r.T0 + 2900*ms
	if variant == "not-judged-sparse" {
		phases = [][2]int64{{r.T0 + 100*ms, r.T0 + 5100*ms}}
		end = r.T0 + 5600*ms
	}
	for k := int64(0); k < 12; k++ {
		c := 100 + k
		for p, w := range phases {
			conn := c*1000 + int64(p+1)
			for t := w[0] + k*us; t < w[1]-300*ms; {
				in := map[string]any{"role": "cp_storm", "phase": int64(p + 1), "conn": conn}
				o := r.k5op(c, Gateways[k%3], "overload", "storm", in, in, t, 300*ms+int64(r.R.IntN(150))*ms)
				t = o.Cmp + us
			}
		}
	}
	p1, p2 := phases[0][0], int64(0)
	if len(phases) > 1 {
		p2 = phases[1][0]
	}
	var fault [2]int64
	switch variant {
	case "fault-between":
		fault = [2]int64{r.T0 + 1350*ms, r.T0 + 1500*ms}
	case "fault-overlap":
		fault = [2]int64{p1 + 300*ms, p1 + 600*ms}
	case "starvation-fault-same":
		fault = [2]int64{p1 + 850*ms, p1 + 1000*ms}
	case "starvation-fault-other":
		fault = [2]int64{p2 + 300*ms, p2 + 450*ms}
	case "not-judged-empty":
		fault = [2]int64{r.T0 + 50*ms, r.T0 + 1300*ms}
	case "not-judged-sparse":
		fault = [2]int64{p1 + 900*ms, p1 + 3500*ms}
	}
	if fault[0] > 0 {
		r.k5Fault("f0", "F2a", "pg-n1", fault[0], fault[1])
	}
	inFault := func(t int64) bool { return fault[0] > 0 && t >= fault[0] && t <= fault[1] }
	// server is the executing PostgreSQL's identity at t: F2a restarts it.
	server := func(t int64) string {
		if fault[0] > 0 && t > fault[1] {
			return "2026-10-13 01:41:15.120000+00"
		}
		return "2026-10-13 01:41:12.508509+00"
	}
	nid := int64(0)
	steady := func(o *Op, c int64) {
		nid++
		in := map[string]any{"id": nid, "payload": fmt.Sprintf("c%d-%016x", c, r.R.Uint64()), "role": "cp_steady"}
		o.F, o.In = "insert", in
		o.Exec = func(s *Store, at int64) any {
			m := s.insert(at, o, in)
			m["role"], m["server"] = "cp_steady", server(at)
			return m
		}
	}
	refused := 0
	if variant == "not-judged-sparse" {
		// Client 8: a slow ok insert before the fault, refused inserts every
		// 50 ms inside it, a slow ok insert after it that outlasts the
		// storm. Client 9: refused inserts inside the fault only.
		slow := func(inv, cmp int64) {
			o := r.k5op(8, Gateways[8%3], "ledger", "insert", nil, nil, inv, cmp-inv)
			steady(o, 8)
		}
		slow(p1+150*ms, p1+850*ms)
		for _, c := range []int64{8, 9} {
			r.Loop(c, Gateways[c%3], "ledger", fault[0]+10*ms, fault[1]-10*ms, 1*ms, 6*ms, func(o *Op) bool {
				steady(o, c)
				o.Fail("57P03", "57", "connect")
				refused++
				return true
			})
		}
		slow(fault[1]+50*ms, end)
	} else {
		starving := 0
		for _, c := range []int64{8, 9} {
			r.Loop(c, Gateways[c%3], "ledger", r.T0, end, 1*ms, 6*ms, func(o *Op) bool {
				steady(o, c)
				switch {
				case inFault(o.Inv):
					o.Fail("57P03", "57", "connect")
					refused++
				case (variant == "starvation" || variant == "starvation-fault-same" || variant == "starvation-fault-other") &&
					o.Inv > p1+200*ms && o.Inv < p1+800*ms && (o.Inv/(50*ms))%2 == 0:
					o.Fail("53300", "53", "connect") // the pool refuses cp_steady
					starving++
				}
				return true
			})
		}
		must(!strings.HasPrefix(variant, "starvation") || starving >= 8, "%s: %d refusals planted", id, starving)
	}
	must(fault[0] == 0 || variant == "fault-between" || refused > 0, "%s: no operation inside the fault", id)
	r.w8gPremise(variant, fault)
	switch {
	case version != K7CorpusContractVersion:
		// The literal rule (contract ≤ 1.1.1): every cp_steady operation of
		// the storm window counts.
		r.Expect("C8", "fair-share-starvation")
	case variant == "fault-overlap":
		r.Observe("C8", "overload-fault-overlap")
	case variant == "starvation" || variant == "starvation-fault-same" || variant == "starvation-fault-other":
		r.Expect("C8", "fair-share-starvation")
		if variant != "starvation" {
			r.Observe("C8", "overload-fault-overlap")
		}
	case variant == "not-judged-empty" || variant == "not-judged-sparse":
		r.Expected["C8"].Valid = "unknown"
		r.Observe("C8", "overload-fault-overlap", "fair-share-not-judged")
	}
	return r
}

// w8gPremise asserts what each variant's expectation presumes about storm
// window 1 (the first storm invoke to the last storm completion of phase
// 1): L its cp_steady inserts by invoke time, J those invoked outside the
// fault, S its time outside the fault. It restates the plant, not the
// checker: a generator change that broke a premise stops the generation.
func (r *Run) w8gPremise(variant string, fault [2]int64) {
	win := map[int64][2]int64{}
	for _, o := range r.Ops {
		if o.WL != "overload" {
			continue
		}
		p, _ := o.In.(map[string]any)["phase"].(int64)
		w, ok := win[p]
		if !ok {
			w = [2]int64{o.Inv, o.Cmp}
		}
		win[p] = [2]int64{min(w[0], o.Inv), max(w[1], o.Cmp)}
	}
	in := func(t int64) bool { return fault[0] > 0 && t >= fault[0] && t <= fault[1] }
	count := func(w [2]int64) (lit, litOK, jud, judOK, outside int64) {
		for _, o := range r.Ops {
			if o.WL != "ledger" || o.Inv < w[0] || o.Inv > w[1] {
				continue
			}
			ok := o.Type == history.TypeOK
			lit++
			if ok {
				litOK++
			}
			if !in(o.Inv) {
				jud++
				if ok {
					judOK++
				}
			}
		}
		outside = w[1] - w[0]
		if a, b := max(w[0], fault[0]), min(w[1], fault[1]); fault[0] > 0 && a <= b {
			outside -= b - a
		}
		return
	}
	sec := int64(time.Second)
	lit, litOK, jud, judOK, outside := count(win[1])
	switch {
	case variant == "none" || variant == "fault-between":
		for p, w := range win {
			l, lo, _, _, _ := count(w)
			must(l > 0 && lo == l && (fault[0] == 0 || fault[1] < w[0] || fault[0] > w[1]), "%s: storm window %d is not fault-free and fair", r.ID, p)
		}
	case variant == "fault-overlap":
		must(litOK*10 < lit*9 && judOK == jud && jud*sec >= outside, "%s: literal %d/%d, judged %d/%d over %d ns", r.ID, litOK, lit, judOK, jud, outside)
	case strings.HasPrefix(variant, "starvation"):
		must(jud > 0 && judOK*10 < jud*9, "%s: judged %d/%d is not starvation", r.ID, judOK, jud)
	case variant == "not-judged-empty":
		must(lit > 0 && jud == 0, "%s: %d judged of %d", r.ID, jud, lit)
	case variant == "not-judged-sparse":
		must(jud == 2 && judOK == 2 && jud*sec < outside && litOK*10 < lit*9, "%s: judged %d/%d over %d ns, literal %d/%d", r.ID, judOK, jud, outside, litOK, lit)
	}
}
