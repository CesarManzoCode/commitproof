package gate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

// K6 evidence layout (CONTRACT §15 K6; ADR-0036).
const (
	k6RefDir    = "reference-run" // the reference run archive and its checks.json
	k6ReportDir = "report"        // the committed example: render-1's output
	k6Renders   = 3
	k6Sabotage  = "report_nondeterminism"
)

// K6 returns the K6 criteria (CONTRACT §15 K6).
func K6() []Criterion {
	return []Criterion{
		{ID: "K6.1", Name: "Byte reproducibility", Eval: evalK61,
			Sabotage: "checker-level: the three render records replaced by the three renders of a cpreport built with cp_sabotage_report_nondeterminism (records' hashes updated)",
			Apply:    sabotageK61},
		{ID: "K6.2", Name: "Forced rebuild", Eval: evalK62,
			Sabotage: "the fresh-clone render's recorded sha256 altered (record hash updated)",
			Apply: func(dir string) error {
				return editSummary(dir, "fresh-clone", func(m map[string]any) { m["render_sha256"] = flipHex(str(m, "render_sha256")) })
			}},
		{ID: "K6.3", Name: "Completeness", Eval: evalK63,
			Sabotage: "one operation's completion index removed from the example report's elements; every recorded render and fresh-clone hash rewritten to the forged file's, so only K6.3's own reading can tell",
			Apply: func(dir string) error {
				return forgeReport(dir, func(b []byte) ([]byte, error) {
					re := regexp.MustCompile(`data-ev="([0-9]+) [0-9]+"`)
					loc := re.FindSubmatchIndex(b)
					if loc == nil {
						return nil, fmt.Errorf("no operation element in the report")
					}
					out := append([]byte{}, b[:loc[0]]...)
					out = append(out, []byte(`data-ev="`+string(b[loc[2]:loc[3]])+`"`)...)
					return append(out, b[loc[1]:]...), nil
				})
			}},
		{ID: "K6.4", Name: "Seal enforcement", Eval: evalK64,
			Sabotage: "the seal-enforcement record rewritten as if the tampered run had rendered (exit 0, output written)",
			Apply: func(dir string) error {
				return editSummary(dir, "seal-enforcement", func(m map[string]any) {
					cs, _ := m["cases"].([]any)
					for _, c := range cs {
						if cm, _ := c.(map[string]any); cm != nil && cm["expect"] == "refused" {
							cm["cpreport_exit"], cm["out_exists"], cm["out_files"] = json.Number("0"), true, json.Number("1")
							break
						}
					}
				})
			}},
		{ID: "K6.5", Name: "Failover metrics", Eval: evalK65,
			Sabotage: "the first gap_ns of the example report's data block increased by 1 ns; every recorded render and fresh-clone hash rewritten to the forged file's",
			Apply: func(dir string) error {
				return forgeReport(dir, func(b []byte) ([]byte, error) {
					re := regexp.MustCompile(`"gap_ns":([0-9]+)`)
					loc := re.FindSubmatchIndex(b)
					if loc == nil {
						return nil, fmt.Errorf("no gap_ns in the data block")
					}
					n, err := strconv.ParseInt(string(b[loc[2]:loc[3]]), 10, 64)
					if err != nil {
						return nil, err
					}
					out := append([]byte{}, b[:loc[2]]...)
					out = append(out, []byte(strconv.FormatInt(n+1, 10))...)
					return append(out, b[loc[3]:]...), nil
				})
			}},
		{ID: "K6.6", Name: "CI-equivalent", Eval: evalK66,
			Sabotage: "replace the recorded exit code of the latest `make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke ci-mutant-smoke ci-report` with 1",
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

func flipHex(s string) string {
	if s == "" {
		return "0"
	}
	c := byte('0')
	if s[0] == '0' {
		c = '1'
	}
	return string(c) + s[1:]
}

// k6Ref returns the reference run's id: the one <id>.tar.zst under
// reference-run/.
func k6Ref(e *Evidence) (string, error) {
	ents, err := os.ReadDir(filepath.Join(e.Dir, k6RefDir))
	if err != nil {
		return "", err
	}
	var ids []string
	for _, x := range ents {
		if n := x.Name(); strings.HasSuffix(n, ArchiveSuffix) && !strings.HasSuffix(n, StubSuffix) {
			ids = append(ids, strings.TrimSuffix(n, ArchiveSuffix))
		}
	}
	if len(ids) != 1 {
		return "", fmt.Errorf("%s/ holds %d run archives, want exactly 1", k6RefDir, len(ids))
	}
	return ids[0], nil
}

// k6Render checks one recorded `cpreport render <reference> --out …` and
// returns its output's sha256.
func k6Render(e *Evidence, name, ref string, wantSabotage bool) (string, []string) {
	r, out, err := e.Get(name)
	if err != nil {
		return "", []string{err.Error()}
	}
	var p []string
	if r.ExitCode != 0 {
		p = append(p, fmt.Sprintf("%s exited %d", name, r.ExitCode))
	}
	if !argvHas(r, "cpreport", "render", k6RefDir+"/"+ref+ArchiveSuffix, "--out") {
		p = append(p, fmt.Sprintf("%s argv %q does not render the reference run", name, r.Argv))
	}
	s, err := LastJSONLine(out)
	if err != nil {
		return "", append(p, name+": "+err.Error())
	}
	if str(s, "mode") != "run" || str(s, "run_id") != ref {
		p = append(p, fmt.Sprintf("%s rendered %q in mode %q", name, str(s, "run_id"), str(s, "mode")))
	}
	sab, _ := s["sabotage"].([]any)
	switch {
	case wantSabotage && (len(sab) != 1 || sab[0] != k6Sabotage):
		p = append(p, fmt.Sprintf("%s: binary sabotage %v, want [%s]", name, sab, k6Sabotage))
	case !wantSabotage && len(sab) > 0:
		p = append(p, fmt.Sprintf("%s: a sabotaged binary (%v)", name, sab))
	}
	outs, _ := s["outputs"].([]any)
	if len(outs) != 1 {
		return "", append(p, fmt.Sprintf("%s wrote %d files, want 1", name, len(outs)))
	}
	o, _ := outs[0].(map[string]any)
	if str(o, "file") != ref+".html" || !hex64.MatchString(str(o, "sha256")) {
		p = append(p, fmt.Sprintf("%s output %v", name, o))
	}
	return str(o, "sha256"), p
}

func k6Example(e *Evidence, ref string) string { return filepath.Join(e.Dir, k6ReportDir, ref+".html") }

// evalK61: three renders of the reference run, byte-identical among
// themselves and to the committed example; the sabotaged build's three
// renders are not (the check can fail).
func evalK61(e *Evidence) (string, []string, []string) {
	ref, err := k6Ref(e)
	if err != nil {
		return k6RefDir, []string{err.Error()}, nil
	}
	ev := filepath.ToSlash(filepath.Join(k6ReportDir, ref+".html"))
	var p, d []string
	hashes := map[string]bool{}
	for n := 1; n <= k6Renders; n++ {
		h, pr := k6Render(e, fmt.Sprintf("render-%d", n), ref, false)
		p = append(p, pr...)
		hashes[h] = true
		d = append(d, fmt.Sprintf("render-%d sha256 %s", n, h))
	}
	if len(hashes) != 1 {
		p = append(p, fmt.Sprintf("the %d renders produced %d different outputs", k6Renders, len(hashes)))
	}
	ex := FileSHA256(k6Example(e, ref))
	if !hashes[ex] {
		p = append(p, fmt.Sprintf("the committed example %s (sha256 %q) is not the renders' output", ev, ex))
	}
	if _, _, pb := exitOK(e, "sabotage-build", "go", "build", "-tags", "cp_sabotage_"+k6Sabotage, "./cmd/cpreport"); len(pb) > 0 {
		p = append(p, pb...)
	}
	sab := map[string]bool{}
	for n := 1; n <= k6Renders; n++ {
		h, pr := k6Render(e, fmt.Sprintf("sabotage-render-%d", n), ref, true)
		p = append(p, pr...)
		sab[h] = true
	}
	if len(sab) < 2 {
		p = append(p, "the sabotaged build's three renders are identical: the nondeterminism sabotage did not show, so K6.1 is not shown to be able to fail")
	}
	d = append(d, fmt.Sprintf("example sha256 %s; sabotaged build: %d distinct outputs of %d", ex, len(sab), k6Renders))
	return ev, p, d
}

// evalK62: a fresh clone of HEAD, built with emptied caches, renders the
// reference run to the same bytes.
func evalK62(e *Evidence) (string, []string, []string) {
	ref, err := k6Ref(e)
	if err != nil {
		return k6RefDir, []string{err.Error()}, nil
	}
	r, out, p := exitOK(e, "fresh-clone", "scripts/k6-fresh-clone.sh", k6RefDir+"/"+ref+ArchiveSuffix)
	if out == nil {
		return "commands.jsonl", p, nil
	}
	s, err := LastJSONLine(out)
	if err != nil {
		return "commands.jsonl", append(p, err.Error()), nil
	}
	commit := str(s, "commit")
	if !hex40.MatchString(commit) {
		p = append(p, fmt.Sprintf("fresh-clone commit %q", commit))
	}
	for _, k := range []string{"clone_exit", "go_clean_exit", "build_exit", "render_exit"} {
		if v, ok := s[k].(json.Number); !ok || v.String() != "0" {
			p = append(p, fmt.Sprintf("fresh-clone %s = %v, want 0", k, s[k]))
		}
	}
	if str(s, "go_clean") != "go clean -cache -modcache" || s["caches_emptied"] != true {
		p = append(p, fmt.Sprintf("fresh-clone go_clean %q, caches_emptied %v", str(s, "go_clean"), s["caches_emptied"]))
	}
	// The clone is of the commit whose clean build made the renders.
	bc, dirty := k6BuildCommit(e)
	if bc != commit || dirty != "false" {
		p = append(p, fmt.Sprintf("fresh-clone of %s, but the renders' binaries were built from %s (dirty %s)", commit, bc, dirty))
	}
	ex := FileSHA256(k6Example(e, ref))
	if got := str(s, "render_sha256"); got != ex || ex == "" {
		p = append(p, fmt.Sprintf("fresh-clone render sha256 %s, the K6.1 output %s", got, ex))
	}
	return "commands.jsonl", p, []string{fmt.Sprintf("fresh-clone (argv %q) of %s: render sha256 %s", r.Argv, commit, str(s, "render_sha256"))}
}

var k6Ldflag = regexp.MustCompile(`buildinfo\.Commit=([0-9a-f]{40}) -X [^ ]*buildinfo\.Dirty=(true|false)`)

// k6BuildCommit: the commit and dirty flag of the last build record before
// render-1 (make build prints the go build command with its ldflags).
func k6BuildCommit(e *Evidence) (string, string) {
	name := ""
	for _, r := range e.Records {
		if r.Name == "render-1" {
			break
		}
		if regexp.MustCompile(`^build(-[0-9]+)?$`).MatchString(r.Name) && r.ExitCode == 0 {
			name = r.Name
		}
	}
	if name == "" {
		return "", ""
	}
	_, out, err := e.Get(name)
	if err != nil {
		return "", ""
	}
	m := k6Ldflag.FindSubmatch(out)
	if m == nil {
		return "", ""
	}
	return string(m[1]), string(m[2])
}

// k6Page is the parsed example report.
type k6Page struct {
	html   []byte
	data   map[string]any
	groups map[int64][]int64 // element id e<k> → its data-ev indices
}

var (
	k6DataRe  = regexp.MustCompile(`(?s)<script type="application/json" id="cp-data">(.*?)</script>`)
	k6ElemRe  = regexp.MustCompile(`id="e([0-9]+)" data-ev="([0-9 ]*)"`)
	k6AnyEvRe = regexp.MustCompile(`data-ev="`)
	k6PhaseRe = regexp.MustCompile(`id="e([0-9]+)" data-ev="[0-9]+" data-fault="([^"]*)" data-kind="([^"]*)" data-phase="([^"]*)"`)
	k6RuleRe  = regexp.MustCompile(`data-rule="([^"]*)" data-leader="([^"]*)"`)
	k6LcRe    = regexp.MustCompile(`data-leader-change="([^"]*)" data-leader="([^"]*)"`)
	k6AnomRe  = regexp.MustCompile(`(?s)<li id="anomaly-[^"]*" data-checker="([^"]*)" data-class="([^"]*)" data-ops="([0-9 ]*)">(.*?)</li>`)
	k6HrefRe  = regexp.MustCompile(`href="#e([0-9]+)"`)
)

func k6LoadPage(e *Evidence, ref string) (*k6Page, error) {
	b, err := os.ReadFile(k6Example(e, ref))
	if err != nil {
		return nil, err
	}
	pg := &k6Page{html: b, groups: map[int64][]int64{}}
	m := k6DataRe.FindSubmatch(b)
	if m == nil {
		return nil, fmt.Errorf("no cp-data block in the report")
	}
	dec := json.NewDecoder(bytes.NewReader(m[1]))
	dec.UseNumber()
	if err := dec.Decode(&pg.data); err != nil {
		return nil, fmt.Errorf("cp-data: %w", err)
	}
	if n := len(k6AnyEvRe.FindAllIndex(b, -1)); n != len(k6ElemRe.FindAllIndex(b, -1)) {
		return nil, fmt.Errorf("%d data-ev attributes, of which only %d follow an element id", n, len(k6ElemRe.FindAllIndex(b, -1)))
	}
	for _, x := range k6ElemRe.FindAllSubmatch(b, -1) {
		id, _ := strconv.ParseInt(string(x[1]), 10, 64)
		if _, dup := pg.groups[id]; dup {
			return nil, fmt.Errorf("element id e%d appears twice", id)
		}
		var g []int64
		for _, f := range strings.Fields(string(x[2])) {
			v, _ := strconv.ParseInt(f, 10, 64)
			g = append(g, v)
		}
		pg.groups[id] = g
	}
	return pg, nil
}

// k6LoadRef extracts and verifies the reference run.
func k6LoadRef(e *Evidence, ref string) (*history.SealReport, map[string]any, []byte, []string) {
	dir, err := e.RunDir(filepath.Join(k6RefDir, ref))
	if err != nil {
		return nil, nil, nil, []string{err.Error()}
	}
	rep := history.VerifySeal(dir)
	if !rep.Valid {
		return nil, nil, nil, append([]string{"reference run seal does not verify"}, rep.Problems...)
	}
	cb, err := os.ReadFile(filepath.Join(e.Dir, k6RefDir, ref+".checks.json"))
	if err != nil {
		return nil, nil, nil, []string{err.Error()}
	}
	var checks map[string]any
	dec := json.NewDecoder(bytes.NewReader(cb))
	dec.UseNumber()
	if err := dec.Decode(&checks); err != nil {
		return nil, nil, nil, []string{"checks.json: " + err.Error()}
	}
	var p []string
	if str(checks, "run_id") != ref || str(checks, "seal_root") != rep.MerkleRoot {
		p = append(p, fmt.Sprintf("checks.json names %q / %q, the run's seal root is %s", str(checks, "run_id"), str(checks, "seal_root"), rep.MerkleRoot))
	}
	return rep, checks, cb, p
}

// evalK63: every history event in exactly one element, every fault phase,
// every observed rule and leader change, every checker anomaly with links
// to the operations involved.
func evalK63(e *Evidence) (string, []string, []string) {
	ref, err := k6Ref(e)
	if err != nil {
		return k6RefDir, []string{err.Error()}, nil
	}
	ev := filepath.ToSlash(filepath.Join(k6ReportDir, ref+".html"))
	rep, checks, cb, p := k6LoadRef(e, ref)
	if rep == nil {
		return ev, p, nil
	}
	pg, err := k6LoadPage(e, ref)
	if err != nil {
		return ev, append(p, err.Error()), nil
	}
	n := int64(len(rep.History))
	if str(pg.data, "run_id") != ref || str(pg.data, "seal_root") != rep.MerkleRoot || str(pg.data, "checks_sha256") != sha(cb) {
		p = append(p, "the report's data block does not name the reference run, its seal root and its checks.json")
	}
	if v, _ := k6Num(pg.data["history_lines"]); v != n {
		p = append(p, fmt.Sprintf("the report counts %d history lines, the run has %d", v, n))
	}
	run := newK6Run(rep.History, rep.Observations)
	// Every index in exactly one element; elements shaped as ADR-0036 says.
	seen := make([]int, n)
	for id, g := range pg.groups {
		if len(g) == 0 || g[0] != id {
			p = append(p, fmt.Sprintf("element e%d holds data-ev %v", id, g))
			continue
		}
		for _, i := range g {
			if i < 0 || i >= n {
				p = append(p, fmt.Sprintf("element e%d names index %d outside the history", id, i))
				continue
			}
			seen[i]++
		}
		h := rep.History[g[0]]
		switch {
		case len(g) == 2:
			o, ok := run.opOf[g[0]]
			if h.Type != history.TypeInvoke || !ok || o.comp != g[1] {
				p = append(p, fmt.Sprintf("element e%d pairs %v, not an invoke with its completion", id, g))
			}
		case len(g) == 1:
			if h.Type != history.TypeNemesis && h.Type != history.TypeMarker {
				p = append(p, fmt.Sprintf("element e%d holds the %s event alone", id, h.Type))
			}
		default:
			p = append(p, fmt.Sprintf("element e%d holds %d indices", id, len(g)))
		}
	}
	missing, twice := 0, 0
	for i, c := range seen {
		switch {
		case c == 0:
			if missing < 5 {
				p = append(p, fmt.Sprintf("history event i=%d is in no element of the report", i))
			}
			missing++
		case c > 1:
			if twice < 5 {
				p = append(p, fmt.Sprintf("history event i=%d is in %d elements", i, c))
			}
			twice++
		}
	}
	// Fault phases.
	phases := map[int64][3]string{}
	for _, x := range k6PhaseRe.FindAllSubmatch(pg.html, -1) {
		i, _ := strconv.ParseInt(string(x[1]), 10, 64)
		phases[i] = [3]string{string(x[2]), string(x[3]), string(x[4])}
	}
	nphase := 0
	for id, f := range run.faults {
		for i, ph := range f.idx {
			nphase++
			if got, ok := phases[i]; !ok || got != [3]string{id, f.kind, ph} {
				p = append(p, fmt.Sprintf("fault %s phase %s (i=%d): report element %v", id, ph, i, got))
			}
		}
	}
	if len(phases) != nphase {
		p = append(p, fmt.Sprintf("the report has %d fault phase elements, the history %d nemesis events", len(phases), nphase))
	}
	// Rules and leader changes from the observations.
	wantRules := map[string]string{}
	for _, r := range run.rules {
		wantRules[r.key()] = r.leader
	}
	gotRules := map[string]string{}
	for _, x := range k6RuleRe.FindAllSubmatch(pg.html, -1) {
		gotRules[string(x[1])] = string(x[2])
	}
	if !equalStringMaps(wantRules, gotRules) {
		p = append(p, fmt.Sprintf("observed rules %v, the report lists %v", wantRules, gotRules))
	}
	wantLC := run.leaderChanges()
	gotLC := map[string]string{}
	for _, x := range k6LcRe.FindAllSubmatch(pg.html, -1) {
		gotLC[string(x[1])] = string(x[2])
	}
	if !equalStringMaps(wantLC, gotLC) {
		p = append(p, fmt.Sprintf("leader changes in the observations %v, the report lists %v", wantLC, gotLC))
	}
	// Checker anomalies, each with one link per op to the element holding it.
	type anom struct{ checker, class, ops string }
	var want []anom
	cs, _ := checks["checkers"].([]any)
	for _, c := range cs {
		cm, _ := c.(map[string]any)
		as, _ := cm["anomalies"].([]any)
		for _, a := range as {
			am, _ := a.(map[string]any)
			ops, _ := am["ops"].([]any)
			var s []string
			for _, o := range ops {
				s = append(s, fmt.Sprint(o))
			}
			want = append(want, anom{str(cm, "id"), str(am, "class"), strings.Join(s, " ")})
		}
	}
	var got []anom
	for _, x := range k6AnomRe.FindAllSubmatch(pg.html, -1) {
		a := anom{string(x[1]), string(x[2]), string(x[3])}
		got = append(got, a)
		hrefs := k6HrefRe.FindAllSubmatch(x[4], -1)
		ops := strings.Fields(a.ops)
		if len(hrefs) != len(ops) {
			p = append(p, fmt.Sprintf("anomaly %v has %d links for %d ops", a, len(hrefs), len(ops)))
			continue
		}
		for k, o := range ops {
			i, _ := strconv.ParseInt(o, 10, 64)
			target := i
			if op, ok := run.opOf[i]; ok {
				target = op.inv
			}
			h, _ := strconv.ParseInt(string(hrefs[k][1]), 10, 64)
			g, ok := pg.groups[h]
			if h != target || !ok || !containsInt(g, i) {
				p = append(p, fmt.Sprintf("anomaly %v: op %d links to e%d, whose element holds %v (want e%d)", a, i, h, g, target))
			}
		}
	}
	key := func(a anom) string { return a.checker + "|" + a.class + "|" + a.ops }
	sort.Slice(want, func(i, j int) bool { return key(want[i]) < key(want[j]) })
	sort.Slice(got, func(i, j int) bool { return key(got[i]) < key(got[j]) })
	if fmt.Sprint(want) != fmt.Sprint(got) {
		p = append(p, fmt.Sprintf("checks.json anomalies %v, the report lists %v", want, got))
	}
	return ev, p, []string{fmt.Sprintf("%d history events in %d elements (%d missing, %d twice); %d fault phases; %d rules, %d leader changes; %d anomalies linked",
		n, len(pg.groups), missing, twice, nphase, len(wantRules), len(wantLC), len(want))}
}

func containsInt(xs []int64, v int64) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func equalStringMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// evalK64: a run whose seal does not verify is refused (exit 2) and nothing
// is written, as a directory and as an archive; the untampered control
// renders.
func evalK64(e *Evidence) (string, []string, []string) {
	ref, err := k6Ref(e)
	if err != nil {
		return k6RefDir, []string{err.Error()}, nil
	}
	_, out, p := exitOK(e, "seal-enforcement", "scripts/k6-seal-enforcement.sh", k6RefDir+"/"+ref+ArchiveSuffix)
	if out == nil {
		return "commands.jsonl", p, nil
	}
	s, err := LastJSONLine(out)
	if err != nil {
		return "commands.jsonl", append(p, err.Error()), nil
	}
	cs, _ := s["cases"].([]any)
	refused, control := 0, 0
	var d []string
	for _, c := range cs {
		cm, _ := c.(map[string]any)
		code, _ := k6Num(cm["cpreport_exit"])
		files, _ := k6Num(cm["out_files"])
		d = append(d, fmt.Sprintf("%s (%s): exit %d, out_exists %v, files %d", str(cm, "name"), str(cm, "tamper"), code, cm["out_exists"], files))
		switch str(cm, "expect") {
		case "refused":
			refused++
			if code != 2 || cm["out_exists"] != false || files != 0 {
				p = append(p, fmt.Sprintf("case %s: cpreport exit %d, output dir exists %v, %d files — want exit 2 and nothing written", str(cm, "name"), code, cm["out_exists"], files))
			}
			if !strings.Contains(str(cm, "cpreport_error"), "seal does not verify") {
				p = append(p, fmt.Sprintf("case %s: cpreport's error %q does not name the seal", str(cm, "name"), str(cm, "cpreport_error")))
			}
			if str(cm, "verify_seal_valid") != "false" {
				p = append(p, fmt.Sprintf("case %s: cpcheck verify-seal on the tampered input says valid %q", str(cm, "name"), str(cm, "verify_seal_valid")))
			}
		case "rendered":
			control++
			if code != 0 || cm["out_exists"] != true || files != 1 {
				p = append(p, fmt.Sprintf("control %s: exit %d, %d files", str(cm, "name"), code, files))
			}
		}
	}
	if refused < 2 || control < 1 {
		p = append(p, fmt.Sprintf("%d refused cases (want ≥ 2: a directory and an archive) and %d controls (want ≥ 1)", refused, control))
	}
	return "commands.jsonl", p, d
}

// evalK65: the report's per-fault metrics equal cpgate's independent
// computation from the raw history and observations (ADR-0036).
func evalK65(e *Evidence) (string, []string, []string) {
	ref, err := k6Ref(e)
	if err != nil {
		return k6RefDir, []string{err.Error()}, nil
	}
	ev := filepath.ToSlash(filepath.Join(k6ReportDir, ref+".html"))
	rep, _, _, p := k6LoadRef(e, ref)
	if rep == nil {
		return ev, p, nil
	}
	pg, err := k6LoadPage(e, ref)
	if err != nil {
		return ev, append(p, err.Error()), nil
	}
	if str(pg.data, "run_id") != ref || str(pg.data, "seal_root") != rep.MerkleRoot {
		p = append(p, "the data block does not name the reference run")
	}
	want := newK6Run(rep.History, rep.Observations).expected()
	got := map[string]map[string]any{}
	fs, _ := pg.data["faults"].([]any)
	for _, f := range fs {
		fm, _ := f.(map[string]any)
		got[str(fm, "id")] = fm
	}
	compared := 0
	for _, id := range sortedKeys(want) {
		g, ok := got[id]
		if !ok {
			p = append(p, fmt.Sprintf("fault %s: no metrics in the report", id))
			continue
		}
		p = append(p, k6Diff("fault "+id, want[id], g)...)
		compared++
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			p = append(p, fmt.Sprintf("the report has metrics for fault %s, which the history does not hold", id))
		}
	}
	return ev, p, []string{fmt.Sprintf("%d faults compared field by field (windows, gaps, non-ok counts per gateway; n, p50, p95, p99 per phase; rule and leader changes)", compared)}
}

// k6Diff compares the expected value w with the report's g (decoded with
// json.Number), recursively over w's keys; g must have no other keys.
func k6Diff(path string, w, g any) []string {
	switch wv := w.(type) {
	case map[string]any:
		gm, ok := g.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: report has %v, want an object", path, g)}
		}
		var out []string
		for _, k := range sortedKeys(wv) {
			out = append(out, k6Diff(path+"."+k, wv[k], gm[k])...)
		}
		for k := range gm {
			if _, ok := wv[k]; !ok && !k6Extra[k] {
				out = append(out, fmt.Sprintf("%s.%s: in the report, not expected", path, k))
			}
		}
		return out
	case int64:
		if gv, ok := k6Num(g); !ok || gv != wv {
			return []string{fmt.Sprintf("%s: report %v, recomputed %d", path, g, wv)}
		}
	case string:
		if gs, _ := g.(string); gs != wv {
			return []string{fmt.Sprintf("%s: report %v, recomputed %q", path, g, wv)}
		}
	case []string:
		ga, _ := g.([]any)
		if len(ga) != len(wv) {
			return []string{fmt.Sprintf("%s: report %v, recomputed %v", path, g, wv)}
		}
		for i := range wv {
			if fmt.Sprint(ga[i]) != wv[i] {
				return []string{fmt.Sprintf("%s: report %v, recomputed %v", path, g, wv)}
			}
		}
	default:
		return []string{fmt.Sprintf("%s: cannot compare %T", path, w)}
	}
	return nil
}

// k6Extra: data-block keys of a fault that K6.5 does not recompute (the
// identity fields K6.3 checks against the history).
var k6Extra = map[string]bool{"id": true, "targets": true, "phases": true}

// evalK66: the latest CI attempt ran every CI target, ci-report included,
// and exited 0; the workflow lints.
func evalK66(e *Evidence) (string, []string, []string) {
	name := lastCI(e)
	if name == "" {
		return "commands.jsonl", []string{"no ci record"}, nil
	}
	r, _, p := exitOK(e, name, "make", "ci-unit", "ci-checkers", "ci-cluster-smoke", "ci-nemesis-smoke", "ci-mutant-smoke", "ci-report")
	_, _, pl := exitOK(e, "lint-workflow", "make", "lint-workflow")
	p = append(p, pl...)
	var earlier []string
	for _, x := range e.Records {
		if ciAttempt.MatchString(x.Name) && x.Name != name {
			earlier = append(earlier, fmt.Sprintf("%s exit %d", x.Name, x.ExitCode))
		}
	}
	return "commands.jsonl", p, []string{fmt.Sprintf("%s: argv %q exit %d; earlier attempts %v", name, r.Argv, r.ExitCode, earlier)}
}

// sabotageK61 replaces render-1..3's stdout by sabotage-render-1..3's.
func sabotageK61(dir string) error {
	e := LoadEvidence(dir)
	for n := 1; n <= k6Renders; n++ {
		_, out, err := e.Get(fmt.Sprintf("sabotage-render-%d", n))
		if err != nil {
			return err
		}
		r, _, err := e.Get(fmt.Sprintf("render-%d", n))
		if err != nil {
			return err
		}
		// The sabotaged summary would be refused by its sabotage field;
		// drop it, so that only the outputs' bytes can fail K6.1.
		s, err := LastJSONLine(out)
		if err != nil {
			return err
		}
		delete(s, "sabotage")
		nb, err := json.Marshal(s)
		if err != nil {
			return err
		}
		nb = append(nb, '\n')
		if err := os.WriteFile(e.Path(r.Stdout), nb, 0o644); err != nil {
			return err
		}
		name := r.Name
		if err := editRecords(dir, func(m map[string]any) {
			if m["name"] == name {
				m["stdout_sha256"] = sha(nb)
			}
		}); err != nil {
			return err
		}
	}
	return nil
}

// forgeReport edits the example report and rewrites every recorded hash of
// it (render-1..3 and fresh-clone) to the forged file's, so the forgery is
// consistent and only the criterion reading the report's content can tell.
func forgeReport(dir string, edit func([]byte) ([]byte, error)) error {
	e := LoadEvidence(dir)
	ref, err := k6Ref(e)
	if err != nil {
		return err
	}
	p := k6Example(e, ref)
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	nb, err := edit(b)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, nb, 0o644); err != nil {
		return err
	}
	h := sha(nb)
	for n := 1; n <= k6Renders; n++ {
		if err := editSummary(dir, fmt.Sprintf("render-%d", n), func(m map[string]any) {
			if outs, _ := m["outputs"].([]any); len(outs) == 1 {
				if o, _ := outs[0].(map[string]any); o != nil {
					o["sha256"] = h
				}
			}
		}); err != nil {
			return err
		}
	}
	return editSummary(dir, "fresh-clone", func(m map[string]any) { m["render_sha256"] = h })
}
