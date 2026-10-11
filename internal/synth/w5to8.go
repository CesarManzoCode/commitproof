package synth

import (
	"fmt"
	"sort"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

// Synthetic forms of W5–W8 (CONTRACT §7), as read by C7–C9 (see
// internal/checker/c5c7.go and c8c9.go for the field definitions).

func moreEntries() []Entry {
	var es []Entry
	add := func(id string, f func() *Run) { es = append(es, Entry{id, f}) }
	for n := 1; n <= 55; n++ {
		id := fmt.Sprintf("w5-clean-%03d", n)
		add(id, func() *Run { return w5Run(id, "clean", n%3 == 0, "") })
		id6 := fmt.Sprintf("w6-clean-%03d", n)
		add(id6, func() *Run { return w6Run(id6, "clean", "") })
		id7 := fmt.Sprintf("w7-clean-%03d", n)
		add(id7, func() *Run { return w7Run(id7, "clean", n%2 == 0, "") })
		id8 := fmt.Sprintf("w8-clean-%03d", n)
		add(id8, func() *Run { return w8Run(id8, "clean", []string{"none", "fault-in-storm", "fault-between"}[n%3]) })
	}
	for n := 1; n <= 3; n++ {
		for _, p := range []struct{ kind, label string }{{"setting-lost", "session-setting-lost"}, {"prepared-26000", "prepared-statement-lost"}} {
			id := fmt.Sprintf("w5-%s-%02d", p.kind, n)
			add(id, func() *Run { return w5Run(id, p.label, true, p.kind) })
		}
		id := fmt.Sprintf("w5-prepared-row-%02d", n)
		if n == 1 {
			add(id, func() *Run { return w5Run(id, "prepared-statement-lost", true, "prepared-row") })
		}
		for _, p := range []struct{ kind, label string }{{"not-resumed", "notify-not-resumed"}, {"window-loss", "notify-window-loss"}} {
			id := fmt.Sprintf("w6-%s-%02d", p.kind, n)
			add(id, func() *Run { return w6Run(id, p.label, p.kind) })
		}
		for _, p := range []struct{ kind, label string }{{"gap", "cdc-gap"}, {"phantom", "cdc-phantom"}, {"order", "cdc-order"}} {
			id := fmt.Sprintf("w7-%s-%02d", p.kind, n)
			add(id, func() *Run { return w7Run(id, p.label, false, p.kind) })
		}
		kind := "duplicate-boundary"
		if n == 3 {
			kind = "duplicate-below"
		}
		id7 := fmt.Sprintf("w7-%s-%02d", kind, n)
		add(id7, func() *Run { return w7Run(id7, "cdc-duplicate", false, kind) })
		for _, p := range []struct{ kind, label string }{{"starvation", "fair-share-starvation"}, {"failover", "overload-failover"}} {
			id := fmt.Sprintf("w8-%s-%02d", p.kind, n)
			add(id, func() *Run { return w8Run(id, p.label, p.kind) })
		}
	}
	for n := 1; n <= 10; n++ {
		id := fmt.Sprintf("g8-clean-%03d", n)
		add(id, func() *Run { return g8Run(id, "clean", "") })
	}
	for n, kind := range []string{"autocommit-info", "autocommit-fail", "explicit-info", "explicit-deadlock"} {
		id := fmt.Sprintf("g8-%s-%02d", kind, n+1)
		add(id, func() *Run { return g8Run(id, "planned-failover-error", kind) })
	}
	return es
}

// --- W5 session ------------------------------------------------------------

var gucValues = map[string][]string{
	"application_name": {"cp-a", "cp-b", "cp-c", "cp-d"},
	"search_path":      {"cp, public", "public, cp", "public"},
	"work_mem":         {"4MB", "8MB", "16MB"},
	"DateStyle":        {"ISO, MDY", "ISO, DMY"},
}
var gucNames = []string{"DateStyle", "application_name", "search_path", "work_mem"}

func w5Run(id, label string, failover bool, plant string) *Run {
	r := NewRun(id, "w5", label)
	r.Workloads = []string{"W5"}
	r.Plant = plant
	end := r.T0 + 1000*ms
	win := [2]int64{r.T0 + 400*ms, r.T0 + 600*ms}
	if failover || plant != "" {
		r.Nemesis(win[0], "f1", "F2a", "inject", "pg-n1")
		r.Nemesis(win[0]+5*ms, "f1", "F2a", "confirmed", "pg-n1")
		r.Nemesis(win[1]-50*ms, "f1", "F2a", "heal", "pg-n1")
		r.Nemesis(win[1], "f1", "F2a", "healed", "pg-n1")
	}
	for c := int64(0); c < 3; c++ {
		conn := c * 1000
		state := map[string]string{}
		prepared := map[string]int64{}
		seq := int64(0)
		r.Loop(c, Gateways[c], "session", r.T0, end, 1*ms, 5*ms, func(o *Op) bool {
			seq++
			x := r.R.IntN(100)
			names := make([]string, 0, len(prepared))
			for n := range prepared {
				names = append(names, n)
			}
			sort.Strings(names)
			meta := map[string]any{}
			r.Meta[o] = meta
			inWin := failover && o.Cmp >= win[0] && o.Inv <= win[1]
			switch {
			case x < 30:
				g := gucNames[r.R.IntN(len(gucNames))]
				v := gucValues[g][r.R.IntN(len(gucValues[g]))]
				o.F, o.In = "set", map[string]any{"conn": conn, "guc": g, "value": v}
				if inWin {
					o.Fail("MTF01", "MT", "statement") // no writable primary: not applied
				} else {
					state[g] = v
				}
			case x < 45:
				name := fmt.Sprintf("cp_s%d", seq)
				o.F, o.In = "prepare", map[string]any{"conn": conn, "name": name, "expect": []any{seq}}
				if inWin {
					o.Fail("MTF01", "MT", "statement")
				} else {
					prepared[name] = seq
				}
			case x < 70 && len(names) > 0:
				name := names[r.R.IntN(len(names))]
				o.F, o.In = "execute", map[string]any{"conn": conn, "name": name}
				o.Out = map[string]any{"conn": conn, "name": name, "row": []any{prepared[name]}}
				meta["prepared"] = true
				if inWin {
					o.Fail("MTF01", "MT", "statement")
				}
			default:
				g := gucNames[r.R.IntN(len(gucNames))]
				v, known := state[g]
				if !known {
					v = "server-default"
				}
				o.F, o.In = "show", map[string]any{"conn": conn, "guc": g}
				o.Out = map[string]any{"conn": conn, "guc": g, "value": v}
				meta["known"], meta["inwin"] = known, inWin
				if inWin {
					o.Fail("MTF01", "MT", "statement")
				}
			}
			// A transport failure ends the connection: its state is gone.
			if o.Type == history.TypeOK && r.R.IntN(60) == 0 {
				o.Type, o.Err, o.Out = history.TypeInfo, map[string]any{"sqlstate": "", "class": "transport", "phase": "transport"}, nil
				conn++
				state, prepared = map[string]string{}, map[string]int64{}
			}
			return true
		})
	}
	pick := func(f string, key string, after int64) *Op {
		var best *Op
		for _, o := range r.Ops {
			m := r.Meta[o]
			if o.F == f && o.Type == history.TypeOK && m[key] == true && o.Inv > after && (best == nil || o.Inv < best.Inv) {
				best = o
			}
		}
		must(best != nil, "%s: no %s to plant on", id, f)
		return best
	}
	switch plant {
	case "setting-lost": // after the failover window an ok SHOW returns the
		// server default instead of the value SET earlier on the connection
		r.Post = append(r.Post, func(r *Run) {
			o := pick("show", "known", win[1])
			m := o.Out.(map[string]any)
			o.Out = map[string]any{"conn": m["conn"], "guc": m["guc"], "value": "server-default"}
		})
		r.Expect("C8", "session-setting-lost")
	case "prepared-26000": // EXECUTE after the window fails: statement gone
		r.Post = append(r.Post, func(r *Run) {
			o := pick("execute", "prepared", win[1])
			o.Type, o.Out = history.TypeInfo, nil
			o.Err = map[string]any{"sqlstate": "26000", "class": "26", "phase": "statement"}
		})
		r.Expect("C8", "prepared-statement-lost")
	case "prepared-row": // EXECUTE returns another statement's row
		r.Post = append(r.Post, func(r *Run) {
			o := pick("execute", "prepared", win[1])
			m := o.Out.(map[string]any)
			row := m["row"].([]any)
			o.Out = map[string]any{"conn": m["conn"], "name": m["name"], "row": []any{row[0].(int64) + 100000}}
		})
		r.Expect("C8", "prepared-statement-lost")
	}
	return r
}

// --- W6 notify ---------------------------------------------------------

type session struct {
	client, conn int64
	listenCmp    int64
	end          int64 // Never if alive at the end
}

func w6Run(id, label, plant string) *Run {
	r := NewRun(id, "w6", label)
	r.Workloads = []string{"W6"}
	r.Plant = plant
	end := r.T0 + 1000*ms
	nid := int64(0)
	r.Loop(0, "gw-z1", "notify", r.T0, end, 1*ms, 6*ms, func(o *Op) bool {
		nid++
		o.F, o.In = "notify", map[string]any{"id": nid, "chan": "cp_chan"}
		in := o.In.(map[string]any)
		o.Exec = func(s *Store, at int64) any { return s.insert(at, o, in) }
		r.Outcome(o, w4Fail, w4Info, ErrConnect)
		return true
	})
	brk := int64(-1)
	if plant == "window-loss" {
		brk = r.T0 + 350*ms
	}
	drop := map[int64]bool{}
	r.Observers = append(r.Observers, func(r *Run, finalEnd int64) {
		type notif struct {
			id, at int64
			op     *Op
		}
		var ns []notif
		for _, o := range r.Ops {
			if o.WL == "notify" && o.F == "notify" && o.Applied {
				ns = append(ns, notif{o.In.(map[string]any)["id"].(int64), o.Eff, o})
			}
		}
		sort.Slice(ns, func(i, j int) bool { return ns[i].at < ns[j].at })
		if plant == "not-resumed" { // listener 2 never receives one ok NOTIFY
			for _, n := range ns {
				if n.op.Type == history.TypeOK && n.at > r.T0+400*ms {
					drop[n.id] = true
					break
				}
			}
		}
		for l := int64(1); l <= 3; l++ {
			gw := Gateways[l-1]
			conn := l * 1000
			t := r.T0 - 15*ms + l*us
			listen := func(at int64) int64 {
				o := &Op{Client: l, GW: gw, WL: "notify", F: "listen", In: map[string]any{"conn": conn}, Out: map[string]any{"conn": conn},
					Type: history.TypeOK, Applied: true, Inv: at, Eff: at + 500*us, Cmp: at + ms}
				r.Ops = append(r.Ops, o)
				return o.Cmp
			}
			start := listen(t)
			t = start + us
			lastNotify := r.T0
			if len(ns) > 0 {
				lastNotify = ns[len(ns)-1].at
			}
			for t < finalEnd-10*ms {
				d := 400 * ms
				if t > lastNotify+500*ms {
					d = finalEnd - 10*ms - t
				}
				o := &Op{Client: l, GW: gw, WL: "notify", F: "wait", In: map[string]any{"conn": conn}, Type: history.TypeOK, Applied: true, Inv: t, Eff: t + us, Cmp: t + d}
				if l == 3 && brk > 0 && o.Inv < brk && o.Cmp > brk { // the connection breaks
					o.Cmp = brk
					o.Type, o.Err = history.TypeInfo, map[string]any{"sqlstate": "", "class": "transport", "phase": "transport"}
					r.Ops = append(r.Ops, o)
					conn++
					start = listen(brk + 300*ms)
					t = start + us
					brk = -1
					continue
				}
				payloads := []any{}
				for _, n := range ns {
					deliver := n.at + 1*ms + (n.id%29)*ms
					if n.at > start && deliver >= o.Inv && deliver < o.Cmp && !(l == 2 && drop[n.id]) {
						payloads = append(payloads, n.id)
					}
				}
				o.Out = map[string]any{"conn": conn, "payloads": payloads}
				r.Ops = append(r.Ops, o)
				t = o.Cmp // waits are contiguous: no delivery falls between two

			}
		}
	})
	switch plant {
	case "not-resumed":
		r.Expect("C8", "notify-not-resumed")
	case "window-loss":
		r.Observe("C8", "notify-window-loss")
	}
	return r
}

// --- W7 cdc --------------------------------------------------------------

func w7Run(id, label string, reconnect bool, plant string) *Run {
	r := NewRun(id, "w7", label)
	r.Workloads = []string{"W7"}
	r.Plant = plant
	end := r.T0 + 1000*ms
	nid := int64(0)
	r.W4([]int64{8, 9}, r.T0, end, w4Fail, w4Info, &nid)
	r.W1([]int64{0}, r.T0, end, w1Fail, w1Info)
	var phantom *Op
	if plant == "phantom" { // an info insert that never committed
		at := r.T0 + 500*ms
		phantom = &Op{Client: 21, Proc: r.NewProc(), FixedProc: true, GW: "gw-z1", WL: "ledger", F: "insert",
			In: map[string]any{"id": int64(8_000_000_002), "payload": "planted"}, Inv: at + 1, Eff: at + 2, Cmp: at + 3*ms + 1}
		phantom.Info(false)
		r.Ops = append(r.Ops, phantom)
	}
	r.Observers = append(r.Observers, func(r *Run, finalEnd int64) {
		type delivery struct {
			ch  Change
			lsn int64
			xid int64
			at  int64
		}
		var ds []delivery
		for _, c := range r.S.Commits {
			for j, ch := range c.Changes {
				ds = append(ds, delivery{ch, c.LSN, c.XID, c.At + 2*ms + int64(j)*us})
			}
		}
		if phantom != nil { // the consumer receives the never-committed insert
			ds = append(ds, delivery{Change{Table: "cp.ledger", K: 8_000_000_002, V: 8_000_000_002}, r.S.lsn + 0x80, 999_999, phantom.Inv + 2*ms})
		}
		sort.SliceStable(ds, func(i, j int) bool { return ds[i].lsn < ds[j].lsn })
		switch plant {
		case "gap": // one ok ledger insert's change is never delivered
			for i, d := range ds {
				if d.ch.Table == "cp.ledger" && d.lsn > ds[len(ds)/2].lsn && d.ch.V < 9_000_000_000 {
					if o := commitOp(r, d.lsn); o != nil && o.Type == history.TypeOK {
						ds = append(ds[:i:i], ds[i+1:]...)
						break
					}
				}
			}
		case "order": // a later change of one append key is delivered just
			// before an earlier change of the same key
			swapped := false
			for i := 0; i < len(ds) && !swapped; i++ {
				for j := i + 1; j < len(ds) && !swapped; j++ {
					a, b := ds[i], ds[j]
					if a.ch.Table == "cp.append" && b.ch.Table == "cp.append" && a.ch.K == b.ch.K && a.lsn < b.lsn {
						moved := append(append(append([]delivery{}, ds[:i]...), b), ds[i:j]...)
						ds = append(moved, ds[j+1:]...)
						swapped = true
					}
				}
			}
			must(swapped, "%s: no two changes of one key to reorder", r.ID)
		}
		const client = 5
		conn := int64(1)
		t := r.T0 - 10*ms
		var flushed, lastFull int64
		op := func(f string, in, out map[string]any, at int64) *Op {
			o := &Op{Client: client, GW: "gw-z1", WL: "cdc", F: f, In: in, Out: out, Type: history.TypeOK, Applied: true, Inv: at, Eff: at + 1, Cmp: at + us}
			r.Ops = append(r.Ops, o)
			return o
		}
		connect := func(at int64) {
			op("connect", map[string]any{"conn": conn}, map[string]any{"conn": conn, "slot": "cp_slot", "start_lsn": flushed}, at)
		}
		connect(t)
		t += 2 * us
		recvs := 0
		resend := -1
		dupAt := map[string]int{"duplicate-boundary": len(ds) / 2, "duplicate-below": len(ds) / 2}[plant]
		for i := 0; i < len(ds); i++ {
			d := ds[i]
			t = max(t+2*us, d.at)
			row := map[string]any{"id": d.ch.V}
			if d.ch.Table == "cp.append" {
				row = map[string]any{"k": d.ch.K, "vals": ints(d.ch.Vals)}
			}
			op("recv", map[string]any{"conn": conn}, map[string]any{"conn": conn, "table": d.ch.Table, "row": row,
				"lsn": d.lsn, "xid": d.xid, "flushed": flushed}, t)
			recvs++
			lastFull = d.lsn
			t += 2 * us
			// Ack every five changes, at a commit boundary.
			if recvs%5 == 0 && (i+1 == len(ds) || ds[i+1].lsn != d.lsn) {
				flushed = lastFull
				op("ack", map[string]any{"conn": conn, "flush": flushed}, map[string]any{"conn": conn, "flush": flushed}, t)
				t += 2 * us
				if plant != "" && plant[:min(len(plant), 9)] == "duplicate" && i >= dupAt && resend < 0 {
					// The connection breaks; the new primary's slot replays from
					// an LSN at (boundary) or below the confirmed flush.
					resend = i
					j := i
					for j > 0 && ds[j-1].lsn == ds[i].lsn {
						j--
					}
					if plant == "duplicate-below" {
						j--
						for j > 0 && ds[j-1].lsn == ds[j].lsn {
							j--
						}
						k := j
						for k+1 < len(ds) && ds[k+1].lsn == ds[j].lsn {
							k++
						}
						// replay only the commit below the flush, then continue
						brk := op("stream-end", map[string]any{"conn": conn}, nil, t)
						brk.Type, brk.Err = history.TypeInfo, map[string]any{"sqlstate": "", "class": "transport", "phase": "transport"}
						conn++
						t += 300 * us
						connect(t)
						t += 2 * us
						for x := j; x <= k; x++ {
							rr := map[string]any{"id": ds[x].ch.V}
							if ds[x].ch.Table == "cp.append" {
								rr = map[string]any{"k": ds[x].ch.K, "vals": ints(ds[x].ch.Vals)}
							}
							op("recv", map[string]any{"conn": conn}, map[string]any{"conn": conn, "table": ds[x].ch.Table, "row": rr,
								"lsn": ds[x].lsn, "xid": ds[x].xid, "flushed": flushed}, t)
							t += 2 * us
						}
						continue
					}
					brk := op("stream-end", map[string]any{"conn": conn}, nil, t)
					brk.Type, brk.Err = history.TypeInfo, map[string]any{"sqlstate": "", "class": "transport", "phase": "transport"}
					conn++
					t += 300 * us
					connect(t)
					t += 2 * us
					i = j - 1 // re-deliver from the commit at the confirmed flush
					continue
				}
			}
			if reconnect && i == len(ds)/2 && plant == "" {
				// A clean reconnect: changes after the last ack are delivered
				// again (lsn > flushed: allowed, at-least-once).
				brk := op("stream-end", map[string]any{"conn": conn}, nil, t)
				brk.Type, brk.Err = history.TypeInfo, map[string]any{"sqlstate": "", "class": "transport", "phase": "transport"}
				conn++
				t += 300 * us
				connect(t)
				t += 2 * us
				j := i
				for j >= 0 && ds[j].lsn > flushed {
					j--
				}
				i = j
				reconnect = false
			}
		}
		last := int64(0)
		if len(ds) > 0 {
			last = ds[len(ds)-1].lsn
		}
		op("caught-up", map[string]any{"conn": conn, "target_lsn": last}, map[string]any{"conn": conn, "target_lsn": last, "received_lsn": last}, max(t+us, finalEnd-5*ms))
	})
	switch plant {
	case "gap":
		r.Expect("C7", "cdc-gap")
	case "phantom":
		r.Expect("C7", "cdc-phantom")
	case "order":
		r.Expect("C7", "cdc-order")
	case "duplicate-boundary", "duplicate-below":
		r.Expect("C7", "cdc-duplicate")
	}
	return r
}

func commitOp(r *Run, lsn int64) *Op {
	for _, c := range r.S.Commits {
		if c.LSN == lsn {
			return c.Op
		}
	}
	return nil
}

// --- W8 overload ---------------------------------------------------------

func w8Run(id, label, variant string) *Run {
	r := NewRun(id, "w8", label)
	r.Workloads = []string{"W8"}
	r.Plant = variant
	phases := [][2]int64{{r.T0 + 100*ms, r.T0 + 1100*ms}, {r.T0 + 1300*ms, r.T0 + 2300*ms}}
	for c := int64(0); c < 4; c++ {
		for p, w := range phases {
			r.Loop(c, Gateways[c%3], "overload", w[0], w[1]-300*ms, 150*ms, 300*ms, func(o *Op) bool {
				o.F, o.In = "storm", map[string]any{"role": "cp_storm", "phase": int64(p + 1)}
				o.Out = o.In
				return true
			})
		}
	}
	server := "10.77.0.11"
	var fault [2]int64
	switch variant {
	case "fault-in-storm":
		fault = [2]int64{r.T0 + 1700*ms, r.T0 + 1720*ms}
	case "fault-between":
		fault = [2]int64{r.T0 + 1150*ms, r.T0 + 1250*ms}
	}
	if fault[0] > 0 {
		r.Nemesis(fault[0], "f1", "F2a", "inject", "pg-n1")
		r.Nemesis(fault[0]+5*ms, "f1", "F2a", "confirmed", "pg-n1")
		r.Nemesis(fault[1]-5*ms, "f1", "F2a", "heal", "pg-n1")
		r.Nemesis(fault[1], "f1", "F2a", "healed", "pg-n1")
	}
	sid := int64(0)
	failures := 0
	r.Loop(4, "gw-z1", "overload", r.T0, r.T0+2500*ms, 1*ms, 6*ms, func(o *Op) bool {
		sid++
		o.F, o.In = "steady", map[string]any{"role": "cp_steady", "id": sid}
		inFault := fault[0] > 0 && o.Cmp >= fault[0] && o.Inv <= fault[1]
		switch {
		case inFault:
			o.Fail("", "connect", "connect")
			server = "10.77.0.12"
		case variant == "starvation" && o.Inv > phases[0][0]+200*ms && failures < 5:
			o.Fail("", "connect", "connect") // pool exhausted: 5 of ~20 fail
			failures++
		case variant == "failover" && o.Inv > phases[0][0]+500*ms:
			server = "10.77.0.13" // leader changed mid-storm, no fault
		}
		o.Out = map[string]any{"role": "cp_steady", "id": sid, "server": server}
		return true
	})
	switch variant {
	case "starvation":
		r.Expect("C8", "fair-share-starvation")
	case "failover":
		r.Expect("C8", "overload-failover")
	}
	return r
}

// --- G8 (C9) ---------------------------------------------------------------

func g8Run(id, label, plant string) *Run {
	r := NewRun(id, "g8", label)
	r.Profile = "buffered"
	r.Plant = plant
	end := r.T0 + 1000*ms
	nid := int64(0)
	r.W4([]int64{8, 9}, r.T0, end, w4Fail, w4Info, &nid)
	r.W1([]int64{0, 1}, r.T0, end, w1Fail, w1Info)
	win := [2]int64{r.T0 + 400*ms, r.T0 + 700*ms}
	r.Nemesis(win[0], "f1", "F6", "inject", "pooler-n1")
	r.Nemesis(win[0]+50*ms, "f1", "F6", "confirmed", "pooler-n1")
	r.Nemesis(win[1]-100*ms, "f1", "F6", "heal", "pooler-n1")
	r.Nemesis(win[1], "f1", "F6", "healed", "pooler-n1")
	r.PreExec = append(r.PreExec, func(r *Run) {
		planted := false
		for _, o := range r.Ops {
			if o.Cmp < win[0] || o.Inv > win[1] || o.Client >= 10 {
				continue
			}
			// With buffering, autocommit statements succeed and explicit
			// transactions open across the switch get 40001.
			switch {
			case o.WL == "ledger":
				o.Type, o.Err, o.Applied = history.TypeOK, nil, true
				if !planted && (plant == "autocommit-info" || plant == "autocommit-fail") {
					if plant == "autocommit-info" {
						o.Info(false)
					} else {
						o.Fail("", "connect", "connect")
					}
					planted = true
				}
			case o.WL == "append":
				if o.Type != history.TypeOK {
					o.Fail("40001", "40", "commit")
				}
				if !planted && (plant == "explicit-info" || plant == "explicit-deadlock") {
					if plant == "explicit-info" {
						o.Info(false)
						o.Err = map[string]any{"sqlstate": "08006", "class": "08", "phase": "commit"}
					} else {
						o.Fail("40P01", "40", "statement")
					}
					planted = true
				}
			}
		}
		must(plant == "" || planted, "%s: nothing to plant on", r.ID)
	})
	if plant != "" {
		r.Expect("C9", "planned-failover-error")
	}
	return r
}

// --- K2.3 ------------------------------------------------------------------

// PathologicalWrites is the number of concurrent open-ended writes of the
// K2.3 history: Porcupine must consider every subset of them before the
// impossible read, which takes far longer than P11.
const PathologicalWrites = 30

// Pathological is the K2.3 history: PathologicalWrites info writes to
// register 0, all concurrent and never completed, then a read of a value no
// write wrote. Porcupine cannot decide it within P11: C4 and C6 are unknown
// and the verdict UNKNOWN.
func Pathological() *Run {
	r := NewRun("porcupine-timeout", "pathological", "unknown")
	r.Workloads = []string{"W2"}
	r.Plant = fmt.Sprintf("%d concurrent info writes to register 0, then a read of a value never written", PathologicalWrites)
	at := r.T0 + 10*ms
	for i := int64(0); i < PathologicalWrites; i++ {
		o := &Op{Client: 30 + i, Proc: 200 + i, FixedProc: true, GW: Gateways[i%3], WL: "register", F: "write",
			In: map[string]any{"k": int64(0), "v": 5_000_000 + i}, Inv: at + i*us, Eff: at + i*us + 1, Cmp: at + 100*ms + i*us}
		o.Info(false)
		r.Ops = append(r.Ops, o)
	}
	r.Ops = append(r.Ops, &Op{Client: 4, Proc: 4, FixedProc: true, GW: "gw-z1", WL: "register", F: "read", In: map[string]any{"k": int64(0)},
		Out: map[string]any{"k": int64(0), "v": int64(5_999_999)}, Type: history.TypeOK, Applied: true, Inv: at + 50*ms, Eff: at + 50*ms + us, Cmp: at + 60*ms})
	r.S.Regs[0] = 5_000_000 + PathologicalWrites - 1
	r.Expected["C4"].Valid = "unknown"
	r.Expected["C6"].Valid = "unknown"
	r.Verdict = "UNKNOWN"
	return r
}
