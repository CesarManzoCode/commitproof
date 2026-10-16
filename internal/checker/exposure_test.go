package checker

import (
	"reflect"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

// ADR-0044: C9 carries the run's exposure (ADR-0036's definitions) for
// plans of contract 1.7.0 or later, and nothing for earlier plans. The
// expected numbers are counted by hand from the history below.
func TestExposure(t *testing.T) {
	i64 := func(n int64) *int64 { return &n }
	var evs []history.Event
	next := int64(0)
	op := func(t, done int64, typ, wl, f string, in any) {
		inv := next
		evs = append(evs, history.Event{I: inv, T: t, Type: history.TypeInvoke, Proc: i64(inv), Client: i64(0), GW: "gw-z1", WL: wl, F: f, Value: in})
		evs = append(evs, history.Event{I: inv + 1, T: done, Type: typ, Op: i64(inv), Value: in})
		next += 2
	}
	nem := func(t int64, id, kind, phase string) {
		evs = append(evs, history.Event{I: next, T: t, Type: history.TypeNemesis, Fault: map[string]any{"id": id, "kind": kind, "phase": phase}})
		next++
	}
	app := func(ks ...any) []any { return ks }
	// ok writes: 1 ledger insert + 2 appends of an ok txn + 1 register write + 1 cas + 1 transfer = 6.
	op(10, 20, history.TypeOK, "ledger", "insert", map[string]any{"id": int64(1)})
	op(10, 20, history.TypeOK, "append", "txn", []any{app("append", int64(1), int64(1)), app("r", int64(2), nil), app("append", int64(3), int64(1))})
	op(10, 20, history.TypeOK, "register", "write", map[string]any{"k": int64(1), "v": int64(5)})
	op(10, 20, history.TypeOK, "register", "cas", map[string]any{"k": int64(1), "old": int64(5), "new": int64(6)})
	op(10, 20, history.TypeOK, "register", "read", map[string]any{"k": int64(1)})
	op(10, 20, history.TypeOK, "bank", "transfer", map[string]any{"from": int64(0), "to": int64(1), "amt": int64(1)})
	op(10, 20, history.TypeOK, "bank", "read-all", map[string]any{})
	// Not ok writes: an info insert, a failed append txn.
	op(10, 20, history.TypeInfo, "ledger", "insert", map[string]any{"id": int64(2)})
	op(10, 20, history.TypeFail, "append", "txn", []any{app("append", int64(4), int64(1))})
	// Faults: f0 confirmed at 100, healed (heal at 400); f1 confirmed, never healed (no fault-time); f2 not confirmed.
	nem(90, "f0", "F1a", "inject")
	nem(100, "f0", "F1a", "confirmed")
	nem(400, "f0", "F1a", "heal")
	nem(450, "f0", "F1a", "healed")
	nem(500, "f1", "F5a", "inject")
	nem(600, "f1", "F5a", "confirmed")
	nem(700, "f2", "F2a", "inject")
	nem(800, "f2", "F2a", "not-applied")
	// A final read: not an operation.
	op(900, 910, history.TypeOK, "ledger", "final-read", map[string]any{})
	want := map[string]any{"operations": int64(9), "ok_writes": int64(6), "confirmed_faults": map[string]any{"F1a": int64(1), "F5a": int64(1)},
		"fault_time_ns": int64(300)}
	for _, c := range []struct {
		version string
		present bool
	}{{"1.6.0", false}, {"1.7.0", true}, {"1.10.0", true}, {"", false}} {
		h, err := FromEvents(evs, map[string]any{"contract_version": c.version, "profile": "baseline"})
		if err != nil {
			t.Fatal(err)
		}
		r := CheckAvailability(h)
		got, ok := r.Stats["exposure"]
		if ok != c.present {
			t.Fatalf("plan %q: exposure present %t, want %t", c.version, ok, c.present)
		}
		if ok && !reflect.DeepEqual(got, want) {
			t.Errorf("plan %q: exposure %v, want %v", c.version, got, want)
		}
		if r.Valid != True {
			t.Errorf("plan %q: C9 %v; exposure must not decide validity", c.version, r.Valid)
		}
	}
}
