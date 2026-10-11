// Package observe reads Multigres internals for the observation stream and
// the HEALTHY predicate (CONTRACT §5.1, §5.3, ADR-0016). It is the only
// package that imports Multigres code; checkers and gates never import it.
// An observation never turns a FAIL into a PASS.
package observe

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	clustermetadatapb "github.com/multigres/multigres/go/pb/clustermetadata"
	multipoolermanagerpb "github.com/multigres/multigres/go/pb/multipoolermanager"
	multipoolermanagerdatapb "github.com/multigres/multigres/go/pb/multipoolermanagerdata"
)

// PoolerGRPCPort is multipooler --grpc-port (S03).
const PoolerGRPCPort = 16100

// Rule is a decided (or proposed) shard rule as reported by one pooler.
type Rule struct {
	CoordinatorTerm int64
	LeaderSubterm   int64
	Leader          string
	Cohort          []string // sorted pooler names
	Policy          string
}

// Key identifies the rule number.
func (r *Rule) Key() string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d", r.CoordinatorTerm, r.LeaderSubterm)
}

func (r *Rule) toMap() any {
	if r == nil {
		return nil
	}
	cohort := make([]any, len(r.Cohort))
	for i, c := range r.Cohort {
		cohort[i] = c
	}
	return map[string]any{
		"coordinator_term": r.CoordinatorTerm, "leader_subterm": r.LeaderSubterm,
		"leader": r.Leader, "cohort": cohort, "policy": r.Policy,
	}
}

// PoolerStatus is the observed state of one pooler.
type PoolerStatus struct {
	Node            string
	OK              bool
	Error           string
	Name            string // pooler's own ID name
	PoolerType      string
	PostgresStatus  string
	PostgresRunning bool
	PostgresReady   bool
	Initialized     bool
	WALPosition     string
	Decision        *Rule
	Proposal        *Rule
	LatencyMS       int64
}

// Map is the canonical-JSON-ready observation payload.
func (p PoolerStatus) Map() map[string]any {
	m := map[string]any{"src": "pooler-status", "node": p.Node, "ok": p.OK, "latency_ms": p.LatencyMS}
	if !p.OK {
		m["error"] = p.Error
		return m
	}
	m["name"] = p.Name
	m["pooler_type"] = p.PoolerType
	m["postgres_status"] = p.PostgresStatus
	m["postgres_running"] = p.PostgresRunning
	m["postgres_ready"] = p.PostgresReady
	m["initialized"] = p.Initialized
	m["wal_position"] = p.WALPosition
	m["decision"] = p.Decision.toMap()
	m["proposal"] = p.Proposal.toMap()
	return m
}

func rule(r *clustermetadatapb.ShardRule) *Rule {
	if r == nil || r.GetRuleNumber() == nil {
		return nil
	}
	out := &Rule{
		CoordinatorTerm: r.GetRuleNumber().GetCoordinatorTerm(),
		LeaderSubterm:   r.GetRuleNumber().GetLeaderSubterm(),
		Leader:          r.GetLeaderId().GetName(),
		Policy:          r.GetDurabilityPolicy().String(),
	}
	for _, m := range r.GetCohortMembers() {
		out.Cohort = append(out.Cohort, m.GetName())
	}
	sort.Strings(out.Cohort)
	return out
}

// Client queries poolers' MultipoolerManager.Status RPC. Connections are
// cached per address.
type Client struct {
	conns map[string]*grpc.ClientConn
}

// NewClient returns an empty client.
func NewClient() *Client { return &Client{conns: map[string]*grpc.ClientConn{}} }

// Close closes cached connections.
func (c *Client) Close() {
	for _, cc := range c.conns {
		cc.Close()
	}
}

// Status calls Status on node (host pooler-<node>) with timeout.
func (c *Client) Status(ctx context.Context, node string, timeout time.Duration) PoolerStatus {
	ps := PoolerStatus{Node: node}
	addr := fmt.Sprintf("pooler-%s:%d", node, PoolerGRPCPort)
	cc, ok := c.conns[addr]
	if !ok {
		var err error
		cc, err = grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			ps.Error = err.Error()
			return ps
		}
		c.conns[addr] = cc
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	resp, err := multipoolermanagerpb.NewMultipoolerManagerClient(cc).Status(cctx, &multipoolermanagerdatapb.StatusRequest{})
	ps.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		ps.Error = err.Error()
		return ps
	}
	ps.OK = true
	st := resp.GetStatus()
	cs := resp.GetConsensusStatus()
	ps.Name = cs.GetId().GetName()
	ps.PoolerType = st.GetPoolerType().String()
	ps.PostgresStatus = st.GetPostgresStatus().String()
	ps.PostgresRunning = st.GetPostgresRunning()
	ps.PostgresReady = st.GetPostgresReady()
	ps.Initialized = st.GetIsInitialized()
	ps.WALPosition = st.GetWalPosition()
	pos := cs.GetCurrentPosition().GetPosition()
	ps.Decision = rule(pos.GetDecision())
	ps.Proposal = rule(pos.GetProposal())
	return ps
}

// Consensus evaluates parts (a) and (b) of the HEALTHY predicate over the
// statuses of n1–n3: (a) exactly one pooler reports itself leader of a
// decided rule and every one of n1–n3 reports that same decided rule;
// (b) the rule's cohort is exactly {n1, n2, n3}. It returns the agreed rule
// (nil if none) and a reason for each failed part.
func Consensus(statuses []PoolerStatus, cohort []string) (*Rule, string, string) {
	want := append([]string(nil), cohort...)
	sort.Strings(want)
	var leaders []PoolerStatus
	for _, s := range statuses {
		if s.OK && s.Decision != nil && s.Decision.Leader == s.Name && s.Name == s.Node {
			leaders = append(leaders, s)
		}
	}
	if len(leaders) != 1 {
		names := []string{}
		for _, l := range leaders {
			names = append(names, l.Node)
		}
		return nil, fmt.Sprintf("%d poolers report themselves leader of their decided rule %v", len(leaders), names), "no agreed rule"
	}
	r := leaders[0].Decision
	for _, s := range statuses {
		switch {
		case !s.OK:
			return nil, fmt.Sprintf("%s: status RPC failed: %s", s.Node, s.Error), "no agreed rule"
		case s.Decision == nil:
			return nil, fmt.Sprintf("%s: no decided rule", s.Node), "no agreed rule"
		case s.Decision.Key() != r.Key() || s.Decision.Leader != r.Leader:
			return nil, fmt.Sprintf("%s: decided rule %s leader %s, leader %s reports %s", s.Node, s.Decision.Key(), s.Decision.Leader, r.Leader, r.Key()), "no agreed rule"
		}
	}
	if strings.Join(r.Cohort, ",") != strings.Join(want, ",") {
		return r, "", fmt.Sprintf("cohort %v, want %v", r.Cohort, want)
	}
	return r, "", ""
}

// StatusOnce calls Status on node over a fresh connection and closes it;
// safe for concurrent use (the runner's status probe, CONTRACT §8).
func StatusOnce(ctx context.Context, node string, timeout time.Duration) PoolerStatus {
	c := NewClient()
	defer c.Close()
	return c.Status(ctx, node, timeout)
}
