package gate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/stats"
)

// ---------------------------------------------------------------- K7.3

// k7M1D1 are M1's five D1 runs (ADR-0046), cross-checked from copies of
// their K4 archives.
var k7M1D1 = []string{"20261011T215900Z-D1-1-M1-59a95336", "20261011T220118Z-D1-2-M1-8b22bb67", "20261011T220335Z-D1-3-M1-da159fa0",
	"20261011T220553Z-D1-4-M1-ad3fca18", "20261011T220810Z-D1-5-M1-3319a30c"}

// checkK7Oracle compares a stored oracle record with ADR-0046's decision
// re-derived from checksJSON and the records.
func checkK7Oracle(e *Evidence, id string, checksJSON []byte) []string {
	var p []string
	b, err := os.ReadFile(filepath.Join(e.Dir, "oracle", id+".json"))
	if err != nil {
		return []string{fmt.Sprintf("%s: C3 anomalies without an oracle record: %v", id, err)}
	}
	var rec K7Oracle
	if err := json.Unmarshal(b, &rec); err != nil {
		return []string{fmt.Sprintf("oracle/%s.json: %v", id, err)}
	}
	want, err := DecideK7Oracle(e, checksJSON)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", id, err)}
	}
	if rec.RunID != id || rec.SealRoot != want.SealRoot || rec.Result != want.Result || rec.DecidedBy != want.DecidedBy || rec.ElleValid != want.ElleValid ||
		strings.Join(rec.ElleClasses, ",") != strings.Join(want.ElleClasses, ",") || strings.Join(rec.C3Classes, ",") != strings.Join(want.C3Classes, ",") {
		p = append(p, fmt.Sprintf("oracle/%s.json says %s by %q (elle %s %v), the evidence gives %s by %q (elle %s %v)", id,
			rec.Result, rec.DecidedBy, rec.ElleValid, rec.ElleClasses, want.Result, want.DecidedBy, want.ElleValid, want.ElleClasses))
	}
	if strings.TrimSpace(rec.Source) == "" {
		p = append(p, fmt.Sprintf("oracle/%s.json: no source", id))
	}
	if rec.Result != "confirmed" && strings.TrimSpace(rec.Triage) == "" {
		p = append(p, fmt.Sprintf("oracle/%s.json: oracle-disagreement without its triage", id))
	}
	return p
}

// withoutElle drops C3's stats.elle (cpcheck --elle-oracle adds it; the
// verdicts and classes are cpcheck's either way).
func withoutElle(m map[string]any) map[string]any {
	cs, _ := m["checkers"].([]any)
	for _, x := range cs {
		c, _ := x.(map[string]any)
		if str(c, "id") == "C3" {
			if st, ok := c["stats"].(map[string]any); ok {
				delete(st, "elle")
			}
		}
	}
	return m
}

func evalK73(e *Evidence) (string, []string, []string) {
	var p, d []string
	ix := e.k7()
	if ix.Error != nil {
		return "oracle", []string{ix.Error.Error()}, nil
	}
	n := 0
	for _, r := range ix.Runs {
		if r.kind() != "primary" || !r.valid() || r.Valid["C3"] != "false" {
			continue
		}
		n++
		cb, err := os.ReadFile(filepath.Join(e.Dir, k7RunsSub, r.ID+".checks.json"))
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		p = append(p, checkK7Oracle(e, r.ID, cb)...)
	}
	d = append(d, fmt.Sprintf("%d K7 matrix runs with C3 anomalies, each with its oracle record", n))
	ce, cix := e.k7Cited("K4")
	_ = ce
	for _, id := range k7M1D1 {
		cp := filepath.Join(e.Dir, "oracle", id+".checks.json")
		cb, err := os.ReadFile(cp)
		if err != nil {
			p = append(p, fmt.Sprintf("M1 D1 %s: %v", id, err))
			continue
		}
		p = append(p, checkK7Oracle(e, id, cb)...)
		// The copy re-checked to K4's verdicts and classes: the checks.json
		// equals K4's apart from stats.elle.
		k4r := cix.ByID[id]
		mine, err1 := readJSON(cp)
		if k4r == nil || err1 != nil {
			p = append(p, fmt.Sprintf("M1 D1 %s: not in K4's runs or unreadable", id))
			continue
		}
		a, _ := json.Marshal(withoutElle(mine))
		k4c, _ := readJSON(filepath.Join(k7Root(e), "evidence", "K4", "runs", id+".checks.json"))
		bb, _ := json.Marshal(withoutElle(k4c))
		if !bytes.Equal(a, bb) {
			p = append(p, fmt.Sprintf("M1 D1 %s: the re-checked checks.json differs from K4's (apart from the oracle)", id))
		}
		var rec K7Oracle
		if b, err := os.ReadFile(filepath.Join(e.Dir, "oracle", id+".json")); err == nil && json.Unmarshal(b, &rec) == nil {
			d = append(d, fmt.Sprintf("M1 D1 %s: %s (%s)", id, rec.Result, rec.DecidedBy))
		}
	}
	return "oracle", p, d
}

func sabotageK73(dir string) error {
	return editJSONCanonical(filepath.Join(dir, "oracle", k7M1D1[0]+".json"), func(m map[string]any) { m["result"] = "confirmed" })
}

// ---------------------------------------------------------------- K7.4

// k7Recorded are the findings recorded before K7 (K4, K5, K6b) and where
// their runs and triage are (ADR-0042, ADR-0043).
var k7Recorded = []string{"FND-D1-fb8d0f8c", "FND-D1-2e860fd2", "FND-D1-a8eb9857", "FND-C2-2a534585", "FND-C2-6806aeca",
	"FND-C9-40b6f05a", "FND-C9-2fe9a9fc", "FND-C9-90da5532", "FND-C11-17a344c1"}

// k7GuaranteeLevel: §3's claim level per guarantee (G2a/G2b, ADR-0042).
var k7GuaranteeLevel = map[string]string{"G1": "EXPLICIT", "G2a": "EXPLICIT", "G2b": "INFERRED", "G3": "EXPLICIT", "G4": "INFERRED", "G5": "INFERRED",
	"G6": "DESIGN", "G7": "DESIGN", "G8": "DESIGN", "G9": "DESIGN", "G10": "DESIGN", "G11": "DESIGN", "G12": "DESIGN", "G13": "INFERRED"}

// k7Class: §11.1's class per guarantee.
func k7Class(g string) string {
	switch g {
	case "G7", "G8", "G9", "G10", "G12":
		return "PROTOCOL_DEVIATION"
	}
	return "SAFETY_VIOLATION"
}

// k7Severity: §11.2.
func k7Severity(class, level, g string) string {
	switch {
	case class == "SAFETY_VIOLATION" && level == "EXPLICIT":
		return "critical"
	case class == "SAFETY_VIOLATION", class == "PROTOCOL_DEVIATION" && g == "G7":
		return "high"
	case class == "PROTOCOL_DEVIATION":
		return "medium"
	case class == "LIVENESS_OBSERVATION":
		return "info"
	}
	return ""
}

func k7Repro(hits int) string {
	switch {
	case hits >= params.ReproducedAtLeast:
		return "REPRODUCED"
	case hits >= 1:
		return "INTERMITTENT"
	}
	return "NOT_REPRODUCED"
}

var (
	k7G1Line   = regexp.MustCompile(`^\*\*Acknowledged writes lost \(G1\) in this finding's original run: (no|yes)\.\*\*`)
	k7Title    = regexp.MustCompile(`^# [a-z0-9][a-z0-9 /_.-]*: \S`)
	k7Sections = []string{"## Summary", "## Version and environment", "## Reproduction", "## Expected vs actual", "## Analysis", "## Limitations"}
)

// k7Package is findings/<id>/finding.json.
type k7Package struct {
	ID                     string            `json:"id"`
	Campaign               string            `json:"campaign"`
	AnomalyClass           string            `json:"anomaly_class"`
	AcknowledgedWriteLost  bool              `json:"acknowledged_write_lost"`
	Classification         map[string]string `json:"classification"`
	RecordedClassification map[string]string `json:"recorded_classification"`
	Original               k7PkgRun          `json:"original_run"`
	Minimized              k7PkgRun          `json:"minimized_run"`
	MinimizedPlanSHA       string            `json:"minimized_plan_sha256"`
	Replays                []k7PkgRun        `json:"replays"`
	ReplaysWithClass       int               `json:"replays_with_class"`
	// K7Replay is K7.4's re-execution of the package's replay command
	// (`commitproof replay <minimized run> --times 5`, P13: the command the
	// CLI accepts and the one that produced the recorded replays): its record,
	// its replays (stored in package-replays/) and their §11.1 class. The
	// recorded classification stands; this result is reported beside it.
	K7Replay struct {
		Record           string     `json:"record"`
		Runs             []k7PkgRun `json:"runs"`
		ReplaysWithClass int        `json:"replays_with_class"`
		Reproducibility  string     `json:"reproducibility"`
	} `json:"k7_replay"`
	Recurrences []struct {
		Run       string `json:"run_id"`
		Cell      string `json:"cell"`
		Seed      int64  `json:"seed"`
		SealRoot  string `json:"seal_root"`
		Predicate string `json:"predicate"`
	} `json:"recurrences"`
	Triage string `json:"triage"`
}

type k7PkgRun struct {
	Run           string   `json:"run_id"`
	Archive       string   `json:"archive"`
	ArchiveSHA256 string   `json:"archive_sha256"`
	SealRoot      string   `json:"seal_root"`
	Verdict       string   `json:"verdict"`
	Classes       []string `json:"classes"`
}

// k7SprintRun finds a run of K4, K5 or K7 by id (archive or stub) with its
// index entry.
func (e *Evidence) k7SprintRun(id string) (*k4Run, string) {
	if r := e.k7().ByID[id]; r != nil {
		return r, "evidence/K7/runs/"
	}
	for _, s := range []string{"K4", "K5"} {
		if _, ix := e.k7Cited(s); ix.ByID[id] != nil {
			return ix.ByID[id], "evidence/" + s + "/runs/"
		}
	}
	return nil, ""
}

func evalK74(e *Evidence) (string, []string, []string) {
	var p, d []string
	root := k7Root(e)
	res, err := readJSON(filepath.Join(e.Dir, "results.json"))
	if err != nil {
		return "results.json", []string{err.Error()}, nil
	}
	ix := e.k7()
	reps, rp := k7Repeats(e, ix)
	p = append(p, rp...)
	// The findings to package: the recorded ones and one per new failure.
	want := map[string]bool{}
	for _, id := range k7Recorded {
		want[id] = true
	}
	news, _ := res["new_failures"].([]any)
	newRuns := map[string]bool{}
	for _, x := range news {
		nf, _ := x.(map[string]any)
		newRuns[str(nf, "run")] = true
	}
	for id, r := range reps {
		if !r.Repeat && !newRuns[id] {
			p = append(p, fmt.Sprintf("FAIL %s matches no predicate (ADR-0045) and is not a new failure in results.json", id))
		}
		if r.Repeat && newRuns[id] {
			p = append(p, fmt.Sprintf("FAIL %s is a repeat (ADR-0045) but listed as a new failure", id))
		}
	}
	ents, err := os.ReadDir(filepath.Join(root, "findings"))
	if err != nil {
		return "findings", append(p, err.Error()), nil
	}
	pkgs := map[string]*k7Package{}
	for _, en := range ents {
		if !en.IsDir() {
			continue
		}
		id := en.Name()
		dir := filepath.Join(root, "findings", id)
		b, err := os.ReadFile(filepath.Join(dir, "finding.json"))
		if err != nil {
			p = append(p, fmt.Sprintf("findings/%s: %v", id, err))
			continue
		}
		var f k7Package
		if err := json.Unmarshal(b, &f); err != nil || f.ID != id {
			p = append(p, fmt.Sprintf("findings/%s/finding.json: %v (id %q)", id, err, f.ID))
			continue
		}
		pkgs[id] = &f
		p = append(p, e.checkK7Package(dir, &f)...)
	}
	// Every recorded finding and every new failure has a package.
	newOrig := map[string]bool{}
	for id := range pkgs {
		if !want[id] {
			newOrig[pkgs[id].Original.Run] = true
		}
	}
	for id := range want {
		if pkgs[id] == nil {
			p = append(p, fmt.Sprintf("recorded finding %s has no package", id))
		}
	}
	for run := range newRuns {
		if !newOrig[run] {
			p = append(p, fmt.Sprintf("new failure %s has no finding package", run))
		}
	}
	for id, f := range pkgs {
		if !want[id] && !newRuns[f.Original.Run] {
			p = append(p, fmt.Sprintf("findings/%s: its original run %s is not a new failure of K7's matrix nor a recorded finding", id, f.Original.Run))
		}
	}
	// Recurrences: every repeat is listed in the package of every finding
	// its predicates stand for, and nothing else is.
	wantRec := map[string]map[string]string{}
	for run, r := range reps {
		if !r.Repeat {
			continue
		}
		for _, g := range r.Groups {
			for _, fid := range g.Findings {
				if wantRec[fid] == nil {
					wantRec[fid] = map[string]string{}
				}
				wantRec[fid][run] = g.Predicate
			}
		}
	}
	for id, f := range pkgs {
		got := map[string]string{}
		for _, rc := range f.Recurrences {
			got[rc.Run] = rc.Predicate
			if r := ix.ByID[rc.Run]; r == nil || r.SealRoot != rc.SealRoot {
				p = append(p, fmt.Sprintf("findings/%s: recurrence %s is not a stored K7 run with seal %s", id, rc.Run, rc.SealRoot))
			}
		}
		for run, pr := range wantRec[id] {
			if got[run] != pr {
				p = append(p, fmt.Sprintf("findings/%s: recurrence %s (%s) not listed", id, run, pr))
			}
		}
		for run := range got {
			if _, ok := wantRec[id][run]; !ok {
				p = append(p, fmt.Sprintf("findings/%s: %s listed as a recurrence, ADR-0045's predicates do not give it", id, run))
			}
		}
		d = append(d, fmt.Sprintf("%s: %s %s %s %s %s, %d recurrences in K7, K7 replays %d of %d with %s (%s)", id, f.Classification["guarantee"], f.Classification["class"],
			f.Classification["claim_level"], f.Classification["severity"], f.Classification["reproducibility"], len(f.Recurrences), f.K7Replay.ReplaysWithClass,
			len(f.K7Replay.Runs), f.AnomalyClass, f.K7Replay.Reproducibility))
	}
	return "findings", p, d
}

// checkK7Package checks one §11.4 package.
func (e *Evidence) checkK7Package(dir string, f *k7Package) []string {
	var p []string
	bad := func(s string, a ...any) { p = append(p, fmt.Sprintf("findings/%s: ", f.ID)+fmt.Sprintf(s, a...)) }
	for _, n := range []string{"README.md", "plan.json", "checks.json", "claim.yaml", "report-excerpt.md"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			bad("%s missing", n)
		}
	}
	// README: the G1 line first, the upstream issue format (S50).
	rb, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	lines := strings.Split(string(rb), "\n")
	if m := k7G1Line.FindStringSubmatch(lines[0]); m == nil {
		bad("README's first line does not state whether an acknowledged write was lost (G1)")
	} else if (m[1] == "yes") != f.AcknowledgedWriteLost {
		bad("README's first line says %s, finding.json acknowledged_write_lost %v", m[1], f.AcknowledgedWriteLost)
	}
	title := ""
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, "# ") {
			title = l
			break
		}
	}
	if !k7Title.MatchString(title) {
		bad("no title in the `component: description` form (S50): %q", title)
	}
	for _, s := range k7Sections {
		if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(s) + `\s*$`).Match(rb) {
			bad("README lacks the section %q", s)
		}
	}
	cl := f.Classification
	g := cl["guarantee"]
	if k7GuaranteeLevel[g] == "" || cl["claim_level"] != k7GuaranteeLevel[g] || cl["class"] != k7Class(g) {
		bad("classification %v: guarantee %s has level %s and class %s (§3, §11.1)", cl, g, k7GuaranteeLevel[g], k7Class(g))
	}
	if sv := k7Severity(cl["class"], cl["claim_level"], g); cl["severity"] != sv {
		bad("severity %s, §11.2 derives %s", cl["severity"], sv)
	}
	if !in([]string{"MULTIGRES", "POSTGRES_INHERITED", "UNDETERMINED"}, cl["origin"]) || !in([]string{"NEW", "KNOWN", "KNOWN_GAP", "KNOWN_FIXED"}, cl["known_status"]) {
		bad("origin %q or known status %q outside §11.1", cl["origin"], cl["known_status"])
	}
	if cl["known_status"] == "KNOWN" && (!bytes.Contains(rb, []byte("S53")) || !bytes.Contains(rb, []byte("independent reproduction"))) {
		bad("a KNOWN finding's README must cite the upstream item (S53) and read as an independent reproduction (ADR-0043)")
	}
	// The runs.
	orig, src := e.k7SprintRun(f.Original.Run)
	min, msrc := e.k7SprintRun(f.Minimized.Run)
	switch {
	case orig == nil || len(orig.Problems) > 0:
		bad("original run %s not found or not sound in K4, K5 or K7's runs", f.Original.Run)
	case min == nil || len(min.Problems) > 0:
		bad("minimized run %s not found or not sound", f.Minimized.Run)
	default:
		for _, x := range []struct {
			r   *k4Run
			src string
			pr  k7PkgRun
		}{{orig, src, f.Original}, {min, msrc, f.Minimized}} {
			if x.r.Stub {
				bad("run %s is a summary; a finding's original and minimized runs are held in full (§11.4, ADR-0045)", x.r.ID)
			}
			a := filepath.Join(k7Root(e), x.src, x.r.ID+ArchiveSuffix)
			if x.pr.Archive != x.src+x.r.ID+ArchiveSuffix || FileSHA256(a) != x.pr.ArchiveSHA256 || x.pr.SealRoot != x.r.SealRoot ||
				x.pr.Verdict != x.r.Verdict || strings.Join(sortedCopy(x.pr.Classes), ",") != strings.Join(sortedCopy(x.r.Classes), ",") {
				bad("run %s: package says %s sha %.12s seal %.12s %s %v; the evidence: %s sha %.12s seal %.12s %s %v", x.r.ID, x.pr.Archive, x.pr.ArchiveSHA256,
					x.pr.SealRoot, x.pr.Verdict, x.pr.Classes, x.src+x.r.ID+ArchiveSuffix, FileSHA256(a), x.r.SealRoot, x.r.Verdict, x.r.Classes)
			}
		}
		if min.PlanSHA != f.MinimizedPlanSHA {
			bad("minimized plan %s, the minimized run's plan is %s", f.MinimizedPlanSHA, min.PlanSHA)
		}
		if pb, err := os.ReadFile(filepath.Join(dir, "plan.json")); err != nil || sha(bytes.TrimSuffix(pb, []byte("\n"))) != min.PlanSHA {
			bad("plan.json is not the minimized run's plan")
		}
		if cb, err := os.ReadFile(filepath.Join(dir, "checks.json")); err != nil || sha(cb) != orig.ChecksSHA {
			bad("checks.json is not the original run's")
		}
		if !orig.has(f.AnomalyClass) {
			bad("anomaly class %s not in the original run", f.AnomalyClass)
		}
		lost := orig.Valid["C1"] == "false" && orig.has("lost-write")
		if lost != f.AcknowledgedWriteLost {
			bad("acknowledged_write_lost %v, the original run's C1 says %v", f.AcknowledgedWriteLost, lost)
		}
		// Recorded replays: the minimized plan's P13 replays.
		hits := 0
		for _, rp := range f.Replays {
			r, _ := e.k7SprintRun(rp.Run)
			if r == nil || str(r.Manifest, k4ReplayOf) != min.ID || r.Verdict != rp.Verdict || r.SealRoot != rp.SealRoot {
				bad("replay %s: not a replay of %s with verdict %s and seal %.12s in the evidence", rp.Run, min.ID, rp.Verdict, rp.SealRoot)
				continue
			}
			if r.has(f.AnomalyClass) {
				hits++
			}
		}
		if len(f.Replays) != params.Replays || hits != f.ReplaysWithClass || k7Repro(hits) != cl["reproducibility"] {
			bad("%d replays, %d with %s; finding.json says %d and %s (§11.1, P13)", len(f.Replays), hits, f.AnomalyClass, f.ReplaysWithClass, cl["reproducibility"])
		}
		// §11.3: a K7 finding's id from its minimized plan; recorded ids stay.
		if !in(k7Recorded, f.ID) {
			h := sha256.Sum256([]byte(cl["class"] + "|" + g + "|" + f.AnomalyClass + "|" + min.PlanSHA))
			if want := "FND-" + f.Campaign + "-" + hex.EncodeToString(h[:])[:8]; want != f.ID {
				bad("§11.3 gives %s", want)
			}
		}
	}
	// K7.4: the package's replay command executed once more, its replays
	// stored.
	p = append(p, e.checkK7PkgReplay(f)...)
	if _, err := os.Stat(filepath.Join(k7Root(e), f.Triage)); err != nil || f.Triage == "" {
		bad("triage %q not found", f.Triage)
	}
	var cy map[string]any
	if b, err := os.ReadFile(filepath.Join(dir, "claim.yaml")); err != nil || yaml.Unmarshal(b, &cy) != nil {
		bad("claim.yaml unreadable")
	} else {
		for _, k := range []string{"id", "text", "contract", "target", "command", "artifact", "sha256", "result", "limitations"} {
			if cy[k] == nil || fmt.Sprint(cy[k]) == "" {
				bad("claim.yaml lacks %s (§17)", k)
			}
		}
	}
	return p
}

// k7ReplayArgv reports whether argv runs `replay <…/run> --times <P13>`:
// the tokens themselves, not substrings of the joined command line.
func k7ReplayArgv(argv []string, run string) bool {
	replay, dir, times := false, false, false
	for i, w := range argv {
		switch {
		case w == "replay":
			replay = true
		case filepath.Base(filepath.Clean(w)) == run:
			dir = true
		case w == "--times":
			times = i+1 < len(argv) && argv[i+1] == strconv.Itoa(params.Replays)
		}
	}
	return replay && dir && times
}

// checkK7PkgReplay checks K7.4's re-execution of a package's replay command:
// the record pkg-replay-<id> (the latest attempt), its summary, and its
// replays among package-replays/ (each a replay of the minimized run, its
// verdict, classes and seal as finding.json lists them; §11.1's class of
// the hits recomputed for the package's anomaly class).
func (e *Evidence) checkK7PkgReplay(f *k7Package) []string {
	var p []string
	bad := func(s string, a ...any) { p = append(p, fmt.Sprintf("findings/%s: ", f.ID)+fmt.Sprintf(s, a...)) }
	rec, ok := e.lastRecord("pkg-replay-" + f.ID)
	if !ok {
		bad("no pkg-replay-%s record (K7.4)", f.ID)
		return p
	}
	if f.K7Replay.Record != rec.Name || rec.ExitCode != 0 || !k7ReplayArgv(rec.Argv, f.Minimized.Run) {
		bad("record %s (argv %q exit %d) is not the package's replay command for %s with --times %d, exit 0", rec.Name, rec.Argv, rec.ExitCode,
			f.Minimized.Run, params.Replays)
		return p
	}
	_, so, err := e.Get(rec.Name)
	if err != nil {
		bad("%v", err)
		return p
	}
	sum, err := LastJSONLine(so)
	if err != nil {
		bad("%s: %v", rec.Name, err)
		return p
	}
	var listed []string
	for _, r := range f.K7Replay.Runs {
		listed = append(listed, r.Run)
	}
	if str(sum, "of") != f.Minimized.Run || strings.Join(strListOf(sum["replays"]), ",") != strings.Join(listed, ",") {
		bad("%s replayed %s into %v; finding.json lists %v of %s", rec.Name, str(sum, "of"), sum["replays"], listed, f.Minimized.Run)
	}
	pr := e.k7pkg()
	hits := 0
	for _, x := range f.K7Replay.Runs {
		r := pr.ByID[x.Run]
		if r == nil || len(r.Problems) > 0 || str(r.Manifest, k4ReplayOf) != f.Minimized.Run || r.Verdict != x.Verdict || r.SealRoot != x.SealRoot ||
			strings.Join(sortedCopy(r.Classes), ",") != strings.Join(sortedCopy(x.Classes), ",") {
			bad("K7 replay %s: not a stored replay of %s with verdict %s, seal %.12s, classes %v", x.Run, f.Minimized.Run, x.Verdict, x.SealRoot, x.Classes)
			continue
		}
		if r.Verdict == "FAIL" && r.has(f.AnomalyClass) {
			hits++
		}
	}
	if len(f.K7Replay.Runs) != params.Replays || hits != f.K7Replay.ReplaysWithClass || k7Repro(hits) != f.K7Replay.Reproducibility {
		bad("K7 replays: %d runs, %d with %s; finding.json says %d and %s (P13, §11.1)", len(f.K7Replay.Runs), hits, f.AnomalyClass,
			f.K7Replay.ReplaysWithClass, f.K7Replay.Reproducibility)
	}
	return p
}

func sabotageK74(dir string) error {
	root := filepath.Dir(filepath.Dir(dir))
	ents, err := os.ReadDir(filepath.Join(root, "findings"))
	if err != nil {
		return err
	}
	for _, en := range ents {
		if !en.IsDir() {
			continue
		}
		p := filepath.Join(root, "findings", en.Name(), "README.md")
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		i := bytes.Index(b, []byte("\n## Limitations"))
		if i < 0 {
			return fmt.Errorf("%s has no Limitations section", p)
		}
		j := bytes.Index(b[i+1:], []byte("\n## "))
		end := len(b)
		if j >= 0 {
			end = i + 1 + j
		}
		return os.WriteFile(p, append(append([]byte{}, b[:i]...), b[end:]...), 0o644)
	}
	return fmt.Errorf("no finding package")
}

// ---------------------------------------------------------------- K7.5

type k7Claim struct {
	ID          string            `yaml:"id"`
	Text        string            `yaml:"text"`
	Contract    string            `yaml:"contract"`
	Target      map[string]string `yaml:"target"`
	Command     string            `yaml:"command"`
	Artifact    string            `yaml:"artifact"`
	SHA256      string            `yaml:"sha256"`
	Result      string            `yaml:"result"`
	Limitations []string          `yaml:"limitations"`
}

type k7Claims struct {
	Claims     []k7Claim `yaml:"claims"`
	Hypotheses []struct {
		ID   string `yaml:"id"`
		Text string `yaml:"text"`
		Why  string `yaml:"why"`
	} `yaml:"hypotheses"`
}

// k7HypothesisMarkers: wording that marks a statement as a hypothesis (§17).
var k7HypothesisMarkers = []string{"hypothesis", "we expect", "not verified", "not tested", "untested", "may ", "might", "would", "not measured", "unknown"}

var (
	k7Tag = regexp.MustCompile(`\[(CLM|HYP)-[0-9]{4}\]`)
	// k7ClaimLine: a statement of a result (counts, verdicts, rates,
	// findings, measurements) — such a line must carry a tag.
	k7ClaimLine = regexp.MustCompile(`\b[0-9]+ of [0-9]+\b|\b[0-9]+/[0-9]+\b|\bPASS(ED)?\b|\bFAIL(ED|S)?\b|\b(NOT_)?DETECTED\b|\bREPRODUCED\b|\bINTERMITTENT\b|\bFND-[A-Z0-9]+-[0-9a-f]{8}\b|\bmeasured\b|\bobserved\b|\bconfirmed\b`)
	k7LimRow    = regexp.MustCompile(`^\| (LIM-[0-9]{2}) \|`)
)

// k7ClaimFiles are the files whose claims K7.5 audits.
var k7ClaimFiles = []string{"README.md", "docs/LIMITATIONS.md", "evidence/K7/NOTES.md"}

func evalK75(e *Evidence) (string, []string, []string) {
	var p, d []string
	root := k7Root(e)
	b, err := os.ReadFile(filepath.Join(root, "claims.yaml"))
	if err != nil {
		return "claims.yaml", []string{err.Error()}, nil
	}
	var cs k7Claims
	if err := yaml.Unmarshal(b, &cs); err != nil {
		return "claims.yaml", []string{err.Error()}, nil
	}
	lims := map[string]bool{}
	lb, _ := os.ReadFile(filepath.Join(root, "docs", "LIMITATIONS.md"))
	for _, l := range strings.Split(string(lb), "\n") {
		if m := k7LimRow.FindStringSubmatch(l); m != nil {
			lims[m[1]] = true
		}
	}
	ids := map[string]bool{}
	for _, c := range cs.Claims {
		bad := func(f string, a ...any) { p = append(p, c.ID+": "+fmt.Sprintf(f, a...)) }
		if ids[c.ID] || !regexp.MustCompile(`^CLM-[0-9]{4}$`).MatchString(c.ID) {
			bad("duplicate or malformed id")
		}
		ids[c.ID] = true
		if c.Text == "" || c.Contract == "" || c.Command == "" || c.Artifact == "" || c.SHA256 == "" || c.Result == "" || len(c.Limitations) == 0 ||
			c.Target["system"] == "" || c.Target["commit"] == "" || c.Target["profile"] == "" || c.Target["mutant"] == "" {
			bad("a §17 field is missing; a statement without one is a hypothesis")
		}
		for _, l := range c.Limitations {
			if !lims[l] {
				bad("limitation %s is not a LIMITATIONS row", l)
			}
		}
		got, err := k7ArtifactSHA(root, c.Artifact)
		if err != nil {
			bad("artifact %s: %v", c.Artifact, err)
		} else if got != c.SHA256 {
			bad("artifact %s hashes to %s, claims.yaml says %s", c.Artifact, got, c.SHA256)
		}
	}
	for _, h := range cs.Hypotheses {
		if ids[h.ID] || !regexp.MustCompile(`^HYP-[0-9]{4}$`).MatchString(h.ID) {
			p = append(p, h.ID+": duplicate or malformed hypothesis id")
		}
		ids[h.ID] = true
		low := strings.ToLower(h.Text)
		marked := false
		for _, m := range k7HypothesisMarkers {
			marked = marked || strings.Contains(low, m)
		}
		if !marked {
			p = append(p, fmt.Sprintf("%s: a hypothesis not worded as one (markers %v)", h.ID, k7HypothesisMarkers))
		}
	}
	// Every claim line of the audited files carries a tag; every tag exists;
	// every claim is referenced.
	used := map[string]bool{}
	for _, f := range k7ClaimFiles {
		fb, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			p = append(p, fmt.Sprintf("%s: %v", f, err))
			continue
		}
		fence := false
		for i, l := range strings.Split(string(fb), "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "```") {
				fence = !fence
				continue
			}
			for _, t := range k7Tag.FindAllString(l, -1) {
				id := strings.Trim(t, "[]")
				used[id] = true
				if !ids[id] {
					p = append(p, fmt.Sprintf("%s:%d: %s is not in claims.yaml", f, i+1, id))
				}
			}
			if fence || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "|---") || strings.HasPrefix(l, "| ID") {
				continue
			}
			if k7ClaimLine.MatchString(l) && !k7Tag.MatchString(l) {
				p = append(p, fmt.Sprintf("%s:%d: a claim without a claims.yaml id: %.120q", f, i+1, l))
			}
		}
	}
	for id := range ids {
		if !used[id] {
			p = append(p, fmt.Sprintf("%s is referenced by none of %v", id, k7ClaimFiles))
		}
	}
	d = append(d, fmt.Sprintf("%d claims, %d hypotheses; every tagged line of %v resolves", len(cs.Claims), len(cs.Hypotheses), k7ClaimFiles))
	return "claims.yaml", p, d
}

// k7ArtifactSHA: a run archive (full or stub) is identified by its seal
// root (§17's example), any other file by its sha256.
func k7ArtifactSHA(root, rel string) (string, error) {
	p := filepath.Join(root, filepath.FromSlash(rel))
	st, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		return "", fmt.Errorf("a directory; cite a file or a run archive")
	}
	if strings.HasSuffix(rel, ArchiveSuffix) {
		tmp, err := os.MkdirTemp("", "cpgate-claim-")
		if err != nil {
			return "", err
		}
		defer removeAll(tmp)
		if err := ExtractRun(p, tmp); err != nil {
			return "", err
		}
		id := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(rel), ArchiveSuffix), ".stub")
		var rep *history.SealReport
		if strings.HasSuffix(rel, StubSuffix) {
			rep = history.VerifyStub(filepath.Join(tmp, id))
		} else {
			rep = history.VerifySeal(filepath.Join(tmp, id))
		}
		if !rep.Valid {
			return "", fmt.Errorf("seal: %v", rep.Problems)
		}
		return rep.MerkleRoot, nil
	}
	return FileSHA256(p), nil
}

func sabotageK75(dir string) error {
	p := filepath.Join(filepath.Dir(filepath.Dir(dir)), "claims.yaml")
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`(?m)^(\s*sha256: ")([0-9a-f])`)
	loc := re.FindSubmatchIndex(b)
	if loc == nil {
		return fmt.Errorf("no sha256 in claims.yaml")
	}
	c := b[loc[4]]
	nc := byte('0')
	if c == '0' {
		nc = '1'
	}
	out := append([]byte{}, b...)
	out[loc[4]] = nc
	return os.WriteFile(p, out, 0o644)
}

// ---------------------------------------------------------------- K7.6

// k7Statement is §12.3's clean-result sentence for a cell (cpreport's
// wording up to the "Not tested" clause).
func k7Statement(c *k7Stat) (string, error) {
	zb, err := stats.ZeroFailureBound95(c.N)
	if err != nil {
		return "", err
	}
	var faults int64
	for _, v := range c.Confirmed {
		faults += v
	}
	return fmt.Sprintf("In %d valid runs of campaign %s (profile %s, Multigres %s), exposing %d ok writes to %d confirmed faults of kinds {%s}, no violation of {%s} was observed; at 95%% confidence the per-run violation probability under this exact schedule distribution is below %s.",
		c.N, c.Campaign, c.Profile, strings.Join(sortedKeys(c.Multigres), ", "), c.OKWrites, faults, strings.Join(sortedKeys(c.Confirmed), ", "),
		strings.Join(sortedKeys(c.Checkers), ", "), stats.Up(zb)), nil
}

func k7Clean(c *k7Stat) bool {
	return c.Mutant == "real" && c.N > 0 && c.Fail == 0 && c.Unk == 0 && c.FullRuns+c.ChecksRuns == c.N
}

func evalK76(e *Evidence) (string, []string, []string) {
	var p, d []string
	root := k7Root(e)
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		return "overclaim-review.md", []string{err.Error()}, nil
	}
	// The overclaim review: rows of | source | before | after | why |.
	ob, err := os.ReadFile(filepath.Join(e.Dir, "overclaim-review.md"))
	if err != nil {
		p = append(p, err.Error())
	} else {
		rows := 0
		for _, l := range strings.Split(string(ob), "\n") {
			if !strings.HasPrefix(l, "| ") || strings.HasPrefix(l, "| Source") || strings.HasPrefix(l, "|---") {
				continue
			}
			cells := strings.Split(strings.Trim(l, "| "), " | ")
			if len(cells) != 4 {
				p = append(p, fmt.Sprintf("overclaim-review.md: row without source, before, after and why: %.80q", l))
				continue
			}
			for _, c := range cells {
				if strings.TrimSpace(c) == "" {
					p = append(p, fmt.Sprintf("overclaim-review.md: empty field in %.80q", l))
				}
			}
			rows++
		}
		if rows == 0 {
			p = append(p, "overclaim-review.md lists no changed statement")
		}
		d = append(d, fmt.Sprintf("overclaim review: %d changed statements", rows))
	}
	// README: the first paragraph, the §4 matrix verbatim.
	paras := strings.Split(strings.TrimSpace(string(readme)), "\n\n")
	first := ""
	for _, pg := range paras {
		if !strings.HasPrefix(pg, "#") {
			first = pg
			break
		}
	}
	for _, w := range []string{"counterexamples", "bounded evidence", "not proofs"} {
		if !strings.Contains(first, w) {
			p = append(p, fmt.Sprintf("README's first paragraph does not say %q", w))
		}
	}
	cb, err := os.ReadFile(filepath.Join(root, "docs", "CONTRACT.md"))
	if err != nil {
		p = append(p, err.Error())
	} else {
		var table []string
		in4 := false
		for _, l := range strings.Split(string(cb), "\n") {
			if strings.HasPrefix(l, "## 4.") {
				in4 = true
				continue
			}
			if in4 && strings.HasPrefix(l, "## ") {
				break
			}
			if in4 && strings.HasPrefix(l, "|") {
				table = append(table, l)
			}
		}
		if len(table) < 3 || !bytes.Contains(readme, []byte(strings.Join(table, "\n"))) {
			p = append(p, "README does not contain the §4 matrix verbatim")
		}
	}
	// §12.3's sentence for every clean real-build cell: K7's and the cited
	// ones with complete exposure.
	var cells []*k7Stat
	if ix := e.k7(); ix.Error == nil {
		cs, _ := k7Stats(ix)
		cells = append(cells, cs...)
	}
	partial := false
	for _, s := range []string{"K4", "K5"} {
		_, cix := e.k7Cited(s)
		cs, _ := k7Stats(cix)
		for _, c := range cs {
			for _, cc := range k7CitedCells {
				if cc.Sprint == s && cc.Campaign == c.Campaign && cc.Profile == c.Profile && cc.Mutant == c.Mutant && c.Mutant == "real" {
					if c.Fail == 0 && c.Unk == 0 && c.FullRuns+c.ChecksRuns != c.N {
						partial = true
					}
					cells = append(cells, c)
				}
			}
		}
	}
	if _, err := os.Stat(filepath.Join(e.Dir, k7CitedSub)); err == nil {
		cs, _ := k7Stats(e.runIndexIn("k7cited", k7CitedSub, k5ContractOK))
		cells = append(cells, cs...)
		partial = false
	}
	n := 0
	for _, c := range cells {
		if !k7Clean(c) {
			continue
		}
		st, err := k7Statement(c)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		n++
		if !bytes.Contains(readme, []byte(st)) {
			p = append(p, fmt.Sprintf("README lacks §12.3's statement for %s/%s: %q", c.Campaign, c.Profile, st))
		}
	}
	if partial && !bytes.Contains(readme, []byte("0 of 5 runs held in full")) {
		p = append(p, "a cited clean cell without complete exposure must read \"0 of 5 runs held in full\" (ADR-0045 §5)")
	}
	d = append(d, fmt.Sprintf("%d clean cells, each with its §12.3 statement in the README", n))
	return "overclaim-review.md", p, d
}

func sabotageK76(dir string) error {
	p := filepath.Join(filepath.Dir(filepath.Dir(dir)), "README.md")
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`violation probability under this exact schedule distribution is below ([0-9])\.([0-9]{3})\.`)
	loc := re.FindSubmatchIndex(b)
	if loc == nil {
		return fmt.Errorf("no §12.3 statement in the README")
	}
	var v int
	fmt.Sscan(string(b[loc[4]:loc[5]]), &v)
	out := append([]byte{}, b[:loc[4]]...)
	out = append(out, []byte(fmt.Sprintf("%03d", (v+1)%1000))...)
	return os.WriteFile(p, append(out, b[loc[5]:]...), 0o644)
}

// ---------------------------------------------------------------- K7.7

// k7FirstCommit is K7's first commit (ADR-0045, before any K7 run).
const k7FirstCommit = "d07fca6c83729ee37972b2762d38a12b50f3e6e4"

var k7CIArgs = []string{"make", "ci-unit", "ci-checkers", "ci-cluster-smoke", "ci-nemesis-smoke", "ci-mutant-smoke", "ci-report"}

func evalK77(e *Evidence) (string, []string, []string) {
	var p, d []string
	name := lastCI(e)
	if name == "" {
		return "commands.jsonl", []string{"no ci record"}, nil
	}
	r, out, pc := exitOK(e, name, k7CIArgs...)
	p = append(p, pc...)
	_, _, pl := exitOK(e, "lint-workflow", "make", "lint-workflow")
	p = append(p, pl...)
	m := k6Ldflag.FindSubmatch(out)
	ciCommit, dirty := "", ""
	if m != nil {
		ciCommit, dirty = string(m[1]), string(m[2])
	}
	if ciCommit == "" || dirty != "false" {
		p = append(p, fmt.Sprintf("%s: the binaries' commit %q dirty %q (a clean commit's build is required)", name, ciCommit, dirty))
	}
	// The tag, recorded after the release commit: annotated, on a commit
	// whose code is the CI's, nothing pushed (no remote).
	trec, tout, pt := exitOK(e, lastTagCheck(e), "scripts/k7-tag-check.sh")
	p = append(p, pt...)
	if len(pt) == 0 && trec.Line < r.Line {
		p = append(p, "tag-check precedes the CI record")
	}
	t, err := LastJSONLine(tout)
	if err != nil {
		p = append(p, "tag-check: "+err.Error())
		return "commands.jsonl", p, d
	}
	if str(t, "tag") != "v1.0.0" || str(t, "object_type") != "tag" {
		p = append(p, fmt.Sprintf("tag %q of object type %q: an annotated v1.0.0 is required", str(t, "tag"), str(t, "object_type")))
	}
	if str(t, "ci_commit") != ciCommit {
		p = append(p, fmt.Sprintf("tag-check compares the tag with %s, the CI built %s", str(t, "ci_commit"), ciCommit))
	}
	if l := strListOf(t["code_diff"]); len(l) > 0 || t["code_diff"] == nil {
		p = append(p, fmt.Sprintf("the tagged commit's code differs from the CI's: %v", t["code_diff"]))
	}
	if l := strListOf(t["k7_on_remote"]); len(l) > 0 || t["k7_on_remote"] == nil {
		p = append(p, fmt.Sprintf("remote-tracking refs %v contain K7's first commit: nothing of K7 may be pushed (AGENTS.md §6)", t["k7_on_remote"]))
	}
	if str(t, "first_k7_commit") != k7FirstCommit {
		p = append(p, fmt.Sprintf("tag-check tested commit %s, K7's first is %s", str(t, "first_k7_commit"), k7FirstCommit))
	}
	d = append(d, fmt.Sprintf("%s exit %d on %s; v1.0.0 (%s) on %s; code diff %v; remote-tracking refs %v, none with K7's commits", name, r.ExitCode, ciCommit, str(t, "object_type"),
		str(t, "tag_commit"), t["code_diff"], t["remote_refs"]))
	return "commands.jsonl", p, d
}

func sabotageK77(dir string) error {
	e := LoadEvidence(dir)
	name := lastTagCheck(e)
	e.Close()
	return editSummary(dir, name, func(m map[string]any) { m["object_type"] = "commit" })
}

var tagCheckAttempt = regexp.MustCompile(`^tag-check(-[0-9]+)?$`)

// lastTagCheck: the latest tag-check attempt's name ("tag-check" if there is
// none). The tag is checked again after a later CI attempt, as lastCI reads
// the latest CI; earlier attempts stay recorded.
func lastTagCheck(e *Evidence) string {
	name := "tag-check"
	for _, r := range e.Records {
		if tagCheckAttempt.MatchString(r.Name) {
			name = r.Name
		}
	}
	return name
}

// ---------------------------------------------------------------- controls

// K7NegativeControls runs NegativeControls with K7's layout: each copy holds
// evidence/K7 and the release files two levels up (findings/, claims.yaml,
// README.md, CHANGELOG.md, docs/, campaigns/), with links to every other
// sprint's evidence (the cited K4/K5 runs, and the artifacts claims.yaml
// names under K2, K3, K6b, pre-K7, …), which no K7 sabotage touches. The
// links are absolute: a relative --evidence would leave them dangling.
func K7NegativeControls(dir string, crits []Criterion) []Control {
	root, rootErr := filepath.Abs(filepath.Dir(filepath.Dir(filepath.Clean(dir))))
	self := filepath.Base(filepath.Clean(dir))
	var out []Control
	for _, c := range crits {
		ctl := Control{Criterion: c.ID, Sabotage: c.Sabotage, Failed: []string{}, TargetProblems: []string{}}
		if rootErr != nil {
			ctl.Error = rootErr.Error()
			out = append(out, ctl)
			continue
		}
		tmp, err := os.MkdirTemp("", "cpgate-"+c.ID+"-")
		if err != nil {
			ctl.Error = err.Error()
			out = append(out, ctl)
			continue
		}
		func() {
			defer removeAll(tmp)
			ev := filepath.Join(tmp, "evidence", "K7")
			if err := copyDir(dir, ev); err != nil {
				ctl.Error = "copy: " + err.Error()
				return
			}
			for _, rel := range []string{"findings", "claims.yaml", "README.md", "CHANGELOG.md", "docs", "campaigns"} {
				src := filepath.Join(root, rel)
				st, err := os.Stat(src)
				if err != nil {
					continue
				}
				dst := filepath.Join(tmp, rel)
				if st.IsDir() {
					err = copyDir(src, dst)
				} else {
					err = copyFile(src, dst)
				}
				if err != nil {
					ctl.Error = "copy " + rel + ": " + err.Error()
					return
				}
			}
			sib, err := os.ReadDir(filepath.Join(root, "evidence"))
			if err != nil {
				ctl.Error = err.Error()
				return
			}
			for _, en := range sib {
				if en.Name() == self {
					continue
				}
				if err := os.Symlink(filepath.Join(root, "evidence", en.Name()), filepath.Join(tmp, "evidence", en.Name())); err != nil {
					ctl.Error = err.Error()
					return
				}
			}
			if err := c.Apply(ev); err != nil {
				ctl.Error = "sabotage: " + err.Error()
				return
			}
			for _, r := range Evaluate(ev, crits) {
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

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
