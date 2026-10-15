// Command cpreport renders CommitProof's static report from sealed runs
// (CONTRACT §5.3, §15 K6; ADR-0036): one self-contained HTML page per run,
// or a campaign directory's summary page with the §12.3 statistics. It
// reads files only and never trusts a status reported by commitproof.
//
// Exit codes: 0 success, 2 INVALID input (a seal that does not verify, a
// stub, a missing or mismatching checks.json: nothing is written), 3 usage
// error.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/report"
)

const usage = "usage: cpreport render <run-dir | run-id.tar.zst | campaign-dir> --out <dir>"

func summary(m map[string]any, code int) int {
	m["exit_code"] = code
	if s := report.ActiveSabotage(); len(s) > 0 {
		a := make([]any, len(s))
		for i, x := range s {
			a[i] = x
		}
		m["sabotage"] = a
	}
	b, err := canonjson.Marshal(m)
	if err != nil {
		fmt.Printf("{\"command\":\"cpreport\",\"exit_code\":2,\"error\":%q}\n", err.Error())
		return 2
	}
	fmt.Println(string(b))
	return code
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 || args[0] != "render" {
		fmt.Fprintln(os.Stderr, usage)
		return summary(map[string]any{"command": "cpreport", "error": "missing command render"}, 3)
	}
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	out := fs.String("out", "", "output directory")
	var pos []string
	rest := args[1:]
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return summary(map[string]any{"command": "render", "error": err.Error() + "; " + usage}, 3)
		}
		rest = fs.Args()
		if len(rest) > 0 {
			pos, rest = append(pos, rest[0]), rest[1:]
		}
	}
	if len(pos) != 1 || *out == "" {
		return summary(map[string]any{"command": "render", "error": usage}, 3)
	}
	in := pos[0]
	m := map[string]any{"command": "render", "input": filepath.Base(filepath.Clean(in))}
	if _, err := os.Stat(in); err != nil {
		m["error"] = fmt.Sprintf("%s: %v", filepath.Base(in), err)
		return summary(m, 3)
	}
	var page []byte
	var name string
	if report.IsRun(in) {
		m["mode"] = "run"
		r, err := report.LoadRun(in)
		if err != nil {
			m["error"] = err.Error()
			return summary(m, 2)
		}
		mod, err := report.Build(r)
		if err != nil {
			m["error"] = err.Error()
			return summary(m, 2)
		}
		if page, err = report.RenderRun(mod); err != nil {
			m["error"] = err.Error()
			return summary(m, 2)
		}
		name = r.ID + ".html"
		m["run_id"], m["seal_root"], m["verdict"] = r.ID, r.SealRoot, r.Checks.Verdict
	} else {
		m["mode"] = "campaign"
		c, err := report.LoadCampaign(in)
		if err != nil {
			m["error"] = err.Error()
			return summary(m, 2)
		}
		if page, err = report.RenderCampaign(c); err != nil {
			m["error"] = err.Error()
			return summary(m, 2)
		}
		name = "index.html"
		m["runs"] = int64(len(c.Runs))
	}
	// Only now, with the page complete, is anything written.
	if _, err := report.WriteFile(*out, name, page); err != nil {
		m["error"] = err.Error()
		return summary(m, 3)
	}
	sum := sha256.Sum256(page)
	m["outputs"] = []any{map[string]any{"file": name, "sha256": hex.EncodeToString(sum[:]), "bytes": int64(len(page))}}
	return summary(m, 0)
}
