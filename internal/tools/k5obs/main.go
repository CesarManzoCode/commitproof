// Command k5obs summarises what Multigres did in sealed K5 runs, for
// evidence/K5/NOTES.md ("system did"): LISTEN/NOTIFY loss windows per
// listener and fault window (W6, S19), W5 reconnects, the CDC slot's
// failover readiness before the first fault and after each fault and the
// consumer's reconnects and re-deliveries relative to its confirmed flush
// (W7, S12), the overload behaviour under the W8 storm (connection
// acquisition, errors, cp_steady availability and latency, server
// identities), and the C8/C9 statistics cpcheck recorded. It reads
// observations, so it is diagnosis only (ADR-0016): no verdict depends on
// it.
//
//	go run ./internal/tools/k5obs <run-dir>... > summary.json
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/history"
)

func num(v any) int64 {
	switch x := v.(type) {
	case json.Number:
		n, _ := x.Int64()
		return n
	case float64:
		return int64(x)
	case int64:
		return x
	}
	return 0
}

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }
func obj(v any) map[string]any              { m, _ := v.(map[string]any); return m }
func ms(ns int64) int64                     { return ns / 1_000_000 }

type op struct {
	i, inv, cmp int64
	typ         string
	client      int64
	gw, wl, f   string
	in, out     map[string]any
	err         map[string]any
}

type window struct {
	id, kind   string
	start, end int64
}

func readJSON(p string) (map[string]any, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var m map[string]any
	return m, d.Decode(&m)
}

func pct(xs []int64, q int) int64 {
	if len(xs) == 0 {
		return -1
	}
	s := append([]int64{}, xs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[(len(s)-1)*q/100]
}

func summarize(dir string) (map[string]any, error) {
	rep := history.VerifySeal(dir)
	if !rep.Valid {
		return nil, fmt.Errorf("%s: seal does not verify: %v", dir, rep.Problems)
	}
	man, err := readJSON(filepath.Join(dir, history.FileManifest))
	if err != nil {
		return nil, err
	}
	out := map[string]any{"run": filepath.Base(dir), "campaign": str(man, "campaign"), "seed": man["seed"], "mutant": str(man, "mutant"),
		"profile": str(man, "profile")}
	if cj, err := readJSON(strings.TrimSuffix(dir, "/") + ".checks.json"); err == nil {
		out["verdict"] = str(cj, "verdict")
		stats := map[string]any{}
		var classes, observations []string
		cs, _ := cj["checkers"].([]any)
		for _, x := range cs {
			c := obj(x)
			id := str(c, "id")
			if id == "C7" || id == "C8" || id == "C9" {
				st := obj(c["stats"])
				delete(st, "latency_us")
				delete(st, "observations")
				stats[id] = st
			}
			for _, a := range c["anomalies"].([]any) {
				classes = append(classes, id+":"+str(obj(a), "class"))
			}
			oc := obj(obj(c["stats"])["observation_counts"])
			for _, cl := range sortedKeys(oc) {
				observations = append(observations, fmt.Sprintf("%s:%s %d", id, cl, num(oc[cl])))
			}
		}
		out["checker_stats"], out["anomalies"], out["observation_classes"] = stats, uniq(classes), observations
	}
	// Operations and fault windows from the history.
	invs := map[int64]history.Event{}
	var ops []*op
	wins := map[string]*window{}
	var markers []map[string]any
	for _, ev := range rep.History {
		switch ev.Type {
		case history.TypeInvoke:
			invs[ev.I] = ev
		case history.TypeOK, history.TypeFail, history.TypeInfo:
			if ev.Op == nil {
				continue
			}
			iv := invs[*ev.Op]
			o := &op{i: *ev.Op, inv: iv.T, cmp: ev.T, typ: ev.Type, gw: ev.GW, wl: ev.WL, f: ev.F, in: obj(iv.Value), out: obj(ev.Value), err: ev.Err}
			if ev.Client != nil {
				o.client = *ev.Client
			}
			ops = append(ops, o)
		case history.TypeNemesis:
			id := str(ev.Fault, "id")
			w := wins[id]
			if w == nil {
				w = &window{id: id, kind: str(ev.Fault, "kind"), start: ev.T, end: ev.T}
				wins[id] = w
			}
			w.end = max(w.end, ev.T)
		case history.TypeMarker:
			markers = append(markers, obj(ev.Value))
		}
	}
	var ws []window
	for _, w := range wins {
		ws = append(ws, *w)
	}
	sort.Slice(ws, func(i, j int) bool { return ws[i].start < ws[j].start })
	var wl []any
	for _, w := range ws {
		wl = append(wl, map[string]any{"id": w.id, "kind": w.kind, "start_ms": ms(w.start), "end_ms": ms(w.end)})
	}
	out["fault_windows"] = wl
	for _, m := range markers {
		switch str(m, "name") {
		case "cdc-slot-ready", "buffer-guard-wait", "cdc-caught-up", "workload-start", "workload-end":
			out["marker_"+str(m, "name")] = m
		}
	}
	counts := map[string]int64{}
	for _, o := range ops {
		if o.f != "final-read" {
			counts[o.wl+"/"+o.f+"/"+o.typ]++
		}
	}
	out["op_counts"] = counts
	if n := notify(ops, ws); n != nil {
		out["notify"] = n
	}
	if s := session(ops); s != nil {
		out["session"] = s
	}
	if c := cdc(ops); c != nil {
		out["cdc"] = c
	}
	if o := overload(ops, ws); o != nil {
		out["overload"] = o
	}
	if s, err := slots(dir, ws); err == nil && s != nil {
		out["slots"] = s
	}
	return out, nil
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// notify: per listener, its sessions (LISTEN to last op) and the gaps
// between them (reconnect windows), and per fault window the ok NOTIFYs a
// listener missed, with the span of their commits (the measured loss
// window, S19).
func notify(ops []*op, ws []window) map[string]any {
	type sess struct {
		start, end int64
		recv       map[int64]bool
	}
	ls := map[int64][]*sess{}
	cur := map[int64]*sess{}
	var notifies []*op
	for _, o := range ops {
		if o.wl != "notify" {
			continue
		}
		switch o.f {
		case "notify":
			if o.typ == history.TypeOK {
				notifies = append(notifies, o)
			}
		case "listen":
			if o.typ == history.TypeOK {
				s := &sess{start: o.cmp, end: o.cmp, recv: map[int64]bool{}}
				ls[o.client] = append(ls[o.client], s)
				cur[o.client] = s
			}
		case "wait":
			s := cur[o.client]
			if s == nil {
				continue
			}
			s.end = max(s.end, o.cmp)
			if o.typ != history.TypeOK {
				cur[o.client] = nil
				continue
			}
			l, _ := o.out["payloads"].([]any)
			for _, x := range l {
				s.recv[num(x)] = true
			}
		}
	}
	if len(notifies) == 0 && len(ls) == 0 {
		return nil
	}
	var clients []int64
	for c := range ls {
		clients = append(clients, c)
	}
	sort.Slice(clients, func(i, j int) bool { return clients[i] < clients[j] })
	per := map[string]any{}
	for _, c := range clients {
		got := map[int64]bool{}
		var gaps []any
		for i, s := range ls[c] {
			for id := range s.recv {
				got[id] = true
			}
			if i > 0 {
				gaps = append(gaps, map[string]any{"from_ms": ms(ls[c][i-1].end), "to_ms": ms(s.start), "ms": ms(s.start - ls[c][i-1].end)})
			}
		}
		var missed []*op
		for _, n := range notifies {
			if !got[num(n.in["id"])] {
				missed = append(missed, n)
			}
		}
		// Group the misses by the latest fault window that started before
		// the NOTIFY's completion.
		groups := map[string][]*op{}
		for _, n := range missed {
			key := "no-fault"
			for _, w := range ws {
				if w.start <= n.cmp {
					key = w.id + "/" + w.kind
				}
			}
			groups[key] = append(groups[key], n)
		}
		var gl []any
		for k, g := range groups {
			first, last := g[0].inv, g[0].cmp
			for _, n := range g {
				first, last = min(first, n.inv), max(last, n.cmp)
			}
			gl = append(gl, map[string]any{"after": k, "missed": int64(len(g)), "first_invoke_ms": ms(first), "last_commit_ms": ms(last), "span_ms": ms(last - first)})
		}
		sort.Slice(gl, func(i, j int) bool { return num(obj(gl[i])["first_invoke_ms"]) < num(obj(gl[j])["first_invoke_ms"]) })
		per[fmt.Sprint(c)] = map[string]any{"sessions": int64(len(ls[c])), "reconnect_gaps": gaps, "received": int64(len(got)), "missed": int64(len(missed)), "missed_by_fault": gl}
	}
	return map[string]any{"notifies_ok": int64(len(notifies)), "listeners": per}
}

// session: W5 connections per client and failed or unknown ops by SQLSTATE.
func session(ops []*op) map[string]any {
	conns := map[int64]map[int64]bool{}
	errs := map[string]int64{}
	n := 0
	for _, o := range ops {
		if o.wl != "session" {
			continue
		}
		n++
		if conns[o.client] == nil {
			conns[o.client] = map[int64]bool{}
		}
		conns[o.client][num(o.in["conn"])] = true
		if o.typ != history.TypeOK {
			errs[o.f+"/"+o.typ+"/"+str(o.err, "sqlstate")+"/"+str(o.err, "phase")]++
		}
	}
	if n == 0 {
		return nil
	}
	pc := map[string]any{}
	for c, m := range conns {
		pc[fmt.Sprint(c)] = int64(len(m))
	}
	return map[string]any{"ops": int64(n), "connections_per_client": pc, "non_ok": errs}
}

// cdc: the consumer's connections, stream ends, receipts, and re-deliveries
// (a change received again) split at the confirmed flush recorded with the
// re-delivery: above it is at-least-once (allowed), at or below it is §10.7's
// cdc-duplicate.
func cdc(ops []*op) map[string]any {
	n := 0
	connects := map[string]int64{}
	var ends []any
	seen := map[string]bool{}
	var recvs, above, atOrBelow, acks int64
	var caught any
	for _, o := range ops {
		if o.wl != "cdc" {
			continue
		}
		n++
		switch o.f {
		case "connect":
			connects[o.typ]++
		case "stream-end":
			ends = append(ends, map[string]any{"t_ms": ms(o.cmp), "err": o.err})
		case "ack":
			acks++
		case "caught-up":
			caught = map[string]any{"type": o.typ, "out": o.out}
		case "recv":
			if o.typ != history.TypeOK {
				continue
			}
			recvs++
			row := obj(o.out["row"])
			key := str(o.out, "table") + "/" + fmt.Sprint(row["id"]) + fmt.Sprint(row["k"]) + "/" + fmt.Sprint(row["vals"])
			if seen[key] {
				if num(o.out["lsn"]) > num(o.out["flushed"]) {
					above++
				} else {
					atOrBelow++
				}
			}
			seen[key] = true
		}
	}
	if n == 0 {
		return nil
	}
	return map[string]any{"connects": connects, "stream_ends": ends, "changes_received": recvs, "acks": acks,
		"redeliveries_above_flush": above, "redeliveries_at_or_below_flush": atOrBelow, "caught_up": caught}
}

// overload: storm statements and connection acquisition by outcome and
// SQLSTATE; cp_steady availability and latency inside and outside storm
// windows; the server identities cp_steady saw, in order of first sight.
func overload(ops []*op, ws []window) map[string]any {
	type phase struct{ start, end int64 }
	phases := map[int64]*phase{}
	storm := map[string]int64{}
	for _, o := range ops {
		if o.wl != "overload" || o.f != "storm" {
			continue
		}
		p := num(o.in["phase"])
		ph := phases[p]
		if ph == nil {
			ph = &phase{o.inv, o.cmp}
			phases[p] = ph
		}
		ph.start, ph.end = min(ph.start, o.inv), max(ph.end, o.cmp)
		storm[o.typ+"/"+str(o.err, "phase")+"/"+str(o.err, "sqlstate")]++
	}
	if len(phases) == 0 {
		return nil
	}
	in := func(t int64) bool {
		for _, ph := range phases {
			if t >= ph.start && t <= ph.end {
				return true
			}
		}
		return false
	}
	type acc struct {
		ok, all int64
		lat     []int64
		errs    map[string]int64
	}
	a := map[string]*acc{"storm": {errs: map[string]int64{}}, "outside": {errs: map[string]int64{}}}
	var servers []any
	sseen := map[string]bool{}
	var changes []any
	last := ""
	for _, o := range ops {
		if o.wl != "ledger" || str(o.in, "role") != "cp_steady" {
			continue
		}
		k := "outside"
		if in(o.inv) {
			k = "storm"
		}
		x := a[k]
		x.all++
		if o.typ == history.TypeOK {
			x.ok++
			x.lat = append(x.lat, (o.cmp-o.inv)/1000)
			s := str(o.out, "server")
			if !sseen[s] {
				sseen[s] = true
				servers = append(servers, s)
			}
			if last != "" && s != last {
				faulted := false
				for _, w := range ws {
					if w.start <= o.cmp {
						faulted = true
					}
				}
				changes = append(changes, map[string]any{"t_ms": ms(o.cmp), "from": last, "to": s, "after_a_fault": faulted})
			}
			last = s
		} else {
			x.errs[o.typ+"/"+str(o.err, "phase")+"/"+str(o.err, "sqlstate")]++
		}
	}
	steady := map[string]any{}
	for k, x := range a {
		steady[k] = map[string]any{"ops": x.all, "ok": x.ok, "non_ok": x.errs, "latency_us_p50": pct(x.lat, 50), "latency_us_p95": pct(x.lat, 95), "latency_us_p99": pct(x.lat, 99)}
	}
	var pl []any
	for p, ph := range phases {
		pl = append(pl, map[string]any{"phase": p, "start_ms": ms(ph.start), "end_ms": ms(ph.end)})
	}
	sort.Slice(pl, func(i, j int) bool { return num(obj(pl[i])["phase"]) < num(obj(pl[j])["phase"]) })
	return map[string]any{"storm_phases": pl, "storm_ops": storm, "steady": steady, "servers": servers, "server_changes": changes}
}

// slots: K5.3's readiness from the cdc-slot observations: the first ready
// polling round before the first fault, and after each fault window the
// first round in which every follower again holds cp_slot failover-ready.
func slots(dir string, ws []window) (map[string]any, error) {
	obs, err := history.ReadChain(filepath.Join(dir, history.FileObservations))
	if err != nil {
		return nil, err
	}
	nodes := []string{"n1", "n2", "n3"}
	type round struct {
		t       int64
		ready   bool
		primary string
		states  []string
	}
	var rounds []round
	var cur []map[string]any
	for _, ev := range obs {
		v := obj(ev.Value)
		if str(v, "src") != "cdc-slot" {
			continue
		}
		if len(cur) < 3 && str(v, "node") == nodes[len(cur)] {
			cur = append(cur, v)
		} else if str(v, "node") == "n1" {
			cur = []map[string]any{v}
		} else {
			cur = nil
		}
		if len(cur) == 3 {
			r := round{t: ev.T, ready: true}
			prim := 0
			for _, s := range cur {
				b := func(k string) bool { return fmt.Sprint(s[k]) == "true" }
				st := fmt.Sprintf("%s:%s", str(s, "node"), map[bool]string{true: "standby", false: "primary"}[b("in_recovery")])
				switch {
				case !b("ok"):
					r.ready = false
					st = str(s, "node") + ":unreachable"
				case !b("in_recovery"):
					prim++
					r.primary = str(s, "node")
					if !b("present") || !b("failover") {
						r.ready = false
						st += "(no failover slot)"
					}
				default:
					if !(b("present") && b("failover") && b("synced") && !b("temporary") && !b("invalidated")) {
						r.ready = false
						st += fmt.Sprintf("(present=%v synced=%v temporary=%v invalidated=%v)", b("present"), b("synced"), b("temporary"), b("invalidated"))
					}
				}
				r.states = append(r.states, st)
			}
			if prim != 1 {
				r.ready = false
			}
			rounds = append(rounds, r)
			cur = nil
		}
	}
	if len(rounds) == 0 {
		return nil, nil
	}
	firstFault := int64(-1)
	if len(ws) > 0 {
		firstFault = ws[0].start
	}
	m := map[string]any{"rounds": int64(len(rounds))}
	for _, r := range rounds {
		if r.ready && (firstFault < 0 || r.t < firstFault) {
			m["first_ready_ms"], m["first_ready_primary"] = ms(r.t), r.primary
			break
		}
	}
	var after []any
	for i, w := range ws {
		next := int64(-1)
		if i+1 < len(ws) {
			next = ws[i+1].start
		}
		e := map[string]any{"fault": w.id + "/" + w.kind, "end_ms": ms(w.end)}
		var notReady []string
		for _, r := range rounds {
			if r.t < w.end || (next >= 0 && r.t > next) {
				continue
			}
			if r.ready {
				e["ready_again_ms"], e["ready_after_heal_ms"], e["primary"] = ms(r.t), ms(r.t-w.end), r.primary
				break
			}
			if len(notReady) == 0 {
				notReady = r.states
			}
		}
		if len(notReady) > 0 {
			e["first_state_after_heal"] = notReady
		}
		after = append(after, e)
	}
	m["after_faults"] = after
	return m, nil
}

func main() {
	var res []any
	for _, d := range os.Args[1:] {
		s, err := summarize(strings.TrimSuffix(d, "/"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		res = append(res, s)
	}
	b, err := canonjson.Marshal(map[string]any{"command": "k5obs", "runs": res})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Println(string(b))
}
