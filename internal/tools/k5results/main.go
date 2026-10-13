// Command k5results writes evidence/K5/results.json from the sealed runs of
// the K5 runs directory (directories, full archives or stubs) and cpcheck's
// checks.json next to each: every run, its verdict and anomaly classes
// copied verbatim from checks.json (CONTRACT §15 K5.2: "results recorded
// whatever they are"), and for every FAIL primary run its minimization and
// replays (§12.2, §11.1; K5.5). It computes no verdict; cpgate re-derives
// everything from the same files.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/gate"
	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

type run struct {
	id, dir  string
	stub     bool
	man      map[string]any
	planSHA  string
	seal     string
	checks   map[string]any
	checkSHA string
	verdict  string
	classes  []string
}

func readJSON(p string) (map[string]any, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var m map[string]any
	return m, d.Decode(&m)
}

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

func load(runs, id, dir string, stub bool) (*run, error) {
	r := &run{id: id, dir: dir, stub: stub}
	var err error
	if r.man, err = readJSON(filepath.Join(dir, history.FileManifest)); err != nil {
		return nil, err
	}
	seal, err := readJSON(filepath.Join(dir, history.FileSeal))
	if err != nil {
		return nil, err
	}
	r.seal, r.planSHA = str(seal, "merkle_root"), str(r.man, "plan_sha256")
	cp := filepath.Join(runs, id+".checks.json")
	if r.checks, err = readJSON(cp); err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	r.checkSHA = gate.FileSHA256(cp)
	r.verdict = str(r.checks, "verdict")
	seen := map[string]bool{}
	cs, _ := r.checks["checkers"].([]any)
	for _, x := range cs {
		c, _ := x.(map[string]any)
		if fmt.Sprint(c["valid"]) != "false" {
			continue
		}
		as, _ := c["anomalies"].([]any)
		for _, y := range as {
			cl := str(y.(map[string]any), "class")
			if !seen[cl] {
				seen[cl] = true
				r.classes = append(r.classes, cl)
			}
		}
	}
	return r, nil
}

func kind(r *run) string {
	switch {
	case r.man["replay_of"] != nil:
		return "replay"
	case r.man["candidate_of"] != nil:
		return "candidate"
	}
	return "primary"
}

func has(r *run, c string) bool {
	for _, x := range r.classes {
		if x == c {
			return true
		}
	}
	return false
}

// target is ADR-0024's and ADR-0030's target class.
func target(r *run) string {
	exp := map[string][]string{"D3": {"duplicate-elements", "g1a"}, "D5": {"cdc-phantom"}}
	for _, c := range exp[str(r.man, "campaign")] {
		if has(r, c) {
			return c
		}
	}
	if len(r.classes) > 0 {
		return r.classes[0]
	}
	return ""
}

func strs(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func main() {
	runsDir := flag.String("runs", "", "K5 runs directory")
	out := flag.String("out", "", "results.json")
	flag.Parse()
	if *runsDir == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: k5results --runs <dir> --out results.json")
		os.Exit(3)
	}
	ents, err := os.ReadDir(*runsDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var all []*run
	by := map[string]*run{}
	tmp, _ := os.MkdirTemp("", "k5results-")
	defer os.RemoveAll(tmp)
	for _, e := range ents {
		n := e.Name()
		var r *run
		switch {
		case e.IsDir() && !strings.HasPrefix(n, "."):
			if _, err := os.Stat(filepath.Join(*runsDir, n, history.FileSeal)); err != nil {
				continue
			}
			r, err = load(*runsDir, n, filepath.Join(*runsDir, n), false)
		case strings.HasSuffix(n, gate.StubSuffix), strings.HasSuffix(n, gate.ArchiveSuffix):
			stub := strings.HasSuffix(n, gate.StubSuffix)
			id := strings.TrimSuffix(strings.TrimSuffix(n, gate.StubSuffix), gate.ArchiveSuffix)
			if err = gate.ExtractRun(filepath.Join(*runsDir, n), tmp); err == nil {
				r, err = load(*runsDir, id, filepath.Join(tmp, id), stub)
			}
		default:
			continue
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, n+":", err)
			os.Exit(2)
		}
		all = append(all, r)
		by[r.id] = r
	}
	sort.Slice(all, func(i, j int) bool { return all[i].id < all[j].id })
	var list []any
	fu := 0
	for _, r := range all {
		e := map[string]any{"run_id": r.id, "campaign": str(r.man, "campaign"), "seed": r.man["seed"], "profile": str(r.man, "profile"),
			"mutant": str(r.man, "mutant"), "kind": kind(r), "seal_root": r.seal, "checks_sha256": r.checkSHA, "verdict": r.verdict,
			"classes": strs(r.classes), "stub": r.stub, "plan_sha256": r.planSHA}
		for _, k := range []string{"replay_of", "replay_index", "candidate_of"} {
			if v, ok := r.man[k]; ok {
				e[k] = v
			}
		}
		if kind(r) == "primary" && r.verdict == "FAIL" {
			j, err := readJSON(filepath.Join(*runsDir, r.id+".minimize.json"))
			f := map[string]any{"target_class": target(r)}
			if err == nil {
				min := str(j, "minimized_run")
				f["minimized_run"] = min
				hits, valid := 0, 0
				var reps []any
				for _, x := range all {
					if str(x.man, "replay_of") == min && x.verdict != "INVALID" {
						valid++
						reps = append(reps, x.id)
						if x.verdict == "FAIL" && has(x, target(r)) {
							hits++
						}
					}
				}
				rep := "NOT_REPRODUCED"
				switch {
				case hits >= params.ReproducedAtLeast:
					rep = "REPRODUCED"
				case hits >= 1:
					rep = "INTERMITTENT"
				}
				f["replays"], f["replays_valid"], f["replays_with_class"], f["reproducibility"] = reps, int64(valid), int64(hits), rep
				if m := by[min]; m != nil {
					f["minimized_plan_sha256"] = m.planSHA
				}
				fu++
			}
			e["follow_up"] = f
		}
		list = append(list, e)
	}
	res := map[string]any{"contract_version": params.ContractVersion, "runs": list,
		"note": "verdicts and classes copied from each run's checks.json (cpcheck); follow-ups from the minimization journals and replay runs"}
	b, err := canonjson.Marshal(res)
	if err == nil {
		err = os.WriteFile(*out, append(b, '\n'), 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	s, _ := canonjson.Marshal(map[string]any{"command": "k5results", "runs": int64(len(all)), "follow_ups": int64(fu), "exit_code": int64(0)})
	fmt.Println(string(s))
}
