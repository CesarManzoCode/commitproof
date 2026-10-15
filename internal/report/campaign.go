package report

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/checker/output"
	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/stats"
)

// CampaignRun is one run of a campaign directory, summarized.
type CampaignRun struct {
	ID, Campaign, Profile, Mutant, Seed, Multigres string
	Kind, Of                                       string // primary, candidate, replay; the run it derives from
	Stub                                           bool
	SealRoot, Verdict                              string
	Classes                                        []string // anomaly classes of checkers with valid false
	Checkers                                       []string
	Ops, OKWrites, FaultTime                       int64
	Confirmed                                      map[string]int64
}

// Campaign is a verified campaign directory.
type Campaign struct {
	Runs []*CampaignRun // by id
}

// LoadCampaign verifies every run of dir — subdirectories holding seal.json,
// <id>.tar.zst archives and <id>.stub.tar.zst stubs, each with
// <id>.checks.json — and summarizes it (ADR-0036). Any failure is an error.
func LoadCampaign(dir string) (*Campaign, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	c := &Campaign{}
	seen := map[string]string{}
	for _, e := range ents { // os.ReadDir sorts by name
		name := e.Name()
		var id string
		var stub bool
		switch {
		case e.IsDir():
			if _, err := os.Stat(filepath.Join(dir, name, history.FileSeal)); err != nil {
				continue
			}
			id = name
		case strings.HasSuffix(name, StubSuffix):
			id, stub = strings.TrimSuffix(name, StubSuffix), true
		case strings.HasSuffix(name, history.ArchiveSuffix):
			id = strings.TrimSuffix(name, history.ArchiveSuffix)
		default:
			continue
		}
		if prev, dup := seen[id]; dup {
			return nil, fmt.Errorf("run %s is present twice (%s, %s)", id, prev, name)
		}
		seen[id] = name
		var cr *CampaignRun
		if stub {
			cr, err = loadStub(filepath.Join(dir, name), id, filepath.Join(dir, id+ChecksSuffix))
		} else {
			cr, err = loadFull(filepath.Join(dir, name))
		}
		if err != nil {
			return nil, err
		}
		c.Runs = append(c.Runs, cr)
	}
	if len(c.Runs) == 0 {
		return nil, fmt.Errorf("no run in the directory")
	}
	sort.Slice(c.Runs, func(a, b int) bool { return c.Runs[a].ID < c.Runs[b].ID })
	return c, nil
}

func summarize(id, sealRoot string, man map[string]any, checks *output.Checks) *CampaignRun {
	cr := &CampaignRun{ID: id, Campaign: str(man, "campaign"), Profile: str(man, "profile"), Mutant: str(man, "mutant"),
		Seed: str(man, "seed"), Multigres: str(man, "multigres_commit"), SealRoot: sealRoot, Verdict: checks.Verdict,
		Kind: "primary", Confirmed: map[string]int64{}}
	switch {
	case man["replay_of"] != nil:
		cr.Kind, cr.Of = "replay", str(man, "replay_of")
	case man["candidate_of"] != nil:
		cr.Kind, cr.Of = "candidate", str(man, "candidate_of")
	}
	classes := map[string]bool{}
	for _, ch := range checks.Checkers {
		cr.Checkers = append(cr.Checkers, ch.ID)
		if ch.Valid != output.ValidFalse {
			continue
		}
		for _, a := range ch.Anomalies {
			classes[a.Class] = true
		}
	}
	cr.Classes = sortedKeys(classes)
	sort.Strings(cr.Checkers)
	return cr
}

func loadFull(path string) (*CampaignRun, error) {
	r, err := LoadRun(path)
	if err != nil {
		return nil, err
	}
	cr := summarize(r.ID, r.SealRoot, r.Manifest, r.Checks)
	exposure(cr, r.History)
	return cr, nil
}

func loadStub(path, id, checksPath string) (*CampaignRun, error) {
	tmp, err := os.MkdirTemp("", "cpreport-stub-")
	if err != nil {
		return nil, err
	}
	defer removeTree(tmp)
	if err := history.ExtractArchive(path, tmp); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	dir := filepath.Join(tmp, id)
	rep := history.VerifyStub(dir)
	if !rep.Valid {
		return nil, fmt.Errorf("stub %s: %s", id, strings.Join(rep.Problems, "; "))
	}
	man, err := readJSONObject(filepath.Join(dir, history.FileManifest))
	if err != nil {
		return nil, fmt.Errorf("stub %s: manifest: %w", id, err)
	}
	if got := str(man, "run_id"); got != id {
		return nil, fmt.Errorf("stub %s: manifest run_id %q", id, got)
	}
	cb, err := os.ReadFile(checksPath)
	if err != nil {
		return nil, fmt.Errorf("stub %s: checks file: %w", id, err)
	}
	checks, err := output.Decode(cb)
	if err != nil {
		return nil, fmt.Errorf("stub %s: %w", id, err)
	}
	if checks.RunID != id || checks.SealRoot != rep.MerkleRoot {
		return nil, fmt.Errorf("stub %s: checks file does not name its run id and seal root", id)
	}
	cr := summarize(id, rep.MerkleRoot, man, checks)
	cr.Stub = true
	return cr, nil
}

// exposure counts ADR-0036's §12.3 exposure items: operations (invokes but
// final reads), ok writes (ok ledger inserts, the append micro-ops of ok
// append operations, ok register write and cas, ok bank transfers),
// confirmed faults by kind, and fault-time (Σ heal − confirmed).
func exposure(cr *CampaignRun, evs []history.Event) {
	inv := map[int64]history.Event{}
	confirmed := map[string]int64{}
	kinds := map[string]string{}
	for _, ev := range evs {
		switch ev.Type {
		case history.TypeInvoke:
			inv[ev.I] = ev
			if ev.F != "final-read" {
				cr.Ops++
			}
		case history.TypeOK:
			if ev.Op == nil {
				continue
			}
			in := inv[*ev.Op]
			switch {
			case in.F == "final-read":
			case in.WL == "ledger" && in.F == "insert", in.WL == "register" && (in.F == "write" || in.F == "cas"), in.WL == "bank" && in.F == "transfer":
				cr.OKWrites++
			case in.WL == "append":
				mops, _ := in.Value.([]any)
				for _, x := range mops {
					if mo, _ := x.([]any); len(mo) == 3 && mo[0] == "append" {
						cr.OKWrites++
					}
				}
			}
		case history.TypeNemesis:
			id := str(ev.Fault, "id")
			kinds[id] = str(ev.Fault, "kind")
			switch str(ev.Fault, "phase") {
			case "confirmed":
				confirmed[id] = ev.T
				cr.Confirmed[kinds[id]]++
			case "heal":
				if t, ok := confirmed[id]; ok {
					cr.FaultTime += ev.T - t
				}
			}
		}
	}
}

type cellKey struct{ campaign, profile, mutant string }

// Cell is one matrix cell's §12.3 statistics.
type Cell struct {
	Campaign, Profile, Mutant string
	Runs                      []*CampaignRun // counted and INVALID primaries
	N, Invalid, Pass, Fail    int64
	Unknown                   int64
	Classes                   map[string]int64 // k per anomaly class
	FullRuns                  int64
	Ops, OKWrites, FaultTime  int64
	Confirmed                 map[string]int64
	Checkers                  []string
	Multigres                 []string
}

// Cells groups the primary runs (ADR-0036).
func (c *Campaign) Cells() []*Cell {
	by := map[cellKey]*Cell{}
	var keys []cellKey
	for _, r := range c.Runs {
		if r.Kind != "primary" {
			continue
		}
		k := cellKey{r.Campaign, r.Profile, r.Mutant}
		cl, ok := by[k]
		if !ok {
			cl = &Cell{Campaign: k.campaign, Profile: k.profile, Mutant: k.mutant, Classes: map[string]int64{}, Confirmed: map[string]int64{}}
			by[k] = cl
			keys = append(keys, k)
		}
		cl.Runs = append(cl.Runs, r)
		if r.Verdict == "INVALID" {
			cl.Invalid++
			continue
		}
		cl.N++
		switch r.Verdict {
		case "PASS":
			cl.Pass++
		case "FAIL":
			cl.Fail++
		default:
			cl.Unknown++
		}
		for _, cls := range r.Classes {
			cl.Classes[cls]++
		}
		if !r.Stub {
			cl.FullRuns++
			cl.Ops += r.Ops
			cl.OKWrites += r.OKWrites
			cl.FaultTime += r.FaultTime
			for k, v := range r.Confirmed {
				cl.Confirmed[k] += v
			}
		}
	}
	sort.Slice(keys, func(a, b int) bool {
		x, y := keys[a], keys[b]
		if x.campaign != y.campaign {
			return x.campaign < y.campaign
		}
		if x.profile != y.profile {
			return x.profile < y.profile
		}
		return x.mutant < y.mutant
	})
	out := make([]*Cell, 0, len(keys))
	for _, k := range keys {
		cl := by[k]
		chk, mg := map[string]bool{}, map[string]bool{}
		for _, r := range cl.Runs {
			if r.Verdict == "INVALID" {
				continue
			}
			for _, x := range r.Checkers {
				chk[x] = true
			}
			if len(r.Multigres) >= 7 {
				mg[r.Multigres[:7]] = true
			}
		}
		cl.Checkers, cl.Multigres = sortedKeys(chk), sortedKeys(mg)
		out = append(out, cl)
	}
	return out
}

func kindsText(m map[string]int64) string {
	var parts []string
	for _, k := range sortedKeys(m) {
		parts = append(parts, fmt.Sprintf("%s ×%d", k, m[k]))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

// RenderCampaign renders the campaign summary page.
func RenderCampaign(c *Campaign) ([]byte, error) {
	cells := c.Cells()
	var b bytes.Buffer
	w := &b
	fmt.Fprintf(w, "<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<title>CommitProof campaign summary</title>\n<style>%s</style>\n</head>\n<body>\n", css)
	fmt.Fprintf(w, "<header><h1>Campaign summary</h1>\n<p class=\"small\">%s · %d sealed runs verified (%d primary). Statistics per CONTRACT §12.3 with ADR-0036's cells, exposure and integer arithmetic: Wilson 95 %% intervals (lower end rounded down, upper end up) and, for a cell without failures, the one-sided bound 1 − 0.05^(1/N) (rounded up). Verdicts are cpcheck's (checks.json); INVALID runs are listed and never counted (§12.1); minimization candidates and replays (§12.2) are listed apart.</p></header>\n", esc(Format), len(c.Runs), countKind(c.Runs, "primary"))
	fmt.Fprint(w, "<h2 id=\"cells\">Cells</h2>\n<table><tr><th>campaign</th><th>profile</th><th>build</th><th>N valid</th><th>INVALID</th><th>PASS</th><th>FAIL</th><th>UNKNOWN</th><th>violations k/N [Wilson 95 %]</th><th>bound if k = 0</th><th>operations</th><th>ok writes</th><th>confirmed faults</th><th>fault-time (s)</th><th>exposure over</th></tr>\n")
	data := []any{}
	for _, cl := range cells {
		var vs []string
		classes := map[string]any{}
		for _, cls := range sortedKeys(cl.Classes) {
			k := cl.Classes[cls]
			iv, err := stats.Wilson95(k, cl.N)
			if err != nil {
				return nil, err
			}
			vs = append(vs, fmt.Sprintf("%s %d/%d [%s, %s]", esc(cls), k, cl.N, stats.Down(iv.Lo), stats.Up(iv.Hi)))
			classes[cls] = map[string]any{"k": k, "wilson_lo": stats.Down(iv.Lo), "wilson_hi": stats.Up(iv.Hi),
				"wilson_lo_e12": int64(iv.Lo), "wilson_hi_e12": int64(iv.Hi)}
		}
		bound := "—"
		cd := map[string]any{"campaign": cl.Campaign, "profile": cl.Profile, "mutant": cl.Mutant, "n": cl.N, "invalid": cl.Invalid,
			"pass": cl.Pass, "fail": cl.Fail, "unknown": cl.Unknown, "classes": classes}
		if cl.N > 0 && cl.Fail == 0 {
			zb, err := stats.ZeroFailureBound95(cl.N)
			if err != nil {
				return nil, err
			}
			bound = stats.Up(zb)
			cd["zero_failure_bound"], cd["zero_failure_bound_e12"] = bound, int64(zb)
		}
		if len(vs) == 0 {
			vs = []string{"none"}
		}
		conf := map[string]any{}
		for k, v := range cl.Confirmed {
			conf[k] = v
		}
		cd["exposure"] = map[string]any{"runs_full": cl.FullRuns, "operations": cl.Ops, "ok_writes": cl.OKWrites, "confirmed_faults": conf, "fault_time_ns": cl.FaultTime}
		var ids []any
		for _, r := range cl.Runs {
			ids = append(ids, r.ID)
		}
		cd["runs"] = ids
		data = append(data, cd)
		fmt.Fprintf(w, "<tr id=\"cell-%s-%s-%s\"><td>%s</td><td>%s</td><td>%s</td><td class=\"n\">%d</td><td class=\"n\">%d</td><td class=\"n\">%d</td><td class=\"n\">%d</td><td class=\"n\">%d</td><td>%s</td><td class=\"n\">%s</td><td class=\"n\">%d</td><td class=\"n\">%d</td><td>%s</td><td class=\"n\">%s</td><td class=\"n\">%d of %d runs</td></tr>\n",
			esc(cl.Campaign), esc(cl.Profile), esc(cl.Mutant), esc(cl.Campaign), esc(cl.Profile), esc(cl.Mutant), cl.N, cl.Invalid, cl.Pass, cl.Fail, cl.Unknown,
			strings.Join(vs, "<br>"), bound, cl.Ops, cl.OKWrites, esc(kindsText(cl.Confirmed)), secs(cl.FaultTime), cl.FullRuns, cl.N)
	}
	fmt.Fprint(w, "</table>\n<p class=\"small\">Exposure counts only runs held in full; a stub (manifest, plan, seal) is verified and counted by its checks.json verdict but its history is not in the directory (CONTRACT §13; LIMITATIONS LIM-30, LIM-37). ok writes = ok ledger inserts + appended elements of ok append operations + ok register writes and CAS + ok bank transfers; fault-time = Σ (heal − confirmed) (ADR-0036).</p>\n")
	fmt.Fprint(w, "<h2 id=\"statements\">Clean-result statements (§12.3)</h2>\n<p class=\"small\">For real-build cells whose valid runs all PASS and are held in full; mutant cells are detection experiments (§12.4), not clean results.</p>\n<ul>\n")
	qualifies := false
	for _, cl := range cells {
		if cl.Mutant != "real" || cl.N == 0 || cl.Fail > 0 || cl.Unknown > 0 || cl.FullRuns != cl.N {
			continue
		}
		qualifies = true
		zb, _ := stats.ZeroFailureBound95(cl.N)
		fmt.Fprintf(w, "<li>In %d valid runs of campaign %s (profile %s, Multigres %s), exposing %d ok writes to %d confirmed faults of kinds {%s}, no violation of {%s} was observed; at 95%% confidence the per-run violation probability under this exact schedule distribution is below %s. Not tested: see docs/LIMITATIONS.md.</li>\n",
			cl.N, esc(cl.Campaign), esc(cl.Profile), esc(strings.Join(cl.Multigres, ", ")), cl.OKWrites, sumValues(cl.Confirmed), esc(strings.Join(sortedKeys(cl.Confirmed), ", ")), esc(strings.Join(cl.Checkers, ", ")), stats.Up(zb))
	}
	if !qualifies {
		fmt.Fprint(w, "<li>No cell qualifies (a real-build cell with valid runs only PASS, all held in full).</li>\n")
	}
	fmt.Fprint(w, "</ul>\n<h2 id=\"runs\">Runs per cell</h2>\n")
	for _, cl := range cells {
		fmt.Fprintf(w, "<details><summary>%s · %s · %s — %d runs</summary>\n<table><tr><th>run</th><th>seed</th><th>verdict</th><th>violation classes</th><th>held</th><th>seal root</th></tr>\n", esc(cl.Campaign), esc(cl.Profile), esc(cl.Mutant), len(cl.Runs))
		for _, r := range cl.Runs {
			fmt.Fprintf(w, "<tr><td class=\"mono\">%s</td><td class=\"n\">%s</td><td><span class=\"verdict v-%s\">%s</span></td><td>%s</td><td>%s</td><td class=\"mono\">%s</td></tr>\n",
				esc(r.ID), esc(r.Seed), esc(r.Verdict), esc(r.Verdict), esc(strings.Join(r.Classes, ", ")), held(r), esc(r.SealRoot))
		}
		fmt.Fprint(w, "</table></details>\n")
	}
	fmt.Fprint(w, "<h2 id=\"follow-ups\">Minimization candidates and replays (not counted)</h2>\n")
	fu := []any{}
	fmt.Fprint(w, "<table><tr><th>run</th><th>kind</th><th>of</th><th>verdict</th><th>violation classes</th><th>held</th></tr>\n")
	for _, r := range c.Runs {
		if r.Kind == "primary" {
			continue
		}
		fu = append(fu, map[string]any{"run": r.ID, "kind": r.Kind, "of": r.Of, "verdict": r.Verdict})
		fmt.Fprintf(w, "<tr><td class=\"mono\">%s</td><td>%s</td><td class=\"mono\">%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n", esc(r.ID), esc(r.Kind), esc(r.Of), esc(r.Verdict), esc(strings.Join(r.Classes, ", ")), held(r))
	}
	fmt.Fprint(w, "</table>\n")
	db, err := dataBlock(map[string]any{"format": Format, "runs": int64(len(c.Runs)), "cells": data, "follow_ups": fu})
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(w, "<script type=\"application/json\" id=\"cp-data\">%s</script>\n</body>\n</html>\n", db)
	return b.Bytes(), nil
}

func held(r *CampaignRun) string {
	if r.Stub {
		return "stub"
	}
	return "full"
}

func countKind(rs []*CampaignRun, kind string) int {
	n := 0
	for _, r := range rs {
		if r.Kind == kind {
			n++
		}
	}
	return n
}

func sumValues(m map[string]int64) int64 {
	var s int64
	for _, k := range sortedKeys(m) {
		s += m[k]
	}
	return s
}
