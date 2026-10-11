// Package runner is the code that runs inside the runner container
// (ADR-0018): the HEALTHY predicate, the workload execution that writes
// history.jsonl and observations.jsonl, and the cluster init job.
package runner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CesarManzoCode/commitproof/internal/observe"
	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/plan"
	"github.com/CesarManzoCode/commitproof/internal/workload"
)

// PinnedCommit is the Multigres source commit (CONTRACT §5.2).
const PinnedCommit = "e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73"

// Nodes started at cluster up; the expected cohort (§5.1 (b)).
var Nodes = []string{"n1", "n2", "n3"}

// Health is one evaluation of the HEALTHY predicate (CONTRACT §5.1).
type Health struct {
	Healthy  bool
	A, B     string            // consensus parts; "" = satisfied
	C        map[string]string // per gateway write/read-back; "" = satisfied
	D        string            // multigres.version(); "" = satisfied
	Version  string
	Rule     *observe.Rule
	Statuses []observe.PoolerStatus
	// Profile h1 (h1.go): the node-level view and the primary.
	SQL    []NodeSQL
	Leader string
}

// Map renders the evaluation for JSON output and observations.
func (h Health) Map() map[string]any {
	c := map[string]any{}
	for gw, r := range h.C {
		c[gw] = r
	}
	sts := make([]any, len(h.Statuses))
	for i, s := range h.Statuses {
		sts[i] = s.Map()
	}
	var rule any
	if h.Rule != nil {
		cohort := make([]any, len(h.Rule.Cohort))
		for i, x := range h.Rule.Cohort {
			cohort[i] = x
		}
		rule = map[string]any{"coordinator_term": h.Rule.CoordinatorTerm, "leader_subterm": h.Rule.LeaderSubterm,
			"leader": h.Rule.Leader, "cohort": cohort, "policy": h.Rule.Policy}
	}
	m := map[string]any{
		"healthy": h.Healthy, "a": h.A, "b": h.B, "c": c, "d": h.D,
		"version": h.Version, "rule": rule, "statuses": sts,
	}
	if h.SQL != nil {
		sq := make([]any, len(h.SQL))
		for i, s := range h.SQL {
			sq[i] = s.Map()
		}
		m["sql"], m["leader"] = sq, h.Leader
	}
	return m
}

func dsn(gw string) string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable connect_timeout=%d",
		gw, workload.GatewayPort, workload.DBUser, workload.DBPassword, workload.DBName, int(params.ClientOpTimeout/time.Second))
}

// withConn runs f on a fresh connection to gw under the P01 deadline.
func withConn(ctx context.Context, gw string, f func(ctx context.Context, c *pgx.Conn) error) error {
	ctx, cancel := context.WithTimeout(ctx, params.ClientOpTimeout)
	defer cancel()
	cfg, err := pgx.ParseConfig(dsn(gw))
	if err != nil {
		return err
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeExec
	cfg.RuntimeParams["application_name"] = "commitproof-healthy"
	c, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connect %s: %w", gw, err)
	}
	defer c.Close(context.Background())
	return f(ctx, c)
}

// CheckHealthy evaluates the HEALTHY predicate once (profile h1: h1.go).
// token makes each write distinguishable from earlier ones.
func CheckHealthy(ctx context.Context, oc *observe.Client, token int64, profile string) Health {
	if profile == ProfileH1 {
		return checkHealthyH1(ctx, token)
	}
	h := Health{C: map[string]string{}}
	for _, n := range Nodes {
		h.Statuses = append(h.Statuses, oc.Status(ctx, n, params.ClientOpTimeout))
	}
	h.Rule, h.A, h.B = observe.Consensus(h.Statuses, Nodes)
	checkGateways(ctx, &h, token)
	// (d): SELECT multigres.version() through gw-z1 reports the pinned commit.
	err := withConn(ctx, plan.Gateways[0], func(ctx context.Context, c *pgx.Conn) error {
		return c.QueryRow(ctx, "select multigres.version()").Scan(&h.Version)
	})
	switch {
	case err != nil:
		h.D = "select multigres.version(): " + err.Error()
	case !strings.Contains(h.Version, PinnedCommit[:12]):
		h.D = fmt.Sprintf("version %q does not name commit %s", h.Version, PinnedCommit[:12])
	}
	h.Healthy = h.A == "" && h.B == "" && h.D == ""
	for _, r := range h.C {
		h.Healthy = h.Healthy && r == ""
	}
	return h
}

// checkGateways is part (c): a write and a read-back through every gateway.
// The table lives outside the workload schema cp.
func checkGateways(ctx context.Context, h *Health, token int64) {
	err := withConn(ctx, plan.Gateways[0], func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "create table if not exists public.cp_health (gw text primary key, token bigint not null)")
		return err
	})
	for _, gw := range plan.Gateways {
		if err != nil {
			h.C[gw] = "create table through gw-z1: " + err.Error()
			continue
		}
		e := withConn(ctx, gw, func(ctx context.Context, c *pgx.Conn) error {
			if _, err := c.Exec(ctx, "insert into public.cp_health (gw, token) values ($1, $2) on conflict (gw) do update set token = excluded.token", gw, token); err != nil {
				return fmt.Errorf("write: %w", err)
			}
			var got int64
			if err := c.QueryRow(ctx, "select token from public.cp_health where gw = $1", gw).Scan(&got); err != nil {
				return fmt.Errorf("read-back: %w", err)
			}
			if got != token {
				return fmt.Errorf("read-back returned %d, wrote %d", got, token)
			}
			return nil
		})
		if e != nil {
			h.C[gw] = e.Error()
		} else {
			h.C[gw] = ""
		}
	}
}

// WaitHealthy evaluates the predicate every second until it holds or the
// deadline passes. It never restarts anything (CONTRACT §15 K1 rejection:
// no retries of cluster up inside a measured attempt).
func WaitHealthy(ctx context.Context, deadline time.Duration, profile string, onEval func(Health)) (Health, int) {
	oc := observe.NewClient()
	defer oc.Close()
	end := time.Now().Add(deadline)
	n := 0
	for {
		n++
		h := CheckHealthy(ctx, oc, time.Now().UnixNano(), profile)
		if onEval != nil {
			onEval(h)
		}
		if h.Healthy || time.Now().After(end) || ctx.Err() != nil {
			return h, n
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}
