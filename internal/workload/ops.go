package workload

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Setup is the single setup transaction of CONTRACT §7 (schema cp, W1–W4
// tables, P22 bank accounts), run by the runner through gw-z1.
func Setup(accounts, initial int64) []string {
	stmts := []string{
		"create schema cp",
		"create table cp.append (k int primary key, vals bigint[] not null)",
		"create table cp.reg (k int primary key, v bigint)",
		"create table cp.acct (id int primary key, bal bigint not null)",
		"create table cp.ledger (id bigint primary key, payload text not null)",
	}
	for id := int64(0); id < accounts; id++ {
		stmts = append(stmts, fmt.Sprintf("insert into cp.acct (id, bal) values (%d, %d)", id, initial))
	}
	return stmts
}

// RunSetup executes Setup in one transaction.
func (s *Session) RunSetup(ctx context.Context, stmts []string) Outcome {
	return s.Do(ctx, false, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		a.At = PhaseBegin
		tx, err := c.Begin(ctx)
		if err != nil {
			return nil, err
		}
		for _, st := range stmts {
			a.At, a.WriteSent = PhaseStatement, true
			if _, err := tx.Exec(ctx, st); err != nil {
				s.rollback(tx)
				return nil, err
			}
			a.WriteExecuted, a.StatementExecuted = true, true
		}
		a.At = PhaseCommit
		return map[string]any{"statements": len(stmts)}, tx.Commit(ctx)
	})
}

func isoLevel(iso string) (pgx.TxIsoLevel, error) {
	switch iso {
	case "serializable":
		return pgx.Serializable, nil
	case "repeatable-read":
		return pgx.RepeatableRead, nil
	case "read-committed":
		return pgx.ReadCommitted, nil
	}
	return "", fmt.Errorf("unknown isolation %q", iso)
}

// Txn runs a W1 list-append transaction: micro-ops ["append", k, v] and
// ["r", k, nil]. The ok value has every read filled with the observed list
// (nil when the key has no row).
func (s *Session) Txn(ctx context.Context, iso string, mops []any) Outcome {
	return s.Do(ctx, false, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		lvl, err := isoLevel(iso)
		if err != nil {
			return nil, err
		}
		a.At = PhaseBegin
		tx, err := c.BeginTx(ctx, pgx.TxOptions{IsoLevel: lvl})
		if err != nil {
			return nil, err
		}
		out := make([]any, len(mops))
		for i, m := range mops {
			mo, ok := m.([]any)
			if !ok || len(mo) != 3 {
				s.rollback(tx)
				return nil, fmt.Errorf("bad micro-op %v", m)
			}
			k, err := Int(mo[1])
			if err != nil {
				s.rollback(tx)
				return nil, err
			}
			a.At = PhaseStatement
			switch mo[0] {
			case "append":
				v, err := Int(mo[2])
				if err != nil {
					s.rollback(tx)
					return nil, err
				}
				a.WriteSent = true
				_, err = tx.Exec(ctx, "insert into cp.append (k, vals) values ($1, array[$2::bigint]) on conflict (k) do update set vals = cp.append.vals || excluded.vals", k, v)
				if err != nil {
					s.rollback(tx)
					return nil, err
				}
				a.WriteExecuted, a.StatementExecuted = true, true
				out[i] = []any{"append", k, v}
			case "r":
				var vals []int64
				err := tx.QueryRow(ctx, "select vals from cp.append where k = $1", k).Scan(&vals)
				if errors.Is(err, pgx.ErrNoRows) {
					out[i] = []any{"r", k, nil}
				} else if err != nil {
					s.rollback(tx)
					return nil, err
				} else {
					if vals == nil {
						vals = []int64{}
					}
					out[i] = []any{"r", k, vals}
				}
				a.StatementExecuted = true
			default:
				s.rollback(tx)
				return nil, fmt.Errorf("bad micro-op %v", m)
			}
		}
		a.At = PhaseCommit
		return out, s.commit(ctx, c, tx, a)
	})
}

// commit sends COMMIT; with CancelCommitAfter set, a CancelRequest goes out
// if COMMIT has not returned by then (C7: "client CancelRequest 1 s after
// COMMIT is sent"). The request's fate is recorded in a.CancelRequest.
func (s *Session) commit(ctx context.Context, c *pgx.Conn, tx pgx.Tx, a *Attempt) error {
	if s.CancelCommitAfter <= 0 {
		return tx.Commit(ctx)
	}
	done := make(chan struct{})
	fired := make(chan string, 1)
	t := time.AfterFunc(s.CancelCommitAfter, func() {
		select {
		case <-done:
			fired <- ""
			return
		default:
		}
		cctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := c.PgConn().CancelRequest(cctx); err != nil {
			fired <- "error: " + err.Error()
			return
		}
		fired <- "sent"
	})
	err := tx.Commit(ctx)
	close(done)
	if t.Stop() {
		return err // the timer never fired
	}
	a.CancelRequest = <-fired
	return err
}

// Probe runs one claims probe (§3 G7) as a single autocommit statement:
// SHOW returns the setting, anything else its command tag.
func (s *Session) Probe(ctx context.Context, sql string) Outcome {
	return s.Do(ctx, true, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		a.At = PhaseStatement
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "SHOW ") {
			var v string
			if err := c.QueryRow(ctx, sql).Scan(&v); err != nil {
				return nil, err
			}
			a.StatementExecuted = true
			return map[string]any{"setting": v}, nil
		}
		tag, err := c.Exec(ctx, sql)
		if err != nil {
			return nil, err
		}
		a.StatementExecuted = true
		return map[string]any{"command_tag": tag.String()}, nil
	})
}

// Register runs a W2 autocommit read, write or cas.
func (s *Session) Register(ctx context.Context, f string, v map[string]any) Outcome {
	return s.Do(ctx, true, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		k, err := Int(v["k"])
		if err != nil {
			return nil, err
		}
		a.At = PhaseStatement
		switch f {
		case "read":
			var val *int64
			err := c.QueryRow(ctx, "select v from cp.reg where k = $1", k).Scan(&val)
			if errors.Is(err, pgx.ErrNoRows) {
				return map[string]any{"k": k, "v": nil}, nil
			}
			if err != nil {
				return nil, err
			}
			if val == nil {
				return map[string]any{"k": k, "v": nil}, nil
			}
			return map[string]any{"k": k, "v": *val}, nil
		case "write":
			nv, err := Int(v["v"])
			if err != nil {
				return nil, err
			}
			a.WriteSent = true
			if _, err := c.Exec(ctx, "insert into cp.reg (k, v) values ($1, $2) on conflict (k) do update set v = excluded.v", k, nv); err != nil {
				return nil, err
			}
			return map[string]any{"k": k, "v": nv}, nil
		case "cas":
			old, err := Int(v["old"])
			if err != nil {
				return nil, err
			}
			nv, err := Int(v["new"])
			if err != nil {
				return nil, err
			}
			a.WriteSent = true
			var got int64
			err = c.QueryRow(ctx, "update cp.reg set v = $3 where k = $1 and v = $2 returning v", k, old, nv).Scan(&got)
			if errors.Is(err, pgx.ErrNoRows) {
				return map[string]any{"k": k, "old": old, "new": nv, "applied": false}, nil
			}
			if err != nil {
				return nil, err
			}
			return map[string]any{"k": k, "old": old, "new": nv, "applied": got == nv}, nil
		}
		return nil, fmt.Errorf("unknown register op %q", f)
	})
}

// Bank runs a W3 serializable transfer or read-all.
func (s *Session) Bank(ctx context.Context, f string, v map[string]any) Outcome {
	return s.Do(ctx, false, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		a.At = PhaseBegin
		tx, err := c.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return nil, err
		}
		switch f {
		case "transfer":
			from, err1 := Int(v["from"])
			to, err2 := Int(v["to"])
			amt, err3 := Int(v["amt"])
			if err := errors.Join(err1, err2, err3); err != nil {
				s.rollback(tx)
				return nil, err
			}
			a.At, a.WriteSent = PhaseStatement, true
			var bal int64
			err := tx.QueryRow(ctx, "update cp.acct set bal = bal - $2 where id = $1 and bal >= $2 returning bal", from, amt).Scan(&bal)
			if errors.Is(err, pgx.ErrNoRows) {
				// Insufficient funds: the client ends the transaction itself.
				a.StatementExecuted = true
				if err := tx.Rollback(ctx); err != nil {
					return nil, err
				}
				return map[string]any{"from": from, "to": to, "amt": amt, "applied": false}, nil
			}
			if err != nil {
				s.rollback(tx)
				return nil, err
			}
			a.WriteExecuted, a.StatementExecuted = true, true
			if _, err := tx.Exec(ctx, "update cp.acct set bal = bal + $2 where id = $1", to, amt); err != nil {
				s.rollback(tx)
				return nil, err
			}
			a.At = PhaseCommit
			return map[string]any{"from": from, "to": to, "amt": amt, "applied": true}, tx.Commit(ctx)
		case "read-all":
			a.At = PhaseStatement
			rows, err := tx.Query(ctx, "select id, bal from cp.acct order by id")
			if err != nil {
				s.rollback(tx)
				return nil, err
			}
			accts, err := pairs(rows)
			if err != nil {
				s.rollback(tx)
				return nil, err
			}
			a.StatementExecuted = true
			a.At = PhaseCommit
			return accts, tx.Commit(ctx)
		}
		s.rollback(tx)
		return nil, fmt.Errorf("unknown bank op %q", f)
	})
}

// Ledger runs a W4 autocommit insert.
func (s *Session) Ledger(ctx context.Context, v map[string]any) Outcome {
	return s.Do(ctx, true, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		id, err := Int(v["id"])
		if err != nil {
			return nil, err
		}
		payload, _ := v["payload"].(string)
		a.At, a.WriteSent = PhaseStatement, true
		if _, err := c.Exec(ctx, "insert into cp.ledger (id, payload) values ($1, $2)", id, payload); err != nil {
			return nil, err
		}
		return map[string]any{"id": id, "payload": payload}, nil
	})
}

// FinalRead reads the complete state of one workload (CONTRACT §6.4) with
// one autocommit statement.
func (s *Session) FinalRead(ctx context.Context, wl string) Outcome {
	return s.Do(ctx, true, func(ctx context.Context, c *pgx.Conn, a *Attempt) (any, error) {
		a.At = PhaseStatement
		switch wl {
		case "append":
			rows, err := c.Query(ctx, "select k, vals from cp.append order by k")
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			out := []any{}
			for rows.Next() {
				var k int64
				var vals []int64
				if err := rows.Scan(&k, &vals); err != nil {
					return nil, err
				}
				if vals == nil {
					vals = []int64{}
				}
				out = append(out, []any{k, vals})
			}
			return out, rows.Err()
		case "register":
			rows, err := c.Query(ctx, "select k, v from cp.reg order by k")
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			out := []any{}
			for rows.Next() {
				var k int64
				var v *int64
				if err := rows.Scan(&k, &v); err != nil {
					return nil, err
				}
				if v == nil {
					out = append(out, []any{k, nil})
				} else {
					out = append(out, []any{k, *v})
				}
			}
			return out, rows.Err()
		case "bank":
			rows, err := c.Query(ctx, "select id, bal from cp.acct order by id")
			if err != nil {
				return nil, err
			}
			return pairs(rows)
		case "ledger":
			rows, err := c.Query(ctx, "select id from cp.ledger order by id")
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			out := []int64{}
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					return nil, err
				}
				out = append(out, id)
			}
			return out, rows.Err()
		}
		return nil, fmt.Errorf("unknown workload %q", wl)
	})
}

func pairs(rows pgx.Rows) ([]any, error) {
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var a, b int64
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		out = append(out, []any{a, b})
	}
	return out, rows.Err()
}
