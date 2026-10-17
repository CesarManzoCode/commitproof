// Command k7results writes evidence/K7/runs-index.json and
// evidence/K7/results.json from what K7's store step committed (CONTRACT
// §13, §15 K7; ADR-0039, ADR-0045): every run of a runs directory — full
// archives (<id>.tar.zst) and summaries (<id>.stub.tar.zst) — with its
// checks.json, its seal root, where it is stored and the archive's sha256;
// the matrix cells of campaigns/k7.json with their valid run, verdict and
// classes copied verbatim from checks.json, and their INVALID attempts with
// the reasons cpcheck gave (from the campaign log's cpcheck events); every
// FAIL primary's ADR-0045 classification (internal/repeats); every new
// failure's minimization journal; with --pkg-replays, K7.4's package
// replays and their INVALID attempts listed in runs-index.json with their
// seal roots. It computes no verdict; cpgate K7 re-derives everything from
// the same files.
//
// usage: k7results --runs <dir> --matrix campaigns/k7.json --log <campaign log, plain or .zst> --out <dir> [--pkg-replays <dir>]
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klauspost/compress/zstd"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/repeats"
)

func readJSON(b []byte) (map[string]any, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var m map[string]any
	return m, d.Decode(&m)
}

func str(m map[string]any, k string) string {
	switch v := m[k].(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	}
	return ""
}

func fileSHA(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func toAny(v any) any {
	b, _ := json.Marshal(v)
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var out any
	d.Decode(&out)
	return out
}

type run struct {
	ID, Stored, Archive, ArchiveSHA, ChecksSHA string
	Man, Plan                                  map[string]any
	PlanSHA, Seal, Verdict, Kind, Of           string
	Classes                                    []string
	Checks                                     []byte
}

func load(dir, id, archive, stored string) (*run, error) {
	tmp, err := os.MkdirTemp("", "k7results-")
	if err != nil {
		return nil, err
	}
	defer func() {
		filepath.Walk(tmp, func(p string, _ os.FileInfo, _ error) error { os.Chmod(p, 0o755); return nil })
		os.RemoveAll(tmp)
	}()
	if err := history.ExtractArchive(filepath.Join(dir, archive), tmp); err != nil {
		return nil, err
	}
	r := &run{ID: id, Stored: stored, Archive: archive, ArchiveSHA: fileSHA(filepath.Join(dir, archive))}
	mb, err := os.ReadFile(filepath.Join(tmp, id, history.FileManifest))
	if err != nil {
		return nil, err
	}
	if r.Man, err = readJSON(mb); err != nil {
		return nil, err
	}
	pb, err := os.ReadFile(filepath.Join(tmp, id, "plan.json"))
	if err != nil {
		return nil, err
	}
	if r.Plan, err = readJSON(pb); err != nil {
		return nil, err
	}
	// plan_sha256 is the sha256 of the canonical plan bytes; plan.json holds
	// them followed by a newline.
	h := sha256.Sum256(bytes.TrimSuffix(pb, []byte("\n")))
	r.PlanSHA = hex.EncodeToString(h[:])
	if r.PlanSHA != str(r.Man, "plan_sha256") {
		return nil, fmt.Errorf("%s: plan.json sha256 %s, manifest %s", id, r.PlanSHA, str(r.Man, "plan_sha256"))
	}
	sb, err := os.ReadFile(filepath.Join(tmp, id, history.FileSeal))
	if err != nil {
		return nil, err
	}
	seal, err := readJSON(sb)
	if err != nil {
		return nil, err
	}
	r.Seal = str(seal, "merkle_root")
	cp := filepath.Join(dir, id+".checks.json")
	if r.Checks, err = os.ReadFile(cp); err != nil {
		return nil, err
	}
	r.ChecksSHA = fileSHA(cp)
	cj, err := readJSON(r.Checks)
	if err != nil {
		return nil, err
	}
	if str(cj, "seal_root") != r.Seal || str(cj, "run_id") != id {
		return nil, fmt.Errorf("%s: checks.json is for %s/%s", id, str(cj, "run_id"), str(cj, "seal_root"))
	}
	r.Verdict = str(cj, "verdict")
	seen := map[string]bool{}
	cs, _ := cj["checkers"].([]any)
	for _, x := range cs {
		c, _ := x.(map[string]any)
		if fmt.Sprint(c["valid"]) != "false" {
			continue
		}
		as, _ := c["anomalies"].([]any)
		for _, y := range as {
			a, _ := y.(map[string]any)
			if cl := str(a, "class"); !seen[cl] {
				seen[cl] = true
				r.Classes = append(r.Classes, cl)
			}
		}
	}
	sort.Strings(r.Classes)
	r.Kind = "primary"
	switch {
	case r.Man["replay_of"] != nil:
		r.Kind, r.Of = "replay", str(r.Man, "replay_of")
	case r.Man["candidate_of"] != nil:
		r.Kind, r.Of = "candidate", str(r.Man, "candidate_of")
	}
	return r, nil
}

// invalidReasons reads cpcheck's summaries from the campaign log.
func invalidReasons(path string) (map[string][]any, error) {
	out := map[string][]any{}
	if path == "" {
		return out, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rd io.Reader = f
	if strings.HasSuffix(path, ".zst") {
		zr, err := zstd.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		rd = zr
	}
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.HasPrefix(line, []byte(`{"event":"cpcheck"`)) {
			continue
		}
		ev, err := readJSON(line)
		if err != nil {
			continue
		}
		sum, err := readJSON([]byte(str(ev, "summary")))
		if err != nil {
			continue
		}
		if rs, _ := sum["invalid_reasons"].([]any); len(rs) > 0 {
			out[str(ev, "run")] = rs
		}
	}
	return out, sc.Err()
}

type entry struct {
	Cell     string   `json:"cell"`
	Campaign string   `json:"campaign"`
	Profile  string   `json:"profile"`
	Seeds    []uint64 `json:"seeds"`
}

func main() {
	runsDir := flag.String("runs", "evidence/K7/runs", "runs directory (archives and stubs)")
	matrix := flag.String("matrix", "campaigns/k7.json", "the K7 matrix")
	logPath := flag.String("log", "", "the campaign log (plain or .zst)")
	out := flag.String("out", "evidence/K7", "output directory")
	pkgDir := flag.String("pkg-replays", "", "K7.4's package replays (summaries), listed in runs-index.json; results.json does not read them")
	flag.Parse()
	fail := func(err error) {
		fmt.Printf("{\"command\":\"k7results\",\"error\":%q,\"exit_code\":2}\n", err.Error())
		os.Exit(2)
	}
	// loadDir loads every run archive and stub of a directory, in run-id order.
	loadDir := func(dir string) []*run {
		ents, err := os.ReadDir(dir)
		if err != nil {
			fail(err)
		}
		var rs []*run
		for _, e := range ents {
			n := e.Name()
			var id, stored string
			switch {
			case strings.HasSuffix(n, ".stub.tar.zst"):
				id, stored = strings.TrimSuffix(n, ".stub.tar.zst"), "summary"
			case strings.HasSuffix(n, ".tar.zst"):
				id, stored = strings.TrimSuffix(n, ".tar.zst"), "full"
			default:
				continue
			}
			r, err := load(dir, id, n, stored)
			if err != nil {
				fail(err)
			}
			rs = append(rs, r)
		}
		sort.Slice(rs, func(i, j int) bool { return rs[i].ID < rs[j].ID })
		return rs
	}
	runs := loadDir(*runsDir)
	byID := map[string]*run{}
	for _, r := range runs {
		byID[r.ID] = r
	}
	reasons, err := invalidReasons(*logPath)
	if err != nil {
		fail(err)
	}
	mb, err := os.ReadFile(*matrix)
	if err != nil {
		fail(err)
	}
	var m struct {
		Entries []entry `json:"entries"`
	}
	if err := json.Unmarshal(mb, &m); err != nil {
		fail(err)
	}
	cellOf := map[string]string{}
	for i := range m.Entries {
		if m.Entries[i].Profile == "" {
			m.Entries[i].Profile = "baseline"
		}
		cellOf[m.Entries[i].Campaign+"/"+m.Entries[i].Profile] = m.Entries[i].Cell
	}
	cell := func(r *run) string {
		for r != nil && r.Kind != "primary" {
			r = byID[r.Of]
		}
		if r == nil {
			return ""
		}
		return cellOf[str(r.Man, "campaign")+"/"+str(r.Man, "profile")]
	}
	var index []any
	for _, r := range runs {
		x := map[string]any{"run_id": r.ID, "kind": r.Kind, "cell": cell(r), "campaign": str(r.Man, "campaign"), "profile": str(r.Man, "profile"),
			"mutant": str(r.Man, "mutant"), "seed": r.Man["seed"], "plan_sha256": r.PlanSHA, "contract_version": str(r.Plan, "contract_version"),
			"commitproof_commit": str(r.Man, "commitproof_commit"), "seal_root": r.Seal, "verdict": r.Verdict, "classes": strAny(r.Classes),
			"stored": r.Stored, "archive": r.Archive, "archive_sha256": r.ArchiveSHA, "checks": r.ID + ".checks.json", "checks_sha256": r.ChecksSHA}
		if r.Of != "" {
			x["of"] = r.Of
		}
		if rs, ok := reasons[r.ID]; ok {
			x["invalid_reasons"] = rs
		}
		index = append(index, x)
	}
	// Cells: per matrix entry and seed, the valid primary and the INVALID
	// attempts of the same plan.
	var cells []any
	var reps, news []any
	for _, e := range m.Entries {
		for _, seed := range e.Seeds {
			var valid *run
			var inv []any
			for _, r := range runs {
				if r.Kind != "primary" || str(r.Man, "campaign") != e.Campaign || str(r.Man, "profile") != e.Profile || str(r.Man, "seed") != fmt.Sprint(seed) || str(r.Man, "mutant") != "real" {
					continue
				}
				if r.Verdict == "INVALID" {
					inv = append(inv, map[string]any{"run": r.ID, "reasons": reasons[r.ID]})
					continue
				}
				if valid != nil {
					fail(fmt.Errorf("%s seed %d has two valid runs: %s, %s", e.Campaign, seed, valid.ID, r.ID))
				}
				valid = r
			}
			c := map[string]any{"cell": e.Cell, "campaign": e.Campaign, "profile": e.Profile, "seed": int64(seed), "invalid_attempts": inv}
			if inv == nil {
				c["invalid_attempts"] = []any{}
			}
			if valid != nil {
				c["run"], c["verdict"], c["classes"], c["seal_root"] = valid.ID, valid.Verdict, strAny(valid.Classes), valid.Seal
				if valid.Verdict == "FAIL" {
					res, err := repeats.Classify(valid.Checks)
					if err != nil {
						fail(err)
					}
					c["repeat"] = res.Repeat
					item := map[string]any{"run": valid.ID, "cell": e.Cell, "campaign": e.Campaign, "profile": e.Profile, "seed": int64(seed),
						"seal_root": valid.Seal, "stored": valid.Stored, "classification": toAny(res)}
					if res.Repeat {
						reps = append(reps, item)
					} else {
						jp := filepath.Join(*runsDir, valid.ID+".minimize.json")
						if jb, err := os.ReadFile(jp); err == nil {
							j, _ := readJSON(jb)
							item["minimization"] = j
							var rr []any
							for _, r := range runs {
								if r.Kind == "replay" && r.Of == str(j, "minimized_run") {
									rr = append(rr, map[string]any{"run": r.ID, "verdict": r.Verdict, "classes": strAny(r.Classes)})
								}
							}
							item["replays"] = rr
						}
						news = append(news, item)
					}
				}
			} else {
				c["run"], c["verdict"] = nil, "MISSING"
			}
			cells = append(cells, c)
		}
	}
	write := func(name string, v any) {
		b, err := canonjson.Marshal(toAny(v))
		if err != nil {
			fail(err)
		}
		if err := os.WriteFile(filepath.Join(*out, name), append(b, '\n'), 0o644); err != nil {
			fail(err)
		}
	}
	if index == nil {
		index = []any{}
	}
	ri := map[string]any{"contract_version": params.ContractVersion, "runs_dir": *runsDir, "matrix": *matrix,
		"storage": "ADR-0039, ADR-0045: full = the original and minimized runs of new findings; summary = stub (manifest, plan, seal) and checks.json, the full archive kept on the machine that ran it",
		"runs":    index}
	// K7.4's replays of each package's minimized run, and their INVALID
	// attempts: the plan is the package's (its own contract version), the
	// manifest the version the replay ran under (ADR-0047).
	if *pkgDir != "" {
		pkg := []any{}
		for _, r := range loadDir(*pkgDir) {
			pkg = append(pkg, map[string]any{"run_id": r.ID, "kind": r.Kind, "of": r.Of, "replay_index": r.Man["replay_index"], "campaign": str(r.Man, "campaign"),
				"profile": str(r.Man, "profile"), "mutant": str(r.Man, "mutant"), "seed": r.Man["seed"], "plan_sha256": r.PlanSHA,
				"contract_version": str(r.Plan, "contract_version"), "manifest_contract_version": str(r.Man, "contract_version"),
				"commitproof_commit": str(r.Man, "commitproof_commit"), "seal_root": r.Seal, "verdict": r.Verdict, "classes": strAny(r.Classes),
				"stored": r.Stored, "archive": r.Archive, "archive_sha256": r.ArchiveSHA, "checks": r.ID + ".checks.json", "checks_sha256": r.ChecksSHA})
		}
		ri["package_replays_dir"], ri["package_replays"] = *pkgDir, pkg
	}
	write("runs-index.json", ri)
	if reps == nil {
		reps = []any{}
	}
	if news == nil {
		news = []any{}
	}
	write("results.json", map[string]any{"contract_version": params.ContractVersion, "matrix": *matrix, "cells": cells, "repeats": reps, "new_failures": news})
	fmt.Printf("{\"command\":\"k7results\",\"runs\":%d,\"cells\":%d,\"repeats\":%d,\"new_failures\":%d,\"exit_code\":0}\n", len(runs), len(cells), len(reps), len(news))
}

func strAny(ss []string) []any {
	out := make([]any, 0, len(ss))
	for _, s := range ss {
		out = append(out, s)
	}
	return out
}
