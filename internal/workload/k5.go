package workload

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// K5 workloads W5–W8 (CONTRACT §7; ADR-0030). Every statement here is sent
// as written: SQL-level session statements use the simple protocol, as a
// psql client would; protocol-level named statements use Parse/Bind/Execute.

// SetupK5 returns the setup statements a campaign adds (ADR-0030): W7's
// publication and W8's login roles and grants.
func SetupK5(publication string, roles []string) []string {
	var out []string
	if publication != "" {
		out = append(out, fmt.Sprintf("create publication %s for table cp.ledger, cp.append", publication))
	}
	for _, r := range roles {
		out = append(out, fmt.Sprintf("create role %s login password '%s'", r, r))
		if r == "cp_steady" {
			out = append(out, "grant usage on schema cp to cp_steady", "grant insert, select on cp.ledger to cp_steady")
		}
	}
	return out
}

// GUCSet runs `SET <guc> = '<value>'` (W5).
func (s *Session) GUCSet(ctx context.Context, guc, value string) Outcome {
	return s.Do(ctx, true, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		a.At = PhaseStatement
		if _, err := c.Exec(ctx, fmt.Sprintf("SET %s = '%s'", guc, value), pgx.QueryExecModeSimpleProtocol); err != nil {
			return nil, err
		}
		a.StatementExecuted = true
		return nil, nil
	})
}

// GUCShow runs `SHOW <guc>` and returns its value (W5).
func (s *Session) GUCShow(ctx context.Context, guc string) Outcome {
	return s.Do(ctx, true, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		a.At = PhaseStatement
		var v string
		if err := c.QueryRow(ctx, "SHOW "+guc, pgx.QueryExecModeSimpleProtocol).Scan(&v); err != nil {
			return nil, err
		}
		a.StatementExecuted = true
		return v, nil
	})
}

// StatementSQL is the text of a W5 named statement's query.
func StatementSQL(value int64) string { return fmt.Sprintf("SELECT %d::bigint", value) }

// Prepare creates a W5 named statement: kind "sql" runs `PREPARE <name> AS
// SELECT <v>::bigint`, kind "protocol" sends a Parse of that query named
// <name>.
func (s *Session) Prepare(ctx context.Context, name, kind string, value int64) Outcome {
	return s.Do(ctx, true, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		a.At = PhaseStatement
		switch kind {
		case "sql":
			if _, err := c.Exec(ctx, fmt.Sprintf("PREPARE %s AS %s", name, StatementSQL(value)), pgx.QueryExecModeSimpleProtocol); err != nil {
				return nil, err
			}
		case "protocol":
			if _, err := c.PgConn().Prepare(ctx, name, StatementSQL(value), nil); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unknown statement kind %q", kind)
		}
		a.StatementExecuted = true
		return nil, nil
	})
}

// Execute runs a W5 named statement and returns its row ([value]).
func (s *Session) Execute(ctx context.Context, name, kind string) Outcome {
	return s.Do(ctx, true, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		a.At = PhaseStatement
		var v int64
		switch kind {
		case "sql":
			if err := c.QueryRow(ctx, "EXECUTE "+name, pgx.QueryExecModeSimpleProtocol).Scan(&v); err != nil {
				return nil, err
			}
		case "protocol":
			rr := c.PgConn().ExecPrepared(ctx, name, nil, nil, nil).Read()
			if rr.Err != nil {
				return nil, rr.Err
			}
			if len(rr.Rows) != 1 || len(rr.Rows[0]) != 1 {
				return nil, fmt.Errorf("EXECUTE %s returned %d rows", name, len(rr.Rows))
			}
			n, err := strconv.ParseInt(string(rr.Rows[0][0]), 10, 64)
			if err != nil {
				return nil, err
			}
			v = n
		default:
			return nil, fmt.Errorf("unknown statement kind %q", kind)
		}
		a.StatementExecuted = true
		return []any{v}, nil
	})
}

// Notify runs W6's notifier transaction: a ledger insert and `NOTIFY
// cp_chan, '<id>'`, committed together.
func (s *Session) Notify(ctx context.Context, channel string, v map[string]any) Outcome {
	return s.Do(ctx, false, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		id, err := Int(v["id"])
		if err != nil {
			return nil, err
		}
		payload, _ := v["payload"].(string)
		a.At = PhaseBegin
		tx, err := c.Begin(ctx)
		if err != nil {
			return nil, err
		}
		a.At, a.WriteSent = PhaseStatement, true
		if _, err := tx.Exec(ctx, "insert into cp.ledger (id, payload) values ($1, $2)", id, payload); err != nil {
			s.rollback(tx)
			return nil, err
		}
		a.WriteExecuted, a.StatementExecuted = true, true
		if _, err := tx.Exec(ctx, fmt.Sprintf("NOTIFY %s, '%d'", channel, id)); err != nil {
			s.rollback(tx)
			return nil, err
		}
		a.At = PhaseCommit
		return map[string]any{"id": id, "payload": payload}, tx.Commit(ctx)
	})
}

// Steady runs a W8 cp_steady insert (a W4 insert) and returns the server
// identity: the executing PostgreSQL's postmaster start time (ADR-0030).
func (s *Session) Steady(ctx context.Context, v map[string]any) Outcome {
	return s.Do(ctx, true, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		id, err := Int(v["id"])
		if err != nil {
			return nil, err
		}
		payload, _ := v["payload"].(string)
		a.At, a.WriteSent = PhaseStatement, true
		var server string
		if err := c.QueryRow(ctx, "insert into cp.ledger (id, payload) values ($1, $2) returning pg_postmaster_start_time()::text", id, payload).Scan(&server); err != nil {
			return nil, err
		}
		return map[string]any{"id": id, "payload": payload, "role": v["role"], "server": server}, nil
	})
}

// Storm runs one W8 cp_storm statement.
func (s *Session) Storm(ctx context.Context) Outcome {
	return s.Do(ctx, true, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		a.At = PhaseStatement
		if _, err := c.Exec(ctx, "select pg_sleep(0.5)"); err != nil {
			return nil, err
		}
		a.StatementExecuted = true
		return nil, nil
	})
}

// AppendOne runs D3's autocommit-append variant of W1 (CONTRACT §9.2): one
// micro-op ["append", k, v] as a single autocommit statement.
func (s *Session) AppendOne(ctx context.Context, mops []any) Outcome {
	return s.Do(ctx, true, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		if len(mops) != 1 {
			return nil, fmt.Errorf("autocommit append of %d micro-ops", len(mops))
		}
		mo, ok := mops[0].([]any)
		if !ok || len(mo) != 3 || mo[0] != "append" {
			return nil, fmt.Errorf("bad micro-op %v", mops[0])
		}
		k, err1 := Int(mo[1])
		v, err2 := Int(mo[2])
		if err := errors.Join(err1, err2); err != nil {
			return nil, err
		}
		a.At, a.WriteSent = PhaseStatement, true
		if _, err := c.Exec(ctx, "insert into cp.append (k, vals) values ($1, array[$2::bigint]) on conflict (k) do update set vals = cp.append.vals || excluded.vals", k, v); err != nil {
			return nil, err
		}
		return []any{[]any{"append", k, v}}, nil
	})
}

// Listener is a W6 listener connection (CONTRACT §7): LISTEN, then waits
// that collect the notifications delivered meanwhile.
type Listener struct {
	GW      string
	Timeout time.Duration
	Port    int

	conn    *pgx.Conn
	mu      sync.Mutex
	pending []string
}

// Connected reports whether the listener holds an open connection.
func (l *Listener) Connected() bool { return l.conn != nil && !l.conn.IsClosed() }

// Close drops the connection.
func (l *Listener) Close() {
	if l.conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		l.conn.Close(ctx)
		cancel()
		l.conn = nil
	}
}

// Listen connects and runs `LISTEN <channel>`.
func (l *Listener) Listen(ctx context.Context, channel string) Outcome {
	l.Close()
	s := &Session{GW: l.GW, Timeout: l.Timeout, Port: l.Port, AppName: "commitproof-listener"}
	ctx, cancel := context.WithTimeout(ctx, l.Timeout)
	defer cancel()
	a := Attempt{Autocommit: true, At: PhaseConnect}
	cfg, err := s.config()
	if err == nil {
		cfg.OnNotification = func(_ *pgconn.PgConn, n *pgconn.Notification) {
			l.mu.Lock()
			l.pending = append(l.pending, n.Payload)
			l.mu.Unlock()
		}
		l.conn, err = pgx.ConnectConfig(ctx, cfg)
	}
	if err == nil {
		a.At = PhaseStatement
		if _, err = l.conn.Exec(ctx, "LISTEN "+channel, pgx.QueryExecModeSimpleProtocol); err == nil {
			a.StatementExecuted = true
		}
	}
	if err != nil {
		fill(&a, err, ctx)
		l.Close()
	}
	return finishAttempt(a, nil)
}

// Wait blocks for d and returns the notification payloads received
// (integers; another payload is reported as an error).
func (l *Listener) Wait(ctx context.Context, d time.Duration) Outcome {
	a := Attempt{Autocommit: true, At: PhaseStatement}
	if !l.Connected() {
		fill(&a, errors.New("listener not connected"), ctx)
		a.At, a.Transport = PhaseConnect, true
		return finishAttempt(a, nil)
	}
	wctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	var err error
	for {
		err = l.conn.PgConn().WaitForNotification(wctx)
		if err != nil {
			break
		}
	}
	if wctx.Err() != nil && ctx.Err() == nil && !l.conn.IsClosed() {
		err = nil // the wait's own deadline: a normal end
	}
	l.mu.Lock()
	got := l.pending
	l.pending = nil
	l.mu.Unlock()
	if err != nil {
		fill(&a, err, ctx)
		l.Close()
		return finishAttempt(a, nil)
	}
	ids := []any{}
	for _, p := range got {
		n, perr := strconv.ParseInt(p, 10, 64)
		if perr != nil {
			fill(&a, fmt.Errorf("unexpected notification payload %q", p), ctx)
			return finishAttempt(a, nil)
		}
		ids = append(ids, n)
	}
	a.StatementExecuted = true
	return finishAttempt(a, ids)
}

func finishAttempt(a Attempt, v any) Outcome {
	o := Outcome{Type: Classify(a), Value: v, Attempt: a}
	if a.Err != nil {
		o.Err = ErrObject(a)
	}
	return o
}

// TupleText extracts the text columns of a pgoutput tuple by name.
func TupleText(names []string, cols [][]byte) map[string]string {
	out := map[string]string{}
	for i, n := range names {
		if i < len(cols) && cols[i] != nil {
			out[n] = string(cols[i])
		}
	}
	return out
}

// ParseBigintArray parses PostgreSQL's text form of bigint[] ("{1,2,3}").
func ParseBigintArray(s string) ([]int64, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return nil, fmt.Errorf("not an array: %q", s)
	}
	body := strings.TrimSpace(s[1 : len(s)-1])
	out := []int64{}
	if body == "" {
		return out, nil
	}
	for _, f := range strings.Split(body, ",") {
		n, err := strconv.ParseInt(strings.TrimSpace(f), 10, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}
