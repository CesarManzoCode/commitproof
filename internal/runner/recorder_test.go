package runner

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/workload"
)

func TestRecorderClosesOpenOpsAsInfo(t *testing.T) {
	dir := t.TempDir()
	h, _ := history.Create(filepath.Join(dir, "h.jsonl"), time.Now())
	o, _ := history.Create(filepath.Join(dir, "o.jsonl"), time.Now())
	r := &recorder{h: h, o: o, open: map[int64]*openOp{}}
	a, err := r.invoke(0, 0, "gw-z1", "ledger", "insert", map[string]any{"id": 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := r.invoke(1, 1, "gw-z2", "ledger", "insert", map[string]any{"id": 2}, "")
	if !r.complete(a, workload.Outcome{Type: workload.OK, Value: map[string]any{"id": 1}}, "") {
		t.Fatal("completion of an open op dropped")
	}
	if n := r.closeOpen(); n != 1 {
		t.Fatalf("closed %d ops, want 1", n)
	}
	if r.complete(b, workload.Outcome{Type: workload.OK}, "") {
		t.Fatal("late completion written after the op was closed as info")
	}
	h.Close()
	o.Close()
	evs, err := history.ReadChain(filepath.Join(dir, "h.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if p := history.WellFormed(evs); len(p) != 0 {
		t.Fatalf("history not well formed: %v", p)
	}
	last := evs[len(evs)-1]
	if last.Type != history.TypeInfo || *last.Op != b || last.Err["class"] != "timeout" {
		t.Fatalf("open op not closed as info: %+v", last)
	}
	obs, _ := history.ReadChain(filepath.Join(dir, "o.jsonl"))
	if len(obs) != 1 {
		t.Fatalf("late completion not observed: %d observations", len(obs))
	}
}

// fakeGateway answers the runner's recovery inserts and final reads: the
// first infos recovery inserts time out (info), every later operation is ok
// with the same values.
type fakeGateway struct {
	infos   int
	inserts int
}

func (f *fakeGateway) Ledger(_ context.Context, v map[string]any) workload.Outcome {
	f.inserts++
	if f.inserts <= f.infos {
		return workload.Outcome{Type: workload.Info, Err: map[string]any{"class": "timeout", "phase": "client-timeout", "sqlstate": ""}}
	}
	return workload.Outcome{Type: workload.OK, Value: v}
}

func (f *fakeGateway) FinalRead(_ context.Context, wl string) workload.Outcome {
	return workload.Outcome{Type: workload.OK, Value: []any{wl}}
}

// Regression (K7, C2 seed 3 INVALID 3×): a recovery insert that ends info
// retires the runner's client process; the final reads through the same
// gateway must not invoke on it again (§6.1). Before the fix the final
// reads restarted on process 10 + g, and verify-seal rejected the history
// ("proc 10 invokes after an info completion").
func TestRecoveryInfoRetiresProcessForFinalReads(t *testing.T) {
	dir := t.TempDir()
	h, _ := history.Create(filepath.Join(dir, "h.jsonl"), time.Now())
	o, _ := history.Create(filepath.Join(dir, "o.jsonl"), time.Now())
	r := &recorder{h: h, o: o, open: map[int64]*openOp{}}
	r.nextProc.Store(13)
	fg := &fakeGateway{infos: 1}
	proc := finalClient(0)
	if !recoverGateway(r, 0, fg, &proc) {
		t.Fatal("recovery did not succeed on the second insert")
	}
	if proc == finalClient(0) {
		t.Fatalf("process %d not retired after an info recovery insert", proc)
	}
	if !finalReads(r, 0, fg, &proc) {
		t.Fatal("final reads not stable")
	}
	h.Close()
	o.Close()
	evs, err := history.ReadChain(filepath.Join(dir, "h.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if p := history.WellFormed(evs); len(p) != 0 {
		t.Fatalf("history not well formed: %v", p)
	}
	for _, ev := range evs {
		if ev.Type == history.TypeInvoke && ev.F == "final-read" && ev.Proc != nil && *ev.Proc == finalClient(0) {
			t.Fatalf("final read i=%d invoked on the retired process %d", ev.I, *ev.Proc)
		}
	}
}

// Fresh processes start above every planned process number (K5 clients use
// their client numbers; K1–K4 plans keep 13).
func TestFirstFreshProc(t *testing.T) {
	k4 := []any{map[string]any{"client": int64(0)}, map[string]any{"client": int64(9)}}
	if got := firstFreshProc(k4); got != 13 {
		t.Errorf("K1–K4 clients: first fresh process %d, want 13", got)
	}
	c9 := append(k4, map[string]any{"client": int64(28), "mode": "listener"})
	if got := firstFreshProc(c9); got != 29 {
		t.Errorf("C9 clients: %d, want 29", got)
	}
	c11 := append(k4, map[string]any{"client": int64(100), "mode": "storm", "connections": int64(200)})
	if got := firstFreshProc(c11); got != 300 {
		t.Errorf("C11 clients: %d, want 300", got)
	}
	d5 := []any{map[string]any{"client": int64(8)}, map[string]any{"client": int64(29), "mode": "consumer"}}
	if got := firstFreshProc(d5); got != 30 {
		t.Errorf("D5 clients: %d, want 30", got)
	}
}
