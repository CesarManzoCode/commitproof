// Package campaign drives matrices of runs, minimization and replays from
// the host (CONTRACT §5.6 `commitproof campaign|minimize|replay`, §12.2,
// §11.1). It is resumable: every run it needs is looked up among the sealed
// run directories already present before anything is run, so an
// interrupted campaign continues where it stopped and a sealed run is never
// recomputed.
//
// It produces no verdict of its own: whether a run is INVALID, PASS or FAIL
// and which anomaly classes it shows come from `cpcheck check` (a separate
// binary, executed as a subprocess) through the checks.json it writes next
// to each run (CONTRACT §5.3). cpgate re-derives everything from the sealed
// runs and those files.
package campaign

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/cluster"
	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/plan"
)

// MaxAttempts: an INVALID run is re-run with the same seed; INVALID three
// times is a HARNESS_DEFECT to fix (CONTRACT §12.1).
const MaxAttempts = 3

// Provenance keys written into the manifest of derived runs.
const (
	KeyReplayOf    = "replay_of"
	KeyReplayIndex = "replay_index"
	KeyCandidateOf = "candidate_of"
)

// Driver runs plans into Out and checks them with CPCheck.
type Driver struct {
	Root    string // repository root (cluster commands run there)
	Out     string // runs directory, as given on the command line
	CPCheck string // cpcheck binary
	Log     io.Writer
	// RunTimeout bounds one cpcheck invocation (P11 + P12 per partition
	// can add up; ten P09 is generous and still finite).
	CheckTimeout time.Duration
}

// Run is a sealed run directory with cpcheck's view of it.
type Run struct {
	ID       string
	Dir      string // as Out/ID
	Manifest map[string]any
	PlanSHA  string
	Seal     string // seal.json Merkle root
	Verdict  string // cpcheck: PASS, FAIL, UNKNOWN, INVALID
	Classes  []string
	Checks   string // checks.json path
	Anoms    []Anomaly
}

// Anomaly is one anomaly of an invalid checker in checks.json.
type Anomaly struct {
	Checker string
	Class   string
	Ops     []int64
}

func (d *Driver) emit(v map[string]any) {
	if d.Log == nil {
		return
	}
	b, err := canonjson.Marshal(v)
	if err != nil {
		b = []byte(fmt.Sprintf(`{"event":"error","error":%q}`, err.Error()))
	}
	fmt.Fprintln(d.Log, string(b))
}

// Lock takes an exclusive lock on Out (one campaign, one cluster at a time,
// AGENTS.md §5). The returned function releases it.
func (d *Driver) Lock() (func(), error) {
	if err := os.MkdirAll(d.Out, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(d.Out, ".campaign.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another campaign holds %s: %w", f.Name(), err)
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}

// runs lists the sealed run directories in Out. Directories that are not
// sealed (a crash before sealing) are moved under Out/.unsealed/ so they are
// neither counted nor mistaken for runs.
func (d *Driver) runs() ([]*Run, error) {
	ents, err := os.ReadDir(d.Out)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Run
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(d.Out, e.Name())
		if _, err := os.Stat(filepath.Join(dir, history.FileSeal)); err != nil {
			if _, err := os.Stat(filepath.Join(dir, history.FileManifest)); err == nil {
				os.MkdirAll(filepath.Join(d.Out, ".unsealed"), 0o755)
				if err := os.Rename(dir, filepath.Join(d.Out, ".unsealed", e.Name())); err == nil {
					d.emit(map[string]any{"event": "unsealed-moved", "run": e.Name()})
				}
			}
			continue
		}
		m, err := readJSON(filepath.Join(dir, history.FileManifest))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		seal, err := readJSON(filepath.Join(dir, history.FileSeal))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		r := &Run{ID: e.Name(), Dir: dir, Manifest: m, PlanSHA: str(m, "plan_sha256"), Seal: str(seal, "merkle_root")}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func readJSON(p string) (map[string]any, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

// Check makes sure r has cpcheck's checks.json for its current seal (running
// `cpcheck check` when absent or stale) and loads verdict and classes.
func (d *Driver) Check(ctx context.Context, r *Run) error {
	r.Checks = r.Dir + ".checks.json"
	cj, err := readJSON(r.Checks)
	if err != nil || str(cj, "seal_root") != r.Seal || r.Seal == "" {
		t := d.CheckTimeout
		if t == 0 {
			t = 10 * params.HealthyDeadline
		}
		cctx, cancel := context.WithTimeout(ctx, t)
		cmd := exec.CommandContext(cctx, d.CPCheck, "check", r.Dir)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		rerr := cmd.Run()
		cancel()
		code := 0
		if ee, ok := rerr.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if rerr != nil {
			return fmt.Errorf("cpcheck %s: %w", r.Dir, rerr)
		}
		d.emit(map[string]any{"event": "cpcheck", "run": r.ID, "exit_code": int64(code), "summary": lastLine(stdout.Bytes())})
		if code == 3 {
			return fmt.Errorf("cpcheck %s: usage error: %s", r.Dir, strings.TrimSpace(stderr.String()))
		}
		if cj, err = readJSON(r.Checks); err != nil {
			return fmt.Errorf("cpcheck %s wrote no checks.json: %w", r.Dir, err)
		}
	}
	if str(cj, "seal_root") != r.Seal {
		return fmt.Errorf("%s: checks.json seal %s, run seal %s", r.ID, str(cj, "seal_root"), r.Seal)
	}
	r.Verdict = str(cj, "verdict")
	r.Classes, r.Anoms = nil, nil
	seen := map[string]bool{}
	cs, _ := cj["checkers"].([]any)
	for _, x := range cs {
		c, _ := x.(map[string]any)
		if fmt.Sprint(c["valid"]) != "false" {
			continue
		}
		as, _ := c["anomalies"].([]any)
		for _, y := range as {
			a, _ := y.(map[string]any)
			cl := str(a, "class")
			var ops []int64
			ol, _ := a["ops"].([]any)
			for _, o := range ol {
				if n, ok := o.(json.Number); ok {
					if v, err := n.Int64(); err == nil {
						ops = append(ops, v)
					}
				}
			}
			r.Anoms = append(r.Anoms, Anomaly{Checker: str(c, "id"), Class: cl, Ops: ops})
			if !seen[cl] {
				seen[cl] = true
				r.Classes = append(r.Classes, cl)
			}
		}
	}
	return nil
}

func lastLine(b []byte) string {
	l := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
	return string(l[len(l)-1])
}

func (r *Run) has(class string) bool {
	for _, c := range r.Classes {
		if c == class {
			return true
		}
	}
	return false
}

// Spec identifies the runs a request accepts.
type Spec struct {
	Plan    *plan.Plan
	Profile string
	Mutant  string // "" = real
	Prov    map[string]any
}

func mutantName(m string) string {
	if m == "" {
		return "real"
	}
	return m
}

// matches reports whether r is a run of s: same plan bytes, profile, build
// and provenance (a plain run has none).
func (s Spec) matches(r *Run, planSHA string) bool {
	if r.PlanSHA != planSHA || str(r.Manifest, "profile") != s.Profile || str(r.Manifest, "mutant") != mutantName(s.Mutant) {
		return false
	}
	for _, k := range []string{KeyReplayOf, KeyReplayIndex, KeyCandidateOf} {
		want, has := s.Prov[k]
		got, gotHas := r.Manifest[k]
		if has != gotHas || (has && fmt.Sprint(want) != fmt.Sprint(got)) {
			return false
		}
	}
	return true
}

// Ensure returns a valid (not INVALID) run of s, running it if no sealed one
// exists; INVALID runs are re-run up to MaxAttempts in total. It returns the
// last run and false when every attempt was INVALID.
func (d *Driver) Ensure(ctx context.Context, s Spec) (*Run, []*Run, bool, error) {
	sha, err := s.Plan.SHA256()
	if err != nil {
		return nil, nil, false, err
	}
	var invalid []*Run
	for {
		all, err := d.runs()
		if err != nil {
			return nil, invalid, false, err
		}
		invalid = invalid[:0]
		for _, r := range all {
			if !s.matches(r, sha) {
				continue
			}
			if err := d.Check(ctx, r); err != nil {
				return nil, invalid, false, err
			}
			if r.Verdict != "INVALID" {
				return r, invalid, true, nil
			}
			invalid = append(invalid, r)
		}
		if len(invalid) >= MaxAttempts {
			return invalid[len(invalid)-1], invalid, false, nil
		}
		if ctx.Err() != nil {
			return nil, invalid, false, ctx.Err()
		}
		sum, code := cluster.RunPlan(ctx, d.Root, s.Profile, cluster.RunOpts{Mutant: s.Mutant, Provenance: s.Prov}, s.Plan, d.Out)
		d.emit(map[string]any{"event": "run", "summary": sum, "exit_code": int64(code)})
		if code == 3 {
			return nil, invalid, false, fmt.Errorf("run: prerequisite error: %v", sum["error"])
		}
		if _, ok := sum["run_dir"]; !ok {
			return nil, invalid, false, fmt.Errorf("run produced no run directory: %v", sum["error"])
		}
	}
}

// TargetClass is the anomaly class a failing run is minimized and replayed
// for: the directed schedule's expected class when the run shows it (§9.2:
// D1/D2/D2′ → lost-write; the probe suite → p1-not-rejected; ADR-0030: D3 →
// duplicate-elements, else g1a; D5 → cdc-phantom; ADR-0033: D3′ as D3),
// else the first class of the first invalid checker (checks.json order).
func TargetClass(r *Run) string {
	expected := map[string][]string{"D1": {"lost-write"}, "D2": {"lost-write"}, "D2p": {"lost-write"}, "DH1": {"lost-write"},
		plan.ProbeSuite: {"p1-not-rejected"},
		// ADR-0030: D3 → duplicate-elements if present, else g1a; D5 → cdc-phantom.
		// ADR-0033: D3′ as D3.
		"D3": {"duplicate-elements", "g1a"}, "D3p": {"duplicate-elements", "g1a"}, "D5": {"cdc-phantom"}}
	for _, c := range expected[str(r.Manifest, "campaign")] {
		if r.has(c) {
			return c
		}
	}
	if len(r.Classes) > 0 {
		return r.Classes[0]
	}
	return ""
}

// PlanOf decodes a run's plan.json.
func PlanOf(r *Run) (*plan.Plan, error) {
	b, err := os.ReadFile(filepath.Join(r.Dir, history.FilePlan))
	if err != nil {
		return nil, err
	}
	return plan.Decode(b)
}

// Find returns the sealed run whose directory is dir (as given or under
// Out).
func (d *Driver) Find(dir string) (*Run, error) {
	all, err := d.runs()
	if err != nil {
		return nil, err
	}
	base := filepath.Base(filepath.Clean(dir))
	for _, r := range all {
		if r.ID == base {
			return r, nil
		}
	}
	return nil, fmt.Errorf("%s is not a sealed run under %s", dir, d.Out)
}

// Replay runs the plan of r times more (replay_index 1…times), each a valid
// run (CONTRACT §11.1, P13). It returns the replays and how many show
// class.
func (d *Driver) Replay(ctx context.Context, r *Run, times int, class string) ([]*Run, int, error) {
	p, err := PlanOf(r)
	if err != nil {
		return nil, 0, err
	}
	var out []*Run
	hits := 0
	for i := 1; i <= times; i++ {
		s := Spec{Plan: p, Profile: str(r.Manifest, "profile"), Mutant: mutantOf(r), Prov: map[string]any{KeyReplayOf: r.ID, KeyReplayIndex: int64(i)}}
		rr, inv, ok, err := d.Ensure(ctx, s)
		if err != nil {
			return out, hits, err
		}
		d.emit(map[string]any{"event": "replay", "of": r.ID, "index": int64(i), "run": idOf(rr), "verdict": verdictOf(rr), "valid": ok,
			"invalid_attempts": ids(inv), "shows_class": ok && rr.has(class), "class": class})
		if !ok {
			return out, hits, fmt.Errorf("replay %d of %s INVALID %d times", i, r.ID, MaxAttempts)
		}
		out = append(out, rr)
		if rr.Verdict == "FAIL" && rr.has(class) {
			hits++
		}
	}
	return out, hits, nil
}

func mutantOf(r *Run) string {
	if m := str(r.Manifest, "mutant"); m != "real" {
		return m
	}
	return ""
}

func idOf(r *Run) string {
	if r == nil {
		return ""
	}
	return r.ID
}

func verdictOf(r *Run) string {
	if r == nil {
		return ""
	}
	return r.Verdict
}

func ids(rs []*Run) []any {
	out := []any{}
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

// Reproducibility is the §11.1 class of k hits out of P13 replays.
func Reproducibility(hits int) string {
	switch {
	case hits >= params.ReproducedAtLeast:
		return "REPRODUCED"
	case hits >= 1:
		return "INTERMITTENT"
	}
	return "NOT_REPRODUCED"
}
