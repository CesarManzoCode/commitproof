package workload

import (
	"errors"
	"testing"
)

// One case per row of CONTRACT §6.2 (expected values written from the table,
// not from running Classify). K2.5 adds wire-level tests.
func TestClassifyTable(t *testing.T) {
	e := errors.New("x")
	cases := []struct {
		row  string
		a    Attempt
		want string
	}{
		{"1 success", Attempt{}, OK},
		{"2 class 40 before commit", Attempt{Err: e, SQLState: "40P01", At: PhaseStatement, WriteSent: true, WriteExecuted: true}, Fail},
		{"2 40001 at begin", Attempt{Err: e, SQLState: "40001", At: PhaseBegin}, Fail},
		{"3 40001 by COMMIT", Attempt{Err: e, SQLState: "40001", At: PhaseCommit, WriteSent: true, WriteExecuted: true}, Fail},
		{"4 class 23 before commit", Attempt{Err: e, SQLState: "23505", At: PhaseStatement, Autocommit: true, WriteSent: true}, Fail},
		{"5 0A000 before any write", Attempt{Err: e, SQLState: "0A000", At: PhaseStatement}, Fail},
		{"5 42P01 before any write", Attempt{Err: e, SQLState: "42P01", At: PhaseStatement, WriteSent: true}, Fail},
		{"5 22003 after a write executed -> row 11", Attempt{Err: e, SQLState: "22003", At: PhaseStatement, WriteSent: true, WriteExecuted: true}, Info},
		{"6 25006", Attempt{Err: e, SQLState: "25006", At: PhaseStatement, WriteSent: true}, Fail},
		{"7 MTF01 before first statement", Attempt{Err: e, SQLState: "MTF01", At: PhaseStatement, Autocommit: true}, Fail},
		{"7 no writable primary (57P03 text)", Attempt{Err: e, SQLState: "57P03", Message: "no writable primary is currently available", At: PhaseBegin}, Fail},
		{"7 no writable primary after a statement -> row 11", Attempt{Err: e, SQLState: "57P03", Message: "no writable primary is currently available", At: PhaseStatement, StatementExecuted: true}, Info},
		{"8 57014 at commit", Attempt{Err: e, SQLState: "57014", At: PhaseCommit}, Info},
		{"8 08006 at commit", Attempt{Err: e, SQLState: "08006", At: PhaseCommit}, Info},
		{"8 transport at commit", Attempt{Err: e, Transport: true, At: PhaseCommit}, Info},
		{"8 timeout at commit", Attempt{Err: e, ClientTimeout: true, At: PhaseCommit}, Info},
		{"9 timeout after first write sent", Attempt{Err: e, ClientTimeout: true, At: PhaseStatement, WriteSent: true}, Info},
		{"10 connection error before any statement", Attempt{Err: e, Transport: true, At: PhaseConnect}, Fail},
		{"10 server refuses startup", Attempt{Err: e, SQLState: "57P03", Message: "the database system is starting up", At: PhaseConnect}, Fail},
		{"10 connect timeout", Attempt{Err: e, ClientTimeout: true, At: PhaseConnect}, Fail},
		{"11 unlisted SQLSTATE", Attempt{Err: e, SQLState: "XX000", At: PhaseStatement}, Info},
		{"11 57P01 mid-transaction", Attempt{Err: e, SQLState: "57P01", At: PhaseStatement, WriteSent: true}, Info},
		{"12 autocommit transport after send", Attempt{Err: e, Transport: true, At: PhaseStatement, Autocommit: true, WriteSent: true}, Info},
		{"12 autocommit timeout (read)", Attempt{Err: e, ClientTimeout: true, At: PhaseStatement, Autocommit: true}, Info},
		{"preamble: read-only txn timeout", Attempt{Err: e, ClientTimeout: true, At: PhaseStatement}, Info},
	}
	for _, c := range cases {
		if got := Classify(c.a); got != c.want {
			t.Errorf("row %s: Classify = %s, want %s", c.row, got, c.want)
		}
	}
}

func TestErrObject(t *testing.T) {
	e := errors.New("x")
	for _, c := range []struct {
		a                   Attempt
		sqlstate, class, ph string
	}{
		{Attempt{Err: e, SQLState: "40001", At: PhaseCommit}, "40001", "40", PhaseCommit},
		{Attempt{Err: e, ClientTimeout: true, At: PhaseStatement}, "", "timeout", PhaseTimeout},
		{Attempt{Err: e, Transport: true, At: PhaseStatement}, "", "transport", PhaseTransport},
		{Attempt{Err: e, Transport: true, At: PhaseConnect}, "", "connect", PhaseConnect},
	} {
		m := ErrObject(c.a)
		if m["sqlstate"] != c.sqlstate || m["class"] != c.class || m["phase"] != c.ph {
			t.Errorf("ErrObject(%+v) = %v", c.a, m)
		}
	}
}
