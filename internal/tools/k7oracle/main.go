// Command k7oracle writes ADR-0046's record of one run's elle-cli
// cross-check (evidence/K7/oracle/<run-id>.json) from the run's checks.json
// and the records of evidence/K7/commands.jsonl. A triage already in the
// output file is kept (it is written by hand for an oracle-disagreement).
//
// usage: k7oracle --ev evidence/K7 --checks <run>.checks.json --source <label> --out <file>
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/gate"
)

func main() {
	ev := flag.String("ev", "evidence/K7", "evidence directory")
	checks := flag.String("checks", "", "the run's checks.json")
	source := flag.String("source", "", "K7 cell, or the cited cell")
	out := flag.String("out", "", "output file")
	flag.Parse()
	fail := func(err error) {
		fmt.Printf("{\"command\":\"k7oracle\",\"error\":%q,\"exit_code\":2}\n", err.Error())
		os.Exit(2)
	}
	b, err := os.ReadFile(*checks)
	if err != nil {
		fail(err)
	}
	o, err := gate.DecideK7Oracle(gate.LoadEvidence(*ev), b)
	if err != nil {
		fail(err)
	}
	o.Source = *source
	if old, err := os.ReadFile(*out); err == nil {
		var prev gate.K7Oracle
		if json.Unmarshal(old, &prev) == nil {
			o.Triage = prev.Triage
		}
	}
	var v any
	j, _ := json.Marshal(o)
	dec := json.NewDecoder(bytes.NewReader(j))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		fail(err)
	}
	cb, err := canonjson.Marshal(v)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(*out, append(cb, '\n'), 0o644); err != nil {
		fail(err)
	}
	code := 0
	if o.Result != "confirmed" && o.Triage == "" {
		code = 1 // an oracle-disagreement awaits its triage
	}
	fmt.Printf("{\"command\":\"k7oracle\",\"run\":%q,\"result\":%q,\"decided_by\":%q,\"elle_valid\":%q,\"needs_triage\":%v,\"exit_code\":%d}\n", o.RunID, o.Result, o.DecidedBy, o.ElleValid, code == 1, code)
	os.Exit(code)
}
