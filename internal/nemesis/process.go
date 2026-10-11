package nemesis

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/netprobe"
	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/slowfs"
)

// Node-side paths and commands (deploy/compose.yaml, ADR-0006).
const (
	pgData        = "/data/pg_data"
	pgSockets     = "/data/pg_sockets"
	slowfsControl = "/run/slowfs/control"
	nodeUser      = "999:999"
)

// psqlArgv runs one statement on the node's PostgreSQL through its Unix
// socket, as the postgres role (pg_hba: local trust), bounded by `timeout`.
func psqlArgv(seconds int, sql string) []string {
	return []string{"timeout", strconv.Itoa(seconds), "psql", "-h", pgSockets, "-p", "5432", "-U", "postgres", "-d", "postgres",
		"-X", "-A", "-t", "-v", "ON_ERROR_STOP=1", "-c", sql}
}

// selectOne reports whether `select 1` succeeds on the node within 5 s.
func (e *Env) selectOne(ctx context.Context, name, node string) (Probe, bool) {
	p := e.exec(ctx, name, Container("pg-"+node), psqlArgv(5, "select 1")...)
	return p, p.OK() && strings.TrimSpace(string(p.Res.Stdout)) == "1"
}

func (e *Env) pgprocs(ctx context.Context, name, node string, pids ...int) (Probe, slowfs.PGProcs, error) {
	argv := []string{"slowfs", "pgprocs", "--pgdata", pgData}
	for _, p := range pids {
		argv = append(argv, "--pid", strconv.Itoa(p))
	}
	pr := e.exec(ctx, name, Container("pg-"+node), argv...)
	var out slowfs.PGProcs
	if !pr.OK() {
		return pr, out, fmt.Errorf("pgprocs exit %d", pr.Res.ExitCode)
	}
	return pr, out, lastJSON(pr, &out)
}

func alive(ps slowfs.PGProcs, pid int) bool {
	for _, p := range ps.Postgres {
		if p.PID == pid && p.State != "Z" && p.State != "X" {
			return true
		}
	}
	return false
}

// killPostmaster is F2a: kill -9 of the postmaster. Its heal is PostgreSQL
// restarting by itself; if it has not within P28, the observation is
// recorded and PostgreSQL is started through pgctld (§8).
type killPostmaster struct {
	e    *Env
	id   string
	node string
	pid  int
}

func (k *killPostmaster) prepare(ctx context.Context) ([]Probe, map[string]any, error) {
	pr, ps, err := k.e.pgprocs(ctx, k.id+"-prepare-pgprocs", k.node)
	if err != nil {
		return []Probe{pr}, nil, err
	}
	k.pid = ps.PostmasterPID
	if k.pid == 0 || !alive(ps, k.pid) {
		return []Probe{pr}, nil, fmt.Errorf("no live postmaster (postmaster.pid %d)", k.pid)
	}
	return []Probe{pr}, map[string]any{"postmaster_pid": int64(k.pid)}, nil
}

func (k *killPostmaster) inject(ctx context.Context) []Probe {
	return []Probe{k.e.exec(ctx, k.id+"-inject", Container("pg-"+k.node), "kill", "-9", strconv.Itoa(k.pid))}
}

// confirm: the postmaster PID is absent from /proc.
func (k *killPostmaster) confirm(ctx context.Context) (bool, []Probe, map[string]any) {
	pr, ps, err := k.e.pgprocs(ctx, k.id+"-confirm-pgprocs", k.node, k.pid)
	st := ps.Asked[strconv.Itoa(k.pid)]
	return err == nil && st == "absent", []Probe{pr}, map[string]any{"postmaster_pid": int64(k.pid), "state": st}
}

func (k *killPostmaster) heal(ctx context.Context) []Probe { return nil }

// healed: a new postmaster PID exists and accepts select 1.
func (k *killPostmaster) healed(ctx context.Context) (bool, []Probe, map[string]any) {
	pr, ps, err := k.e.pgprocs(ctx, k.id+"-healed-pgprocs", k.node)
	det := map[string]any{"old_postmaster_pid": int64(k.pid), "postmaster_pid": int64(ps.PostmasterPID)}
	if err != nil || ps.PostmasterPID == 0 || ps.PostmasterPID == k.pid || !alive(ps, ps.PostmasterPID) {
		return false, []Probe{pr}, det
	}
	sp, ok := k.e.selectOne(ctx, k.id+"-healed-select", k.node)
	return ok, []Probe{pr, sp}, det
}

// manualHeal starts PostgreSQL through pgctld, as the node user.
func (k *killPostmaster) manualHeal(ctx context.Context) []Probe {
	return []Probe{k.e.docker(ctx, k.id+"-manual-start", cmdTimeout+params.HealConfirmDeadline, "exec", "-u", nodeUser,
		Container("pg-"+k.node), "pgctld", "start", "--pooler-dir=/data", "--pg-port=5432")}
}

// pausePostgres is F3a: SIGSTOP of every postgres process of the node.
type pausePostgres struct {
	e        *Env
	id       string
	node     string
	pm       int          // postmaster
	targeted map[int]bool // every postgres PID signalled
}

func (f *pausePostgres) list() []int {
	var out []int
	for p := range f.targeted {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

func (f *pausePostgres) prepare(ctx context.Context) ([]Probe, map[string]any, error) {
	pr, ps, err := f.e.pgprocs(ctx, f.id+"-prepare-pgprocs", f.node)
	if err != nil {
		return []Probe{pr}, nil, err
	}
	if ps.PostmasterPID == 0 || !alive(ps, ps.PostmasterPID) {
		return []Probe{pr}, nil, fmt.Errorf("no live postmaster")
	}
	f.pm = ps.PostmasterPID
	f.targeted = map[int]bool{f.pm: true}
	for _, p := range ps.Postgres {
		if p.State != "Z" {
			f.targeted[p.PID] = true
		}
	}
	return []Probe{pr}, map[string]any{"postmaster_pid": int64(f.pm), "postgres_pids": intList(f.list())}, nil
}

// inject stops the postmaster first (it then forks no new backend), then
// every postgres process not yet stopped, for up to five rounds.
func (f *pausePostgres) inject(ctx context.Context) []Probe {
	c := Container("pg-" + f.node)
	probes := []Probe{f.e.exec(ctx, f.id+"-inject-stop-postmaster", c, "kill", "-STOP", strconv.Itoa(f.pm))}
	stop := without(itoa(f.list()), strconv.Itoa(f.pm))
	for round := 0; round < 5 && len(stop) > 0; round++ {
		probes = append(probes, f.e.exec(ctx, fmt.Sprintf("%s-inject-stop-%d", f.id, round), c, append([]string{"kill", "-STOP"}, stop...)...))
		pr, ps, err := f.e.pgprocs(ctx, fmt.Sprintf("%s-inject-pgprocs-%d", f.id, round), f.node)
		probes = append(probes, pr)
		if err != nil {
			break
		}
		stop = stop[:0]
		for _, p := range ps.Postgres {
			if p.State != "T" && p.State != "Z" {
				stop = append(stop, strconv.Itoa(p.PID))
				f.targeted[p.PID] = true
			}
		}
	}
	return probes
}

// confirm: the postmaster and every postgres process of the node are in
// state T (a process that exited before it could be stopped is gone, not
// running), and `select 1` on the node times out after 3 s (timeout exits
// 124).
func (f *pausePostgres) confirm(ctx context.Context) (bool, []Probe, map[string]any) {
	pr, ps, err := f.e.pgprocs(ctx, f.id+"-confirm-pgprocs", f.node, f.list()...)
	sp := f.e.exec(ctx, f.id+"-confirm-select", Container("pg-"+f.node), psqlArgv(3, "select 1")...)
	ok := err == nil && ps.Asked[strconv.Itoa(f.pm)] == "T"
	running := []any{}
	for _, p := range ps.Postgres {
		if p.State != "T" && p.State != "Z" {
			running = append(running, int64(p.PID))
			ok = false
		}
	}
	timedOut := sp.Res.Err == nil && sp.Res.ExitCode == 124
	return ok && timedOut, []Probe{pr, sp}, map[string]any{"postmaster_pid": int64(f.pm), "states": anyMap(ps.Asked),
		"not_stopped": running, "select_exit_code": int64(sp.Res.ExitCode)}
}

func (f *pausePostgres) heal(ctx context.Context) []Probe {
	return []Probe{f.e.exec(ctx, f.id+"-heal", Container("pg-"+f.node), append([]string{"kill", "-CONT"}, itoa(f.list())...)...)}
}

// healed: no postgres process of the node is in state T and select 1
// succeeds.
func (f *pausePostgres) healed(ctx context.Context) (bool, []Probe, map[string]any) {
	pr, ps, err := f.e.pgprocs(ctx, f.id+"-healed-pgprocs", f.node, f.list()...)
	ok := err == nil
	stopped := []any{}
	for _, p := range ps.Postgres {
		if p.State == "T" {
			stopped = append(stopped, int64(p.PID))
			ok = false
		}
	}
	sp, sok := f.e.selectOne(ctx, f.id+"-healed-select", f.node)
	return ok && sok, []Probe{pr, sp}, map[string]any{"states": anyMap(ps.Asked), "stopped": stopped}
}

func anyMap(m map[string]string) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func intList(xs []int) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = int64(x)
	}
	return out
}

func itoa(xs []int) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = strconv.Itoa(x)
	}
	return out
}

// slowDisk is F4a: the node's slowfs delay set to P17's 200 ms.
type slowDisk struct {
	e    *Env
	id   string
	node string
}

func (s *slowDisk) probe(ctx context.Context, tag string) (Probe, int64, error) {
	pr := s.e.exec(ctx, s.id+"-"+tag+"-io", Container("pg-"+s.node), "slowfs", "probe", "--dir", "/data", "--size", "4096", "--count", "5")
	var out slowfs.IOProbe
	if !pr.OK() {
		return pr, 0, fmt.Errorf("io probe exit %d", pr.Res.ExitCode)
	}
	if err := lastJSON(pr, &out); err != nil {
		return pr, 0, err
	}
	if len(out.SamplesUS) != 5 {
		return pr, 0, fmt.Errorf("io probe returned %d samples", len(out.SamplesUS))
	}
	return pr, slowfs.Median(out.SamplesUS), nil
}

// prepare: the baseline median must be below P17's 50 ms.
func (s *slowDisk) prepare(ctx context.Context) ([]Probe, map[string]any, error) {
	pr, m, err := s.probe(ctx, "baseline")
	det := map[string]any{"baseline_median_us": m, "baseline_limit_us": params.SlowfsBaselineMedian.Microseconds()}
	if err == nil && m >= params.SlowfsBaselineMedian.Microseconds() {
		err = fmt.Errorf("baseline median %d µs ≥ %d µs", m, params.SlowfsBaselineMedian.Microseconds())
	}
	return []Probe{pr}, det, err
}

func (s *slowDisk) set(ctx context.Context, tag string, ms int64) []Probe {
	return []Probe{s.e.exec(ctx, s.id+"-"+tag, Container("pg-"+s.node), "slowfs", "control", "--file", slowfsControl,
		"--set-ms", strconv.FormatInt(ms, 10))}
}

func (s *slowDisk) inject(ctx context.Context) []Probe {
	return s.set(ctx, "inject", params.SlowfsDelay.Milliseconds())
}
func (s *slowDisk) heal(ctx context.Context) []Probe { return s.set(ctx, "heal", 0) }

// confirm: median of 5 write+fsync of 4 KiB ≥ 150 ms (P17).
func (s *slowDisk) confirm(ctx context.Context) (bool, []Probe, map[string]any) {
	pr, m, err := s.probe(ctx, "confirm")
	return err == nil && m >= params.SlowfsConfirmMedian.Microseconds(), []Probe{pr},
		map[string]any{"median_us": m, "threshold_us": params.SlowfsConfirmMedian.Microseconds()}
}

// healed: median < 50 ms (P17).
func (s *slowDisk) healed(ctx context.Context) (bool, []Probe, map[string]any) {
	pr, m, err := s.probe(ctx, "healed")
	return err == nil && m < params.SlowfsBaselineMedian.Microseconds(), []Probe{pr},
		map[string]any{"median_us": m, "limit_us": params.SlowfsBaselineMedian.Microseconds()}
}

// readiness is how a restarted container's readiness is probed (§8 F2b–F2e).
type readiness struct {
	helperNS string // namespace whose helper probes (localhost)
	url      string // HTTP /ready (status 200), or
	connect  string // a TCP port that must accept, or
	etcd     bool   // etcdctl endpoint health
}

// killContainer is F2b–F2e: docker kill -s KILL, then docker start.
type killContainer struct {
	e       *Env
	id      string
	svc     string
	ready   readiness
	restart bool // the helper shares the killed container's namespace: restart it after start
}

func (k *killContainer) prepare(ctx context.Context) ([]Probe, map[string]any, error) {
	pr, st, err := k.e.inspectState(ctx, k.id+"-prepare-inspect", Container(k.svc))
	if err == nil && !st.Running {
		err = fmt.Errorf("%s is not running (%s)", k.svc, st.Status)
	}
	return []Probe{pr}, nil, err
}

func (k *killContainer) inject(ctx context.Context) []Probe {
	return []Probe{k.e.docker(ctx, k.id+"-inject", cmdTimeout, "kill", "-s", "KILL", Container(k.svc))}
}

// confirm: the container exited with code 137.
func (k *killContainer) confirm(ctx context.Context) (bool, []Probe, map[string]any) {
	pr, st, err := k.e.inspectState(ctx, k.id+"-confirm-inspect", Container(k.svc))
	return err == nil && st.Status == "exited" && st.ExitCode == 137, []Probe{pr},
		map[string]any{"status": st.Status, "exit_code": int64(st.ExitCode)}
}

func (k *killContainer) heal(ctx context.Context) []Probe {
	ps := []Probe{k.e.docker(ctx, k.id+"-heal-start", cmdTimeout, "start", Container(k.svc))}
	if k.restart {
		// The helper still holds the killed container's old namespace.
		ps = append(ps, k.e.docker(ctx, k.id+"-heal-helper", cmdTimeout+params.ClientOpTimeout, "restart", "-t", "1", Helper(k.ready.helperNS)))
	}
	return ps
}

// healed: the container runs and its readiness probe passes.
func (k *killContainer) healed(ctx context.Context) (bool, []Probe, map[string]any) {
	pr, st, err := k.e.inspectState(ctx, k.id+"-healed-inspect", Container(k.svc))
	probes := []Probe{pr}
	det := map[string]any{"status": st.Status}
	if err != nil || !st.Running {
		return false, probes, det
	}
	rp, ok := k.e.ready(ctx, k.id+"-healed-ready", k.ready)
	return ok, append(probes, rp), det
}

func (e *Env) ready(ctx context.Context, name string, r readiness) (Probe, bool) {
	switch {
	case r.etcd:
		p := e.exec(ctx, name, Container("etcd"), "etcdctl", "--endpoints=http://127.0.0.1:2379", "endpoint", "health")
		return p, p.OK() && strings.Contains(string(p.Res.Stdout)+string(p.Res.Stderr), "is healthy")
	case r.url != "":
		p := e.exec(ctx, name, Helper(r.helperNS), "cpnet", "http", "--timeout", "2s", r.url)
		var h netprobe.HTTP
		return p, p.OK() && lastJSON(p, &h) == nil && h.Status == 200
	default:
		p := e.exec(ctx, name, Helper(r.helperNS), "cpnet", "connect", "--timeout", connectTimeout.String(), r.connect)
		var c netprobe.Connect
		return p, p.OK() && lastJSON(p, &c) == nil && c.Result(r.connect).AllOK()
	}
}

// pauseContainer is F3b: docker pause / unpause (cgroup freezer).
type pauseContainer struct {
	e   *Env
	id  string
	svc string
}

func (f *pauseContainer) prepare(ctx context.Context) ([]Probe, map[string]any, error) {
	pr, st, err := f.e.inspectState(ctx, f.id+"-prepare-inspect", Container(f.svc))
	if err == nil && (!st.Running || st.Paused) {
		err = fmt.Errorf("%s is %s (paused %v)", f.svc, st.Status, st.Paused)
	}
	return []Probe{pr}, nil, err
}

func (f *pauseContainer) inject(ctx context.Context) []Probe {
	return []Probe{f.e.docker(ctx, f.id+"-inject", cmdTimeout, "pause", Container(f.svc))}
}

func (f *pauseContainer) heal(ctx context.Context) []Probe {
	return []Probe{f.e.docker(ctx, f.id+"-heal", cmdTimeout, "unpause", Container(f.svc))}
}

// confirm: State.Paused = true.
func (f *pauseContainer) confirm(ctx context.Context) (bool, []Probe, map[string]any) {
	pr, st, err := f.e.inspectState(ctx, f.id+"-confirm-inspect", Container(f.svc))
	return err == nil && st.Paused, []Probe{pr}, map[string]any{"paused": st.Paused, "status": st.Status}
}

// healed: State.Paused = false and the container runs.
func (f *pauseContainer) healed(ctx context.Context) (bool, []Probe, map[string]any) {
	pr, st, err := f.e.inspectState(ctx, f.id+"-healed-inspect", Container(f.svc))
	return err == nil && !st.Paused && st.Running, []Probe{pr}, map[string]any{"paused": st.Paused, "status": st.Status}
}

// switchover is F6: docker kill -s TERM of the leader's pooler (graceful
// drain), docker start after the plan's duration.
type switchover struct {
	e      *Env
	id     string
	leader string
	rule   *ruleObs
	nodes  []string
}

func (s *switchover) prepare(ctx context.Context) ([]Probe, map[string]any, error) {
	return nil, map[string]any{"leader": s.leader, "rule": s.rule.key()}, nil
}

func (s *switchover) inject(ctx context.Context) []Probe {
	return []Probe{s.e.docker(ctx, s.id+"-inject", cmdTimeout, "kill", "-s", "TERM", Container("pooler-"+s.leader))}
}

// confirm: another pooler reports itself leader of a decided rule whose
// number is greater than the rule before the fault.
func (s *switchover) confirm(ctx context.Context) (bool, []Probe, map[string]any) {
	pr, sts, err := s.e.statusProbe(ctx, s.id+"-confirm-status", s.nodes)
	det := map[string]any{"old_leader": s.leader, "old_rule": s.rule.key()}
	if err != nil {
		det["error"] = err.Error()
		return false, []Probe{pr}, det
	}
	for _, st := range sts {
		if st.Node == s.leader || !st.OK || st.Decision == nil {
			continue
		}
		if st.Decision.Leader == st.Name && st.Name == st.Node && st.Decision.after(s.rule) {
			det["new_leader"], det["new_rule"] = st.Node, st.Decision.key()
			return true, []Probe{pr}, det
		}
	}
	return false, []Probe{pr}, det
}

func (s *switchover) heal(ctx context.Context) []Probe {
	return []Probe{s.e.docker(ctx, s.id+"-heal-start", cmdTimeout, "start", Container("pooler-"+s.leader))}
}

// healed: the pooler container runs and /ready on 16000 answers 200.
func (s *switchover) healed(ctx context.Context) (bool, []Probe, map[string]any) {
	pr, st, err := s.e.inspectState(ctx, s.id+"-healed-inspect", Container("pooler-"+s.leader))
	if err != nil || !st.Running {
		return false, []Probe{pr}, map[string]any{"status": st.Status}
	}
	rp, ok := s.e.ready(ctx, s.id+"-healed-ready", readiness{helperNS: s.leader, url: "http://127.0.0.1:16000/ready"})
	return ok, []Probe{pr, rp}, map[string]any{"status": st.Status}
}

// membership is F5a (add n4) and F5b (remove n4).
type membership struct {
	e   *Env
	id  string
	add bool
}

var standbyToken = regexp.MustCompile(`[A-Za-z0-9_]+`)

func names(standbyNames, app string) bool {
	for _, t := range standbyToken.FindAllString(standbyNames, -1) {
		if t == app {
			return true
		}
	}
	return false
}

func (m *membership) prepare(ctx context.Context) ([]Probe, map[string]any, error) {
	var probes []Probe
	for _, svc := range []string{"pg-n4", "pooler-n4", "nem-pg-n4"} {
		pr, st, err := m.e.inspectState(ctx, m.id+"-prepare-inspect-"+svc, Container(svc))
		probes = append(probes, pr)
		switch {
		case err != nil:
			return probes, nil, fmt.Errorf("%s: %w", svc, err)
		case m.add && st.Running:
			return probes, nil, fmt.Errorf("%s already running", svc)
		case !m.add && !st.Running && svc != "nem-pg-n4":
			return probes, nil, fmt.Errorf("%s is not running", svc)
		}
	}
	return probes, map[string]any{"application_name": AppName("n4")}, nil
}

// inject: F5a starts n4's containers (created by the host at run start,
// ADR-0022); F5b stops pooler-n4 gracefully (docker stop -t 30: SHUTDOWN →
// tombstone, S24), then pg-n4, then its helper.
func (m *membership) inject(ctx context.Context) []Probe {
	var ps []Probe
	if m.add {
		for _, svc := range []string{"pg-n4", "pooler-n4", "nem-pg-n4"} {
			ps = append(ps, m.e.docker(ctx, m.id+"-inject-start-"+svc, cmdTimeout, "start", Container(svc)))
		}
		if m.e.NodeStarted != nil {
			m.e.NodeStarted("n4")
		}
		return ps
	}
	ps = append(ps, m.e.docker(ctx, m.id+"-inject-stop-pooler-n4", cmdTimeout+30*time.Second, "stop", "-t", "30", Container("pooler-n4")))
	for _, svc := range []string{"pg-n4", "nem-pg-n4"} {
		ps = append(ps, m.e.docker(ctx, m.id+"-inject-stop-"+svc, cmdTimeout+params.ClientOpTimeout, "stop", Container(svc)))
	}
	return ps
}

// confirm: on the leader, synchronous_standby_names names n4's application
// name and pg_stat_replication has its row (F5a); the names no longer name
// it (F5b). Deadline 120 s (§8).
func (m *membership) confirm(ctx context.Context) (bool, []Probe, map[string]any) {
	v, err := m.e.observeView(ctx, m.id+"-confirm", true)
	probes := v.probes
	det := map[string]any{}
	if err != nil {
		det["error"] = err.Error()
		return false, probes, det
	}
	det["leader"] = v.leader
	c := Container("pg-" + v.leader)
	sn := m.e.exec(ctx, m.id+"-confirm-standby-names", c, psqlArgv(5, "select current_setting('synchronous_standby_names')")...)
	probes = append(probes, sn)
	if !sn.OK() {
		return false, probes, det
	}
	names4 := names(string(sn.Res.Stdout), AppName("n4"))
	det["standby_names"] = strings.TrimSpace(string(sn.Res.Stdout))
	if !m.add {
		return !names4, probes, det
	}
	rp := m.e.exec(ctx, m.id+"-confirm-replication", c, psqlArgv(5, "select application_name, state, sync_state from pg_stat_replication order by 1")...)
	probes = append(probes, rp)
	replicates := false
	for _, l := range strings.Split(string(rp.Res.Stdout), "\n") {
		if f := strings.Split(strings.TrimSpace(l), "|"); len(f) == 3 && f[0] == AppName("n4") {
			replicates = true
			det["replication_state"], det["sync_state"] = f[1], f[2]
		}
	}
	return rp.OK() && names4 && replicates, probes, det
}

func (m *membership) heal(ctx context.Context) []Probe { return nil }
func (m *membership) healed(ctx context.Context) (bool, []Probe, map[string]any) {
	return true, nil, nil
}
