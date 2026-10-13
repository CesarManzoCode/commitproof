package synth

import (
	"fmt"
	"sort"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

// K5 corpus (CONTRACT §15 K5.1; ADR-0029, ADR-0030): testdata/corpus-k5.
// Histories in the forms the K5 clients write — W5 sessions set up again on
// every connection, W6 listeners that wait in 1 s steps with notifiers on
// every gateway, W7 with commit end LSNs and reconnects at the confirmed
// flush, W8 steady inserts as ledger inserts with their role, C8's buffered
// switchovers under the timing guard — with nemesis fault windows. As for
// the K2 corpus, every expectation is written per plant from CONTRACT §10
// (here, never from running the checkers).

// K5CorpusContractVersion is the contract version of testdata/corpus-k5.
const K5CorpusContractVersion = "1.0.9"

// K5 corpus families and labels. A label names what was planted; "clean"
// histories have no anomaly and no observation.
const (
	LabelClean           = "clean"
	LabelReconnectWindow = "notify-reconnect-window" // observation notify-window-loss only
	LabelUnbuffered      = "planned-failover-unbuffered"
)

// K5Corpus returns every entry of testdata/corpus-k5, in id order.
func K5Corpus() []Entry {
	var es []Entry
	add := func(id string, f func() *Run) { es = append(es, Entry{id, f}) }
	for n := 1; n <= 25; n++ {
		id := fmt.Sprintf("w5k-clean-%03d", n)
		add(id, func() *Run { return w5kRun(id, LabelClean, "", n%2 == 0) })
		id6 := fmt.Sprintf("w6k-clean-%03d", n)
		add(id6, func() *Run { return w6kRun(id6, LabelClean, "") })
		id7 := fmt.Sprintf("w7k-clean-%03d", n)
		add(id7, func() *Run { return w7kRun(id7, LabelClean, "") })
		id8 := fmt.Sprintf("w8k-clean-%03d", n)
		add(id8, func() *Run { return w8kRun(id8, LabelClean, []string{"none", "fault-in-storm", "fault-between"}[n%3]) })
	}
	for n := 1; n <= 15; n++ {
		id := fmt.Sprintf("g8k-clean-%03d", n)
		add(id, func() *Run { return g8kRun(id, LabelClean, "") })
	}
	for n := 1; n <= 3; n++ {
		id := fmt.Sprintf("w5k-setting-lost-%02d", n)
		add(id, func() *Run { return w5kRun(id, "session-setting-lost", "setting-lost", n == 2) })
		for _, k := range []string{"prepared-26000", "prepared-row"} {
			id := fmt.Sprintf("w5k-%s-%02d", k, n)
			if k == "prepared-row" && n == 3 {
				continue
			}
			add(id, func() *Run { return w5kRun(id, "prepared-statement-lost", k, n == 1) })
		}
		for _, k := range []string{"gap", "phantom", "order"} {
			id := fmt.Sprintf("w7k-%s-%02d", k, n)
			add(id, func() *Run { return w7kRun(id, "cdc-"+k, k) })
		}
		dup := "duplicate-boundary"
		if n == 3 {
			dup = "duplicate-below"
		}
		idd := fmt.Sprintf("w7k-%s-%02d", dup, n)
		add(idd, func() *Run { return w7kRun(idd, "cdc-duplicate", dup) })
		for _, k := range []string{"starvation", "failover"} {
			id := fmt.Sprintf("w8k-%s-%02d", k, n)
			add(id, func() *Run {
				return w8kRun(id, map[string]string{"starvation": "fair-share-starvation", "failover": "overload-failover"}[k], k)
			})
		}
	}
	for n := 1; n <= 4; n++ {
		id := fmt.Sprintf("w6k-reconnect-window-%02d", n)
		add(id, func() *Run { return w6kRun(id, LabelReconnectWindow, "reconnect-window") })
		kind := []string{"never", "never", "one", "after-second"}[n-1]
		idn := fmt.Sprintf("w6k-not-resumed-%s-%02d", kind, n)
		add(idn, func() *Run { return w6kRun(idn, "notify-not-resumed", "not-resumed-"+kind) })
		idu := fmt.Sprintf("g8k-unbuffered-%02d", n)
		add(idu, func() *Run { return g8kRun(idu, LabelUnbuffered, "unbuffered") })
		kg := []string{"autocommit-info", "autocommit-fail", "explicit-info", "explicit-deadlock"}[n-1]
		idg := fmt.Sprintf("g8k-%s-%02d", kg, n)
		add(idg, func() *Run { return g8kRun(idg, "planned-failover-error", kg) })
	}
	sort.Slice(es, func(i, j int) bool { return es[i].ID < es[j].ID })
	return es
}

func k5Run(id, family, label string) *Run {
	r := NewRun(id, family, label)
	r.Version = K5CorpusContractVersion
	return r
}

// k5Fault adds one fault window: inject at from, confirmed, heal, healed at
// to (the window is the first to the last event, CONTRACT §10.9).
func (r *Run) k5Fault(id, kind, target string, from, to int64) {
	r.Nemesis(from, id, kind, "inject", target)
	r.Nemesis(from+5*ms, id, kind, "confirmed", target)
	r.Nemesis(to-1*ms, id, kind, "heal", target)
	r.Nemesis(to, id, kind, "healed", target)
}

func overlaps(ws [][2]int64, a, b int64) bool {
	for _, w := range ws {
		if a <= w[1] && b >= w[0] {
			return true
		}
	}
	return false
}

// k5op appends one completed operation.
func (r *Run) k5op(client int64, gw, wl, f string, in, out any, inv, dur int64) *Op {
	o := &Op{Client: client, GW: gw, WL: wl, F: f, In: in, Out: out, Type: history.TypeOK, Applied: true, Inv: inv, Eff: inv + dur/2, Cmp: inv + dur}
	r.Ops = append(r.Ops, o)
	return o
}

var errNoPrimary = [3]string{"MTF01", "MT", "statement"}

// --- W5 (ADR-0030: state set up again on every connection) ------------------

var k5GUCs = []string{"application_name", "search_path", "work_mem", "DateStyle"}

func w5kRun(id, label, plant string, transport bool) *Run {
	r := k5Run(id, "w5k", label)
	r.Workloads = []string{"W5"}
	r.Plant = plant
	wins := [][2]int64{{r.T0 + 600*ms, r.T0 + 900*ms}, {r.T0 + 1500*ms, r.T0 + 1800*ms}}
	r.k5Fault("f0", "F2a", "pg-n1", wins[0][0], wins[0][1])
	r.k5Fault("f1", "F6", "pooler-n2", wins[1][0], wins[1][1])
	end := r.T0 + 2500*ms
	var shows, execs []*Op // plant candidates after the second window, outside windows
	for c := int64(20); c < 23; c++ {
		gw := Gateways[c-20]
		vals := map[string]string{}
		for _, g := range k5GUCs {
			pool := gucValues[g]
			vals[g] = pool[r.R.IntN(len(pool))]
		}
		type stmt struct {
			name, kind string
			v          int64
		}
		var stmts []stmt
		for j := int64(1); j <= 4; j++ {
			v := c*10 + j
			if j <= 2 {
				stmts = append(stmts, stmt{fmt.Sprintf("cp_s%d", v), "sql", v})
			} else {
				stmts = append(stmts, stmt{fmt.Sprintf("cp_p%d", v), "protocol", v})
			}
		}
		conn := c * 1000
		var state map[string]string
		var prepared map[string]int64
		t := r.T0 + (c-20)*us
		setup := true
		op := func(f string, in, out map[string]any) *Op {
			d := 1*ms + int64(r.R.IntN(3))*ms
			o := r.k5op(c, gw, "session", f, in, out, t, d)
			t = o.Cmp + 5*ms + int64(r.R.IntN(10))*ms
			if overlaps(wins, o.Inv, o.Cmp) {
				o.Fail(errNoPrimary[0], errNoPrimary[1], errNoPrimary[2])
				o.Out = nil
			}
			return o
		}
		for t < end {
			if setup {
				conn++
				state, prepared = map[string]string{}, map[string]int64{}
				for _, g := range k5GUCs {
					in := map[string]any{"conn": conn, "guc": g, "value": vals[g]}
					if o := op("set", in, in); o.Type == history.TypeOK {
						state[g] = vals[g]
					}
				}
				for _, st := range stmts {
					in := map[string]any{"conn": conn, "name": st.name, "kind": st.kind, "sql": fmt.Sprintf("SELECT %d::bigint", st.v), "expect": []any{st.v}}
					if o := op("prepare", in, in); o.Type == history.TypeOK {
						prepared[st.name] = st.v
					}
				}
				setup = false
				continue
			}
			x := r.R.IntN(100)
			var o *Op
			switch {
			case x < 45:
				g := k5GUCs[r.R.IntN(len(k5GUCs))]
				v, known := state[g]
				if !known {
					v = "server-default"
				}
				o = op("show", map[string]any{"conn": conn, "guc": g}, map[string]any{"conn": conn, "guc": g, "value": v})
				if known && o.Type == history.TypeOK && o.Inv > wins[1][1] {
					shows = append(shows, o)
				}
			case x < 90:
				st := stmts[r.R.IntN(len(stmts))]
				v, ok := prepared[st.name]
				o = op("execute", map[string]any{"conn": conn, "name": st.name}, map[string]any{"conn": conn, "name": st.name, "row": []any{v}})
				if o.Type == history.TypeOK && !ok {
					// Not prepared on this connection: 26000, which C8 does
					// not judge (no ok PREPARE on the connection, §10.9).
					o.Type, o.Out = history.TypeInfo, nil
					o.Err = map[string]any{"sqlstate": "26000", "class": "26", "phase": "statement"}
				}
				if ok && o.Type == history.TypeOK && o.Inv > wins[1][1] {
					execs = append(execs, o)
				}
			default:
				g := k5GUCs[r.R.IntN(len(k5GUCs))]
				pool := gucValues[g]
				v := pool[r.R.IntN(len(pool))]
				in := map[string]any{"conn": conn, "guc": g, "value": v}
				if o = op("set", in, in); o.Type == history.TypeOK {
					state[g], vals[g] = v, v
				}
			}
			// A transport failure ends the connection (show and execute only).
			if transport && o.Type == history.TypeOK && (o.F == "show" || o.F == "execute") && r.R.IntN(70) == 0 {
				o.Type, o.Out = history.TypeInfo, nil
				o.Err = map[string]any{"sqlstate": "", "class": "transport", "phase": "transport"}
				setup = true
				shows, execs = dropOp(shows, o), dropOp(execs, o)
			}
		}
	}
	switch plant {
	case "setting-lost": // after both failovers an ok SHOW returns a value no SET on its connection installed
		must(len(shows) > 0, "%s: no SHOW to plant on", id)
		o := shows[0]
		m := o.Out.(map[string]any)
		o.Out = map[string]any{"conn": m["conn"], "guc": m["guc"], "value": "server-default"}
		r.Expect("C8", "session-setting-lost")
	case "prepared-26000": // an EXECUTE of a statement prepared ok on its connection fails with 26000
		must(len(execs) > 0, "%s: no EXECUTE to plant on", id)
		o := execs[0]
		o.Type, o.Out = history.TypeInfo, nil
		o.Err = map[string]any{"sqlstate": "26000", "class": "26", "phase": "statement"}
		r.Expect("C8", "prepared-statement-lost")
	case "prepared-row": // an EXECUTE returns another statement's row
		must(len(execs) > 0, "%s: no EXECUTE to plant on", id)
		o := execs[0]
		m := o.Out.(map[string]any)
		row := m["row"].([]any)
		o.Out = map[string]any{"conn": m["conn"], "name": m["name"], "row": []any{row[0].(int64) + 100000}}
		r.Expect("C8", "prepared-statement-lost")
	}
	return r
}

func dropOp(os []*Op, x *Op) []*Op {
	out := os[:0]
	for _, o := range os {
		if o != x {
			out = append(out, o)
		}
	}
	return out
}

// --- W6 (ADR-0030: notifiers on every gateway, 1 s waits) -----------------

func w6kRun(id, label, plant string) *Run {
	r := k5Run(id, "w6k", label)
	r.Workloads = []string{"W6"}
	r.Plant = plant
	wins := [][2]int64{{r.T0 + 5*s_, r.T0 + 7*s_}, {r.T0 + 20*s_, r.T0 + 27*s_}}
	r.k5Fault("f0", "F2a", "pg-n1", wins[0][0], wins[0][1])
	r.k5Fault("f1", "F6", "pooler-n1", wins[1][0], wins[1][1])
	end := r.T0 + 40*s_
	nid := int64(0)
	for c := int64(23); c < 26; c++ {
		gw := Gateways[c-23]
		for t := r.T0 + (c-23)*500*ms; t < end; t += 1500*ms + int64(r.R.IntN(20))*ms {
			nid++
			in := map[string]any{"id": nid, "payload": fmt.Sprintf("n%d-%016x", c, r.R.Uint64())}
			o := r.k5op(c, gw, "notify", "notify", in, nil, t, 2*ms+int64(r.R.IntN(4))*ms)
			if overlaps(wins, o.Inv, o.Cmp) {
				o.Fail(errNoPrimary[0], errNoPrimary[1], errNoPrimary[2])
				continue
			}
			o.Exec = func(s *Store, at int64) any { return s.insert(at, o, in) }
		}
	}
	// drop reports whether listener l never receives notification n
	// (committed at eff), per plant.
	var target int64 = -1 // "not-resumed-one": the one notification lost
	drop := func(l, id, eff int64) bool {
		switch plant {
		case "reconnect-window": // lost inside each window and 600 ms after it (before any anchor)
			if l == 26 {
				return false
			}
			for _, w := range wins {
				if eff >= w[0] && eff <= w[1]+600*ms {
					return true
				}
			}
		case "not-resumed-never": // listener 27 never receives anything after the first fault starts
			return l == 27 && eff >= wins[0][0]
		case "not-resumed-after-second":
			return l == 28 && eff >= wins[1][0]
		case "not-resumed-one":
			return l == 26 && id == target
		}
		return false
	}
	r.Observers = append(r.Observers, func(r *Run, finalEnd int64) {
		type notif struct{ id, eff int64 }
		var ns []notif
		for _, o := range r.Ops {
			if o.WL == "notify" && o.F == "notify" && o.Applied {
				id := o.In.(map[string]any)["id"].(int64)
				ns = append(ns, notif{id, o.Eff})
				if target < 0 && o.Eff > wins[1][1]+6*s_ {
					target = id
				}
			}
		}
		sort.Slice(ns, func(i, j int) bool { return ns[i].eff < ns[j].eff })
		for l := int64(26); l < 29; l++ {
			gw := Gateways[l-26]
			conn := l*1000 + 1
			lst := r.k5op(l, gw, "notify", "listen", map[string]any{"conn": conn}, map[string]any{"conn": conn}, r.T0-15*ms+l*us, ms)
			for t := lst.Cmp + us; t < finalEnd-10*ms; {
				d := min(1*s_, finalEnd-10*ms-t)
				payloads := []any{}
				for _, n := range ns {
					deliver := n.eff + 1*ms + (n.id%29)*ms
					if deliver >= t && deliver < t+d && !drop(l, n.id, n.eff) {
						payloads = append(payloads, n.id)
					}
				}
				o := r.k5op(l, gw, "notify", "wait", map[string]any{"conn": conn}, map[string]any{"conn": conn, "payloads": payloads}, t, d)
				t = o.Cmp // contiguous waits
			}
		}
	})
	switch plant {
	case "reconnect-window":
		r.Observe("C8", "notify-window-loss")
	case "not-resumed-never", "not-resumed-after-second":
		r.Expect("C8", "notify-not-resumed")
		r.Observe("C8", "notify-window-loss")
	case "not-resumed-one":
		r.Expect("C8", "notify-not-resumed")
	}
	return r
}

// --- W7 (ADR-0029: commit end LSNs; reconnect at the confirmed flush) ------

func w7kRun(id, label, plant string) *Run {
	r := k5Run(id, "w7k", label)
	r.Workloads = []string{"W7"}
	r.Plant = plant
	wins := [][2]int64{{r.T0 + 500*ms, r.T0 + 600*ms}, {r.T0 + 1000*ms, r.T0 + 1150*ms}}
	r.k5Fault("f0", "F2a", "pg-n1", wins[0][0], wins[0][1])
	r.k5Fault("f1", "F6", "pooler-n2", wins[1][0], wins[1][1])
	// K5.3's warm-up writes (client 14) before the workload.
	for n := int64(1); n <= 3; n++ {
		in := map[string]any{"id": 8_000_000_000 + n, "payload": fmt.Sprintf("warmup-%d", n)}
		o := r.k5op(14, "gw-z1", "ledger", "insert", in, nil, r.T0+(n-1)*10*ms, 2*ms)
		o.Exec = func(s *Store, at int64) any { return s.insert(at, o, in) }
	}
	start, end := r.T0+40*ms, r.T0+1500*ms
	nid := int64(0)
	r.W4([]int64{8, 9}, start, end, w4Fail, w4Info, &nid)
	r.W1([]int64{0}, start, end, w1Fail, w1Info)
	r.PreExec = append(r.PreExec, func(r *Run) {
		for _, o := range r.Ops {
			if o.Client < 10 && overlaps(wins, o.Inv, o.Cmp) {
				o.Fail(errNoPrimary[0], errNoPrimary[1], errNoPrimary[2])
			}
		}
	})
	var phantom *Op
	if plant == "phantom" { // an info insert that never took effect
		at := r.T0 + 300*ms
		phantom = &Op{Client: 21, Proc: r.NewProc(), FixedProc: true, GW: "gw-z1", WL: "ledger", F: "insert",
			In: map[string]any{"id": int64(8_500_000_001), "payload": "planted"}, Inv: at + 1, Eff: at + 2, Cmp: at + 3*ms + 1}
		phantom.Info(false)
		r.Ops = append(r.Ops, phantom)
	}
	r.Observers = append(r.Observers, func(r *Run, finalEnd int64) {
		type delivery struct {
			ch       Change
			lsn, xid int64
			at       int64
		}
		var ds []delivery
		for _, c := range r.S.Commits {
			for j, ch := range c.Changes {
				ds = append(ds, delivery{ch, c.LSN, c.XID, c.At + 2*ms + int64(j)*us})
			}
		}
		if phantom != nil {
			// Delivered between two real commits, before the first window.
			lsn := int64(-1)
			for _, d := range ds {
				if d.at < phantom.Inv && d.lsn > lsn {
					lsn = d.lsn
				}
			}
			ds = append(ds, delivery{Change{Table: "cp.ledger", K: 8_500_000_001, V: 8_500_000_001}, lsn + 0x80, 999_999, phantom.Inv + 2*ms})
		}
		sort.SliceStable(ds, func(i, j int) bool { return ds[i].lsn < ds[j].lsn })
		if plant == "gap" { // one ok ledger insert committed after the second window is never delivered
			for i, d := range ds {
				if d.ch.Table == "cp.ledger" && d.at > wins[1][1]+20*ms {
					if o := commitOp(r, d.lsn); o != nil && o.Type == history.TypeOK && o.Client < 10 {
						ds = append(ds[:i:i], ds[i+1:]...)
						break
					}
				}
			}
		}
		const client = 29
		conn := int64(1)
		var flushed, pending int64
		t := r.T0 - 10*ms
		op := func(f string, in, out map[string]any) *Op {
			o := r.k5op(client, "gw-z1", "cdc", f, in, out, t, us)
			t += 2 * us
			return o
		}
		connect := func() {
			op("connect", map[string]any{"conn": conn}, map[string]any{"conn": conn, "slot": "cp_slot", "start_lsn": flushed})
		}
		recv := func(d delivery) {
			row := map[string]any{"id": d.ch.V}
			if d.ch.Table == "cp.append" {
				row = map[string]any{"k": d.ch.K, "vals": ints(d.ch.Vals)}
			}
			op("recv", map[string]any{"conn": conn}, map[string]any{"conn": conn, "table": d.ch.Table, "row": row, "lsn": d.lsn, "xid": d.xid, "flushed": flushed})
		}
		ack := func() {
			flushed = pending
			op("ack", map[string]any{"conn": conn, "flush": flushed}, map[string]any{"conn": conn, "flush": flushed})
		}
		connect()
		nextAck := r.T0 + 100*ms
		w := 0
		swapped := false
		for i := 0; i < len(ds); i++ {
			d := ds[i]
			for t < d.at && nextAck <= d.at {
				t = max(t, nextAck)
				ack()
				nextAck += 100 * ms
			}
			if w < len(wins) && d.at >= wins[w][0] {
				// The stream ends at the window and the consumer reconnects at
				// its confirmed flush after it; the server re-sends what was
				// not confirmed (lsn > flushed: at-least-once, allowed).
				t = max(t, wins[w][0]+5*ms)
				brk := op("stream-end", map[string]any{"conn": conn}, nil)
				brk.Type, brk.Err = history.TypeInfo, map[string]any{"sqlstate": "", "class": "transport", "phase": "transport"}
				t = wins[w][1] + 20*ms
				nextAck = max(nextAck, t+100*ms)
				conn++
				connect()
				w++
				// ds[:i] were delivered; everything above the confirmed
				// flush is delivered again from j on.
				j := i
				for j > 0 && ds[j-1].lsn > flushed {
					j--
				}
				switch {
				case w == 1 && plant == "duplicate-boundary":
					// The server re-sends the last confirmed commit (its
					// end LSN equals the confirmed flush).
					must(j > 0 && ds[j-1].lsn == flushed, "%s: no commit at the flush", id)
					for j > 0 && ds[j-1].lsn == flushed {
						j--
					}
				case w == 1 && plant == "duplicate-below":
					// The server re-sends an older confirmed commit.
					b := -1
					for x := range ds[:i] {
						if ds[x].lsn < flushed {
							b = x
						}
					}
					must(b >= 0, "%s: nothing below the flush", id)
					for x := range ds {
						if ds[x].lsn == ds[b].lsn {
							recv(ds[x])
						}
					}
				}
				i = j - 1
				continue
			}
			t = max(t, d.at)
			if plant == "order" && !swapped && d.ch.Table == "cp.append" && i+1 < len(ds) {
				// A later change of the same key is delivered first.
				for k := i + 1; k < len(ds) && ds[k].lsn-d.lsn < 0x3000; k++ {
					if ds[k].ch.Table == "cp.append" && ds[k].ch.K == d.ch.K && ds[k].lsn > d.lsn && (w >= len(wins) || ds[k].at < wins[w][0]) {
						recv(ds[k])
						swapped = true
						break
					}
				}
			}
			recv(d)
			if i+1 == len(ds) || ds[i+1].lsn != d.lsn {
				pending = d.lsn
			}
		}
		must(plant != "order" || swapped, "%s: no two changes of one key to reorder", id)
		t = max(t, nextAck)
		ack()
		last := int64(0)
		for _, d := range ds {
			last = max(last, d.lsn)
		}
		t = max(t, finalEnd-5*ms)
		op("caught-up", map[string]any{"conn": conn, "target_lsn": last}, map[string]any{"conn": conn, "target_lsn": last, "received_lsn": last})
	})
	switch plant {
	case "gap", "phantom", "order":
		r.Expect("C7", "cdc-"+plant)
	case "duplicate-boundary", "duplicate-below":
		r.Expect("C7", "cdc-duplicate")
	}
	return r
}

// --- W8 (ADR-0030: steady inserts are W4 inserts with their role) ----------

func w8kRun(id, label, variant string) *Run {
	r := k5Run(id, "w8k", label)
	r.Workloads = []string{"W8"}
	r.Plant = variant
	phases := [][2]int64{{r.T0 + 100*ms, r.T0 + 1100*ms}, {r.T0 + 1300*ms, r.T0 + 2300*ms}}
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
	var fault [2]int64
	switch variant {
	case "fault-in-storm":
		fault = [2]int64{r.T0 + 1700*ms, r.T0 + 1720*ms}
	case "fault-between":
		fault = [2]int64{r.T0 + 1150*ms, r.T0 + 1250*ms}
	}
	if fault[0] > 0 {
		r.k5Fault("f0", "F2a", "pg-n1", fault[0], fault[1])
	}
	// server returns the executing PostgreSQL's identity at time t: a leader
	// change moves it (after the fault, or mid-storm without one).
	server := func(t int64) string {
		switch {
		case fault[0] > 0 && t > fault[1]:
			return "2026-10-13 01:41:15.120000+00"
		case variant == "failover" && t > phases[0][0]+500*ms:
			return "2026-10-13 01:41:14.002000+00"
		}
		return "2026-10-13 01:41:12.508509+00"
	}
	nid := int64(0)
	for _, c := range []int64{8, 9} {
		failures := 0
		r.Loop(c, Gateways[c%3], "ledger", r.T0, r.T0+2500*ms, 1*ms, 6*ms, func(o *Op) bool {
			nid++
			in := map[string]any{"id": nid, "payload": fmt.Sprintf("c%d-%016x", c, r.R.Uint64()), "role": "cp_steady"}
			o.F, o.In = "insert", in
			switch {
			case fault[0] > 0 && o.Cmp >= fault[0] && o.Inv <= fault[1]:
				o.Fail("", "connect", "connect")
			case variant == "starvation" && o.Inv > phases[0][0]+200*ms && o.Inv < phases[0][1] && failures < 12 && r.R.IntN(3) == 0:
				o.Fail("", "connect", "connect") // the pool refuses cp_steady
				failures++
			default:
				o.Exec = func(s *Store, at int64) any {
					m := s.insert(at, o, in)
					m["role"], m["server"] = "cp_steady", server(at)
					return m
				}
			}
			return true
		})
	}
	switch variant {
	case "starvation":
		r.Expect("C8", "fair-share-starvation")
	case "failover":
		r.Expect("C8", "overload-failover")
	}
	return r
}

// --- G8 (C9, profile buffered; ADR-0029's timing guard) --------------------

func g8kRun(id, label, plant string) *Run {
	r := k5Run(id, "g8k", label)
	r.Profile = "buffered"
	r.Plant = plant
	end := r.T0 + 1800*ms
	nid := int64(0)
	r.W4([]int64{8, 9}, r.T0, end, w4Fail, w4Info, &nid)
	r.W1([]int64{0, 1, 2, 3}, r.T0, end, w1Fail, w1Info)
	wins := [][2]int64{{r.T0 + 300*ms, r.T0 + 500*ms}, {r.T0 + 800*ms, r.T0 + 1000*ms}, {r.T0 + 1300*ms, r.T0 + 1500*ms}}
	for i, w := range wins {
		r.k5Fault(fmt.Sprintf("f%d", i), "F6", fmt.Sprintf("pooler-n%d", i+1), w[0], w[1])
	}
	r.PreExec = append(r.PreExec, func(r *Run) {
		planted, unbuffered := false, 0
		for _, o := range r.Ops {
			if o.Client >= 10 {
				continue
			}
			wi := -1
			for i, w := range wins {
				if o.Cmp >= w[0] && o.Inv <= w[1] {
					wi = i
				}
			}
			if wi < 0 {
				continue
			}
			// Buffered switchovers: autocommit statements succeed, explicit
			// transactions open across the switch get 40001.
			switch o.WL {
			case "ledger":
				o.Type, o.Err, o.Applied = history.TypeOK, nil, true
			case "append":
				if o.Type != history.TypeOK {
					o.Fail("40001", "40", "commit")
				}
			}
			switch {
			case wi > 0 && plant == "unbuffered" && o.WL == "ledger" && unbuffered < 3:
				// The second and third switchovers come within the timing
				// guard of the first: not buffered, errors pass through.
				o.Fail(errNoPrimary[0], errNoPrimary[1], errNoPrimary[2])
				unbuffered++
			case wi == 0 && !planted:
				switch {
				case plant == "autocommit-info" && o.WL == "ledger":
					o.Info(false)
				case plant == "autocommit-fail" && o.WL == "ledger":
					o.Fail("", "connect", "connect")
				case plant == "explicit-info" && o.WL == "append":
					o.Info(false)
					o.Err = map[string]any{"sqlstate": "08006", "class": "08", "phase": "commit"}
				case plant == "explicit-deadlock" && o.WL == "append":
					o.Fail("40P01", "40", "statement")
				default:
					continue
				}
				planted = true
			}
		}
		must(plant == "" || plant == "unbuffered" || planted, "%s: nothing to plant on", r.ID)
		must(plant != "unbuffered" || unbuffered > 0, "%s: no unbuffered window op", r.ID)
	})
	switch plant {
	case "unbuffered":
		r.Observe("C9", "planned-failover-unbuffered")
	case "":
	default:
		r.Expect("C9", "planned-failover-error")
	}
	return r
}
