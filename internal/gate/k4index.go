package gate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// The K4 run index: every run of evidence/K4/runs, committed in full
// (<id>.tar.zst, CONTRACT §13) or as a stub (<id>.stub.tar.zst: manifest,
// plan and seal only; §13's summary of a run not committed in full), with
// cpcheck's checks.json (<id>.checks.json). Built once per evaluation.

// StubSuffix marks a run stub archive.
const StubSuffix = ".stub.tar.zst"

// Provenance keys of derived runs (internal/campaign writes them into the
// manifest; cpgate only reads them).
const (
	k4ReplayOf    = "replay_of"
	k4ReplayIndex = "replay_index"
	k4CandidateOf = "candidate_of"
)

type k4Anom struct {
	Checker string
	Class   string
	Ops     []int64
}

// k4Run is one indexed run.
type k4Run struct {
	ID        string
	Stub      bool
	Dir       string
	Manifest  map[string]any
	Plan      map[string]any
	PlanSHA   string
	SealRoot  string
	Checks    map[string]any
	ChecksSHA string
	Verdict   string
	Classes   []string
	Anoms     []k4Anom
	Valid     map[string]string // checker id → true / false / unknown
	Problems  []string          // integrity problems (seal, manifest, checks)
}

func (r *k4Run) campaign() string { return str(r.Manifest, "campaign") }
func (r *k4Run) profile() string  { return str(r.Manifest, "profile") }
func (r *k4Run) mutant() string   { return str(r.Manifest, "mutant") }
func (r *k4Run) seed() string     { return fmt.Sprint(r.Manifest["seed"]) }

// kind is "primary", "candidate" or "replay".
func (r *k4Run) kind() string {
	switch {
	case r.Manifest[k4ReplayOf] != nil:
		return "replay"
	case r.Manifest[k4CandidateOf] != nil:
		return "candidate"
	}
	return "primary"
}

func (r *k4Run) has(class string) bool {
	for _, c := range r.Classes {
		if c == class {
			return true
		}
	}
	return false
}

// valid: a run that is not INVALID (CONTRACT §12.1) and has no integrity
// problem.
func (r *k4Run) valid() bool {
	return len(r.Problems) == 0 && r.Verdict != "" && r.Verdict != "INVALID"
}

type k4Index struct {
	Runs  []*k4Run
	ByID  map[string]*k4Run
	Error error
}

func (e *Evidence) k4() *k4Index {
	if e.memo == nil {
		e.memo = map[string]any{}
	}
	if ix, ok := e.memo["k4"].(*k4Index); ok {
		return ix
	}
	ix := buildK4Index(e)
	e.memo["k4"] = ix
	return ix
}

func buildK4Index(e *Evidence) *k4Index {
	ix := &k4Index{ByID: map[string]*k4Run{}}
	dir := filepath.Join(e.Dir, "runs")
	ents, err := os.ReadDir(dir)
	if err != nil {
		ix.Error = err
		return ix
	}
	for _, ent := range ents {
		name := ent.Name()
		var id string
		stub := false
		switch {
		case strings.HasSuffix(name, StubSuffix):
			id, stub = strings.TrimSuffix(name, StubSuffix), true
		case strings.HasSuffix(name, ArchiveSuffix):
			id = strings.TrimSuffix(name, ArchiveSuffix)
		default:
			continue
		}
		r := &k4Run{ID: id, Stub: stub}
		ix.Runs = append(ix.Runs, r)
		if ix.ByID[id] != nil {
			r.Problems = append(r.Problems, "run committed twice (full and stub)")
		}
		ix.ByID[id] = r
		e.loadK4Run(r)
	}
	sort.Slice(ix.Runs, func(i, j int) bool { return ix.Runs[i].ID < ix.Runs[j].ID })
	return ix
}

func (e *Evidence) loadK4Run(r *k4Run) {
	bad := func(f string, a ...any) { r.Problems = append(r.Problems, fmt.Sprintf(f, a...)) }
	var err error
	if r.Stub {
		r.Dir, err = e.extractStub(filepath.Join(e.Dir, "runs", r.ID+StubSuffix), r.ID)
	} else {
		r.Dir, err = e.RunDir("runs/" + r.ID)
	}
	if err != nil {
		bad("%v", err)
		return
	}
	var rep *history.SealReport
	if r.Stub {
		rep = history.VerifyStub(r.Dir)
	} else {
		rep = history.VerifySeal(r.Dir)
	}
	r.SealRoot = rep.MerkleRoot
	if !rep.Valid {
		bad("seal: %v", rep.Problems)
	}
	if r.Manifest, err = readJSON(filepath.Join(r.Dir, history.FileManifest)); err != nil {
		bad("manifest: %v", err)
		return
	}
	if r.Plan, err = readJSON(filepath.Join(r.Dir, history.FilePlan)); err != nil {
		bad("plan: %v", err)
	}
	r.PlanSHA = str(r.Manifest, "plan_sha256")
	if got := FileSHA256(filepath.Join(r.Dir, history.FilePlan)); got != r.PlanSHA {
		bad("plan.json sha256 %s, manifest says %s", got, r.PlanSHA)
	}
	if str(r.Manifest, "run_id") != r.ID {
		bad("manifest run_id %q", str(r.Manifest, "run_id"))
	}
	if str(r.Manifest, "contract_version") != params.ContractVersion {
		bad("contract version %s, not %s", str(r.Manifest, "contract_version"), params.ContractVersion)
	}
	if fmt.Sprint(r.Manifest["commitproof_dirty"]) != "false" || fmt.Sprint(r.Manifest["runner_image_dirty"]) != "false" {
		bad("run from a dirty tree")
	}
	if s := sabotageList(r.Manifest["sabotage"]); len(s) > 0 {
		bad("run built with sabotage %v", s)
	}
	if r.Plan != nil && (str(r.Plan, "campaign") != r.campaign() || fmt.Sprint(r.Plan["seed"]) != r.seed() ||
		str(r.Plan, "profile") != r.profile()) {
		bad("plan.json (campaign %s seed %v profile %s) does not match the manifest", str(r.Plan, "campaign"), r.Plan["seed"], str(r.Plan, "profile"))
	}
	cp := filepath.Join(e.Dir, "runs", r.ID+".checks.json")
	if r.Checks, err = readJSON(cp); err != nil {
		bad("checks.json: %v", err)
		return
	}
	r.ChecksSHA = FileSHA256(cp)
	if str(r.Checks, "seal_root") != r.SealRoot || str(r.Checks, "run_id") != r.ID {
		bad("checks.json (run %s, seal %s) is not for this run (seal %s)", str(r.Checks, "run_id"), str(r.Checks, "seal_root"), r.SealRoot)
	}
	r.Verdict = str(r.Checks, "verdict")
	r.Valid = map[string]string{}
	seen := map[string]bool{}
	cs, _ := r.Checks["checkers"].([]any)
	for _, x := range cs {
		c, _ := x.(map[string]any)
		id := str(c, "id")
		r.Valid[id] = fmt.Sprint(c["valid"])
		if r.Valid[id] != "false" {
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
			r.Anoms = append(r.Anoms, k4Anom{Checker: id, Class: cl, Ops: ops})
			if !seen[cl] {
				seen[cl] = true
				r.Classes = append(r.Classes, cl)
			}
		}
	}
	// The verdict follows from the checkers (§12.1), unless the run is
	// INVALID (cpcheck's invalid reasons).
	want := "PASS"
	for _, v := range r.Valid {
		switch {
		case v == "false":
			want = "FAIL"
		case v != "true" && want != "FAIL":
			want = "UNKNOWN"
		}
	}
	if r.Verdict != "INVALID" && r.Verdict != want {
		bad("checks.json verdict %s, checkers say %s", r.Verdict, want)
	}
}

func (e *Evidence) extractStub(archive, id string) (string, error) {
	tmp, err := os.MkdirTemp("", "cpgate-stub-")
	if err != nil {
		return "", err
	}
	e.temps = append(e.temps, tmp)
	if err := ExtractRun(archive, tmp); err != nil {
		return "", err
	}
	d := filepath.Join(tmp, id)
	if st, err := os.Stat(d); err != nil || !st.IsDir() {
		return "", fmt.Errorf("%s does not contain %s/", archive, id)
	}
	return d, nil
}

// primaries returns the primary runs of (campaign, profile, mutant, seed),
// INVALID attempts included.
func (ix *k4Index) primaries(campaign, profile, mutant, seed string) []*k4Run {
	var out []*k4Run
	for _, r := range ix.Runs {
		if r.kind() == "primary" && r.campaign() == campaign && r.profile() == profile && r.mutant() == mutant && r.seed() == seed {
			out = append(out, r)
		}
	}
	return out
}

// cell returns the valid primary run of a matrix cell and every attempt.
func (ix *k4Index) cell(campaign, profile, mutant, seed string) (*k4Run, []*k4Run, []string) {
	all := ix.primaries(campaign, profile, mutant, seed)
	var valid []*k4Run
	for _, r := range all {
		if r.valid() {
			valid = append(valid, r)
		}
	}
	name := fmt.Sprintf("%s/%s/%s seed %s", campaign, profile, mutant, seed)
	switch len(valid) {
	case 0:
		var why []string
		for _, r := range all {
			why = append(why, fmt.Sprintf("%s %s %v", r.ID, r.Verdict, r.Problems))
		}
		return nil, all, []string{fmt.Sprintf("%s: no valid run (attempts: %v)", name, why)}
	case 1:
		return valid[0], all, nil
	}
	return valid[0], all, []string{fmt.Sprintf("%s: %d valid runs (the campaign runs a cell until one is valid)", name, len(valid))}
}

// k4TargetClass is ADR-0024's target class (the same rule as
// internal/campaign.TargetClass, restated here: cpgate imports neither the
// campaign driver nor the plan generator, CONTRACT §5.3).
func k4TargetClass(r *k4Run) string {
	expected := map[string]string{"D1": "lost-write", "D2": "lost-write", "DH1": "lost-write", "claims": "p1-not-rejected"}
	if c, ok := expected[r.campaign()]; ok && r.has(c) {
		return c
	}
	if len(r.Classes) > 0 {
		return r.Classes[0]
	}
	return ""
}

// k4Results reads results.json into run id → entry.
func k4Results(e *Evidence) (map[string]map[string]any, error) {
	m, err := readJSON(filepath.Join(e.Dir, "results.json"))
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]any{}
	rs, _ := m["runs"].([]any)
	for _, x := range rs {
		r, _ := x.(map[string]any)
		out[str(r, "run_id")] = r
	}
	return out, nil
}

// checkResult compares a results.json entry with the indexed run.
func checkResult(res map[string]map[string]any, r *k4Run) []string {
	e, ok := res[r.ID]
	if !ok {
		return []string{fmt.Sprintf("results.json lacks run %s", r.ID)}
	}
	var p []string
	cls := []string{}
	if l, ok := e["classes"].([]any); ok {
		for _, c := range l {
			cls = append(cls, fmt.Sprint(c))
		}
	}
	got := strings.Join(cls, ",")
	want := strings.Join(r.Classes, ",")
	if str(e, "verdict") != r.Verdict || got != want || str(e, "seal_root") != r.SealRoot || str(e, "checks_sha256") != r.ChecksSHA ||
		str(e, "kind") != r.kind() || fmt.Sprint(e["stub"]) != fmt.Sprint(r.Stub) {
		p = append(p, fmt.Sprintf("results.json entry of %s (%s %s seal %.8s stub %v) differs from the run (%s %s seal %.8s stub %v)",
			r.ID, str(e, "verdict"), got, str(e, "seal_root"), e["stub"], r.Verdict, want, r.SealRoot, r.Stub))
	}
	return p
}
