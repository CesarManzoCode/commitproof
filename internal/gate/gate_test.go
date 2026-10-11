package gate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitBlobID(t *testing.T) {
	// `printf 'hello\n' | git hash-object --stdin`
	if got := GitBlobID([]byte("hello\n")); got != "ce013625030ba8dba906f756967f9e9ca394464a" {
		t.Fatalf("GitBlobID = %s", got)
	}
}

func TestCheckDerivation(t *testing.T) {
	up := "ARG GO_VERSION=1.26\nFROM golang:${GO_VERSION}-alpine AS builder\nRUN make\nFROM debian:bookworm-slim\nUSER 1\n"
	pin := "@sha256:" + strings.Repeat("a", 64)
	good := "ARG GO_VERSION=1.26\nFROM golang:1.26.9-alpine" + pin + " AS builder\nRUN make\nFROM debian:bookworm-slim" + pin + "\nUSER 1\n"
	if r := CheckDerivation([]byte(up), []byte(good)); !r.OK || len(r.Changed) != 2 {
		t.Fatalf("good derivation rejected: %+v", r)
	}
	for name, bad := range map[string]string{
		"extra RUN":      strings.Replace(good, "RUN make\n", "RUN make\nRUN evil\n", 1),
		"changed RUN":    strings.Replace(good, "RUN make", "RUN make all", 1),
		"stage renamed":  strings.Replace(good, " AS builder", " AS b2", 1),
		"tag not pinned": strings.Replace(good, "debian:bookworm-slim"+pin, "debian:bookworm-slim", 1),
		"latest":         strings.Replace(good, "debian:bookworm-slim"+pin, "debian:latest"+pin, 1),
	} {
		if r := CheckDerivation([]byte(up), []byte(bad)); r.OK {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestParseMem(t *testing.T) {
	for in, want := range map[string]int64{
		"12.5MiB / 15.53GiB": 12.5 * (1 << 20),
		"1GiB / 2GiB":        1 << 30,
		"512KiB / 1GiB":      512 << 10,
		"0B / 0B":            0,
		"7GiB / 15.53GiB":    7 << 30,
	} {
		got, err := ParseMem(in)
		if err != nil || got != want {
			t.Errorf("ParseMem(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := ParseMem("--"); err == nil {
		t.Error("ParseMem accepted garbage")
	}
}

func TestPathResolvesCopies(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "logs"), 0o755)
	os.WriteFile(filepath.Join(dir, "logs", "x.stdout"), nil, 0o644)
	e := &Evidence{Dir: dir}
	if got := e.Path("evidence/K1/logs/x.stdout"); got != filepath.Join(dir, "logs", "x.stdout") {
		t.Fatalf("Path = %s", got)
	}
	if got := e.Path("/elsewhere/K1-dry/logs/x.stdout"); got != filepath.Join(dir, "logs", "x.stdout") {
		t.Fatalf("Path = %s", got)
	}
}
