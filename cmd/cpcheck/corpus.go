package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/checker"
	"github.com/CesarManzoCode/commitproof/internal/checker/elle"
)

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// expectation is one checker's entry in expected.json.
type expectation struct {
	Valid        any      `json:"valid"`
	Anomalies    []string `json:"anomalies"`
	Observations []string `json:"observations"`
}

// expected is expected.json, written by the synthetic generator from what it
// planted (never by running this code).
type expected struct {
	ID       string                 `json:"id"`
	Label    string                 `json:"label"`
	Family   string                 `json:"family"`
	Elle     bool                   `json:"elle"`
	Verdict  string                 `json:"verdict"`
	Checkers map[string]expectation `json:"checkers"`
}

func validString(v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "true"
		}
		return "false"
	case string:
		return x
	}
	return fmt.Sprint(v)
}

func sortedSet(xs []string) []string {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	out := make([]string, 0, len(m))
	for x := range m {
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

// compare returns the differences between expected and reported results
// (set equality of anomaly and observation classes per checker, validity,
// verdict).
func compare(exp *expected, rs []checker.Result, verdict string) []string {
	var diffs []string
	got := map[string]checker.Result{}
	for _, r := range rs {
		got[r.ID] = r
	}
	for _, id := range checker.IDs {
		e, ok := exp.Checkers[id]
		if !ok {
			diffs = append(diffs, id+": no expectation")
			continue
		}
		r := got[id]
		if validString(e.Valid) != string(r.Valid) {
			diffs = append(diffs, fmt.Sprintf("%s valid %s, expected %s", id, r.Valid, validString(e.Valid)))
		}
		if a, b := checker.Classes(r.Anomalies), sortedSet(e.Anomalies); !equal(a, b) {
			diffs = append(diffs, fmt.Sprintf("%s anomalies %v, expected %v", id, a, b))
		}
		if a, b := checker.Classes(r.Observations), sortedSet(e.Observations); !equal(a, b) {
			diffs = append(diffs, fmt.Sprintf("%s observations %v, expected %v", id, a, b))
		}
	}
	if verdict != exp.Verdict {
		diffs = append(diffs, fmt.Sprintf("verdict %s, expected %s", verdict, exp.Verdict))
	}
	return diffs
}

type entryResult struct {
	id      string
	exp     *expected
	expRaw  []byte
	h       *checker.History
	rs      []checker.Result
	verdict string
	checks  []byte
	diffs   []string
	root    string
}

func cmdCorpus(args []string) int {
	fs := flag.NewFlagSet("corpus", flag.ContinueOnError)
	oracle := fs.Bool("elle-oracle", false, "cross-check C3 with elle-cli on entries marked elle")
	out := fs.String("out", "", "write results.jsonl, per-entry checks.json and oracle outputs here")
	resultsOnly := fs.Bool("results-only", false, "with --out, write results.jsonl only (no per-entry files)")
	dir, err := parse(fs, args)
	if err != nil {
		return summary(map[string]any{"command": "corpus", "error": err.Error() + "; " + usage}, 3)
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		return summary(map[string]any{"command": "corpus", "error": err.Error()}, 3)
	}
	var entries []*entryResult
	for _, de := range des {
		if !de.IsDir() {
			continue
		}
		p := filepath.Join(dir, de.Name())
		raw, err := os.ReadFile(filepath.Join(p, "expected.json"))
		if err != nil {
			continue
		}
		var e expected
		if err := json.Unmarshal(raw, &e); err != nil {
			return summary(map[string]any{"command": "corpus", "error": de.Name() + "/expected.json: " + err.Error()}, 3)
		}
		er := &entryResult{id: de.Name(), exp: &e, expRaw: raw}
		h, rs, invalid, rep, err := checker.Check(filepath.Join(p, "run"))
		if err != nil {
			invalid = append(invalid, err.Error())
		}
		er.h, er.rs = h, rs
		if rep != nil {
			er.root = rep.MerkleRoot
		}
		er.verdict = checker.Verdict(invalid, rs)
		runID := de.Name()
		if h != nil {
			runID = h.RunID
		}
		if er.checks, err = checker.ChecksJSON(runID, er.root, rs, er.verdict); err != nil {
			return summary(map[string]any{"command": "corpus", "error": err.Error()}, 3)
		}
		er.diffs = compare(&e, rs, er.verdict)
		entries = append(entries, er)
	}
	if len(entries) == 0 {
		return summary(map[string]any{"command": "corpus", "error": "no corpus entries (subdirectories with expected.json and run/) in " + dir}, 3)
	}
	m := map[string]any{"command": "corpus", "dir": filepath.ToSlash(dir), "entries": int64(len(entries))}
	var mismatched []any
	for _, er := range entries {
		if len(er.diffs) > 0 {
			mismatched = append(mismatched, er.id)
		}
	}
	m["matched"], m["mismatched"] = int64(len(entries)-len(mismatched)), mismatched
	ok := len(mismatched) == 0
	var elleLines map[string]map[string]any
	if *oracle {
		lines, stats, err := oracleCorpus(entries, *out)
		if err != nil {
			return summary(map[string]any{"command": "corpus", "error": "elle oracle: " + err.Error()}, 3)
		}
		elleLines = lines
		m["elle"] = stats
		if d, _ := stats["disagreed"].(int64); d > 0 {
			ok = false
		}
	}
	if *out != "" {
		if err := writeCorpusOut(*out, entries, elleLines, !*resultsOnly); err != nil {
			return summary(map[string]any{"command": "corpus", "error": err.Error()}, 3)
		}
		m["out"] = filepath.ToSlash(*out)
		// The results file's hash ties a stored (e.g. compressed) copy of it
		// to this recorded run (K5 evidence, P25).
		b, err := os.ReadFile(filepath.Join(*out, "results.jsonl"))
		if err != nil {
			return summary(map[string]any{"command": "corpus", "error": err.Error()}, 3)
		}
		m["results_sha256"] = sha(b)
	}
	for _, er := range entries {
		if len(er.diffs) > 0 {
			fmt.Fprintf(os.Stderr, "MISMATCH %s: %v\n", er.id, er.diffs)
		}
	}
	if !ok {
		return summary(m, 1)
	}
	return summary(m, 0)
}

// writeCorpusOut writes results.jsonl (one line per entry: the reported
// validity and classes of every checker, the verdict, the seal root and the
// comparison with expected.json) and, if entries is set, each entry's
// checks.json and a copy of its expected.json.
func writeCorpusOut(out string, entries []*entryResult, elleLines map[string]map[string]any, perEntry bool) error {
	var lines bytes.Buffer
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	for _, er := range entries {
		if perEntry {
			ed := filepath.Join(out, "entries", er.id)
			if err := os.MkdirAll(ed, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(ed, "checks.json"), append(er.checks, '\n'), 0o644); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(ed, "expected.json"), er.expRaw, 0o644); err != nil {
				return err
			}
		}
		reported := map[string]any{}
		for _, r := range er.rs {
			reported[r.ID] = map[string]any{"valid": string(r.Valid), "anomalies": strs(checker.Classes(r.Anomalies)),
				"observations": strs(checker.Classes(r.Observations))}
		}
		l := map[string]any{"id": er.id, "label": er.exp.Label, "family": er.exp.Family, "seal_root": er.root,
			"verdict": er.verdict, "expected_verdict": er.exp.Verdict, "match": len(er.diffs) == 0, "diffs": strs(er.diffs),
			"reported": reported, "expected_sha256": sha(er.expRaw)}
		if el, ok := elleLines[er.id]; ok {
			l["elle"] = el
		}
		b, err := canonjson.Marshal(l)
		if err != nil {
			return err
		}
		lines.Write(b)
		lines.WriteByte('\n')
	}
	return os.WriteFile(filepath.Join(out, "results.jsonl"), lines.Bytes(), 0o644)
}

// oracleCorpus exports the entries marked elle, runs elle-cli per model and
// compares with C3: validity and the class set (anomalies ∪ observations)
// must agree wherever Elle's result is not unknown.
func oracleCorpus(entries []*entryResult, out string) (map[string]map[string]any, map[string]any, error) {
	ctx := context.Background()
	img, err := elle.ImageID(ctx)
	if err != nil {
		return nil, nil, err
	}
	dir := filepath.Join(out, "elle")
	if out == "" {
		if dir, err = os.MkdirTemp("", "cpcheck-elle-"); err != nil {
			return nil, nil, err
		}
		defer os.RemoveAll(dir)
	}
	exportDir := filepath.Join(dir, "export")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return nil, nil, err
	}
	byModel := map[string][]*entryResult{}
	for _, er := range entries {
		if !er.exp.Elle || er.h == nil {
			continue
		}
		exp, model, err := elle.Export(er.h)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", er.id, err)
		}
		if err := os.WriteFile(filepath.Join(exportDir, er.id+".json"), exp, 0o644); err != nil {
			return nil, nil, err
		}
		byModel[model] = append(byModel[model], er)
	}
	lines := map[string]map[string]any{}
	var compared, agreed, disagreed, unknown int64
	perModel := map[string]any{}
	var disagreements []any
	for _, model := range []string{"serializable", "repeatable-read"} {
		es := byModel[model]
		if len(es) == 0 {
			continue
		}
		perModel[model] = int64(len(es))
		files := make([]string, len(es))
		for i, er := range es {
			files[i] = er.id + ".json"
		}
		res, _, err := elle.CheckFiles(ctx, exportDir, files, model)
		if err != nil {
			return nil, nil, err
		}
		rawDir := filepath.Join(dir, "raw")
		if err := os.MkdirAll(rawDir, 0o755); err != nil {
			return nil, nil, err
		}
		for i, er := range es {
			r := res[i]
			for _, k := range elle.Kinds {
				if err := os.WriteFile(filepath.Join(rawDir, er.id+"."+k+".json"), append(r.Raw[k], '\n'), 0o644); err != nil {
					return nil, nil, err
				}
			}
			var c3 checker.Result
			for _, x := range er.rs {
				if x.ID == "C3" {
					c3 = x
				}
			}
			cc := c3Classes(c3)
			l := map[string]any{"model": model, "elle_valid": r.Valid, "elle_classes": strs(r.Classes),
				"c3_valid": string(c3.Valid), "c3_classes": strs(cc)}
			switch {
			case r.Valid == "unknown":
				unknown++
				l["agree"] = "elle-unknown"
			case r.Valid == string(c3.Valid) && equal(r.Classes, cc):
				compared++
				agreed++
				l["agree"] = true
			default:
				compared++
				disagreed++
				l["agree"] = false
				disagreements = append(disagreements, er.id)
				fmt.Fprintf(os.Stderr, "ELLE DISAGREES %s: C3 %s %v, Elle %s %v\n", er.id, c3.Valid, cc, r.Valid, r.Classes)
			}
			lines[er.id] = l
		}
	}
	return lines, map[string]any{"image": img, "models": perModel, "compared": compared, "agreed": agreed,
		"disagreed": disagreed, "elle_unknown": unknown, "disagreements": disagreements}, nil
}
