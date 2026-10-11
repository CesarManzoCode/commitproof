// Package independence holds the binary-independence test of CONTRACT §5.3:
// cpcheck, cpgate (and cpreport, from K6) must not depend on runner,
// nemesis, cluster, workload or observation packages, nor on any Multigres
// package. The verdict binaries read files; they never link the code that
// drives or observes the system under test.
package independence

import (
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
