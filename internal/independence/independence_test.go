// Package independence holds the binary-independence test of CONTRACT §5.3:
// cpcheck, cpgate (and cpreport, from K6) must not depend on runner,
// nemesis, cluster, workload or observation packages, nor on any Multigres
// package. The verdict binaries read files; they never link the code that
// drives or observes the system under test.
package independence

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const module = "github.com/CesarManzoCode/commitproof"

// Forbidden import path prefixes for verdict binaries. Beyond the contract's
// list (runner, nemesis, cluster, workload, observation, Multigres), the
// host-side helpers that run Docker or generate plans are excluded too.
var forbidden = []string{
	module + "/internal/runner",
	module + "/internal/nemesis",
	module + "/internal/cluster",
	module + "/internal/workload",
	module + "/internal/observe",
	module + "/internal/images",
	module + "/internal/host",
	module + "/internal/cmdlog",
	module + "/internal/plan",
	module + "/internal/synth",
	"github.com/multigres/",
	"github.com/jackc/pgx",
	"google.golang.org/grpc",
}

func deps(t *testing.T, pkg string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", pkg)
	cmd.Dir = filepath.Join("..", "..")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list -deps %s: %v\n%s", pkg, err, ee.Stderr)
		}
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	return strings.Fields(string(out))
}

func TestVerdictBinariesAreIndependent(t *testing.T) {
	for _, bin := range []string{"./cmd/cpcheck", "./cmd/cpgate"} {
		d := deps(t, bin)
		if len(d) < 5 {
			t.Fatalf("%s: implausible dependency list %v", bin, d)
		}
		for _, p := range d {
			for _, f := range forbidden {
				if strings.HasPrefix(p, f) {
					t.Errorf("%s depends on forbidden package %s", bin, p)
				}
			}
		}
	}
}

// TestControlIsSensitive proves the check can fail: commitproof itself links
// the observation package and Multigres.
func TestControlIsSensitive(t *testing.T) {
	d := strings.Join(deps(t, "./cmd/commitproof"), "\n")
	for _, want := range []string{module + "/internal/observe", "github.com/multigres/multigres/go/pb/multipoolermanager"} {
		if !strings.Contains(d, want) {
			t.Errorf("commitproof does not depend on %s; the independence test would not notice a leak", want)
		}
	}
}

// TestExpectationsIndependentOfCheckers enforces AGENTS.md: expected outputs
// of the synthetic corpus are never produced by running the code under
// test. The generator must not link the checkers, and the checkers must not
// link the generator.
func TestExpectationsIndependentOfCheckers(t *testing.T) {
	for _, c := range []struct{ pkg, forbid string }{
		{"./internal/synth", module + "/internal/checker"},
		{"./internal/tools/gencorpus", module + "/internal/checker"},
		{"./internal/checker", module + "/internal/synth"},
	} {
		for _, p := range deps(t, c.pkg) {
			if strings.HasPrefix(p, c.forbid) {
				t.Errorf("%s depends on %s", c.pkg, p)
			}
		}
	}
}

// sabotageMarker is the symbol prefix only sabotage builds define
// (internal/checker/sabotage_*_on.go, CONTRACT §15 K2.6;
// internal/nemesis/sabotage_*_on.go, §15 K3.2–K3.3).
const sabotageMarker = "cpSabotageActive"

// sabotageDirs may declare sabotage build tags; nowhere else may.
var sabotageDirs = []string{filepath.Join("internal", "checker"), filepath.Join("internal", "nemesis")}

func buildAndNM(t *testing.T, tags, pkg string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "bin")
	args := []string{"build", "-o", out}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	cmd := exec.Command("go", append(args, pkg)...)
	cmd.Dir = filepath.Join("..", "..")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s %s: %v\n%s", tags, pkg, err, b)
	}
	nm, err := exec.Command("go", "tool", "nm", out).Output()
	if err != nil {
		t.Fatalf("go tool nm: %v", err)
	}
	return string(nm)
}

// TestProductBinariesHaveNoSabotage builds the release binaries as `make
// build` does (no tags) and asserts no sabotage symbol is linked; the
// control builds cpcheck with one sabotage tag and requires the symbol, so
// the check can fail.
func TestProductBinariesHaveNoSabotage(t *testing.T) {
	for _, bin := range []string{"./cmd/cpcheck", "./cmd/cpgate", "./cmd/commitproof"} {
		if strings.Contains(buildAndNM(t, "", bin), sabotageMarker) {
			t.Errorf("%s contains %s symbols", bin, sabotageMarker)
		}
	}
	if !strings.Contains(buildAndNM(t, "cp_sabotage_bank_sum", "./cmd/cpcheck"), sabotageMarker+"_bank_sum") {
		t.Error("control: a cp_sabotage_bank_sum build of cpcheck lacks the marker; the check would not notice sabotage")
	}
	if !strings.Contains(buildAndNM(t, "cp_sabotage_inject_f1a", "./cmd/commitproof"), sabotageMarker+"_inject_f1a") {
		t.Error("control: a cp_sabotage_inject_f1a build of commitproof lacks the marker; the check would not notice sabotage")
	}
	// No build-tag file outside the sabotage packages may name a sabotage tag.
	inSabotageDir := func(p string) bool {
		for _, d := range sabotageDirs {
			if strings.Contains(p, d) {
				return true
			}
		}
		return false
	}
	filepath.WalkDir(filepath.Join("..", ".."), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || inSabotageDir(p) {
			if d != nil && d.IsDir() && (d.Name() == ".cache" || d.Name() == "testdata" || d.Name() == "evidence") {
				return filepath.SkipDir
			}
			return nil
		}
		b, _ := os.ReadFile(p)
		if bytes.Contains(b, []byte("//go:"+"build cp_sabotage")) {
			t.Errorf("%s declares a sabotage build tag outside %v", p, sabotageDirs)
		}
		return nil
	})
}

// TestMultigresImporters enforces CONTRACT §10.8 / §5.3 (ADR-0016): outside
// the observation package, the only importer of a Multigres package is the
// separate quorumxcheck binary. The control requires both to be found, so
// the check can fail.
func TestMultigresImporters(t *testing.T) {
	cmd := exec.Command("go", "list", "-f", `{{.ImportPath}}{{range .Imports}} {{.}}{{end}}`, "./...")
	cmd.Dir = filepath.Join("..", "..")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	allowed := map[string]bool{module + "/internal/observe": true, module + "/cmd/quorumxcheck": true}
	found := map[string]bool{}
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(l)
		for _, imp := range f[1:] {
			if strings.HasPrefix(imp, "github.com/multigres/") {
				found[f[0]] = true
				if !allowed[f[0]] {
					t.Errorf("%s imports %s (only the observation code and quorumxcheck may)", f[0], imp)
				}
			}
		}
	}
	for p := range allowed {
		if !found[p] {
			t.Errorf("control: %s imports no Multigres package; the check would not notice a leak", p)
		}
	}
}
