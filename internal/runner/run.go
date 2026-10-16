package runner

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/buildinfo"
	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/cmdlog"
	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/nemesis"
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
func (r *recorder) closeOpen() int { return r.closeOpenExcept(nil) }

// closeOpenExcept is closeOpen for the ops of clients keep does not name
// (W6 listeners and the W7 consumer outlive the workload deadline).
func (r *recorder) closeOpenExcept(keep map[int64]bool) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for i, op := range r.open {
		if keep[op.client] {
			continue
		}
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

// Config is one runner execution.
type Config struct {
	RunDir       string
	NemesisImage string // locked image ID of the nemesis helper (plans with faults)
	NetPrefix    string // cpnet /24 prefix, "10.77.0"
}

// Run executes the plan in the run directory inside the runner container
// and writes history.jsonl, observations.jsonl and the runner's
// commands.jsonl records (nemesis commands). It returns the run status and,
// for invalid runs, the reason.
func Run(ctx context.Context, cfg Config) (string, string, error) {
	runDir := cfg.RunDir
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
	profile, _ := p["profile"].(string)
	setupPlan, _ := p["setup"].(map[string]any)
	publication, _ := setupPlan["publication"].(string)
	var roles []string
	for _, x := range asList(setupPlan["roles"]) {
		if r, ok := x.(string); ok {
			roles = append(roles, r)
		}
	}
	cdcSlot, _ := setupPlan["cdc_slot"].(string)
	var steps []plan.Step
	if nv, ok := p["nemesis"].([]any); ok && len(nv) > 0 {
		if steps, err = plan.ParseSteps(nv); err != nil {
			return StatusInvalid, "", fmt.Errorf("plan: %w", err)
		}
	}

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
	rec.nextProc.Store(firstFreshProc(clients))
	finish := func(status, reason string) (string, string, error) {
		n := rec.closeOpen()
		rec.marker("run-end", map[string]any{"status": status, "reason": reason, "closed_open_ops": n})
		e1, e2 := hw.Close(), ow.Close()
		if err := firstErr(e1, e2); err != nil {
			return StatusInvalid, reason, err
		}
		return status, reason, nil
	}
	sab := []any{}
	for _, s := range nemesis.ActiveSabotage() {
		sab = append(sab, "cp_sabotage_"+s)
	}
	rec.marker("run-start", map[string]any{
		"mono_origin_ns": monoOrigin, "commitproof_commit": buildinfo.Commit, "commitproof_dirty": buildinfo.Dirty,
		"contract_version": params.ContractVersion, "sabotage": sab,
	})

	// The cluster must be HEALTHY at start (CONTRACT §12.1).
	oc := observe.NewClient()
	defer oc.Close()
	h := CheckHealthy(ctx, oc, time.Now().UnixNano(), profile)
	healthyAt := time.Now()
	hv := h.Map()
	hv["src"] = "healthy"
	hv["phase"] = "run-start"
	rec.obs(hv)
	if !h.Healthy {
		return finish(StatusInvalid, "cluster not HEALTHY at start")
	}

	// Setup transaction through gw-z1 (CONTRACT §7), recorded as markers.
	stmts := append(workload.Setup(params.BankAccounts, params.BankInitialBalance), workload.SetupK5(publication, roles)...)
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

	// Observation pollers (P24), one per node, and metric scrapers (P23,
	// ADR-0038), one per gateway, pooler and orchestrator; n4's start when
	// F5a starts the node. Profile h1 has no exporter and is not scraped.
	obsCtx, stopObs := context.WithCancel(ctx)
	var obsWG sync.WaitGroup
	var pollMu sync.Mutex
	polled := map[string]bool{}
	scraping := profile != ProfileH1 && cfg.NetPrefix != ""
	scraper := observe.NewScraper()
	defer scraper.Close()
	startScraper := func(t observe.MetricsTarget) {
		obsWG.Add(1)
		go func() {
			defer obsWG.Done()
			tk := time.NewTicker(params.MetricsScrapeInterval)
			defer tk.Stop()
			for {
				rec.obs(scraper.Scrape(obsCtx, t))
				select {
				case <-obsCtx.Done():
					return
				case <-tk.C:
				}
			}
		}()
	}
	startPoller := func(node string) {
		pollMu.Lock()
		defer pollMu.Unlock()
		if polled[node] {
			return
		}
		polled[node] = true
		if scraping {
			startScraper(observe.PoolerMetricsTarget(cfg.NetPrefix, node))
		}
		obsWG.Add(1)
		if profile == ProfileH1 {
			// The historical pooler API differs from the pinned protos:
			// observe PostgreSQL itself (ADR-0025).
			go func() {
				defer obsWG.Done()
				pollSQL(obsCtx, node, rec.obs)
			}()
			return
		}
		go func() {
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
		}()
	}
	if scraping {
		for _, t := range observe.MetricsTargets(cfg.NetPrefix, nil) {
			startScraper(t)
		}
	} else if profile != ProfileH1 {
		rec.obs(map[string]any{"src": "runner", "event": "metrics-not-scraped", "reason": "no --net-prefix"})
	}
	for _, n := range Nodes {
		startPoller(n)
	}
	stopAll := func() {
		stopObs()
		obsWG.Wait()
	}

	// Nemesis (CONTRACT §8): commands recorded in the run's commands.jsonl
	// with the runner- prefix, events in history.jsonl.
	var nem *nemesis.Env
	if len(steps) > 0 {
		if cfg.NemesisImage == "" || cfg.NetPrefix == "" {
			stopAll()
			return finish(StatusInvalid, "runner: a plan with faults needs --nemesis-image and --net-prefix")
		}
		cr, err := cmdlog.New(filepath.Join(runDir, history.FileCommands), runDir, filepath.Join(runDir, history.DirLogs, "cmd"), "runner-")
		if err != nil {
			stopAll()
			return finish(StatusInvalid, "runner: "+err.Error())
		}
		exe, err := os.Executable()
		if err != nil {
			stopAll()
			return finish(StatusInvalid, "runner: "+err.Error())
		}
		nem = &nemesis.Env{Rec: cr, Prefix: cfg.NetPrefix, Exe: exe, NemImage: cfg.NemesisImage,
			Emit: func(f map[string]any) error {
				_, _, err := hw.Append(map[string]any{"type": history.TypeNemesis, "fault": f})
				return err
			},
			Marker:      rec.marker,
			NodeStarted: startPoller,
			Profile:     profile,
		}
		// Peers probe the runner's namespace on this port (F1).
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", nemesis.RunnerProbePort))
		if err != nil {
			stopAll()
			return finish(StatusInvalid, "runner probe listener: "+err.Error())
		}
		defer ln.Close()
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				c.Close()
			}
		}()
		if err := nem.StartHelper(ctx); err != nil {
			nem.StopHelper(context.Background())
			stopAll()
			return finish(StatusInvalid, "nemesis helper: "+err.Error())
		}
		defer nem.StopHelper(context.Background())
	}

	// Workload (CONTRACT §7, P02–P04).
	pacing := time.Duration(paceMS) * time.Millisecond
	opTimeout := time.Duration(timeoutMS) * time.Millisecond

	// K5 clients that are not planned op loops (ADR-0030): W6 listeners and
	// the W7 consumer outlive the workload deadline; the W8 storm runs its
	// phases.
	var opClients, listeners, storms []map[string]any
	var consumerPlan map[string]any
	extended := map[int64]bool{}
	for _, cv := range clients {
		c, _ := cv.(map[string]any)
		ci, _ := workload.Int(c["client"])
		switch mode, _ := c["mode"].(string); mode {
		case plan.ModeListener:
			listeners = append(listeners, c)
			extended[ci] = true
		case plan.ModeConsumer:
			consumerPlan = c
			extended[ci] = true
		case plan.ModeStorm:
			storms = append(storms, c)
		case "":
			opClients = append(opClients, c)
		default:
			stopAll()
			return finish(StatusInvalid, fmt.Sprintf("plan client %d: unknown mode %q", ci, mode))
		}
	}
	// W7: the consumer creates the slot and streams before the workload;
	// K5.3's precondition is awaited here (ADR-0030).
	var consumer *workload.Consumer
	stopConsumer := func() {}
	if consumerPlan != nil {
		consumer, stopConsumer = runConsumer(consumerPlan, rec, publication, opTimeout)
		select {
		case <-consumer.Ready():
		case <-time.After(plan.SlotReadyDeadline):
		}
		select {
		case <-consumer.Ready():
		default:
			stopConsumer()
			stopAll()
			return finish(StatusInvalid, fmt.Sprintf("W7: the consumer did not start streaming within %s (slot creation: %v)", plan.SlotReadyDeadline, consumer.CreateErr()))
		}
		ready, why, waited := awaitSlotReady(rec, opTimeout)
		rec.marker("cdc-slot-ready", map[string]any{"ready": ready, "reason": why, "waited_ms": waited.Milliseconds(), "mode": cdcSlot})
		if !ready && cdcSlot == plan.CDCSlotRequired {
			stopConsumer()
			stopAll()
			return finish(StatusInvalid, "K5.3: cp_slot not failover-ready on every follower before the workload: "+why)
		}
		obsWG.Add(1)
		go func() {
			defer obsWG.Done()
			t := time.NewTicker(params.ObservationInterval)
			defer t.Stop()
			for {
				select {
				case <-obsCtx.Done():
					return
				case <-t.C:
					pollSlots(obsCtx, Nodes, rec.obs)
				}
			}
		}()
	}
	defer stopConsumer()
	// Profile buffered: bootstrap's buffering episode must not suppress the
	// first failover's (ADR-0029: buffer-min-time-between-failovers).
	if profile == "buffered" {
		wait := time.Until(healthyAt.Add(plan.BufferGuardWait))
		rec.marker("buffer-guard-wait", map[string]any{"ms": max(wait, 0).Milliseconds(), "guard_ms": plan.BufferGuardWait.Milliseconds()})
		if wait > 0 {
			time.Sleep(wait)
		}
	}
	start := time.Now()
	deadline := start.Add(time.Duration(durMS) * time.Millisecond)
	rec.marker("workload-start", map[string]any{"duration_ms": durMS, "pacing_ms": paceMS, "nemesis_steps": int64(len(steps))})
	// A fault not-applied or not-healed aborts the workload (CONTRACT §8).
	wctx, abort := context.WithCancel(ctx)
	defer abort()
	nemDone := make(chan nemesis.Outcome, 1)
	if nem != nil {
		go func() {
			out := nem.Run(ctx, steps, start)
			if !out.OK {
				abort()
			}
			nemDone <- out
		}()
	} else {
		nemDone <- nemesis.Outcome{OK: true}
	}
	var wg, lwg sync.WaitGroup
	var internalErr atomic.Value
	for _, c := range opClients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run := runClient
			if wl, _ := c["wl"].(string); wl == plan.WLSession {
				run = runSession
			}
			if err := run(wctx, c, rec, start, deadline, pacing, opTimeout); err != nil {
				internalErr.Store(err.Error())
			}
		}()
	}
	for _, c := range storms {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runStorm(wctx, c, rec, start, deadline, opTimeout); err != nil {
				internalErr.Store(err.Error())
			}
		}()
	}
	for _, c := range listeners {
		lwg.Add(1)
		go func() {
			defer lwg.Done()
			if err := runListener(c, rec, deadline.Add(plan.ListenerTail), opTimeout); err != nil {
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
	rec.marker("workload-end", map[string]any{"open_ops_closed": rec.closeOpenExcept(extended), "aborted": wctx.Err() != nil})
	// The nemesis may still be healing after the workload deadline;
	// recovery (P06) starts from the final heal.
	nout := <-nemDone
	if nem != nil {
		rec.marker("nemesis-end", map[string]any{"ok": nout.OK, "reason": nout.Reason, "faults_confirmed": int64(nout.Faults)})
	}
	if !nout.OK {
		stopAll()
		return finish(StatusInvalid, nout.Reason)
	}
	if e, ok := internalErr.Load().(string); ok {
		stopAll()
		return finish(StatusInvalid, "runner internal error: "+e)
	}

	// Recovery (P06): a successful write through each gateway. The runner's
	// client of each gateway keeps one process across recovery and final
	// reads: an info completion of either retires it (§6.1).
	procs := make([]int64, len(plan.Gateways))
	for g := range procs {
		procs[g] = finalClient(g)
	}
	rec.marker("recovery-begin", nil)
	recovered := parallel(len(plan.Gateways), func(g int) bool {
		s := &workload.Session{GW: plan.Gateways[g], AppName: fmt.Sprintf("commitproof-recovery-%s", plan.Gateways[g]), Timeout: opTimeout}
		defer s.Close()
		return recoverGateway(rec, g, s, &procs[g])
	})
	rec.marker("recovery-end", map[string]any{"recovered": boolsAny(recovered)})
	for _, ok := range recovered {
		if !ok {
			stopAll()
			return finish(StatusInvalid, "recovery deadline P06 exceeded")
		}
	}

	// Quiescence (P07), then stable final reads (P08).
	rec.marker("quiescence-begin", map[string]any{"ms": params.Quiescence.Milliseconds()})
	time.Sleep(params.Quiescence)
	if len(listeners) > 0 {
		lwg.Wait()
		rec.marker("listeners-end", nil)
	}
	rec.marker("final-reads-begin", nil)
	stable := parallel(len(plan.Gateways), func(g int) bool {
		s := &workload.Session{GW: plan.Gateways[g], AppName: fmt.Sprintf("commitproof-final-%s", plan.Gateways[g]), Timeout: opTimeout}
		defer s.Close()
		return finalReads(rec, g, s, &procs[g])
	})
	rec.marker("final-reads-end", map[string]any{"stable": boolsAny(stable)})
	if consumer != nil {
		// W7: everything committed up to the server's position after the
		// final reads must reach the consumer (C7 cdc-gap needs it).
		ws := &workload.Session{GW: plan.Gateways[0], AppName: "commitproof-cdc-target", Timeout: opTimeout}
		target, err := ws.ServerWALPosition(context.Background())
		ws.Close()
		if err != nil {
			rec.obs(map[string]any{"src": "runner", "event": "cdc-target", "error": err.Error()})
		} else {
			caught := consumer.CatchUp(context.Background(), target, plan.CatchUpDeadline)
			rec.marker("cdc-caught-up", map[string]any{"caught_up": caught, "target_lsn": int64(target)})
		}
		stopConsumer()
	}
	stopAll()
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
func runClient(wctx context.Context, c map[string]any, rec *recorder, start, deadline time.Time, pacing, opTimeout time.Duration) error {
	ci, err := workload.Int(c["client"])
	if err != nil {
		return fmt.Errorf("plan client: %w", err)
	}
	wl, _ := c["wl"].(string)
	gw, _ := c["gw"].(string)
	iso, _ := c["iso"].(string)
	ops, _ := c["ops"].([]any)
	role, _ := c["role"].(string)
	s := &workload.Session{GW: gw, AppName: fmt.Sprintf("commitproof-c%d", ci), Timeout: opTimeout, User: role, Password: role}
	// K5: a client's own pacing (D3's W4 at 2×, ADR-0030).
	if v, ok := c["pacing_ms"]; ok {
		ms, err := workload.Int(v)
		if err != nil {
			return fmt.Errorf("plan client %d: %w", ci, err)
		}
		pacing = time.Duration(ms) * time.Millisecond
	}
	// C7 options (plan.Client): session statement_timeout (P19) and a
	// CancelRequest after COMMIT.
	if v, ok := c["statement_timeout_ms"]; ok {
		if s.StatementTimeoutMS, err = workload.Int(v); err != nil {
			return fmt.Errorf("plan client %d: %w", ci, err)
		}
	}
	if v, ok := c["cancel_commit_after_ms"]; ok {
		ms, err := workload.Int(v)
		if err != nil {
			return fmt.Errorf("plan client %d: %w", ci, err)
		}
		s.CancelCommitAfter = time.Duration(ms) * time.Millisecond
	}
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
		if !begun.Before(deadline) || wctx.Err() != nil {
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
			if f == "append" {
				out = s.AppendOne(ctx, mops) // D3's autocommit-append variant
			} else {
				out = s.Txn(ctx, iso, mops)
			}
		case plan.WLRegister:
			m, _ := val.(map[string]any)
			out = s.Register(ctx, f, m)
		case plan.WLBank:
			m, _ := val.(map[string]any)
			out = s.Bank(ctx, f, m)
		case plan.WLLedger:
			m, _ := val.(map[string]any)
			if role != "" {
				out = s.Steady(ctx, m) // W8's cp_steady
			} else {
				out = s.Ledger(ctx, m)
			}
		case plan.WLNotify:
			m, _ := val.(map[string]any)
			out = s.Notify(ctx, plan.Channel, m)
		case plan.WLProbe:
			m, _ := val.(map[string]any)
			sql, _ := m["sql"].(string)
			out = s.Probe(ctx, sql)
			if out.Type == workload.OK {
				out.Value = map[string]any{"id": m["id"], "sql": sql, "result": out.Value}
			}
		default:
			return fmt.Errorf("client %d: unknown workload %q", ci, wl)
		}
		if out.Attempt.CancelRequest != "" {
			rec.obs(map[string]any{"src": "client", "event": "cancel-request", "op": i, "result": out.Attempt.CancelRequest,
				"completion": out.Type})
		}
		rec.complete(i, out, txIso)
		if out.Type == workload.Info {
			proc = rec.nextProc.Add(1) - 1 // Jepsen: the client continues as a new process
		}
	}
	return nil
}

// gatewayOps are the operations of the runner's own client of one gateway
// after the workload (*workload.Session; a fake in tests).
type gatewayOps interface {
	Ledger(ctx context.Context, v map[string]any) workload.Outcome
	FinalRead(ctx context.Context, wl string) workload.Outcome
}

// recoverGateway retries a ledger insert through gateway g until one is ok
// or P06 expires. *proc is the client's process; an info completion retires
// it (§6.1, Jepsen) and *proc becomes a fresh one, which the final reads
// continue on.
func recoverGateway(rec *recorder, g int, s gatewayOps, proc *int64) bool {
	gw := plan.Gateways[g]
	client := finalClient(g)
	end := time.Now().Add(params.RecoveryDeadline)
	for attempt := 0; time.Now().Before(end); attempt++ {
		v := map[string]any{"id": recoveryID(g, attempt), "payload": fmt.Sprintf("recovery-%s-%d", gw, attempt)}
		i, err := rec.invoke(*proc, client, gw, plan.WLLedger, "insert", v, "")
		if err != nil {
			return false
		}
		out := s.Ledger(context.Background(), v)
		rec.complete(i, out, "")
		switch out.Type {
		case workload.OK:
			return true
		case workload.Info:
			*proc = rec.nextProc.Add(1) - 1
		}
		time.Sleep(time.Second)
	}
	return false
}

// finalReads reads every workload's full state through gateway g every P08
// interval until P08's repeat count of consecutive identical rounds, within
// P08's deadline, on the client's process *proc (retired by an info
// completion, as in recoverGateway).
func finalReads(rec *recorder, g int, s gatewayOps, proc *int64) bool {
	gw := plan.Gateways[g]
	client := finalClient(g)
	end := time.Now().Add(params.FinalReadDeadline)
	var prev []byte
	consecutive := 0
	for round := 0; time.Now().Before(end); round++ {
		var vals []any
		okRound := true
		for _, wl := range []string{plan.WLAppend, plan.WLRegister, plan.WLBank, plan.WLLedger} {
			i, err := rec.invoke(*proc, client, gw, wl, "final-read", map[string]any{}, "")
			if err != nil {
				return false
			}
			out := s.FinalRead(context.Background(), wl)
			rec.complete(i, out, "")
			if out.Type != workload.OK {
				okRound = false
				if out.Type == workload.Info {
					*proc = rec.nextProc.Add(1) - 1
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
