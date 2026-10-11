package gate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Outputs of cpgate inside the evidence directory; never inputs.
var outputs = map[string]bool{"gate.json": true, "negative-controls.json": true}

// Result of one criterion.
type Result struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Result   string   `json:"result"`
	Evidence string   `json:"evidence"`
	SHA256   string   `json:"sha256"`
	Problems []string `json:"problems"`
	Details  []string `json:"details"`
}

// Evaluate runs every criterion on dir.
func Evaluate(dir string, crits []Criterion) []Result {
	e := LoadEvidence(dir)
	defer e.Close()
	out := make([]Result, 0, len(crits))
	for _, c := range crits {
		ev, probs, details := c.Eval(e)
		r := Result{ID: c.ID, Name: c.Name, Result: "PASS", Evidence: ev, Problems: probs, Details: details}
		if r.Problems == nil {
			r.Problems = []string{}
		}
		if r.Details == nil {
			r.Details = []string{}
		}
		if len(probs) > 0 {
			r.Result = "FAIL"
		}
		if ev != "" {
			r.SHA256 = FileSHA256(filepath.Join(dir, ev))
		}
		out = append(out, r)
	}
	return out
}

// Control is the outcome of one negative control.
type Control struct {
	Criterion        string   `json:"criterion"`
	Sabotage         string   `json:"sabotage"`
	Failed           []string `json:"failed"`
	FailedOnlyTarget bool     `json:"failed_only_target"`
	Error            string   `json:"error,omitempty"`
	TargetProblems   []string `json:"target_problems"`
}

// NegativeControls copies dir once per criterion, applies that criterion's
// sabotage, re-evaluates every criterion and requires that exactly the
// target fails (CONTRACT §15).
func NegativeControls(dir string, crits []Criterion) []Control {
	var out []Control
	for _, c := range crits {
		ctl := Control{Criterion: c.ID, Sabotage: c.Sabotage, Failed: []string{}, TargetProblems: []string{}}
		tmp, err := os.MkdirTemp("", "cpgate-"+c.ID+"-")
		if err != nil {
			ctl.Error = err.Error()
			out = append(out, ctl)
			continue
		}
		func() {
			defer removeAll(tmp)
			if err := copyDir(dir, tmp); err != nil {
				ctl.Error = "copy: " + err.Error()
				return
			}
			if err := c.Apply(tmp); err != nil {
				ctl.Error = "sabotage: " + err.Error()
				return
			}
			for _, r := range Evaluate(tmp, crits) {
				if r.Result != "PASS" {
					ctl.Failed = append(ctl.Failed, r.ID)
					if r.ID == c.ID {
						ctl.TargetProblems = r.Problems
					}
				}
			}
		}()
		ctl.FailedOnlyTarget = ctl.Error == "" && len(ctl.Failed) == 1 && ctl.Failed[0] == c.ID
		out = append(out, ctl)
	}
	return out
}

func removeAll(dir string) {
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil {
			os.Chmod(p, 0o755)
		}
		return nil
	})
	os.RemoveAll(dir)
}

// copyDir copies regular files (writable copies; sealed runs are 0444).
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel != "." && outputs[filepath.ToSlash(rel)] {
			return nil
		}
		t := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(t, 0o755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file", rel)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		o, err := os.OpenFile(t, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(o, in); err != nil {
			o.Close()
			return err
		}
		return o.Close()
	})
}

func editJSON(p string, f func(map[string]any)) error {
	m, err := readJSON(p)
	if err != nil {
		return err
	}
	f(m)
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

// editRecords rewrites commands.jsonl through f.
func editRecords(dir string, f func(map[string]any)) error {
	p := filepath.Join(dir, "commands.jsonl")
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	for _, l := range bytes.Split(bytes.TrimRight(b, "\n"), []byte("\n")) {
		var m map[string]any
		dec := json.NewDecoder(bytes.NewReader(l))
		dec.UseNumber()
		if err := dec.Decode(&m); err != nil {
			return err
		}
		f(m)
		nb, err := json.Marshal(m)
		if err != nil {
			return err
		}
		out.Write(nb)
		out.WriteByte('\n')
	}
	return os.WriteFile(p, out.Bytes(), 0o644)
}

// editSummary rewrites the JSON summary line of a recorded command's stdout
// and updates the recorded hash, so the sabotage is internally consistent
// and only the criterion's own logic can catch it.
func editSummary(dir, name string, f func(map[string]any)) error {
	e := LoadEvidence(dir)
	r, out, err := e.Get(name)
	if err != nil {
		return err
	}
	lines := bytes.Split(bytes.TrimRight(out, "\n"), []byte("\n"))
	m, err := LastJSONLine(out)
	if err != nil {
		return err
	}
	f(m)
	nb, err := json.Marshal(m)
	if err != nil {
		return err
	}
	lines[len(lines)-1] = nb
	nout := append(bytes.Join(lines, []byte("\n")), '\n')
	if err := os.WriteFile(e.Path(r.Stdout), nout, 0o644); err != nil {
		return err
	}
	return editRecords(dir, func(rec map[string]any) {
		if rec["name"] == name {
			rec["stdout_sha256"] = sha(nout)
		}
	})
}

func sabotageMemory(dir string) error {
	p := filepath.Join(dir, "memory-samples.jsonl")
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	var lines [][]byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<22)
	for sc.Scan() {
		lines = append(lines, append([]byte{}, sc.Bytes()...))
	}
	f.Close()
	// The middle valid sample: inside the run window by construction of the
	// sampling (it starts before and ends after the run).
	var valid []int
	for i, l := range lines {
		if bytes.Contains(l, []byte(`"mem_usage"`)) && !bytes.Contains(l, []byte(`"error"`)) {
			valid = append(valid, i)
		}
	}
	if len(valid) == 0 {
		return fmt.Errorf("no valid sample to sabotage")
	}
	i := valid[len(valid)/2]
	var m map[string]any
	if err := json.Unmarshal(lines[i], &m); err != nil {
		return err
	}
	cs, _ := m["containers"].([]any)
	c0, _ := cs[0].(map[string]any)
	c0["mem_usage"] = fmt.Sprintf("%dGiB / 15.53GiB", params.MemoryBudgetGiB+1)
	nb, err := json.Marshal(m)
	if err != nil {
		return err
	}
	lines[i] = nb
	return os.WriteFile(p, append(bytes.Join(lines, []byte("\n")), '\n'), 0o644)
}

func sabotageHistory(dir string) error {
	e := LoadEvidence(dir)
	rd, _, p := smokeRunDir(e)
	if len(p) > 0 {
		return fmt.Errorf("%s", strings.Join(p, "; "))
	}
	hp := filepath.Join(rd, history.FileHistory)
	b, err := os.ReadFile(hp)
	if err != nil {
		return err
	}
	b[len(b)/2] ^= 0x01
	return os.WriteFile(hp, b, 0o644)
}

// GateFile is gate.json.
type GateFile struct {
	Sprint          string   `json:"sprint"`
	ContractVersion string   `json:"contract_version"`
	Evidence        string   `json:"evidence"`
	Result          string   `json:"result"`
	Criteria        []Result `json:"criteria"`
}

// Passed reports whether every criterion passed.
func Passed(rs []Result) bool {
	for _, r := range rs {
		if r.Result != "PASS" {
			return false
		}
	}
	return true
}

// ControlsPassed reports whether every control failed only its target.
func ControlsPassed(cs []Control) bool {
	for _, c := range cs {
		if !c.FailedOnlyTarget {
			return false
		}
	}
	return len(cs) > 0
}
