package runner

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/plan"
	"github.com/CesarManzoCode/commitproof/internal/workload"
)

// K5 runner parts (CONTRACT §7 W5–W8, §15 K5.3; ADR-0029, ADR-0030).

// procs hands out a client's logical processes: the client number first,
// then a fresh process after each info completion (Jepsen).
type procs struct {
	rec  *recorder
	proc int64
}

func (p *procs) after(out workload.Outcome) {
	if out.Type == workload.Info {
		p.proc = p.rec.nextProc.Add(1) - 1
	}
}

// op records one operation of a process: invoke, run, complete.
func (p *procs) op(client int64, gw, wl, f string, value any, iso string, run func() workload.Outcome) (workload.Outcome, error) {
	i, err := p.rec.invoke(p.proc, client, gw, wl, f, value, iso)
	if err != nil {
		return workload.Outcome{}, err
	}
	out := run()
	p.rec.complete(i, out, iso)
	p.after(out)
	return out, nil
}

// runSession is a W5 client: on every new connection it SETs the tracked
// GUCs and prepares its four statements, then issues the planned probes.
func runSession(wctx context.Context, c map[string]any, rec *recorder, start, deadline time.Time, pacing, opTimeout time.Duration) error {
	ci, _ := workload.Int(c["client"])
	gw, _ := c["gw"].(string)
	ops, _ := c["ops"].([]any)
	spec, _ := c["session"].(map[string]any)
	cur := map[string]string{}
	var order []string
	for _, x := range asList(spec["gucs"]) {
		pr := asList(x)
		if len(pr) != 2 {
			return fmt.Errorf("client %d: bad session gucs", ci)
		}
		g, _ := pr[0].(string)
		v, _ := pr[1].(string)
		cur[g] = v
		order = append(order, g)
	}
	type stmt struct {
		name, kind string
		value      int64
	}
	var stmts []stmt
	kind := map[string]string{}
	for _, x := range asList(spec["statements"]) {
		m, _ := x.(map[string]any)
		st := stmt{}
		st.name, _ = m["name"].(string)
		st.kind, _ = m["kind"].(string)
		st.value, _ = workload.Int(m["value"])
		stmts = append(stmts, st)
		kind[st.name] = st.kind
	}
	s := &workload.Session{GW: gw, AppName: fmt.Sprintf("commitproof-c%d", ci), Timeout: opTimeout}
	defer s.Close()
	p := &procs{rec: rec, proc: ci}
	conn := ci * 1000
	ctx := context.Background()
	set := func(g, v string) (workload.Outcome, error) {
		return p.op(ci, gw, plan.WLSession, "set", map[string]any{"conn": conn, "guc": g, "value": v}, "", func() workload.Outcome {
			out := s.GUCSet(ctx, g, v)
			if out.Type == workload.OK {
				out.Value = map[string]any{"conn": conn, "guc": g, "value": v}
			}
			return out
		})
	}
	next := start
	for _, ov := range ops {
		if d := time.Until(next); d > 0 {
			time.Sleep(d)
		}
		begun := time.Now()
		if !begun.Before(deadline) || wctx.Err() != nil {
			return nil
		}
		next = begun.Add(pacing)
		if !s.Connected() {
			// A new connection: its session state is set up again (ADR-0030).
			conn++
			for _, g := range order {
				if _, err := set(g, cur[g]); err != nil {
					return err
				}
			}
			for _, st := range stmts {
				v := map[string]any{"conn": conn, "name": st.name, "kind": st.kind, "sql": workload.StatementSQL(st.value), "expect": []any{st.value}}
				if _, err := p.op(ci, gw, plan.WLSession, "prepare", v, "", func() workload.Outcome { return s.Prepare(ctx, st.name, st.kind, st.value) }); err != nil {
					return err
				}
			}
		}
		op, _ := ov.(map[string]any)
		f, _ := op["f"].(string)
		val, _ := op["value"].(map[string]any)
		var err error
		switch f {
		case "show":
			g, _ := val["guc"].(string)
			_, err = p.op(ci, gw, plan.WLSession, "show", map[string]any{"conn": conn, "guc": g}, "", func() workload.Outcome {
				out := s.GUCShow(ctx, g)
				if out.Type == workload.OK {
					out.Value = map[string]any{"conn": conn, "guc": g, "value": out.Value}
				}
				return out
			})
		case "execute":
			name, _ := val["name"].(string)
			_, err = p.op(ci, gw, plan.WLSession, "execute", map[string]any{"conn": conn, "name": name}, "", func() workload.Outcome {
				out := s.Execute(ctx, name, kind[name])
				if out.Type == workload.OK {
					out.Value = map[string]any{"conn": conn, "name": name, "row": out.Value}
				}
				return out
			})
		case "set":
			g, _ := val["guc"].(string)
			v, _ := val["value"].(string)
			var out workload.Outcome
			if out, err = set(g, v); err == nil && out.Type != workload.Fail {
				cur[g] = v
			}
		default:
			return fmt.Errorf("client %d: unknown session op %q", ci, f)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func asList(v any) []any { l, _ := v.([]any); return l }

// runListener is a W6 listener: LISTEN on every (re)connect, then
// contiguous waits until end (the workload deadline + ListenerTail).
func runListener(c map[string]any, rec *recorder, end time.Time, opTimeout time.Duration) error {
	ci, _ := workload.Int(c["client"])
	gw, _ := c["gw"].(string)
	l := &workload.Listener{GW: gw, Timeout: opTimeout}
	defer l.Close()
	p := &procs{rec: rec, proc: ci}
	conn := ci * 1000
	ctx := context.Background()
	for time.Now().Before(end) {
		if !l.Connected() {
			conn++
			out, err := p.op(ci, gw, plan.WLNotify, "listen", map[string]any{"conn": conn}, "", func() workload.Outcome {
				out := l.Listen(ctx, plan.Channel)
				if out.Type == workload.OK {
					out.Value = map[string]any{"conn": conn}
				}
				return out
			})
			if err != nil {
				return err
			}
			if out.Type != workload.OK {
				time.Sleep(500 * time.Millisecond)
			}
			continue
		}
		d := min(plan.ListenerWait, time.Until(end))
		if d <= 0 {
			break
		}
		if _, err := p.op(ci, gw, plan.WLNotify, "wait", map[string]any{"conn": conn}, "", func() workload.Outcome {
			out := l.Wait(ctx, d)
			if out.Type == workload.OK {
				out.Value = map[string]any{"conn": conn, "payloads": out.Value}
			}
			return out
		}); err != nil {
			return err
		}
	}
	return nil
}

// runStorm runs W8's cp_storm phases: in each phase (clipped to the workload
// deadline) Connections connections, connection c through gateway c mod 3,
// each running select pg_sleep(0.5) back to back.
func runStorm(wctx context.Context, c map[string]any, rec *recorder, start, deadline time.Time, opTimeout time.Duration) error {
	base, _ := workload.Int(c["client"])
	n, _ := workload.Int(c["connections"])
	role, _ := c["role"].(string)
	var errMu sync.Mutex
	var firstErr error
	for pi, ph := range asList(c["phases_ms"]) {
		pr := asList(ph)
		if len(pr) != 2 {
			return fmt.Errorf("storm client %d: bad phase", base)
		}
		a, _ := workload.Int(pr[0])
		b, _ := workload.Int(pr[1])
		from := start.Add(time.Duration(a) * time.Millisecond)
		to := start.Add(time.Duration(b) * time.Millisecond)
		if to.After(deadline) {
			to = deadline
		}
		if !from.Before(to) {
			continue
		}
		if d := time.Until(from); d > 0 {
			time.Sleep(d)
		}
		if wctx.Err() != nil {
			return nil
		}
		phase := int64(pi + 1)
		var wg sync.WaitGroup
		for k := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				client := base + k
				gw := plan.Gateways[k%3]
				s := &workload.Session{GW: gw, AppName: fmt.Sprintf("commitproof-storm-%d", client), Timeout: opTimeout, User: role, Password: role}
				defer s.Close()
				p := &procs{rec: rec, proc: client}
				conn := client*1000 + phase
				for time.Now().Before(to) && wctx.Err() == nil {
					v := map[string]any{"role": role, "phase": phase, "conn": conn}
					out, err := p.op(client, gw, plan.WLOverload, "storm", v, "", func() workload.Outcome {
						out := s.Storm(context.Background())
						if out.Type == workload.OK {
							out.Value = v
						}
						return out
					})
					if err != nil {
						errMu.Lock()
						firstErr = err
						errMu.Unlock()
						return
					}
					if out.Type != workload.OK && out.Attempt.At == workload.PhaseConnect {
						time.Sleep(500 * time.Millisecond)
					}
				}
			}()
		}
		wg.Wait()
	}
	return firstErr
}

// slotState is one node's view of cp_slot (K5.3).
type slotState struct {
	Node                                     string
	OK                                       bool
	Error                                    string
	InRecovery, Present                      bool
	Failover, Synced, Temporary, Invalidated bool
	ConfirmedFlush, InvalidationReason, Raw  string
}

func (s slotState) Map() map[string]any {
	return map[string]any{"src": "cdc-slot", "node": s.Node, "ok": s.OK, "error": s.Error, "in_recovery": s.InRecovery,
		"present": s.Present, "failover": s.Failover, "synced": s.Synced, "temporary": s.Temporary, "invalidated": s.Invalidated,
		"invalidation_reason": s.InvalidationReason, "confirmed_flush": s.ConfirmedFlush, "raw": s.Raw}
}

// failoverReady is upstream's readiness test (S12; ADR-0030).
func (s slotState) failoverReady() bool {
	return s.OK && s.Present && s.Failover && s.Synced && !s.Temporary && !s.Invalidated
}

const slotQuery = "select pg_is_in_recovery(), coalesce((select failover::text || ',' || synced::text || ',' || temporary::text || ',' || " +
	"coalesce(invalidation_reason, '') || ',' || coalesce(confirmed_flush_lsn::text, '') from pg_replication_slots where slot_name = '" + plan.Slot + "'), 'absent')"

// querySlot reads cp_slot on node through psql in its container (as
// QueryNodeSQL), bounded by P01.
func querySlot(ctx context.Context, node string) slotState {
	st := slotState{Node: node}
	ctx, cancel := context.WithTimeout(ctx, params.ClientOpTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "exec", "commitproof-pg-"+node+"-1", "timeout", "5", "psql", "-h", "/data/pg_sockets",
		"-p", "5432", "-U", "postgres", "-d", "postgres", "-X", "-A", "-t", "-F", "|", "-v", "ON_ERROR_STOP=1", "-c", slotQuery).CombinedOutput()
	st.Raw = strings.TrimSpace(string(out))
	if err != nil {
		st.Error = fmt.Sprintf("%v: %s", err, truncate(st.Raw, 256))
		return st
	}
	f := strings.SplitN(st.Raw, "|", 2)
	if len(f) != 2 {
		st.Error = "unexpected psql output"
		return st
	}
	st.OK, st.InRecovery = true, f[0] == "t"
	if f[1] == "absent" {
		return st
	}
	g := strings.Split(f[1], ",")
	if len(g) != 5 {
		st.OK, st.Error = false, "unexpected slot row"
		return st
	}
	st.Present = true
	st.Failover, st.Synced, st.Temporary = g[0] == "true", g[1] == "true", g[2] == "true"
	st.InvalidationReason, st.Invalidated = g[3], g[3] != ""
	st.ConfirmedFlush = g[4]
	return st
}

// slotReady evaluates K5.3 on one polling round: every node in recovery
// holds cp_slot failover-ready, the primary's cp_slot has failover = true,
// and every polled node answered (fail-closed).
func slotReady(sts []slotState) (bool, string) {
	primaries, followers := 0, 0
	for _, s := range sts {
		switch {
		case !s.OK:
			return false, s.Node + ": " + s.Error
		case !s.InRecovery:
			primaries++
			if !s.Present || !s.Failover {
				return false, s.Node + " (primary): cp_slot missing or not failover"
			}
		default:
			followers++
			if !s.failoverReady() {
				return false, fmt.Sprintf("%s: cp_slot present=%v failover=%v synced=%v temporary=%v invalidated=%v", s.Node, s.Present, s.Failover, s.Synced, s.Temporary, s.Invalidated)
			}
		}
	}
	if primaries != 1 || followers == 0 {
		return false, fmt.Sprintf("%d primaries, %d followers", primaries, followers)
	}
	return true, ""
}

// pollSlots records cp_slot on every node of nodes once.
func pollSlots(ctx context.Context, nodes []string, emit func(map[string]any)) []slotState {
	sts := make([]slotState, len(nodes))
	var wg sync.WaitGroup
	for i, n := range nodes {
		wg.Add(1)
		go func() { defer wg.Done(); sts[i] = querySlot(ctx, n) }()
	}
	wg.Wait()
	for _, s := range sts {
		emit(s.Map())
	}
	return sts
}

// awaitSlotReady is K5.3's wait before the workload (ADR-0030): poll every
// node every P24 and write one warm-up row per second through gw-z1 (client
// 14, ledger ids 8 000 000 000 + n) until the slot is failover-ready on
// every follower, within SlotReadyDeadline.
func awaitSlotReady(rec *recorder, opTimeout time.Duration) (bool, string, time.Duration) {
	begun := time.Now()
	s := &workload.Session{GW: plan.Gateways[0], AppName: "commitproof-warmup", Timeout: opTimeout}
	defer s.Close()
	p := &procs{rec: rec, proc: plan.WarmupClient}
	why := "not polled"
	for n := int64(1); time.Since(begun) < plan.SlotReadyDeadline; n++ {
		v := map[string]any{"id": 8_000_000_000 + n, "payload": fmt.Sprintf("warmup-%d", n)}
		if _, err := p.op(plan.WarmupClient, plan.Gateways[0], plan.WLLedger, "insert", v, "", func() workload.Outcome {
			return s.Ledger(context.Background(), v)
		}); err != nil {
			return false, err.Error(), time.Since(begun)
		}
		ok, w := slotReady(pollSlots(context.Background(), Nodes, rec.obs))
		if ok {
			return true, "", time.Since(begun)
		}
		why = w
		time.Sleep(params.ObservationInterval)
	}
	return false, why, time.Since(begun)
}

// runConsumer starts W7 and returns it with a stop function.
func runConsumer(c map[string]any, rec *recorder, publication string, opTimeout time.Duration) (*workload.Consumer, func()) {
	ci, _ := workload.Int(c["client"])
	gw, _ := c["gw"].(string)
	p := &procs{rec: rec, proc: ci}
	var mu sync.Mutex
	cons := &workload.Consumer{GW: gw, Slot: plan.Slot, Publication: publication, Timeout: opTimeout, StatusInterval: time.Second}
	cons.Record = func(f string, in map[string]any) func(workload.Outcome) {
		mu.Lock()
		proc := p.proc
		if f == "caught-up" {
			// caught-up runs beside the stream's own operations: its own
			// process of the same client.
			proc = rec.nextProc.Add(1) - 1
		}
		i, err := rec.invoke(proc, ci, gw, plan.WLCDC, f, in, "")
		mu.Unlock()
		return func(out workload.Outcome) {
			if err != nil {
				return
			}
			rec.complete(i, out, "")
			if f != "caught-up" {
				mu.Lock()
				p.after(out)
				mu.Unlock()
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); cons.Run(ctx) }()
	return cons, func() { cancel(); <-done }
}
