// Package repeats decides whether a K7 matrix FAIL repeats a recorded
// finding (ADR-0045, the owner's decision 1 of 2026-10-15). It reads only a
// run's checks.json — what K7 keeps in git for a summarized run — so that
// the campaign driver (which then skips the follow-up of a repeat) and
// cpgate K7 (which re-evaluates every FAIL) decide alike. The predicates
// are ADR-0045 §2's, fixed before any K7 run.
package repeats

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/checker/output"
)

// GuaranteeOf is ADR-0045 §1's table: the guarantee group of an anomaly
// class. A class not in the table is a group of its own ("class:<name>"),
// which never matches.
var GuaranteeOf = map[string]string{
	"incompatible-order":      "G2b",
	"non-linearizable":        "G6",
	"stale-read":              "G6",
	"notify-not-resumed":      "G10",
	"fair-share-starvation":   "G12",
	"overload-failover":       "G12",
	"session-setting-lost":    "G9",
	"prepared-statement-lost": "G9",
	"lost-write":              "G1",
	"final-read-divergence":   "G2a",
	"g1a":                     "G4",
	"bank-sum":                "G13",
	"bank-negative":           "G13",
	"cdc-gap":                 "G11",
	"cdc-phantom":             "G11",
	"cdc-duplicate":           "G11",
	"cdc-order":               "G11",
	"planned-failover-error":  "G8",
	"p1-not-rejected":         "G7",
	"p1-wrong-sqlstate":       "G7",
	"p2-not-rejected":         "G7",
}

// guaranteeOf maps a class of checker id to its group. C3's g1a (an
// aborted read inside list-append) is C3's own class, not C2's: only C2's
// g1a is in the table's G4 row.
func guaranteeOf(checker, class string) string {
	if class == "g1a" && checker != "C2" {
		return "class:" + checker + "/" + class
	}
	if g, ok := GuaranteeOf[class]; ok {
		return g
	}
	return "class:" + checker + "/" + class
}

// Group is one guarantee group of a FAIL run: the classes with a nonzero
// total in stats.counts of the checkers whose valid is false.
type Group struct {
	Guarantee string           `json:"guarantee"`
	Classes   []string         `json:"classes"` // sorted
	Counts    map[string]int64 `json:"counts"`
	// First is the position (checker order, then the class's first listed
	// anomaly, then its name) used to pick a minimization target.
	first int
}

// Match is the outcome of the predicates for one group.
type Match struct {
	Group
	Predicate string         `json:"predicate,omitempty"` // "" when none matched
	Findings  []string       `json:"findings,omitempty"`
	Fields    map[string]any `json:"fields"`
	// Failed lists, per predicate of the group's guarantee, the conditions
	// that did not hold (empty when one matched).
	Failed map[string][]string `json:"failed,omitempty"`
}

// Result is the classification of one run.
type Result struct {
	RunID    string   `json:"run_id"`
	SealRoot string   `json:"seal_root"`
	Verdict  string   `json:"verdict"`
	Faults   []string `json:"confirmed_fault_kinds"` // F, sorted; nil when checks.json has no exposure
	HasF     bool     `json:"has_exposure"`
	Groups   []Match  `json:"groups"`
	// Repeat: the run is a FAIL and every group matched a predicate.
	Repeat bool `json:"repeat"`
	// Target is the minimization target of a new failure: the first class,
	// in checks.json order, of its first unmatched group ("" for a repeat).
	Target string `json:"target,omitempty"`
}

// Predicate is one of ADR-0045 §2's predicates.
type Predicate struct {
	ID        string
	Guarantee string
	Classes   []string // exactly this set
	Findings  []string // the recorded findings it stands for
	cond      func(c *output.Checks, f map[string]bool) (map[string]any, []string)
}

// Predicates are ADR-0045 §2's, in its order.
var Predicates = []Predicate{
	{ID: "R-G2b", Guarantee: "G2b", Classes: []string{"incompatible-order"},
		Findings: []string{"FND-D1-fb8d0f8c", "FND-D1-2e860fd2", "FND-D1-a8eb9857", "FND-C2-2a534585"}, cond: condG2b},
	{ID: "R-G6", Guarantee: "G6", Classes: []string{"non-linearizable"},
		Findings: []string{"FND-C2-6806aeca"}, cond: condG6},
	{ID: "R-G10", Guarantee: "G10", Classes: []string{"notify-not-resumed"},
		Findings: []string{"FND-C9-40b6f05a", "FND-C9-2fe9a9fc", "FND-C9-90da5532"}, cond: condG10},
	{ID: "R-G12", Guarantee: "G12", Classes: []string{"fair-share-starvation"},
		Findings: []string{"FND-C11-17a344c1"}, cond: condG12},
}

// Classify decodes checks.json and classifies the run with F taken from
// C9's stats.exposure.confirmed_faults (ADR-0044; absent → no F).
func Classify(checksJSON []byte) (*Result, error) {
	c, err := output.Decode(checksJSON)
	if err != nil {
		return nil, err
	}
	f, has, err := ConfirmedKinds(c)
	if err != nil {
		return nil, err
	}
	return ClassifyWith(c, f, has), nil
}

// ConfirmedKinds returns F: the kinds with a count ≥ 1 in C9's
// stats.exposure.confirmed_faults, and whether the field exists.
func ConfirmedKinds(c *output.Checks) (map[string]bool, bool, error) {
	for _, ch := range c.Checkers {
		if ch.ID != "C9" {
			continue
		}
		ex, ok := ch.Stats["exposure"].(map[string]any)
		if !ok {
			return nil, false, nil
		}
		cf, ok := ex["confirmed_faults"].(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("checks.json: C9 exposure without confirmed_faults")
		}
		f := map[string]bool{}
		for k, v := range cf {
			n, err := num(v)
			if err != nil {
				return nil, false, fmt.Errorf("checks.json: confirmed_faults[%s]: %w", k, err)
			}
			if n >= 1 {
				f[k] = true
			}
		}
		return f, true, nil
	}
	return nil, false, nil
}

// ClassifyWith classifies with an explicit F (has = false: no F, nothing
// matches). Tests use it for recorded runs whose plans predate ADR-0044.
func ClassifyWith(c *output.Checks, f map[string]bool, has bool) *Result {
	r := &Result{RunID: c.RunID, SealRoot: c.SealRoot, Verdict: c.Verdict, HasF: has}
	if has {
		for k := range f {
			r.Faults = append(r.Faults, k)
		}
		sort.Strings(r.Faults)
	}
	if c.Verdict != "FAIL" {
		return r
	}
	groups := Groups(c)
	r.Repeat = len(groups) > 0
	for _, g := range groups {
		m := Match{Group: g, Fields: map[string]any{}}
		for _, p := range Predicates {
			if p.Guarantee != g.Guarantee {
				continue
			}
			var failed []string
			if strings.Join(p.Classes, ",") != strings.Join(g.Classes, ",") {
				failed = append(failed, fmt.Sprintf("classes %v, the predicate's %v", g.Classes, p.Classes))
			}
			fields, cf := p.cond(c, f)
			if !has {
				cf = append(cf, "no confirmed fault kinds in checks.json (C9 exposure, contract ≥ 1.7.0)")
			}
			failed = append(failed, cf...)
			if len(failed) == 0 {
				m.Predicate, m.Findings, m.Fields, m.Failed = p.ID, p.Findings, fields, nil
				break
			}
			if m.Failed == nil {
				m.Failed = map[string][]string{}
			}
			m.Failed[p.ID] = failed
			for k, v := range fields {
				m.Fields[k] = v
			}
		}
		if m.Predicate == "" {
			r.Repeat = false
			if r.Target == "" {
				r.Target = targetOf(c, g)
			}
		}
		r.Groups = append(r.Groups, m)
	}
	return r
}

// Groups returns the guarantee groups of a run in checks.json order.
func Groups(c *output.Checks) []Group {
	by := map[string]*Group{}
	var order []string
	pos := 0
	for _, ch := range c.Checkers {
		if ch.Valid != output.ValidFalse {
			continue
		}
		counts := countsOf(ch)
		// Classes in the order of their first listed anomaly, then the
		// count-only ones by name.
		var classes []string
		seen := map[string]bool{}
		for _, a := range ch.Anomalies {
			if !seen[a.Class] && counts[a.Class] > 0 {
				seen[a.Class] = true
				classes = append(classes, a.Class)
			}
		}
		var rest []string
		for k, n := range counts {
			if n > 0 && !seen[k] {
				rest = append(rest, k)
			}
		}
		sort.Strings(rest)
		classes = append(classes, rest...)
		for _, cl := range classes {
			gid := guaranteeOf(ch.ID, cl)
			g := by[gid]
			if g == nil {
				g = &Group{Guarantee: gid, Counts: map[string]int64{}, first: pos}
				by[gid] = g
				order = append(order, gid)
			}
			if _, dup := g.Counts[cl]; !dup {
				g.Classes = append(g.Classes, cl)
			}
			g.Counts[cl] += counts[cl]
			pos++
		}
	}
	out := make([]Group, 0, len(order))
	for _, gid := range order {
		g := by[gid]
		sort.Strings(g.Classes)
		out = append(out, *g)
	}
	return out
}

// targetOf is the first class of g in checks.json order.
func targetOf(c *output.Checks, g Group) string {
	for _, ch := range c.Checkers {
		if ch.Valid != output.ValidFalse {
			continue
		}
		counts := countsOf(ch)
		for _, a := range ch.Anomalies {
			if counts[a.Class] > 0 && guaranteeOf(ch.ID, a.Class) == g.Guarantee {
				return a.Class
			}
		}
		var rest []string
		for k, n := range counts {
			if n > 0 && guaranteeOf(ch.ID, k) == g.Guarantee {
				rest = append(rest, k)
			}
		}
		if len(rest) > 0 {
			sort.Strings(rest)
			return rest[0]
		}
	}
	return ""
}

func countsOf(ch output.Checker) map[string]int64 {
	out := map[string]int64{}
	cm, _ := ch.Stats["counts"].(map[string]any)
	for k, v := range cm {
		if n, err := num(v); err == nil {
			out[k] = n
		}
	}
	// A listed anomaly whose class has no total still counts (fail closed:
	// it is a class of the run).
	for _, a := range ch.Anomalies {
		if out[a.Class] == 0 {
			out[a.Class] = 1
		}
	}
	return out
}

func checker(c *output.Checks, id string) *output.Checker {
	for i := range c.Checkers {
		if c.Checkers[i].ID == id {
			return &c.Checkers[i]
		}
	}
	return nil
}

func validOf(c *output.Checks, id string) string {
	if ch := checker(c, id); ch != nil {
		return ch.Valid
	}
	return "absent"
}

// timeouts is C9's stats.errors["info/timeout"] (0 if absent).
func timeouts(c *output.Checks) int64 {
	ch := checker(c, "C9")
	if ch == nil {
		return 0
	}
	em, _ := ch.Stats["errors"].(map[string]any)
	n, _ := num(em["info/timeout"])
	return n
}

func num(v any) (int64, error) {
	switch x := v.(type) {
	case json.Number:
		return x.Int64()
	case int64:
		return x, nil
	case float64:
		return int64(x), nil
	case nil:
		return 0, nil
	}
	return 0, fmt.Errorf("not a number: %v", v)
}

func kinds(f map[string]bool) []string {
	var out []string
	for k := range f {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// common checks conditions 3–5 of R-G2b and R-G6 (C1 and C2 valid,
// timeouts ≥ 1, F1c ∈ F).
func common(c *output.Checks, f map[string]bool, fields map[string]any) []string {
	var failed []string
	fields["valid_C1"], fields["valid_C2"] = validOf(c, "C1"), validOf(c, "C2")
	if validOf(c, "C1") != output.ValidTrue || validOf(c, "C2") != output.ValidTrue {
		failed = append(failed, fmt.Sprintf("valid(C1) %s, valid(C2) %s; both must be true", validOf(c, "C1"), validOf(c, "C2")))
	}
	t := timeouts(c)
	fields["timeouts"] = t
	if t < 1 {
		failed = append(failed, "no info/timeout completion (C9 stats.errors)")
	}
	fields["confirmed_fault_kinds"] = kinds(f)
	if !f["F1c"] {
		failed = append(failed, "F1c not among the confirmed fault kinds")
	}
	return failed
}

var incompatibleRE = regexp.MustCompile(`^reads of key (\d+) are not prefixes of each other: \[([0-9 ]*)\] \(i=(\d+)\) and \[([0-9 ]*)\] \(i=(\d+)\)$`)

// condG2b: R-G2b conditions 2–5.
func condG2b(c *output.Checks, f map[string]bool) (map[string]any, []string) {
	fields := map[string]any{}
	var failed []string
	ch := checker(c, "C3")
	var vanished []any
	n := 0
	if ch != nil {
		for _, a := range ch.Anomalies {
			if a.Class != "incompatible-order" {
				continue
			}
			n++
			m := incompatibleRE.FindStringSubmatch(a.Explanation)
			if m == nil {
				failed = append(failed, fmt.Sprintf("an incompatible-order explanation is not in C3's form: %q", a.Explanation))
				continue
			}
			x, _ := strconv.ParseInt(m[3], 10, 64)
			y, _ := strconv.ParseInt(m[5], 10, 64)
			later := map[string]bool{}
			for _, e := range strings.Fields(m[4]) {
				later[e] = true
			}
			var gone []any
			for _, e := range strings.Fields(m[2]) {
				if !later[e] {
					v, _ := strconv.ParseInt(e, 10, 64)
					gone = append(gone, v)
				}
			}
			key, _ := strconv.ParseInt(m[1], 10, 64)
			vanished = append(vanished, map[string]any{"key": key, "read_i": x, "longest_read_i": y, "absent_from_longest": gone})
			if x >= y {
				failed = append(failed, fmt.Sprintf("key %d: the read (i=%d) is not earlier than the longest read (i=%d)", key, x, y))
			}
			if len(gone) == 0 {
				failed = append(failed, fmt.Sprintf("key %d: no element of the read (i=%d) is absent from the longest read (a reordering, not a vanish)", key, x))
			}
		}
	}
	if n == 0 {
		failed = append(failed, "no listed incompatible-order anomaly")
	}
	fields["incompatible_order"] = vanished
	return fields, append(failed, common(c, f, fields)...)
}

// condG6: R-G6 conditions 2–5.
func condG6(c *output.Checks, f map[string]bool) (map[string]any, []string) {
	fields := map[string]any{"valid_C6": validOf(c, "C6")}
	var failed []string
	if validOf(c, "C6") != output.ValidTrue {
		failed = append(failed, fmt.Sprintf("valid(C6) %s; must be true (no stale read)", validOf(c, "C6")))
	}
	return fields, append(failed, common(c, f, fields)...)
}

var notifyRE = regexp.MustCompile(`by listener client (\d+), whose session \(LISTEN i=(\d+)\) was alive and past its first successful op after recovery$`)

// condG10: R-G10 conditions 2–3.
func condG10(c *output.Checks, f map[string]bool) (map[string]any, []string) {
	fields := map[string]any{}
	var failed []string
	listens := map[string]map[string]bool{}
	n := 0
	if ch := checker(c, "C8"); ch != nil {
		for _, a := range ch.Anomalies {
			if a.Class != "notify-not-resumed" {
				continue
			}
			n++
			m := notifyRE.FindStringSubmatch(a.Explanation)
			if m == nil {
				failed = append(failed, fmt.Sprintf("a notify-not-resumed explanation is not in C8's form: %q", a.Explanation))
				continue
			}
			if listens[m[1]] == nil {
				listens[m[1]] = map[string]bool{}
			}
			listens[m[1]][m[2]] = true
		}
	}
	if n == 0 {
		failed = append(failed, "no listed notify-not-resumed anomaly")
	}
	sessions := map[string]any{}
	for cl, ls := range listens {
		var idx []string
		for l := range ls {
			idx = append(idx, l)
		}
		sort.Strings(idx)
		sessions["client "+cl] = strings.Join(idx, ",")
		if len(idx) != 1 {
			failed = append(failed, fmt.Sprintf("listener client %s appears with %d LISTEN indexes (%s): it re-established its session", cl, len(idx), strings.Join(idx, ",")))
		}
	}
	fields["listener_sessions"] = sessions
	fields["confirmed_fault_kinds"] = kinds(f)
	if !f["F2a"] && !f["F2b"] && !f["F6"] {
		failed = append(failed, "none of F2a, F2b, F6 among the confirmed fault kinds")
	}
	return fields, failed
}

// condG12: R-G12 conditions 2–3.
func condG12(c *output.Checks, f map[string]bool) (map[string]any, []string) {
	fields := map[string]any{}
	var failed []string
	var windows []any
	n := 0
	if ch := checker(c, "C8"); ch != nil {
		ws, _ := ch.Stats["storm_windows"].([]any)
		for _, x := range ws {
			w, _ := x.(map[string]any)
			if fmt.Sprint(w["result"]) != "fair-share-starvation" {
				continue
			}
			n++
			jt, _ := num(w["judged_total"])
			jo, _ := num(w["judged_ok"])
			lt, _ := num(w["literal_total"])
			lo, _ := num(w["literal_ok"])
			windows = append(windows, map[string]any{"phase": w["phase"], "judged_ok": jo, "judged_total": jt, "literal_ok": lo, "literal_total": lt})
			if !(jt > 0 && jo*10 >= jt*9) {
				failed = append(failed, fmt.Sprintf("storm phase %v: %d of %d operations outside fault windows ok (< 90 %%)", w["phase"], jo, jt))
			}
			if !(lt > 0 && lo*10 < lt*9) {
				failed = append(failed, fmt.Sprintf("storm phase %v: literal fraction %d of %d is not below 90 %%", w["phase"], lo, lt))
			}
		}
	}
	if n == 0 {
		failed = append(failed, "no storm window with result fair-share-starvation in C8's stats (per-window fractions exist from contract 1.2.0)")
	}
	fields["starved_windows"] = windows
	fields["confirmed_fault_kinds"] = kinds(f)
	if !f["F2a"] {
		failed = append(failed, "F2a not among the confirmed fault kinds")
	}
	return fields, failed
}
