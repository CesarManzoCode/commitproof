// Command cpgate evaluates a sprint gate from an evidence directory alone
// and, with --negative-controls, sabotages a copy of the evidence once per
// criterion and requires exactly that criterion to fail (CONTRACT §15,
// ADR-0017). It never trusts a status reported by commitproof.
//
// Exit codes: 0 gate PASS (and every negative control behaved), 1 gate FAIL,
// 3 usage error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/gate"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

func summary(m map[string]any, code int) int {
	m["exit_code"] = code
	b, err := canonjson.Marshal(m)
	if err != nil {
		fmt.Printf("{\"command\":\"cpgate\",\"exit_code\":2,\"error\":%q}\n", err.Error())
		return 2
	}
	fmt.Println(string(b))
	return code
}

func writeJSON(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: cpgate K<n> --evidence evidence/K<n> [--negative-controls]")
		return summary(map[string]any{"command": "cpgate", "error": "missing sprint"}, 3)
	}
	sprint := args[0]
	fs := flag.NewFlagSet("cpgate", flag.ContinueOnError)
	evidence := fs.String("evidence", "", "evidence directory")
	controls := fs.Bool("negative-controls", false, "run the negative controls")
	if err := fs.Parse(args[1:]); err != nil || *evidence == "" {
		return summary(map[string]any{"command": "cpgate", "sprint": sprint, "error": "--evidence is required"}, 3)
	}
	var crits []gate.Criterion
	switch sprint {
	case "K1":
		crits = gate.K1()
	case "K2":
		crits = gate.K2()
	case "K3":
		crits = gate.K3()
	case "K4":
		crits = gate.K4()
	case "K5":
		crits = gate.K5()
	case "K6":
		crits = gate.K6()
	default:
		return summary(map[string]any{"command": "cpgate", "sprint": sprint, "error": "gate not implemented yet"}, 3)
	}
	results := gate.Evaluate(*evidence, crits)
	gf := gate.GateFile{Sprint: sprint, ContractVersion: params.ContractVersion, Evidence: filepath.ToSlash(*evidence),
		Result: "FAIL", Criteria: results}
	if gate.Passed(results) {
		gf.Result = "PASS"
	}
	if err := writeJSON(filepath.Join(*evidence, "gate.json"), gf); err != nil {
		return summary(map[string]any{"command": "cpgate", "error": err.Error()}, 3)
	}
	crit := map[string]any{}
	for _, r := range results {
		crit[r.ID] = r.Result
	}
	m := map[string]any{"command": "cpgate", "sprint": sprint, "result": gf.Result, "criteria": crit}
	ok := gf.Result == "PASS"
	if *controls {
		cs := gate.NegativeControls(*evidence, crits)
		if err := writeJSON(filepath.Join(*evidence, "negative-controls.json"), cs); err != nil {
			return summary(map[string]any{"command": "cpgate", "error": err.Error()}, 3)
		}
		nc := map[string]any{}
		for _, c := range cs {
			failed := make([]any, len(c.Failed))
			for i, f := range c.Failed {
				failed[i] = f
			}
			nc[c.Criterion] = map[string]any{"failed": failed, "failed_only_target": c.FailedOnlyTarget}
		}
		m["negative_controls"] = nc
		m["negative_controls_ok"] = gate.ControlsPassed(cs)
		ok = ok && gate.ControlsPassed(cs)
	}
	if !ok {
		return summary(m, 1)
	}
	return summary(m, 0)
}
