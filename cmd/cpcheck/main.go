// Command cpcheck reads sealed run directories: it verifies seals (K1) and
// runs checkers (K2). It never trusts a status reported by commitproof
// (CONTRACT §5.3): verdicts come from the files alone.
//
// Exit codes: 0 success, 1 verdict FAIL, 2 INVALID/UNKNOWN, 3 usage error.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/checker"
	"github.com/CesarManzoCode/commitproof/internal/checker/elle"
	"github.com/CesarManzoCode/commitproof/internal/history"
)

func summary(m map[string]any, code int) int {
	m["exit_code"] = code
	if s := checker.ActiveSabotage(); len(s) > 0 {
		m["sabotage"] = strs(s)
	}
	b, err := canonjson.Marshal(m)
	if err != nil {
		fmt.Printf("{\"command\":\"error\",\"exit_code\":2,\"error\":%q}\n", err.Error())
		return 2
	}
	fmt.Println(string(b))
	return code
}

func strs(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func main() { os.Exit(run(os.Args[1:])) }

const usage = "usage: cpcheck verify-seal <run-dir> | check <run-dir> [--elle-oracle] | corpus <dir> [--elle-oracle] [--out <dir> [--results-only]]"

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return summary(map[string]any{"command": "cpcheck", "error": "missing command"}, 3)
	}
	switch args[0] {
	case "verify-seal":
		if len(args) != 2 {
			return summary(map[string]any{"command": "verify-seal", "error": "usage: cpcheck verify-seal <run-dir>"}, 3)
		}
		if st, err := os.Stat(args[1]); err != nil || !st.IsDir() {
			return summary(map[string]any{"command": "verify-seal", "error": fmt.Sprintf("%s is not a directory", args[1])}, 3)
		}
		r := history.VerifySeal(args[1])
		m := map[string]any{"command": "verify-seal", "run_dir": args[1], "valid": r.Valid, "seal_root": r.MerkleRoot,
			"files": r.Files, "history_lines": r.HistoryLines, "observation_lines": r.ObservationLines, "problems": strs(r.Problems)}
		if !r.Valid {
			return summary(m, 2)
		}
		return summary(m, 0)
	case "check":
		return cmdCheck(args[1:])
	case "corpus":
		return cmdCorpus(args[1:])
	}
	return summary(map[string]any{"command": "cpcheck", "error": "unknown command " + args[0]}, 3)
}

// parse splits one positional argument from flags given before or after it.
func parse(fs *flag.FlagSet, args []string) (string, error) {
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return "", err
		}
		args = fs.Args()
		if len(args) > 0 {
			pos = append(pos, args[0])
			args = args[1:]
		}
	}
	if len(pos) != 1 {
		return "", fmt.Errorf("want exactly one directory, got %d", len(pos))
	}
	return pos[0], nil
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	oracle := fs.Bool("elle-oracle", false, "cross-check C3 with elle-cli")
	dir, err := parse(fs, args)
	if err != nil {
		return summary(map[string]any{"command": "check", "error": err.Error() + "; " + usage}, 3)
	}
	dir = filepath.Clean(dir)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return summary(map[string]any{"command": "check", "error": fmt.Sprintf("%s is not a directory", dir)}, 3)
	}
	m := map[string]any{"command": "check", "run_dir": filepath.ToSlash(dir)}
	h, rs, invalid, rep, err := checker.Check(dir)
	if err != nil {
		invalid = append(invalid, err.Error())
	}
	runID, root := filepath.Base(dir), ""
	if rep != nil {
		root = rep.MerkleRoot
	}
	if h != nil {
		runID = h.RunID
	}
	if *oracle && h != nil {
		if err := oracleOne(h, rs, dir+".elle"); err != nil {
			return summary(map[string]any{"command": "check", "run_dir": filepath.ToSlash(dir), "error": "elle oracle: " + err.Error()}, 3)
		}
	}
	verdict := checker.Verdict(invalid, rs)
	b, err := checker.ChecksJSON(runID, root, rs, verdict)
	if err != nil {
		return summary(map[string]any{"command": "check", "error": err.Error()}, 3)
	}
	out := dir + ".checks.json"
	if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
		return summary(map[string]any{"command": "check", "error": err.Error()}, 3)
	}
	cs := map[string]any{}
	for _, r := range rs {
		cs[r.ID] = r.Valid.JSON()
	}
	m["checks"], m["checks_sha256"], m["verdict"], m["checkers"], m["seal_root"] = filepath.ToSlash(out), sha(append(b, '\n')), verdict, cs, root
	m["invalid_reasons"] = strs(invalid)
	return summary(m, checker.ExitCode(verdict))
}

// oracleOne runs elle-cli on one run's W1 history and records the result in
// C3's stats (stats.elle); raw outputs go to outDir.
func oracleOne(h *checker.History, rs []checker.Result, outDir string) error {
	if len(checker.W1Txns(h)) == 0 {
		return nil
	}
	ctx := context.Background()
	img, err := elle.ImageID(ctx)
	if err != nil {
		return err
	}
	exp, model, err := elle.Export(h)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "history.elle.json"), exp, 0o644); err != nil {
		return err
	}
	res, _, err := elle.CheckFiles(ctx, outDir, []string{"history.elle.json"}, model)
	if err != nil {
		return err
	}
	r := res[0]
	for _, k := range elle.Kinds {
		if err := os.WriteFile(filepath.Join(outDir, k+".json"), append(r.Raw[k], '\n'), 0o644); err != nil {
			return err
		}
	}
	for i := range rs {
		if rs[i].ID == "C3" {
			c3 := rs[i]
			agree := r.Valid == "unknown" || (string(c3.Valid) == r.Valid && equal(c3Classes(c3), r.Classes))
			rs[i].Stats["elle"] = map[string]any{"image": img, "model": model, "valid": r.Valid, "classes": strs(r.Classes), "agree": agree}
		}
	}
	return nil
}

func c3Classes(r checker.Result) []string {
	all := append(append([]checker.Anomaly{}, r.Anomalies...), r.Observations...)
	return checker.Classes(all)
}

func equal(a, b []string) bool {
	return strings.Join(a, ",") == strings.Join(b, ",")
}
