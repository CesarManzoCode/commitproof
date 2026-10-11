package gate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

// Independent re-evaluation of cpcheck corpus outputs (CONTRACT §15 K2,
// ADR-0017): cpgate never trusts cpcheck's own "match" field; it compares
// the reported classes with the committed expected.json itself.

// K2Reported is one checker's reported result in results.jsonl.
type K2Reported struct {
	Valid        string   `json:"valid"`
	Anomalies    []string `json:"anomalies"`
	Observations []string `json:"observations"`
}

// K2Elle is the oracle comparison recorded for one W1 entry.
type K2Elle struct {
	Model      string   `json:"model"`
	ElleValid  string   `json:"elle_valid"`
	C3Valid    string   `json:"c3_valid"`
	C3Classes  []string `json:"c3_classes"`
	ElleClass  []string `json:"elle_classes"`
	AgreeField any      `json:"agree"`
}

// K2Result is one results.jsonl line.
type K2Result struct {
	ID             string                `json:"id"`
	Label          string                `json:"label"`
	Family         string                `json:"family"`
	SealRoot       string                `json:"seal_root"`
	Verdict        string                `json:"verdict"`
	ExpectedSHA256 string                `json:"expected_sha256"`
	Reported       map[string]K2Reported `json:"reported"`
	Elle           *K2Elle               `json:"elle"`
}

// K2Expected is expected.json.
type K2Expected struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Family    string `json:"family"`
	Workloads []string
	Iso       string `json:"iso"`
	Elle      bool   `json:"elle"`
	Verdict   string `json:"verdict"`
	Checkers  map[string]struct {
		Valid        any      `json:"valid"`
		Anomalies    []string `json:"anomalies"`
		Observations []string `json:"observations"`
	} `json:"checkers"`
}

var k2Checkers = []string{"C1", "C2", "C3", "C4", "C5", "C6", "C7", "C8", "C9"}

// LoadK2Results reads a results.jsonl.
func LoadK2Results(path string) ([]K2Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []K2Result
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for n := 1; sc.Scan(); n++ {
		var r K2Result
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", filepath.Base(path), n, err)
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

// LoadK2Expected reads corpus/<id>/expected.json and checks its sha256
// against the one cpcheck recorded.
func LoadK2Expected(corpus string, r K2Result) (*K2Expected, error) {
	b, err := os.ReadFile(filepath.Join(corpus, r.ID, "expected.json"))
	if err != nil {
		return nil, err
	}
	if r.ExpectedSHA256 != "" && sha(b) != r.ExpectedSHA256 {
		return nil, fmt.Errorf("%s: expected.json sha256 %s != recorded %s", r.ID, sha(b), r.ExpectedSHA256)
	}
	var e K2Expected
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, fmt.Errorf("%s/expected.json: %w", r.ID, err)
	}
	var w struct {
		Workloads []string `json:"workloads"`
	}
	json.Unmarshal(b, &w)
	e.Workloads = w.Workloads
	return &e, nil
}

func validStr(v any) string {
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

func setOf(xs []string) string {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	out := make([]string, 0, len(m))
	for x := range m {
		out = append(out, x)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// K2Diff compares one reported result with its expectation: validity,
// anomaly-class set and observation-class set of every checker (set
// equality), and the verdict.
func K2Diff(r K2Result, e *K2Expected) []string {
	var d []string
	for _, c := range k2Checkers {
		x, ok := e.Checkers[c]
		if !ok {
			d = append(d, c+": no expectation")
			continue
		}
		g, ok := r.Reported[c]
		if !ok {
			d = append(d, c+": not reported")
			continue
		}
		if g.Valid != validStr(x.Valid) {
			d = append(d, fmt.Sprintf("%s valid %s, expected %s", c, g.Valid, validStr(x.Valid)))
		}
		if setOf(g.Anomalies) != setOf(x.Anomalies) {
			d = append(d, fmt.Sprintf("%s anomalies {%s}, expected {%s}", c, setOf(g.Anomalies), setOf(x.Anomalies)))
		}
		if setOf(g.Observations) != setOf(x.Observations) {
			d = append(d, fmt.Sprintf("%s observations {%s}, expected {%s}", c, setOf(g.Observations), setOf(x.Observations)))
		}
	}
	if r.Verdict != e.Verdict {
		d = append(d, fmt.Sprintf("verdict %s, expected %s", r.Verdict, e.Verdict))
	}
	return d
}

// CorpusIDs lists the entries of a corpus directory.
func CorpusIDs(corpus string) ([]string, error) {
	des, err := os.ReadDir(corpus)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, de := range des {
		if _, err := os.Stat(filepath.Join(corpus, de.Name(), "expected.json")); de.IsDir() && err == nil {
			ids = append(ids, de.Name())
		}
	}
	return ids, nil
}

// K2Failing returns the labels of entries whose reported results differ
// from their expectations, and problems reading the inputs.
func K2Failing(corpus, results string) (map[string]bool, []string) {
	rs, err := LoadK2Results(results)
	if err != nil {
		return nil, []string{err.Error()}
	}
	ids, err := CorpusIDs(corpus)
	if err != nil {
		return nil, []string{err.Error()}
	}
	var p []string
	if len(rs) != len(ids) {
		p = append(p, fmt.Sprintf("%s has %d entries, the corpus %d", results, len(rs), len(ids)))
	}
	failing := map[string]bool{}
	for _, r := range rs {
		e, err := LoadK2Expected(corpus, r)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		if len(K2Diff(r, e)) > 0 {
			failing[r.Label] = true
		}
	}
	return failing, p
}

// K2CheckSabotage compares a sabotage run's failing labels with
// K2SabotageDeps[rule].
func K2CheckSabotage(rule, corpus, results string) (string, []string) {
	failing, p := K2Failing(corpus, results)
	want := map[string]bool{}
	for _, l := range K2SabotageDeps[rule] {
		want[l] = true
	}
	var got []string
	for l := range failing {
		got = append(got, l)
	}
	sort.Strings(got)
	if setOf(got) != setOf(K2SabotageDeps[rule]) {
		p = append(p, fmt.Sprintf("sabotage %s fails labels {%s}, expected exactly {%s}", rule, setOf(got), setOf(K2SabotageDeps[rule])))
	}
	return fmt.Sprintf("%s: failing labels {%s}", rule, setOf(got)), p
}

// ElleMap converts an Elle 0.2.7 anomaly name to the CONTRACT §10.3 name
// (independent copy of the oracle's mapping): G-single-item → G-single,
// G-nonadjacent-item → G-nonadjacent, G1a/G1b → g1a/g1b, -process and
// -realtime suffixes kept; empty-transaction-graph is informational ("");
// anything else is unmapped ("elle:<name>", always a disagreement).
func ElleMap(name string) string {
	if name == "empty-transaction-graph" {
		return ""
	}
	suffix := ""
	for _, s := range []string{"-process", "-realtime"} {
		if strings.HasSuffix(name, s) {
			suffix, name = s, strings.TrimSuffix(name, s)
		}
	}
	base := map[string]string{"G0": "G0", "G1c": "G1c", "G-single-item": "G-single", "G-nonadjacent-item": "G-nonadjacent", "G2-item": "G2-item"}
	if v, ok := base[name]; ok {
		return v + suffix
	}
	plain := map[string]string{"G1a": "g1a", "G1b": "g1b", "duplicate-elements": "duplicate-elements",
		"incompatible-order": "incompatible-order", "internal": "internal", "dirty-update": "dirty-update"}
	if v, ok := plain[name]; ok && suffix == "" {
		return v
	}
	return "elle:" + name + suffix
}

// ElleRaw is the part of an elle-cli analysis the comparison reads.
type ElleRaw struct {
	Valid        any      `json:"valid?"`
	AnomalyTypes []string `json:"anomaly-types"`
}

// ElleVerdict recomputes Elle's validity and mapped classes from the three
// raw analyses of one history (validity run; process and realtime class
// runs). Any run returning unknown makes the result unknown.
func ElleVerdict(dir, id string) (string, []string, error) {
	valid := ""
	classes := map[string]bool{}
	for _, kind := range []string{"validity", "process", "realtime"} {
		b, err := os.ReadFile(filepath.Join(dir, id+"."+kind+".json"))
		if err != nil {
			return "", nil, err
		}
		var a ElleRaw
		if err := json.Unmarshal(b, &a); err != nil {
			return "", nil, fmt.Errorf("%s.%s.json: %w", id, kind, err)
		}
		v := "unknown"
		if x, ok := a.Valid.(bool); ok {
			v = validStr(x)
		}
		if v == "unknown" {
			valid = "unknown"
		}
		if kind == "validity" && valid == "" {
			valid = v
		}
		if kind != "validity" {
			for _, t := range a.AnomalyTypes {
				if m := ElleMap(t); m != "" {
					classes[m] = true
				}
			}
		}
	}
	var cs []string
	for c := range classes {
		cs = append(cs, c)
	}
	sort.Strings(cs)
	return valid, cs, nil
}

// Divergence is one registered disagreement between C3 and elle-cli
// (evidence/K2/divergences.json).
type Divergence struct {
	ID             string `json:"id"`
	MinimalHistory string `json:"minimal_history"`
	Definition     string `json:"definition"`
	WrongTool      string `json:"wrong_tool"`
}

// LoadDivergences reads the registry (a missing file is an empty registry).
func LoadDivergences(path string) (map[string]Divergence, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]Divergence{}, nil
	}
	if err != nil {
		return nil, err
	}
	var ds []Divergence
	if err := json.Unmarshal(b, &ds); err != nil {
		return nil, fmt.Errorf("divergences.json: %w", err)
	}
	out := map[string]Divergence{}
	for _, d := range ds {
		out[d.ID] = d
	}
	return out, nil
}

// K2ElleStats is the K2.4 recomputation.
type K2ElleStats struct {
	Compared, Clean, Planted, Unknown int
	PerModel                          map[string]int
	Disagreements                     []string
}

// K2ElleCompare recomputes the oracle comparison of every entry marked elle
// from the raw analyses and the reported C3 results; every disagreement must
// be registered with all fields and wrong_tool "elle-cli".
func K2ElleCompare(corpus, results, rawDir, divPath string) (K2ElleStats, []string) {
	st := K2ElleStats{PerModel: map[string]int{}}
	rs, err := LoadK2Results(results)
	if err != nil {
		return st, []string{err.Error()}
	}
	divs, err := LoadDivergences(divPath)
	if err != nil {
		return st, []string{err.Error()}
	}
	var p []string
	for _, r := range rs {
		e, err := LoadK2Expected(corpus, r)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		if !e.Elle {
			continue
		}
		ev, ec, err := ElleVerdict(rawDir, r.ID)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		if ev == "unknown" {
			st.Unknown++
			continue
		}
		c3 := r.Reported["C3"]
		st.Compared++
		model := e.Iso
		if e.Family == "mixed" || model == "" {
			model = "serializable"
		}
		st.PerModel[model]++
		if r.Label == "clean" {
			st.Clean++
		} else {
			st.Planted++
		}
		if c3.Valid == ev && setOf(append(append([]string{}, c3.Anomalies...), c3.Observations...)) == setOf(ec) {
			continue
		}
		st.Disagreements = append(st.Disagreements, r.ID)
		d, ok := divs[r.ID]
		switch {
		case !ok:
			p = append(p, fmt.Sprintf("%s: C3 %s {%s} vs elle-cli %s {%s}, not in divergences.json", r.ID, c3.Valid,
				setOf(append(append([]string{}, c3.Anomalies...), c3.Observations...)), ev, setOf(ec)))
		case d.MinimalHistory == "" || d.Definition == "":
			p = append(p, r.ID+": divergence record lacks minimal_history or definition")
		case d.WrongTool != "elle-cli":
			p = append(p, fmt.Sprintf("%s: divergence where %s is wrong must be fixed, not registered", r.ID, d.WrongTool))
		}
	}
	return st, p
}

// verifyCorpusSeals checks that every reported seal root is the seal of the
// committed corpus run (the evidence refers to these sealed runs).
func verifyCorpusSeals(corpus string, rs []K2Result) []string {
	var p []string
	for _, r := range rs {
		rep := history.VerifySeal(filepath.Join(corpus, r.ID, "run"))
		if !rep.Valid {
			p = append(p, fmt.Sprintf("%s: corpus run seal does not verify: %v", r.ID, rep.Problems))
		} else if rep.MerkleRoot != r.SealRoot {
			p = append(p, fmt.Sprintf("%s: reported seal root %s != corpus %s", r.ID, r.SealRoot, rep.MerkleRoot))
		}
	}
	return p
}
