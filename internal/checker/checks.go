package checker

import (
	"fmt"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/history"
)

// Verdicts (CONTRACT §12.1).
const (
	VerdictPass    = "PASS"
	VerdictFail    = "FAIL"
	VerdictUnknown = "UNKNOWN"
	VerdictInvalid = "INVALID"
)

// InvalidReasons lists why a run with a verified seal is INVALID (§12.1):
// no run-end marker (runner crash), a run-end status other than complete
// (e.g. cluster not HEALTHY at start), a fault not-applied or not-healed,
// final reads not completed (recomputed from the history, P08).
func InvalidReasons(h *History) []string {
	var out []string
	var end map[string]any
	for _, m := range h.Markers {
		if v := obj(m.Value); str(v, "name") == "run-end" {
			end = v
		}
	}
	switch {
	case end == nil:
		out = append(out, "no run-end marker (runner crash)")
	case str(end, "status") != "complete":
		out = append(out, fmt.Sprintf("run-end status %q: %s", str(end, "status"), str(end, "reason")))
	}
	for _, ev := range h.Nemesis {
		if p := str(ev.Fault, "phase"); p == "not-applied" || p == "not-healed" {
			out = append(out, fmt.Sprintf("fault %v %s", ev.Fault["id"], p))
		}
	}
	if _, missing := h.StableFinal(); len(missing) > 0 {
		out = append(out, fmt.Sprintf("final reads not completed: %v", missing))
	}
	return out
}

// Verdict combines the run's validity and the checker results.
func Verdict(invalid []string, rs []Result) string {
	if len(invalid) > 0 {
		return VerdictInvalid
	}
	v := VerdictPass
	for _, r := range rs {
		switch r.Valid {
		case False:
			return VerdictFail
		case Unknown:
			v = VerdictUnknown
		}
	}
	return v
}

// ExitCode maps a verdict to the §5.6 exit code.
func ExitCode(verdict string) int {
	switch verdict {
	case VerdictPass:
		return 0
	case VerdictFail:
		return 1
	}
	return 2
}

func anomaliesJSON(as []Anomaly) []any {
	out := make([]any, 0, len(as))
	for _, a := range as {
		ops := make([]any, len(a.Ops))
		for i, o := range a.Ops {
			ops[i] = o
		}
		out = append(out, map[string]any{"class": a.Class, "ops": ops, "explanation": a.Explanation})
	}
	return out
}

// ChecksJSON encodes checks.json exactly as CONTRACT §15 K2: {run_id,
// seal_root, checkers: [{id, valid, anomalies: [{class, ops, explanation}],
// stats}], verdict}. Observations are under stats.observations.
func ChecksJSON(runID, sealRoot string, rs []Result, verdict string) ([]byte, error) {
	cs := make([]any, 0, len(rs))
	for _, r := range rs {
		stats := map[string]any{}
		for k, v := range r.Stats {
			stats[k] = v
		}
		stats["observations"] = anomaliesJSON(r.Observations)
		cs = append(cs, map[string]any{"id": r.ID, "valid": r.Valid.JSON(), "anomalies": anomaliesJSON(r.Anomalies), "stats": stats})
	}
	return canonjson.Marshal(map[string]any{"run_id": runID, "seal_root": sealRoot, "checkers": cs, "verdict": verdict})
}

// Check runs every checker on a sealed run directory. A run whose seal does
// not verify gets no checker results and verdict INVALID.
func Check(dir string) (*History, []Result, []string, *history.SealReport, error) {
	h, rep, err := Load(dir)
	if err != nil {
		if rep != nil && !rep.Valid {
			return nil, nil, append([]string{"seal does not verify"}, rep.Problems...), rep, nil
		}
		return nil, nil, nil, rep, err
	}
	rs := RunAll(h)
	return h, rs, InvalidReasons(h), rep, nil
}
