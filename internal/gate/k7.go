package gate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/repeats"
	"github.com/CesarManzoCode/commitproof/internal/stats"
)

// K7's evidence layout (CONTRACT §15 K7, §16; ADR-0039, ADR-0044–ADR-0046).
// cpgate K7 reads evidence/K7 and, two levels up, the repository's release
// files: campaigns/k7.json, findings/, claims.yaml, README.md,
// docs/LIMITATIONS.md, docs/CONTRACT.md, and the cited evidence/K4 and
// evidence/K5.
const (
	// k7MatrixVersion is the contract version K7's matrix ran under: the
	// version every K7 run's manifest carries (no bump while the matrix
	// ran, ADR-0041 / prompts/K7.md).
	k7MatrixVersion = "1.8.1"
	// k7ReplayVersion is the contract version K7.4's package replays ran
	// under: ADR-0047 bumped the contract after the matrix, and the runner
	// records 1.9.0 in the manifests of the runs made after it (the K7.4
	// replays, which reuse their packages' plans).
	k7ReplayVersion = "1.9.0"
	k7RunsSub       = "runs"            // matrix runs, their follow-ups
	k7PkgSub        = "package-replays" // K7.4's replays of each package
	k7CitedSub      = "cited"           // D3′/D5 real-build runs in full, if committed (ADR-0045 §5)
)

func k7Root(e *Evidence) string { return filepath.Dir(filepath.Dir(filepath.Clean(e.Dir))) }

func k7ContractOK(_, version string) bool { return version == k7MatrixVersion }

func k7ReplayContractOK(_, version string) bool { return version == k7ReplayVersion }

// k7IndexContract: the contract version each K7 runs directory's manifests
// must carry.
var k7IndexContract = map[string]func(campaign, version string) bool{
	k7RunsSub: k7ContractOK,
	k7PkgSub:  k7ReplayContractOK,
}

func (e *Evidence) runIndexIn(key, sub string, contractOK func(campaign, version string) bool) *k4Index {
	if e.memo == nil {
		e.memo = map[string]any{}
	}
	if ix, ok := e.memo[key].(*k4Index); ok {
		return ix
	}
	ix := buildRunIndexIn(e, sub, contractOK)
	e.memo[key] = ix
	return ix
}

func (e *Evidence) k7() *k4Index {
	return e.runIndexIn("k7", k7RunsSub, k7IndexContract[k7RunsSub])
}

func (e *Evidence) k7pkg() *k4Index {
	return e.runIndexIn("k7pkg", k7PkgSub, k7IndexContract[k7PkgSub])
}

// k7Cited loads the evidence of an earlier sprint K7 cites (K4, K5).
func (e *Evidence) k7Cited(sprint string) (*Evidence, *k4Index) {
	if e.memo == nil {
		e.memo = map[string]any{}
	}
	if c, ok := e.memo["cited-"+sprint].(*Evidence); ok {
		return c, e.memo["cited-ix-"+sprint].(*k4Index)
	}
	c := LoadEvidence(filepath.Join(k7Root(e), "evidence", sprint))
	var ix *k4Index
	if sprint == "K4" {
		ix = c.k4()
	} else {
		ix = c.k5()
	}
	e.memo["cited-"+sprint], e.memo["cited-ix-"+sprint] = c, ix // closed with e
	return c, ix
}

// k7CellSpec is a §15 K7 cell K7 runs (P15; ADR-0040 removed V4).
type k7CellSpec struct {
	Cell, Campaign, Profile string
	Runs                    int
}

func k7Spec() []k7CellSpec {
	var out []k7CellSpec
	for _, c := range []string{"C1", "C2", "C3", "C4", "C5", "C6", "C7", "C9", "C10", "C12"} {
		out = append(out, k7CellSpec{"V1", c, "baseline", params.RepsV1V3})
	}
	for _, c := range []string{"C1", "C2", "C4"} {
		out = append(out, k7CellSpec{"V2", c, "slotless", params.RepsV2V5})
	}
	out = append(out, k7CellSpec{"V3", "C8", "buffered", params.RepsV1V3}, k7CellSpec{"V5", "C11", "baseline", params.RepsV2V5})
	return out
}

// k7CitedCell is a directed cell K7 cites from K4 or K5 (ADR-0039).
type k7CitedCell struct {
	Sprint, Campaign, Profile, Mutant, Version string
}

var k7CitedCells = []k7CitedCell{
	{"K4", "D1", "baseline", "M1", "1.0.6"}, {"K4", "D1", "baseline", "real", "1.0.6"},
	{"K4", "D2p", "baseline", "M2", "1.0.7"}, {"K4", "D2p", "baseline", "real", "1.0.7"},
	{"K4", "claims", "baseline", "M4", "1.0.6"}, {"K4", "claims", "baseline", "real", "1.0.6"},
	{"K4", "DH1", "h1", "real", "1.0.6"},
	{"K5", "D3p", "buffered", "M3", "1.1.0"}, {"K5", "D3p", "buffered", "real", "1.1.0"},
	{"K5", "D5", "baseline", "M5", "1.0.9"}, {"K5", "D5", "baseline", "real", "1.0.9"},
}

// k7Matrix reads campaigns/k7.json.
type k7MatrixFile struct {
	Name             string `json:"name"`
	MinimizeFailures bool   `json:"minimize_failures"`
	SkipRepeats      bool   `json:"skip_repeats"`
	Replays          int    `json:"replays"`
	Entries          []struct {
		Cell     string   `json:"cell"`
		Campaign string   `json:"campaign"`
		Profile  string   `json:"profile"`
		Mutant   string   `json:"mutant"`
		Seeds    []uint64 `json:"seeds"`
	} `json:"entries"`
}

func k7Matrix(e *Evidence) (*k7MatrixFile, error) {
	b, err := os.ReadFile(filepath.Join(k7Root(e), "campaigns", "k7.json"))
	if err != nil {
		return nil, err
	}
	var m k7MatrixFile
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("campaigns/k7.json: %w", err)
	}
	for i := range m.Entries {
		if m.Entries[i].Profile == "" {
			m.Entries[i].Profile = "baseline"
		}
	}
	return &m, nil
}

// K7 returns the seven criteria of CONTRACT §15 K7 with their sabotages.
func K7() []Criterion {
	return []Criterion{
		{ID: "K7.1", Name: "Matrix complete", Eval: evalK71,
			Sabotage: "campaigns/k7.json of the copy lists an eleventh seed for V1's C1 (the cell needs a valid run for it; P15 says 10)",
			Apply:    sabotageK71},
		{ID: "K7.2", Name: "Statistics", Eval: evalK72,
			Sabotage: "cpreport's data block for K7's first cell reports one more valid run (n + 1); the render record's output hash rewritten to the forged page's",
			Apply:    sabotageK72},
		{ID: "K7.3", Name: "Oracle on findings", Eval: evalK73,
			Sabotage: "one oracle record's result rewritten from oracle-disagreement to confirmed (M1 D1 seed 1, where elle-cli gave no answer)",
			Apply:    sabotageK73},
		{ID: "K7.4", Name: "Findings packages", Eval: evalK74,
			Sabotage: "the Limitations section removed from the first finding package's README",
			Apply:    sabotageK74},
		{ID: "K7.5", Name: "Claims audit", Eval: evalK75,
			Sabotage: "the sha256 of the first claim of claims.yaml altered",
			Apply:    sabotageK75},
		{ID: "K7.6", Name: "Overclaim correction", Eval: evalK76,
			Sabotage: "the bound of the first §12.3 clean statement in the README altered (one more thousandth)",
			Apply:    sabotageK76},
		{ID: "K7.7", Name: "Release", Eval: evalK77,
			Sabotage: "the tag-check record rewritten as if v1.0.0 were a lightweight tag (object type commit)",
			Apply:    sabotageK77},
	}
}

// ---------------------------------------------------------------- K7.1

func evalK71(e *Evidence) (string, []string, []string) {
	var p, d []string
	m, err := k7Matrix(e)
	if err != nil {
		return "results.json", []string{err.Error()}, nil
	}
	// The matrix file is §15 K7's: every cell, its profile and P15's count,
	// seeds 1…N, the real build; ADR-0045's settings.
	if !m.MinimizeFailures || !m.SkipRepeats || m.Replays != params.Replays {
		p = append(p, fmt.Sprintf("matrix: minimize_failures %v skip_repeats %v replays %d (ADR-0045, P13)", m.MinimizeFailures, m.SkipRepeats, m.Replays))
	}
	want := map[string]k7CellSpec{}
	for _, c := range k7Spec() {
		want[c.Campaign+"/"+c.Profile] = c
	}
	seen := map[string]bool{}
	for _, en := range m.Entries {
		key := en.Campaign + "/" + en.Profile
		c, ok := want[key]
		switch {
		case !ok:
			p = append(p, fmt.Sprintf("matrix: %s is not a K7 cell (§15 K7)", key))
			continue
		case seen[key]:
			p = append(p, fmt.Sprintf("matrix: %s listed twice", key))
		case en.Cell != c.Cell || en.Mutant != "":
			p = append(p, fmt.Sprintf("matrix: %s labelled %q build %q, want %s on the real build", key, en.Cell, en.Mutant, c.Cell))
		}
		seen[key] = true
		if len(en.Seeds) != c.Runs {
			p = append(p, fmt.Sprintf("matrix: %s has %d seeds, P15 says %d", key, len(en.Seeds), c.Runs))
		}
	}
	for k := range want {
		if !seen[k] {
			p = append(p, fmt.Sprintf("matrix: cell %s missing", k))
		}
	}
	ix := e.k7()
	if ix.Error != nil {
		return "runs", append(p, ix.Error.Error()), nil
	}
	for _, r := range ix.Runs {
		if len(r.Problems) > 0 {
			p = append(p, fmt.Sprintf("run %s: %v", r.ID, r.Problems))
		}
		if r.mutant() != "real" {
			p = append(p, fmt.Sprintf("run %s: build %s, K7 runs the real build only", r.ID, r.mutant()))
		}
	}
	res, err := readJSON(filepath.Join(e.Dir, "results.json"))
	if err != nil {
		return "results.json", append(p, err.Error()), nil
	}
	rcells, _ := res["cells"].([]any)
	byKey := map[string]map[string]any{}
	for _, x := range rcells {
		c, _ := x.(map[string]any)
		byKey[fmt.Sprintf("%s/%s/%v", str(c, "campaign"), str(c, "profile"), c["seed"])] = c
	}
	counted := 0
	listedInvalid := map[string]bool{}
	for _, en := range m.Entries {
		for _, seed := range en.Seeds {
			valid, all, cp := ix.cell(en.Campaign, en.Profile, "real", fmt.Sprint(seed))
			p = append(p, cp...)
			rc := byKey[fmt.Sprintf("%s/%s/%d", en.Campaign, en.Profile, seed)]
			if rc == nil {
				p = append(p, fmt.Sprintf("results.json: no entry for %s/%s seed %d", en.Campaign, en.Profile, seed))
				continue
			}
			for _, x := range rc["invalid_attempts"].([]any) {
				ia, _ := x.(map[string]any)
				listedInvalid[str(ia, "run")] = true
				if len(strListOf(ia["reasons"])) == 0 {
					p = append(p, fmt.Sprintf("results.json: INVALID attempt %s listed without its cause", str(ia, "run")))
				}
			}
			for _, r := range all {
				if r.Verdict == "INVALID" && !listedInvalid[r.ID] {
					p = append(p, fmt.Sprintf("%s/%s seed %d: INVALID run %s not listed with its cause", en.Campaign, en.Profile, seed, r.ID))
				}
			}
			if valid == nil {
				continue
			}
			counted++
			if str(rc, "run") != valid.ID || str(rc, "verdict") != valid.Verdict || strings.Join(strListOf(rc["classes"]), ",") != strings.Join(sortedCopy(valid.Classes), ",") {
				p = append(p, fmt.Sprintf("results.json: %s/%s seed %d says %s %s %v, the run is %s %s %v", en.Campaign, en.Profile, seed,
					str(rc, "run"), str(rc, "verdict"), rc["classes"], valid.ID, valid.Verdict, valid.Classes))
			}
			if v := str(valid.Plan, "contract_version"); v != k7MatrixVersion {
				p = append(p, fmt.Sprintf("%s: plan contract version %s, the matrix ran under %s", valid.ID, v, k7MatrixVersion))
			}
		}
	}
	d = append(d, fmt.Sprintf("%d matrix cells with a valid run; %d INVALID attempts listed with causes", counted, len(listedInvalid)))
	// The cited directed cells: 5 valid runs each, with their contract
	// versions (ADR-0039).
	for _, c := range k7CitedCells {
		_, cix := e.k7Cited(c.Sprint)
		if cix.Error != nil {
			p = append(p, fmt.Sprintf("%s: %v", c.Sprint, cix.Error))
			continue
		}
		n := 0
		for _, r := range cix.Runs {
			if r.kind() != "primary" || r.campaign() != c.Campaign || r.profile() != c.Profile || r.mutant() != c.Mutant || !r.valid() {
				continue
			}
			n++
			if v := str(r.Plan, "contract_version"); v != c.Version {
				p = append(p, fmt.Sprintf("cited %s %s/%s: run %s plan version %s, ADR-0039 says %s", c.Sprint, c.Campaign, c.Mutant, r.ID, v, c.Version))
			}
		}
		if n != params.DirectedRuns {
			p = append(p, fmt.Sprintf("cited %s %s/%s/%s: %d valid runs, want %d", c.Sprint, c.Campaign, c.Profile, c.Mutant, n, params.DirectedRuns))
		}
	}
	d = append(d, fmt.Sprintf("%d cited directed cells, %d valid runs each", len(k7CitedCells), params.DirectedRuns))
	return "results.json", p, d
}

func sortedCopy(ss []string) []string {
	out := append([]string{}, ss...)
	sort.Strings(out)
	return out
}

func sabotageK71(dir string) error {
	p := filepath.Join(filepath.Dir(filepath.Dir(dir)), "campaigns", "k7.json")
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`("campaign": "C1", "seeds": \[1, 2, 3, 4, 5, 6, 7, 8, 9, 10)\]`)
	if !re.Match(b) {
		return fmt.Errorf("no V1 C1 entry in the matrix copy")
	}
	return os.WriteFile(p, re.ReplaceAll(b, []byte("${1}, 11]")), 0o644)
}

// ---------------------------------------------------------------- K7.2

// k7Stat is one cell's §12.3 statistics as cpgate recomputes them (ADR-0036,
// ADR-0044).
type k7Stat struct {
	Campaign, Profile, Mutant       string
	N, Invalid, Pass, Fail, Unk     int64
	Classes                         map[string]int64
	FullRuns, ChecksRuns            int64
	Ops, OKWrites, FaultTime        int64
	Confirmed                       map[string]int64
	Checkers, Multigres             map[string]bool
	G12Windows, G12LitTot, G12LitOK int64
	G12JudTot, G12JudOK             int64
	G12LitBelow, G12JudBelow        int64
	G12NotJudged                    int64
	Runs                            []string
}

// k7Exposure is ADR-0036's exposure of a run, computed by cpgate from its
// history (independently of cpcheck's C9 and of cpreport).
type k7Exposure struct {
	Ops, OKWrites, FaultTime int64
	Confirmed                map[string]int64
}

func k7HistoryExposure(dir string) (*k7Exposure, error) {
	evs, err := history.ReadChain(filepath.Join(dir, history.FileHistory))
	if err != nil {
		return nil, err
	}
	x := &k7Exposure{Confirmed: map[string]int64{}}
	inv := map[int64]history.Event{}
	confirmed := map[string]int64{}
	for _, ev := range evs {
		switch ev.Type {
		case "invoke":
			inv[ev.I] = ev
			if ev.F != "final-read" {
				x.Ops++
			}
		case "ok":
			if ev.Op == nil {
				continue
			}
			iv, ok := inv[*ev.Op]
			if !ok || iv.F == "final-read" {
				continue
			}
			switch {
			case iv.WL == "ledger" && iv.F == "insert", iv.WL == "register" && (iv.F == "write" || iv.F == "cas"), iv.WL == "bank" && iv.F == "transfer":
				x.OKWrites++
			case iv.WL == "append":
				mops, _ := iv.Value.([]any)
				for _, m := range mops {
					if mo, _ := m.([]any); len(mo) == 3 && fmt.Sprint(mo[0]) == "append" {
						x.OKWrites++
					}
				}
			}
		case "nemesis":
			id := fmt.Sprint(ev.Fault["id"])
			switch fmt.Sprint(ev.Fault["phase"]) {
			case "confirmed":
				confirmed[id] = ev.T
				x.Confirmed[fmt.Sprint(ev.Fault["kind"])]++
			case "heal":
				if t, ok := confirmed[id]; ok {
					x.FaultTime += ev.T - t
				}
			}
		}
	}
	return x, nil
}

// k7ChecksExposure is C9's stats.exposure of a checks.json (nil without).
func k7ChecksExposure(checks map[string]any) *k7Exposure {
	cs, _ := checks["checkers"].([]any)
	for _, y := range cs {
		c, _ := y.(map[string]any)
		if str(c, "id") != "C9" {
			continue
		}
		st, _ := c["stats"].(map[string]any)
		ex, ok := st["exposure"].(map[string]any)
		if !ok {
			return nil
		}
		x := &k7Exposure{Ops: k7i(ex["operations"]), OKWrites: k7i(ex["ok_writes"]), FaultTime: k7i(ex["fault_time_ns"]), Confirmed: map[string]int64{}}
		cf, _ := ex["confirmed_faults"].(map[string]any)
		for k, v := range cf {
			x.Confirmed[k] = k7i(v)
		}
		return x
	}
	return nil
}

func (a *k7Exposure) equal(b *k7Exposure) bool {
	return a.Ops == b.Ops && a.OKWrites == b.OKWrites && a.FaultTime == b.FaultTime && equalIntMaps(a.Confirmed, b.Confirmed)
}

func equalIntMaps(a, b map[string]int64) bool {
	for k, v := range a {
		if v != 0 && b[k] != v {
			return false
		}
	}
	for k, v := range b {
		if v != 0 && a[k] != v {
			return false
		}
	}
	return true
}

// k7Stats computes the cells of an index's primary runs. Exposure: from the
// history of a run held in full, from checks.json for a summary that
// carries it (ADR-0044). For a run held in full whose checks.json carries
// an exposure, the two are compared (problems returned).
func k7Stats(ix *k4Index) ([]*k7Stat, []string) {
	var p []string
	by := map[string]*k7Stat{}
	var keys []string
	for _, r := range ix.Runs {
		if r.kind() != "primary" {
			continue
		}
		k := r.campaign() + "\x00" + r.profile() + "\x00" + r.mutant()
		c := by[k]
		if c == nil {
			c = &k7Stat{Campaign: r.campaign(), Profile: r.profile(), Mutant: r.mutant(), Classes: map[string]int64{}, Confirmed: map[string]int64{},
				Checkers: map[string]bool{}, Multigres: map[string]bool{}}
			by[k] = c
			keys = append(keys, k)
		}
		c.Runs = append(c.Runs, r.ID)
		if !r.valid() {
			if r.Verdict == "INVALID" {
				c.Invalid++
			} else {
				p = append(p, fmt.Sprintf("run %s: %v", r.ID, r.Problems))
			}
			continue
		}
		c.N++
		switch r.Verdict {
		case "PASS":
			c.Pass++
		case "FAIL":
			c.Fail++
		default:
			c.Unk++
		}
		for _, cl := range r.Classes {
			c.Classes[cl]++
		}
		for id := range r.Valid {
			c.Checkers[id] = true
		}
		if mg := str(r.Manifest, "multigres_commit"); len(mg) >= 7 {
			c.Multigres[mg[:7]] = true
		}
		var x *k7Exposure
		ce := k7ChecksExposure(r.Checks)
		if !r.Stub {
			hx, err := k7HistoryExposure(r.Dir)
			if err != nil {
				p = append(p, fmt.Sprintf("run %s: history: %v", r.ID, err))
				continue
			}
			if ce != nil && !hx.equal(ce) {
				p = append(p, fmt.Sprintf("run %s: checks.json exposure %+v, the history gives %+v (ADR-0044)", r.ID, *ce, *hx))
			}
			x = hx
			c.FullRuns++
		} else if ce != nil {
			x = ce
			c.ChecksRuns++
		}
		if x != nil {
			c.Ops += x.Ops
			c.OKWrites += x.OKWrites
			c.FaultTime += x.FaultTime
			for k, v := range x.Confirmed {
				c.Confirmed[k] += v
			}
		}
		cs, _ := r.Checks["checkers"].([]any)
		for _, y := range cs {
			ch, _ := y.(map[string]any)
			if str(ch, "id") != "C8" {
				continue
			}
			st, _ := ch["stats"].(map[string]any)
			ws, _ := st["storm_windows"].([]any)
			for _, z := range ws {
				w, _ := z.(map[string]any)
				c.G12Windows++
				lt, lo, jt, jo := k7i(w["literal_total"]), k7i(w["literal_ok"]), k7i(w["judged_total"]), k7i(w["judged_ok"])
				c.G12LitTot += lt
				c.G12LitOK += lo
				c.G12JudTot += jt
				c.G12JudOK += jo
				if lo*10 < lt*9 {
					c.G12LitBelow++
				}
				switch str(w, "result") {
				case "fair-share-starvation":
					c.G12JudBelow++
				case "not-judged":
					c.G12NotJudged++
				}
			}
		}
	}
	sort.Strings(keys)
	out := make([]*k7Stat, 0, len(keys))
	for _, k := range keys {
		out = append(out, by[k])
	}
	return out, p
}

// compareReport compares cpgate's cells with a cpreport campaign page's
// data block.
func compareReport(page []byte, cells []*k7Stat, only func(*k7Stat) bool) []string {
	var p []string
	data, err := ReportDataBlock(page)
	if err != nil {
		return []string{err.Error()}
	}
	rcs, _ := data["cells"].([]any)
	byKey := map[string]map[string]any{}
	for _, x := range rcs {
		c, _ := x.(map[string]any)
		byKey[str(c, "campaign")+"/"+str(c, "profile")+"/"+str(c, "mutant")] = c
	}
	for _, c := range cells {
		if only != nil && !only(c) {
			continue
		}
		name := c.Campaign + "/" + c.Profile + "/" + c.Mutant
		rc := byKey[name]
		if rc == nil {
			p = append(p, fmt.Sprintf("%s: not in cpreport's page", name))
			continue
		}
		bad := func(f string, a ...any) { p = append(p, name+": "+fmt.Sprintf(f, a...)) }
		for k, v := range map[string]int64{"n": c.N, "invalid": c.Invalid, "pass": c.Pass, "fail": c.Fail, "unknown": c.Unk} {
			if k7i(rc[k]) != v {
				bad("%s: cpreport %v, cpgate %d", k, rc[k], v)
			}
		}
		rcl, _ := rc["classes"].(map[string]any)
		if len(rcl) != len(c.Classes) {
			bad("classes: cpreport %v, cpgate %v", rcl, c.Classes)
		}
		for cl, k := range c.Classes {
			x, _ := rcl[cl].(map[string]any)
			iv, err := stats.Wilson95(k, c.N)
			if err != nil {
				bad("%v", err)
				continue
			}
			if x == nil || k7i(x["k"]) != k || k7i(x["wilson_lo_e12"]) != int64(iv.Lo) || k7i(x["wilson_hi_e12"]) != int64(iv.Hi) {
				bad("class %s: cpreport %v, cpgate k %d Wilson [%d, %d]e-12", cl, x, k, iv.Lo, iv.Hi)
			}
		}
		if c.N > 0 && c.Fail == 0 {
			zb, _ := stats.ZeroFailureBound95(c.N)
			if k7i(rc["zero_failure_bound_e12"]) != int64(zb) {
				bad("zero-failure bound: cpreport %v, cpgate %d e-12", rc["zero_failure_bound_e12"], zb)
			}
		} else if rc["zero_failure_bound_e12"] != nil {
			bad("cpreport gives a zero-failure bound to a cell with failures")
		}
		ex, _ := rc["exposure"].(map[string]any)
		cf, _ := ex["confirmed_faults"].(map[string]any)
		rconf := map[string]int64{}
		for k, v := range cf {
			rconf[k] = k7i(v)
		}
		if k7i(ex["runs_full"]) != c.FullRuns || k7i(ex["runs_checks_json"]) != c.ChecksRuns || k7i(ex["operations"]) != c.Ops ||
			k7i(ex["ok_writes"]) != c.OKWrites || k7i(ex["fault_time_ns"]) != c.FaultTime || !equalIntMaps(rconf, c.Confirmed) {
			bad("exposure: cpreport %v, cpgate full %d checks %d ops %d ok writes %d fault-time %d faults %v", ex, c.FullRuns, c.ChecksRuns, c.Ops, c.OKWrites, c.FaultTime, c.Confirmed)
		}
		if c.G12Windows > 0 {
			g, _ := rc["g12"].(map[string]any)
			want := map[string]int64{"storm_windows": c.G12Windows, "literal_total": c.G12LitTot, "literal_ok": c.G12LitOK, "literal_below_90": c.G12LitBelow,
				"judged_total": c.G12JudTot, "judged_ok": c.G12JudOK, "judged_below_90": c.G12JudBelow, "not_judged": c.G12NotJudged}
			for k, v := range want {
				if k7i(g[k]) != v {
					bad("G12 %s: cpreport %v, cpgate %d", k, g[k], v)
				}
			}
		}
		delete(byKey, name)
	}
	if only == nil {
		for k := range byKey {
			p = append(p, fmt.Sprintf("%s: in cpreport's page, not among the runs", k))
		}
	}
	return p
}

// k7Render returns the page of the latest successful render record whose
// argv renders <in> into <out>, checking the page's sha256 against the
// record's output.
func k7Render(e *Evidence, prefix, in, out string) ([]byte, []string) {
	var name string
	for _, r := range e.Records {
		if (r.Name == prefix || strings.HasPrefix(r.Name, prefix+"-")) && r.ExitCode == 0 && argvHas(r, "render", in, "--out", out) {
			name = r.Name
		}
	}
	if name == "" {
		return nil, []string{fmt.Sprintf("no successful %s record (cpreport render %s --out %s)", prefix, in, out)}
	}
	_, so, err := e.Get(name)
	if err != nil {
		return nil, []string{err.Error()}
	}
	sum, err := LastJSONLine(so)
	if err != nil {
		return nil, []string{name + ": " + err.Error()}
	}
	outs, _ := sum["outputs"].([]any)
	if len(outs) != 1 {
		return nil, []string{name + ": not one output"}
	}
	o, _ := outs[0].(map[string]any)
	page, err := os.ReadFile(filepath.Join(e.Path(out), str(o, "file")))
	if err != nil {
		return nil, []string{name + ": " + err.Error()}
	}
	if sha(page) != str(o, "sha256") {
		return nil, []string{fmt.Sprintf("%s: the page's sha256 %s differs from the render's %s", name, sha(page), str(o, "sha256"))}
	}
	return page, nil
}

func evalK72(e *Evidence) (string, []string, []string) {
	var p, d []string
	ix := e.k7()
	if ix.Error != nil {
		return "runs", []string{ix.Error.Error()}, nil
	}
	cells, sp := k7Stats(ix)
	p = append(p, sp...)
	page, rp := k7Render(e, "render-k7", "evidence/K7/runs", "evidence/K7/report/k7")
	p = append(p, rp...)
	if page != nil {
		p = append(p, compareReport(page, cells, nil)...)
	}
	full := 0
	for _, c := range cells {
		full += int(c.FullRuns)
		d = append(d, fmt.Sprintf("%s/%s: N %d (PASS %d, FAIL %d, UNKNOWN %d, INVALID %d) classes %v; exposure over %d full + %d checks.json runs: %d ops, %d ok writes, faults %v",
			c.Campaign, c.Profile, c.N, c.Pass, c.Fail, c.Unk, c.Invalid, c.Classes, c.FullRuns, c.ChecksRuns, c.Ops, c.OKWrites, c.Confirmed))
		if c.N > 0 && c.FullRuns+c.ChecksRuns != c.N {
			p = append(p, fmt.Sprintf("%s/%s: exposure over %d of %d valid runs; every K7 run carries it (ADR-0044)", c.Campaign, c.Profile, c.FullRuns+c.ChecksRuns, c.N))
		}
	}
	d = append(d, fmt.Sprintf("%d K7 runs held in full: their checks.json exposure equals cpgate's recomputation from the history", full))
	// The cited cells, from K4's and K5's runs, against cpreport's pages over
	// those directories.
	for _, s := range []string{"K4", "K5"} {
		_, cix := e.k7Cited(s)
		if cix.Error != nil {
			p = append(p, fmt.Sprintf("%s: %v", s, cix.Error))
			continue
		}
		ccells, cp := k7Stats(cix)
		p = append(p, cp...)
		lower := strings.ToLower(s)
		pg, rp := k7Render(e, "render-cited-"+lower, "evidence/"+s+"/runs", "evidence/K7/report/cited-"+lower)
		p = append(p, rp...)
		if pg == nil {
			continue
		}
		isCited := func(c *k7Stat) bool {
			for _, cc := range k7CitedCells {
				if cc.Sprint == s && cc.Campaign == c.Campaign && cc.Profile == c.Profile && cc.Mutant == c.Mutant {
					return true
				}
			}
			return false
		}
		p = append(p, compareReport(pg, ccells, isCited)...)
		for _, c := range ccells {
			if isCited(c) {
				d = append(d, fmt.Sprintf("cited %s %s/%s/%s: N %d FAIL %d classes %v; exposure over %d of %d runs held in full", s, c.Campaign, c.Profile, c.Mutant, c.N, c.Fail, c.Classes, c.FullRuns, c.N))
			}
		}
	}
	// D3′ and D5 on the real build in full (ADR-0045 §5), when committed.
	if _, err := os.Stat(filepath.Join(e.Dir, k7CitedSub)); err == nil {
		cix := e.runIndexIn("k7cited", k7CitedSub, k5ContractOK)
		ccells, cp := k7Stats(cix)
		p = append(p, cp...)
		pg, rp := k7Render(e, "render-cited-full", "evidence/K7/cited", "evidence/K7/report/cited-full")
		p = append(p, rp...)
		if pg != nil {
			p = append(p, compareReport(pg, ccells, nil)...)
		}
		for _, c := range ccells {
			if c.Mutant != "real" || (c.Campaign != "D3p" && c.Campaign != "D5") || c.FullRuns != params.DirectedRuns {
				p = append(p, fmt.Sprintf("cited/: %s/%s/%s with %d runs in full; only D3′ and D5 on the real build, 5 each (ADR-0045)", c.Campaign, c.Profile, c.Mutant, c.FullRuns))
			}
		}
	}
	return "report/k7/index.html", p, d
}

func sabotageK72(dir string) error {
	return forgeReportAt(dir, "report/k7/index.html", "render-k7", func(b []byte) ([]byte, error) {
		re := regexp.MustCompile(`"n":([0-9]+)`)
		loc := re.FindSubmatchIndex(b)
		if loc == nil {
			return nil, fmt.Errorf("no cell n in the data block")
		}
		var n int64
		fmt.Sscan(string(b[loc[2]:loc[3]]), &n)
		out := append([]byte{}, b[:loc[2]]...)
		out = append(out, []byte(fmt.Sprint(n+1))...)
		return append(out, b[loc[3]:]...), nil
	})
}

// forgeReportAt edits a rendered page and rewrites the render record's
// output hash (latest record named prefix or prefix-<n>) to the forged
// page's, so that only a reading of the page's contents can tell. A claim
// whose artifact is the page names the same hash in claims.yaml (two levels
// above dir, K7's release layout); it is rewritten too.
func forgeReportAt(dir, rel, prefix string, edit func([]byte) ([]byte, error)) error {
	p := filepath.Join(dir, rel)
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
	cp := filepath.Join(filepath.Dir(filepath.Dir(filepath.Clean(dir))), "claims.yaml")
	if cb, err := os.ReadFile(cp); err == nil {
		cb = bytes.ReplaceAll(cb, []byte(`sha256: "`+sha(b)+`"`), []byte(`sha256: "`+sha(nb)+`"`))
		if err := os.WriteFile(cp, cb, 0o644); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	e := LoadEvidence(dir)
	name := ""
	for _, r := range e.Records {
		if (r.Name == prefix || strings.HasPrefix(r.Name, prefix+"-")) && r.ExitCode == 0 {
			name = r.Name
		}
	}
	if name == "" {
		return fmt.Errorf("no %s record", prefix)
	}
	return editSummary(dir, name, func(m map[string]any) {
		outs, _ := m["outputs"].([]any)
		for _, o := range outs {
			if om, _ := o.(map[string]any); om != nil {
				om["sha256"] = sha(nb)
			}
		}
	})
}

// k7i reads a JSON integer (0 when absent).
func k7i(v any) int64 {
	switch x := v.(type) {
	case json.Number:
		n, _ := x.Int64()
		return n
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	}
	return 0
}

// k7Repeats re-evaluates ADR-0045's predicates for every FAIL primary of the
// matrix, from its committed checks.json.
func k7Repeats(e *Evidence, ix *k4Index) (map[string]*repeats.Result, []string) {
	out := map[string]*repeats.Result{}
	var p []string
	for _, r := range ix.Runs {
		if r.kind() != "primary" || !r.valid() || r.Verdict != "FAIL" {
			continue
		}
		cb, err := os.ReadFile(filepath.Join(e.Dir, k7RunsSub, r.ID+".checks.json"))
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		res, err := repeats.Classify(cb)
		if err != nil {
			p = append(p, fmt.Sprintf("%s: %v", r.ID, err))
			continue
		}
		out[r.ID] = res
	}
	return out, p
}
