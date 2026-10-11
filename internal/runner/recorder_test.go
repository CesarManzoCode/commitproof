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
