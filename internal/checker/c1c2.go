package checker

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/history"
)

// appendOf is one W1 append micro-op (k, v) of an operation.
type kv struct{ K, V int64 }

// appendsOf returns the append micro-ops of a W1 op, in order.
func appendsOf(o *Op) []kv {
	mops, _ := o.InValue.([]any)
	var out []kv
	for _, m := range mops {
		mo, _ := m.([]any)
		if len(mo) != 3 || mo[0] != "append" {
			continue
		}
		k, ok1 := Int(mo[1])
		v, ok2 := Int(mo[2])
		if ok1 && ok2 {
			out = append(out, kv{k, v})
		}
	}
	return out
}

// appendFinal decodes an append final read ([[k, [vals…]], …]) into k → set.
func appendFinal(v any) map[int64]map[int64]bool {
	out := map[int64]map[int64]bool{}
	rows, _ := v.([]any)
	for _, r := range rows {
		row, _ := r.([]any)
		if len(row) != 2 {
			continue
		}
		k, ok := Int(row[0])
		if !ok {
			continue
		}
		vals, _ := IntList(row[1])
		set := map[int64]bool{}
		for _, x := range vals {
			set[x] = true
		}
		out[k] = set
	}
	return out
}

// ledgerFinal decodes a ledger final read ([id, …]) into a set.
func ledgerFinal(v any) map[int64]bool {
	ids, _ := IntList(v)
	out := map[int64]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// ledgerID returns the id of a W4 insert.
func ledgerID(o *Op) (int64, bool) { return Int(obj(o.InValue)["id"]) }

// CheckLostWrites is C1 (CONTRACT §10.1). lost-write: an ok W4 insert or W1
// append absent from the stable final read through every gateway.
// final-read-divergence: the stable final reads of some workload differ
// between gateways. info writes may be present or absent.
func CheckLostWrites(h *History) Result {
	c := newCollector()
	stable, missing := h.StableFinal()
	gws := h.Gateways()
	stats := map[string]any{"gateways": int64(len(gws))}
	if len(missing) > 0 {
		return c.result("C1", "no stable final read for "+strings.Join(missing, ", "), stats)
	}
	// final-read-divergence.
	for _, wl := range FinalWorkloads {
		var ref []byte
		var refGW string
		for _, gw := range gws {
			b, err := canonjson.Marshal(stable[gw][wl].Value)
			if err != nil {
				return c.result("C1", fmt.Sprintf("final read %s/%s: %v", gw, wl, err), stats)
			}
			if ref == nil {
				ref, refGW = b, gw
				continue
			}
			if !bytes.Equal(b, ref) {
				ops := append(append([]int64{}, stable[refGW][wl].Ops...), stable[gw][wl].Ops...)
				c.add("final-read-divergence", ops, "stable %s final read through %s differs from %s", wl, gw, refGW)
			}
		}
	}
	// lost-write: present through no gateway.
	appendSets := map[string]map[int64]map[int64]bool{}
	ledgerSets := map[string]map[int64]bool{}
	for _, gw := range gws {
		appendSets[gw] = appendFinal(stable[gw]["append"].Value)
		ledgerSets[gw] = ledgerFinal(stable[gw]["ledger"].Value)
	}
	var checked int64
	for _, o := range h.Ops {
		if o.Type != history.TypeOK || o.F == "final-read" {
			continue
		}
		switch o.WL {
		case "append":
			for _, a := range appendsOf(o) {
				checked++
				if lostWriteDiff(gws, func(gw string) bool { return appendSets[gw][a.K][a.V] }) {
					c.add("lost-write", []int64{o.Index}, "ok append of %d to key %d is absent from the stable final read through every gateway", a.V, a.K)
				}
			}
		case "ledger":
			id, ok := ledgerID(o)
			if !ok {
				continue
			}
			checked++
			if lostWriteDiff(gws, func(gw string) bool { return ledgerSets[gw][id] }) {
				c.add("lost-write", []int64{o.Index}, "ok ledger insert %d is absent from the stable final read through every gateway", id)
			}
		}
	}
	stats["ok_writes_checked"] = checked
	return c.result("C1", "", stats)
}

// lostWriteDiff is C1's set difference: a write is lost iff it is present
// through no gateway.
func lostWriteDiff(gws []string, present func(gw string) bool) bool {
	if sabotageLostWriteDiff {
		return false
	}
	for _, gw := range gws {
		if present(gw) {
			return false
		}
	}
	return true
}

// CheckAbortedVisible is C2 (CONTRACT §10.2). g1a: a value whose write
// completed fail is observed by any read, including final reads. Traceable
// writes: W1 appends (k, v), W4 inserts (id), W2 writes and cas (k, value);
// values are unique per run (P20, P21).
func CheckAbortedVisible(h *History) Result {
	c := newCollector()
	failedAppend := map[kv]*Op{}
	failedLedger := map[int64]*Op{}
	failedReg := map[kv]*Op{}
	for _, o := range h.Ops {
		if o.Type != history.TypeFail || o.F == "final-read" {
			continue
		}
		switch o.WL {
		case "append":
			for _, a := range appendsOf(o) {
				failedAppend[a] = o
			}
		case "ledger":
			if id, ok := ledgerID(o); ok {
				failedLedger[id] = o
			}
		case "register":
			if w, ok := regWritten(o); ok {
				failedReg[w] = o
			}
		}
	}
	var reads int64
	report := func(w, r *Op, what string) {
		if !sabotageG1a {
			c.add("g1a", []int64{w.Index, r.Index}, "%s written by failed op i=%d is observed by read i=%d", what, w.Index, r.Index)
		}
	}
	for _, o := range h.Ops {
		if o.Type != history.TypeOK {
			continue
		}
		switch {
		case o.WL == "append" && o.F == "final-read":
			reads++
			for _, k := range sortedKeys(appendFinal(o.Value)) {
				for _, v := range sortedKeys(appendFinal(o.Value)[k]) {
					if w, ok := failedAppend[kv{k, v}]; ok {
						report(w, o, fmt.Sprintf("element %d of key %d", v, k))
					}
				}
			}
		case o.WL == "append":
			for _, rd := range readsOf(o) {
				reads++
				for _, v := range rd.vals {
					if w, ok := failedAppend[kv{rd.k, v}]; ok {
						report(w, o, fmt.Sprintf("element %d of key %d", v, rd.k))
					}
				}
			}
		case o.WL == "ledger" && o.F == "final-read":
			reads++
			ids, _ := IntList(o.Value)
			for _, id := range ids {
				if w, ok := failedLedger[id]; ok {
					report(w, o, fmt.Sprintf("ledger row %d", id))
				}
			}
		case o.WL == "register" && o.F == "final-read":
			reads++
			for _, r := range regFinal(o.Value) {
				if r.set {
					if w, ok := failedReg[kv{r.k, r.v}]; ok {
						report(w, o, fmt.Sprintf("value %d of register %d", r.v, r.k))
					}
				}
			}
		case o.WL == "register" && o.F == "read":
			reads++
			m := obj(o.Value)
			k, ok1 := Int(m["k"])
			v, ok2 := Int(m["v"])
			if ok1 && ok2 {
				if w, ok := failedReg[kv{k, v}]; ok {
					report(w, o, fmt.Sprintf("value %d of register %d", v, k))
				}
			}
		}
	}
	stats := map[string]any{"reads_checked": reads, "failed_writes": int64(len(failedAppend) + len(failedLedger) + len(failedReg))}
	return c.result("C2", "", stats)
}

type readMop struct {
	k    int64
	vals []int64
	nil_ bool
}

// readsOf returns the read micro-ops of an ok W1 op with their results.
func readsOf(o *Op) []readMop {
	mops, _ := o.Value.([]any)
	var out []readMop
	for _, m := range mops {
		mo, _ := m.([]any)
		if len(mo) != 3 || mo[0] != "r" {
			continue
		}
		k, ok := Int(mo[1])
		if !ok {
			continue
		}
		if mo[2] == nil {
			out = append(out, readMop{k: k, nil_: true})
			continue
		}
		vals, ok := IntList(mo[2])
		if ok {
			out = append(out, readMop{k: k, vals: vals})
		}
	}
	return out
}

// regWritten returns the (k, value) a register write or cas would install.
func regWritten(o *Op) (kv, bool) {
	m := obj(o.InValue)
	k, ok := Int(m["k"])
	if !ok {
		return kv{}, false
	}
	switch o.F {
	case "write":
		v, ok := Int(m["v"])
		return kv{k, v}, ok
	case "cas":
		v, ok := Int(m["new"])
		return kv{k, v}, ok
	}
	return kv{}, false
}

type regRow struct {
	k, v int64
	set  bool
}

// regFinal decodes a register final read ([[k, v|null], …]).
func regFinal(v any) []regRow {
	rows, _ := v.([]any)
	var out []regRow
	for _, r := range rows {
		row, _ := r.([]any)
		if len(row) != 2 {
			continue
		}
		k, ok := Int(row[0])
		if !ok {
			continue
		}
		if row[1] == nil {
			out = append(out, regRow{k: k})
			continue
		}
		if x, ok := Int(row[1]); ok {
			out = append(out, regRow{k: k, v: x, set: true})
		}
	}
	return out
}

func sortedKeys[V any](m map[int64]V) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
