package gate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// K4 matrix (CONTRACT §15 K4; ADR-0024).
var (
	K4Campaigns    = []string{"C1", "C2", "C3", "C4", "C5", "C6", "C7"}
	K4CampaignSeed = []string{"1", "2", "3"}
	K4DirectedSeed = []string{"1", "2", "3", "4", "5"}
	K4Mutants      = []string{"M1", "M2", "M4"}
)

// D2′ (ADR-0027) is M2's directed schedule; the original D2 is superseded
// and its runs stay recorded (contract 1.0.6).
const (
	k4D2Prime      = "D2p"
	k4SupersededD2 = "D2"
)

// k4Directed: mutant → (directed campaign, expected class, checker).
var k4Directed = map[string]struct{ Campaign, Class, Checker string }{
	"M1": {"D1", "lost-write", "C1"},
	"M2": {k4D2Prime, "lost-write", "C1"},
	"M4": {"claims", "p1-not-rejected", "P"},
}

// K4 returns the eight criteria of CONTRACT §15 K4 with their sabotages.
func K4() []Criterion {
	return []Criterion{
		{ID: "K4.1", Name: "Quorum table", Eval: evalK41,
			Sabotage: "flip upstream_sufficient of row (n=4, N=2, r=2) in the quorum table and update the recorded table hash",
			Apply:    sabotageK41},
		{ID: "K4.2", Name: "Campaign runs", Eval: evalK42,
			Sabotage: "flip the verdict of the first C1 run in results.json",
			Apply:    sabotageK42},
		{ID: "K4.3", Name: "Mutant detection", Eval: evalK43,
			Sabotage: "rewrite the checks.json of M1's D1 runs for seeds 1 and 2 as PASS (seal roots kept)",
			Apply:    sabotageK43},
		{ID: "K4.4", Name: "Mutant isolation", Eval: evalK44,
			Sabotage: "replace multiorch's hash in the M1 image entry of the lock copy",
			Apply:    sabotageK44},
		{ID: "K4.5", Name: "Minimize and replay", Eval: evalK45,
			Sabotage: "delete the last replay of the minimized run of the first FAIL run of M1 (archive and checks.json)",
			Apply:    sabotageK45},
		{ID: "K4.6", Name: "Historical control", Eval: evalK46,
			Sabotage: "replace the recorded H1 outcome in h1.json with another outcome",
			Apply:    sabotageK46},
		{ID: "K4.7", Name: "Findings triage", Eval: evalK47,
			Sabotage: "delete one triage record (or, with none, add one for a run that is not a real-build FAIL)",
			Apply:    sabotageK47},
		{ID: "K4.8", Name: "CI-equivalent", Eval: evalK48,
			Sabotage: "replace the recorded exit code of the latest `make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke ci-mutant-smoke` with 1",
			Apply: func(dir string) error {
				last := lastCI(LoadEvidence(dir))
				return editRecords(dir, func(r map[string]any) {
					if r["name"] == last {
						r["exit_code"] = 1
					}
				})
			}},
	}
}

// ---- K4.1 ------------------------------------------------------------

func evalK41(e *Evidence) (string, []string, []string) {
	_, out, p := exitOK(e, "quorumxcheck", "quorumxcheck", "--out")
	if out == nil {
		return "quorum-table.json", p, nil
	}
	s, err := LastJSONLine(out)
	if err != nil {
		return "quorum-table.json", append(p, err.Error()), nil
	}
	tp := filepath.Join(e.Dir, "quorum-table.json")
	if got := FileSHA256(tp); got != str(s, "table_sha256") {
		p = append(p, fmt.Sprintf("quorum-table.json sha256 %s, recorded %s", got, str(s, "table_sha256")))
	}
	t, err := readJSON(tp)
	if err != nil {
		return "quorum-table.json", append(p, err.Error()), nil
	}
	if str(t, "multigres_commit") != params.MultigresCommit || str(t, "function") != "consensus.CheckSufficientRecruitment" {
		p = append(p, "quorum table is not of CheckSufficientRecruitment at the pinned commit")
	}
	seen := map[[3]int]bool{}
	mism, bc := 0, 0
	rows, _ := t["rows"].([]any)
	for _, x := range rows {
		row, _ := x.(map[string]any)
		n, N, rr := intOf(row["n"]), intOf(row["N"]), intOf(row["r"])
		k := [3]int{n, N, rr}
		if seen[k] {
			p = append(p, fmt.Sprintf("row %v twice", k))
		}
		seen[k] = true
		a, b, c := 2*rr > n, n-rr < N, rr+N > n
		suff := a && b
		if b != c {
			bc++
		}
		if fmt.Sprint(row["a_term_uniqueness"]) != fmt.Sprint(a) || fmt.Sprint(row["b_no_rogue_quorum"]) != fmt.Sprint(b) ||
			fmt.Sprint(row["c_flexpaxos_intersection"]) != fmt.Sprint(c) || fmt.Sprint(row["sufficient"]) != fmt.Sprint(suff) ||
			intOf(row["threshold"]) != max(n/2+1, n-N+1) {
			p = append(p, fmt.Sprintf("row %v: recorded terms differ from the definitions", k))
		}
		if fmt.Sprint(row["upstream_sufficient"]) != fmt.Sprint(suff) {
			mism++
			p = append(p, fmt.Sprintf("row %v: upstream CheckSufficientRecruitment says %v, the definitions %v", k, row["upstream_sufficient"], suff))
		}
	}
	for n := 1; n <= 9; n++ {
		for N := 1; N <= n; N++ {
			for rr := 0; rr <= n; rr++ {
				if !seen[[3]int{n, N, rr}] {
					p = append(p, fmt.Sprintf("row (n=%d, N=%d, r=%d) missing", n, N, rr))
				}
			}
		}
	}
	if len(seen) != 330 {
		p = append(p, fmt.Sprintf("%d rows, want 330", len(seen)))
	}
	var want []string
	for n := 1; n <= 9; n++ {
		for N := 1; N <= n; N++ {
			if n/2+1 == n-N+1 {
				want = append(want, fmt.Sprintf("%d/%d", n, N))
			}
		}
	}
	var got []string
	co, _ := t["coincident_terms"].([]any)
	for _, x := range co {
		m, _ := x.(map[string]any)
		got = append(got, fmt.Sprintf("%d/%d", intOf(m["n"]), intOf(m["N"])))
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		p = append(p, fmt.Sprintf("coincidence list %v, definitions give %v", got, want))
	}
	if intOf(s["mismatches"]) != 0 || mism != 0 || bc != 0 || intOf(s["b_iff_c_violations"]) != 0 {
		p = append(p, fmt.Sprintf("mismatches recorded %v recomputed %d; (b)⇔(c) violations %d", s["mismatches"], mism, bc))
	}
	at2 := []string{}
	for _, w := range want {
		if strings.HasSuffix(w, "/2") {
			at2 = append(at2, strings.TrimSuffix(w, "/2"))
		}
	}
	return "quorum-table.json", p, []string{fmt.Sprintf("%d rows, %d mismatches, coincident (n/N) %v; AT_LEAST_2 coincides for n ∈ %v", len(seen), mism, want, at2)}
}

func intOf(v any) int {
	switch x := v.(type) {
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	case float64:
		return int(x)
	case int64:
		return int(x)
	case int:
		return x
	}
	return -1
}

func sabotageK41(dir string) error {
	tp := filepath.Join(dir, "quorum-table.json")
	t, err := readJSON(tp)
	if err != nil {
		return err
	}
	rows, _ := t["rows"].([]any)
	done := false
	for _, x := range rows {
		row, _ := x.(map[string]any)
		if intOf(row["n"]) == 4 && intOf(row["N"]) == 2 && intOf(row["r"]) == 2 {
			row["upstream_sufficient"] = fmt.Sprint(row["upstream_sufficient"]) != "true"
			done = true
		}
	}
	if !done {
		return fmt.Errorf("row (4,2,2) not found")
	}
	b, err := canonjson.Marshal(t)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.WriteFile(tp, b, 0o644); err != nil {
		return err
	}
	return editSummary(dir, "quorumxcheck", func(m map[string]any) { m["table_sha256"] = sha(b) })
}

// ---- K4.2 ------------------------------------------------------------

// runOK checks a run's integrity for a criterion; stubs are accepted only
// where noted (derived runs of mutants and of H1, §13 summary).
func runOK(r *k4Run, allowStub bool) []string {
	var p []string
	for _, x := range r.Problems {
		p = append(p, r.ID+": "+x)
	}
	if r.Stub && !allowStub {
		p = append(p, r.ID+": committed as a stub; this criterion needs the full run")
	}
	return p
}

func evalK42(e *Evidence) (string, []string, []string) {
	ix := e.k4()
	if ix.Error != nil {
		return "runs/", []string{ix.Error.Error()}, nil
	}
	res, err := k4Results(e)
	if err != nil {
		return "results.json", []string{"results.json: " + err.Error()}, nil
	}
	var p, details []string
	for _, c := range K4Campaigns {
		var vs []string
		for _, s := range K4CampaignSeed {
			r, all, pp := ix.cell(c, "baseline", "real", s)
			p = append(p, pp...)
			for _, a := range all {
				p = append(p, runOK(a, false)...)
				p = append(p, checkResult(res, a)...)
			}
			if r != nil {
				vs = append(vs, fmt.Sprintf("seed %s %s%v", s, r.Verdict, r.Classes))
			}
		}
		details = append(details, fmt.Sprintf("%s: %s", c, strings.Join(vs, "; ")))
	}
	return "results.json", p, details
}

func sabotageK42(dir string) error {
	p := filepath.Join(dir, "results.json")
	m, err := readJSON(p)
	if err != nil {
		return err
	}
	rs, _ := m["runs"].([]any)
	for _, x := range rs {
		r, _ := x.(map[string]any)
		if str(r, "campaign") == "C1" && str(r, "kind") == "primary" {
			if str(r, "verdict") == "PASS" {
				r["verdict"] = "FAIL"
			} else {
				r["verdict"] = "PASS"
			}
			b, err := canonjson.Marshal(m)
			if err != nil {
				return err
			}
			return os.WriteFile(p, append(b, '\n'), 0o644)
		}
	}
	return fmt.Errorf("no C1 run in results.json")
}

// ---- K4.3 ------------------------------------------------------------

func evalK43(e *Evidence) (string, []string, []string) {
	ix := e.k4()
	if ix.Error != nil {
		return "runs/", []string{ix.Error.Error()}, nil
	}
	res, err := k4Results(e)
	if err != nil {
		return "results.json", []string{"results.json: " + err.Error()}, nil
	}
	var p, details []string
	for _, m := range K4Mutants {
		d := k4Directed[m]
		hits := 0
		var mv, rv []string
		for _, s := range K4DirectedSeed {
			r, all, pp := ix.cell(d.Campaign, "baseline", m, s)
			p = append(p, pp...)
			for _, a := range all {
				p = append(p, runOK(a, false)...)
				p = append(p, checkResult(res, a)...)
				if a.valid() && str(a.Manifest, "patch_sha256") == "" {
					p = append(p, a.ID+": mutant run without patch_sha256 in its manifest")
				}
			}
			if r != nil {
				if r.Verdict == "FAIL" && r.has(d.Class) && r.Valid[d.Checker] == "false" {
					hits++
				}
				mv = append(mv, fmt.Sprintf("%s%v", r.Verdict, r.Classes))
			}
			// The real build on the same schedule, recorded whatever it is.
			rr, rall, rp := ix.cell(d.Campaign, "baseline", "real", s)
			p = append(p, rp...)
			for _, a := range rall {
				p = append(p, runOK(a, false)...)
				p = append(p, checkResult(res, a)...)
			}
			if rr != nil {
				rv = append(rv, fmt.Sprintf("%s%v", rr.Verdict, rr.Classes))
				if m == "M4" && rr.Valid["P"] != "true" {
					p = append(p, fmt.Sprintf("real build probe run %s: checker P %s, want true (probe passes on real in 5/5)", rr.ID, rr.Valid["P"]))
				}
			}
		}
		need := params.DetectedAtLeast
		if m == "M4" {
			need = params.DirectedRuns // "probe fails on M4 in 5/5"
		}
		outcome := "DETECTED"
		if hits < need {
			outcome = "UNDETECTED"
			p = append(p, fmt.Sprintf("%s via %s: %d/%d runs FAIL with %s, need %d", m, d.Campaign, hits, len(K4DirectedSeed), d.Class, need))
		}
		details = append(details, fmt.Sprintf("%s via %s: %s %d/5 with %s; mutant %v; real %v", m, d.Campaign, outcome, hits, d.Class, mv, rv))
	}
	// The superseded D2 (ADR-0027): its runs stay recorded, unedited, and
	// are checked like any run; they no longer decide M2's detection.
	sup := map[string][]string{}
	for _, s := range K4DirectedSeed {
		for _, b := range []string{"M2", "real"} {
			r, all, pp := ix.cell(k4SupersededD2, "baseline", b, s)
			p = append(p, pp...)
			for _, a := range all {
				p = append(p, runOK(a, false)...)
				p = append(p, checkResult(res, a)...)
			}
			if r != nil {
				sup[b] = append(sup[b], fmt.Sprintf("%s%v", r.Verdict, r.Classes))
			}
		}
	}
	details = append(details, fmt.Sprintf("superseded %s (ADR-0027, recorded): mutant M2 %v; real %v", k4SupersededD2, sup["M2"], sup["real"]))
	return "runs/", p, details
}

func sabotageK43(dir string) error {
	e := LoadEvidence(dir)
	defer e.Close()
	ix := e.k4()
	for _, s := range []string{"1", "2"} {
		r, _, _ := ix.cell("D1", "baseline", "M1", s)
		if r == nil {
			return fmt.Errorf("no M1 D1 run for seed %s", s)
		}
		cp := filepath.Join(dir, "runs", r.ID+".checks.json")
		if err := editJSONCanonical(cp, func(m map[string]any) {
			m["verdict"] = "PASS"
			cs, _ := m["checkers"].([]any)
			for _, x := range cs {
				c, _ := x.(map[string]any)
				c["valid"], c["anomalies"] = true, []any{}
			}
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---- K4.4 ------------------------------------------------------------

var k4MultigresBinaries = []string{"/multigres/bin/multigres", "/multigres/bin/multiadmin", "/multigres/bin/multigateway",
	"/multigres/bin/multiorch", "/multigres/bin/multipooler", "/usr/bin/pgbackrest"}

func evalK44(e *Evidence) (string, []string, []string) {
	var p, details []string
	lock, err := readJSON(filepath.Join(e.Dir, "images.lock.json"))
	if err != nil {
		return "images.lock.json", []string{"images.lock.json: " + err.Error()}, nil
	}
	imgs, _ := lock["images"].(map[string]any)
	vars, _ := lock["variants"].(map[string]any)
	real, _ := imgs["commitproof/multigres:real"].(map[string]any)
	rb, _ := real["binaries"].(map[string]any)
	ix := e.k4()
	for _, m := range K4Mutants {
		if _, _, pp := exitOK(e, "images-build-"+m, "images", "build", "--mutant", m); len(pp) > 0 {
			p = append(p, pp...)
		}
		v, _ := vars[m].(map[string]any)
		img, _ := imgs["commitproof/multigres:"+m].(map[string]any)
		if v == nil || img == nil || str(img, "variant") != m {
			p = append(p, m+": no variant record or image entry in the lock")
			continue
		}
		patch, err := os.ReadFile(filepath.Join(e.Dir, "mutants", m+".patch"))
		if err != nil {
			p = append(p, m+": "+err.Error())
			continue
		}
		if sha(patch) != str(v, "patch_sha256") {
			p = append(p, fmt.Sprintf("%s: patch sha256 %s, lock %s", m, sha(patch), str(v, "patch_sha256")))
		}
		p = append(p, checkPatch(m, patch)...)
		mb, _ := img["binaries"].(map[string]any)
		iso, _ := v["isolation"].(map[string]any)
		changed := 0
		for _, bin := range k4MultigresBinaries {
			ie, _ := iso[bin].(map[string]any)
			if ie == nil {
				p = append(p, fmt.Sprintf("%s: no isolation entry for %s", m, bin))
				continue
			}
			base, mut := str(rb, bin), str(mb, bin)
			if str(ie, "baseline_sha256") != base || str(ie, "mutant_sha256") != mut || base == "" || mut == "" {
				p = append(p, fmt.Sprintf("%s %s: isolation record does not match the lock's hashes", m, bin))
			}
			contains := fmt.Sprint(ie["contains_patched_package"]) == "true"
			if !contains && base != mut {
				p = append(p, fmt.Sprintf("%s: %s does not link a patched package but differs from the real build", m, bin))
			}
			if contains && base != mut {
				changed++
			}
		}
		if changed == 0 {
			p = append(p, m+": no binary changed: the patch has no effect")
		}
		for _, r := range ix.Runs {
			if r.mutant() == m && str(r.Manifest, "patch_sha256") != str(v, "patch_sha256") {
				p = append(p, fmt.Sprintf("%s: run %s carries patch %s", m, r.ID, str(r.Manifest, "patch_sha256")))
			}
		}
		details = append(details, fmt.Sprintf("%s: %d binary(ies) linking %v differ; the others equal the real build", m, changed, v["files"]))
	}
	return "images.lock.json", p, details
}

// checkPatch: one file, one hunk, the §9.1 header (intent, failure mode,
// sources).
func checkPatch(m string, b []byte) []string {
	var p []string
	s := string(b)
	if strings.Count(s, "\ndiff --git ") != 1 {
		p = append(p, m+": the patch does not modify exactly one file")
	}
	if strings.Count(s, "\n@@ ") != 1 {
		p = append(p, m+": the patch has more than one hunk")
	}
	head, _, _ := strings.Cut(s, "\ndiff --git ")
	for _, k := range []string{"Intent:", "Failure mode modelled:", "Sources:"} {
		if !strings.Contains(head, k) {
			p = append(p, fmt.Sprintf("%s: header lacks %q", m, k))
		}
	}
	return p
}

func sabotageK44(dir string) error {
	return editJSON(filepath.Join(dir, "images.lock.json"), func(m map[string]any) {
		imgs, _ := m["images"].(map[string]any)
		img, _ := imgs["commitproof/multigres:M1"].(map[string]any)
		bins, _ := img["binaries"].(map[string]any)
		if bins != nil {
			bins["/multigres/bin/multiorch"] = strings.Repeat("0", 64)
		}
	})
}

// ---- K4.5 ------------------------------------------------------------

// k4FollowUp verifies the minimization and replays of a FAIL primary run.
func k4FollowUp(ix *k4Index, res map[string]map[string]any, dir string, orig *k4Run, allowStub bool) (string, []string) {
	return followUp(ix, res, dir, orig, allowStub, allowStub)
}

// followUp is k4FollowUp with separate stub allowances for the minimized
// run and for the replays (K5 commits a real-build failure's minimized run
// in full and its replays as stubs, ADR-0030).
func followUp(ix *k4Index, res map[string]map[string]any, dir string, orig *k4Run, allowStubMin, allowStubReplays bool) (string, []string) {
	var p []string
	class := k4TargetClass(orig)
	j, err := readJSON(filepath.Join(dir, "runs", orig.ID+".minimize.json"))
	if err != nil {
		return "", []string{fmt.Sprintf("%s: minimization journal: %v", orig.ID, err)}
	}
	if str(j, "original") != orig.ID || fmt.Sprint(j["done"]) != "true" || str(j, "target_class") != class {
		p = append(p, fmt.Sprintf("%s: journal (original %s, done %v, class %s) does not match (class %s)", orig.ID, str(j, "original"),
			j["done"], str(j, "target_class"), class))
	}
	min := ix.ByID[str(j, "minimized_run")]
	switch {
	case min == nil:
		return "", append(p, fmt.Sprintf("%s: minimized run %s not committed", orig.ID, str(j, "minimized_run")))
	case min != orig && (str(min.Manifest, k4CandidateOf) != orig.ID || !min.valid() || min.Verdict != "FAIL" || !min.has(class)):
		p = append(p, fmt.Sprintf("%s: minimized run %s is not a FAIL candidate of it with %s", orig.ID, min.ID, class))
	}
	p = append(p, runOK(min, allowStubMin)...)
	// The minimized plan derives from the original plan through the
	// candidates' plans (reduced_from chain).
	byPlan := map[string]*k4Run{}
	for _, r := range ix.Runs {
		if str(r.Manifest, k4CandidateOf) == orig.ID {
			byPlan[r.PlanSHA] = r
		}
	}
	cur, steps := min, 0
	for cur.PlanSHA != orig.PlanSHA && steps < 64 {
		parent := str(cur.Plan, "reduced_from")
		if parent == orig.PlanSHA {
			break
		}
		next := byPlan[parent]
		if next == nil {
			p = append(p, fmt.Sprintf("%s: plan chain of %s broken at %s", orig.ID, min.ID, parent))
			break
		}
		cur, steps = next, steps+1
	}
	hits, valid := 0, 0
	var reps []string
	for i := 1; i <= params.Replays; i++ {
		var got *k4Run
		for _, r := range ix.Runs {
			if str(r.Manifest, k4ReplayOf) == min.ID && fmt.Sprint(r.Manifest[k4ReplayIndex]) == fmt.Sprint(i) && r.valid() {
				got = r
			}
		}
		if got == nil {
			p = append(p, fmt.Sprintf("%s: replay %d of %s missing or INVALID", orig.ID, i, min.ID))
			continue
		}
		p = append(p, runOK(got, allowStubReplays)...)
		p = append(p, checkResult(res, got)...)
		if got.PlanSHA != min.PlanSHA {
			p = append(p, fmt.Sprintf("%s: replay %s ran another plan", orig.ID, got.ID))
		}
		valid++
		if got.Verdict == "FAIL" && got.has(class) {
			hits++
		}
		reps = append(reps, got.Verdict)
	}
	repro := "NOT_REPRODUCED"
	switch {
	case hits >= params.ReproducedAtLeast:
		repro = "REPRODUCED"
	case hits >= 1:
		repro = "INTERMITTENT"
	}
	if e := res[orig.ID]; e != nil {
		fu, _ := e["follow_up"].(map[string]any)
		if str(fu, "reproducibility") != repro || str(fu, "target_class") != class || str(fu, "minimized_run") != min.ID {
			p = append(p, fmt.Sprintf("%s: results.json follow-up %v differs (%s, %s, %s)", orig.ID, fu, repro, class, min.ID))
		}
	}
	return fmt.Sprintf("%s → %s (%s, %d reductions) replays %v: %d/%d with %s → %s", orig.ID, min.ID, class,
		len(strListOf(min.Plan["reductions"])), reps, hits, valid, class, repro), p
}

func strListOf(v any) []string {
	l, _ := v.([]any)
	var out []string
	for _, x := range l {
		out = append(out, fmt.Sprint(x))
	}
	return out
}

// reproIn returns the reproducibility class K4.5 (K5.5) computes for a FAIL
// run of a sprint's run index.
func reproIn(e *Evidence, ix *k4Index, r *k4Run) string {
	res, _ := k4Results(e)
	_, p := followUp(ix, res, e.Dir, r, true, true)
	if len(p) > 0 {
		return ""
	}
	j, _ := readJSON(filepath.Join(e.Dir, "runs", r.ID+".minimize.json"))
	min := ix.ByID[str(j, "minimized_run")]
	hits := 0
	for _, x := range ix.Runs {
		if min != nil && str(x.Manifest, k4ReplayOf) == min.ID && x.valid() && x.Verdict == "FAIL" && x.has(k4TargetClass(r)) {
			hits++
		}
	}
	switch {
	case hits >= params.ReproducedAtLeast:
		return "REPRODUCED"
	case hits >= 1:
		return "INTERMITTENT"
	}
	return "NOT_REPRODUCED"
}

func evalK45(e *Evidence) (string, []string, []string) {
	ix := e.k4()
	if ix.Error != nil {
		return "runs/", []string{ix.Error.Error()}, nil
	}
	res, err := k4Results(e)
	if err != nil {
		return "results.json", []string{"results.json: " + err.Error()}, nil
	}
	var p, details []string
	n := 0
	for _, r := range ix.Runs {
		if r.kind() != "primary" || !r.valid() || r.Verdict != "FAIL" {
			continue
		}
		n++
		// Derived runs of mutant and H1 failures may be stubs (§13); a
		// real-build failure's follow-ups are committed in full.
		allowStub := r.mutant() != "real" || r.profile() == "h1"
		d, pp := k4FollowUp(ix, res, e.Dir, r, allowStub)
		p = append(p, pp...)
		details = append(details, d)
	}
	details = append(details, fmt.Sprintf("%d FAIL runs minimized and replayed", n))
	return "runs/", p, details
}

func sabotageK45(dir string) error {
	e := LoadEvidence(dir)
	defer e.Close()
	ix := e.k4()
	for _, s := range K4DirectedSeed {
		r, _, _ := ix.cell("D1", "baseline", "M1", s)
		if r == nil || r.Verdict != "FAIL" {
			continue
		}
		j, err := readJSON(filepath.Join(dir, "runs", r.ID+".minimize.json"))
		if err != nil {
			return err
		}
		min := str(j, "minimized_run")
		for _, x := range ix.Runs {
			if str(x.Manifest, k4ReplayOf) == min && fmt.Sprint(x.Manifest[k4ReplayIndex]) == fmt.Sprint(params.Replays) {
				for _, f := range []string{x.ID + ArchiveSuffix, x.ID + StubSuffix, x.ID + ".checks.json"} {
					os.Remove(filepath.Join(dir, "runs", f))
				}
				return nil
			}
		}
		return fmt.Errorf("no replay %d of %s", params.Replays, min)
	}
	return fmt.Errorf("no FAIL run of M1")
}

// ---- K4.6 ------------------------------------------------------------

func evalK46(e *Evidence) (string, []string, []string) {
	ix := e.k4()
	if ix.Error != nil {
		return "runs/", []string{ix.Error.Error()}, nil
	}
	var p []string
	h, err := readJSON(filepath.Join(e.Dir, "h1.json"))
	if err != nil {
		return "h1.json", []string{"h1.json: " + err.Error()}, nil
	}
	if _, _, pp := exitOK(e, "images-build-h1", "images", "build", "--profile", "h1"); len(pp) > 0 {
		p = append(p, pp...)
	}
	res, _ := k4Results(e)
	detected, valid := 0, 0
	var vs []string
	for _, s := range K4DirectedSeed {
		r, all, _ := ix.cell("DH1", "h1", "real", s)
		for _, a := range all {
			p = append(p, runOK(a, false)...)
			p = append(p, checkResult(res, a)...)
			if str(a.Manifest, "multigres_commit") != params.H1Commit || str(a.Manifest, "variant") != "h1" {
				p = append(p, a.ID+": not a run of the historical commit's images")
			}
		}
		if r == nil {
			continue
		}
		valid++
		if r.Verdict == "FAIL" && r.has("lost-write") {
			detected++
		}
		vs = append(vs, fmt.Sprintf("%s%v", r.Verdict, r.Classes))
	}
	outcome := ""
	switch {
	case valid == len(K4DirectedSeed) && detected >= 1:
		outcome = "DETECTED"
	case valid == len(K4DirectedSeed):
		outcome = "NOT_DETECTED"
	case valid == 0:
		// NOT_DEPLOYABLE needs the derivation attempt: the images built and
		// a recorded cluster up of h1 that did not reach HEALTHY.
		if r, _, err := e.Get("h1-cluster-up"); err != nil || r.ExitCode == 0 {
			p = append(p, "no valid DH1 run and no failed `cluster up --profile h1` record")
		} else {
			outcome = "NOT_DEPLOYABLE"
		}
	default:
		p = append(p, fmt.Sprintf("%d of %d DH1 runs valid", valid, len(K4DirectedSeed)))
	}
	if str(h, "outcome") != outcome {
		p = append(p, fmt.Sprintf("h1.json outcome %q, the runs give %q", str(h, "outcome"), outcome))
	}
	return "h1.json", p, []string{fmt.Sprintf("H1 %s: %d/%d DH1 runs with lost-write; %v", outcome, detected, valid, vs)}
}

func sabotageK46(dir string) error {
	return editJSON(filepath.Join(dir, "h1.json"), func(m map[string]any) {
		if str(m, "outcome") == "DETECTED" {
			m["outcome"] = "NOT_DETECTED"
		} else {
			m["outcome"] = "DETECTED"
		}
	})
}

// ---- K4.7 ------------------------------------------------------------

// Closed values of §11.1 and the guarantee claim levels of §3.
var (
	k4Classes      = []string{"SAFETY_VIOLATION", "PROTOCOL_DEVIATION", "LIVENESS_OBSERVATION", "HARNESS_DEFECT"}
	k4Origins      = []string{"MULTIGRES", "POSTGRES_INHERITED", "UNDETERMINED"}
	k4Known        = []string{"NEW", "KNOWN_GAP", "KNOWN_FIXED"}
	k4ClaimLevel   = map[string]string{"G1": "EXPLICIT", "G2": "EXPLICIT", "G3": "EXPLICIT", "G4": "INFERRED", "G5": "INFERRED", "G6": "DESIGN", "G7": "DESIGN", "G8": "DESIGN", "G9": "DESIGN", "G10": "DESIGN", "G11": "DESIGN", "G12": "DESIGN", "G13": "INFERRED"}
	k4RealCampaign = map[string]bool{"C1": true, "C2": true, "C3": true, "C4": true, "C5": true, "C6": true, "C7": true, "D1": true, "D2": true, k4D2Prime: true, "claims": true}
)

func in(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// k4Severity is §11.2's derivation.
func k4Severity(class, level, guarantee string) string {
	switch {
	case class == "SAFETY_VIOLATION" && level == "EXPLICIT":
		return "critical"
	case class == "SAFETY_VIOLATION", class == "PROTOCOL_DEVIATION" && level == "DESIGN" && guarantee == "G7":
		return "high"
	case class == "PROTOCOL_DEVIATION":
		return "medium"
	case class == "LIVENESS_OBSERVATION":
		return "info"
	}
	return ""
}

// k4FindingID is §11.3: FND-<campaign>-<sha256(class|guarantee|anomaly
// class|minimized plan hash)[:8]>.
func k4FindingID(campaign, class, guarantee, anomaly, planSHA string) string {
	s := sha256.Sum256([]byte(class + "|" + guarantee + "|" + anomaly + "|" + planSHA))
	return "FND-" + campaign + "-" + hex.EncodeToString(s[:])[:8]
}

// k4Row is the §6.2 row a recorded completion must have been classified by.
func k4RowConsistent(typ string, row int) bool {
	switch typ {
	case "ok":
		return row == 1
	case "fail":
		return row >= 2 && row <= 7 || row == 10
	case "info":
		return row == 8 || row == 9 || row == 11 || row == 12
	}
	return false
}

func evalK47(e *Evidence) (string, []string, []string) {
	ix := e.k4()
	if ix.Error != nil {
		return "runs/", []string{ix.Error.Error()}, nil
	}
	var p, details []string
	want := map[string]*k4Run{}
	for _, r := range ix.Runs {
		if r.kind() == "primary" && r.mutant() == "real" && r.profile() == "baseline" && k4RealCampaign[r.campaign()] && r.valid() && r.Verdict == "FAIL" {
			want[r.ID] = r
		}
	}
	tdir := filepath.Join(e.Dir, "triage")
	ents, _ := os.ReadDir(tdir)
	have := map[string]bool{}
	for _, ent := range ents {
		if !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(ent.Name(), ".json")
		have[id] = true
		r := want[id]
		if r == nil {
			p = append(p, fmt.Sprintf("triage/%s: not a real-build FAIL run of K4", ent.Name()))
			continue
		}
		t, err := readJSON(filepath.Join(tdir, ent.Name()))
		if err != nil {
			p = append(p, "triage/"+ent.Name()+": "+err.Error())
			continue
		}
		pp, d := checkTriage(e, e.k4(), r, t)
		p = append(p, pp...)
		details = append(details, d)
	}
	for id := range want {
		if !have[id] {
			p = append(p, fmt.Sprintf("real-build FAIL %s has no triage record", id))
		}
	}
	details = append(details, fmt.Sprintf("%d real-build FAIL runs, %d triage records", len(want), len(have)))
	sort.Strings(details)
	return "triage/", p, details
}

func checkTriage(e *Evidence, ix *k4Index, r *k4Run, t map[string]any) ([]string, string) {
	var p []string
	bad := func(f string, a ...any) { p = append(p, fmt.Sprintf("triage %s: ", r.ID)+fmt.Sprintf(f, a...)) }
	if str(t, "run_id") != r.ID || str(t, "seal_root") != r.SealRoot || str(t, "checks_sha256") != r.ChecksSHA {
		bad("run id, seal or checks.json hash do not match the run")
	}
	if got := strings.Join(strListOf(t["classes"]), ","); got != strings.Join(r.Classes, ",") {
		bad("classes %s, checks.json %s", got, strings.Join(r.Classes, ","))
	}
	// Elle cross-check of C3 anomalies (§10.11): cpcheck --elle-oracle's
	// result in checks.json stats.elle or, when cpcheck's three passes could
	// not finish, recorded elle-cli runs of the validity pass. A finding the
	// oracle does not confirm (other classes, or no answer) is labelled
	// oracle-disagreement and triaged.
	if r.Valid["C3"] == "false" {
		p = append(p, checkOracle(e, r, t)...)
	}
	c, _ := t["classification"].(map[string]any)
	cl, g, lvl := str(c, "class"), str(c, "guarantee"), str(c, "claim_level")
	if !in(k4Classes, cl) || k4ClaimLevel[g] == "" || lvl != k4ClaimLevel[g] || !in(k4Origins, str(c, "origin")) || !in(k4Known, str(c, "known_status")) {
		bad("classification %v outside §11.1 or the guarantee's claim level (%s)", c, k4ClaimLevel[g])
	}
	if cl == "HARNESS_DEFECT" {
		bad("a HARNESS_DEFECT is fixed and re-run, never triaged as a finding (§11.1)")
	}
	if sv := k4Severity(cl, lvl, g); str(c, "severity") != sv {
		bad("severity %s, §11.2 derives %s", str(c, "severity"), sv)
	}
	if rp := reproIn(e, ix, r); str(c, "reproducibility") != rp {
		bad("reproducibility %s, K4.5 computes %q", str(c, "reproducibility"), rp)
	}
	anomaly := str(t, "anomaly_class")
	if !r.has(anomaly) {
		bad("anomaly class %s not in the run", anomaly)
	}
	j, _ := readJSON(filepath.Join(e.Dir, "runs", r.ID+".minimize.json"))
	min := ix.ByID[str(j, "minimized_run")]
	if min != nil {
		if id := k4FindingID(r.campaign(), cl, g, anomaly, min.PlanSHA); str(t, "finding_id") != id {
			bad("finding id %s, §11.3 gives %s", str(t, "finding_id"), id)
		}
	}
	// §6.2 review: every op of the anomaly class, its recorded completion
	// and the table row it was classified by.
	hist, err := history.ReadChain(filepath.Join(r.Dir, history.FileHistory))
	if err != nil {
		bad("history: %v", err)
		return p, ""
	}
	comp := map[int64]history.Event{}
	for _, ev := range hist {
		if ev.Op != nil {
			comp[*ev.Op] = ev
		}
	}
	reviewed := map[int64]bool{}
	rv, _ := t["completion_review"].([]any)
	for _, x := range rv {
		m, _ := x.(map[string]any)
		op := int64(intOf(m["op"]))
		ev, ok := comp[op]
		if !ok {
			bad("review of op %d: no completion in the history", op)
			continue
		}
		if str(m, "completion") != ev.Type || !k4RowConsistent(ev.Type, intOf(m["row"])) {
			bad("review of op %d: completion %s row %v, history %s", op, str(m, "completion"), m["row"], ev.Type)
		}
		reviewed[op] = true
	}
	for _, a := range r.Anoms {
		if a.Class != anomaly {
			continue
		}
		for _, op := range a.Ops {
			if !reviewed[op] {
				bad("op %d of %s not reviewed against §6.2", op, anomaly)
			}
		}
	}
	if fmt.Sprint(t["harness_defect"]) != "false" || strings.TrimSpace(str(t, "harness_review")) == "" {
		bad("no harness-defect review")
	}
	return p, fmt.Sprintf("%s: %s %s %s %s %s", r.ID, str(t, "finding_id"), cl, g, str(c, "reproducibility"), str(c, "severity"))
}

func sabotageK47(dir string) error {
	tdir := filepath.Join(dir, "triage")
	ents, _ := os.ReadDir(tdir)
	for _, ent := range ents {
		if strings.HasSuffix(ent.Name(), ".json") {
			return os.Remove(filepath.Join(tdir, ent.Name()))
		}
	}
	if err := os.MkdirAll(tdir, 0o755); err != nil {
		return err
	}
	b, _ := json.Marshal(map[string]any{"run_id": "no-such-run"})
	return os.WriteFile(filepath.Join(tdir, "no-such-run.json"), b, 0o644)
}

// ---- K4.8 ------------------------------------------------------------

var ciAttempt = regexp.MustCompile(`^ci(-[0-9]+)?$`)

// lastCI is the name of the latest recorded CI attempt ("ci", "ci-2", …).
func lastCI(e *Evidence) string {
	name := ""
	for _, r := range e.Records {
		if ciAttempt.MatchString(r.Name) {
			name = r.Name
		}
	}
	return name
}

// evalK48: the latest CI attempt ran every CI target and exited 0 (earlier
// failed attempts stay recorded and are listed).
func evalK48(e *Evidence) (string, []string, []string) {
	name := lastCI(e)
	if name == "" {
		return "commands.jsonl", []string{"no ci record"}, nil
	}
	r, _, p := exitOK(e, name, "make", "ci-unit", "ci-checkers", "ci-cluster-smoke", "ci-nemesis-smoke", "ci-mutant-smoke")
	var earlier []string
	for _, x := range e.Records {
		if ciAttempt.MatchString(x.Name) && x.Name != name {
			earlier = append(earlier, fmt.Sprintf("%s exit %d", x.Name, x.ExitCode))
		}
	}
	return "commands.jsonl", p, []string{fmt.Sprintf("%s: argv %q exit %d; earlier attempts %v", name, r.Argv, r.ExitCode, earlier)}
}

// elleName maps an Elle 0.2.7 anomaly name to CONTRACT §10.3's ("" for
// informational names).
func elleName(n string) string {
	switch n {
	case "empty-transaction-graph":
		return ""
	case "G1a":
		return "g1a"
	case "G1b":
		return "g1b"
	}
	n = strings.Replace(n, "G-single-item", "G-single", 1)
	return strings.Replace(n, "G-nonadjacent-item", "G-nonadjacent", 1)
}

func c3Classes(r *k4Run) []string {
	var out []string
	seen := map[string]bool{}
	for _, a := range r.Anoms {
		if a.Checker == "C3" && !seen[a.Class] {
			seen[a.Class] = true
			out = append(out, a.Class)
		}
	}
	sort.Strings(out)
	return out
}

// checkOracle verifies a triage record's oracle field against the evidence.
func checkOracle(e *Evidence, r *k4Run, t map[string]any) []string {
	var p []string
	bad := func(f string, a ...any) { p = append(p, fmt.Sprintf("triage %s oracle: ", r.ID)+fmt.Sprintf(f, a...)) }
	o, _ := t["oracle"].(map[string]any)
	if o == nil {
		return []string{fmt.Sprintf("triage %s: C3 anomalies without an oracle record", r.ID)}
	}
	result := str(o, "result")
	var elle map[string]any
	cs, _ := r.Checks["checkers"].([]any)
	for _, x := range cs {
		c, _ := x.(map[string]any)
		if str(c, "id") == "C3" {
			st, _ := c["stats"].(map[string]any)
			elle, _ = st["elle"].(map[string]any)
		}
	}
	if elle != nil {
		agree := fmt.Sprint(elle["agree"]) == "true"
		if fmt.Sprint(o["agree"]) != fmt.Sprint(agree) || str(o, "elle_valid") != fmt.Sprint(elle["valid"]) {
			bad("%v differs from checks.json stats.elle %v", o, elle)
		}
		if want := map[bool]string{true: "confirmed", false: "oracle-disagreement"}[agree]; result != want {
			bad("result %q, cpcheck's oracle says %s", result, want)
		}
		if !agree && strings.TrimSpace(str(o, "triage")) == "" {
			bad("oracle-disagreement without its triage")
		}
		return p
	}
	recs := strListOf(o["records"])
	if len(recs) == 0 {
		return append(p, fmt.Sprintf("triage %s: no stats.elle in checks.json and no recorded elle-cli run", r.ID))
	}
	answered, confirmed := false, false
	for _, name := range recs {
		rec, out, err := e.Get(name)
		if err != nil {
			bad("%v", err)
			continue
		}
		if !argvHas(rec, "commitproof/elle-oracle", r.ID+".elle") {
			bad("record %s is not elle-cli on this run's export", name)
			continue
		}
		var v map[string]any
		if rec.ExitCode != 1 && rec.ExitCode != 0 || json.Unmarshal(out, &v) != nil || strings.Contains(strings.Join(rec.Argv, " "), "-process") {
			continue // no answer from this run (e.g. OutOfMemoryError) or not the validity pass
		}
		answered = true
		var got []string
		for _, a := range strListOf(v["anomaly-types"]) {
			if n := elleName(a); n != "" {
				got = append(got, n)
			}
		}
		sort.Strings(got)
		confirmed = fmt.Sprint(v["valid?"]) == "false" && r.Valid["C3"] == "false" && strings.Join(got, ",") == strings.Join(c3Classes(r), ",")
	}
	want := "oracle-disagreement"
	if answered && confirmed {
		want = "confirmed"
	}
	if result != want {
		bad("result %q, the recorded elle-cli runs give %s", result, want)
	}
	if want != "confirmed" && strings.TrimSpace(str(o, "triage")) == "" {
		bad("oracle-disagreement without its triage")
	}
	return p
}
