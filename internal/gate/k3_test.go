package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

func ev(typ, gw, wl, f string, v any) history.Event {
	return history.Event{Type: typ, GW: gw, WL: wl, F: f, Value: v}
}

func TestStableFinalReads(t *testing.T) {
	var evs []history.Event
	for _, wl := range []string{"append", "register", "bank", "ledger"} {
		for i := 0; i < 3; i++ {
			evs = append(evs, ev("ok", "gw-z1", wl, "final-read", map[string]any{"x": "1"}))
		}
	}
	if p := stableFinalReads(evs); len(p) != 0 {
		t.Fatalf("stable reads rejected: %v", p)
	}
	evs[len(evs)-1] = ev("ok", "gw-z1", "ledger", "final-read", map[string]any{"x": "2"})
	if p := stableFinalReads(evs); len(p) != 1 || !strings.Contains(p[0], "ledger") {
		t.Fatalf("unstable ledger: %v", p)
	}
	if p := stableFinalReads(nil); len(p) == 0 {
		t.Fatal("no final reads accepted")
	}
}

func TestFullLifecycle(t *testing.T) {
	if !fullLifecycle(&faultTrace{Kind: "F1a", Phases: []string{"inject", "confirmed", "heal", "healed"}}) ||
		!fullLifecycle(&faultTrace{Kind: "F5a", Phases: []string{"inject", "confirmed"}}) ||
		fullLifecycle(&faultTrace{Kind: "F1a", Phases: []string{"inject", "confirmed"}}) ||
		fullLifecycle(&faultTrace{Kind: "F3a", Phases: []string{"inject", "confirmed", "heal", "not-healed"}}) {
		t.Fatal("lifecycle")
	}
}

func TestRunArchiveRoundTrip(t *testing.T) {
	src := filepath.Join(t.TempDir(), "20261011T000000Z-x-1-real-00000000")
	os.MkdirAll(filepath.Join(src, "logs", "cmd"), 0o755)
	os.WriteFile(filepath.Join(src, "history.jsonl"), []byte("h\n"), 0o444)
	os.WriteFile(filepath.Join(src, "logs", "cmd", "a.stdout"), []byte("out"), 0o444)
	arch := src + ArchiveSuffix
	if err := ArchiveRun(src, arch); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	if err := ExtractRun(arch, dst); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dst, filepath.Base(src), "logs", "cmd", "a.stdout"))
	st, _ := os.Stat(filepath.Join(dst, filepath.Base(src), "history.jsonl"))
	if err != nil || string(b) != "out" || st.Mode().Perm() != 0o444 {
		t.Fatalf("extracted %q %v %v", b, err, st.Mode())
	}
	e := &Evidence{Dir: filepath.Dir(src)}
	defer e.Close()
	os.RemoveAll(src)
	d, err := e.RunDir(filepath.Base(src))
	if err != nil || filepath.Base(d) != filepath.Base(src) {
		t.Fatalf("RunDir %s %v", d, err)
	}
}
