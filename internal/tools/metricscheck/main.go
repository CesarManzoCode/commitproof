// Command metricscheck runs cpgate's independent recomputation of a
// rendered run report's metrics block (CONTRACT P23, §15 K6.5; ADR-0038)
// on any sealed run, for evidence outside K6's reference run: the run (a
// directory or its <run-id>.tar.zst archive) must verify its seal, and the
// report's cp-data block must name the run and hold exactly the metrics
// the gate recomputes from observations.jsonl. Prints one JSON line; exit
// 0 when they agree, 1 when they do not, 2 when an input does not verify,
// 3 on usage errors.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/gate"
	"github.com/CesarManzoCode/commitproof/internal/history"
)

func main() {
	report := flag.String("report", "", "rendered run report (<run-id>.html)")
	run := flag.String("run", "", "sealed run directory or <run-id>.tar.zst")
	flag.Parse()
	code, m := check(*report, *run)
	m["command"], m["exit_code"] = "metricscheck", code
	b, _ := json.Marshal(m)
	fmt.Println(string(b))
	os.Exit(code)
}

func check(report, run string) (int, map[string]any) {
	fail := func(code int, err error) (int, map[string]any) { return code, map[string]any{"error": err.Error()} }
	if report == "" || run == "" {
		return fail(3, fmt.Errorf("usage: metricscheck --report <html> --run <dir|archive>"))
	}
	dir := run
	if strings.HasSuffix(dir, history.ArchiveSuffix) {
		tmp, err := os.MkdirTemp("", "metricscheck-")
		if err != nil {
			return fail(3, err)
		}
		defer func() { chmodTree(tmp); os.RemoveAll(tmp) }()
		if err := history.ExtractArchive(dir, tmp); err != nil {
			return fail(2, err)
		}
		dir = filepath.Join(tmp, strings.TrimSuffix(filepath.Base(dir), history.ArchiveSuffix))
	}
	rep := history.VerifySeal(dir)
	if !rep.Valid {
		return 2, map[string]any{"error": "seal does not verify", "problems": rep.Problems}
	}
	page, err := os.ReadFile(report)
	if err != nil {
		return fail(3, err)
	}
	data, err := gate.ReportDataBlock(page)
	if err != nil {
		return fail(2, err)
	}
	id := filepath.Base(dir)
	var problems []string
	if fmt.Sprint(data["run_id"]) != id || fmt.Sprint(data["seal_root"]) != rep.MerkleRoot {
		problems = append(problems, "the data block does not name the run and its seal root")
	}
	p, note := gate.CheckMetrics(data, rep.Observations)
	problems = append(problems, p...)
	code := 0
	if len(problems) > 0 {
		code = 1
	}
	return code, map[string]any{"run": id, "seal_root": rep.MerkleRoot, "summary": note, "problems": problems}
}

// chmodTree makes an extracted sealed directory (0555) removable.
func chmodTree(dir string) {
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() {
			os.Chmod(p, 0o755)
		}
		return nil
	})
}
