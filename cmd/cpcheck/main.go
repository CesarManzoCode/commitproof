// Command cpcheck reads sealed run directories: it verifies seals (K1) and
// runs checkers (K2). It never trusts a status reported by commitproof
// (CONTRACT §5.3): verdicts come from the files alone.
//
// Exit codes: 0 success, 1 verdict FAIL, 2 INVALID/UNKNOWN, 3 usage error.
package main

import (
	"fmt"
	"os"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/history"
)

func summary(m map[string]any, code int) int {
	m["exit_code"] = code
	b, err := canonjson.Marshal(m)
	if err != nil {
		fmt.Printf("{\"command\":\"error\",\"exit_code\":2,\"error\":%q}\n", err.Error())
		return 2
	}
	fmt.Println(string(b))
	return code
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: cpcheck verify-seal <run-dir> | check <run-dir> [--elle-oracle] | corpus <dir>")
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
		probs := make([]any, len(r.Problems))
		for i, p := range r.Problems {
			probs[i] = p
		}
		m := map[string]any{"command": "verify-seal", "run_dir": args[1], "valid": r.Valid, "seal_root": r.MerkleRoot,
			"files": r.Files, "history_lines": r.HistoryLines, "observation_lines": r.ObservationLines, "problems": probs}
		if !r.Valid {
			return summary(m, 2)
		}
		return summary(m, 0)
	case "check", "corpus":
		return summary(map[string]any{"command": args[0], "error": "checkers are K2 (CONTRACT §15)"}, 3)
	}
	return summary(map[string]any{"command": "cpcheck", "error": "unknown command " + args[0]}, 3)
}
