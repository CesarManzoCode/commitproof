// Package synth generates the K2 synthetic corpus (CONTRACT §15 K2): sealed
// run directories whose histories come from an in-memory reference store
// that executes every operation atomically at one instant inside its
// [invoke, complete] interval, in that instant's order — serializable (and
// strictly so) by construction — plus a planting layer that injects one
// anomaly class at a time. expected.json is derived from what was planted
// (hand-written per plant from the definitions of CONTRACT §10), never from
// running the checkers: this package must not import internal/checker
// (enforced by the independence test).
package synth

import (
	"fmt"
	"sort"
)

// Store is the reference database: W1 lists, W2 registers, W3 accounts, W4
// ledger, and the commit log W6/W7 observers read.
type Store struct {
	Lists  map[int64][]int64
	Regs   map[int64]int64
	Accts  map[int64]int64
	Ledger map[int64]bool
	// Commits in commit order (effect time), for W7; LSN is assigned per
	// committed transaction.
	Commits []Commit
	lsn     int64
}

// Change is one row change of a committed transaction (W7).
type Change struct {
	Table string // cp.ledger or cp.append
	K, V  int64  // ledger: id, id; append: k and the appended element
	Vals  []int64
}

// Commit is one committed transaction that changed published tables.
type Commit struct {
	At      int64
	LSN     int64
	XID     int64
	Changes []Change
	Op      *Op
}

// NewStore returns the state after the setup transaction (CONTRACT §7).
func NewStore(accounts, initial int64) *Store {
	s := &Store{Lists: map[int64][]int64{}, Regs: map[int64]int64{}, Accts: map[int64]int64{}, Ledger: map[int64]bool{}, lsn: 0x1000000}
	for id := int64(0); id < accounts; id++ {
		s.Accts[id] = initial
	}
	return s
}

func (s *Store) commit(at int64, op *Op, chs []Change) {
	if len(chs) == 0 {
		return
	}
	s.lsn += 0x100
	s.Commits = append(s.Commits, Commit{At: at, LSN: s.lsn, XID: 1000 + int64(len(s.Commits)), Changes: chs, Op: op})
}

// txn executes W1 micro-ops ([append k v] / [r k nil]) and returns them with
// reads filled (nil when the key has no row).
func (s *Store) txn(at int64, op *Op, mops []any) []any {
	out := make([]any, len(mops))
	var chs []Change
	for i, m := range mops {
		mo := m.([]any)
		k := mo[1].(int64)
		if mo[0] == "append" {
			v := mo[2].(int64)
			s.Lists[k] = append(append([]int64{}, s.Lists[k]...), v)
			out[i] = []any{"append", k, v}
			chs = append(chs, Change{Table: "cp.append", K: k, V: v, Vals: append([]int64{}, s.Lists[k]...)})
			continue
		}
		if l, ok := s.Lists[k]; ok {
			out[i] = []any{"r", k, ints(l)}
		} else {
			out[i] = []any{"r", k, nil}
		}
	}
	s.commit(at, op, chs)
	return out
}

func (s *Store) register(f string, in map[string]any) map[string]any {
	k := in["k"].(int64)
	switch f {
	case "read":
		if v, ok := s.Regs[k]; ok {
			return map[string]any{"k": k, "v": v}
		}
		return map[string]any{"k": k, "v": nil}
	case "write":
		s.Regs[k] = in["v"].(int64)
		return map[string]any{"k": k, "v": in["v"]}
	case "cas":
		old, nw := in["old"].(int64), in["new"].(int64)
		v, ok := s.Regs[k]
		applied := ok && v == old
		if applied {
			s.Regs[k] = nw
		}
		return map[string]any{"k": k, "old": old, "new": nw, "applied": applied}
	}
	panic("register op " + f)
}

func (s *Store) bank(f string, in map[string]any) any {
	switch f {
	case "transfer":
		from, to, amt := in["from"].(int64), in["to"].(int64), in["amt"].(int64)
		applied := s.Accts[from] >= amt
		if applied {
			s.Accts[from] -= amt
			s.Accts[to] += amt
		}
		return map[string]any{"from": from, "to": to, "amt": amt, "applied": applied}
	case "read-all":
		return s.acctRows()
	}
	panic("bank op " + f)
}

func (s *Store) acctRows() []any {
	var ids []int64
	for id := range s.Accts {
		ids = append(ids, id)
	}
	sortInts(ids)
	out := []any{}
	for _, id := range ids {
		out = append(out, []any{id, s.Accts[id]})
	}
	return out
}

func (s *Store) insert(at int64, op *Op, in map[string]any) map[string]any {
	id := in["id"].(int64)
	s.Ledger[id] = true
	s.commit(at, op, []Change{{Table: "cp.ledger", K: id, V: id}})
	return map[string]any{"id": id, "payload": in["payload"]}
}

// Final returns the final-read values of the four tables (CONTRACT §6.4).
func (s *Store) Final() map[string]any {
	app := []any{}
	for _, k := range keys(s.Lists) {
		app = append(app, []any{k, ints(s.Lists[k])})
	}
	reg := []any{}
	for _, k := range keys(s.Regs) {
		reg = append(reg, []any{k, s.Regs[k]})
	}
	var ids []int64
	for id := range s.Ledger {
		ids = append(ids, id)
	}
	sortInts(ids)
	return map[string]any{"append": app, "register": reg, "bank": s.acctRows(), "ledger": ints(ids)}
}

func ints(xs []int64) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

func keys[V any](m map[int64]V) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sortInts(out)
	return out
}

func sortInts(xs []int64) { sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] }) }

func must(cond bool, f string, a ...any) {
	if !cond {
		panic(fmt.Sprintf(f, a...))
	}
}
