package workload

// C7 client behaviors and the claims probes (CONTRACT §15 K4) against the
// fake server of wire_test.go. Expected outcomes are written from §6.2 and
// §15 K4, not from running Classify.

import (
	"context"
	"strings"
	"testing"
	"time"
)

const s61 = "canceling wait for synchronous replication due to user request"

// P19: every new connection sets statement_timeout before the operation.
func TestStatementTimeoutSetOnConnect(t *testing.T) {
	f := newFakePG(t, func(string) action { return action{} })
	s := session(f, 5*time.Second)
	s.StatementTimeoutMS = 2000
	defer s.Close()
	expect(t, "insert", s.Ledger(context.Background(), ledgerRow), OK, "", "")
	expect(t, "insert 2", s.Ledger(context.Background(), ledgerRow), OK, "", "")
	st := f.statements()
	if len(st) != 3 || st[0] != "set statement_timeout = 2000" || !strings.HasPrefix(st[1], "insert") {
		t.Fatalf("statements %q", st)
	}
}

// A failing SET is a connection failure: nothing of the operation was sent
// (§6.2 "connection error before any statement was sent" → fail).
func TestStatementTimeoutSetFailureIsFail(t *testing.T) {
	f := newFakePG(t, on("set statement_timeout", action{code: "57P03", message: "no writable primary"}))
	s := session(f, 5*time.Second)
	s.StatementTimeoutMS = 2000
	defer s.Close()
	expect(t, "insert", s.Ledger(context.Background(), ledgerRow), Fail, "57P03", PhaseConnect)
	for _, q := range f.statements() {
		if strings.HasPrefix(q, "insert") {
			t.Fatalf("insert sent after a failed SET: %q", f.statements())
		}
	}
}

// C7: COMMIT outstanding after CancelCommitAfter → a CancelRequest is sent;
// COMMIT then succeeding with the S61 WARNING is ok (§6.2 last paragraph),
// the WARNING recorded.
func TestCancelRequestAfterCommit(t *testing.T) {
	f := newFakePG(t, on("commit", action{delay: 700 * time.Millisecond, warning: s61}))
	s := session(f, 5*time.Second)
	s.CancelCommitAfter = 200 * time.Millisecond
	defer s.Close()
	o := s.Txn(context.Background(), "serializable", txnAppendRead)
	expect(t, "txn", o, OK, "", "")
	if o.Attempt.CancelRequest != "sent" {
		t.Errorf("cancel request %q", o.Attempt.CancelRequest)
	}
	if len(o.Notices) != 1 || !strings.Contains(o.Notices[0].(map[string]any)["message"].(string), "canceling wait for synchronous replication") {
		t.Errorf("notices %v", o.Notices)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		f.mu.Lock()
		n := f.cancels
		f.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server received %d CancelRequests", n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A COMMIT answered before CancelCommitAfter sends no CancelRequest; COMMIT
// failing with 57014 after a cancel is info (§6.2 row 8).
func TestCancelRequestOnlyWhenOutstanding(t *testing.T) {
	f := newFakePG(t, func(string) action { return action{} })
	s := session(f, 5*time.Second)
	s.CancelCommitAfter = time.Second
	defer s.Close()
	o := s.Txn(context.Background(), "serializable", txnAppendRead)
	expect(t, "txn", o, OK, "", "")
	if o.Attempt.CancelRequest != "" {
		t.Errorf("cancel request %q for a fast COMMIT", o.Attempt.CancelRequest)
	}
	g := newFakePG(t, on("commit", action{delay: 500 * time.Millisecond, code: "57014", message: "canceling statement due to user request"}))
	s2 := session(g, 5*time.Second)
	s2.CancelCommitAfter = 100 * time.Millisecond
	defer s2.Close()
	o = s2.Txn(context.Background(), "serializable", txnAppendRead)
	expect(t, "txn cancelled", o, Info, "57014", PhaseCommit)
	if o.Attempt.CancelRequest != "sent" {
		t.Errorf("cancel request %q", o.Attempt.CancelRequest)
	}
}

// Probes: a rejection carries its SQLSTATE (0A000 before anything executed:
// fail, §6.2 row 5); an accepted assignment is ok with its command tag;
// SHOW returns the setting.
func TestProbe(t *testing.T) {
	f := newFakePG(t, on("set synchronous_commit", action{code: "0A000", message: "setting synchronous_commit is not supported"}))
	s := session(f, 5*time.Second)
	defer s.Close()
	expect(t, "P1 rejected", s.Probe(context.Background(), "SET synchronous_commit = off"), Fail, "0A000", PhaseStatement)
	o := s.Probe(context.Background(), "SHOW synchronous_commit")
	expect(t, "show", o, OK, "", "")
	if o.Value.(map[string]any)["setting"] != "on" {
		t.Errorf("show value %v", o.Value)
	}
	g := newFakePG(t, func(string) action { return action{} })
	s2 := session(g, 5*time.Second)
	defer s2.Close()
	o = s2.Probe(context.Background(), "SET synchronous_commit = off")
	expect(t, "P1 accepted", o, OK, "", "")
	if o.Value.(map[string]any)["command_tag"] != "SET" {
		t.Errorf("command tag %v", o.Value)
	}
}
