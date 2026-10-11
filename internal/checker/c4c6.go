package checker

import (
	"fmt"
	"sort"

	"github.com/anishathalye/porcupine"

	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// linTimeout is P11, the Porcupine timeout per partition.
var linTimeout = params.PorcupineTimeout

// C4 wraps Porcupine v1.3.1 (CONTRACT §10.4, ADR-0004): a register with
// read, write and cas, partitioned by key (each key checked separately, with
// its own P11 timeout). ok ops keep their observed output; info ops are
// open-ended (return at infinity, output unknown; info reads constrain
// nothing and are dropped); failed ops did not happen and are dropped. Every
// ok final read is a read of each key (a key without a row reads null).

type regIn struct {
	f            string // read, write, cas
	v, old, new_ int64
}

type regOut struct {
	known   bool
	set     bool // read: a value was present
	v       int64
	applied bool // cas
}

// regState is the register's value; the zero value is "no row".
type regState struct {
	set bool
	v   int64
}

var registerModel = porcupine.Model{
	Init: func() any { return regState{} },
	Step: func(state, input, output any) (bool, any) {
		s, in, out := state.(regState), input.(regIn), output.(regOut)
		switch in.f {
		case "read":
			if !out.known {
				return true, s
			}
			return out.set == s.set && (!out.set || out.v == s.v), s
		case "write":
			return true, regState{true, in.v}
		case "cas":
			match := s.set && s.v == in.old
			if !out.known {
				if match {
					return true, regState{true, in.new_}
				}
				return true, s
			}
			if sabotagePorcupineCAS {
				if out.applied {
					return true, regState{true, in.new_}
				}
				return true, s
			}
			if out.applied {
				return match, regState{true, in.new_}
			}
			return !match, s
		}
		return false, s
	},
	DescribeOperation: func(input, output any) string { return fmt.Sprintf("%+v -> %+v", input, output) },
}

// regOp is one operation of a key's partition.
type regOp struct {
	op   *Op
	in   regIn
	out  regOut
	call int64
	ret  int64 // Never if open-ended
}

// registerPartitions builds the per-key operation lists.
func registerPartitions(h *History) map[int64][]regOp {
	parts := map[int64][]regOp{}
	keys := map[int64]bool{}
	for _, o := range h.Ops {
		if o.WL != "register" || o.F == "final-read" {
			continue
		}
		in := obj(o.InValue)
		k, ok := Int(in["k"])
		if !ok {
			continue
		}
		keys[k] = true
		if o.Type == history.TypeFail {
			continue
		}
		ro := regOp{op: o, call: o.Invoke, ret: o.End()}
		switch o.F {
		case "read":
			if o.Type != history.TypeOK {
				continue
			}
			ro.in = regIn{f: "read"}
			v, set := Int(obj(o.Value)["v"])
			ro.out = regOut{known: true, set: set, v: v}
		case "write":
			v, _ := Int(in["v"])
			ro.in = regIn{f: "write", v: v}
			ro.out = regOut{known: o.Type == history.TypeOK}
		case "cas":
			old, _ := Int(in["old"])
			nw, _ := Int(in["new"])
			ro.in = regIn{f: "cas", old: old, new_: nw}
			if o.Type == history.TypeOK {
				ap, _ := obj(o.Value)["applied"].(bool)
				ro.out = regOut{known: true, applied: ap}
			}
		default:
			continue
		}
		parts[k] = append(parts[k], ro)
	}
	for _, o := range h.finalReads("register") {
		got := map[int64]regRow{}
		for _, r := range regFinal(o.Value) {
			got[r.k] = r
			keys[r.k] = true
		}
		for k := range keys {
			r := got[k]
			parts[k] = append(parts[k], regOp{op: o, in: regIn{f: "read"}, out: regOut{known: true, set: r.set, v: r.v}, call: o.Invoke, ret: o.Complete})
		}
	}
	for k := range keys {
		if _, ok := parts[k]; !ok {
			parts[k] = nil
		}
	}
	return parts
}

func porcupineOps(ops []regOp) []porcupine.Operation {
	out := make([]porcupine.Operation, 0, len(ops))
	for _, o := range ops {
		out = append(out, porcupine.Operation{ClientId: int(o.op.Proc), Input: o.in, Output: o.out, Call: o.call, Return: o.ret})
	}
	return out
}

func checkPartition(ops []regOp) porcupine.CheckResult {
	return porcupine.CheckOperationsTimeout(registerModel, porcupineOps(ops), linTimeout)
}

// prefix returns the operations invoked at or before t; those completing
// after t become open-ended with an unknown output (reads are dropped). If
// the full partition is not linearizable, prefix(t) is not linearizable for
// every t at or after the earliest offending completion, so a binary search
// finds the earliest offending operation.
func prefix(ops []regOp, t int64) []regOp {
	var out []regOp
	for _, o := range ops {
		if o.call > t {
			continue
		}
		if o.ret > t {
			if o.in.f == "read" {
				continue
			}
			o.ret, o.out = Never, regOut{}
		}
		out = append(out, o)
	}
	return out
}

// keyResult is C4's verdict on one key, kept for C6.
type keyResult struct {
	res      porcupine.CheckResult
	ops      []regOp
	earliest *regOp // the earliest offending op, if found
	searchUnknown bool
}

// checkLinearizable is C4; it also returns per-key results for C6.
func checkLinearizable(h *History) (Result, map[int64]*keyResult) {
	c := newCollector()
	parts := registerPartitions(h)
	keys := sortedKeys(parts)
	results := map[int64]*keyResult{}
	var unknown []int64
	perKey := map[string]any{}
	for _, k := range keys {
		ops := parts[k]
		kr := &keyResult{ops: ops, res: checkPartition(ops)}
		results[k] = kr
		perKey[fmt.Sprint(k)] = map[string]any{"ops": int64(len(ops)), "result": resultName(kr.res)}
		switch kr.res {
		case porcupine.Unknown:
			unknown = append(unknown, k)
		case porcupine.Illegal:
			kr.earliest, kr.searchUnknown = earliestOffending(ops)
			if kr.earliest != nil {
				e := kr.earliest
				c.add("non-linearizable", []int64{e.op.Index}, "register %d: no linearization; the earliest offending operation is %s i=%d (%+v -> %+v)", k, e.op.F, e.op.Index, e.in, e.out)
			} else {
				c.add("non-linearizable", nil, "register %d: no linearization of its %d operations", k, len(ops))
			}
		}
	}
	reason := ""
	if len(unknown) > 0 {
		reason = fmt.Sprintf("Porcupine timed out (P11 %s) on register keys %v", linTimeout, unknown)
	}
	return c.result("C4", reason, map[string]any{"keys": perKey}), results
}

func resultName(r porcupine.CheckResult) string {
	switch r {
	case porcupine.Ok:
		return "ok"
	case porcupine.Illegal:
		return "illegal"
	}
	return "unknown"
}

func earliestOffending(ops []regOp) (*regOp, bool) {
	var ts []int64
	seen := map[int64]bool{}
	for _, o := range ops {
		if o.ret != Never && o.out.known && !seen[o.ret] {
			seen[o.ret] = true
			ts = append(ts, o.ret)
		}
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
	if len(ts) == 0 {
		return nil, false
	}
	switch checkPartition(prefix(ops, ts[len(ts)-1])) {
	case porcupine.Unknown:
		return nil, true
	case porcupine.Ok:
		return nil, false
	}
	lo, hi := 0, len(ts)-1 // prefix(ts[hi]) is illegal
	for lo < hi {
		mid := (lo + hi) / 2
		switch checkPartition(prefix(ops, ts[mid])) {
		case porcupine.Illegal:
			hi = mid
		case porcupine.Ok:
			lo = mid + 1
		default:
			return nil, true
		}
	}
	var best *regOp
	for i := range ops {
		o := &ops[i]
		if o.ret == ts[lo] && o.out.known && (best == nil || o.op.Index < best.op.Index) {
			best = o
		}
	}
	return best, false
}

// checkStaleReads is C6 (CONTRACT §10.6): for a non-linearizable key, the
// earliest offending operation is a stale-read if it is a read whose value
// equals a value superseded by a write that completed before the read was
// invoked: some write W' of another value completed before the read was
// invoked, and the write of the read value (or the initial empty state)
// completed before W' was invoked.
func checkStaleReads(h *History, keys map[int64]*keyResult) Result {
	c := newCollector()
	var unknown []int64
	classified := int64(0)
	for _, k := range sortedKeys(keys) {
		kr := keys[k]
		if kr.res == porcupine.Unknown || kr.searchUnknown {
			unknown = append(unknown, k)
			continue
		}
		if kr.res != porcupine.Illegal || kr.earliest == nil {
			continue
		}
		r := kr.earliest
		if r.in.f != "read" || !r.out.known {
			continue
		}
		classified++
		for _, w2 := range kr.ops {
			if !installs(w2, true) || w2.ret >= r.call || (r.out.set && w2.in.f == "write" && w2.in.v == r.out.v) ||
				(r.out.set && w2.in.f == "cas" && w2.in.new_ == r.out.v) {
				continue
			}
			if !r.out.set { // the initial state was superseded by w2
				c.add("stale-read", []int64{r.op.Index, w2.op.Index}, "register %d: read i=%d returned no value although write i=%d completed before it was invoked", k, r.op.Index, w2.op.Index)
				break
			}
			found := false
			for _, w1 := range kr.ops {
				if installs(w1, false) && writes(w1) == r.out.v && w1.ret < w2.call {
					c.add("stale-read", []int64{r.op.Index, w1.op.Index, w2.op.Index}, "register %d: read i=%d returned %d, written by i=%d and superseded by i=%d, which completed before the read was invoked", k, r.op.Index, r.out.v, w1.op.Index, w2.op.Index)
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	reason := ""
	if len(unknown) > 0 {
		reason = fmt.Sprintf("C4 or its earliest-offender search did not complete for register keys %v", unknown)
	}
	return c.result("C6", reason, map[string]any{"offending_reads_classified": classified})
}

// installs reports whether o wrote a value: an ok write or applied cas, or
// (if okOnly is false) an info write or cas.
func installs(o regOp, okOnly bool) bool {
	switch o.in.f {
	case "write":
		return o.out.known || !okOnly
	case "cas":
		if o.out.known {
			return o.out.applied
		}
		return !okOnly
	}
	return false
}

func writes(o regOp) int64 {
	if o.in.f == "cas" {
		return o.in.new_
	}
	return o.in.v
}
