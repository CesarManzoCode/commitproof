package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeRun(t *testing.T, dir string, events []map[string]any) {
	t.Helper()
	h, err := Create(filepath.Join(dir, FileHistory), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if _, _, err := h.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	o, err := Create(filepath.Join(dir, FileObservations), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := o.Append(map[string]any{"type": TypeObs, "value": map[string]any{"src": "test"}}); err != nil {
		t.Fatal(err)
	}
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{FileManifest, FilePlan} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, FileCommands), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, DirLogs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, DirLogs, "x.log"), []byte("log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func goodEvents() []map[string]any {
	return []map[string]any{
		{"type": TypeMarker, "value": map[string]any{"name": "start"}},
		{"type": TypeInvoke, "proc": 0, "client": 0, "gw": "gw-z1", "wl": "ledger", "f": "insert", "value": map[string]any{"id": 1}},
		{"type": TypeOK, "proc": 0, "client": 0, "gw": "gw-z1", "wl": "ledger", "f": "insert", "op": 1, "value": map[string]any{"id": 1}},
		{"type": TypeInvoke, "proc": 0, "client": 0, "gw": "gw-z1", "wl": "ledger", "f": "insert", "value": map[string]any{"id": 2}},
		{"type": TypeInfo, "proc": 0, "client": 0, "gw": "gw-z1", "wl": "ledger", "f": "insert", "op": 3, "value": map[string]any{"id": 2},
			"err": map[string]any{"sqlstate": "", "class": "transport", "phase": "statement"}},
	}
}

func TestSealRoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeRun(t, dir, goodEvents())
	s, err := SealDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	r := VerifySeal(dir)
	if !r.Valid {
		t.Fatalf("fresh seal invalid: %v", r.Problems)
	}
	if r.MerkleRoot != s.MerkleRoot || r.HistoryLines != 5 || r.Files != 6 {
		t.Fatalf("unexpected report %+v", r)
	}
	st, _ := os.Stat(filepath.Join(dir, FileHistory))
	if st.Mode().Perm() != 0o444 {
		t.Fatalf("history mode %v", st.Mode().Perm())
	}
	if _, err := SealDir(dir); err == nil {
		t.Fatal("resealing accepted")
	}
	// make writable again so TempDir cleanup works
	t.Cleanup(func() {
		filepath.WalkDir(dir, func(p string, d os.DirEntry, _ error) error { return os.Chmod(p, 0o755) })
	})
}

func TestTamperDetected(t *testing.T) {
	for name, tamper := range map[string]func(dir string){
		"flip history byte": func(dir string) {
			p := filepath.Join(dir, FileHistory)
			b, _ := os.ReadFile(p)
			b[10] ^= 1
			os.Chmod(p, 0o644)
			os.WriteFile(p, b, 0o644)
		},
		"extra file": func(dir string) {
			os.Chmod(dir, 0o755)
			os.WriteFile(filepath.Join(dir, "extra"), []byte("x"), 0o644)
		},
		"log changed": func(dir string) {
			p := filepath.Join(dir, DirLogs, "x.log")
			os.Chmod(p, 0o644)
			os.WriteFile(p, []byte("LOG\n"), 0o644)
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Cleanup(func() {
				filepath.WalkDir(dir, func(p string, d os.DirEntry, _ error) error { return os.Chmod(p, 0o755) })
			})
			writeRun(t, dir, goodEvents())
			if _, err := SealDir(dir); err != nil {
				t.Fatal(err)
			}
			tamper(dir)
			if r := VerifySeal(dir); r.Valid {
				t.Fatal("tampering not detected")
			}
		})
	}
}

func TestChainRewriteDetectedEvenWithResealedFile(t *testing.T) {
	// A line edited consistently with a recomputed seal still breaks the chain
	// of the next line.
	dir := t.TempDir()
	writeRun(t, dir, goodEvents())
	p := filepath.Join(dir, FileHistory)
	b, _ := os.ReadFile(p)
	lines := strings.Split(string(b), "\n")
	lines[1] = strings.Replace(lines[1], `"id":1`, `"id":7`, 1)
	os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0o644)
	if _, err := ReadChain(p); err == nil || !strings.Contains(err.Error(), "chain broken") {
		t.Fatalf("want chain error, got %v", err)
	}
}

func TestWellFormed(t *testing.T) {
	bad := goodEvents()[:4] // last invoke unanswered
	dir := t.TempDir()
	h, _ := Create(filepath.Join(dir, "h"), time.Now())
	for _, e := range bad {
		h.Append(e)
	}
	h.Close()
	evs, err := ReadChain(filepath.Join(dir, "h"))
	if err != nil {
		t.Fatal(err)
	}
	if p := WellFormed(evs); len(p) != 1 || !strings.Contains(p[0], "no completion") {
		t.Fatalf("problems = %v", p)
	}
	// invoke after info retirement
	ok := goodEvents()
	ok = append(ok, map[string]any{"type": TypeInvoke, "proc": 0, "client": 0, "gw": "gw-z1", "wl": "ledger", "f": "insert", "value": map[string]any{"id": 3}})
	h2, _ := Create(filepath.Join(dir, "h2"), time.Now())
	for _, e := range ok {
		h2.Append(e)
	}
	h2.Close()
	evs3, _ := ReadChain(filepath.Join(dir, "h2"))
	found := false
	for _, p := range WellFormed(evs3) {
		if strings.Contains(p, "after an info") {
			found = true
		}
	}
	if !found {
		t.Fatalf("retired proc reuse not detected: %v", WellFormed(evs3))
	}
}

func TestWriterRejectsReservedFields(t *testing.T) {
	h, _ := Create(filepath.Join(t.TempDir(), "h"), time.Now())
	defer h.Close()
	if _, _, err := h.Append(map[string]any{"type": TypeMarker, "t": 5}); err == nil {
		t.Fatal("accepted t")
	}
	if _, _, err := h.Append(map[string]any{"type": TypeMarker, "value": 1.5}); err == nil {
		t.Fatal("accepted float")
	}
}
