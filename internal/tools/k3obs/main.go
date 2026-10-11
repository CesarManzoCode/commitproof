// Command k3obs summarises the observed reaction of Multigres to each fault
// of sealed K3 runs, for evidence/K3/NOTES.md ("system did"): confirmation
// and heal latencies, the leader and decided rules the pooler status
// observations report around each fault (failover time, rule changes), F2a's
// self-restart, and per-gateway client availability (longest gap between ok
// completions overlapping the fault). It reads observations, so it is
// diagnosis only (ADR-0016): no verdict depends on it.
//
//	go run ./internal/tools/k3obs <run-dir>... > summary.json
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

type ruleKey struct{ term, sub int64 }

func (r ruleKey) String() string { return fmt.Sprintf("%d.%d", r.term, r.sub) }
func (r ruleKey) less(o ruleKey) bool {
	return r.term < o.term || (r.term == o.term && r.sub < o.sub)
}

type obsLeader struct {
	t      int64
	node   string
	leader string
	rule   ruleKey
}

type fault struct {
	ID, Kind string
	Targets  []any
	Phase    map[string]int64
	Evidence map[string]map[string]any
}

func num(v any) int64 {
	switch x := v.(type) {
	case json.Number:
		n, _ := x.Int64()
		return n
	case float64:
		return int64(x)
	}
	return 0
}

func ms(ns int64) int64 { return ns / 1_000_000 }

func summarize(dir string) (map[string]any, error) {
	rep := history.VerifySeal(dir)
	if !rep.Valid {
		return nil, fmt.Errorf("%s: seal does not verify: %v", dir, rep.Problems)
	}
	var start int64
	faults := map[string]*fault{}
	var order []string
	type comp struct {
		t  int64
		ok bool
	}
	byGW := map[string][]comp{}
	for _, ev := range rep.History {
		switch ev.Type {
		case history.TypeMarker:
			if v, _ := ev.Value.(map[string]any); v["name"] == "workload-start" {
				start = ev.T
			}
		case history.TypeNemesis:
			id := fmt.Sprint(ev.Fault["id"])
			f, ok := faults[id]
			if !ok {
				f = &fault{ID: id, Kind: fmt.Sprint(ev.Fault["kind"]), Phase: map[string]int64{}, Evidence: map[string]map[string]any{}}
				f.Targets, _ = ev.Fault["targets"].([]any)
				faults[id] = f
				order = append(order, id)
			}
			ph := fmt.Sprint(ev.Fault["phase"])
			f.Phase[ph] = ev.T
			f.Evidence[ph], _ = ev.Fault["evidence"].(map[string]any)
		case history.TypeOK, history.TypeFail, history.TypeInfo:
			if ev.Client != nil && *ev.Client < 10 {
				byGW[ev.GW] = append(byGW[ev.GW], comp{ev.T, ev.Type == history.TypeOK})
			}
		}
	}
	obs, err := history.ReadChain(filepath.Join(dir, history.FileObservations))
	if err != nil {
		return nil, err
	}
	var leaders []obsLeader
	rules := map[ruleKey]int64{} // first time each decided rule is observed
	for _, o := range obs {
		v, _ := o.Value.(map[string]any)
		if v["src"] != "pooler-status" || v["ok"] != true {
			continue
		}
		d, _ := v["decision"].(map[string]any)
		if d == nil {
			continue
		}
		k := ruleKey{num(d["coordinator_term"]), num(d["leader_subterm"])}
		if _, seen := rules[k]; !seen {
			rules[k] = o.T
		}
		if d["leader"] == v["name"] {
			leaders = append(leaders, obsLeader{o.T, fmt.Sprint(v["node"]), fmt.Sprint(d["leader"]), k})
		}
	}
	var out []any
	for i, id := range order {
		f := faults[id]
		inj := f.Phase["inject"]
		end := rep.History[len(rep.History)-1].T
		if i+1 < len(order) {
			end = faults[order[i+1]].Phase["inject"]
		}
		m := map[string]any{"id": id, "kind": f.Kind, "targets": f.Targets, "inject_s": float64(ms(inj-start)) / 1000}
		for _, ph := range []string{"confirmed", "healed", "not-applied", "not-healed"} {
			if t, ok := f.Phase[ph]; ok {
				m[ph+"_after_inject_ms"] = ms(t - inj)
			}
		}
		if h, ok := f.Phase["heal"]; ok {
			if t, ok := f.Phase["healed"]; ok {
				m["heal_to_healed_ms"] = ms(t - h)
			}
		}
		if res, _ := f.Evidence["inject"]["resolution"].(map[string]any); res != nil {
			m["leader_at_inject"], m["rule_at_inject"] = res["leader"], res["rule"]
		}
		if r, ok := f.Evidence["healed"]["restarted_by_itself"]; ok {
			m["restarted_by_itself"] = r
		}
		if d, _ := f.Evidence["confirmed"]["details"].(map[string]any); d != nil && f.Kind == "F6" {
			m["new_leader"], m["new_rule"] = d["new_leader"], d["new_rule"]
		}
		// Leader before: the last self-reported leader observed before
		// inject; changes: self-reported leaders with a newer rule observed
		// in [inject, next fault).
		var before *obsLeader
		for i := range leaders {
			if leaders[i].t < inj {
				before = &leaders[i]
			}
		}
		if before != nil {
			m["observed_leader_before"] = before.leader + "@" + before.rule.String()
			var first *obsLeader
			for i := range leaders {
				l := leaders[i]
				if l.t >= inj && l.t < end && before.rule.less(l.rule) && (first == nil || l.t < first.t) {
					first = &leaders[i]
				}
			}
			if first != nil {
				m["new_leader_observed"] = first.leader + "@" + first.rule.String()
				m["new_leader_after_inject_ms"] = ms(first.t - inj)
			}
		}
		var newRules []string
		for k, t := range rules {
			if t >= inj && t < end {
				newRules = append(newRules, k.String())
			}
		}
		sort.Strings(newRules)
		m["rules_first_seen_until_next_fault"] = newRules
		// Longest gap between consecutive ok completions per gateway in
		// [inject - 1 s, healed (or confirmed for F5) + 10 s].
		hi := f.Phase["healed"]
		if hi == 0 {
			hi = f.Phase["confirmed"]
		}
		hi += 10_000_000_000
		lo := inj - 1_000_000_000
		gaps := map[string]any{}
		for gw, cs := range byGW {
			var last, worst int64 = lo, 0
			nonOK := 0
			for _, c := range cs {
				if c.t < lo || c.t > hi {
					continue
				}
				if !c.ok {
					nonOK++
					continue
				}
				worst = max(worst, c.t-last)
				last = c.t
			}
			worst = max(worst, hi-last)
			gaps[gw] = map[string]any{"longest_ok_gap_ms": ms(worst), "non_ok_completions": nonOK}
		}
		m["clients"] = gaps
		out = append(out, m)
	}
	man := map[string]any{}
	if b, err := os.ReadFile(filepath.Join(dir, history.FileManifest)); err == nil {
		json.Unmarshal(b, &man)
	}
	var all []string
	for k := range rules {
		all = append(all, k.String())
	}
	sort.Strings(all)
	return map[string]any{"run_id": man["run_id"], "seal_root": rep.MerkleRoot, "decided_rules_observed": all, "faults": out}, nil
}

func main() {
	var res []any
	for _, d := range os.Args[1:] {
		s, err := summarize(d)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		res = append(res, s)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(res)
}
