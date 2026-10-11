package campaign

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/plan"
)

// MinFaultFreeDuration: §12.2 halves the workload while keeping ≥ 10 s.
const minDurationMS = 10_000

// Minimized is the outcome of Minimize.
type Minimized struct {
	Original   *Run
	Class      string
	Run        *Run // the run of the minimal plan (Original if nothing reduced)
	Candidates []map[string]any
}

// journal is <Out>/<original>.minimize.json: an index of the candidates, the
// result, rewritten after each step. It is never an input of a verdict.
func (d *Driver) journal(m *Minimized, done bool) {
	cands := make([]any, len(m.Candidates))
	for i, c := range m.Candidates {
		cands[i] = c
	}
	v := map[string]any{"original": m.Original.ID, "target_class": m.Class, "candidates": cands, "done": done,
		"minimized_run": idOf(m.Run), "algorithm": "CONTRACT §12.2: ddmin over nemesis steps; halve duration (≥ 10 s); clients in the anomaly + 1"}
	b, err := canonjson.Marshal(v)
	if err == nil {
		os.WriteFile(filepath.Join(d.Out, m.Original.ID+".minimize.json"), append(b, '\n'), 0o644)
	}
}

// Minimize applies delta debugging to a FAIL run (CONTRACT §12.2): first
// over nemesis steps (the empty schedule is tried first, then ddmin), then
// halving the workload duration while it stays ≥ 10 s, then reducing the
// clients to those whose operations appear in the target class's anomalies
// plus one. A candidate keeps the failure if its run (one run, valid; an
// INVALID candidate is re-run like any run and counts as not keeping after
// MaxAttempts) is FAIL with the same class. Candidates already run (sealed,
// candidate_of the original) are reused.
func (d *Driver) Minimize(ctx context.Context, orig *Run) (*Minimized, error) {
	if err := d.Check(ctx, orig); err != nil {
		return nil, err
	}
	if orig.Verdict != "FAIL" {
		return nil, fmt.Errorf("%s is %s, not FAIL", orig.ID, orig.Verdict)
	}
	m := &Minimized{Original: orig, Class: TargetClass(orig), Run: orig}
	cur, err := PlanOf(orig)
	if err != nil {
		return nil, err
	}
	profile, mutant := str(orig.Manifest, "profile"), mutantOf(orig)
	test := func(p *plan.Plan, err error) (bool, error) {
		if err != nil {
			return false, err
		}
		s := Spec{Plan: p, Profile: profile, Mutant: mutant, Prov: map[string]any{KeyCandidateOf: orig.ID}}
		r, inv, ok, err := d.Ensure(ctx, s)
		if err != nil {
			return false, err
		}
		sha, _ := p.SHA256()
		keeps := ok && r.Verdict == "FAIL" && r.has(m.Class)
		c := map[string]any{"plan_sha256": sha, "reductions": strAny(p.Reductions), "run": idOf(r), "verdict": verdictOf(r),
			"classes": strAny(classesOf(r)), "valid": ok, "invalid_attempts": ids(inv), "keeps": keeps}
		m.Candidates = append(m.Candidates, c)
		d.emit(map[string]any{"event": "minimize-candidate", "of": orig.ID, "candidate": c})
		d.journal(m, false)
		if keeps {
			m.Run = r
		}
		return keeps, nil
	}

	// 1. Nemesis steps.
	if n := len(cur.Steps); n > 0 {
		base := cur
		keep, err := test(base.KeepSteps(nil))
		if err != nil {
			return m, err
		}
		if keep {
			if cur, err = base.KeepSteps(nil); err != nil {
				return m, err
			}
		} else if n > 1 {
			all := make([]int, n)
			for i := range all {
				all[i] = i
			}
			best, err := ddmin(all, func(sub []int) (bool, error) { return test(base.KeepSteps(sub)) })
			if err != nil {
				return m, err
			}
			if len(best) < n {
				if cur, err = base.KeepSteps(best); err != nil {
					return m, err
				}
			}
		}
	}
	// 2. Workload length.
	for cur.DurationMS/2 >= minDurationMS {
		next, err := cur.HalveDuration()
		keep, terr := test(next, err)
		if terr != nil {
			return m, terr
		}
		if !keep {
			break
		}
		cur = next
	}
	// 3. Clients: those in the target class's anomalies, plus one.
	involved, err := involvedClients(m.Run, m.Class)
	if err != nil {
		return m, err
	}
	var all []int
	for _, c := range cur.Clients {
		all = append(all, c.Client)
	}
	keep := map[int]bool{}
	for _, c := range involved {
		keep[c] = true
	}
	for _, c := range all {
		if !keep[c] {
			keep[c] = true
			break
		}
	}
	var ks []int
	for _, c := range all {
		if keep[c] {
			ks = append(ks, c)
		}
	}
	if len(ks) < len(all) {
		next, err := cur.KeepClients(ks)
		if _, err := test(next, err); err != nil {
			return m, err
		}
	}
	d.journal(m, true)
	return m, nil
}

// ddmin is Zeller's minimizing delta debugging over c (test(c) is known to
// keep the failure): subsets, then complements, at increasing granularity.
// Every candidate costs a run, so it stops at 1-minimality.
func ddmin(c []int, test func([]int) (bool, error)) ([]int, error) {
	n := 2
	for len(c) >= 2 {
		parts := split(c, n)
		reduced := false
		for _, p := range parts {
			ok, err := test(p)
			if err != nil {
				return c, err
			}
			if ok {
				c, n, reduced = p, 2, true
				break
			}
		}
		if !reduced && n > 2 {
			for i := range parts {
				comp := complement(parts, i)
				ok, err := test(comp)
				if err != nil {
					return c, err
				}
				if ok {
					c, n, reduced = comp, max(n-1, 2), true
					break
				}
			}
		}
		if reduced {
			continue
		}
		if n >= len(c) {
			break
		}
		n = min(2*n, len(c))
	}
	return c, nil
}

func split(c []int, n int) [][]int {
	var out [][]int
	size, rem := len(c)/n, len(c)%n
	i := 0
	for k := 0; k < n; k++ {
		s := size
		if k < rem {
			s++
		}
		out = append(out, append([]int{}, c[i:i+s]...))
		i += s
	}
	return out
}

func complement(parts [][]int, skip int) []int {
	var out []int
	for i, p := range parts {
		if i != skip {
			out = append(out, p...)
		}
	}
	return out
}

func classesOf(r *Run) []string {
	if r == nil {
		return nil
	}
	return r.Classes
}

func strAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// involvedClients maps the ops of class's anomalies in r (history line
// indices) to their plan clients (final-read and recovery clients, numbered
// ≥ P02's total, are not plan clients).
func involvedClients(r *Run, class string) ([]int, error) {
	evs, err := history.ReadChain(filepath.Join(r.Dir, history.FileHistory))
	if err != nil {
		return nil, err
	}
	byLine := map[int64]int{}
	for _, ev := range evs {
		if ev.Client != nil {
			byLine[ev.I] = int(*ev.Client)
		}
	}
	set := map[int]bool{}
	for _, a := range r.Anoms {
		if a.Class != class {
			continue
		}
		for _, op := range a.Ops {
			if c, ok := byLine[op]; ok && c < params.ClientsTotal {
				set[c] = true
			}
		}
	}
	var out []int
	for c := range set {
		out = append(out, c)
	}
	sort.Ints(out)
	return out, nil
}
