package workload

// G10's harness review (K5, owner's decision of 2026-10-14): the W6 listener
// is not the cause of the lost notifications if it keeps receiving on its
// session, and, when the session breaks, reconnects, issues LISTEN again and
// receives what is notified afterwards (CONTRACT §7 W6; ADR-0030). A fake
// server sends one notification per session and breaks the first session
// on request; the test drives Listen and Wait as runner.runListener does.

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"
)

type notifyPG struct {
	ln       net.Listener
	mu       sync.Mutex
	sessions int   // StartupMessage connections so far
	cancels  int   // CancelRequest connections (pgx closing a broken session)
	listens  []int // session number of every LISTEN received
	breakUp  chan struct{}
	wg       sync.WaitGroup
}

func newNotifyPG(t *testing.T) *notifyPG {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &notifyPG{ln: ln, breakUp: make(chan struct{})}
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			f.wg.Add(1)
			go func() { defer f.wg.Done(); f.session(c) }()
		}
	}()
	t.Cleanup(func() { ln.Close(); f.wg.Wait() })
	return f
}

// session answers startup and LISTEN; after LISTEN it notifies the payload
// "<n>" (its session number, counting StartupMessage connections). Session 1
// is then closed once breakUp is closed; later sessions stay open until the
// client leaves. A CancelRequest connection is counted and closed.
func (f *notifyPG) session(c net.Conn) {
	defer c.Close()
	be := pgproto3.NewBackend(c, c)
	sm, err := be.ReceiveStartupMessage()
	if err != nil {
		return
	}
	f.mu.Lock()
	if _, ok := sm.(*pgproto3.StartupMessage); !ok {
		f.cancels++
		f.mu.Unlock()
		return
	}
	f.sessions++
	n := f.sessions
	f.mu.Unlock()
	be.Send(&pgproto3.AuthenticationOk{})
	for k, v := range map[string]string{"server_version": "17.7", "client_encoding": "UTF8", "standard_conforming_strings": "on",
		"DateStyle": "ISO, MDY", "integer_datetimes": "on", "TimeZone": "UTC"} {
		be.Send(&pgproto3.ParameterStatus{Name: k, Value: v})
	}
	be.Send(&pgproto3.BackendKeyData{ProcessID: uint32(n), SecretKey: []byte{0, 0, 0, 1}})
	be.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	if be.Flush() != nil {
		return
	}
	done := make(chan struct{})
	defer close(done)
	if n == 1 {
		go func() {
			select {
			case <-f.breakUp:
				c.Close() // the session breaks, as at a gateway or pooler restart
			case <-done:
			}
		}()
	}
	for {
		msg, err := be.Receive()
		if err != nil {
			return
		}
		switch m := msg.(type) {
		case *pgproto3.Query:
			q := strings.ToLower(strings.TrimSpace(m.String))
			be.Send(&pgproto3.CommandComplete{CommandTag: []byte(strings.ToUpper(strings.Fields(q)[0]))})
			be.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
			if strings.HasPrefix(q, "listen ") {
				f.mu.Lock()
				f.listens = append(f.listens, n)
				f.mu.Unlock()
				be.Send(&pgproto3.NotificationResponse{PID: uint32(n), Channel: "cp_chan", Payload: strconv.Itoa(n)})
			}
		case *pgproto3.Terminate:
			return
		}
		if be.Flush() != nil {
			return
		}
	}
}

func TestListenerRelistensAfterReconnect(t *testing.T) {
	f := newNotifyPG(t)
	l := &Listener{GW: "127.0.0.1", Port: f.ln.Addr().(*net.TCPAddr).Port, Timeout: 2 * time.Second}
	defer l.Close()
	ctx := context.Background()
	// listen is runner.runListener's step for a listener without a
	// connection: LISTEN on a new connection.
	listen := func() {
		t.Helper()
		if l.Connected() {
			t.Fatal("listen with a live connection")
		}
		if o := l.Listen(ctx, "cp_chan"); o.Type != OK {
			t.Fatalf("LISTEN: %s %v", o.Type, o.Err)
		}
	}
	got := func(o Outcome) []any {
		t.Helper()
		if o.Type != OK {
			t.Fatalf("wait: %s %v", o.Type, o.Err)
		}
		v, _ := o.Value.([]any)
		return v
	}
	listen()
	if v := got(l.Wait(ctx, 300*time.Millisecond)); len(v) != 1 || v[0] != int64(1) {
		t.Fatalf("session 1 received %v, want [1]", v)
	}
	// Waits on an idle, open session end normally at their own deadline.
	if v := got(l.Wait(ctx, 200*time.Millisecond)); len(v) != 0 || !l.Connected() {
		t.Fatalf("idle wait: %v, connected %v", v, l.Connected())
	}
	close(f.breakUp)
	if o := l.Wait(ctx, 2*time.Second); o.Type == OK || l.Connected() {
		t.Fatalf("wait on the broken session: %s, connected %v", o.Type, l.Connected())
	}
	listen()
	if v := got(l.Wait(ctx, 300*time.Millisecond)); len(v) != 1 || v[0] != int64(2) {
		t.Fatalf("session 2 received %v, want [2]", v)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.listens) != 2 || f.listens[0] != 1 || f.listens[1] != 2 || f.sessions != 2 {
		t.Fatalf("LISTEN per session %v over %d sessions, want [1 2] over 2", f.listens, f.sessions)
	}
}
