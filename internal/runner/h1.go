package runner

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Profile h1 (CONTRACT §5.5, §9.4; ADR-0025): Multigres at the historical
// commit has neither the rule model nor multigres.version(), and its pooler
// API differs from the pinned commit's protos. Its HEALTHY predicate and
// leader view therefore read PostgreSQL itself, in each node container:
//
//	(a') exactly one of n1–n3 is out of recovery (the primary);
//	(b') the primary's synchronous_standby_names names exactly the other two
//	     nodes and its pg_stat_replication shows both streaming;
//	(c)  unchanged: a write and a read-back through every gateway;
//	(d') every Multigres container runs commitproof/multigres:h1 and every
//	     node commitproof/pgctld:h1 (the images built from the historical
//	     commit, locked by `commitproof images build --profile h1`; cluster up
//	     checked their IDs against the lock).
const ProfileH1 = "h1"

// H1Cell is the only cell of the historical demo (CM:20).
const H1Cell = "zone1"

// NodeSQL is one node's PostgreSQL state as read through psql in its
// container: recovery state, synchronous_standby_names and the walsenders.
type NodeSQL struct {
	Node        string
	OK          bool
	Error       string
	InRecovery  bool
	StandbyName string
	Replication map[string]string // application_name → state
	LSN         string
	Raw         string
}

// Map renders the state for observations and the predicate output.
func (n NodeSQL) Map() map[string]any {
	rep := map[string]any{}
	for k, v := range n.Replication {
		rep[k] = v
	}
	return map[string]any{"node": n.Node, "ok": n.OK, "error": n.Error, "in_recovery": n.InRecovery,
		"synchronous_standby_names": n.StandbyName, "replication": rep, "lsn": n.LSN, "raw": n.Raw}
}

// h1Query reports recovery, standby names, walsenders and the current LSN.
const h1Query = "select pg_is_in_recovery(), current_setting('synchronous_standby_names'), " +
	"coalesce((select string_agg(application_name || '=' || state, ',' order by application_name) from pg_stat_replication), ''), " +
	"case when pg_is_in_recovery() then pg_last_wal_replay_lsn()::text else pg_current_wal_lsn()::text end"

// QueryNodeSQL runs h1Query in node's container (docker exec, psql over the
// node's Unix socket), bounded by P01.
func QueryNodeSQL(ctx context.Context, node string) NodeSQL {
	n := NodeSQL{Node: node, Replication: map[string]string{}}
	ctx, cancel := context.WithTimeout(ctx, params.ClientOpTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "exec", "commitproof-pg-"+node+"-1", "timeout", "5", "psql", "-h", "/data/pg_sockets",
		"-p", "5432", "-U", "postgres", "-d", "postgres", "-X", "-A", "-t", "-F", "|", "-v", "ON_ERROR_STOP=1", "-c", h1Query).CombinedOutput()
	n.Raw = strings.TrimSpace(string(out))
	if err != nil {
		n.Error = fmt.Sprintf("%v: %s", err, truncate(n.Raw, 256))
		return n
	}
	f := strings.Split(n.Raw, "|")
	if len(f) != 4 {
		n.Error = "unexpected psql output"
		return n
	}
	n.OK = true
	n.InRecovery = f[0] == "t"
	n.StandbyName = f[1]
	for _, kv := range strings.Split(f[2], ",") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			n.Replication[k] = v
		}
	}
	n.LSN = f[3]
	return n
}

var standbyTok = regexp.MustCompile(`[A-Za-z0-9_]+`)

// standbyApps extracts the application names of synchronous_standby_names
// ("ANY 1 (\"zone1_n2\", \"zone1_n3\")" → zone1_n2, zone1_n3).
func standbyApps(v string) []string {
	var out []string
	for _, t := range standbyTok.FindAllString(v, -1) {
		if strings.HasPrefix(t, H1Cell+"_") {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// H1Leader returns the node out of recovery when exactly one is.
func H1Leader(sts []NodeSQL) (string, error) {
	var prim []string
	for _, s := range sts {
		if s.OK && !s.InRecovery {
			prim = append(prim, s.Node)
		}
	}
	if len(prim) != 1 {
		return "", fmt.Errorf("nodes out of recovery: %v (want exactly one)", prim)
	}
	return prim[0], nil
}

// h1Consensus evaluates (a') and (b').
func h1Consensus(sts []NodeSQL) (leader, a, b string) {
	leader, err := H1Leader(sts)
	if err != nil {
		return "", err.Error(), "no primary"
	}
	var want []string
	for _, n := range Nodes {
		if n != leader {
			want = append(want, H1Cell+"_"+n)
		}
	}
	for _, s := range sts {
		if s.Node != leader {
			continue
		}
		if got := standbyApps(s.StandbyName); strings.Join(got, ",") != strings.Join(want, ",") {
			return leader, "", fmt.Sprintf("primary %s synchronous_standby_names %q, want %v", leader, s.StandbyName, want)
		}
		for _, w := range want {
			if s.Replication[w] != "streaming" {
				return leader, "", fmt.Sprintf("primary %s: %s is %q in pg_stat_replication", leader, w, s.Replication[w])
			}
		}
	}
	return leader, "", ""
}

// h1Images evaluates (d').
func h1Images(ctx context.Context) string {
	want := map[string]string{}
	for _, n := range Nodes {
		want["pooler-"+n] = "commitproof/multigres:h1"
		want["pg-"+n] = "commitproof/pgctld:h1"
	}
	for _, s := range []string{"orch-z1", "gw-z1", "gw-z2", "gw-z3"} {
		want[s] = "commitproof/multigres:h1"
	}
	var bad []string
	svcs := make([]string, 0, len(want))
	for s := range want {
		svcs = append(svcs, s)
	}
	sort.Strings(svcs)
	for _, s := range svcs {
		cctx, cancel := context.WithTimeout(ctx, params.ClientOpTimeout)
		out, err := exec.CommandContext(cctx, "docker", "inspect", "--format", "{{.Config.Image}}", "commitproof-"+s+"-1").Output()
		cancel()
		if got := strings.TrimSpace(string(out)); err != nil || got != want[s] {
			bad = append(bad, fmt.Sprintf("%s runs %q, want %s", s, got, want[s]))
		}
	}
	return strings.Join(bad, "; ")
}

// checkHealthyH1 is CheckHealthy for profile h1.
func checkHealthyH1(ctx context.Context, token int64) Health {
	h := Health{C: map[string]string{}}
	var sts []NodeSQL
	for _, n := range Nodes {
		sts = append(sts, QueryNodeSQL(ctx, n))
	}
	leader := ""
	leader, h.A, h.B = h1Consensus(sts)
	h.SQL = sts
	h.Version = "commit " + params.H1Commit + " (image identity; no multigres.version() at that commit)"
	checkGateways(ctx, &h, token)
	h.D = h1Images(ctx)
	if leader != "" {
		h.Leader = leader
	}
	h.Healthy = h.A == "" && h.B == "" && h.D == ""
	for _, r := range h.C {
		h.Healthy = h.Healthy && r == ""
	}
	return h
}

// pollSQL is the h1 observation poller (P24): one QueryNodeSQL per node and
// interval (diagnosis only, ADR-0016).
func pollSQL(ctx context.Context, node string, emit func(map[string]any)) {
	t := time.NewTicker(params.ObservationInterval)
	defer t.Stop()
	for {
		v := QueryNodeSQL(ctx, node).Map()
		v["src"] = "node-sql"
		emit(v)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
