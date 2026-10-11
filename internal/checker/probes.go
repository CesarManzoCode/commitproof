package checker

import (
	"sort"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

// ProbeID is the probe checker (CONTRACT §3 G7 "probes P1, P2", §15 K4;
// ADR-0024). It judges the claims probe suite from the history alone:
//
//   - P1 `SET synchronous_commit = off` must be rejected with SQLSTATE
//     0A000 (S11). Completed ok → p1-not-rejected; rejected with another
//     SQLSTATE → p1-wrong-sqlstate.
//   - P2 `ALTER SYSTEM SET work_mem = '8MB'` must be rejected (S11:
//     ALTER SYSTEM is blocked wholesale): any error carrying a SQLSTATE is a
//     rejection. Completed ok → p2-not-rejected.
//
// A probe whose outcome is unknown (an info completion without a SQLSTATE:
// timeout or transport failure) makes the checker unknown, never valid. A
// run without probe operations is vacuously valid. P1-show (the setting
// read back after P1) is diagnosis only.
const ProbeID = "P"

// probeExpect: SQLSTATE a probe's rejection must carry ("*" = any).
var probeExpect = map[string]string{"P1": "0A000", "P2": "*"}

func probeIDOf(o *Op) string {
	m, _ := o.InValue.(map[string]any)
	id, _ := m["id"].(string)
	return id
}

// CheckProbes is the probe checker.
func CheckProbes(h *History) Result {
	c := newCollector()
	unknown := ""
	seen := map[string]int64{}
	for _, o := range h.Ops {
		if o.WL != "probe" || o.F != "probe" {
			continue
		}
		id := probeIDOf(o)
		want, judged := probeExpect[id]
		if !judged {
			continue
		}
		seen[id]++
		state := o.SQLState()
		switch {
		case o.Type == history.TypeOK:
			if id == "P1" {
				c.add("p1-not-rejected", []int64{o.Index}, "P1 through %s: SET synchronous_commit = off completed ok; G7 requires rejection with 0A000", o.GW)
			} else {
				c.add("p2-not-rejected", []int64{o.Index}, "P2 through %s: ALTER SYSTEM completed ok; G7 requires rejection", o.GW)
			}
		case state == "":
			unknown = "a probe's outcome is unknown (no SQLSTATE: timeout or transport failure)"
		case want != "*" && state != want:
			c.add("p1-wrong-sqlstate", []int64{o.Index}, "P1 through %s rejected with %s, G7 requires %s", o.GW, state, want)
		}
	}
	// The probe suite's plan lists its probes: each judged probe must have
	// run, else the outcome is unknown.
	if camp, _ := h.Plan["campaign"].(string); camp == "claims" {
		for _, id := range []string{"P1", "P2"} {
			if seen[id] == 0 && unknown == "" {
				unknown = "probe " + id + " did not run"
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	counts := map[string]any{}
	for _, id := range ids {
		counts[id] = seen[id]
	}
	return c.result(ProbeID, unknown, map[string]any{"probes": counts})
}
