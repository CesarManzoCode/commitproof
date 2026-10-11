package workload

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A statement fails with a SQLSTATE and the rollback that follows loses the
// connection: rollback drops the session's connection, and Do must not use
// it afterwards (found by K3's first F2a run: a nil dereference in Do). The
// classification is §6.2 row 2's, and the next operation reconnects.
func TestSessionSQLStateThenRollbackLost(t *testing.T) {
	f := newFakePG(t, func(sql string) action {
		switch {
		case strings.HasPrefix(sql, "insert"):
			return action{code: "40001", message: "could not serialize"}
		case strings.HasPrefix(sql, "rollback"):
			return action{close: true}
		}
		return action{}
	})
	s := session(f, 5*time.Second)
	defer s.Close()
	expect(t, "txn", s.Txn(context.Background(), "serializable", txnAppendRead), Fail, "40001", PhaseStatement)
	if s.conn != nil {
		t.Fatal("connection kept after the rollback lost it")
	}
	expect(t, "register", s.Register(context.Background(), "read", map[string]any{"k": int64(1)}), OK, "", "")
}
