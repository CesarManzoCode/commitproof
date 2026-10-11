// Command k4triage drafts the mechanical part of a K4.7 triage record for a
// real-build FAIL run (CONTRACT §15 K4.7): the run's identity, its anomaly
// classes from checks.json, cpcheck's elle-cli cross-check (C3), and the
// §6.2 review of every operation of the target class's anomalies: the
// recorded completion, its err, and the table row that err maps to. The
// classification (§11), the origin hypothesis and the harness review are
// written by the person triaging; this tool only prints the draft.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/gate"
	"github.com/CesarManzoCode/commitproof/internal/history"
)

func readJSON(p string) (map[string]any, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var m map[string]any
	return m, d.Decode(&m)
}

func str(m map[string]any, k string) string { s, _ := m[k].(string); return s }

// row maps a recorded completion to its §6.2 row (the rows of the closed
// table, in order: 1 success; 2 class 40 before COMMIT; 3 40001 by COMMIT;
// 4 class 23; 5 0A000/42/22 before a write; 6 25006; 7 no writable primary
// before the first statement; 8 error at or after COMMIT; 9 client timeout
// after a write was sent; 10 connection error before any statement; 11 any
// other SQLSTATE; 12 autocommit transport error or timeout).
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

func main() {
	dir := flag.String("run", "", "sealed run directory")
	class := flag.String("class", "", "anomaly class to review (default: the run's target class)")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "usage: k4triage --run <run-dir> [--class <anomaly>]")
		os.Exit(3)
	}
	rep := history.VerifySeal(*dir)
	if !rep.Valid {
		fmt.Fprintln(os.Stderr, "seal:", rep.Problems)
		os.Exit(2)
	}
	cp := strings.TrimSuffix(*dir, "/") + ".checks.json"
	cj, err := readJSON(cp)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var classes []any
	seen := map[string]bool{}
	ops := map[string][]int64{}
	var oracle any = "not-applicable (no C3 anomaly)"
	cs, _ := cj["checkers"].([]any)
	for _, x := range cs {
		c, _ := x.(map[string]any)
		if str(c, "id") == "C3" && fmt.Sprint(c["valid"]) == "false" {
			st, _ := c["stats"].(map[string]any)
			if e, ok := st["elle"].(map[string]any); ok {
				oracle = map[string]any{"elle_valid": fmt.Sprint(e["valid"]), "elle_classes": e["classes"], "agree": e["agree"],
					"artifact": filepath.Base(*dir) + ".elle/"}
			} else {
				oracle = "MISSING: run `cpcheck check <run> --elle-oracle`"
			}
		}
		if fmt.Sprint(c["valid"]) != "false" {
			continue
		}
		as, _ := c["anomalies"].([]any)
		for _, y := range as {
			a, _ := y.(map[string]any)
			cl := str(a, "class")
			if !seen[cl] {
				seen[cl] = true
				classes = append(classes, cl)
			}
			ol, _ := a["ops"].([]any)
			for _, o := range ol {
				if n, ok := o.(json.Number); ok {
					v, _ := n.Int64()
					ops[cl] = append(ops[cl], v)
				}
			}
		}
	}
	target := *class
	if target == "" && len(classes) > 0 {
		target = fmt.Sprint(classes[0])
	}
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
	var review []any
	for _, op := range ops[target] {
		c, ok := comp[op]
		if !ok {
			review = append(review, map[string]any{"op": op, "completion": "MISSING"})
			continue
		}
		r, why := row(c.Type, c.Err)
		e := map[string]any{"op": op, "completion": c.Type, "row": int64(r), "row_reason": why, "wl": c.WL, "f": c.F, "gw": c.GW}
		if c.Err != nil {
			e["err"] = c.Err
		}
		if len(c.Notices) > 0 {
			e["notices"] = c.Notices
		}
		if iv, ok := inv[op]; ok {
			e["invoke_t_ns"], e["complete_t_ns"] = iv.T, c.T
		}
		review = append(review, e)
	}
	out := map[string]any{"run_id": filepath.Base(*dir), "seal_root": rep.MerkleRoot, "checks_sha256": gate.FileSHA256(cp),
		"classes": classes, "anomaly_class": target, "oracle": oracle, "completion_review": review,
		"classification": map[string]any{"class": "TODO", "guarantee": "TODO", "claim_level": "TODO", "origin": "TODO",
			"reproducibility": "TODO (K4.5)", "known_status": "TODO", "severity": "TODO (§11.2)"},
		"finding_id": "TODO (§11.3)", "harness_defect": "TODO", "harness_review": "TODO"}
	b, _ := canonjson.Marshal(out)
	var pretty bytes.Buffer
	json.Indent(&pretty, b, "", "  ")
	fmt.Println(pretty.String())
}
