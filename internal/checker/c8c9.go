package checker

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
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
//   - notify-not-resumed: an ok NOTIFY not received within 10 s of its
//     completion by a listener client that had a live session over that
//     window (LISTEN completed before the NOTIFY was invoked; the session's
//     last op, or its first non-ok op, completed after the window), no fault
//     window overlapping it, and invoked after that session's first
//     successful op post-recovery (CONTRACT §10.9, ADR-0030): recovery from
//     the latest fault window ended before the NOTIFY is the completion of
//     the first ok write (W4 insert, NOTIFY transaction) through the
//     listener's gateway invoked after that window; without an earlier fault
//     the LISTEN itself is the anchor.
//   - notify-window-loss (observation): every other miss — no live session,
//     a fault window overlapping the 10 s, or before the anchor (S19's
//     reconnect windows).
//   - fair-share-starvation: fewer than 90% of the cp_steady operations
//     invoked within a storm window (first invoke to last completion of the
//     storm operations of one phase) completed ok.
//   - overload-failover: two consecutive ok cp_steady operations completing
//     within a storm window report different servers, and no nemesis fault
//     overlaps the window.
//
// From contract 1.2.0 on (plans carrying 1.2.0 or later; ADR-0037, the
// owner's decision on ADR-0035) fair-share-starvation counts only the
// cp_steady operations invoked outside every fault window; see
// judgedFairShare. Plans of earlier versions keep the rule above.
//
// cp_steady operations are W4 inserts: wl "ledger" with value role
// "cp_steady" (K5 runs, so that C1, C2 and C7 judge them as W4 inserts) or
// wl "overload" with that role (K2's synthetic form).
const notifyDeadline = 10 * time.Second

// The buffer's documented timing guard and maximum failover duration (S14
// defaults: --buffer-min-time-between-failovers 1 min,
// --buffer-max-failover-duration 20 s; CONTRACT §10.10, ADR-0029).
const (
	bufferMinBetween  = 60 * time.Second
	bufferMaxFailover = 20 * time.Second
)

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
				if s := gucs[g]; s != nil && s.known && !s.possible[got] && !sabotageSessionSetting {
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
				if !ok || sabotagePreparedStatement {
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
		start, end int64 // LISTEN completion; completion of its last op
		listen     int64
		gw         string
		oks        []*Op           // the session's ok ops, in invoke order
		recv       map[int64]int64 // payload id → invoke t of the wait that returned it
	}
	listeners := map[int64][]*session{} // client → sessions
	conns := byConn(ops)
	for _, conn := range sortedKeys(conns) {
		var cur *session
		for _, o := range conns[conn] {
			switch o.F {
			case "listen":
				if o.Type == history.TypeOK {
					cur = &session{start: o.Complete, end: o.Complete, listen: o.Index, gw: o.GW, oks: []*Op{o}, recv: map[int64]int64{}}
					listeners[o.Client] = append(listeners[o.Client], cur)
				}
			case "wait":
				if cur == nil {
					continue
				}
				cur.end = max(cur.end, o.Complete)
				if o.Type != history.TypeOK {
					cur = nil
					continue
				}
				cur.oks = append(cur.oks, o)
				ids, _ := IntList(obj(o.Value)["payloads"])
				for _, id := range ids {
					if _, seen := cur.recv[id]; !seen {
						cur.recv[id] = o.Invoke
					}
				}
			}
		}
	}
	var notifies, writes []*Op
	for _, o := range ops {
		if o.F == "notify" && o.Type == history.TypeOK {
			notifies = append(notifies, o)
		}
	}
	for _, o := range h.Ops {
		if o.Type == history.TypeOK && ((o.WL == "ledger" && o.F == "insert") || (o.WL == "notify" && o.F == "notify")) {
			writes = append(writes, o)
		}
	}
	faults := h.faultWindows()
	// anchor is the session's first successful op post-recovery for a
	// NOTIFY invoked at inv (ok false: none, the miss is window loss).
	anchor := func(s *session, inv int64) (int64, bool) {
		var prev *faultWindow
		for i := range faults {
			if faults[i].End < inv && (prev == nil || faults[i].End > prev.End) {
				prev = &faults[i]
			}
		}
		if prev == nil {
			return s.start, true
		}
		rec := int64(-1)
		for _, w := range writes {
			if w.GW == s.gw && w.Invoke > prev.End && (rec < 0 || w.Complete < rec) {
				rec = w.Complete
			}
		}
		if rec < 0 {
			return 0, false
		}
		for _, o := range s.oks {
			if o.Invoke > rec {
				return max(o.Complete, s.start), true
			}
		}
		return 0, false
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
		overlap := false
		for _, f := range faults {
			if f.Start <= deadline && f.End >= n.Invoke {
				overlap = true
			}
		}
		for _, cl := range clients {
			ss := listeners[cl]
			sort.SliceStable(ss, func(i, j int) bool { return ss[i].start < ss[j].start })
			delivered := false
			var liveS *session
			for _, s := range ss {
				if t, ok := s.recv[id]; ok && t <= deadline {
					delivered = true
				}
				if s.start < n.Invoke && s.end > deadline {
					liveS = s
				}
			}
			if delivered {
				continue
			}
			judged++
			why := ""
			switch {
			case liveS == nil:
				why = "had no live session over the window"
			case overlap && !sabotageNotifyAnchor:
				why = "had a fault window overlapping it (reconnect window)"
			default:
				if a, ok := anchor(liveS, n.Invoke); !sabotageNotifyAnchor && (!ok || n.Invoke <= a) {
					why = "was invoked before the session's first successful op post-recovery (reconnect window)"
				}
			}
			if why == "" {
				if !sabotageNotifyResume {
					c.add("notify-not-resumed", []int64{liveS.listen, n.Index}, "NOTIFY %d (i=%d) was not received within %s by listener client %d, whose session (LISTEN i=%d) was alive and past its first successful op after recovery", id, n.Index, notifyDeadline, cl, liveS.listen)
				}
				continue
			}
			windowLoss++
			c.observe("notify-window-loss", []int64{n.Index}, "NOTIFY %d (i=%d) was not received by listener client %d, which %s", id, n.Index, cl, why)
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
	for _, o := range h.opsOf("ledger") {
		if str(obj(o.InValue), "role") == "cp_steady" {
			steady = append(steady, o)
		}
	}
	sort.SliceStable(steady, func(i, j int) bool { return steady[i].Invoke < steady[j].Invoke })
	faults := h.faultWindows()
	judged := g12Judged(h.Plan)
	reason := ""
	var notJudged []string
	windows := []any{}
	for _, p := range sortedKeys(phases) {
		w := phases[p]
		if judged {
			ws, why, empty := judgedFairShare(c, p, w.start, w.end, steady, faults)
			windows = append(windows, ws)
			if why != "" {
				notJudged = append(notJudged, why)
			}
			if empty {
				continue
			}
		} else {
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
			if okN*10 < total*9 && !sabotageFairShare {
				c.add("fair-share-starvation", nil, "storm phase %d: %d of %d cp_steady operations completed ok (< 90%%)", p, okN, total)
			}
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
			if a != b && !sabotageOverloadFailover {
				c.add("overload-failover", []int64{oks[i].Index, oks[i+1].Index}, "storm phase %d: cp_steady served by %s (i=%d) then %s (i=%d) with no nemesis fault in the window", p, a, oks[i].Index, b, oks[i+1].Index)
				break
			}
		}
	}
	stats["storm_phases"] = int64(len(phases))
	if judged && len(phases) > 0 {
		stats["storm_windows"] = windows
		reason = strings.Join(notJudged, "; ")
	}
	return reason
}

// g12JudgedFrom is the first contract version whose plans judge G12's ok
// fraction outside fault windows (ADR-0037).
var g12JudgedFrom = [3]int64{1, 2, 0}

// g12Judged reports whether the plan carries contract 1.2.0 or later. A
// plan without a well-formed version keeps the literal rule, as every run
// recorded before ADR-0037 does.
func g12Judged(plan map[string]any) bool { return planAtLeast(plan, g12JudgedFrom) }

// planAtLeast reports whether the plan's contract_version is v or later; a
// plan without a well-formed version is not.
func planAtLeast(plan map[string]any, v [3]int64) bool {
	s, _ := plan["contract_version"].(string)
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return false
	}
	var n [3]int64
	for i, x := range parts {
		k, err := strconv.ParseInt(x, 10, 64)
		if err != nil || k < 0 {
			return false
		}
		n[i] = k
	}
	for i := range n {
		if n[i] != v[i] {
			return n[i] > v[i]
		}
	}
	return true
}

// judgedFairShare is G12's fair-share rule for plans of contract 1.2.0 or
// later (ADR-0037, the owner's decision on ADR-0035) over storm phase p's
// window [start, end]. L are the cp_steady operations invoked in it, O
// those invoked inside a fault window (first to last nemesis event of a
// fault id, as for G10), J = L \ O. fair-share-starvation if fewer than
// 90 % of J completed ok; otherwise the window is not judged — observation
// fair-share-not-judged and a reason that makes C8 unknown, never PASS — if
// J is empty or holds fewer than one operation per second of the window's
// time outside fault windows. O is the observation overload-fault-overlap.
// It returns the window's statistics (both fractions), the not-judged
// reason ("" if judged), and whether L is empty.
func judgedFairShare(c *collector, p, start, end int64, steady []*Op, faults []faultWindow) (map[string]any, string, bool) {
	var lit, litOK, jud, judOK int64
	var inside []int64
	ids := map[string]bool{}
	byType := map[string]int64{}
	for _, o := range steady {
		if o.Invoke < start || o.Invoke > end {
			continue
		}
		ok := o.Type == history.TypeOK
		lit++
		if ok {
			litOK++
		}
		if in := faultsAt(faults, o.Invoke); len(in) > 0 {
			inside = append(inside, o.Index)
			byType[o.Type]++
			for _, id := range in {
				ids[id] = true
			}
			if !sabotageG12FaultExclusion {
				continue
			}
		}
		jud++
		if ok {
			judOK++
		}
	}
	outside := end - start - coveredNS(start, end, faults)
	ws := map[string]any{"phase": p, "start": start, "end": end, "literal_total": lit, "literal_ok": litOK,
		"judged_total": jud, "judged_ok": judOK, "inside_fault_total": int64(len(inside)), "outside_fault_ns": outside}
	if len(inside) > 0 {
		c.observe("overload-fault-overlap", inside, "storm phase %d: %d cp_steady operations invoked inside fault windows %v (ok %d, fail %d, info %d), not judged (ADR-0037)",
			p, len(inside), sortedStrings(ids), byType[history.TypeOK], byType[history.TypeFail], byType[history.TypeInfo])
	}
	switch {
	case jud > 0 && judOK*10 < jud*9:
		ws["result"] = "fair-share-starvation"
		if !sabotageFairShare {
			c.add("fair-share-starvation", nil, "storm phase %d: %d of %d cp_steady operations invoked outside fault windows completed ok (< 90%%); literal fraction %d of %d (ADR-0037)",
				p, judOK, jud, litOK, lit)
		}
	case !sabotageG12JudgedMinimum && (jud == 0 || jud*int64(time.Second) < outside):
		ws["result"] = "not-judged"
		c.observe("fair-share-not-judged", nil, "storm phase %d: %d cp_steady operations invoked outside fault windows over %d ms of storm outside them (fewer than one per second), %d ok; literal fraction %d of %d (ADR-0037)",
			p, jud, outside/int64(time.Millisecond), judOK, litOK, lit)
		return ws, fmt.Sprintf("storm phase %d: not judged (%d cp_steady operations outside fault windows in %d ms)", p, jud, outside/int64(time.Millisecond)), lit == 0
	default:
		ws["result"] = "pass"
	}
	return ws, "", lit == 0
}

// faultsAt returns the ids of the fault windows containing t.
func faultsAt(faults []faultWindow, t int64) []string {
	var out []string
	for _, f := range faults {
		if f.Start <= t && t <= f.End {
			out = append(out, f.ID)
		}
	}
	return out
}

// coveredNS is the length of the union of [start, end]'s intersections
// with the fault windows (sorted by start).
func coveredNS(start, end int64, faults []faultWindow) int64 {
	var total int64
	curS, curE := int64(-1), int64(-1)
	for _, f := range faults {
		a, b := max(start, f.Start), min(end, f.End)
		if a > b {
			continue
		}
		if curE >= 0 && a <= curE {
			curE = max(curE, b)
			continue
		}
		if curE >= 0 {
			total += curE - curS
		}
		curS, curE = a, b
	}
	if curE >= 0 {
		total += curE - curS
	}
	return total
}

// CheckAvailability is C9 (CONTRACT §10.10). Stats: unavailability windows
// per fault and gateway, latency percentiles before/during/after faults,
// error class counts, and (plans of contract 1.7.0 or later, ADR-0044) the
// run's §12.3 exposure. G8 (profile buffered only): within an F6 switchover
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
	if planAtLeast(h.Plan, exposureFrom) {
		stats["exposure"] = Exposure(h)
	}
	if h.Profile == "buffered" {
		var judged, windows, unbuffered int64
		for wi, f := range faults {
			if f.Kind != "F6" {
				continue
			}
			// The documented timing guard (§10.10, ADR-0029): a switchover
			// less than bufferMinBetween after an earlier fault window's
			// buffering could have ended is not buffered by design.
			buffered := true
			for _, v := range faults[:wi] {
				if f.Start < max(v.End, v.Start+int64(bufferMaxFailover))+int64(bufferMinBetween) {
					buffered = false
				}
			}
			if sabotageBufferGuard {
				buffered = true
			}
			if buffered {
				windows++
			} else {
				unbuffered++
			}
			flag := func(ops []int64, format string, a ...any) {
				if !buffered {
					c.observe("planned-failover-unbuffered", ops, format+" (not buffered: within the timing guard of an earlier fault window)", a...)
					return
				}
				if !sabotagePlannedFailover {
					c.add("planned-failover-error", ops, format, a...)
				}
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
					flag([]int64{o.Index}, "autocommit %s %s i=%d completed %s (%v) within switchover %s", o.WL, o.F, o.Index, o.Type, o.Err, f.ID)
				case explicit:
					judged++
					if !(o.Type == history.TypeFail && o.SQLState() == "40001") {
						flag([]int64{o.Index}, "explicit transaction %s i=%d completed %s (%v), not 40001, within switchover %s", o.WL, o.Index, o.Type, o.Err, f.ID)
					}
				}
			}
		}
		stats["g8_errors_judged"] = judged
		stats["g8_windows_judged"], stats["g8_windows_unbuffered"] = windows, unbuffered
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
