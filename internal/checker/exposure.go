package checker

import "github.com/CesarManzoCode/commitproof/internal/history"

// exposureFrom is the first contract version whose plans carry the run's
// exposure in C9's stats (ADR-0044).
var exposureFrom = [3]int64{1, 7, 0}

// Exposure is a run's §12.3 exposure with ADR-0036's definitions: operations
// (every invoke but final reads), ok writes (ok ledger inserts, the append
// micro-ops of ok append operations, ok register write and cas, ok bank
// transfers), confirmed faults by kind, and fault-time (Σ heal t −
// confirmed t over the faults with both). Stats only (C9, an observation
// checker): it never decides a verdict.
func Exposure(h *History) map[string]any {
	var ops, okWrites, faultTime int64
	for _, o := range h.Ops {
		if o.F == "final-read" {
			continue
		}
		ops++
		if o.Type != history.TypeOK {
			continue
		}
		switch {
		case o.WL == "ledger" && o.F == "insert", o.WL == "register" && (o.F == "write" || o.F == "cas"), o.WL == "bank" && o.F == "transfer":
			okWrites++
		case o.WL == "append":
			mops, _ := o.InValue.([]any)
			for _, x := range mops {
				if mo, _ := x.([]any); len(mo) == 3 && mo[0] == "append" {
					okWrites++
				}
			}
		}
	}
	confirmed := map[string]int64{}
	byKind := map[string]int64{}
	for _, ev := range h.Nemesis {
		id, _ := ev.Fault["id"].(string)
		switch ev.Fault["phase"] {
		case "confirmed":
			confirmed[id] = ev.T
			kind, _ := ev.Fault["kind"].(string)
			byKind[kind]++
		case "heal":
			if t, ok := confirmed[id]; ok {
				faultTime += ev.T - t
			}
		}
	}
	return map[string]any{"operations": ops, "ok_writes": okWrites, "confirmed_faults": intMap(byKind), "fault_time_ns": faultTime}
}
