package gate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// K5 matrix (CONTRACT §15 K5; ADR-0030).
type k5Cell struct {
	Campaign, Profile string
	Seeds             []string
}

var (
	K5Campaigns = []k5Cell{
		{"C8", "buffered", []string{"1", "2", "3"}},
		{"C9", "baseline", []string{"1", "2", "3"}},
		{"C10", "baseline", []string{"1", "2", "3", "4", "5"}},
		{"C11", "baseline", []string{"1", "2", "3"}},
	}
	K5DirectedSeed = []string{"1", "2", "3", "4", "5"}
	K5Mutants      = []string{"M3", "M5"}
	// k5Buffered: the only campaigns that may run profile buffered (§15 K5
	// rejection condition; §5.5): C8, D3′ and the superseded D3.
	k5Buffered = map[string]bool{"C8": true, k5SupersededD3: true, k5D3Prime: true}
)

// D3′ (ADR-0033) is M3's directed schedule; the original D3 is superseded
// and its runs stay recorded (contract 1.0.9). The predictions were
// committed before any D3′ run, as a file of the evidence.
const (
	k5D3Prime        = "D3p"
	k5SupersededD3   = "D3"
	k5D3PPredictions = "predictions-D3p.md"
)

// k5Directed: mutant → directed campaign, profile and the expected
// (checker, class) pairs, any of which detects (§9.2, ADR-0030).
var k5Directed = map[string]struct {
	Campaign, Profile string
	Expected          [][2]string
}{
	"M3": {k5D3Prime, "buffered", [][2]string{{"C3", "duplicate-elements"}, {"C2", "g1a"}}},
	"M5": {"D5", "baseline", [][2]string{{"C7", "cdc-phantom"}}},
}

// The K5.1 records, which must precede every K5 run (prompts/K5.md §1:
// "done before any live K5 run").
const (
	k5CorpusOut   = "corpus"
	k5K2CorpusOut = "k2-corpus"
)

const (
	k5MinPerClass       = 3
	k5MinCleanPerFamily = 15
)

// k5ContractOK: a K5 run carries the current contract version, or the
// version of the K5 runs made before ADR-0033 unless it is a D3′ run
// (contracts 1.0.10 and 1.1.0 added D3′ and changed P25, no other K5 plan).
func k5ContractOK(campaign, version string) bool {
	return version == params.ContractVersion || version == params.K5RunsContractVersion && campaign != k5D3Prime
}

func (e *Evidence) k5() *k4Index { return e.runIndex("k5", k5ContractOK) }

// K5 returns the six criteria of CONTRACT §15 K5 with their sabotages.
func K5() []Criterion {
	return []Criterion{
		{ID: "K5.1", Name: "Synthetic validation", Eval: evalK51,
			Sabotage: "drop the reported cdc-order of planted entry w7k-order-01 from the K5 corpus results (the recorded results hash updated)",
			Apply:    sabotageK51},
		{ID: "K5.2", Name: "Campaign runs", Eval: evalK52,
			Sabotage: "flip the verdict of the first C9 run in results.json",
			Apply:    sabotageK52},
		{ID: "K5.3", Name: "CDC slot failover precondition", Eval: evalK53,
			Sabotage: "rewrite the cdc-slot observations before the first fault of one C10 run as not synced (run re-chained and re-sealed; its seal root updated in checks.json, results.json and any triage record)",
			Apply:    sabotageK53},
		{ID: "K5.4", Name: "Mutant detection", Eval: evalK54,
			Sabotage: "rewrite the checks.json of M3's D3′ runs for seeds 1 and 2 as PASS (seal roots kept)",
			Apply:    sabotageK54},
		{ID: "K5.5", Name: "Minimize/replay/triage", Eval: evalK55,
			Sabotage: "delete the last replay of the minimized run of the first FAIL run of M3 (archive and checks.json)",
			Apply:    sabotageK55},
		{ID: "K5.6", Name: "CI-equivalent", Eval: evalK56,
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

// ---- K5.1 ------------------------------------------------------------

// corpusRun checks one recorded `cpcheck corpus` run (or sabotage-corpus.sh
// run): argv (words), exit code, and that the stored results under out
// (possibly compressed) are the file it wrote.
func corpusRun(e *Evidence, name, corpus, out string, wantExit int, words ...string) ([]K2Result, map[string]any, []string) {
	r, stdout, err := e.Get(name)
	if err != nil {
		return nil, nil, []string{err.Error()}
	}
	var p []string
	if !argvHas(r, words...) {
		p = append(p, fmt.Sprintf("%s argv %q", name, r.Argv))
	}
	if r.ExitCode != wantExit {
		p = append(p, fmt.Sprintf("%s exited %d, want %d", name, r.ExitCode, wantExit))
	}
	s, err := LastJSONLine(stdout)
	if err != nil {
		return nil, nil, append(p, name+" summary: "+err.Error())
	}
	path := ResultsFile(filepath.Join(e.Dir, out))
	b, err := ReadMaybeZstd(path)
	if err != nil {
		return nil, s, append(p, err.Error())
	}
	if got := sha(b); got != str(s, "results_sha256") || got == "" {
		p = append(p, fmt.Sprintf("%s: stored results sha256 %s, the recorded run wrote %q", name, got, str(s, "results_sha256")))
	}
	rs, err := LoadK2Results(path)
	if err != nil {
		return nil, s, append(p, err.Error())
	}
	ids, err := CorpusIDs(corpus)
	if err != nil {
		return nil, s, append(p, err.Error())
	}
	seen := map[string]int{}
	for _, x := range rs {
		seen[x.ID]++
	}
	for _, id := range ids {
		if seen[id] != 1 {
			p = append(p, fmt.Sprintf("%s: corpus entry %s reported %d times", name, id, seen[id]))
		}
	}
	if len(rs) != len(ids) {
		p = append(p, fmt.Sprintf("%s: %d results for %d corpus entries", name, len(rs), len(ids)))
	}
	return rs, s, append(p, verifyCorpusSeals(corpus, rs)...)
}

// k5Setup are the records of K5.1, in the order k5-evidence.sh setup makes
// them.
func k5SetupRecords() []string {
	names := []string{"gencorpus-k5-verify", "gencorpus-k2-verify", "corpus-k5", "corpus-k2", "no-sabotage-symbols"}
	for _, r := range SabotageRules(K5SabotageDeps) {
		names = append(names, "sabotage-"+r)
	}
	return names
}

func evalK51(e *Evidence) (string, []string, []string) {
	var p, details []string
	for _, v := range []struct{ name, set string }{{"gencorpus-k5-verify", "k5"}, {"gencorpus-k2-verify", ""}} {
		words := []string{"gencorpus", "--verify"}
		if v.set != "" {
			words = append(words, "--set", v.set)
		}
		_, out, pp := exitOK(e, v.name, words...)
		p = append(p, pp...)
		if s, err := LastJSONLine(out); err != nil || fmt.Sprint(s["identical"]) != "true" {
			p = append(p, v.name+": the corpus does not regenerate byte for byte")
		}
	}
	// The K5 corpus: exact classification of every plant, clean histories
	// valid for every checker.
	rs, _, pp := corpusRun(e, "corpus-k5", K5Corpus, k5CorpusOut, 0, "cpcheck", "corpus", K5Corpus, "--results-only", "--out", "evidence/K5/"+k5CorpusOut)
	p = append(p, pp...)
	count := map[string]int{}
	clean := map[string]int{}
	for _, r := range rs {
		x, err := LoadK2Expected(K5Corpus, r)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		if d := K2Diff(r, x); len(d) > 0 {
			p = append(p, fmt.Sprintf("%s: %s", r.ID, strings.Join(d, "; ")))
			continue
		}
		if r.Label == "clean" {
			for _, c := range k2Checkers {
				if r.Reported[c].Valid != "true" {
					p = append(p, fmt.Sprintf("clean %s: %s not valid", r.ID, c))
				}
			}
			clean[r.Family]++
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
	var cd []string
	for _, c := range K5Classes {
		if count[c] < k5MinPerClass {
			p = append(p, fmt.Sprintf("class %s: %d matching planted histories, want ≥ %d", c, count[c], k5MinPerClass))
		}
		cd = append(cd, fmt.Sprintf("%s %d", c, count[c]))
	}
	var fd []string
	for _, f := range K5Families {
		if clean[f] < k5MinCleanPerFamily {
			p = append(p, fmt.Sprintf("%d clean %s histories, want ≥ %d", clean[f], f, k5MinCleanPerFamily))
		}
		fd = append(fd, fmt.Sprintf("%s %d", f, clean[f]))
	}
	details = append(details, fmt.Sprintf("K5 corpus %d entries; planted per class: %s; clean valid per family: %s", len(rs),
		strings.Join(cd, ", "), strings.Join(fd, ", ")))
	// The K2 corpus with the K5 checkers: unchanged (ADR-0030).
	k2, _, pp := corpusRun(e, "corpus-k2", K2Corpus, k5K2CorpusOut, 0, "cpcheck", "corpus", K2Corpus+" ", "--results-only", "--out", "evidence/K5/"+k5K2CorpusOut)
	p = append(p, pp...)
	mism := 0
	for _, r := range k2 {
		x, err := LoadK2Expected(K2Corpus, r)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		if d := K2Diff(r, x); len(d) > 0 {
			mism++
			p = append(p, fmt.Sprintf("K2 corpus %s: %s", r.ID, strings.Join(d, "; ")))
		}
	}
	details = append(details, fmt.Sprintf("K2 corpus with the K5 checkers: %d entries, %d mismatches", len(k2), mism))
	// Checker sabotage per rule.
	for _, rule := range SabotageRules(K5SabotageDeps) {
		name := "sabotage-" + rule
		_, s, pp := corpusRun(e, name, K5Corpus, "sabotage/"+rule, 1, "scripts/sabotage-corpus.sh", rule, K5Corpus, "evidence/K5/sabotage/"+rule)
		p = append(p, pp...)
		if s != nil {
			if sab, _ := s["sabotage"].([]any); len(sab) != 1 || sab[0] != rule {
				p = append(p, fmt.Sprintf("%s: binary reports active sabotage %v", name, s["sabotage"]))
			}
		}
		d, pp := CheckSabotage(K5SabotageDeps, rule, K5Corpus, ResultsFile(filepath.Join(e.Dir, "sabotage", rule)))
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
	// Order: K5.1 is done before any live K5 run (prompts/K5.md §1).
	var last time.Time
	for _, n := range k5SetupRecords() {
		if rec, _, err := e.Get(n); err == nil {
			if t, err := time.Parse(time.RFC3339Nano, rec.End); err == nil && t.After(last) {
				last = t
			} else if err != nil {
				p = append(p, fmt.Sprintf("%s: end time %q", n, rec.End))
			}
		}
	}
	first, firstID := time.Time{}, ""
	ix := e.k5()
	for _, x := range ix.Runs {
		t, err := time.Parse(time.RFC3339, str(x.Manifest, "start_wall_time"))
		if err != nil {
			continue
		}
		if first.IsZero() || t.Before(first) {
			first, firstID = t, x.ID
		}
	}
	if !first.IsZero() && !last.Before(first) {
		p = append(p, fmt.Sprintf("K5.1's records end at %s, not before the first K5 run %s (%s)", last.Format(time.RFC3339), firstID, first.Format(time.RFC3339)))
	}
	details = append(details, fmt.Sprintf("K5.1 records end %s; first K5 run %s starts %s", last.Format(time.RFC3339), firstID, first.Format(time.RFC3339)))
	return "corpus/, sabotage/, k2-corpus/", p, details
}

// sabotageK51 drops one reported class from the K5 corpus results and
// keeps the recorded results hash consistent, so that only the
// classification check can notice.
func sabotageK51(dir string) error {
	out := filepath.Join(dir, k5CorpusOut)
	path := ResultsFile(out)
	b, err := ReadMaybeZstd(path)
	if err != nil {
		return err
	}
	plain := filepath.Join(out, "results.jsonl")
	if err := os.WriteFile(plain, b, 0o644); err != nil {
		return err
	}
	if path != plain {
		os.Remove(path)
	}
	if err := editResult(plain, "w7k-order-01", func(m map[string]any) {
		obj(m, "reported")["C7"] = map[string]any{"valid": "true", "anomalies": []any{}, "observations": []any{}}
	}); err != nil {
		return err
	}
	nb, err := os.ReadFile(plain)
	if err != nil {
		return err
	}
	return editSummary(dir, "corpus-k5", func(m map[string]any) { m["results_sha256"] = sha(nb) })
}

// ---- K5.2 ------------------------------------------------------------

func k5Results(e *Evidence) (map[string]map[string]any, error) { return k4Results(e) }

func evalK52(e *Evidence) (string, []string, []string) {
	ix := e.k5()
	if ix.Error != nil {
		return "runs/", []string{ix.Error.Error()}, nil
	}
	res, err := k5Results(e)
	if err != nil {
		return "results.json", []string{"results.json: " + err.Error()}, nil
	}
	var p, details []string
	for _, c := range K5Campaigns {
		var vs []string
		for _, s := range c.Seeds {
			r, all, pp := ix.cell(c.Campaign, c.Profile, "real", s)
			p = append(p, pp...)
			for _, a := range all {
				// Primary runs other than C10's are committed as stubs
				// (ADR-0030); K5.3 requires C10's in full.
				p = append(p, runOK(a, true)...)
				p = append(p, checkResult(res, a)...)
			}
			if r != nil {
				vs = append(vs, fmt.Sprintf("seed %s %s%v", s, r.Verdict, r.Classes))
			}
		}
		details = append(details, fmt.Sprintf("%s (%s): %s", c.Campaign, c.Profile, strings.Join(vs, "; ")))
	}
	// Buffering only in C8 and D3′ (and the superseded D3), always there
	// (§15 K5).
	for _, r := range ix.Runs {
		if (r.profile() == "buffered") != k5Buffered[r.campaign()] {
			p = append(p, fmt.Sprintf("%s: campaign %s on profile %s (buffering is enabled in C8 and D3′/D3 only, and always there)", r.ID, r.campaign(), r.profile()))
		}
		if r.profile() != str(r.Plan, "profile") {
			p = append(p, fmt.Sprintf("%s: manifest profile %s, plan %s", r.ID, r.profile(), str(r.Plan, "profile")))
		}
	}
	return "results.json", p, details
}

func sabotageK52(dir string) error {
	p := filepath.Join(dir, "results.json")
	m, err := readJSON(p)
	if err != nil {
		return err
	}
	rs, _ := m["runs"].([]any)
	for _, x := range rs {
		r, _ := x.(map[string]any)
		if str(r, "campaign") == "C9" && str(r, "kind") == "primary" {
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
	return fmt.Errorf("no C9 run in results.json")
}

// ---- K5.3 ------------------------------------------------------------

// slotObs is one cdc-slot observation (runner K5.3 polling, ADR-0030).
type slotObs struct {
	t                                                         int64
	node                                                      string
	ok, inRecovery, present, failover, synced, temporary, inv bool
}

// slotRound reports whether one polling round (one observation per node of
// n1–n3) shows K5.3's readiness: every node answered, exactly one primary
// whose cp_slot has failover = true, and every node in recovery holding
// cp_slot failover-ready (synced, not temporary, not invalidated; S12).
func slotRound(round []slotObs) bool {
	primaries, followers := 0, 0
	for _, s := range round {
		switch {
		case !s.ok:
			return false
		case !s.inRecovery:
			primaries++
			if !s.present || !s.failover {
				return false
			}
		default:
			followers++
			if !(s.present && s.failover && s.synced && !s.temporary && !s.inv) {
				return false
			}
		}
	}
	return primaries == 1 && followers > 0
}

var k5Nodes = []string{"n1", "n2", "n3"}

// k5SlotReady recomputes K5.3 for one full run: the first nemesis event's
// time, whether a ready polling round precedes it, and the runner's marker.
func k5SlotReady(dir string) (ready bool, readyAt, firstFault int64, marker map[string]any, problems []string) {
	evs, err := history.ReadChain(filepath.Join(dir, history.FileHistory))
	if err != nil {
		return false, 0, 0, nil, []string{"history: " + err.Error()}
	}
	firstFault = -1
	for _, ev := range evs {
		if ev.Type == history.TypeNemesis && firstFault < 0 {
			firstFault = ev.T
		}
		if ev.Type == history.TypeMarker {
			v, _ := ev.Value.(map[string]any)
			if str(v, "name") == "cdc-slot-ready" && marker == nil {
				marker = v
			}
		}
	}
	obs, err := history.ReadChain(filepath.Join(dir, history.FileObservations))
	if err != nil {
		return false, 0, firstFault, marker, []string{"observations: " + err.Error()}
	}
	var cur []slotObs
	readyAt = -1
	for _, ev := range obs {
		v, _ := ev.Value.(map[string]any)
		if str(v, "src") != "cdc-slot" {
			continue
		}
		b := func(k string) bool { return fmt.Sprint(v[k]) == "true" }
		s := slotObs{t: ev.T, node: str(v, "node"), ok: b("ok"), inRecovery: b("in_recovery"), present: b("present"),
			failover: b("failover"), synced: b("synced"), temporary: b("temporary"), inv: b("invalidated")}
		if len(cur) < len(k5Nodes) && s.node == k5Nodes[len(cur)] {
			cur = append(cur, s)
		} else if s.node == k5Nodes[0] {
			cur = []slotObs{s}
		} else {
			cur = nil
		}
		if len(cur) == len(k5Nodes) {
			if (firstFault < 0 || s.t < firstFault) && slotRound(cur) && readyAt < 0 {
				readyAt = s.t
			}
			cur = nil
		}
	}
	return readyAt >= 0, readyAt, firstFault, marker, nil
}

func evalK53(e *Evidence) (string, []string, []string) {
	ix := e.k5()
	if ix.Error != nil {
		return "runs/", []string{ix.Error.Error()}, nil
	}
	var p, details []string
	full, stubs, invalid := 0, 0, 0
	for _, r := range ix.Runs {
		if r.campaign() != "C10" {
			continue
		}
		if r.Stub {
			// Derived runs (candidates, replays) may be stubs (ADR-0030);
			// the primary runs are read in full.
			if r.kind() == "primary" {
				p = append(p, r.ID+": a C10 primary run committed as a stub; K5.3 reads its observations")
			}
			stubs++
			continue
		}
		if len(r.Problems) > 0 {
			continue // K5.2 / K5.5 report integrity problems
		}
		full++
		ready, at, ff, marker, pp := k5SlotReady(r.Dir)
		for _, x := range pp {
			p = append(p, r.ID+": "+x)
		}
		mready := fmt.Sprint(marker["ready"]) == "true"
		switch {
		case r.Verdict == "INVALID":
			invalid++ // never counted (§12.1); a slot not ready is one reason
		case !ready || !mready:
			p = append(p, fmt.Sprintf("%s: counted as %s but the slot is not failover-ready on every follower before the first fault (recomputed %v, marker %v)", r.ID, r.Verdict, ready, marker["ready"]))
		default:
			details = append(details, fmt.Sprintf("%s: ready at %.1f s, first fault at %.1f s (marker waited %v ms)", r.ID, float64(at)/1e9, float64(ff)/1e9, marker["waited_ms"]))
		}
	}
	if full == 0 {
		p = append(p, "no C10 run committed in full")
	}
	details = append(details, fmt.Sprintf("%d C10 runs read in full (%d INVALID), %d stubs (derived runs; LIM-30)", full, invalid, stubs))
	sort.Strings(details)
	return "runs/ (C10 observations)", p, details
}

// rechain rewrites a hash-chained stream: edit may change a line's fields
// (never i, prev, t); every prev is recomputed.
func rechain(path string, edit func(m map[string]any) bool) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	prev := history.ZeroHash
	var out bytes.Buffer
	n := 0
	for _, l := range bytes.Split(bytes.TrimRight(b, "\n"), []byte("\n")) {
		var m map[string]any
		dec := json.NewDecoder(bytes.NewReader(l))
		dec.UseNumber()
		if err := dec.Decode(&m); err != nil {
			return 0, err
		}
		if edit(m) {
			n++
		}
		m["prev"] = prev
		nl, err := canonjson.Marshal(m)
		if err != nil {
			return 0, err
		}
		s := sha256.Sum256(nl)
		prev = hex.EncodeToString(s[:])
		out.Write(nl)
		out.WriteByte('\n')
	}
	return n, os.WriteFile(path, out.Bytes(), 0o644)
}

// sabotageK53 makes one C10 run's slot observations before its first fault
// say "not synced", consistently: the stream is re-chained, the run
// re-sealed and re-archived, and its seal root follows in checks.json,
// results.json and its triage record; only K5.3's recomputation can notice.
func sabotageK53(dir string) error {
	e := LoadEvidence(dir)
	defer e.Close()
	ix := e.k5()
	var target *k4Run
	for _, r := range ix.Runs {
		if r.campaign() == "C10" && r.kind() == "primary" && !r.Stub && r.valid() {
			target = r
			break
		}
	}
	if target == nil {
		return fmt.Errorf("no full, valid C10 primary run")
	}
	run := target.Dir
	filepath.WalkDir(run, func(p string, d fs.DirEntry, err error) error {
		if err == nil {
			os.Chmod(p, 0o755)
		}
		return nil
	})
	_, _, ff, _, pp := k5SlotReady(run)
	if len(pp) > 0 {
		return fmt.Errorf("%v", pp)
	}
	n, err := rechain(filepath.Join(run, history.FileObservations), func(m map[string]any) bool {
		v, _ := m["value"].(map[string]any)
		t, _ := m["t"].(json.Number).Int64()
		if str(v, "src") != "cdc-slot" || (ff >= 0 && t >= ff) || fmt.Sprint(v["in_recovery"]) != "true" {
			return false
		}
		v["synced"] = false
		return true
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%s: no follower cdc-slot observation before the first fault", target.ID)
	}
	os.Remove(filepath.Join(run, history.FileSeal))
	seal, err := history.SealDir(run)
	if err != nil {
		return err
	}
	if err := ArchiveRun(run, filepath.Join(dir, "runs", target.ID+ArchiveSuffix)); err != nil {
		return err
	}
	if err := editJSONCanonical(filepath.Join(dir, "runs", target.ID+".checks.json"), func(m map[string]any) { m["seal_root"] = seal.MerkleRoot }); err != nil {
		return err
	}
	cb, _ := os.ReadFile(filepath.Join(dir, "runs", target.ID+".checks.json"))
	if err := editResults(dir, target.ID, func(r map[string]any) {
		r["seal_root"], r["checks_sha256"] = seal.MerkleRoot, sha(cb)
	}); err != nil {
		return err
	}
	tp := filepath.Join(dir, "triage", target.ID+".json")
	if _, err := os.Stat(tp); err == nil {
		return editJSON(tp, func(m map[string]any) { m["seal_root"], m["checks_sha256"] = seal.MerkleRoot, sha(cb) })
	}
	return nil
}

// editResults rewrites one run's entry of results.json.
func editResults(dir, id string, f func(map[string]any)) error {
	p := filepath.Join(dir, "results.json")
	m, err := readJSON(p)
	if err != nil {
		return err
	}
	rs, _ := m["runs"].([]any)
	found := false
	for _, x := range rs {
		r, _ := x.(map[string]any)
		if str(r, "run_id") == id {
			f(r)
			found = true
		}
	}
	if !found {
		return fmt.Errorf("results.json lacks %s", id)
	}
	b, err := canonjson.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

// ---- K5.4 ------------------------------------------------------------

// k5Detects: a directed run detects its mutant iff it is FAIL with one of
// the expected classes, reported by that class's checker.
func k5Detects(r *k4Run, expected [][2]string) bool {
	if r.Verdict != "FAIL" {
		return false
	}
	for _, x := range r.Anoms {
		for _, w := range expected {
			if x.Checker == w[0] && x.Class == w[1] {
				return true
			}
		}
	}
	return false
}

func evalK54(e *Evidence) (string, []string, []string) {
	ix := e.k5()
	if ix.Error != nil {
		return "runs/", []string{ix.Error.Error()}, nil
	}
	res, err := k5Results(e)
	if err != nil {
		return "results.json", []string{"results.json: " + err.Error()}, nil
	}
	var p, details []string
	for _, m := range K5Mutants {
		d := k5Directed[m]
		hits := 0
		var mv, rv []string
		for _, s := range K5DirectedSeed {
			r, all, pp := ix.cell(d.Campaign, d.Profile, m, s)
			p = append(p, pp...)
			for _, a := range all {
				p = append(p, runOK(a, true)...)
				p = append(p, checkResult(res, a)...)
				if a.valid() && str(a.Manifest, "patch_sha256") == "" {
					p = append(p, a.ID+": mutant run without patch_sha256 in its manifest")
				}
			}
			if r != nil {
				if k5Detects(r, d.Expected) {
					hits++
				}
				mv = append(mv, fmt.Sprintf("%s%v", r.Verdict, r.Classes))
			}
			// The real build on the same schedule, recorded whatever it is.
			rr, rall, rp := ix.cell(d.Campaign, d.Profile, "real", s)
			p = append(p, rp...)
			for _, a := range rall {
				p = append(p, runOK(a, true)...)
				p = append(p, checkResult(res, a)...)
			}
			if rr != nil {
				rv = append(rv, fmt.Sprintf("%s%v", rr.Verdict, rr.Classes))
			}
		}
		outcome := "DETECTED"
		if hits < params.DetectedAtLeast {
			outcome = "UNDETECTED"
			p = append(p, fmt.Sprintf("%s via %s: %d/%d runs FAIL with %v, need %d", m, d.Campaign, hits, len(K5DirectedSeed), d.Expected, params.DetectedAtLeast))
		}
		details = append(details, fmt.Sprintf("%s via %s (%s): %s %d/5 with %v; mutant %v; real %v", m, d.Campaign, d.Profile, outcome, hits, d.Expected, mv, rv))
	}
	// D3′'s predictions are part of the evidence (ADR-0033: committed before
	// any D3′ run).
	if st, err := os.Stat(filepath.Join(e.Dir, k5D3PPredictions)); err != nil || st.Size() == 0 {
		p = append(p, k5D3PPredictions+": missing or empty (ADR-0033)")
	}
	// The superseded D3 (ADR-0033): its runs stay recorded, unedited, and
	// are checked like any run; they no longer decide M3's detection.
	sup := map[string][]string{}
	for _, s := range K5DirectedSeed {
		for _, b := range []string{"M3", "real"} {
			r, all, pp := ix.cell(k5SupersededD3, "buffered", b, s)
			p = append(p, pp...)
			for _, a := range all {
				p = append(p, runOK(a, true)...)
				p = append(p, checkResult(res, a)...)
			}
			if r != nil {
				sup[b] = append(sup[b], fmt.Sprintf("%s%v", r.Verdict, r.Classes))
			}
		}
	}
	details = append(details, fmt.Sprintf("superseded %s (ADR-0033, recorded): mutant M3 %v; real %v", k5SupersededD3, sup["M3"], sup["real"]))
	// Isolation as K4.4: the mutant images differ from the real build only
	// in binaries that link a patched package.
	pp, dd := k5Isolation(e, ix)
	return "runs/, images.lock.json, mutants/", append(p, pp...), append(details, dd...)
}

func k5Isolation(e *Evidence, ix *k4Index) ([]string, []string) {
	var p, details []string
	lock, err := readJSON(filepath.Join(e.Dir, "images.lock.json"))
	if err != nil {
		return []string{"images.lock.json: " + err.Error()}, nil
	}
	imgs, _ := lock["images"].(map[string]any)
	vars, _ := lock["variants"].(map[string]any)
	real, _ := imgs["commitproof/multigres:real"].(map[string]any)
	rb, _ := real["binaries"].(map[string]any)
	for _, m := range K5Mutants {
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
		details = append(details, fmt.Sprintf("%s isolation: %d binary(ies) linking %v differ; the others equal the real build", m, changed, v["files"]))
	}
	return p, details
}

func sabotageK54(dir string) error {
	e := LoadEvidence(dir)
	defer e.Close()
	ix := e.k5()
	for _, s := range []string{"1", "2"} {
		r, _, _ := ix.cell(k5D3Prime, "buffered", "M3", s)
		if r == nil {
			return fmt.Errorf("no M3 D3′ run for seed %s", s)
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

// ---- K5.5 ------------------------------------------------------------

func evalK55(e *Evidence) (string, []string, []string) {
	ix := e.k5()
	if ix.Error != nil {
		return "runs/", []string{ix.Error.Error()}, nil
	}
	res, err := k5Results(e)
	if err != nil {
		return "results.json", []string{"results.json: " + err.Error()}, nil
	}
	var p, details []string
	n := 0
	realFail := map[string]*k4Run{}
	for _, r := range ix.Runs {
		if r.kind() != "primary" || !r.valid() || r.Verdict != "FAIL" {
			continue
		}
		n++
		real := r.mutant() == "real"
		if real {
			realFail[r.ID] = r
			// Triage reads the run's history: committed in full (ADR-0030).
			p = append(p, runOK(r, false)...)
		}
		// A real-build failure's minimized run is committed in full; its
		// replays and every derived run of a mutant failure may be stubs.
		d, pp := followUp(ix, res, e.Dir, r, !real, true)
		p = append(p, pp...)
		details = append(details, d)
	}
	details = append(details, fmt.Sprintf("%d FAIL runs minimized and replayed", n))
	// Triage (as K4.7) of every real-build FAIL.
	tdir := filepath.Join(e.Dir, "triage")
	ents, _ := os.ReadDir(tdir)
	have := map[string]bool{}
	for _, ent := range ents {
		if !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(ent.Name(), ".json")
		have[id] = true
		r := realFail[id]
		if r == nil {
			p = append(p, fmt.Sprintf("triage/%s: not a real-build FAIL run of K5", ent.Name()))
			continue
		}
		t, err := readJSON(filepath.Join(tdir, ent.Name()))
		if err != nil {
			p = append(p, "triage/"+ent.Name()+": "+err.Error())
			continue
		}
		pp, d := checkTriage(e, ix, r, t)
		p = append(p, pp...)
		details = append(details, d)
	}
	for id := range realFail {
		if !have[id] {
			p = append(p, fmt.Sprintf("real-build FAIL %s has no triage record", id))
		}
	}
	details = append(details, fmt.Sprintf("%d real-build FAIL runs, %d triage records", len(realFail), len(have)))
	sort.Strings(details)
	return "runs/ (*.minimize.json, replays), triage/", p, details
}

func sabotageK55(dir string) error {
	e := LoadEvidence(dir)
	defer e.Close()
	ix := e.k5()
	// The first FAIL run of M3 on D3′, else on the superseded D3.
	var cells [][2]string
	for _, c := range []string{k5D3Prime, k5SupersededD3} {
		for _, s := range K5DirectedSeed {
			cells = append(cells, [2]string{c, s})
		}
	}
	for _, cs := range cells {
		r, _, _ := ix.cell(cs[0], "buffered", "M3", cs[1])
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
	return fmt.Errorf("no FAIL run of M3")
}

// ---- K5.6 ------------------------------------------------------------

func evalK56(e *Evidence) (string, []string, []string) {
	name := lastCI(e)
	if name == "" {
		return "commands.jsonl", []string{"no ci record"}, nil
	}
	r, _, p := exitOK(e, name, "make", "ci-unit", "ci-checkers", "ci-cluster-smoke", "ci-nemesis-smoke", "ci-mutant-smoke")
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
