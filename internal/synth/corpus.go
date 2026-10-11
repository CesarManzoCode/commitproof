package synth

import (
	"fmt"
	"sort"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

// Entry is one corpus history.
type Entry struct {
	ID    string
	Build func() *Run
}

// Classes planted in W1 histories (CONTRACT §10.3).
var (
	W1BaseClasses = []string{"G0", "G1c", "G-single", "G-nonadjacent", "G2-item", "g1a", "g1b", "dirty-update",
		"internal", "duplicate-elements", "incompatible-order"}
	W1OrderedClasses = []string{"G0-process", "G1c-process", "G-single-process", "G-nonadjacent-process", "G2-item-process",
		"G0-realtime", "G1c-realtime", "G-single-realtime", "G-nonadjacent-realtime", "G2-item-realtime"}
	isos = []string{"serializable", "repeatable-read"}
)

func isoTag(iso string) string {
	if iso == "serializable" {
		return "ser"
	}
	return "rr"
}

// Corpus returns every entry of testdata/corpus, in id order.
func Corpus() []Entry {
	var es []Entry
	add := func(id string, f func() *Run) { es = append(es, Entry{id, f}) }
	for _, iso := range isos {
		for n := 1; n <= 60; n++ {
			id := fmt.Sprintf("w1-%s-clean-%03d", isoTag(iso), n)
			add(id, func() *Run { return w1Base(id, "clean", iso) })
		}
		for _, cl := range W1BaseClasses {
			for n := 1; n <= 6; n++ {
				id := fmt.Sprintf("w1-%s-%s-%02d", isoTag(iso), cl, n)
				add(id, func() *Run { r := w1Base(id, cl, iso); r.PlantW1(cl, r.T0+300*ms); return r })
			}
		}
		for _, cl := range W1OrderedClasses {
			for n := 1; n <= 4; n++ {
				id := fmt.Sprintf("w1-%s-%s-%02d", isoTag(iso), cl, n)
				add(id, func() *Run { r := w1Base(id, cl, iso); r.PlantW1(cl, r.T0+300*ms); return r })
			}
		}
	}
	for n := 1; n <= 3; n++ {
		id := fmt.Sprintf("w1-ser-lost-write-%02d", n)
		add(id, func() *Run { // an ok append on a fresh key; no final read has the key
			r := w1Base(id, "lost-write", "serializable")
			r.Plant = "ok append of 1 to key 1000, absent from every final read"
			r.Gadget(r.T0+300*ms, []gtx{okT(0, app(1000, 1))}, nil)
			r.Expect("C1", "lost-write")
			return r
		})
		id2 := fmt.Sprintf("w1-ser-g1a-final-%02d", n)
		add(id2, func() *Run { // a failed append observed only by the final reads
			r := w1Base(id2, "g1a", "serializable")
			r.Plant = "failed append of 1 to key 1000, present in every final read"
			r.Gadget(r.T0+300*ms, []gtx{failT(0, app(1000, 1))}, map[int64][]int64{1000: {1}})
			r.Expect("C2", "g1a")
			return r
		})
	}
	for n := 1; n <= 2; n++ {
		id := fmt.Sprintf("w1-ser-divergence-%02d", n)
		add(id, func() *Run {
			r := w1Base(id, "final-read-divergence", "serializable")
			r.Plant = "gw-z2's stable append final read lacks the last element of the longest key"
			r.PreFinal = append(r.PreFinal, func(r *Run) {
				var best int64 = -1
				for _, k := range keys(r.S.Lists) {
					if best < 0 || len(r.S.Lists[k]) > len(r.S.Lists[best]) {
						best = k
					}
				}
				fin := []any{}
				for _, k := range keys(r.S.Lists) {
					l := r.S.Lists[k]
					if k == best {
						l = l[:len(l)-1]
					}
					fin = append(fin, []any{k, ints(l)})
				}
				r.FinalOverride["gw-z2"] = map[string]any{"append": fin}
			})
			r.Expect("C1", "final-read-divergence")
			return r
		})
	}
	// W2.
	for n := 1; n <= 55; n++ {
		id := fmt.Sprintf("w2-clean-%03d", n)
		add(id, func() *Run { return w2Base(id, "clean") })
	}
	for _, p := range []struct {
		kind, label string
		n           int
	}{{"stale-read", "stale-read", 2}, {"stale-read-initial", "stale-read", 2}, {"phantom-value", "non-linearizable", 2},
		{"cas-applied", "non-linearizable", 2}, {"cas-not-applied", "non-linearizable", 1}, {"g1a", "g1a", 3}} {
		for n := 1; n <= p.n; n++ {
			id := fmt.Sprintf("w2-%s-%02d", p.kind, n)
			add(id, func() *Run { r := w2Base(id, p.label); r.PlantW2(p.kind, r.T0+400*ms); return r })
		}
	}
	// W3.
	for n := 1; n <= 55; n++ {
		id := fmt.Sprintf("w3-clean-%03d", n)
		add(id, func() *Run { return w3Base(id, "clean") })
	}
	for _, p := range []struct {
		kind, label string
		n           int
	}{{"sum-read", "bank-sum", 2}, {"sum-final", "bank-sum", 1}, {"negative", "bank-negative", 3}, {"divergence", "final-read-divergence", 1}} {
		for n := 1; n <= p.n; n++ {
			id := fmt.Sprintf("w3-%s-%02d", p.kind, n)
			add(id, func() *Run { r := w3Base(id, p.label); r.PlantW3(p.kind); return r })
		}
	}
	// W4.
	for n := 1; n <= 55; n++ {
		id := fmt.Sprintf("w4-clean-%03d", n)
		add(id, func() *Run { return w4Base(id, "clean") })
	}
	for _, p := range []struct {
		kind, label string
		n           int
	}{{"lost-write", "lost-write", 3}, {"divergence", "final-read-divergence", 2}, {"g1a", "g1a", 3}} {
		for n := 1; n <= p.n; n++ {
			id := fmt.Sprintf("w4-%s-%02d", p.kind, n)
			add(id, func() *Run { r := w4Base(id, p.label); r.PlantW4(p.kind); return r })
		}
	}
	// Mixed W1–W4 (the client layout of P02).
	for n := 1; n <= 10; n++ {
		id := fmt.Sprintf("mixed-clean-%03d", n)
		add(id, func() *Run {
			r := NewRun(id, "mixed", "clean")
			r.Elle = true
			end := r.T0 + 1000*ms
			nid := int64(0)
			r.W1([]int64{0, 1, 2, 3}, r.T0, end, w1Fail, w1Info)
			r.W2([]int64{4, 5}, r.T0, end, w2Fail, w2Info)
			r.W3([]int64{6, 7}, r.T0, end, w3Fail, w3Info)
			r.W4([]int64{8, 9}, r.T0, end, w4Fail, w4Info, &nid)
			return r
		})
	}
	es = append(es, moreEntries()...)
	sort.Slice(es, func(i, j int) bool { return es[i].ID < es[j].ID })
	return es
}

// w1Base is a W1 history: 1 s of four clients for clean histories, 0.6 s
// for planted ones (the plant, not the base, is the point; the base keeps
// realistic concurrency, info and fail outcomes around it).
func w1Base(id, label, iso string) *Run {
	r := NewRun(id, "w1", label)
	r.Iso, r.Elle = iso, true
	d := 1000 * ms
	if label != "clean" {
		d = 600 * ms
	}
	r.W1([]int64{0, 1, 2, 3}, r.T0, r.T0+d, w1Fail, w1Info)
	return r
}

func w2Base(id, label string) *Run {
	r := NewRun(id, "w2", label)
	r.W2([]int64{4, 5, 6}, r.T0, r.T0+800*ms, w2Fail, w2Info)
	return r
}

func w3Base(id, label string) *Run {
	r := NewRun(id, "w3", label)
	r.W3([]int64{6, 7}, r.T0, r.T0+1000*ms, w3Fail, w3Info)
	return r
}

func w4Base(id, label string) *Run {
	r := NewRun(id, "w4", label)
	nid := int64(0)
	r.W4([]int64{8, 9}, r.T0, r.T0+1000*ms, w4Fail, w4Info, &nid)
	return r
}

// okOps returns the ok ops of (wl, f), in invoke order.
func (r *Run) okOps(wl, f string) []*Op {
	var out []*Op
	for _, o := range r.Ops {
		if o.WL == wl && o.F == f && o.Type == history.TypeOK && o.Client < 10 {
			out = append(out, o)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Inv < out[j].Inv })
	return out
}

func rows(v any) [][2]int64 {
	var out [][2]int64
	for _, x := range v.([]any) {
		p := x.([]any)
		out = append(out, [2]int64{p[0].(int64), p[1].(int64)})
	}
	return out
}

func unrows(rs [][2]int64) []any {
	out := []any{}
	for _, x := range rs {
		out = append(out, []any{x[0], x[1]})
	}
	return out
}

// PlantW3 alters bank observations (CONTRACT §10.5).
func (r *Run) PlantW3(kind string) {
	r.Plant = kind
	switch kind {
	case "sum-read": // one ok read-all shows account 0 one unit short
		r.Post = append(r.Post, func(r *Run) {
			ra := r.okOps("bank", "read-all")
			o := ra[len(ra)/2]
			rs := rows(o.Out)
			rs[0][1]--
			o.Out = unrows(rs)
		})
		r.Expect("C5", "bank-sum")
	case "sum-final": // every gateway's final read shows account 0 one short
		r.PreFinal = append(r.PreFinal, func(r *Run) {
			rs := rows(r.S.acctRows())
			rs[0][1]--
			for _, gw := range Gateways {
				r.FinalOverride[gw] = map[string]any{"bank": unrows(rs)}
			}
		})
		r.Expect("C5", "bank-sum")
	case "negative": // one ok read-all: account 0 at -5, account 1 compensates
		r.Post = append(r.Post, func(r *Run) {
			ra := r.okOps("bank", "read-all")
			o := ra[len(ra)/2]
			rs := rows(o.Out)
			rs[1][1] += rs[0][1] + 5
			rs[0][1] = -5
			o.Out = unrows(rs)
		})
		r.Expect("C5", "bank-negative")
	case "divergence": // gw-z3's final read moves one unit between accounts
		r.PreFinal = append(r.PreFinal, func(r *Run) {
			rs := rows(r.S.acctRows())
			hi, lo := 0, 0
			for i := range rs {
				if rs[i][1] > rs[hi][1] {
					hi = i
				}
				if rs[i][1] < rs[lo][1] {
					lo = i
				}
			}
			if hi == lo {
				lo = (hi + 1) % len(rs)
			}
			rs[hi][1]--
			rs[lo][1]++
			r.FinalOverride["gw-z3"] = map[string]any{"bank": unrows(rs)}
		})
		r.Expect("C1", "final-read-divergence")
	default:
		panic("unknown W3 plant " + kind)
	}
}

// PlantW4 plants ledger anomalies (CONTRACT §10.1, §10.2).
func (r *Run) PlantW4(kind string) {
	r.Plant = kind
	switch kind {
	case "lost-write": // an ok insert's row is missing from the final state
		r.PreFinal = append(r.PreFinal, func(r *Run) {
			ins := r.okOps("ledger", "insert")
			id := ins[len(ins)/2].In.(map[string]any)["id"].(int64)
			delete(r.S.Ledger, id)
		})
		r.Expect("C1", "lost-write")
	case "divergence": // gw-z2's final read lacks one ok insert
		r.PreFinal = append(r.PreFinal, func(r *Run) {
			ins := r.okOps("ledger", "insert")
			drop := ins[len(ins)/2].In.(map[string]any)["id"].(int64)
			var ids []int64
			for _, id := range keys(r.S.Ledger) {
				if id != drop {
					ids = append(ids, id)
				}
			}
			r.FinalOverride["gw-z2"] = map[string]any{"ledger": ints(ids)}
		})
		r.Expect("C1", "final-read-divergence")
	case "g1a": // an insert reported failed with 23505 whose row exists (M3)
		at := r.T0 + 500*ms
		id := int64(8_000_000_001)
		o := &Op{Client: 20, Proc: r.NewProc(), FixedProc: true, GW: "gw-z1", WL: "ledger", F: "insert",
			In: map[string]any{"id": id, "payload": "planted"}, Inv: at + 1, Eff: at + 2, Cmp: at + 3*ms + 1}
		o.Fail("23505", "23", "statement")
		r.Ops = append(r.Ops, o)
		r.S.Ledger[id] = true
		r.Expect("C2", "g1a")
	default:
		panic("unknown W4 plant " + kind)
	}
}
