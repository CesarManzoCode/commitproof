// Package checker implements the checkers of CONTRACT §10 (C1–C9) as pure
// functions of a sealed run: the client history and the plan, never the
// observation stream (ADR-0016). It is imported by cpcheck only; it must not
// depend on runner, nemesis, cluster, workload, observation or Multigres
// packages (CONTRACT §5.3, enforced by the independence test), nor on the
// synthetic generator (internal/synth), whose expected outputs must never
// come from this code (AGENTS.md).
//
// Every checker returns valid true, false or unknown, the anomalies that
// make it invalid ({class, ops, explanation}; class names exactly as §10),
// and stats. Anomaly classes a checker finds but the expected model allows
// (C3's -process/-realtime variants and G2-item under repeatable-read, C8's
// notify-window-loss) are observations: they are listed under
// stats.observations with the same {class, ops, explanation} shape and never
// change validity.
package checker

import (
	"fmt"
	"sort"
)

// Validity is the tri-state result of a checker.
type Validity string

// Validity values.
const (
	True    Validity = "true"
	False   Validity = "false"
	Unknown Validity = "unknown"
)

// JSON returns the checks.json encoding: true, false or "unknown".
func (v Validity) JSON() any {
	switch v {
	case True:
		return true
	case False:
		return false
	}
	return string(Unknown)
}

// Anomaly is one finding. Ops are history line indices (i) of the invoke
// events of the operations involved.
type Anomaly struct {
	Class       string
	Ops         []int64
	Explanation string
}

// Result is one checker's output.
type Result struct {
	ID           string
	Valid        Validity
	Anomalies    []Anomaly
	Observations []Anomaly
	Stats        map[string]any
}

// maxPerClass bounds the anomalies listed per class; the total count per
// class is in stats.counts.
const maxPerClass = 20

// collector accumulates anomalies and observations with per-class bounds.
type collector struct {
	anoms, obs []Anomaly
	counts     map[string]int64
	obsCounts  map[string]int64
}

func newCollector() *collector {
	return &collector{counts: map[string]int64{}, obsCounts: map[string]int64{}}
}

func (c *collector) add(class string, ops []int64, f string, a ...any) {
	c.counts[class]++
	if c.counts[class] <= maxPerClass {
		c.anoms = append(c.anoms, Anomaly{Class: class, Ops: sortedOps(ops), Explanation: fmt.Sprintf(f, a...)})
	}
}

func (c *collector) observe(class string, ops []int64, f string, a ...any) {
	c.obsCounts[class]++
	if c.obsCounts[class] <= maxPerClass {
		c.obs = append(c.obs, Anomaly{Class: class, Ops: sortedOps(ops), Explanation: fmt.Sprintf(f, a...)})
	}
}

// result builds a Result. unknown is the reason the checker could not
// decide ("" if it could); a definite anomaly makes the result false even
// when part of the analysis was undecidable (a violation found is found).
func (c *collector) result(id, unknown string, stats map[string]any) Result {
	if stats == nil {
		stats = map[string]any{}
	}
	stats["counts"] = intMap(c.counts)
	if len(c.obsCounts) > 0 {
		stats["observation_counts"] = intMap(c.obsCounts)
	}
	r := Result{ID: id, Anomalies: c.anoms, Observations: c.obs, Stats: stats}
	switch {
	case len(c.anoms) > 0:
		r.Valid = False
	case unknown != "":
		r.Valid = Unknown
	default:
		r.Valid = True
	}
	if unknown != "" {
		stats["unknown"] = unknown
	}
	return r
}

func intMap(m map[string]int64) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func sortedOps(ops []int64) []int64 {
	out := append([]int64{}, ops...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	n := 0
	for i, x := range out {
		if i == 0 || x != out[i-1] {
			out[n] = x
			n++
		}
	}
	return out[:n]
}

// Classes returns the sorted, de-duplicated class names of a list.
func Classes(as []Anomaly) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range as {
		if !seen[a.Class] {
			seen[a.Class] = true
			out = append(out, a.Class)
		}
	}
	sort.Strings(out)
	return out
}

// IDs of the checkers, in checks.json order.
var IDs = []string{"C1", "C2", "C3", "C4", "C5", "C6", "C7", "C8", "C9"}

// RunAll runs C1–C9 on a loaded history. C6 refines C4's per-key results.
func RunAll(h *History) []Result {
	c4, keys := checkLinearizable(h)
	return []Result{
		CheckLostWrites(h),
		CheckAbortedVisible(h),
		CheckListAppend(h),
		c4,
		CheckBank(h),
		checkStaleReads(h, keys),
		CheckCDC(h),
		CheckSession(h),
		CheckAvailability(h),
	}
}
