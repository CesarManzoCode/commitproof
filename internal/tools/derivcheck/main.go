// Command derivcheck implements `make check-dockerfile-derivation` (CONTRACT
// §5.2, K1.3): build/Dockerfile and build/Dockerfile.pgctld must differ from
// upstream's files at the pinned commit only in FROM lines. With --out it
// also writes the evidence the gate re-checks on its own: copies of both
// sides and derivation.json with the upstream git blob ids.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/gate"
	"github.com/CesarManzoCode/commitproof/internal/host"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

func git(dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), host.QuickTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

func main() {
	upstream := flag.String("upstream", ".cache/multigres", "clone of Multigres")
	build := flag.String("build", "build", "directory with the derived Dockerfiles")
	out := flag.String("out", "", "evidence directory to write (optional)")
	flag.Parse()
	files := []any{}
	ok := true
	for _, name := range []string{"Dockerfile", "Dockerfile.pgctld"} {
		up, err := git(*upstream, "show", params.MultigresCommit+":"+name)
		if err != nil {
			fail(err)
		}
		blob, err := git(*upstream, "rev-parse", params.MultigresCommit+":"+name)
		if err != nil {
			fail(err)
		}
		b := strings.TrimSpace(string(blob))
		if gate.GitBlobID(up) != b {
			fail(fmt.Errorf("%s: content does not hash to git blob %s", name, b))
		}
		der, err := os.ReadFile(filepath.Join(*build, name))
		if err != nil {
			fail(err)
		}
		r := gate.CheckDerivation(up, der)
		ok = ok && r.OK
		for _, v := range r.Violations {
			fmt.Fprintf(os.Stderr, "%s: %s\n", name, v)
		}
		files = append(files, map[string]any{"upstream": name, "derived": filepath.ToSlash(filepath.Join(*build, name)),
			"upstream_blob": b, "changed": r.Changed, "violations": r.Violations, "ok": r.OK})
		if *out != "" {
			for sub, content := range map[string][]byte{"upstream": up, "build": der} {
				if err := os.MkdirAll(filepath.Join(*out, sub), 0o755); err != nil {
					fail(err)
				}
				if err := os.WriteFile(filepath.Join(*out, sub, name), content, 0o644); err != nil {
					fail(err)
				}
			}
		}
	}
	rep := map[string]any{"command": "check-dockerfile-derivation", "pin": params.MultigresCommit, "files": files, "ok": ok}
	if *out != "" {
		b, _ := json.MarshalIndent(rep, "", "  ")
		if err := os.WriteFile(filepath.Join(*out, "derivation.json"), append(b, '\n'), 0o644); err != nil {
			fail(err)
		}
	}
	b, _ := json.Marshal(rep)
	fmt.Println(string(b))
	if !ok {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "derivcheck:", err)
	fmt.Printf("{\"command\":\"check-dockerfile-derivation\",\"error\":%q,\"ok\":false}\n", err.Error())
	os.Exit(3)
}
