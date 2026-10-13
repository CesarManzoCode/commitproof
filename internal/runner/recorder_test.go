package runner

import (
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
