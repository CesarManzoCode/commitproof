// Command k7findings writes the mechanical part of K7's findings packages
// (CONTRACT §11.4, §15 K7.4; ADR-0043, ADR-0045): for every finding of
// evidence/K7/packages.json (written by hand: id, campaign, anomaly class,
// the original and minimized runs' archives, the §11 classification, the
// recorded classification where it differs, the triage record) it writes
// findings/<id>/finding.json, plan.json (the minimized run's plan) and
// checks.json (the original run's), computing from the files themselves:
// each run's archive sha256, seal root, verdict and classes; the recorded
// replays of the minimized run (same plan hash, manifest replay_of) and how
// many show the anomaly class; whether an acknowledged write was lost (the
// original run's C1); the K7 runs that repeat the finding (results.json's
// repeats whose groups name it, ADR-0045); and, once recorded, K7.4's
// re-execution of the replay command (record pkg-replay-<id>, its replays
// in evidence/K7/package-replays). It decides nothing: classification and
// reproducibility are copied from packages.json, and cpgate K7 re-derives
// every computed field independently.
//
// usage: k7findings [--packages evidence/K7/packages.json] [--out findings]
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

type spec struct {
	ID                     string            `json:"id"`
	Campaign               string            `json:"campaign"`
	AnomalyClass           string            `json:"anomaly_class"`
	Original               string            `json:"original"`  // archive path
	Minimized              string            `json:"minimized"` // archive path
	Classification         map[string]string `json:"classification"`
	RecordedClassification map[string]string `json:"recorded_classification,omitempty"`
	Triage                 string            `json:"triage"`
}

type runInfo struct {
	Run           string   `json:"run_id"`
	Archive       string   `json:"archive,omitempty"`
	ArchiveSHA256 string   `json:"archive_sha256,omitempty"`
	SealRoot      string   `json:"seal_root"`
	Verdict       string   `json:"verdict"`
	Classes       []string `json:"classes"`

	man     map[string]any
	plan    []byte
	checks  []byte
	planSHA string
	c1Lost  bool
}

func readJSON(b []byte) (map[string]any, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var m map[string]any
	return m, d.Decode(&m)
}

func str(m map[string]any, k string) string {
	switch v := m[k].(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	}
	return ""
}

func fileSHA(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func runID(archive string) string {
	b := filepath.Base(archive)
	b = strings.TrimSuffix(b, ".tar.zst")
	return strings.TrimSuffix(b, ".stub")
}

// load reads a run archive (full or stub) and the checks.json beside it.
func load(archive string) (*runInfo, error) {
	id := runID(archive)
	tmp, err := os.MkdirTemp("", "k7findings-")
	if err != nil {
		return nil, err
	}
	defer func() {
		filepath.Walk(tmp, func(p string, _ os.FileInfo, _ error) error { os.Chmod(p, 0o755); return nil })
		os.RemoveAll(tmp)
	}()
	if err := history.ExtractArchive(archive, tmp); err != nil {
		return nil, fmt.Errorf("%s: %w", archive, err)
	}
	r := &runInfo{Run: id, Archive: filepath.ToSlash(archive)}
	if r.ArchiveSHA256, err = fileSHA(archive); err != nil {
		return nil, err
	}
	mb, err := os.ReadFile(filepath.Join(tmp, id, history.FileManifest))
	if err != nil {
		return nil, err
	}
	if r.man, err = readJSON(mb); err != nil {
		return nil, err
	}
	if r.plan, err = os.ReadFile(filepath.Join(tmp, id, "plan.json")); err != nil {
		return nil, err
	}
	h := sha256.Sum256(bytes.TrimSuffix(r.plan, []byte("\n")))
	r.planSHA = hex.EncodeToString(h[:])
	if r.planSHA != str(r.man, "plan_sha256") {
		return nil, fmt.Errorf("%s: plan.json %s, manifest %s", id, r.planSHA, str(r.man, "plan_sha256"))
	}
	sb, err := os.ReadFile(filepath.Join(tmp, id, history.FileSeal))
	if err != nil {
		return nil, err
	}
	seal, err := readJSON(sb)
	if err != nil {
		return nil, err
	}
	r.SealRoot = str(seal, "merkle_root")
	dir := filepath.Dir(archive)
	if r.checks, err = os.ReadFile(filepath.Join(dir, id+".checks.json")); err != nil {
		return nil, err
	}
	if err := r.readChecks(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *runInfo) readChecks() error {
	cj, err := readJSON(r.checks)
	if err != nil {
		return err
	}
	if str(cj, "seal_root") != r.SealRoot || str(cj, "run_id") != r.Run {
		return fmt.Errorf("%s: checks.json is for %s/%s", r.Run, str(cj, "run_id"), str(cj, "seal_root"))
	}
	r.Verdict = str(cj, "verdict")
	seen := map[string]bool{}
	r.Classes = []string{}
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
			if !seen[cl] {
				seen[cl] = true
				r.Classes = append(r.Classes, cl)
			}
			if str(c, "id") == "C1" && cl == "lost-write" {
				r.c1Lost = true
			}
		}
	}
	sort.Strings(r.Classes)
	return nil
}

func (r *runInfo) has(class string) bool {
	for _, c := range r.Classes {
		if c == class {
			return true
		}
	}
	return false
}

// replaysOf lists the recorded replays of the minimized run m: the runs of
// m's directory with m's plan hash whose manifest says replay_of m.
func replaysOf(m *runInfo, archive string) ([]*runInfo, error) {
	dir := filepath.Dir(archive)
	suffix := "-" + m.planSHA[:8]
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []*runInfo
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".tar.zst") || !strings.HasSuffix(runID(n), suffix) || runID(n) == m.Run {
			continue
		}
		r, err := load(filepath.Join(dir, n))
		if err != nil {
			return nil, err
		}
		// An INVALID attempt counts toward nothing (§12.1); the driver re-ran
		// it with the same plan.
		if str(r.man, "replay_of") == m.Run && r.Verdict != "INVALID" {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Run < out[j].Run })
	return out, nil
}

// lastRecord is the latest record named name or name-<n>.
func lastRecord(jsonl, name string) (map[string]any, error) {
	f, err := os.Open(jsonl)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	re := regexp.MustCompile("^" + regexp.QuoteMeta(name) + "(-[0-9]+)?$")
	var got map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		m, err := readJSON(sc.Bytes())
		if err != nil {
			return nil, err
		}
		if re.MatchString(str(m, "name")) {
			got = m
		}
	}
	return got, sc.Err()
}

func lastJSONLine(b []byte) (map[string]any, error) {
	lines := bytes.Split(bytes.TrimSpace(b), []byte("\n"))
	return readJSON(lines[len(lines)-1])
}

func strs(v any) []string {
	l, _ := v.([]any)
	out := []string{}
	for _, x := range l {
		out = append(out, fmt.Sprint(x))
	}
	return out
}

func repro(hits int) string {
	switch {
	case hits >= params.ReproducedAtLeast:
		return "REPRODUCED"
	case hits >= 1:
		return "INTERMITTENT"
	}
	return "NOT_REPRODUCED"
}

func toAny(v any) any {
	b, _ := json.Marshal(v)
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var out any
	d.Decode(&out)
	return out
}

func main() {
	pkgs := flag.String("packages", "evidence/K7/packages.json", "the hand-written list of findings")
	out := flag.String("out", "findings", "findings directory")
	ev := flag.String("evidence", "evidence/K7", "K7's evidence directory")
	flag.Parse()
	fail := func(err error) {
		fmt.Printf("{\"command\":\"k7findings\",\"error\":%q,\"exit_code\":2}\n", err.Error())
		os.Exit(2)
	}
	b, err := os.ReadFile(*pkgs)
	if err != nil {
		fail(err)
	}
	var specs []spec
	if err := json.Unmarshal(b, &specs); err != nil {
		fail(err)
	}
	// K7's repeats (ADR-0045), when results.json exists.
	type rec struct {
		Run       string `json:"run_id"`
		Cell      string `json:"cell"`
		Seed      int64  `json:"seed"`
		SealRoot  string `json:"seal_root"`
		Predicate string `json:"predicate"`
	}
	recur := map[string][]rec{}
	if rb, err := os.ReadFile(filepath.Join(*ev, "results.json")); err == nil {
		var res struct {
			Repeats []struct {
				Run            string `json:"run"`
				Cell           string `json:"cell"`
				Seed           int64  `json:"seed"`
				SealRoot       string `json:"seal_root"`
				Classification struct {
					Groups []struct {
						Predicate string   `json:"predicate"`
						Findings  []string `json:"findings"`
					} `json:"groups"`
				} `json:"classification"`
			} `json:"repeats"`
		}
		if err := json.Unmarshal(rb, &res); err != nil {
			fail(err)
		}
		for _, r := range res.Repeats {
			for _, g := range r.Classification.Groups {
				for _, fid := range g.Findings {
					recur[fid] = append(recur[fid], rec{r.Run, r.Cell, r.Seed, r.SealRoot, g.Predicate})
				}
			}
		}
	}
	n := 0
	for _, s := range specs {
		orig, err := load(s.Original)
		if err != nil {
			fail(err)
		}
		min, err := load(s.Minimized)
		if err != nil {
			fail(err)
		}
		if !orig.has(s.AnomalyClass) {
			fail(fmt.Errorf("%s: %s is not among the original run's classes %v", s.ID, s.AnomalyClass, orig.Classes))
		}
		reps, err := replaysOf(min, s.Minimized)
		if err != nil {
			fail(err)
		}
		hits := 0
		var rl []runInfo
		for _, r := range reps {
			if r.Verdict == "FAIL" && r.has(s.AnomalyClass) {
				hits++
			}
			rl = append(rl, *r)
		}
		if len(rl) != params.Replays {
			fail(fmt.Errorf("%s: %d recorded replays of %s, P13 says %d", s.ID, len(rl), min.Run, params.Replays))
		}
		if repro(hits) != s.Classification["reproducibility"] {
			fail(fmt.Errorf("%s: %d of %d recorded replays show %s (%s); packages.json says %s", s.ID, hits, len(rl), s.AnomalyClass, repro(hits), s.Classification["reproducibility"]))
		}
		f := map[string]any{"id": s.ID, "campaign": s.Campaign, "anomaly_class": s.AnomalyClass, "acknowledged_write_lost": orig.c1Lost,
			"classification": s.Classification, "original_run": *orig, "minimized_run": *min, "minimized_plan_sha256": min.planSHA,
			"replays": rl, "replays_with_class": hits, "triage": s.Triage}
		if s.RecordedClassification != nil {
			f["recorded_classification"] = s.RecordedClassification
		}
		rs := recur[s.ID]
		sort.Slice(rs, func(i, j int) bool { return rs[i].Run < rs[j].Run })
		if rs == nil {
			rs = []rec{}
		}
		f["recurrences"] = rs
		// K7.4's re-execution, once recorded.
		kr := map[string]any{"record": "", "runs": []any{}, "replays_with_class": 0, "reproducibility": ""}
		if r, err := lastRecord(filepath.Join(*ev, "commands.jsonl"), "pkg-replay-"+s.ID); err != nil {
			fail(err)
		} else if r != nil && str(r, "exit_code") == "0" {
			so, err := os.ReadFile(str(r, "stdout"))
			if err != nil {
				fail(err)
			}
			sum, err := lastJSONLine(so)
			if err != nil {
				fail(err)
			}
			var runs []runInfo
			h := 0
			for _, id := range strs(sum["replays"]) {
				x, err := load(filepath.Join(*ev, "package-replays", id+".stub.tar.zst"))
				if err != nil {
					fail(err)
				}
				if str(x.man, "replay_of") != min.Run {
					fail(fmt.Errorf("%s: %s is not a replay of %s", s.ID, id, min.Run))
				}
				if x.Verdict == "FAIL" && x.has(s.AnomalyClass) {
					h++
				}
				x.Archive, x.ArchiveSHA256 = "", ""
				runs = append(runs, *x)
			}
			kr = map[string]any{"record": str(r, "name"), "runs": runs, "replays_with_class": h, "reproducibility": repro(h)}
		}
		f["k7_replay"] = kr
		dir := filepath.Join(*out, s.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail(err)
		}
		fb, err := canonjson.Marshal(toAny(f))
		if err != nil {
			fail(err)
		}
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, fb, "", "  "); err != nil {
			fail(err)
		}
		pretty.WriteByte('\n')
		for name, data := range map[string][]byte{"finding.json": pretty.Bytes(), "plan.json": min.plan, "checks.json": orig.checks} {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				fail(err)
			}
		}
		n++
		fmt.Printf("{\"finding\":%q,\"original\":%q,\"minimized\":%q,\"replays_with_class\":%d,\"recurrences\":%d,\"k7_replay\":%q}\n", s.ID, orig.Run, min.Run, hits, len(rs), kr["record"])
	}
	fmt.Printf("{\"command\":\"k7findings\",\"packages\":%d,\"exit_code\":0}\n", n)
}
