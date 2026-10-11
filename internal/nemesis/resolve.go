package nemesis

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/plan"
)

// ruleObs is a decided rule as reported by a pooler's Status RPC.
type ruleObs struct {
	CoordinatorTerm int64    `json:"coordinator_term"`
	LeaderSubterm   int64    `json:"leader_subterm"`
	Leader          string   `json:"leader"`
	Cohort          []string `json:"cohort"`
}

// after reports whether r's rule number is greater than o's.
func (r *ruleObs) after(o *ruleObs) bool {
	if r.CoordinatorTerm != o.CoordinatorTerm {
		return r.CoordinatorTerm > o.CoordinatorTerm
	}
	return r.LeaderSubterm > o.LeaderSubterm
}

func (r *ruleObs) key() string { return fmt.Sprintf("%d.%d", r.CoordinatorTerm, r.LeaderSubterm) }

// statusObs is one pooler's status (`commitproof __probe status`).
type statusObs struct {
	Node     string   `json:"node"`
	OK       bool     `json:"ok"`
	Error    string   `json:"error"`
	Name     string   `json:"name"`
	Decision *ruleObs `json:"decision"`
}

// statusProbe runs the runner's status probe on nodes.
func (e *Env) statusProbe(ctx context.Context, name string, nodes []string) (Probe, []statusObs, error) {
	p := e.run(ctx, name, statusTimeout+cmdTimeout, e.Exe, "__probe", "status",
		"--nodes", strings.Join(nodes, ","), fmt.Sprintf("--timeout-ms=%d", statusTimeout.Milliseconds()))
	var out struct {
		Statuses []statusObs `json:"statuses"`
	}
	if err := lastJSON(p, &out); err != nil {
		return p, nil, err
	}
	return p, out.Statuses, nil
}

// leaderOf returns the single pooler that reports itself leader of its
// decided rule. Zero or several self-reported leaders is an error: roles
// are resolved only on an unambiguous view.
func leaderOf(sts []statusObs) (string, *ruleObs, error) {
	var leaders []string
	var rule *ruleObs
	for _, s := range sts {
		if s.OK && s.Decision != nil && s.Decision.Leader == s.Name && s.Name == s.Node {
			leaders = append(leaders, s.Node)
			rule = s.Decision
		}
	}
	if len(leaders) != 1 {
		return "", nil, fmt.Errorf("%d poolers report themselves leader %v", len(leaders), leaders)
	}
	return leaders[0], rule, nil
}

// view is the state roles are resolved against.
type view struct {
	running  map[string]bool // running containers
	nodes    []string        // running nodes
	leader   string
	rule     *ruleObs
	probes   []Probe
	leaderOK bool
}

// observeView lists running containers and, if needLeader, the leader.
func (e *Env) observeView(ctx context.Context, id string, needLeader bool) (view, error) {
	v := view{}
	p, running := e.runningContainers(ctx, id+"-ps")
	v.probes = append(v.probes, p)
	if !p.OK() {
		return v, fmt.Errorf("docker ps exit %d", p.Res.ExitCode)
	}
	v.running = running
	for _, n := range Nodes {
		if running[Container("pg-"+n)] {
			v.nodes = append(v.nodes, n)
		}
	}
	if !needLeader {
		return v, nil
	}
	sp, sts, err := e.statusProbe(ctx, id+"-status", v.nodes)
	v.probes = append(v.probes, sp)
	if err != nil {
		return v, err
	}
	v.leader, v.rule, err = leaderOf(sts)
	v.leaderOK = err == nil
	return v, err
}

// followers are the leader's cohort minus the leader, running, sorted.
func (v view) followers() []string {
	var out []string
	if v.rule == nil {
		return out
	}
	for _, c := range v.rule.Cohort {
		if c != v.leader && v.running[Container("pg-"+c)] {
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

func pickOf(xs []string, pick int64) string {
	s := append([]string{}, xs...)
	sort.Strings(s)
	return s[int(pick%int64(len(s)))]
}

// node resolves a node role.
func (v view) node(role string, pick int64) (string, error) {
	switch {
	case role == "leader":
		if !v.leaderOK {
			return "", fmt.Errorf("leader unresolved")
		}
		return v.leader, nil
	case role == "random-follower":
		fs := v.followers()
		if len(fs) == 0 {
			return "", fmt.Errorf("no running follower")
		}
		return pickOf(fs, pick), nil
	case IsNode(role):
		if !v.running[Container("pg-"+role)] {
			return "", fmt.Errorf("node %s is not running", role)
		}
		return role, nil
	}
	return "", fmt.Errorf("%q is not a node role", role)
}

// netns resolves a network-namespace role.
func (v view) netns(role string, pick int64) (string, error) {
	switch role {
	case "random-orch":
		return pickOf([]string{"orch-z1", "orch-z2", "orch-z3"}, pick), nil
	case "random-gw":
		return pickOf([]string{"gw-z1", "gw-z2", "gw-z3"}, pick), nil
	}
	if n, err := v.node(role, pick); err == nil {
		return n, nil
	} else if role == "leader" || role == "random-follower" || IsNode(role) {
		return "", err
	}
	if _, ok := LookupNS(role); !ok {
		return "", fmt.Errorf("unknown namespace %q", role)
	}
	if role != "runner" && !v.running[OwnerContainer(role)] {
		return "", fmt.Errorf("%s is not running", role)
	}
	return role, nil
}

// service resolves a container role to a Compose service name.
func (v view) service(role string, pick int64) (string, error) {
	if pfx, nr, ok := strings.Cut(role, ":"); ok && (pfx == "pooler" || pfx == "pg") {
		n, err := v.node(nr, pick)
		if err != nil {
			return "", err
		}
		return pfx + "-" + n, nil
	}
	switch role {
	case "random-orch", "random-gw":
		return v.netns(role, pick)
	case "etcd", "orch-z1", "orch-z2", "orch-z3", "gw-z1", "gw-z2", "gw-z3":
		return role, nil
	}
	return "", fmt.Errorf("%q is not a container role", role)
}

// needsLeader reports whether resolving e needs the leader.
func needsLeader(e plan.Element) bool {
	switch e.Kind {
	case "F1c", "F1d", "F6":
		return true
	}
	for _, r := range append(append(append([]string{}, e.Targets...), e.A...), e.B...) {
		_, nr, _ := strings.Cut(r, ":")
		if r == "leader" || r == "random-follower" || nr == "leader" || nr == "random-follower" {
			return true
		}
	}
	return false
}

// resolveEvidence records a resolution (§6.3: roles resolved at run time,
// the resolution recorded).
func (v view) evidence(e plan.Element, resolved map[string]any) map[string]any {
	m := map[string]any{"roles": strList(e.Targets), "pick": e.Pick, "running_nodes": strList(v.nodes),
		"resolved": resolved, "probes": evidenceList(v.probes)}
	if e.Kind == "F1b" {
		m["roles_a"], m["roles_b"] = strList(e.A), strList(e.B)
	}
	if v.leaderOK {
		m["leader"] = v.leader
		m["rule"] = v.rule.key()
		m["cohort"] = strList(v.rule.Cohort)
	}
	return m
}

func strList(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
