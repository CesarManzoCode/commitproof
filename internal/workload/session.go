package workload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Gateway PostgreSQL port and the demo credentials (S01, S03:
// k8s-postgres-password-secret.yaml).
const (
	GatewayPort = 15432
	DBUser      = "postgres"
	DBPassword  = "postgres"
	DBName      = "postgres"
)

// Session is one client's connection to one gateway. Not safe for concurrent
// use: a logical process issues one operation at a time.
type Session struct {
	GW      string
	AppName string
	Timeout time.Duration
	// Port overrides GatewayPort (tests drive a fake server, K2.5).
	Port int
	// StatementTimeoutMS > 0: every new connection first runs
	// `SET statement_timeout = <ms>` (C7, P19; gateway-managed, S16). A
	// failure there is a connection failure (§6.2: fail, nothing sent).
	StatementTimeoutMS int64
	// CancelCommitAfter > 0: a transaction whose COMMIT has not returned
	// this long after it was sent gets a CancelRequest (C7, S17).
	CancelCommitAfter time.Duration
	// User and Password override the demo credentials (W8's roles, whose
	// password is their name; ADR-0030).
	User, Password string

	conn    *pgx.Conn
	mu      sync.Mutex
	notices []any
}

// Outcome is one completed operation.
type Outcome struct {
	Type    string
	Value   any
	Err     map[string]any
	Notices []any
	Attempt Attempt
}

func (s *Session) config() (*pgx.ConnConfig, error) {
	port := GatewayPort
	if s.Port != 0 {
		port = s.Port
	}
	user, pw := DBUser, DBPassword
	if s.User != "" {
		user, pw = s.User, s.Password
	}
	cfg, err := pgx.ParseConfig(fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		s.GW, port, user, pw, DBName))
	if err != nil {
		return nil, err
	}
	cfg.ConnectTimeout = s.Timeout
	// One round trip per statement, no named prepared statements, no client
	// caches: what the client sends is exactly the statement text.
	cfg.DefaultQueryExecMode = pgx.QueryExecModeExec
	cfg.StatementCacheCapacity = 0
	cfg.DescriptionCacheCapacity = 0
	if s.AppName != "" {
		cfg.RuntimeParams["application_name"] = s.AppName
	}
	cfg.OnNotice = func(_ *pgconn.PgConn, n *pgconn.Notice) {
		msg := n.Message
		if len(msg) > 256 {
			msg = msg[:256]
			for !utf8.ValidString(msg) {
				msg = msg[:len(msg)-1]
			}
		}
		s.mu.Lock()
		s.notices = append(s.notices, map[string]any{"severity": n.Severity, "sqlstate": n.Code, "message": msg})
		s.mu.Unlock()
	}
	return cfg, nil
}

// Connected reports whether the session holds an open connection (the next
// operation reuses it rather than connecting).
func (s *Session) Connected() bool { return s.conn != nil && !s.conn.IsClosed() }

// Close drops the connection.
func (s *Session) Close() {
	if s.conn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		s.conn.Close(ctx)
		cancel()
		s.conn = nil
	}
}

func (s *Session) takeNotices() []any {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.notices
	s.notices = nil
	return n
}

// fill sets SQLState/Message/ClientTimeout/Transport from err.
func fill(a *Attempt, err error, ctx context.Context) {
	a.Err = err
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		a.SQLState, a.Message = pgErr.Code, pgErr.Message
		return
	}
	a.Message = err.Error()
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil || (errors.As(err, &ne) && ne.Timeout()) || pgconn.Timeout(err) {
		a.ClientTimeout = true
		return
	}
	a.Transport = true
}

// Do runs one operation with the P01 deadline and classifies it (§6.2).
// exec does the work on a live connection and records progress in a.
func (s *Session) Do(parent context.Context, autocommit bool, exec func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error)) Outcome {
	ctx, cancel := context.WithTimeout(parent, s.Timeout)
	defer cancel()
	s.takeNotices()
	a := Attempt{Autocommit: autocommit}
	if s.conn == nil || s.conn.IsClosed() {
		s.conn = nil
		a.At = PhaseConnect
		cfg, err := s.config()
		if err != nil {
			fill(&a, err, ctx)
			return s.finish(a, nil)
		}
		conn, err := pgx.ConnectConfig(ctx, cfg)
		if err != nil {
			fill(&a, err, ctx)
			return s.finish(a, nil)
		}
		if s.StatementTimeoutMS > 0 {
			if _, err := conn.Exec(ctx, fmt.Sprintf("SET statement_timeout = %d", s.StatementTimeoutMS)); err != nil {
				fill(&a, err, ctx)
				cctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				conn.Close(cctx)
				cancel()
				return s.finish(a, nil)
			}
		}
		s.conn = conn
	}
	v, err := exec(ctx, s.conn, &a)
	if err != nil {
		fill(&a, err, ctx)
		// exec may already have dropped the connection (a failed rollback
		// closes it).
		if a.SQLState == "" || s.conn == nil || s.conn.IsClosed() {
			s.Close() // transport failure or timeout: never reuse
		}
	}
	return s.finish(a, v)
}

func (s *Session) finish(a Attempt, v any) Outcome {
	o := Outcome{Type: Classify(a), Value: v, Attempt: a, Notices: s.takeNotices()}
	if a.Err != nil {
		o.Err = ErrObject(a)
	}
	return o
}

// rollback ends a failed explicit transaction. If the connection cannot be
// rolled back cleanly it is closed (the server then aborts the transaction).
func (s *Session) rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		s.Close()
	}
}

// Int extracts an int64 from a decoded plan value (json.Number or int64).
func Int(v any) (int64, error) {
	switch x := v.(type) {
	case int64:
		return x, nil
	case int:
		return int64(x), nil
	case json.Number:
		return strconv.ParseInt(string(x), 10, 64)
	}
	return 0, fmt.Errorf("not an integer: %T %v", v, v)
}
