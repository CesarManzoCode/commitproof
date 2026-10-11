package gate

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/history"
)

// K3FaultKinds is the closed fault catalog of CONTRACT §8.
var K3FaultKinds = []string{"F1a", "F1b", "F1c", "F1d", "F2a", "F2b", "F2c", "F2d", "F2e", "F3a", "F3b", "F4a", "F5a", "F5b", "F6", "F7"}

// k3Applications is K3.1's count per kind: 3, F5a/F5b once, F6 twice.
func k3Applications(kind string) int {
	switch kind {
	case "F5a", "F5b":
		return 1
	case "F6":
		return 2
	}
	return 3
}

// k3Campaign is the campaign suffix of a kind's K3.1 run (k3-<suffix>).
func k3Campaign(kind string) string {
	if kind == "F5a" || kind == "F5b" {
		return "f5"
	}
	return strings.ToLower(kind)
}

// K3Campaigns are the K3.1 runs: one per campaign, plus the composite.
var K3Campaigns = []string{"f1a", "f1b", "f1c", "f1d", "f2a", "f2b", "f2c", "f2d", "f2e", "f3a", "f3b", "f4a", "f5", "f6", "f7", "composite"}

// K3HealSabotaged are the kinds of K3.3.
var K3HealSabotaged = []string{"F1a", "F3a"}

// k3Run is a sealed run directory referenced by a record.
type k3Run struct {
	Dir      string
	Rel      string
	Summary  map[string]any
	Seal     *history.SealReport
	Manifest map[string]any
	Plan     map[string]any
}

// loadK3Run reads the run named by a recorded `commitproof run` and
// verifies its seal. wantExit is the expected exit code of the record.
func loadK3Run(e *Evidence, name, campaign string, wantExit int) (*k3Run, []string) {
	r, out, err := e.Get(name)
	if err != nil {
		return nil, []string{err.Error()}
	}
	var p []string
	if r.ExitCode != wantExit {
		p = append(p, fmt.Sprintf("%s exited %d, want %d", name, r.ExitCode, wantExit))
	}
	if !argvHas(r, "run", "--campaign", "k3-"+campaign, "--seed") {
		p = append(p, fmt.Sprintf("%s argv %q is not a k3-%s run", name, r.Argv, campaign))
	}
	s, err := LastJSONLine(out)
	if err != nil {
		return nil, append(p, name+": "+err.Error())
	}
	rel := str(s, "run_dir")
	if rel == "" {
		return nil, append(p, name+": summary has no run_dir")
	}
	d, err := e.RunDir(rel)
	if err != nil {
		return nil, append(p, name+": "+err.Error())
	}
	run := &k3Run{Dir: d, Rel: rel, Summary: s}
	run.Seal = history.VerifySeal(run.Dir)
	if !run.Seal.Valid {
		p = append(p, fmt.Sprintf("%s: seal of %s does not verify: %v", name, rel, run.Seal.Problems))
		return run, p
	}
	if run.Manifest, err = readJSON(filepath.Join(run.Dir, history.FileManifest)); err != nil {
		p = append(p, name+": manifest: "+err.Error())
	}
	if run.Plan, err = readJSON(filepath.Join(run.Dir, history.FilePlan)); err != nil {
		p = append(p, name+": plan: "+err.Error())
	}
	if c := str(run.Manifest, "campaign"); c != "k3-"+campaign {
		p = append(p, fmt.Sprintf("%s: run campaign %q, want k3-%s", name, c, campaign))
	}
	if str(run.Manifest, "mutant") != "real" {
		p = append(p, name+": not a real-build run")
	}
	if fmt.Sprint(run.Manifest["commitproof_dirty"]) != "false" || fmt.Sprint(run.Manifest["runner_image_dirty"]) != "false" {
		p = append(p, name+": run from a dirty tree")
	}
	return run, p
}

// faultTrace is the phase sequence of one fault id.
type faultTrace struct {
	ID     string
	Kind   string
	Phases []string
	Events []history.Event
}

func (r *k3Run) faults() ([]*faultTrace, map[string]any, map[string]any) {
	by := map[string]*faultTrace{}
	var order []*faultTrace
	var start, end map[string]any
	for _, ev := range r.Seal.History {
		switch ev.Type {
		case history.TypeNemesis:
			id := fmt.Sprint(ev.Fault["id"])
			t, ok := by[id]
			if !ok {
				t = &faultTrace{ID: id, Kind: str(ev.Fault, "kind")}
				by[id] = t
				order = append(order, t)
			}
			t.Phases = append(t.Phases, str(ev.Fault, "phase"))
			t.Events = append(t.Events, ev)
		case history.TypeMarker:
			v, _ := ev.Value.(map[string]any)
			switch str(v, "name") {
			case "run-start":
				start = v
			case "run-end":
				end = v
			}
		}
	}
	return order, start, end
}

func sabotageList(v any) []string {
	l, _ := v.([]any)
	out := []string{}
	for _, x := range l {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// planKinds lists the plan's element kinds per step.
func planKinds(plan map[string]any) [][]string {
	steps, _ := plan["nemesis"].([]any)
	var out [][]string
	for _, s := range steps {
		m, _ := s.(map[string]any)
		els, _ := m["elements"].([]any)
		var ks []string
		for _, e := range els {
			em, _ := e.(map[string]any)
			ks = append(ks, str(em, "kind"))
		}
		out = append(out, ks)
	}
	return out
}

func healable(kind string) bool { return kind != "F5a" && kind != "F5b" }

func fullLifecycle(t *faultTrace) bool {
	want := []string{"inject", "confirmed"}
	if healable(t.Kind) {
		want = append(want, "heal", "healed")
	}
	return reflect.DeepEqual(t.Phases, want)
}

// stableFinalReads recomputes P08 from the history: for every gateway with
// final reads and every final-read workload, the last three ok final reads
// are identical. It never trusts the runner's markers.
func stableFinalReads(evs []history.Event) []string {
	type key struct{ gw, wl string }
	vals := map[key][][]byte{}
	gws := map[string]bool{}
	for _, ev := range evs {
		if ev.Type == history.TypeOK && ev.F == "final-read" {
			b, err := canonjson.Marshal(ev.Value)
			if err != nil {
				return []string{"final read value: " + err.Error()}
			}
			vals[key{ev.GW, ev.WL}] = append(vals[key{ev.GW, ev.WL}], b)
			gws[ev.GW] = true
		}
	}
	var p []string
	if len(gws) == 0 {
		return []string{"no final reads"}
	}
	for gw := range gws {
		for _, wl := range []string{"append", "register", "bank", "ledger"} {
			v := vals[key{gw, wl}]
			if len(v) < 3 || !bytes.Equal(v[len(v)-1], v[len(v)-2]) || !bytes.Equal(v[len(v)-2], v[len(v)-3]) {
				p = append(p, fmt.Sprintf("final reads of %s through %s not stable", wl, gw))
			}
		}
	}
	sort.Strings(p)
	return p
}

// K3 returns the six criteria of CONTRACT §15 K3 with their sabotages.
func K3() []Criterion {
	return []Criterion{
		{ID: "K3.1", Name: "Each fault confirmed", Eval: evalK31,
			Sabotage: "point the k3-f1a run record at the K3.3 F1a heal-sabotage run (hashes kept consistent)",
			Apply: func(dir string) error {
				_, out, err := LoadEvidence(dir).Get("k3-sab-heal-f1a")
				if err != nil {
					return err
				}
				s, err := LastJSONLine(out)
				if err != nil {
					return err
				}
				return editSummary(dir, "k3-run-f1a", func(m map[string]any) { m["run_dir"] = s["run_dir"] })
			}},
		{ID: "K3.2", Name: "Fail-closed", Eval: evalK32,
			Sabotage: "point the F2a inject-sabotage record at the K3.1 F2a run (hashes kept consistent)",
			Apply: func(dir string) error {
				_, out, err := LoadEvidence(dir).Get("k3-run-f2a")
				if err != nil {
					return err
				}
				s, err := LastJSONLine(out)
				if err != nil {
					return err
				}
				return editSummary(dir, "k3-sab-inject-f2a", func(m map[string]any) { m["run_dir"] = s["run_dir"] })
			}},
		{ID: "K3.3", Name: "Heal fail-closed", Eval: evalK33,
			Sabotage: "point the F3a heal-sabotage record at the K3.1 F3a run (hashes kept consistent)",
			Apply: func(dir string) error {
				_, out, err := LoadEvidence(dir).Get("k3-run-f3a")
				if err != nil {
					return err
				}
				s, err := LastJSONLine(out)
				if err != nil {
					return err
				}
				return editSummary(dir, "k3-sab-heal-f3a", func(m map[string]any) { m["run_dir"] = s["run_dir"] })
			}},
		{ID: "K3.4", Name: "Observability of confirmation", Eval: evalK34,
			Sabotage: "in the F7 run, change the recorded stdout sha256 of the first confirmation probe and re-seal consistently (checks.json and its record follow)",
			Apply:    sabotageK34},
		{ID: "K3.5", Name: "Checkers on real runs", Eval: evalK35,
			Sabotage: "delete the checks.json of the F7 run",
			Apply: func(dir string) error {
				_, out, err := LoadEvidence(dir).Get("k3-check-f7")
				if err != nil {
					return err
				}
				s, err := LastJSONLine(out)
				if err != nil {
					return err
				}
				return os.Remove(LoadEvidence(dir).Path(str(s, "checks")))
			}},
		{ID: "K3.6", Name: "CI-equivalent", Eval: evalK36,
			Sabotage: "replace the recorded exit code of `make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke` with 1",
			Apply: func(dir string) error {
				return editRecords(dir, func(r map[string]any) {
					if r["name"] == "ci" {
						r["exit_code"] = 1
					}
				})
			}},
	}
}

// evalK31: for each kind, its K3.1 run applies it the required number of
// times, every injection confirmed and every heal healed, and the run is
// not INVALID (recomputed: seal, run-end complete, no not-applied /
// not-healed, stable final reads). The composite run's elements are
// confirmed in order and healed in reverse order.
func evalK31(e *Evidence) (string, []string, []string) {
	var p, details []string
	runs := map[string]*k3Run{}
	for _, c := range K3Campaigns {
		run, pp := loadK3Run(e, "k3-run-"+c, c, 0)
		p = append(p, pp...)
		if run == nil || !run.Seal.Valid {
			continue
		}
		runs[c] = run
		if str(run.Summary, "status") != "complete" {
			p = append(p, fmt.Sprintf("k3-%s: run status %q", c, str(run.Summary, "status")))
		}
		if s := sabotageList(run.Manifest["sabotage"]); len(s) > 0 {
			p = append(p, fmt.Sprintf("k3-%s: run built with sabotage %v", c, s))
		}
		traces, start, end := run.faults()
		if s := sabotageList(start["sabotage"]); len(s) > 0 {
			p = append(p, fmt.Sprintf("k3-%s: runner reports sabotage %v", c, s))
		}
		if str(end, "status") != "complete" {
			p = append(p, fmt.Sprintf("k3-%s: run-end status %q (%s)", c, str(end, "status"), str(end, "reason")))
		}
		for _, t := range traces {
			for _, ph := range t.Phases {
				if ph == "not-applied" || ph == "not-healed" {
					p = append(p, fmt.Sprintf("k3-%s: fault %s (%s) %s", c, t.ID, t.Kind, ph))
				}
			}
		}
		for _, x := range stableFinalReads(run.Seal.History) {
			p = append(p, fmt.Sprintf("k3-%s: %s", c, x))
		}
	}
	for _, kind := range K3FaultKinds {
		c := k3Campaign(kind)
		run := runs[c]
		if run == nil {
			p = append(p, kind+": no verified run")
			continue
		}
		planned := 0
		for _, st := range planKinds(run.Plan) {
			for _, k := range st {
				if k == kind {
					planned++
				}
			}
		}
		if planned < k3Applications(kind) {
			p = append(p, fmt.Sprintf("%s: plan of k3-%s applies it %d times, want %d", kind, c, planned, k3Applications(kind)))
		}
		traces, _, _ := run.faults()
		done := 0
		for _, t := range traces {
			if t.Kind != kind {
				continue
			}
			if fullLifecycle(t) {
				done++
			} else {
				p = append(p, fmt.Sprintf("%s: fault %s phases %v", kind, t.ID, t.Phases))
			}
		}
		if done != planned || done < k3Applications(kind) {
			p = append(p, fmt.Sprintf("%s: %d complete applications in k3-%s, planned %d, want ≥ %d", kind, done, c, planned, k3Applications(kind)))
		}
		details = append(details, fmt.Sprintf("%s: %d applications confirmed and healed in %s", kind, done, filepath.Base(run.Rel)))
	}
	if run := runs["composite"]; run != nil {
		steps := planKinds(run.Plan)
		traces, _, _ := run.faults()
		if len(steps) != 1 || len(steps[0]) < 2 {
			p = append(p, fmt.Sprintf("composite: plan steps %v are not one multi-element step", steps))
		} else {
			// Event order: inject/confirmed of element k before element
			// k+1's inject; heals in reverse order.
			var seq []string
			for _, ev := range run.Seal.History {
				if ev.Type == history.TypeNemesis {
					seq = append(seq, fmt.Sprintf("%v:%s", ev.Fault["id"], str(ev.Fault, "phase")))
				}
			}
			var want []string
			n := len(steps[0])
			for k := 0; k < n; k++ {
				want = append(want, fmt.Sprintf("f0.%d:inject", k), fmt.Sprintf("f0.%d:confirmed", k))
			}
			for k := n - 1; k >= 0; k-- {
				if healable(steps[0][k]) {
					want = append(want, fmt.Sprintf("f0.%d:heal", k), fmt.Sprintf("f0.%d:healed", k))
				}
			}
			if !reflect.DeepEqual(seq, want) {
				p = append(p, fmt.Sprintf("composite: event order %v, want %v", seq, want))
			}
			for _, t := range traces {
				if !fullLifecycle(t) {
					p = append(p, fmt.Sprintf("composite: fault %s phases %v", t.ID, t.Phases))
				}
			}
			details = append(details, fmt.Sprintf("composite %v: injected and confirmed in order, healed in reverse (%s)", steps[0], filepath.Base(run.Rel)))
		}
	}
	return "runs/", p, details
}

// sabotagedRun checks one K3.2/K3.3 run: the record exits 2, the run and
// its runner carry exactly the sabotage tag, the fault of kind ends with
// phase, the run is INVALID, and cpcheck says INVALID with exit 2.
func sabotagedRun(e *Evidence, name, checkName, campaign, kind, tag, phase string) []string {
	run, p := loadK3Run(e, name, campaign, 2)
	if run == nil || !run.Seal.Valid {
		return p
	}
	if str(run.Summary, "status") != "invalid" {
		p = append(p, fmt.Sprintf("%s: run status %q, want invalid", name, str(run.Summary, "status")))
	}
	if s := sabotageList(run.Manifest["sabotage"]); !reflect.DeepEqual(s, []string{tag}) {
		p = append(p, fmt.Sprintf("%s: manifest sabotage %v, want [%s]", name, s, tag))
	}
	traces, start, end := run.faults()
	if s := sabotageList(start["sabotage"]); !reflect.DeepEqual(s, []string{tag}) {
		p = append(p, fmt.Sprintf("%s: runner sabotage %v, want [%s]", name, s, tag))
	}
	if str(end, "status") != "invalid" {
		p = append(p, fmt.Sprintf("%s: run-end status %q, want invalid", name, str(end, "status")))
	}
	hit := false
	for _, t := range traces {
		last := t.Phases[len(t.Phases)-1]
		switch {
		case t.Kind == kind && last == phase:
			hit = true
			want := []string{"inject", phase}
			if phase == "not-healed" {
				want = []string{"inject", "confirmed", "heal", phase}
			}
			if !reflect.DeepEqual(t.Phases, want) {
				p = append(p, fmt.Sprintf("%s: fault %s phases %v, want %v", name, t.ID, t.Phases, want))
			}
		case last == "not-applied" || last == "not-healed":
			p = append(p, fmt.Sprintf("%s: fault %s (%s) ended %s, not the sabotaged one", name, t.ID, t.Kind, last))
		}
	}
	if !hit {
		p = append(p, fmt.Sprintf("%s: no %s fault ended %s", name, kind, phase))
	}
	// cpcheck: INVALID, exit 2, on this run, for this reason.
	cr, out, err := e.Get(checkName)
	if err != nil {
		return append(p, err.Error())
	}
	if cr.ExitCode != 2 {
		p = append(p, fmt.Sprintf("%s exited %d, want 2", checkName, cr.ExitCode))
	}
	s, err := LastJSONLine(out)
	if err != nil {
		return append(p, checkName+": "+err.Error())
	}
	if filepath.Clean(str(s, "run_dir")) != filepath.Clean(run.Rel) {
		p = append(p, fmt.Sprintf("%s checked %s, not %s", checkName, str(s, "run_dir"), run.Rel))
	}
	if str(s, "verdict") != "INVALID" || str(s, "seal_root") != run.Seal.MerkleRoot {
		p = append(p, fmt.Sprintf("%s: verdict %s seal %s", checkName, str(s, "verdict"), str(s, "seal_root")))
	}
	reasons, _ := s["invalid_reasons"].([]any)
	if !strings.Contains(fmt.Sprint(reasons), phase) {
		p = append(p, fmt.Sprintf("%s: invalid reasons %v do not name %s", checkName, reasons, phase))
	}
	return p
}

func evalK32(e *Evidence) (string, []string, []string) {
	var p, details []string
	for _, kind := range K3FaultKinds {
		k := strings.ToLower(kind)
		pp := sabotagedRun(e, "k3-sab-inject-"+k, "k3-sabcheck-inject-"+k, k3Campaign(kind), kind, "cp_sabotage_inject_"+k, "not-applied")
		p = append(p, pp...)
		if len(pp) == 0 {
			details = append(details, kind+": injector no-op → not-applied, INVALID, exit 2")
		}
	}
	return "runs/", p, details
}

func evalK33(e *Evidence) (string, []string, []string) {
	var p, details []string
	for _, kind := range K3HealSabotaged {
		k := strings.ToLower(kind)
		pp := sabotagedRun(e, "k3-sab-heal-"+k, "k3-sabcheck-heal-"+k, k3Campaign(kind), kind, "cp_sabotage_heal_"+k, "not-healed")
		p = append(p, pp...)
		if len(pp) == 0 {
			details = append(details, kind+": heal no-op → not-healed, INVALID, exit 2")
		}
	}
	return "runs/", p, details
}

// cmdRecord is one line of a run directory's commands.jsonl (cmdlog).
type cmdRecord struct {
	Seq          int64    `json:"seq"`
	Name         string   `json:"name"`
	Argv         []string `json:"argv"`
	ExitCode     int64    `json:"exit_code"`
	Stdout       string   `json:"stdout"`
	StdoutSHA256 string   `json:"stdout_sha256"`
	Stderr       string   `json:"stderr"`
	StderrSHA256 string   `json:"stderr_sha256"`
}

func runCommands(dir string) (map[string][]cmdRecord, error) {
	f, err := os.Open(filepath.Join(dir, history.FileCommands))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string][]cmdRecord{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r cmdRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		if r.Name != "" {
			k := fmt.Sprintf("%s#%d", r.Name, r.Seq)
			out[k] = append(out[k], r)
		}
	}
	return out, sc.Err()
}

func rawOf(m map[string]any, k string) ([]byte, bool) {
	if s, ok := m[k].(string); ok {
		return []byte(s), true
	}
	if s, ok := m[k+"_b64"].(string); ok {
		b, err := base64.StdEncoding.DecodeString(s)
		return b, err == nil
	}
	return nil, false
}

// checkProbe verifies one evidence probe entry against the run's
// commands.jsonl record and its output files.
func checkProbe(dir string, recs map[string][]cmdRecord, m map[string]any) string {
	seq, _ := m["seq"].(json.Number)
	k := fmt.Sprintf("%s#%s", str(m, "name"), seq)
	rs := recs[k]
	if len(rs) != 1 {
		return fmt.Sprintf("probe %s: %d commands.jsonl records", k, len(rs))
	}
	r := rs[0]
	var argv []string
	if l, ok := m["argv"].([]any); ok {
		for _, a := range l {
			argv = append(argv, fmt.Sprint(a))
		}
	}
	if !reflect.DeepEqual(argv, r.Argv) {
		return fmt.Sprintf("probe %s: argv differs from the record", k)
	}
	if ec, _ := m["exit_code"].(json.Number); string(ec) != fmt.Sprint(r.ExitCode) {
		return fmt.Sprintf("probe %s: exit code %s, record %d", k, ec, r.ExitCode)
	}
	for _, f := range []struct{ field, recSHA, recPath string }{
		{"stdout", r.StdoutSHA256, r.Stdout}, {"stderr", r.StderrSHA256, r.Stderr},
	} {
		raw, ok := rawOf(m, f.field)
		if !ok {
			return fmt.Sprintf("probe %s: no raw %s in the evidence", k, f.field)
		}
		if sha(raw) != str(m, f.field+"_sha256") || sha(raw) != f.recSHA {
			return fmt.Sprintf("probe %s: %s sha256 %s, evidence says %s, record %s", k, f.field, sha(raw), str(m, f.field+"_sha256"), f.recSHA)
		}
		if got := FileSHA256(filepath.Join(dir, filepath.FromSlash(f.recPath))); got != f.recSHA {
			return fmt.Sprintf("probe %s: %s file hashes to %q, record %s", k, f.field, got, f.recSHA)
		}
	}
	return ""
}

func probeList(ev map[string]any, key string) []map[string]any {
	l, _ := ev[key].([]any)
	var out []map[string]any
	for _, x := range l {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// evalK34: every confirmation event of the K3.1 runs carries the raw
// outputs of probes other than its injection commands, and every probe
// and command entry of confirmation and heal-confirmation events matches
// its commands.jsonl record (argv, exit code, stdout/stderr sha256) and
// the recorded output files.
func evalK34(e *Evidence) (string, []string, []string) {
	var p []string
	nConfirm, nProbes := 0, 0
	for _, c := range K3Campaigns {
		_, out, err := e.Get("k3-run-" + c)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		s, err := LastJSONLine(out)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		dir, err := e.RunDir(str(s, "run_dir"))
		if err != nil {
			p = append(p, fmt.Sprintf("k3-%s: %v", c, err))
			continue
		}
		recs, err := runCommands(dir)
		if err != nil {
			p = append(p, fmt.Sprintf("k3-%s commands.jsonl: %v", c, err))
			continue
		}
		evs, err := history.ReadChain(filepath.Join(dir, history.FileHistory))
		if err != nil {
			p = append(p, fmt.Sprintf("k3-%s history: %v", c, err))
			continue
		}
		for _, ev := range evs {
			if ev.Type != history.TypeNemesis {
				continue
			}
			phase := str(ev.Fault, "phase")
			if phase != "confirmed" && phase != "healed" {
				continue
			}
			evd, _ := ev.Fault["evidence"].(map[string]any)
			probes := probeList(evd, "probes")
			if phase == "confirmed" {
				nConfirm++
				if len(probes) == 0 {
					p = append(p, fmt.Sprintf("k3-%s fault %v: confirmation without probe outputs", c, ev.Fault["id"]))
				}
				inj := map[string]bool{}
				for _, m := range probeList(evd, "inject") {
					inj[str(m, "name")] = true
				}
				for _, m := range probes {
					if inj[str(m, "name")] {
						p = append(p, fmt.Sprintf("k3-%s fault %v: confirmation probe %s is an injection command", c, ev.Fault["id"], str(m, "name")))
					}
				}
			}
			for _, key := range []string{"probes", "inject", "heal", "manual_start"} {
				for _, m := range probeList(evd, key) {
					nProbes++
					if msg := checkProbe(dir, recs, m); msg != "" {
						p = append(p, fmt.Sprintf("k3-%s fault %v %s: %s", c, ev.Fault["id"], phase, msg))
					}
				}
			}
		}
	}
	if nConfirm == 0 {
		p = append(p, "no confirmation event")
	}
	return "runs/", p, []string{fmt.Sprintf("%d confirmation events, %d probe/command entries verified against commands.jsonl", nConfirm, nProbes)}
}

// evalK35: cpcheck ran on every K3.1 run (found by the check records, each
// naming a sealed real-build run of its campaign); checks.json matches the
// recorded summary and the run's seal. Any verdict is accepted.
func evalK35(e *Evidence) (string, []string, []string) {
	var p, details []string
	for _, c := range K3Campaigns {
		name := "k3-check-" + c
		r, out, err := e.Get(name)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		if !argvHas(r, "cpcheck", "check") {
			p = append(p, fmt.Sprintf("%s argv %q is not cpcheck check", name, r.Argv))
		}
		s, err := LastJSONLine(out)
		if err != nil {
			p = append(p, name+": "+err.Error())
			continue
		}
		dir, err := e.RunDir(str(s, "run_dir"))
		if err != nil {
			p = append(p, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		rep := history.VerifySeal(dir)
		if !rep.Valid {
			p = append(p, fmt.Sprintf("%s: run seal does not verify", name))
			continue
		}
		man, err := readJSON(filepath.Join(dir, history.FileManifest))
		if err != nil || str(man, "campaign") != "k3-"+c || len(sabotageList(man["sabotage"])) > 0 {
			p = append(p, fmt.Sprintf("%s: checked run is not the real k3-%s run", name, c))
			continue
		}
		verdict := str(s, "verdict")
		want := map[string]int{"PASS": 0, "FAIL": 1, "UNKNOWN": 2, "INVALID": 2}
		code, known := want[verdict]
		if !known || code != r.ExitCode {
			p = append(p, fmt.Sprintf("%s: verdict %q with exit %d", name, verdict, r.ExitCode))
		}
		cpath := e.Path(str(s, "checks"))
		b, err := os.ReadFile(cpath)
		if err != nil {
			p = append(p, fmt.Sprintf("%s: checks.json: %v", name, err))
			continue
		}
		if sha(b) != str(s, "checks_sha256") {
			p = append(p, fmt.Sprintf("%s: checks.json sha256 differs from the recorded summary", name))
		}
		cj, err := readJSON(cpath)
		if err != nil {
			p = append(p, name+": "+err.Error())
			continue
		}
		if str(cj, "seal_root") != rep.MerkleRoot || str(s, "seal_root") != rep.MerkleRoot {
			p = append(p, fmt.Sprintf("%s: checks.json seal %s, run seal %s", name, str(cj, "seal_root"), rep.MerkleRoot))
		}
		if str(cj, "verdict") != verdict {
			p = append(p, fmt.Sprintf("%s: checks.json verdict %s, summary %s", name, str(cj, "verdict"), verdict))
		}
		var fails []string
		cs, _ := cj["checkers"].([]any)
		for _, x := range cs {
			m, _ := x.(map[string]any)
			if v := fmt.Sprint(m["valid"]); v != "true" {
				fails = append(fails, fmt.Sprintf("%s=%s", str(m, "id"), v))
			}
		}
		details = append(details, fmt.Sprintf("k3-%s: %s %v", c, verdict, fails))
	}
	return "runs/", p, details
}

func evalK36(e *Evidence) (string, []string, []string) {
	r, _, p := exitOK(e, "ci", "make", "ci-unit", "ci-checkers", "ci-cluster-smoke", "ci-nemesis-smoke")
	return "commands.jsonl", p, []string{fmt.Sprintf("argv %q exit %d", r.Argv, r.ExitCode)}
}

// sabotageK34 forges one probe hash in the F7 run consistently: the run is
// re-sealed, its checks.json and the check record follow, so only the
// probe-to-record comparison of K3.4 can notice.
func sabotageK34(dir string) error {
	e := LoadEvidence(dir)
	defer e.Close()
	_, out, err := e.Get("k3-run-f7")
	if err != nil {
		return err
	}
	s, err := LastJSONLine(out)
	if err != nil {
		return err
	}
	run, err := e.RunDir(str(s, "run_dir"))
	if err != nil {
		return err
	}
	archive := e.Path(str(s, "run_dir") + ArchiveSuffix)
	if _, err := os.Stat(archive); err != nil {
		archive = "" // the run is a plain directory of the copy
	}
	filepath.WalkDir(run, func(p string, d fs.DirEntry, err error) error {
		if err == nil {
			os.Chmod(p, 0o755)
		}
		return nil
	})
	evs, err := history.ReadChain(filepath.Join(run, history.FileHistory))
	if err != nil {
		return err
	}
	target := ""
	for _, ev := range evs {
		if ev.Type == history.TypeNemesis && str(ev.Fault, "phase") == "confirmed" {
			evd, _ := ev.Fault["evidence"].(map[string]any)
			if ps := probeList(evd, "probes"); len(ps) > 0 {
				target = str(ps[0], "name")
				break
			}
		}
	}
	if target == "" {
		return fmt.Errorf("no confirmation probe in the F7 run")
	}
	cp := filepath.Join(run, history.FileCommands)
	b, err := os.ReadFile(cp)
	if err != nil {
		return err
	}
	var lines [][]byte
	for _, l := range bytes.Split(bytes.TrimRight(b, "\n"), []byte("\n")) {
		var m map[string]any
		dec := json.NewDecoder(bytes.NewReader(l))
		dec.UseNumber()
		if err := dec.Decode(&m); err != nil {
			return err
		}
		if m["name"] == target {
			m["stdout_sha256"] = strings.Repeat("0", 64)
			if l, err = canonjson.Marshal(m); err != nil {
				return err
			}
		}
		lines = append(lines, l)
	}
	if err := os.WriteFile(cp, append(bytes.Join(lines, []byte("\n")), '\n'), 0o644); err != nil {
		return err
	}
	os.Remove(filepath.Join(run, history.FileSeal))
	seal, err := history.SealDir(run)
	if err != nil {
		return err
	}
	if archive != "" {
		if err := ArchiveRun(run, archive); err != nil {
			return err
		}
	}
	_, cout, err := e.Get("k3-check-f7")
	if err != nil {
		return err
	}
	cs, err := LastJSONLine(cout)
	if err != nil {
		return err
	}
	checks := e.Path(str(cs, "checks"))
	if err := editJSONCanonical(checks, func(m map[string]any) { m["seal_root"] = seal.MerkleRoot }); err != nil {
		return err
	}
	nb, _ := os.ReadFile(checks)
	return editSummary(dir, "k3-check-f7", func(m map[string]any) {
		m["seal_root"] = seal.MerkleRoot
		m["checks_sha256"] = sha(nb)
	})
}

// editJSONCanonical rewrites a canonical JSON file (checks.json) through f.
func editJSONCanonical(p string, f func(map[string]any)) error {
	m, err := readJSON(p)
	if err != nil {
		return err
	}
	f(m)
	b, err := canonjson.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}
