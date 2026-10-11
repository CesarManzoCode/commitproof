package workload

// K2.5 (CONTRACT §15 K2): one test per row of the §6.2 completion
// classification table, driving the real workload client code (pgx through
// Session) against a fake PostgreSQL wire server that answers each
// statement as scripted: success, an ErrorResponse with a given SQLSTATE, a
// WARNING notice, a closed connection, or no answer at all. Expected
// classifications are written from the table, not from running Classify.

import (
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"
)

// action is the scripted answer to one statement.
type action struct {
	code    string // SQLSTATE of an ErrorResponse ("" = success)
	message string
	warning string // WARNING notice sent before success
	hang    bool   // never answer (client timeout)
	close   bool   // close the connection (transport error)
}

// fakePG is a minimal PostgreSQL server for one test.
type fakePG struct {
	t      *testing.T
	ln     net.Listener
	script func(sql string) action
	mu     sync.Mutex
	seen   []string
	wg     sync.WaitGroup
}

func newFakePG(t *testing.T, script func(sql string) action) *fakePG {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakePG{t: t, ln: ln, script: script}
	f.wg.Add(1)
	go f.serve()
	t.Cleanup(func() { ln.Close(); f.wg.Wait() })
	return f
}

func (f *fakePG) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakePG) serve() {
	defer f.wg.Done()
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return
		}
		f.wg.Add(1)
		go func() { defer f.wg.Done(); f.conn(c) }()
	}
}

func (f *fakePG) statements() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.seen...)
}

// rows returns the result columns (name, OID) and one text row for a
// statement that returns rows, nil otherwise.
func rows(sql string) ([]pgproto3.FieldDescription, []byte) {
	switch {
	case strings.HasPrefix(sql, "select vals from cp.append"):
		return []pgproto3.FieldDescription{{Name: []byte("vals"), DataTypeOID: 1016, DataTypeSize: -1, TypeModifier: -1}}, []byte("{1,2}")
	case strings.HasPrefix(sql, "select v from cp.reg"), strings.HasPrefix(sql, "update cp.reg"):
		return []pgproto3.FieldDescription{{Name: []byte("v"), DataTypeOID: 20, DataTypeSize: 8, TypeModifier: -1}}, []byte("7")
	case strings.HasPrefix(sql, "update cp.acct"):
		return []pgproto3.FieldDescription{{Name: []byte("bal"), DataTypeOID: 20, DataTypeSize: 8, TypeModifier: -1}}, []byte("95")
	}
	return nil, nil
}

func tag(sql string) string {
	switch w := strings.Fields(sql)[0]; w {
	case "insert":
		return "INSERT 0 1"
	case "select":
		return "SELECT 1"
	case "update":
		return "UPDATE 1"
	default:
		return strings.ToUpper(w)
	}
}

func (f *fakePG) conn(c net.Conn) {
	defer c.Close()
	be := pgproto3.NewBackend(c, c)
	msg, err := be.ReceiveStartupMessage()
	if err != nil {
		return
	}
	if _, ok := msg.(*pgproto3.StartupMessage); !ok {
		return // a CancelRequest from a timed-out client
	}
	be.Send(&pgproto3.AuthenticationOk{})
	for k, v := range map[string]string{"server_version": "17.7", "client_encoding": "UTF8", "standard_conforming_strings": "on",
		"DateStyle": "ISO, MDY", "integer_datetimes": "on", "TimeZone": "UTC"} {
		be.Send(&pgproto3.ParameterStatus{Name: k, Value: v})
	}
	be.Send(&pgproto3.BackendKeyData{ProcessID: 1, SecretKey: []byte{0, 0, 0, 1}})
	tx := byte('I')
	be.Send(&pgproto3.ReadyForQuery{TxStatus: tx})
	if be.Flush() != nil {
		return
	}
	var sql string
	var act action
	failed := false
	respond := func(withRows bool) bool { // false: stop serving this connection
		f.mu.Lock()
		f.seen = append(f.seen, sql)
		f.mu.Unlock()
		act = f.script(sql)
		switch {
		case act.close:
			return false
		case act.hang:
			c.SetDeadline(time.Now().Add(2 * time.Second)) // the client gives up first
			io.Copy(io.Discard, c)
			return false
		case act.code != "":
			be.Send(&pgproto3.ErrorResponse{Severity: "ERROR", Code: act.code, Message: act.message})
			failed = true
			if tx == 'T' {
				tx = 'E'
			}
			return true
		}
		if act.warning != "" {
			be.Send(&pgproto3.NoticeResponse{Severity: "WARNING", Code: "01000", Message: act.warning})
		}
		if withRows {
			if fd, row := rows(sql); fd != nil {
				be.Send(&pgproto3.DataRow{Values: [][]byte{row}})
			}
		}
		be.Send(&pgproto3.CommandComplete{CommandTag: []byte(tag(sql))})
		switch strings.Fields(sql)[0] {
		case "begin":
			tx = 'T'
		case "commit", "rollback":
			tx = 'I'
		}
		return true
	}
	for {
		msg, err := be.Receive()
		if err != nil {
			return
		}
		switch m := msg.(type) {
		case *pgproto3.Query:
			sql, failed = strings.ToLower(strings.TrimSpace(m.String)), false
			if fd, _ := rows(sql); fd != nil && f.script(sql).code == "" && !f.script(sql).hang && !f.script(sql).close {
				be.Send(&pgproto3.RowDescription{Fields: fd})
			}
			if !respond(true) {
				return
			}
			be.Send(&pgproto3.ReadyForQuery{TxStatus: tx})
		case *pgproto3.Parse:
			sql, failed = strings.ToLower(strings.TrimSpace(m.Query)), false
			be.Send(&pgproto3.ParseComplete{})
		case *pgproto3.Bind:
			if !failed {
				be.Send(&pgproto3.BindComplete{})
			}
		case *pgproto3.Describe:
			if failed {
				continue
			}
			if fd, _ := rows(sql); fd != nil {
				be.Send(&pgproto3.RowDescription{Fields: fd})
			} else {
				be.Send(&pgproto3.NoData{})
			}
		case *pgproto3.Execute:
			if !failed && !respond(true) {
				return
			}
		case *pgproto3.Sync:
			be.Send(&pgproto3.ReadyForQuery{TxStatus: tx})
			failed = false
		case *pgproto3.Terminate:
			return
		}
		if be.Flush() != nil {
			return
		}
	}
}

// on returns a script answering statements starting with prefix with a,
// and everything else with success.
func on(prefix string, a action) func(string) action {
	return func(sql string) action {
		if strings.HasPrefix(sql, prefix) {
			return a
		}
		return action{}
	}
}

func session(f *fakePG, timeout time.Duration) *Session {
	return &Session{GW: "127.0.0.1", Port: f.port(), AppName: "k2.5", Timeout: timeout}
}

var (
	txnAppendRead = []any{[]any{"append", int64(3), int64(4)}, []any{"r", int64(3), nil}}
	txnReadFirst  = []any{[]any{"r", int64(3), nil}, []any{"append", int64(3), int64(4)}}
	ledgerRow     = map[string]any{"id": int64(42), "payload": "x"}
	regWrite      = map[string]any{"k": int64(1), "v": int64(7)}
	transfer      = map[string]any{"from": int64(0), "to": int64(1), "amt": int64(5)}
)

func expect(t *testing.T, name string, o Outcome, want, sqlstate, phase string) {
	t.Helper()
	if o.Type != want {
		t.Errorf("%s: classified %s, want %s (attempt %+v)", name, o.Type, want, o.Attempt)
	}
	if want == OK {
		if o.Err != nil {
			t.Errorf("%s: ok outcome carries err %v", name, o.Err)
		}
		return
	}
	if o.Err == nil {
		t.Fatalf("%s: %s outcome without err", name, o.Type)
	}
	if o.Err["sqlstate"] != sqlstate || (phase != "" && o.Err["phase"] != phase) {
		t.Errorf("%s: err %v, want sqlstate %q phase %q", name, o.Err, sqlstate, phase)
	}
}

const short = 400 * time.Millisecond

// Row 1: success response for every statement and COMMIT → ok.
func TestWireRow01Success(t *testing.T) {
	f := newFakePG(t, func(string) action { return action{} })
	s := session(f, 5*time.Second)
	defer s.Close()
	o := s.Txn(context.Background(), "serializable", txnAppendRead)
	expect(t, "txn", o, OK, "", "")
	got := o.Value.([]any)[1].([]any)[2].([]int64)
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("read value %v, want [1 2]", got)
	}
	expect(t, "ledger", s.Ledger(context.Background(), ledgerRow), OK, "", "")
	expect(t, "register", s.Register(context.Background(), "write", regWrite), OK, "", "")
	if st := f.statements(); !strings.HasPrefix(st[len(st)-1], "insert into cp.reg") {
		t.Errorf("last statement %q", st[len(st)-1])
	}
}

// Row 2: SQLSTATE class 40 at a statement before COMMIT → fail.
func TestWireRow02Class40BeforeCommit(t *testing.T) {
	for _, code := range []string{"40001", "40P01"} {
		f := newFakePG(t, on("insert", action{code: code, message: "could not serialize"}))
		s := session(f, 5*time.Second)
		expect(t, code, s.Txn(context.Background(), "serializable", txnAppendRead), Fail, code, PhaseStatement)
		s.Close()
	}
	f := newFakePG(t, on("update cp.acct", action{code: "40001"}))
	s := session(f, 5*time.Second)
	defer s.Close()
	expect(t, "bank", s.Bank(context.Background(), "transfer", transfer), Fail, "40001", PhaseStatement)
}

// Row 3: 40001 returned by COMMIT → fail.
func TestWireRow03SerializationFailureAtCommit(t *testing.T) {
	f := newFakePG(t, on("commit", action{code: "40001", message: "could not serialize access"}))
	s := session(f, 5*time.Second)
	defer s.Close()
	expect(t, "txn", s.Txn(context.Background(), "serializable", txnAppendRead), Fail, "40001", PhaseCommit)
}

// Row 4: SQLSTATE class 23 before commit → fail.
func TestWireRow04IntegrityBeforeCommit(t *testing.T) {
	f := newFakePG(t, on("insert", action{code: "23505", message: "duplicate key value"}))
	s := session(f, 5*time.Second)
	defer s.Close()
	expect(t, "ledger", s.Ledger(context.Background(), ledgerRow), Fail, "23505", PhaseStatement)
	expect(t, "txn", s.Txn(context.Background(), "serializable", txnAppendRead), Fail, "23505", PhaseStatement)
}

// Row 5: 0A000, 42xxx, 22xxx before any write statement executed → fail.
func TestWireRow05RejectedBeforeAnyWrite(t *testing.T) {
	for _, code := range []string{"0A000", "42P01", "22P02"} {
		f := newFakePG(t, on("select vals", action{code: code}))
		s := session(f, 5*time.Second)
		expect(t, code, s.Txn(context.Background(), "serializable", txnReadFirst), Fail, code, PhaseStatement)
		s.Close()
	}
	f := newFakePG(t, on("insert", action{code: "0A000"}))
	s := session(f, 5*time.Second)
	defer s.Close()
	expect(t, "write rejected", s.Ledger(context.Background(), ledgerRow), Fail, "0A000", PhaseStatement)
}

// Row 6: SQLSTATE 25006 (read-only) on a statement before commit → fail.
func TestWireRow06ReadOnly(t *testing.T) {
	f := newFakePG(t, on("insert", action{code: "25006", message: "cannot execute INSERT in a read-only transaction"}))
	s := session(f, 5*time.Second)
	defer s.Close()
	expect(t, "ledger", s.Ledger(context.Background(), ledgerRow), Fail, "25006", PhaseStatement)
	expect(t, "txn", s.Txn(context.Background(), "serializable", txnAppendRead), Fail, "25006", PhaseStatement)
}

// Row 7: MTF01 / "no writable primary" before the first statement executed
// → fail.
func TestWireRow07NoWritablePrimary(t *testing.T) {
	f := newFakePG(t, on("insert", action{code: "MTF01", message: "no writable primary"}))
	s := session(f, 5*time.Second)
	expect(t, "MTF01", s.Register(context.Background(), "write", regWrite), Fail, "MTF01", PhaseStatement)
	s.Close()
	f = newFakePG(t, on("begin", action{code: "57P03", message: "no writable primary is currently available"}))
	s = session(f, 5*time.Second)
	defer s.Close()
	expect(t, "57P03 text", s.Txn(context.Background(), "serializable", txnAppendRead), Fail, "57P03", PhaseBegin)
}

// Row 8: any error, timeout or transport failure during or after sending
// COMMIT → info.
func TestWireRow08ErrorAtCommit(t *testing.T) {
	for _, code := range []string{"57014", "57P01", "08006", "XX000"} {
		f := newFakePG(t, on("commit", action{code: code}))
		s := session(f, 5*time.Second)
		expect(t, code, s.Txn(context.Background(), "serializable", txnAppendRead), Info, code, PhaseCommit)
		s.Close()
	}
	f := newFakePG(t, on("commit", action{close: true}))
	s := session(f, 5*time.Second)
	expect(t, "transport", s.Txn(context.Background(), "serializable", txnAppendRead), Info, "", "")
	s.Close()
	f = newFakePG(t, on("commit", action{hang: true}))
	s = session(f, short)
	defer s.Close()
	expect(t, "timeout", s.Txn(context.Background(), "serializable", txnAppendRead), Info, "", PhaseTimeout)
}

// Row 9: client-side timeout (P01) at any phase after the first write
// statement was sent → info.
func TestWireRow09TimeoutAfterWriteSent(t *testing.T) {
	f := newFakePG(t, on("insert", action{hang: true}))
	s := session(f, short)
	defer s.Close()
	expect(t, "txn", s.Txn(context.Background(), "serializable", txnAppendRead), Info, "", PhaseTimeout)
	f2 := newFakePG(t, on("select vals", action{hang: true}))
	s2 := session(f2, short)
	defer s2.Close()
	expect(t, "read after write", s2.Txn(context.Background(), "serializable", txnAppendRead), Info, "", PhaseTimeout)
}

// Row 10: connection error before any statement was sent → fail.
func TestWireRow10ConnectionError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // nothing listens: connection refused
	s := &Session{GW: "127.0.0.1", Port: port, Timeout: 5 * time.Second}
	expect(t, "refused", s.Ledger(context.Background(), ledgerRow), Fail, "", PhaseConnect)
	// A server that closes during startup.
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln2.Close()
	go func() {
		for {
			c, err := ln2.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	s2 := &Session{GW: "127.0.0.1", Port: ln2.Addr().(*net.TCPAddr).Port, Timeout: 5 * time.Second}
	expect(t, "closed at startup", s2.Txn(context.Background(), "serializable", txnAppendRead), Fail, "", PhaseConnect)
}

// Row 11: any SQLSTATE not listed → info.
func TestWireRow11UnlistedSQLState(t *testing.T) {
	for _, code := range []string{"XX000", "57P01", "53300"} {
		f := newFakePG(t, on("insert", action{code: code}))
		s := session(f, 5*time.Second)
		expect(t, code, s.Txn(context.Background(), "serializable", txnAppendRead), Info, code, PhaseStatement)
		s.Close()
	}
}

// Row 12: autocommit single statement, transport error or timeout after
// sending → info.
func TestWireRow12AutocommitTransport(t *testing.T) {
	f := newFakePG(t, on("insert", action{close: true}))
	s := session(f, 5*time.Second)
	expect(t, "ledger transport", s.Ledger(context.Background(), ledgerRow), Info, "", "")
	s.Close()
	f = newFakePG(t, on("insert", action{hang: true}))
	s = session(f, short)
	expect(t, "ledger timeout", s.Ledger(context.Background(), ledgerRow), Info, "", PhaseTimeout)
	s.Close()
	f = newFakePG(t, on("select v from cp.reg", action{hang: true}))
	s = session(f, short)
	defer s.Close()
	expect(t, "register read timeout", s.Register(context.Background(), "read", map[string]any{"k": int64(1)}), Info, "", PhaseTimeout)
}

// §6.2 note (S61): COMMIT succeeding with the WARNING "canceling wait for
// synchronous replication" is ok, and the notice is recorded.
func TestWireS61CommitWarning(t *testing.T) {
	const w = "canceling wait for synchronous replication due to user request"
	f := newFakePG(t, on("commit", action{warning: w}))
	s := session(f, 5*time.Second)
	defer s.Close()
	o := s.Txn(context.Background(), "serializable", txnAppendRead)
	expect(t, "txn", o, OK, "", "")
	found := false
	for _, n := range o.Notices {
		m := n.(map[string]any)
		if m["severity"] == "WARNING" && strings.Contains(m["message"].(string), "canceling wait for synchronous replication") {
			found = true
		}
	}
	if !found {
		t.Errorf("WARNING not recorded: notices %v", o.Notices)
	}
}
