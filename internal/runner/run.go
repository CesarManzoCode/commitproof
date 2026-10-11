package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/buildinfo"
	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/cmdlog"
	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/observe"
	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/plan"
	"github.com/CesarManzoCode/commitproof/internal/workload"
)

// Run statuses written in the run-end marker.
const (
	StatusComplete = "complete"
	StatusInvalid  = "invalid"
)

// Recovery and final-read clients: one per gateway, numbered after the
// plan's clients (client 10 → gw-z1, 11 → gw-z2, 12 → gw-z3).
func finalClient(g int) int64 { return int64(params.ClientsTotal + g) }

// Recovery writes use ledger ids far above any planned id.
func recoveryID(g, attempt int) int64 { return 9_000_000_000 + int64(g)*1_000_000 + int64(attempt) }

type openOp struct {
	proc, client int64
	gw, wl, f    string
	value        any
	closed       bool
}

// recorder writes client events to history.jsonl and tracks open ops so
// they can be closed as info at the end (CONTRACT §15 K1.7).
type recorder struct {
	h        *history.Writer
	o        *history.Writer
	mu       sync.Mutex
	open     map[int64]*openOp
	nextProc atomic.Int64
}

func (r *recorder) marker(name string, v map[string]any) {
	val := map[string]any{"name": name}
	for k, x := range v {
		val[k] = x
	}
	r.h.Append(map[string]any{"type": history.TypeMarker, "value": val})
}

func (r *recorder) obs(v map[string]any) {
	r.o.Append(map[string]any{"type": history.TypeObs, "value": v})
}

func (r *recorder) invoke(proc, client int64, gw, wl, f string, value any, iso string) (int64, error) {
	ev := map[string]any{"type": history.TypeInvoke, "proc": proc, "client": client, "gw": gw, "wl": wl, "f": f, "value": value}
	if iso != "" {
		ev["iso"] = iso
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	i, _, err := r.h.Append(ev)
	if err != nil {
		return 0, err
	}
	r.open[i] = &openOp{proc: proc, client: client, gw: gw, wl: wl, f: f, value: value}
	return i, nil
}

// complete writes the completion of op i. It returns false if the op was
// already closed as info by closeOpen (the late result is then only
// observed).
func (r *recorder) complete(i int64, out workload.Outcome, iso string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	op, ok := r.open[i]
	if !ok || op.closed {
		r.o.Append(map[string]any{"type": history.TypeObs, "value": map[string]any{
			"src": "client", "event": "late-completion", "op": i, "result": out.Type}})
		return false
	}
	op.closed = true
	delete(r.open, i)
	ev := map[string]any{"type": out.Type, "proc": op.proc, "client": op.client, "gw": op.gw, "wl": op.wl, "f": op.f, "op": i}
	if iso != "" {
		ev["iso"] = iso
	}
	if out.Type == workload.OK && out.Value != nil {
		ev["value"] = out.Value
	} else {
		ev["value"] = op.value
	}
	if out.Err != nil {
		ev["err"] = out.Err
	}
	if len(out.Notices) > 0 {
		ev["notices"] = out.Notices
	}
	if _, _, err := r.h.Append(ev); err != nil {
		return false
	}
	if out.Err != nil {
		r.o.Append(map[string]any{"type": history.TypeObs, "value": map[string]any{
			"src": "client", "event": "error", "op": i, "at": out.Attempt.At, "message": truncate(out.Attempt.Message, 512)}})
	}
	return true
}

// closeOpen closes every open op as info (client-timeout): its outcome is
// unknown to the history.
func (r *recorder) closeOpen() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for i, op := range r.open {
		op.closed = true
		r.h.Append(map[string]any{"type": history.TypeInfo, "proc": op.proc, "client": op.client, "gw": op.gw,
			"wl": op.wl, "f": op.f, "op": i, "value": op.value,
			"err": map[string]any{"sqlstate": "", "class": "timeout", "phase": workload.PhaseTimeout}})
		delete(r.open, i)
		n++
	}
	return n
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Run executes the plan in runDir inside the runner container and writes
// history.jsonl, observations.jsonl and the runner's commands.jsonl records.
// It returns the run status and, for invalid runs, the reason.
func Run(ctx context.Context, runDir string) (string, string, error) {
	raw, err := os.ReadFile(filepath.Join(runDir, history.FilePlan))
	if err != nil {
		return StatusInvalid, "", fmt.Errorf("read plan: %w", err)
	}
	pv, err := canonjson.Decode(bytes.TrimSpace(raw))
	if err != nil {
		return StatusInvalid, "", fmt.Errorf("plan: %w", err)
	}
	p, ok := pv.(map[string]any)
	if !ok {
		return StatusInvalid, "", fmt.Errorf("plan is not an object")
	}
	durMS, err1 := workload.Int(p["duration_ms"])
	paceMS, err2 := workload.Int(p["pacing_ms"])
	timeoutMS, err3 := workload.Int(p["client_timeout_ms"])
	if err := firstErr(err1, err2, err3); err != nil {
		return StatusInvalid, "", fmt.Errorf("plan: %w", err)
	}
	clients, _ := p["clients"].([]any)

	origin := time.Now()
	monoOrigin := cmdlog.MonoNS()
	hw, err := history.Create(filepath.Join(runDir, history.FileHistory), origin)
	if err != nil {
		return StatusInvalid, "", err
	}
	ow, err := history.Create(filepath.Join(runDir, history.FileObservations), origin)
	if err != nil {
		hw.Close()
		return StatusInvalid, "", err
	}
	rec := &recorder{h: hw, o: ow, open: map[int64]*openOp{}}
	rec.nextProc.Store(int64(params.ClientsTotal + len(plan.Gateways)))
	finish := func(status, reason string) (string, string, error) {
		n := rec.closeOpen()
		rec.marker("run-end", map[string]any{"status": status, "reason": reason, "closed_open_ops": n})
		e1, e2 := hw.Close(), ow.Close()
		if err := firstErr(e1, e2); err != nil {
			return StatusInvalid, reason, err
		}
		return status, reason, nil
	}
	rec.marker("run-start", map[string]any{
		"mono_origin_ns": monoOrigin, "commitproof_commit": buildinfo.Commit, "commitproof_dirty": buildinfo.Dirty,
		"contract_version": params.ContractVersion,
	})

	// The cluster must be HEALTHY at start (CONTRACT §12.1).
	oc := observe.NewClient()
	defer oc.Close()
	h := CheckHealthy(ctx, oc, time.Now().UnixNano())
	hv := h.Map()
	hv["src"] = "healthy"
	hv["phase"] = "run-start"
	rec.obs(hv)
	if !h.Healthy {
		return finish(StatusInvalid, "cluster not HEALTHY at start")
	}

	// Setup transaction through gw-z1 (CONTRACT §7), recorded as markers.
	stmts := workload.Setup(params.BankAccounts, params.BankInitialBalance)
	stmtsAny := make([]any, len(stmts))
	for i, s := range stmts {
		stmtsAny[i] = s
	}
	rec.marker("setup-begin", map[string]any{"gw": plan.Gateways[0], "statements": stmtsAny})
	setup := &workload.Session{GW: plan.Gateways[0], AppName: "commitproof-setup", Timeout: params.ClientOpTimeout}
	out := setup.RunSetup(context.Background(), stmts)
	setup.Close()
	sv := map[string]any{"result": out.Type}
	if out.Err != nil {
		sv["err"] = out.Err
		sv["message"] = truncate(out.Attempt.Message, 512)
	}
	rec.marker("setup-end", sv)
	if out.Type != workload.OK {
		return finish(StatusInvalid, "setup transaction did not commit")
	}

	// Observation pollers (P24), one per node.
	obsCtx, stopObs := context.WithCancel(ctx)
	var obsWG sync.WaitGroup
	for _, n := range Nodes {
		obsWG.Add(1)
		go func(node string) {
			defer obsWG.Done()
			poc := observe.NewClient()
			defer poc.Close()
			t := time.NewTicker(params.ObservationInterval)
			defer t.Stop()
			for {
				st := poc.Status(obsCtx, node, params.ClientOpTimeout)
				rec.obs(st.Map())
				select {
				case <-obsCtx.Done():
					return
				case <-t.C:
				}
			}
		}(n)
	}

	// Workload (CONTRACT §7, P02–P04).
	pacing := time.Duration(paceMS) * time.Millisecond
	opTimeout := time.Duration(timeoutMS) * time.Millisecond
	start := time.Now()
	deadline := start.Add(time.Duration(durMS) * time.Millisecond)
	rec.marker("workload-start", map[string]any{"duration_ms": durMS, "pacing_ms": paceMS})
	var wg sync.WaitGroup
	var internalErr atomic.Value
	for _, cv := range clients {
		c, _ := cv.(map[string]any)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runClient(c, rec, start, deadline, pacing, opTimeout); err != nil {
				internalErr.Store(err.Error())
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Until(deadline) + opTimeout + 5*time.Second):
		rec.obs(map[string]any{"src": "runner", "event": "clients-overran", "note": "open ops closed as info"})
	}
	rec.marker("workload-end", map[string]any{"open_ops_closed": rec.closeOpen()})
	if e, ok := internalErr.Load().(string); ok {
		stopObs()
		obsWG.Wait()
		return finish(StatusInvalid, "runner internal error: "+e)
	}

	// Recovery (P06): a successful write through each gateway.
	rec.marker("recovery-begin", nil)
	recovered := parallel(len(plan.Gateways), func(g int) bool {
		return recoverGateway(rec, g, opTimeout)
	})
	rec.marker("recovery-end", map[string]any{"recovered": boolsAny(recovered)})
	for _, ok := range recovered {
		if !ok {
			stopObs()
			obsWG.Wait()
			return finish(StatusInvalid, "recovery deadline P06 exceeded")
		}
	}

	// Quiescence (P07), then stable final reads (P08).
	rec.marker("quiescence-begin", map[string]any{"ms": params.Quiescence.Milliseconds()})
	time.Sleep(params.Quiescence)
	rec.marker("final-reads-begin", nil)
	stable := parallel(len(plan.Gateways), func(g int) bool {
		return finalReads(rec, g, opTimeout)
	})
	rec.marker("final-reads-end", map[string]any{"stable": boolsAny(stable)})
	stopObs()
	obsWG.Wait()
	for _, ok := range stable {
		if !ok {
			return finish(StatusInvalid, "final reads not stable within P08")
		}
	}
	return finish(StatusComplete, "")
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func boolsAny(b []bool) []any {
	out := make([]any, len(b))
	for i, x := range b {
		out[i] = x
	}
	return out
}

func parallel(n int, f func(i int) bool) []bool {
	out := make([]bool, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() { defer wg.Done(); out[i] = f(i) }()
	}
	wg.Wait()
	return out
}

// runClient issues one client's planned operations, closed loop (P03), until
// the workload deadline. Operations started before the deadline run to
// completion (bounded by P01).
func runClient(c map[string]any, rec *recorder, start, deadline time.Time, pacing, opTimeout time.Duration) error {
	ci, err := workload.Int(c["client"])
	if err != nil {
		return fmt.Errorf("plan client: %w", err)
	}
	wl, _ := c["wl"].(string)
	gw, _ := c["gw"].(string)
	iso, _ := c["iso"].(string)
	ops, _ := c["ops"].([]any)
	s := &workload.Session{GW: gw, AppName: fmt.Sprintf("commitproof-c%d", ci), Timeout: opTimeout}
	defer s.Close()
	proc := ci
	// P03, closed loop: an operation starts one pacing interval after the
	// previous one started, or when the previous one completes if that is
	// later. A slow operation delays the schedule; it never causes a burst.
	next := start
	for _, ov := range ops {
		if d := time.Until(next); d > 0 {
			time.Sleep(d)
		}
		begun := time.Now()
		if !begun.Before(deadline) {
			return nil
		}
		next = begun.Add(pacing)
		op, _ := ov.(map[string]any)
		f, _ := op["f"].(string)
		val := op["value"]
		txIso := ""
		if wl == plan.WLAppend || wl == plan.WLBank {
			txIso = iso
		}
		i, err := rec.invoke(proc, ci, gw, wl, f, val, txIso)
		if err != nil {
			return err
		}
		var out workload.Outcome
		ctx := context.Background()
		switch wl {
		case plan.WLAppend:
			mops, _ := val.([]any)
			out = s.Txn(ctx, iso, mops)
		case plan.WLRegister:
			m, _ := val.(map[string]any)
			out = s.Register(ctx, f, m)
		case plan.WLBank:
			m, _ := val.(map[string]any)
			out = s.Bank(ctx, f, m)
		case plan.WLLedger:
			m, _ := val.(map[string]any)
			out = s.Ledger(ctx, m)
		default:
			return fmt.Errorf("client %d: unknown workload %q", ci, wl)
		}
		rec.complete(i, out, txIso)
		if out.Type == workload.Info {
			proc = rec.nextProc.Add(1) - 1 // Jepsen: the client continues as a new process
		}
	}
	return nil
}

// recoverGateway retries a ledger insert through gateway g until one is ok
// or P06 expires.
func recoverGateway(rec *recorder, g int, opTimeout time.Duration) bool {
	gw := plan.Gateways[g]
	client := finalClient(g)
	proc := client
	s := &workload.Session{GW: gw, AppName: fmt.Sprintf("commitproof-recovery-%s", gw), Timeout: opTimeout}
	defer s.Close()
	end := time.Now().Add(params.RecoveryDeadline)
	for attempt := 0; time.Now().Before(end); attempt++ {
		v := map[string]any{"id": recoveryID(g, attempt), "payload": fmt.Sprintf("recovery-%s-%d", gw, attempt)}
		i, err := rec.invoke(proc, client, gw, plan.WLLedger, "insert", v, "")
		if err != nil {
			return false
		}
		out := s.Ledger(context.Background(), v)
		rec.complete(i, out, "")
		switch out.Type {
		case workload.OK:
			return true
		case workload.Info:
			proc = rec.nextProc.Add(1) - 1
		}
		time.Sleep(time.Second)
	}
	return false
}

// finalReads reads every workload's full state through gateway g every P08
// interval until P08's repeat count of consecutive identical rounds, within
// P08's deadline.
func finalReads(rec *recorder, g int, opTimeout time.Duration) bool {
	gw := plan.Gateways[g]
	client := finalClient(g)
	proc := client
	s := &workload.Session{GW: gw, AppName: fmt.Sprintf("commitproof-final-%s", gw), Timeout: opTimeout}
	defer s.Close()
	end := time.Now().Add(params.FinalReadDeadline)
	var prev []byte
	consecutive := 0
	for round := 0; time.Now().Before(end); round++ {
		var vals []any
		okRound := true
		for _, wl := range []string{plan.WLAppend, plan.WLRegister, plan.WLBank, plan.WLLedger} {
			i, err := rec.invoke(proc, client, gw, wl, "final-read", map[string]any{}, "")
			if err != nil {
				return false
			}
			out := s.FinalRead(context.Background(), wl)
			rec.complete(i, out, "")
			if out.Type != workload.OK {
				okRound = false
				if out.Type == workload.Info {
					proc = rec.nextProc.Add(1) - 1
				}
				break
			}
			vals = append(vals, out.Value)
		}
		if okRound {
			cur, err := canonjson.Marshal(vals)
			if err != nil {
				return false
			}
			if prev != nil && bytes.Equal(cur, prev) {
				consecutive++
			} else {
				consecutive = 1
			}
			prev = cur
			if consecutive >= params.FinalReadRepeats {
				rec.marker("final-read-stable", map[string]any{"gw": gw, "rounds": round + 1})
				return true
			}
		} else {
			consecutive, prev = 0, nil
		}
		time.Sleep(params.FinalReadInterval)
	}
	rec.marker("final-read-unstable", map[string]any{"gw": gw})
	return false
}
