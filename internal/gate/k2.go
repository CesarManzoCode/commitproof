package gate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// K2 inputs. The corpus is the committed testdata (sealed runs referenced
// by the evidence, ADR-0017); everything else is under evidence/K2.
const (
	K2Corpus        = "testdata/corpus"
	K2Pathological  = "testdata/pathological/porcupine-timeout/run"
	k2Results       = "corpus/results.jsonl"
	k2Raw           = "corpus/elle/raw"
	k2PathoRun      = "runs/porcupine-timeout"
	k2Divergences   = "divergences.json"
	ElleCLICommit   = "9c2a18b3719f573dab06ae05c955c9c9d5225673"
	ElleBaseImage   = "clojure:temurin-21-lein-2.13.0-bookworm@sha256:724a7f5840394bf9e659730e5039bf7498f31185e747e3586dbab626b717c34f"
	pathoMinWrites  = 20
	k2MinPerModel   = 50
	k2MinCompared   = 300
	k2MinClean      = 100
	k2MinPlanted    = 200
	k2MinPerClass   = 3
	k2MinCleanPerWL = 50
)

// K2WireTests are the K2.5 tests: one per row of the §6.2 table and the
// S61 WARNING case (internal/workload/wire_test.go).
var K2WireTests = []string{
	"TestWireRow01Success", "TestWireRow02Class40BeforeCommit", "TestWireRow03SerializationFailureAtCommit",
	"TestWireRow04IntegrityBeforeCommit", "TestWireRow05RejectedBeforeAnyWrite", "TestWireRow06ReadOnly",
	"TestWireRow07NoWritablePrimary", "TestWireRow08ErrorAtCommit", "TestWireRow09TimeoutAfterWriteSent",
	"TestWireRow10ConnectionError", "TestWireRow11UnlistedSQLState", "TestWireRow12AutocommitTransport",
	"TestWireS61CommitWarning",
}

// K2 returns the seven criteria of CONTRACT §15 K2 with their sabotages.
func K2() []Criterion {
	return []Criterion{
		{ID: "K2.1", Name: "Planted anomalies", Eval: evalK21,
			Sabotage: "drop the reported bank-sum of planted entry w3-sum-read-01 from the corpus results",
			Apply: func(dir string) error {
				return editResult(filepath.Join(dir, k2Results), "w3-sum-read-01", func(m map[string]any) {
					obj(m, "reported")["C5"] = map[string]any{"valid": "true", "anomalies": []any{}, "observations": []any{}}
				})
			}},
		{ID: "K2.2", Name: "Clean histories", Eval: evalK22,
			Sabotage: "report a bank-sum on clean entry w3-clean-001 in the corpus results",
			Apply: func(dir string) error {
				return editResult(filepath.Join(dir, k2Results), "w3-clean-001", func(m map[string]any) {
					obj(m, "reported")["C5"] = map[string]any{"valid": "false", "anomalies": []any{"bank-sum"}, "observations": []any{}}
				})
			}},
		{ID: "K2.3", Name: "Unknown is not pass", Eval: evalK23,
			Sabotage: "rewrite the pathological run's checks.json verdict to PASS",
			Apply: func(dir string) error {
				return editJSON(filepath.Join(dir, k2PathoRun+".checks.json"), func(m map[string]any) { m["verdict"] = "PASS" })
			}},
		{ID: "K2.4", Name: "Elle agreement", Eval: evalK24,
			Sabotage: "make elle-cli's validity run of w1-ser-clean-001 say false",
			Apply: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, k2Raw, "w1-ser-clean-001.validity.json"), []byte(`{"valid?":false}`+"\n"), 0o644)
			}},
		{ID: "K2.5", Name: "Classification table", Eval: evalK25,
			Sabotage: "turn the pass event of TestWireRow03SerializationFailureAtCommit into fail (hashes kept consistent)",
			Apply: func(dir string) error {
				return editStdout(dir, "wire-tests", func(out []byte) []byte {
					var lines [][]byte
					for _, l := range bytes.Split(out, []byte("\n")) {
						if bytes.Contains(l, []byte(`"Action":"pass"`)) && bytes.Contains(l, []byte(`"Test":"TestWireRow03SerializationFailureAtCommit"`)) {
							l = bytes.Replace(l, []byte(`"Action":"pass"`), []byte(`"Action":"fail"`), 1)
						}
						lines = append(lines, l)
					}
					return bytes.Join(lines, []byte("\n"))
				})
			}},
		{ID: "K2.6", Name: "Checker sabotage", Eval: evalK26,
			Sabotage: "report a bank-sum on clean entry w3-clean-001 in the bank_sum sabotage results",
			Apply: func(dir string) error {
				return editResult(filepath.Join(dir, "sabotage", "bank_sum", "results.jsonl"), "w3-clean-001", func(m map[string]any) {
					obj(m, "reported")["C5"] = map[string]any{"valid": "false", "anomalies": []any{"bank-sum"}, "observations": []any{}}
				})
			}},
		{ID: "K2.7", Name: "CI-equivalent", Eval: evalK27,
			Sabotage: "replace the recorded exit code of `make ci-unit ci-checkers` with 1",
			Apply: func(dir string) error {
				return editRecords(dir, func(r map[string]any) {
					if r["name"] == "ci" {
						r["exit_code"] = 1
					}
				})
			}},
	}
}

// editResult rewrites one results.jsonl line.
func editResult(path, id string, f func(map[string]any)) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	found := false
	for _, l := range bytes.Split(bytes.TrimRight(b, "\n"), []byte("\n")) {
		var m map[string]any
		if err := json.Unmarshal(l, &m); err != nil {
			return err
		}
		if m["id"] == id {
			f(m)
			found = true
		}
		nb, err := json.Marshal(m)
		if err != nil {
			return err
		}
		out.Write(nb)
		out.WriteByte('\n')
	}
	if !found {
		return fmt.Errorf("%s: no entry %s", path, id)
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

// editStdout rewrites a recorded command's stdout and its recorded hash.
func editStdout(dir, name string, f func([]byte) []byte) error {
	e := LoadEvidence(dir)
	r, out, err := e.Get(name)
	if err != nil {
		return err
	}
	nout := f(out)
	if err := os.WriteFile(e.Path(r.Stdout), nout, 0o644); err != nil {
		return err
	}
	return editRecords(dir, func(rec map[string]any) {
		if rec["name"] == name {
			rec["stdout_sha256"] = sha(nout)
		}
	})
}

func corpusRecord(e *Evidence) (Record, []string) {
	r, out, err := e.Get("corpus")
	if err != nil {
		return r, []string{err.Error()}
	}
	var p []string
	if !argvHas(r, "cpcheck", "corpus", K2Corpus, "--elle-oracle", "--out", "evidence/K2/corpus") {
		p = append(p, fmt.Sprintf("corpus argv %q", r.Argv))
	}
	if r.ExitCode != 0 && r.ExitCode != 1 {
		p = append(p, fmt.Sprintf("corpus exited %d", r.ExitCode))
	}
	if _, err := LastJSONLine(out); err != nil {
		p = append(p, "corpus summary: "+err.Error())
	}
	return r, p
}

// k2Entries loads the corpus results and checks they cover every committed
// corpus entry exactly once, each on the committed sealed run.
func k2Entries(e *Evidence) ([]K2Result, []string) {
	_, p := corpusRecord(e)
	rs, err := LoadK2Results(filepath.Join(e.Dir, k2Results))
	if err != nil {
		return nil, append(p, err.Error())
	}
	ids, err := CorpusIDs(K2Corpus)
	if err != nil {
		return nil, append(p, err.Error())
	}
	seen := map[string]int{}
	for _, r := range rs {
		seen[r.ID]++
	}
	for _, id := range ids {
		if seen[id] != 1 {
			p = append(p, fmt.Sprintf("corpus entry %s reported %d times", id, seen[id]))
		}
	}
	if len(rs) != len(ids) {
		p = append(p, fmt.Sprintf("%d results for %d corpus entries", len(rs), len(ids)))
	}
	return rs, append(p, verifyCorpusSeals(K2Corpus, rs)...)
}

func evalK21(e *Evidence) (string, []string, []string) {
	rs, p := k2Entries(e)
	count := map[string]int{}
	planted := 0
	for _, r := range rs {
		if r.Label == "clean" {
			continue
		}
		planted++
		x, err := LoadK2Expected(K2Corpus, r)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		if d := K2Diff(r, x); len(d) > 0 {
			p = append(p, fmt.Sprintf("%s: %s", r.ID, strings.Join(d, "; ")))
			continue
		}
		cls := map[string]bool{}
		for _, c := range x.Checkers {
			for _, a := range append(append([]string{}, c.Anomalies...), c.Observations...) {
				cls[a] = true
			}
		}
		for c := range cls {
			count[c]++
		}
	}
	var details []string
	for _, c := range K2Classes {
		if count[c] < k2MinPerClass {
			p = append(p, fmt.Sprintf("class %s: %d matching planted histories, want ≥ %d", c, count[c], k2MinPerClass))
		}
		details = append(details, fmt.Sprintf("%s: %d", c, count[c]))
	}
	return k2Results, p, append([]string{fmt.Sprintf("%d planted histories, %d classes", planted, len(K2Classes))}, details...)
}

func evalK22(e *Evidence) (string, []string, []string) {
	rs, p := k2Entries(e)
	perWL := map[string]int{}
	clean := 0
	for _, r := range rs {
		if r.Label != "clean" {
			continue
		}
		clean++
		x, err := LoadK2Expected(K2Corpus, r)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		bad := K2Diff(r, x)
		for _, c := range k2Checkers {
			if r.Reported[c].Valid != "true" {
				bad = append(bad, c+" not valid")
			}
		}
		if len(bad) > 0 {
			p = append(p, fmt.Sprintf("clean %s: %s", r.ID, strings.Join(bad, "; ")))
			continue
		}
		for _, w := range x.Workloads {
			perWL[w]++
		}
	}
	var details []string
	for _, w := range []string{"W1", "W2", "W3", "W4", "W5", "W6", "W7"} {
		if perWL[w] < k2MinCleanPerWL {
			p = append(p, fmt.Sprintf("%d clean histories with %s, want ≥ %d", perWL[w], w, k2MinCleanPerWL))
		}
		details = append(details, fmt.Sprintf("%s: %d clean", w, perWL[w]))
	}
	return k2Results, p, append([]string{fmt.Sprintf("%d clean histories", clean)}, details...)
}

func evalK23(e *Evidence) (string, []string, []string) {
	const f = k2PathoRun + ".checks.json"
	r, out, err := e.Get("pathological")
	if err != nil {
		return f, []string{err.Error()}, nil
	}
	var p []string
	if r.ExitCode != 2 {
		p = append(p, fmt.Sprintf("cpcheck check exited %d, want 2", r.ExitCode))
	}
	if !argvHas(r, "cpcheck", "check", "evidence/K2/"+k2PathoRun) {
		p = append(p, fmt.Sprintf("pathological argv %q", r.Argv))
	}
	s, err := LastJSONLine(out)
	if err != nil {
		return f, append(p, err.Error()), nil
	}
	b, err := os.ReadFile(filepath.Join(e.Dir, f))
	if err != nil {
		return f, append(p, err.Error()), nil
	}
	if str(s, "verdict") != "UNKNOWN" || str(s, "checks_sha256") != sha(b) {
		p = append(p, fmt.Sprintf("summary verdict %q, checks_sha256 %q (file %s)", str(s, "verdict"), str(s, "checks_sha256"), sha(b)))
	}
	var cj struct {
		SealRoot string `json:"seal_root"`
		Verdict  string `json:"verdict"`
		Checkers []struct {
			ID    string `json:"id"`
			Valid any    `json:"valid"`
		} `json:"checkers"`
	}
	if err := json.Unmarshal(b, &cj); err != nil {
		return f, append(p, err.Error()), nil
	}
	if cj.Verdict != "UNKNOWN" {
		p = append(p, "checks.json verdict "+cj.Verdict)
	}
	want := map[string]string{"C4": "unknown", "C6": "unknown"}
	if len(cj.Checkers) != len(k2Checkers) {
		p = append(p, fmt.Sprintf("checks.json has %d checkers", len(cj.Checkers)))
	}
	for _, c := range cj.Checkers {
		w := want[c.ID]
		if w == "" {
			w = "true"
		}
		if validStr(c.Valid) != w {
			p = append(p, fmt.Sprintf("%s valid %v, want %s", c.ID, c.Valid, w))
		}
	}
	rep := history.VerifySeal(filepath.Join(e.Dir, k2PathoRun))
	orig := history.VerifySeal(K2Pathological)
	if !rep.Valid || rep.MerkleRoot != cj.SealRoot || orig.MerkleRoot != rep.MerkleRoot {
		p = append(p, fmt.Sprintf("seal: valid %v root %s, checks.json %s, committed %s", rep.Valid, rep.MerkleRoot, cj.SealRoot, orig.MerkleRoot))
	}
	writes := 0
	for _, ev := range rep.History {
		if ev.Type == history.TypeInfo && ev.WL == "register" && ev.F == "write" {
			writes++
		}
	}
	if writes < pathoMinWrites {
		p = append(p, fmt.Sprintf("%d open-ended register writes, not pathological (want ≥ %d)", writes, pathoMinWrites))
	}
	st, err1 := time.Parse(time.RFC3339Nano, r.Start)
	en, err2 := time.Parse(time.RFC3339Nano, r.End)
	if err1 != nil || err2 != nil || en.Sub(st) < params.PorcupineTimeout {
		p = append(p, fmt.Sprintf("check took %s, less than P11 %s", en.Sub(st), params.PorcupineTimeout))
	}
	return f, p, []string{fmt.Sprintf("%d open-ended writes; check took %s; exit %d", writes, en.Sub(st).Round(time.Millisecond), r.ExitCode)}
}

func evalK24(e *Evidence) (string, []string, []string) {
	_, out, err := e.Get("corpus")
	var p []string
	if err != nil {
		return k2Raw, []string{err.Error()}, nil
	}
	s, _ := LastJSONLine(out)
	if _, err := os.Stat(filepath.Join(e.Dir, k2Divergences)); err != nil {
		p = append(p, "divergences.json: "+err.Error())
	}
	st, pp := K2ElleCompare(K2Corpus, filepath.Join(e.Dir, k2Results), filepath.Join(e.Dir, k2Raw), filepath.Join(e.Dir, k2Divergences))
	p = append(p, pp...)
	if st.Compared < k2MinCompared || st.Clean < k2MinClean || st.Planted < k2MinPlanted {
		p = append(p, fmt.Sprintf("compared %d (clean %d, planted %d), want ≥ %d (≥ %d, ≥ %d)", st.Compared, st.Clean, st.Planted, k2MinCompared, k2MinClean, k2MinPlanted))
	}
	for _, m := range []string{"serializable", "repeatable-read"} {
		if st.PerModel[m] < k2MinPerModel {
			p = append(p, fmt.Sprintf("%d histories compared under %s, want ≥ %d", st.PerModel[m], m, k2MinPerModel))
		}
	}
	// Oracle provenance: the image used is the one built from the pins.
	_, bo, err := e.Get("elle-oracle")
	if err != nil {
		p = append(p, err.Error())
	} else if b, err := LastJSONLine(bo); err != nil {
		p = append(p, err.Error())
	} else {
		if str(b, "base") != ElleBaseImage || str(b, "elle_cli_commit") != ElleCLICommit {
			p = append(p, fmt.Sprintf("oracle built from %q at %q, not the pins", str(b, "base"), str(b, "elle_cli_commit")))
		}
		if el := obj(s, "elle"); str(el, "image") != str(b, "image_id") || str(b, "image_id") == "" {
			p = append(p, fmt.Sprintf("corpus used oracle image %q, built image is %q", str(el, "image"), str(b, "image_id")))
		}
	}
	return k2Raw, p, []string{fmt.Sprintf("compared %d (clean %d, planted %d; %v), elle unknown %d, disagreements %d %v",
		st.Compared, st.Clean, st.Planted, st.PerModel, st.Unknown, len(st.Disagreements), st.Disagreements)}
}

func evalK25(e *Evidence) (string, []string, []string) {
	r, out, err := e.Get("wire-tests")
	if err != nil {
		return "commands.jsonl", []string{err.Error()}, nil
	}
	var p []string
	if r.ExitCode != 0 {
		p = append(p, fmt.Sprintf("wire tests exited %d", r.ExitCode))
	}
	if !argvHas(r, "go", "test", "-json", "TestWire", "./internal/workload") {
		p = append(p, fmt.Sprintf("wire-tests argv %q", r.Argv))
	}
	passed := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var ev struct{ Action, Test string }
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch ev.Action {
		case "pass":
			if ev.Test != "" {
				passed[ev.Test] = true
			}
		case "fail":
			p = append(p, "test event fail: "+ev.Test)
		}
	}
	for _, t := range K2WireTests {
		if !passed[t] {
			p = append(p, t+" did not pass")
		}
	}
	return "commands.jsonl", p, []string{fmt.Sprintf("%d required tests passed", len(K2WireTests))}
}

func evalK26(e *Evidence) (string, []string, []string) {
	var p, details []string
	rules := make([]string, 0, len(K2SabotageDeps))
	for r := range K2SabotageDeps {
		rules = append(rules, r)
	}
	sort.Strings(rules)
	for _, rule := range rules {
		r, out, err := e.Get("sabotage-" + rule)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		if r.ExitCode != 1 {
			p = append(p, fmt.Sprintf("sabotage-%s exited %d, want 1", rule, r.ExitCode))
		}
		if !argvHas(r, "scripts/sabotage-corpus.sh", rule, K2Corpus, "evidence/K2/sabotage/"+rule) {
			p = append(p, fmt.Sprintf("sabotage-%s argv %q", rule, r.Argv))
		}
		s, err := LastJSONLine(out)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		if sab, _ := s["sabotage"].([]any); len(sab) != 1 || sab[0] != rule {
			p = append(p, fmt.Sprintf("sabotage-%s: binary reports active sabotage %v", rule, s["sabotage"]))
		}
		d, pp := K2CheckSabotage(rule, K2Corpus, filepath.Join(e.Dir, "sabotage", rule, "results.jsonl"))
		p = append(p, pp...)
		details = append(details, d)
	}
	r, out, err := e.Get("no-sabotage-symbols")
	switch {
	case err != nil:
		p = append(p, err.Error())
	case r.ExitCode != 0 || !argvHas(r, "TestProductBinariesHaveNoSabotage") || !bytes.Contains(out, []byte("--- PASS: TestProductBinariesHaveNoSabotage")):
		p = append(p, fmt.Sprintf("no-sabotage-symbols: exit %d argv %q", r.ExitCode, r.Argv))
	}
	return "sabotage/", p, details
}

func evalK27(e *Evidence) (string, []string, []string) {
	r, _, p := exitOK(e, "ci", "make", "ci-unit", "ci-checkers")
	return "commands.jsonl", p, []string{fmt.Sprintf("argv %q exit %d", r.Argv, r.ExitCode)}
}
