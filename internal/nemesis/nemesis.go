package nemesis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/plan"
)

// Fault phases of CONTRACT §6.1.
const (
	PhaseInject     = "inject"
	PhaseConfirmed  = "confirmed"
	PhaseHeal       = "heal"
	PhaseHealed     = "healed"
	PhaseNotApplied = "not-applied"
	PhaseNotHealed  = "not-healed"
)

// fault is one element of a step: prepare reads what the predicates compare
// against (baselines, PIDs, the rule before the fault); inject and heal run
// the commands of §8; confirm and healed evaluate one round of the
// predicates on fresh probes.
type fault interface {
	prepare(ctx context.Context) ([]Probe, map[string]any, error)
	inject(ctx context.Context) []Probe
	confirm(ctx context.Context) (bool, []Probe, map[string]any)
	heal(ctx context.Context) []Probe
	healed(ctx context.Context) (bool, []Probe, map[string]any)
}

// manualHealer is F2a: if the self-heal window passes, a manual start.
type manualHealer interface {
	manualHeal(ctx context.Context) []Probe
}

// active is an injected, confirmed element awaiting its heal.
type active struct {
	id      string
	kind    string
	targets []string
	f       fault
}

// Outcome of the schedule.
type Outcome struct {
	OK     bool
	Reason string // why the run is INVALID (not-applied / not-healed / harness)
	// Faults counts elements confirmed.
	Faults int
}

func (e *Env) emit(id, kind, phase string, targets []string, ev map[string]any) error {
	if ev == nil {
		ev = map[string]any{}
	}
	return e.Emit(map[string]any{"id": id, "kind": kind, "phase": phase, "targets": strList(targets), "evidence": ev})
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// StartHelper starts nem-runner in the runner's own network namespace (the
// runner is a cpnet address like any other; §5.1 lists a helper for it).
func (e *Env) StartHelper(ctx context.Context) error {
	p := e.docker(ctx, "nem-runner-start", cmdTimeout, "run", "--detach", "--name", NemRunner,
		"--label", "com.docker.compose.project="+project, "--label", "com.docker.compose.service=nem-runner",
		"--network", "container:"+RunnerContainer, "--cap-drop", "ALL", "--cap-add", "NET_ADMIN", e.NemImage, "idle")
	if !p.OK() {
		return fmt.Errorf("start %s: exit %d: %s", NemRunner, p.Res.ExitCode, strings.TrimSpace(string(p.Res.Stderr)))
	}
	_, st, err := e.inspectState(ctx, "nem-runner-inspect", NemRunner)
	if err != nil || !st.Running {
		return fmt.Errorf("%s not running after start (%v)", NemRunner, err)
	}
	return nil
}

// StopHelper removes nem-runner.
func (e *Env) StopHelper(ctx context.Context) {
	e.docker(ctx, "nem-runner-remove", cmdTimeout, "rm", "--force", NemRunner)
}

// Run executes the nemesis schedule (CONTRACT §6.3, §8, P05). start is the
// workload start. It returns when every step is healed and heal-confirmed,
// or at the first not-applied / not-healed (the run is then INVALID).
func (e *Env) Run(ctx context.Context, steps []plan.Step, start time.Time) Outcome {
	out := Outcome{OK: true}
	lastHeal := start
	for _, st := range steps {
		anchor := start
		if st.After == plan.AfterPreviousHeal {
			anchor = lastHeal
		}
		if !sleepCtx(ctx, time.Until(anchor.Add(time.Duration(st.DelayMS)*time.Millisecond))) {
			return Outcome{Reason: "nemesis interrupted"}
		}
		e.Marker("nemesis-step", map[string]any{"step": st.Index, "elements": int64(len(st.Elements))})
		var act []active
		for k, el := range st.Elements {
			a, reason := e.apply(ctx, plan.FaultID(st, k), el)
			if reason != "" {
				return Outcome{Reason: reason, Faults: out.Faults}
			}
			out.Faults++
			act = append(act, a)
		}
		if !sleepCtx(ctx, time.Duration(st.DurationMS)*time.Millisecond) {
			return Outcome{Reason: "nemesis interrupted", Faults: out.Faults}
		}
		for i := len(act) - 1; i >= 0; i-- {
			if reason := e.healOne(ctx, act[i]); reason != "" {
				return Outcome{Reason: reason, Faults: out.Faults}
			}
		}
		lastHeal = time.Now()
	}
	return out
}

// build resolves an element's roles and returns its fault.
func (e *Env) build(ctx context.Context, id string, el plan.Element) (fault, []string, map[string]any, error) {
	v, err := e.observeView(ctx, id+"-resolve", needsLeader(el))
	if err != nil {
		return nil, nil, v.evidence(el, nil), fmt.Errorf("resolve: %w", err)
	}
	first := func() string {
		if len(el.Targets) != 1 {
			return ""
		}
		return el.Targets[0]
	}()
	resolved := map[string]any{}
	var f fault
	var targets []string
	fail := func(err error) (fault, []string, map[string]any, error) {
		return nil, nil, v.evidence(el, resolved), fmt.Errorf("resolve: %w", err)
	}
	running := RunningNamespaces(v.running)
	switch el.Kind {
	case "F1a":
		t, err := v.netns(first, el.Pick)
		if err != nil {
			return fail(err)
		}
		targets, f = []string{t}, isolate(e, id, t, running)
	case "F1b":
		var a, b []string
		for _, r := range el.A {
			n, err := v.netns(r, el.Pick)
			if err != nil {
				return fail(err)
			}
			a = append(a, n)
		}
		for _, r := range el.B {
			if r == "rest" {
				b = append(b, without(running, a...)...)
				continue
			}
			n, err := v.netns(r, el.Pick)
			if err != nil {
				return fail(err)
			}
			b = append(b, n)
		}
		b = without(b, a...)
		if len(a) == 0 || len(b) == 0 {
			return fail(fmt.Errorf("empty side: a %v b %v", a, b))
		}
		resolved["a"], resolved["b"] = strList(a), strList(b)
		targets, f = append(append([]string{}, a...), b...), split(e, id, a, b)
	case "F1c", "F1d":
		l, err := v.node(first, el.Pick)
		if err != nil {
			return fail(err)
		}
		targets = []string{l}
		if el.Kind == "F1c" {
			f = asymmetric(e, id, l, running)
		} else {
			fs := without(v.nodes, l)
			if len(fs) == 0 {
				return fail(fmt.Errorf("no follower"))
			}
			resolved["followers"] = strList(fs)
			f = oneWay(e, id, l, fs)
		}
	case "F7":
		t, err := v.netns(first, el.Pick)
		if err != nil {
			return fail(err)
		}
		d := newDelay(e, id, t)
		resolved["prober"] = d.prober
		targets, f = []string{t}, d
	case "F2a", "F3a", "F4a":
		n, err := v.node(first, el.Pick)
		if err != nil {
			return fail(err)
		}
		targets = []string{n}
		switch el.Kind {
		case "F2a":
			f = &killPostmaster{e: e, id: id, node: n}
		case "F3a":
			f = &pausePostgres{e: e, id: id, node: n}
		default:
			f = &slowDisk{e: e, id: id, node: n}
		}
	case "F2b", "F6":
		n, err := v.node(first, el.Pick)
		if err != nil {
			return fail(err)
		}
		targets = []string{"pooler-" + n}
		if el.Kind == "F2b" {
			f = &killContainer{e: e, id: id, svc: "pooler-" + n, ready: readiness{helperNS: n, url: "http://127.0.0.1:16000/ready"}}
		} else {
			if !v.leaderOK || n != v.leader {
				return fail(fmt.Errorf("F6 targets the leader; %s is not (leader %q)", n, v.leader))
			}
			f = &switchover{e: e, id: id, leader: n, rule: v.rule, nodes: v.nodes}
		}
	case "F2c", "F2d", "F2e", "F3b":
		svc, err := v.service(first, el.Pick)
		if err != nil {
			return fail(err)
		}
		targets = []string{svc}
		switch el.Kind {
		case "F2c":
			f = &killContainer{e: e, id: id, svc: svc, restart: true, ready: readiness{helperNS: svc, url: "http://127.0.0.1:17000/ready"}}
		case "F2d":
			f = &killContainer{e: e, id: id, svc: svc, restart: true, ready: readiness{helperNS: svc, connect: "127.0.0.1:15432"}}
		case "F2e":
			f = &killContainer{e: e, id: id, svc: svc, restart: true, ready: readiness{helperNS: "etcd", etcd: true}}
		default:
			f = &pauseContainer{e: e, id: id, svc: svc}
		}
	case "F5a", "F5b":
		if first != "n4" {
			return fail(fmt.Errorf("%s targets n4, plan says %v", el.Kind, el.Targets))
		}
		targets, f = []string{"n4"}, &membership{e: e, id: id, add: el.Kind == "F5a"}
	default:
		return fail(fmt.Errorf("unknown fault kind %q", el.Kind))
	}
	resolved["targets"] = strList(targets)
	return f, targets, v.evidence(el, resolved), nil
}

// apply resolves, prepares, injects and confirms one element. It returns
// the reason the run is INVALID, if it is.
func (e *Env) apply(ctx context.Context, id string, el plan.Element) (active, string) {
	// Resolution and preparation are retried until P27: a role may be
	// momentarily unresolvable (a failover in progress).
	t0 := time.Now()
	var (
		f        fault
		targets  []string
		res      map[string]any
		prep     []Probe
		prepDet  map[string]any
		err      error
		attempts int
	)
	for {
		attempts++
		f, targets, res, err = e.build(ctx, id, el)
		if err == nil {
			prep, prepDet, err = f.prepare(ctx)
		}
		if err == nil || time.Since(t0) > params.FaultConfirmDeadline || ctx.Err() != nil {
			break
		}
		sleepCtx(ctx, time.Second)
	}
	ev := map[string]any{"resolution": res, "prepare": evidenceList(prep), "prepare_details": prepDet,
		"prepare_attempts": int64(attempts)}
	if err != nil {
		ev["error"] = err.Error()
		e.emit(id, el.Kind, PhaseInject, targets, ev)
		e.emit(id, el.Kind, PhaseNotApplied, targets, map[string]any{"reason": "could not prepare within P27: " + err.Error()})
		return active{}, fmt.Sprintf("fault %s (%s) not-applied: %v", id, el.Kind, err)
	}
	if noop := injectNoop(el.Kind); noop != "" {
		ev["injection"] = "skipped: " + noop
	}
	if err := e.emit(id, el.Kind, PhaseInject, targets, ev); err != nil {
		return active{}, "history: " + err.Error()
	}
	var inj []Probe
	if injectNoop(el.Kind) == "" {
		inj = f.inject(ctx)
	}
	deadline := plan.ConfirmDeadline(el.Kind)
	returned := time.Now()
	for round := 1; ; round++ {
		ok, probes, det := f.confirm(ctx)
		elapsed := time.Since(returned)
		cev := map[string]any{"inject": evidenceList(inj), "probes": evidenceList(probes), "details": det,
			"rounds": int64(round), "elapsed_ms": elapsed.Milliseconds(), "deadline_ms": deadline.Milliseconds()}
		if ok && elapsed <= deadline {
			e.emit(id, el.Kind, PhaseConfirmed, targets, cev)
			return active{id: id, kind: el.Kind, targets: targets, f: f}, ""
		}
		if elapsed > deadline || ctx.Err() != nil {
			e.emit(id, el.Kind, PhaseNotApplied, targets, cev)
			return active{}, fmt.Sprintf("fault %s (%s) not-applied: confirmation predicate did not hold within %s", id, el.Kind, deadline)
		}
		sleepCtx(ctx, pollInterval)
	}
}

// healOne heals one element and waits for its heal predicate (P28). F5a and
// F5b have no heal. F2a's heal is PostgreSQL restarting by itself; if it has
// not within P28, the observation is recorded, PostgreSQL is started through
// pgctld, and the predicate gets another P28 (§8).
func (e *Env) healOne(ctx context.Context, a active) string {
	if !plan.Healable(a.kind) {
		return ""
	}
	action := "heal commands"
	if a.kind == "F2a" {
		action = "none: wait for PostgreSQL to restart by itself"
	}
	hev := map[string]any{"action": action}
	if noop := healNoop(a.kind); noop != "" {
		hev["heal"] = "skipped: " + noop
	}
	e.emit(a.id, a.kind, PhaseHeal, a.targets, hev)
	var cmds []Probe
	if healNoop(a.kind) == "" {
		cmds = a.f.heal(ctx)
	}
	ok, probes, det, rounds, elapsed := e.waitHealed(ctx, a.f)
	ev := map[string]any{"heal": evidenceList(cmds), "probes": evidenceList(probes), "details": det,
		"rounds": int64(rounds), "elapsed_ms": elapsed.Milliseconds(), "deadline_ms": params.HealConfirmDeadline.Milliseconds()}
	if mh, isManual := a.f.(manualHealer); isManual {
		ev["restarted_by_itself"] = ok
		if !ok && healNoop(a.kind) == "" {
			e.Marker("observation", map[string]any{"fault": a.id, "note": "PostgreSQL did not restart by itself within P28; starting it through pgctld"})
			manual := mh.manualHeal(ctx)
			ok, probes, det, rounds, elapsed = e.waitHealed(ctx, a.f)
			ev["manual_start"], ev["probes"], ev["details"] = evidenceList(manual), evidenceList(probes), det
			ev["manual_rounds"], ev["manual_elapsed_ms"] = int64(rounds), elapsed.Milliseconds()
		}
	}
	if ok {
		e.emit(a.id, a.kind, PhaseHealed, a.targets, ev)
		return ""
	}
	e.emit(a.id, a.kind, PhaseNotHealed, a.targets, ev)
	return fmt.Sprintf("fault %s (%s) not-healed: heal predicate did not hold within %s", a.id, a.kind, params.HealConfirmDeadline)
}

func (e *Env) waitHealed(ctx context.Context, f fault) (bool, []Probe, map[string]any, int, time.Duration) {
	start := time.Now()
	for round := 1; ; round++ {
		ok, probes, det := f.healed(ctx)
		elapsed := time.Since(start)
		if ok && elapsed <= params.HealConfirmDeadline {
			return true, probes, det, round, elapsed
		}
		if elapsed > params.HealConfirmDeadline || ctx.Err() != nil {
			return false, probes, det, round, elapsed
		}
		sleepCtx(ctx, pollInterval)
	}
}
