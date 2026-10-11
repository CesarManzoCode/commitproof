package synth

import (
	"fmt"

	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// W1 key rotation shared by a run's list-append clients (P20).
type w1keys struct {
	active []int64
	next   int64
	count  map[int64]int64
}

func newW1Keys() *w1keys {
	k := &w1keys{count: map[int64]int64{}, next: params.AppendActiveKeys}
	for i := int64(0); i < params.AppendActiveKeys; i++ {
		k.active = append(k.active, i)
	}
	return k
}

// W1 adds list-append clients (CONTRACT §7) at the run's isolation level.
func (r *Run) W1(clients []int64, start, end int64, pf, pi float64) {
	ks := newW1Keys()
	r.Workloads = appendOnce(r.Workloads, "W1")
	for _, c := range clients {
		r.Loop(c, Gateways[c%3], "append", start, end, 2*ms, 20*ms, func(o *Op) bool {
			n := 1 + r.R.IntN(4)
			mops := make([]any, 0, n)
			for range n {
				slot := r.R.IntN(len(ks.active))
				k := ks.active[slot]
				if r.R.IntN(2) == 0 {
					mops = append(mops, []any{"r", k, nil})
					continue
				}
				ks.count[k]++
				mops = append(mops, []any{"append", k, ks.count[k]})
				if ks.count[k] >= params.AppendKeyRetireAt {
					ks.active[slot] = ks.next
					ks.next++
				}
			}
			o.F, o.Iso, o.In = "txn", r.Iso, mops
			o.Exec = func(s *Store, at int64) any { return s.txn(at, o, mops) }
			fe := Err40001Stmt
			if r.R.IntN(3) == 0 {
				fe = Err40001Commit
			}
			r.Outcome(o, pf, pi, fe)
			return true
		})
	}
}

// W2 adds register clients: read 40%, write 40%, cas 20% on P21's keys,
// values unique per run.
func (r *Run) W2(clients []int64, start, end int64, pf, pi float64) {
	r.Workloads = appendOnce(r.Workloads, "W2")
	val := int64(0)
	last := map[int64]int64{}
	for _, c := range clients {
		r.Loop(c, Gateways[c%3], "register", start, end, 1*ms, 8*ms, func(o *Op) bool {
			k := int64(r.R.IntN(params.RegisterKeys))
			x := r.R.IntN(10)
			old, has := last[k]
			switch {
			case x < 4 || (x >= 8 && !has):
				o.F, o.In = "read", map[string]any{"k": k}
			case x < 8:
				val++
				last[k] = val
				o.F, o.In = "write", map[string]any{"k": k, "v": val}
			default:
				val++
				last[k] = val
				o.F, o.In = "cas", map[string]any{"k": k, "old": old, "new": val}
			}
			in, f := o.In.(map[string]any), o.F
			o.Exec = func(s *Store, at int64) any { return s.register(f, in) }
			r.Outcome(o, pf, pi, ErrConnect)
			if f == "read" && o.Type == history.TypeInfo {
				o.Applied = false
			}
			return true
		})
	}
}

// W3 adds bank clients: transfer 70%, read-all 30%, serializable (P22).
func (r *Run) W3(clients []int64, start, end int64, pf, pi float64) {
	r.Workloads = appendOnce(r.Workloads, "W3")
	for _, c := range clients {
		r.Loop(c, Gateways[c%3], "bank", start, end, 2*ms, 15*ms, func(o *Op) bool {
			o.Iso = "serializable"
			if r.R.IntN(10) < 7 {
				from := int64(r.R.IntN(params.BankAccounts))
				to := int64(r.R.IntN(params.BankAccounts - 1))
				if to >= from {
					to++
				}
				o.F, o.In = "transfer", map[string]any{"from": from, "to": to, "amt": int64(1 + r.R.IntN(5))}
			} else {
				o.F, o.In = "read-all", map[string]any{}
			}
			in, f := o.In.(map[string]any), o.F
			o.Exec = func(s *Store, at int64) any { return s.bank(f, in) }
			r.Outcome(o, pf, pi, Err40001Stmt)
			if f == "read-all" && o.Type == history.TypeInfo {
				o.Applied = false
			}
			return true
		})
	}
}

// W4 adds ledger clients: autocommit inserts with unique ids.
func (r *Run) W4(clients []int64, start, end int64, pf, pi float64, nextID *int64) {
	r.Workloads = appendOnce(r.Workloads, "W4")
	for _, c := range clients {
		r.Loop(c, Gateways[c%3], "ledger", start, end, 1*ms, 8*ms, func(o *Op) bool {
			*nextID++
			in := map[string]any{"id": *nextID, "payload": fmt.Sprintf("c%d-%016x", c, r.R.Uint64())}
			o.F, o.In = "insert", in
			o.Exec = func(s *Store, at int64) any { return s.insert(at, o, in) }
			r.Outcome(o, pf, pi, ErrConnect)
			return true
		})
	}
}

func appendOnce(xs []string, x string) []string {
	for _, y := range xs {
		if y == x {
			return xs
		}
	}
	return append(xs, x)
}

// Standard clean outcome rates.
const (
	w1Fail, w1Info = 0.08, 0.03
	w2Fail, w2Info = 0.03, 0.03
	w3Fail, w3Info = 0.08, 0.02
	w4Fail, w4Info = 0.02, 0.03
)

// --- Planted W1 gadgets -------------------------------------------------

// gtx is one planted transaction: proc groups transactions into one process
// (equal non-negative proc = same process), times are offsets from the
// gadget start in ns.
type gtx struct {
	proc     int
	typ      string
	mops     []any // with read results (ok); reads nil otherwise
	inv, cmp int64
}

func app(k, v int64) []any { return []any{"append", k, v} }
func rd(k int64, vals ...int64) []any {
	if vals == nil {
		return []any{"r", k, nil}
	}
	return []any{"r", k, ints(vals)}
}
func ms_(xs ...[]any) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

// Concurrent timing: every planted transaction overlaps every other.
func conc(j int) (int64, int64) { return 1 + int64(j)*3, 3*ms + 1 + int64(j)*3 }

// Sequential pair timing: a then b (b invoked 1 ns after a completed);
// others overlap both.
var (
	seqA = [2]int64{1, 2*ms + 1}
	seqB = [2]int64{2*ms + 2, 4*ms + 3}
)

func seqOther(j int) (int64, int64) { return 3 + int64(j)*3, 5*ms + 3 + int64(j)*3 }

// Gadget plants W1 transactions at time at on fresh keys, fresh processes
// and clients 20+; final sets the planted keys' rows (nil list = no row).
func (r *Run) Gadget(at int64, txs []gtx, final map[int64][]int64) {
	procs := map[int]int64{}
	for i, t := range txs {
		p := t.proc
		if p < 0 {
			p = 1000 + i
		}
		if _, ok := procs[p]; !ok {
			procs[p] = r.NewProc()
		}
		in := make([]any, len(t.mops))
		for j, m := range t.mops {
			mo := m.([]any)
			if mo[0] == "r" {
				in[j] = []any{"r", mo[1], nil}
			} else {
				in[j] = mo
			}
		}
		o := &Op{Client: 20 + procs[p] - 100, Proc: procs[p], FixedProc: true, GW: "gw-z1", WL: "append", F: "txn", Iso: r.Iso,
			In: in, Out: t.mops, Type: t.typ, Applied: t.typ == history.TypeOK, Inv: at + t.inv, Eff: at + t.inv + 1, Cmp: at + t.cmp}
		switch t.typ {
		case history.TypeFail:
			o.Err = map[string]any{"sqlstate": "40001", "class": "40", "phase": "commit"}
		case history.TypeInfo:
			o.Err = map[string]any{"sqlstate": "", "class": "timeout", "phase": "client-timeout"}
		}
		r.Ops = append(r.Ops, o)
	}
	for k, l := range final {
		if l != nil {
			r.S.Lists[k] = l
		}
	}
}

func okT(j int, mops ...[]any) gtx {
	i, c := conc(j)
	return gtx{proc: -1, typ: history.TypeOK, mops: ms_(mops...), inv: i, cmp: c}
}
func failT(j int, mops ...[]any) gtx {
	t := okT(j, mops...)
	t.typ = history.TypeFail
	return t
}

// W1Plants maps a planted C3 class to its gadget and expected classes.
// Keys 1000+ are never used by the base workload. Each expected set is
// derived from the definitions (CONTRACT §10.1–§10.3, Elle's per-element
// version model): see the comment of each case.
func (r *Run) PlantW1(class string, at int64) {
	const x, y, a, b, c, d = 1000, 1001, 1002, 1003, 1004, 1005
	r.Plant = class
	switch class {
	case "G0": // ww x: T1→T2, ww y: T2→T1
		r.Gadget(at, []gtx{okT(0, app(x, 1), app(y, 1)), okT(1, app(x, 2), app(y, 2)), okT(2, rd(x, 1, 2), rd(y, 2, 1))},
			map[int64][]int64{x: {1, 2}, y: {2, 1}})
		r.ExpectC3("G0")
	case "G1c": // wr x: T1→T2, wr y: T2→T1
		r.Gadget(at, []gtx{okT(0, app(x, 1), rd(y, 1)), okT(1, app(y, 1), rd(x, 1))}, map[int64][]int64{x: {1}, y: {1}})
		r.ExpectC3("G1c")
	case "G-single": // wr x: T1→T2, rw y: T2→T1
		r.Gadget(at, []gtx{okT(0, app(x, 1), app(y, 1)), okT(1, rd(x, 1), rd(y))}, map[int64][]int64{x: {1}, y: {1}})
		r.ExpectC3("G-single")
	case "G-nonadjacent": // rw a: T1→T2, wr b: T2→T3, rw c: T3→T4, wr d: T4→T1
		r.Gadget(at, []gtx{okT(0, rd(a), rd(d, 1)), okT(1, app(a, 1), app(b, 1)), okT(2, rd(b, 1), rd(c)), okT(3, app(c, 1), app(d, 1))},
			map[int64][]int64{a: {1}, b: {1}, c: {1}, d: {1}})
		r.ExpectC3("G-nonadjacent")
	case "G2-item": // rw x: T1→T2, rw y: T2→T1 (write skew)
		r.Gadget(at, []gtx{okT(0, rd(x), app(y, 1)), okT(1, rd(y), app(x, 1))}, map[int64][]int64{x: {1}, y: {1}})
		r.ExpectC3("G2-item")
	case "g1a": // a read observes a failed append; C2 sees the same read
		r.Gadget(at, []gtx{failT(0, app(x, 1)), okT(1, rd(x, 1))}, map[int64][]int64{x: nil})
		r.ExpectC3("g1a")
		r.Expect("C2", "g1a")
	case "g1b": // read [1] ends at T1's intermediate append; per-element
		// versions also give wr T1→T2 and rw T2→T1 (the version after [1] is
		// T1's [1 2]): G-single
		r.Gadget(at, []gtx{okT(0, app(x, 1), app(x, 2)), okT(1, rd(x, 1))}, map[int64][]int64{x: {1, 2}})
		r.ExpectC3("g1b", "G-single")
	case "dirty-update": // T2's append follows failed T1's; T3 and the final
		// reads observe the failed element (g1a in C2 and C3)
		r.Gadget(at, []gtx{failT(0, app(x, 1)), okT(1, app(x, 2)), okT(2, rd(x, 1, 2))}, map[int64][]int64{x: {1, 2}})
		r.ExpectC3("dirty-update", "g1a")
		r.Expect("C2", "g1a")
	case "internal": // T1 reads x as empty after appending to it
		r.Gadget(at, []gtx{okT(0, app(x, 1), rd(x))}, map[int64][]int64{x: {1}})
		r.ExpectC3("internal")
	case "duplicate-elements":
		r.Gadget(at, []gtx{okT(0, app(x, 1)), okT(1, rd(x, 1, 1))}, map[int64][]int64{x: {1}})
		r.ExpectC3("duplicate-elements")
	case "incompatible-order": // [1 2] and [2 1]; all four concurrent
		r.Gadget(at, []gtx{okT(0, app(x, 1)), okT(1, app(x, 2)), okT(2, rd(x, 1, 2)), okT(3, rd(x, 2, 1))}, map[int64][]int64{x: {1, 2}})
		r.ExpectC3("incompatible-order")
	default:
		r.plantW1Ordered(class, at)
	}
}

// plantW1Ordered plants the -process and -realtime variants: transactions a
// and b run one after the other (same process for -process, different
// processes for -realtime) and the cycle closes through that order. A
// process order between completed transactions is also a realtime order, so
// -process plants also exhibit the -realtime class.
func (r *Run) plantW1Ordered(class string, at int64) {
	const x, y, a, b, c = 1000, 1001, 1002, 1003, 1004
	var base, suffix string
	for _, s := range []string{"-process", "-realtime"} {
		if len(class) > len(s) && class[len(class)-len(s):] == s {
			base, suffix = class[:len(class)-len(s)], s
		}
	}
	pa, pb := 0, 1
	if suffix == "-process" {
		pb = 0
	} else if suffix != "-realtime" {
		panic("unknown W1 plant " + class)
	}
	A := func(mops ...[]any) gtx {
		return gtx{proc: pa, typ: history.TypeOK, mops: ms_(mops...), inv: seqA[0], cmp: seqA[1]}
	}
	B := func(mops ...[]any) gtx {
		return gtx{proc: pb, typ: history.TypeOK, mops: ms_(mops...), inv: seqB[0], cmp: seqB[1]}
	}
	O := func(j int, mops ...[]any) gtx {
		i, cm := seqOther(j)
		return gtx{proc: -1, typ: history.TypeOK, mops: ms_(mops...), inv: i, cmp: cm}
	}
	r.Plant = class
	switch base {
	case "G0": // ww x: b→a (order [2 1]); order a→b
		r.Gadget(at, []gtx{A(app(x, 1)), B(app(x, 2)), O(0, rd(x, 2, 1))}, map[int64][]int64{x: {2, 1}})
	case "G1c": // wr y: b→a; order a→b
		r.Gadget(at, []gtx{A(app(x, 1), rd(y, 1)), B(app(y, 1))}, map[int64][]int64{x: {1}, y: {1}})
	case "G-single": // rw x: b→a; order a→b
		r.Gadget(at, []gtx{A(app(x, 1)), B(rd(x))}, map[int64][]int64{x: {1}})
	case "G-nonadjacent": // rw b: b→T3, wr c: T3→T4, rw a: T4→a; order a→b
		r.Gadget(at, []gtx{A(app(a, 1)), B(rd(b)), O(0, app(b, 1), app(c, 1)), O(1, rd(c, 1), rd(a))},
			map[int64][]int64{a: {1}, b: {1}, c: {1}})
	case "G2-item": // rw y: b→T3, rw x: T3→a (adjacent); order a→b
		r.Gadget(at, []gtx{A(app(x, 1)), B(rd(y)), O(0, rd(x), app(y, 1))}, map[int64][]int64{x: {1}, y: {1}})
	default:
		panic("unknown W1 plant " + class)
	}
	if suffix == "-process" {
		r.ExpectC3(base+"-process", base+"-realtime")
	} else {
		r.ExpectC3(base + "-realtime")
	}
}

// --- W2 register gadgets on key 100 (P21's keys are 0–4) ---------------

type rop struct {
	f        string
	in, out  map[string]any
	typ      string
	inv, cmp int64
}

func (r *Run) regGadget(at int64, ops []rop, final *int64) {
	for i, g := range ops {
		p := r.NewProc()
		o := &Op{Client: 20 + int64(i), Proc: p, FixedProc: true, GW: "gw-z1", WL: "register", F: g.f, In: g.in, Out: g.out,
			Type: g.typ, Applied: g.typ == history.TypeOK, Inv: at + g.inv, Eff: at + g.inv + 1, Cmp: at + g.cmp}
		if g.typ == history.TypeFail {
			o.Err = map[string]any{"sqlstate": "", "class": "connect", "phase": "connect"}
		}
		r.Ops = append(r.Ops, o)
	}
	if final != nil {
		r.S.Regs[100] = *final
	}
}

func wr(v int64, inv, cmp int64) rop {
	return rop{"write", map[string]any{"k": int64(100), "v": v}, map[string]any{"k": int64(100), "v": v}, history.TypeOK, inv, cmp}
}
func rdr(v any, inv, cmp int64) rop {
	return rop{"read", map[string]any{"k": int64(100)}, map[string]any{"k": int64(100), "v": v}, history.TypeOK, inv, cmp}
}
func casr(old, nw int64, applied bool, inv, cmp int64) rop {
	return rop{"cas", map[string]any{"k": int64(100), "old": old, "new": nw},
		map[string]any{"k": int64(100), "old": old, "new": nw, "applied": applied}, history.TypeOK, inv, cmp}
}

// PlantW2 plants a register anomaly on key 100 (values 7_000_000+, unique).
func (r *Run) PlantW2(kind string, at int64) {
	const A, B, C, Z = 7_000_001, 7_000_002, 7_000_003, 7_999_999
	p := func(v int64) *int64 { return &v }
	r.Plant = kind
	switch kind {
	case "stale-read": // write A, write B, then a read returns A
		r.regGadget(at, []rop{wr(A, 1, 2*ms), wr(B, 3*ms, 5*ms), rdr(A, 6*ms, 8*ms)}, p(B))
		r.Expect("C4", "non-linearizable")
		r.Expect("C6", "stale-read")
	case "stale-read-initial": // write A completes, then a read returns no value
		r.regGadget(at, []rop{wr(A, 1, 2*ms), rdr(nil, 3*ms, 5*ms)}, p(A))
		r.Expect("C4", "non-linearizable")
		r.Expect("C6", "stale-read")
	case "phantom-value": // a read returns a value nobody wrote: not stale
		r.regGadget(at, []rop{wr(A, 1, 2*ms), rdr(int64(Z), 3*ms, 5*ms)}, p(A))
		r.Expect("C4", "non-linearizable")
	case "cas-applied": // cas(A→C) applied after B superseded A
		r.regGadget(at, []rop{wr(A, 1, 2*ms), wr(B, 3*ms, 5*ms), casr(A, C, true, 6*ms, 8*ms)}, p(C))
		r.Expect("C4", "non-linearizable")
	case "cas-not-applied": // cas(A→C) not applied although A was current
		r.regGadget(at, []rop{wr(A, 1, 2*ms), casr(A, C, false, 3*ms, 5*ms)}, p(A))
		r.Expect("C4", "non-linearizable")
	case "g1a": // a failed write's value is read: C2 g1a; for C4 the read
		// observes a value no completed write installed
		f := wr(A, 1, 2*ms)
		f.typ, f.out = history.TypeFail, nil
		r.regGadget(at, []rop{f, rdr(int64(A), 3*ms, 5*ms)}, p(A))
		r.Expect("C2", "g1a")
		r.Expect("C4", "non-linearizable")
	default:
		panic("unknown W2 plant " + kind)
	}
}
