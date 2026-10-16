// Command k7triage drafts the mechanical part of a K7 triage record for a
// real-build FAIL of K7's matrix (CONTRACT §15 K7; ADR-0042, ADR-0045): the
// run's identity and cell, its classes, ADR-0045's repeat classification
// (internal/repeats, the code the driver and cpgate use), and the §6.2
// review, from the run's full local history, of the operations each
// anomaly names — for incompatible-order also the append operations that
// wrote each value absent from the key's longest read; for a register
// read the writes of the value it returned. The classification (§11), the
// origin, the analysis and the harness review are written by the person
// triaging.
//
// usage: k7triage --run <run-dir> [--cell <V1…>]
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/checker/output"
	"github.com/CesarManzoCode/commitproof/internal/gate"
	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/repeats"
)

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

// row maps a recorded completion to its §6.2 row (as k4triage).
func row(typ string, err map[string]any) (int, string) {
	st, ph := str(err, "sqlstate"), str(err, "phase")
	class := ""
	if len(st) == 5 {
		class = st[:2]
	}
	switch typ {
	case "ok":
		return 1, "success"
	case "fail":
		switch {
		case st == "40001" && ph == "commit":
			return 3, "40001 returned by COMMIT"
		case class == "40":
			return 2, "class 40 before COMMIT"
		case class == "23":
			return 4, "class 23 before commit"
		case st == "0A000" || class == "42" || class == "22":
			return 5, st + " before any write executed"
		case st == "25006":
			return 6, "25006 read-only"
		case st == "MTF01" || st == "57P03":
			return 7, "no writable primary before the first statement"
		case ph == "connect":
			return 10, "connection error before any statement"
		}
		return 0, "fail with no matching row: " + st + "/" + ph
	case "info":
		switch {
		case ph == "commit":
			return 8, "error during or after COMMIT"
		case ph == "client-timeout":
			return 9, "client timeout after a write was sent"
		case st != "":
			return 11, "SQLSTATE " + st + " not listed"
		}
		return 12, "autocommit transport error or timeout after sending"
	}
	return 0, "unknown completion " + typ
}

var (
	incompatibleRE = regexp.MustCompile(`^reads of key (\d+) are not prefixes of each other: \[([0-9 ]*)\] \(i=(\d+)\) and \[([0-9 ]*)\] \(i=(\d+)\)$`)
	registerRE     = regexp.MustCompile(`^register (\d+): .* i=(\d+) .*-> \{known:true set:true v:(-?\d+)`)
	registerKeyRE  = regexp.MustCompile(`^register (\d+): `)
)

func main() {
	dir := flag.String("run", "", "sealed run directory (full, local)")
	cell := flag.String("cell", "", "K7 cell label")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "usage: k7triage --run <run-dir> [--cell <V1…>]")
		os.Exit(3)
	}
	d := strings.TrimSuffix(*dir, "/")
	rep := history.VerifySeal(d)
	if !rep.Valid {
		fmt.Fprintln(os.Stderr, "seal:", rep.Problems)
		os.Exit(2)
	}
	cb, err := os.ReadFile(d + ".checks.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	checks, err := output.Decode(cb)
	if err != nil || checks.SealRoot != rep.MerkleRoot {
		fmt.Fprintln(os.Stderr, "checks.json:", err, "or seal root differs")
		os.Exit(2)
	}
	cls, err := repeats.Classify(cb)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	mb, _ := os.ReadFile(d + "/manifest.json")
	var man map[string]any
	dec := json.NewDecoder(bytes.NewReader(mb))
	dec.UseNumber()
	dec.Decode(&man)

	comp := map[int64]history.Event{}
	inv := map[int64]history.Event{}
	for _, ev := range rep.History {
		if ev.Op != nil {
			comp[*ev.Op] = ev
		}
		if ev.Type == "invoke" {
			inv[ev.I] = ev
		}
	}
	review := func(op int64, why string) map[string]any {
		e := map[string]any{"op": op, "why": why}
		iv, ok := inv[op]
		if ok {
			e["invoke_t_ns"], e["wl"], e["f"], e["gw"] = iv.T, iv.WL, iv.F, iv.GW
			if iv.Client != nil {
				e["client"] = *iv.Client
			}
		}
		c, ok := comp[op]
		if !ok {
			e["completion"] = "none (op not completed in the history)"
			return e
		}
		r, rw := row(c.Type, c.Err)
		e["completion"], e["row"], e["row_reason"], e["complete_t_ns"] = c.Type, int64(r), rw, c.T
		if c.Err != nil {
			e["err"] = c.Err
		}
		if len(c.Notices) > 0 {
			e["notices"] = c.Notices
		}
		return e
	}
	// appendsOf: the invokes whose transaction appends v to key k.
	appendsOf := func(k, v int64) []int64 {
		var out []int64
		for _, ev := range rep.History {
			if ev.Type != "invoke" || ev.WL != "append" {
				continue
			}
			mops, _ := ev.Value.([]any)
			for _, m := range mops {
				mo, _ := m.([]any)
				if len(mo) == 3 && fmt.Sprint(mo[0]) == "append" && fmt.Sprint(mo[1]) == strconv.FormatInt(k, 10) && fmt.Sprint(mo[2]) == strconv.FormatInt(v, 10) {
					out = append(out, ev.I)
				}
			}
		}
		return out
	}
	// writesOf: register writes or cas of value v to key k.
	writesOf := func(k, v int64) []int64 {
		var out []int64
		for _, ev := range rep.History {
			if ev.Type != "invoke" || ev.WL != "register" || (ev.F != "write" && ev.F != "cas") {
				continue
			}
			val, _ := ev.Value.(map[string]any)
			if fmt.Sprint(val["k"]) != strconv.FormatInt(k, 10) {
				continue
			}
			nv := val["v"]
			if ev.F == "cas" {
				nv = val["new"]
			}
			if fmt.Sprint(nv) == strconv.FormatInt(v, 10) {
				out = append(out, ev.I)
			}
		}
		return out
	}
	var reviews []any
	for _, ch := range checks.Checkers {
		if ch.Valid != output.ValidFalse {
			continue
		}
		for _, a := range ch.Anomalies {
			ar := map[string]any{"checker": ch.ID, "class": a.Class, "explanation": a.Explanation}
			var ops []any
			for _, op := range a.Ops {
				ops = append(ops, review(op, "named by the anomaly"))
			}
			if m := incompatibleRE.FindStringSubmatch(a.Explanation); m != nil {
				k, _ := strconv.ParseInt(m[1], 10, 64)
				later := map[string]bool{}
				for _, e := range strings.Fields(m[4]) {
					later[e] = true
				}
				for _, e := range strings.Fields(m[2]) {
					if later[e] {
						continue
					}
					v, _ := strconv.ParseInt(e, 10, 64)
					ws := appendsOf(k, v)
					if len(ws) == 0 {
						ops = append(ops, map[string]any{"why": fmt.Sprintf("writer of vanished value %d of key %d", v, k), "completion": "no append of it in the history"})
					}
					for _, w := range ws {
						ops = append(ops, review(w, fmt.Sprintf("writer of value %d of key %d, absent from the longest read", v, k)))
					}
				}
			}
			if m := registerRE.FindStringSubmatch(a.Explanation); m != nil {
				k, _ := strconv.ParseInt(m[1], 10, 64)
				v, _ := strconv.ParseInt(m[3], 10, 64)
				for _, w := range writesOf(k, v) {
					ops = append(ops, review(w, fmt.Sprintf("writer of value %d of register %d, which the offending read returned", v, k)))
				}
			}
			if m := registerKeyRE.FindStringSubmatch(a.Explanation); m != nil {
				// The mechanism of the recorded G6 finding: a value whose write
				// did not complete ok, returned by an ok read of the register.
				k, _ := strconv.ParseInt(m[1], 10, 64)
				for _, ev := range rep.History {
					if ev.Type != "ok" || ev.WL != "register" || ev.F != "read" {
						continue
					}
					val, _ := ev.Value.(map[string]any)
					if fmt.Sprint(val["k"]) != strconv.FormatInt(k, 10) || val["v"] == nil {
						continue
					}
					v, err := strconv.ParseInt(fmt.Sprint(val["v"]), 10, 64)
					if err != nil {
						continue
					}
					for _, w := range writesOf(k, v) {
						if c, ok := comp[w]; ok && c.Type == "ok" {
							continue
						}
						ops = append(ops, review(*ev.Op, fmt.Sprintf("ok read of register %d returning value %d, whose write did not complete ok", k, v)),
							review(w, fmt.Sprintf("writer of value %d of register %d (completion not ok), read by op %d", v, k, *ev.Op)))
					}
				}
			}
			ar["review"] = ops
			reviews = append(reviews, ar)
		}
	}
	var clsAny any
	j, _ := json.Marshal(cls)
	dj := json.NewDecoder(bytes.NewReader(j))
	dj.UseNumber()
	dj.Decode(&clsAny)
	out := map[string]any{"run_id": checks.RunID, "seal_root": rep.MerkleRoot, "checks_sha256": gate.FileSHA256(d + ".checks.json"),
		"cell": *cell, "campaign": str(man, "campaign"), "profile": str(man, "profile"), "seed": man["seed"], "contract_version": str(man, "contract_version"),
		"verdict": checks.Verdict, "repeat": clsAny, "completion_review": reviews,
		"classification": "TODO (§11; ADR-0042 for G2a/G2b)", "harness_review": "TODO", "analysis": "TODO"}
	b, err := canonjson.Marshal(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var pretty bytes.Buffer
	json.Indent(&pretty, b, "", "  ")
	fmt.Println(pretty.String())
}
