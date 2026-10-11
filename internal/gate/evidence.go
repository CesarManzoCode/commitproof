// Package gate evaluates sprint gates from evidence files alone and runs the
// evidence-level negative controls (CONTRACT §15, ADR-0017). It must not
// import runner, nemesis, cluster, workload, observation or Multigres
// packages (§5.3, enforced by the independence test).
package gate

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Record is one line of an evidence commands.jsonl (scripts/record-command.sh).
type Record struct {
	Line         int      `json:"-"`
	Name         string   `json:"name"`
	Argv         []string `json:"argv"`
	ExitCode     int      `json:"exit_code"`
	Start        string   `json:"start"`
	End          string   `json:"end"`
	Stdout       string   `json:"stdout"`
	StdoutSHA256 string   `json:"stdout_sha256"`
	Stderr       string   `json:"stderr"`
	StderrSHA256 string   `json:"stderr_sha256"`
}

// Evidence is an evidence directory.
type Evidence struct {
	Dir     string
	Records []Record
	LoadErr error
}

// LoadEvidence reads dir/commands.jsonl.
func LoadEvidence(dir string) *Evidence {
	ev := &Evidence{Dir: dir}
	f, err := os.Open(filepath.Join(dir, "commands.jsonl"))
	if err != nil {
		ev.LoadErr = err
		return ev
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for n := 0; sc.Scan(); n++ {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			ev.LoadErr = fmt.Errorf("commands.jsonl line %d: %w", n+1, err)
			return ev
		}
		r.Line = n
		ev.Records = append(ev.Records, r)
	}
	if err := sc.Err(); err != nil {
		ev.LoadErr = err
	}
	return ev
}

// Path maps a recorded path onto Dir. Records store paths as written by the
// recorder ("evidence/K1/logs/x.stdout", "evidence/K1/runs/<id>"); the gate
// must work on copies of the evidence (negative controls), so the path is
// resolved as the shortest trailing part of it that exists under Dir.
func (e *Evidence) Path(p string) string {
	parts := strings.Split(filepath.ToSlash(p), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		c := filepath.Join(append([]string{e.Dir}, parts[i:]...)...)
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return filepath.Join(e.Dir, p)
}

// Get returns the record named name and checks that its stdout and stderr
// files still hash to the recorded values.
func (e *Evidence) Get(name string) (Record, []byte, error) {
	if e.LoadErr != nil {
		return Record{}, nil, e.LoadErr
	}
	var found []Record
	for _, r := range e.Records {
		if r.Name == name {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		return Record{}, nil, fmt.Errorf("commands.jsonl: %d records named %q, want exactly 1", len(found), name)
	}
	r := found[0]
	out, err := os.ReadFile(e.Path(r.Stdout))
	if err != nil {
		return r, nil, fmt.Errorf("%s stdout: %w", name, err)
	}
	if sha(out) != r.StdoutSHA256 {
		return r, nil, fmt.Errorf("%s: stdout sha256 %s != recorded %s", name, sha(out), r.StdoutSHA256)
	}
	errb, err := os.ReadFile(e.Path(r.Stderr))
	if err != nil {
		return r, nil, fmt.Errorf("%s stderr: %w", name, err)
	}
	if sha(errb) != r.StderrSHA256 {
		return r, nil, fmt.Errorf("%s: stderr sha256 mismatch", name)
	}
	return r, out, nil
}

// Last returns the last record whose name starts with prefix and exited 0.
func (e *Evidence) Last(prefix string) (Record, []byte, error) {
	if e.LoadErr != nil {
		return Record{}, nil, e.LoadErr
	}
	name := ""
	for _, r := range e.Records {
		if strings.HasPrefix(r.Name, prefix) && r.ExitCode == 0 {
			name = r.Name
		}
	}
	if name == "" {
		return Record{}, nil, fmt.Errorf("no successful %q record", prefix)
	}
	return e.Get(name)
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// FileSHA256 hashes a file ("" if unreadable).
func FileSHA256(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return sha(b)
}

// LastJSONLine parses the last non-empty line of out (CONTRACT §5.6 summary).
func LastJSONLine(out []byte) (map[string]any, error) {
	lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(lines[len(lines)-1]))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("summary line: %w", err)
	}
	return m, nil
}

func argvHas(r Record, words ...string) bool {
	j := strings.Join(r.Argv, " ")
	for _, w := range words {
		if !strings.Contains(j, w) {
			return false
		}
	}
	return true
}
