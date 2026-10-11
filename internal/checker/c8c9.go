package checker

import (
	"fmt"
	"sort"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/history"
)

// C8 session-state (CONTRACT §10.9) reads these synthetic forms (the K5
// clients W5, W6, W8 must produce them):
//
//	W5 wl "session"  set {conn, guc, value}; show {conn, guc} → ok
//	                 {conn, guc, value}; prepare {conn, name, expect};
//	                 execute {conn, name} → ok {conn, name, row}
//	W6 wl "notify"   notify {id} (a transaction inserting ledger row id and
//	                 NOTIFY cp_chan, 'id'); listen {conn} (LISTEN cp_chan);
//	                 wait {conn} → ok {conn, payloads: [id…]} (notifications
//	                 received during the wait), fail/info ends the session
//	W8 wl "overload" storm {role "cp_storm", phase}; steady {role
//	                 "cp_steady", id} → ok {…, server} (the address of the
//	                 PostgreSQL server that executed it)
//
// conn identifies one client connection to a gateway; session state is
// expected to survive backend failovers within a conn, and starts empty on
// a new conn.
//
//   - session-setting-lost: an ok SHOW in a conn returns a value other than
//     the one(s) its earlier SETs may have installed. Checked at every ok
//     SHOW, not only after a failover: no leader-change information is
//     needed (only observations carry it), and the rule must hold at all
//     times.
//   - prepared-statement-lost: after an ok PREPARE in a conn, an EXECUTE
//     fails with 26000 or returns a row other than the expected one.
//   - notify-not-resumed: an ok NOTIFY invoked after a listener session's
//     LISTEN completed, while that session stayed alive for 10 s after the
//     NOTIFY completed, was not received within those 10 s.
//   - notify-window-loss (observation): an ok NOTIFY not received by a
//     listener that had no live session over that 10 s window.
//   - fair-share-starvation: fewer than 90% of the cp_steady operations
//     invoked within a storm window (first invoke to last completion of the
//     storm operations of one phase) completed ok.
//   - overload-failover: two consecutive ok cp_steady operations completing
//     within a storm window report different servers, and no nemesis fault
//     overlaps the window.
const notifyDeadline = 10 * time.Second

// CheckSession is C8.
func CheckSession(h *History) Result {
	c := newCollector()
	stats := map[string]any{}
	unknown := ""
	sessionRules(h, c, stats)
	notifyRules(h, c, stats)
	if r := overloadRules(h, c, stats); r != "" {
		unknown = r
	}
	return c.result("C8", unknown, stats)
}

func byConn(ops []*Op) map[int64][]*Op {
	out := map[int64][]*Op{}
	for _, o := range ops {
		if conn, ok := Int(obj(o.InValue)["conn"]); ok {
			out[conn] = append(out[conn], o)
		}
	}
	for _, os := range out {
		sort.SliceStable(os, func(i, j int) bool { return os[i].Invoke < os[j].Invoke })
	}
	return out
}

func sessionRules(h *History, c *collector, stats map[string]any) {
	conns := byConn(h.opsOf("session"))
	var shows, executes int64
	for _, conn := range sortedKeys(conns) {
		type gucState struct {
			known    bool
			possible map[string]bool
			setOps   []int64
		}
		gucs := map[string]*gucState{}
		prepared := map[string]any{} // name → expected row (known prepares only)
		prepOp := map[string]int64{}
		for _, o := range conns[conn] {
			in := obj(o.InValue)
			switch o.F {
			case "set":
				g, v := str(in, "guc"), str(in, "value")
				s := gucs[g]
				if s == nil {
					s = &gucState{possible: map[string]bool{}}
					gucs[g] = s
				}
				switch o.Type {
				case history.TypeOK:
					s.known, s.possible, s.setOps = true, map[string]bool{v: true}, []int64{o.Index}
				case history.TypeInfo:
					if s.known {
						s.possible[v] = true
						s.setOps = append(s.setOps, o.Index)
					}
				}
			case "show":
				if o.Type != history.TypeOK {
					continue
				}
				shows++
				g := str(in, "guc")
				got := str(obj(o.Value), "value")
				if s := gucs[g]; s != nil && s.known && !s.possible[got] {
					c.add("session-setting-lost", append([]int64{o.Index}, s.setOps...), "conn %d: SHOW %s i=%d returned %q; earlier SETs installed %v", conn, g, o.Index, got, sortedStrings(s.possible))
				}
			case "prepare":
				name := str(in, "name")
				switch o.Type {
				case history.TypeOK:
					prepared[name], prepOp[name] = in["expect"], o.Index
				case history.TypeInfo:
					delete(prepared, name)
				}
			case "execute":
				name := str(in, "name")
				want, ok := prepared[name]
				if !ok {
					continue
				}
				executes++
				switch {
				case o.Type != history.TypeOK && o.SQLState() == "26000":
					c.add("prepared-statement-lost", []int64{prepOp[name], o.Index}, "conn %d: EXECUTE %s i=%d failed with 26000 after PREPARE i=%d", conn, name, o.Index, prepOp[name])
				case o.Type == history.TypeOK && !sameJSON(obj(o.Value)["row"], want):
					c.add("prepared-statement-lost", []int64{prepOp[name], o.Index}, "conn %d: EXECUTE %s i=%d returned %v, PREPARE i=%d expects %v", conn, name, o.Index, obj(o.Value)["row"], prepOp[name], want)
				}
			}
		}
	}
	stats["shows_checked"], stats["executes_checked"] = shows, executes
}

func sameJSON(a, b any) bool {
	x, e1 := canonjson.Marshal(a)
	y, e2 := canonjson.Marshal(b)
	return e1 == nil && e2 == nil && string(x) == string(y)
}

func sortedStrings(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func notifyRules(h *History, c *collector, stats map[string]any) {
	ops := h.opsOf("notify")
	type session struct {
		start, end int64 // LISTEN completion; end of the session
		listen     int64
		recv       map[int64]int64 // payload id → invoke t of the wait that returned it
	}
	listeners := map[int64][]*session{} // client → sessions
	var notifies []*Op
	conns := byConn(ops)
	for _, conn := range sortedKeys(conns) {
		var cur *session
		for _, o := range conns[conn] {
			switch o.F {
			case "listen":
				if o.Type == history.TypeOK {
					cur = &session{start: o.Complete, end: Never, listen: o.Index, recv: map[int64]int64{}}
					listeners[o.Client] = append(listeners[o.Client], cur)
				}
			case "wait":
				if cur == nil {
					continue
				}
				if o.Type != history.TypeOK {
					cur.end = o.Complete
					cur = nil
					continue
				}
				ids, _ := IntList(obj(o.Value)["payloads"])
				for _, id := range ids {
					if _, seen := cur.recv[id]; !seen {
						cur.recv[id] = o.Invoke
					}
				}
			}
		}
	}
	for _, o := range ops {
		if o.F == "notify" && o.Type == history.TypeOK {
			notifies = append(notifies, o)
		}
	}
	clients := sortedKeys(listeners)
	var judged, windowLoss int64
	for _, n := range notifies {
		id, ok := Int(obj(n.InValue)["id"])
		if !ok {
			continue
		}
		deadline := n.Complete + int64(notifyDeadline)
		if deadline > h.End {
			continue // not enough history after the NOTIFY to judge it
		}
		for _, cl := range clients {
			ss := listeners[cl]
			sort.SliceStable(ss, func(i, j int) bool { return ss[i].start < ss[j].start })
			delivered, live := false, false
			var liveS *session
			for _, s := range ss {
				if t, ok := s.recv[id]; ok && t <= deadline {
					delivered = true
				}
				if s.start < n.Invoke && s.end > deadline {
					live, liveS = true, s
				}
			}
			if delivered {
				continue
			}
			judged++
			if live {
				c.add("notify-not-resumed", []int64{liveS.listen, n.Index}, "NOTIFY %d (i=%d) was not received within %s by listener client %d, whose session (LISTEN i=%d) was alive", id, n.Index, notifyDeadline, cl, liveS.listen)
			} else {
				windowLoss++
				c.observe("notify-window-loss", []int64{n.Index}, "NOTIFY %d (i=%d) was not received by listener client %d, which had no live session over the %s window", id, n.Index, cl, notifyDeadline)
			}
		}
	}
	stats["notifies"], stats["notify_misses_judged"], stats["notify_window_loss"] = int64(len(notifies)), judged, windowLoss
}

func overloadRules(h *History, c *collector, stats map[string]any) string {
	ops := h.opsOf("overload")
	type window struct{ start, end int64 }
	phases := map[int64]*window{}
	var steady []*Op
	for _, o := range ops {
		in := obj(o.InValue)
		switch str(in, "role") {
		case "cp_storm":
			p, _ := Int(in["phase"])
			w := phases[p]
			if w == nil {
				w = &window{o.Invoke, o.Complete}
				phases[p] = w
			}
			w.start, w.end = min(w.start, o.Invoke), max(w.end, o.Complete)
		case "cp_steady":
			steady = append(steady, o)
		}
	}
	faults := h.faultWindows()
	reason := ""
	for _, p := range sortedKeys(phases) {
		w := phases[p]
		var total, okN int64
		for _, o := range steady {
			if o.Invoke >= w.start && o.Invoke <= w.end {
				total++
				if o.Type == history.TypeOK {
					okN++
				}
			}
		}
		if total == 0 {
			reason = fmt.Sprintf("storm phase %d: no cp_steady operation was invoked in its window", p)
			continue
		}
		if okN*10 < total*9 {
			c.add("fair-share-starvation", nil, "storm phase %d: %d of %d cp_steady operations completed ok (< 90%%)", p, okN, total)
		}
		faulted := false
		for _, f := range faults {
			if f.Start <= w.end && f.End >= w.start {
				faulted = true
			}
		}
		if faulted {
			continue
		}
		var oks []*Op
		for _, o := range steady {
			if o.Type == history.TypeOK && o.Complete >= w.start && o.Complete <= w.end {
				oks = append(oks, o)
			}
		}
		sort.SliceStable(oks, func(i, j int) bool { return oks[i].Complete < oks[j].Complete })
		for i := 0; i+1 < len(oks); i++ {
			a, b := str(obj(oks[i].Value), "server"), str(obj(oks[i+1].Value), "server")
			if a != b {
				c.add("overload-failover", []int64{oks[i].Index, oks[i+1].Index}, "storm phase %d: cp_steady served by %s (i=%d) then %s (i=%d) with no nemesis fault in the window", p, a, oks[i].Index, b, oks[i+1].Index)
				break
			}
		}
	}
	stats["storm_phases"] = int64(len(phases))
	return reason
}

// CheckAvailability is C9 (CONTRACT §10.10). Stats: unavailability windows
// per fault and gateway, latency percentiles before/during/after faults,
// error class counts. G8 (profile buffered only): within an F6 switchover
// window (inject to healed; an operation is within it if [invoke,
// completion] overlaps it), a fail or info on a single autocommit statement
// (W4 insert, W2 op, autocommit append) is planned-failover-error; an
// explicit transaction (W1 txn, W3) may fail only with 40001.
func CheckAvailability(h *History) Result {
	c := newCollector()
	faults := h.faultWindows()
	stats := map[string]any{"faults": int64(len(faults)), "profile": h.Profile}
	errClasses := map[string]int64{}
	for _, o := range h.Ops {
		if o.Type != history.TypeOK {
			cl, _ := o.Err["class"].(string)
			errClasses[o.Type+"/"+cl]++
		}
	}
	stats["errors"] = intMap(errClasses)
	stats["unavailability_ms"] = unavailability(h, faults)
	stats["latency_us"] = latencies(h, faults)
	if h.Profile == "buffered" {
		var judged int64
		for _, f := range faults {
			if f.Kind != "F6" {
				continue
			}
			s, e := f.Start, f.End
			for _, o := range h.Ops {
				// The client observes an outcome at the completion event, also
				// for info ops (their effect time is unbounded, but the error the
				// client saw is not).
				if o.F == "final-read" || o.Type == history.TypeOK || o.Invoke > e || o.Complete < s {
					continue
				}
				explicit := (o.WL == "append" && o.F == "txn") || o.WL == "bank"
				auto := o.WL == "ledger" || o.WL == "register" || (o.WL == "append" && o.F != "txn")
				switch {
				case auto:
					judged++
					c.add("planned-failover-error", []int64{o.Index}, "autocommit %s %s i=%d completed %s (%v) within switchover %s", o.WL, o.F, o.Index, o.Type, o.Err, f.ID)
				case explicit:
					judged++
					if !(o.Type == history.TypeFail && o.SQLState() == "40001") {
						c.add("planned-failover-error", []int64{o.Index}, "explicit transaction %s i=%d completed %s (%v), not 40001, within switchover %s", o.WL, o.Index, o.Type, o.Err, f.ID)
					}
				}
			}
		}
		stats["g8_errors_judged"] = judged
	}
	return c.result("C9", "", stats)
}

func unavailability(h *History, faults []faultWindow) map[string]any {
	out := map[string]any{}
	for _, f := range faults {
		per := map[string]any{}
		for _, gw := range h.Gateways() {
			var firstBad, firstOK int64 = -1, -1
			for _, o := range h.Ops {
				if o.GW != gw || o.F == "final-read" || o.Complete < f.Start {
					continue
				}
				if o.Type != history.TypeOK && firstBad < 0 {
					firstBad = o.Complete
				}
				if o.Type == history.TypeOK && firstBad >= 0 && o.Complete >= f.End && (firstOK < 0 || o.Complete < firstOK) {
					firstOK = o.Complete
				}
			}
			if firstBad >= 0 && firstOK >= 0 {
				per[gw] = (firstOK - firstBad) / int64(time.Millisecond)
			} else if firstBad >= 0 {
				per[gw] = "no ok op after heal"
			} else {
				per[gw] = int64(0)
			}
		}
		out[f.ID] = per
	}
	return out
}

func latencies(h *History, faults []faultWindow) map[string]any {
	phase := func(o *Op) string {
		in := false
		for _, f := range faults {
			if o.Invoke <= f.End && o.Complete >= f.Start {
				in = true
			}
		}
		switch {
		case in:
			return "during"
		case len(faults) > 0 && o.Invoke < faults[0].Start:
			return "before"
		case len(faults) > 0:
			return "after"
		}
		return "no-fault"
	}
	byPhase := map[string][]int64{}
	for _, o := range h.Ops {
		if o.Type == history.TypeOK && o.F != "final-read" {
			p := phase(o)
			byPhase[p] = append(byPhase[p], (o.Complete-o.Invoke)/int64(time.Microsecond))
		}
	}
	out := map[string]any{}
	for p, ls := range byPhase {
		sort.Slice(ls, func(i, j int) bool { return ls[i] < ls[j] })
		pct := func(q int) int64 { return ls[(len(ls)-1)*q/100] }
		out[p] = map[string]any{"n": int64(len(ls)), "p50": pct(50), "p95": pct(95), "p99": pct(99)}
	}
	return out
}
