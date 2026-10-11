// Command gencorpus writes the K2 synthetic corpus (CONTRACT §15 K2) from
// internal/synth: testdata/corpus/<id>/{run/ (sealed), expected.json} and
// testdata/pathological/. Output is a pure function of the code: --verify
// regenerates into a temporary directory and compares every file byte for
// byte with the committed corpus.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/CesarManzoCode/commitproof/internal/synth"
)

func main() {
	out := flag.String("out", "testdata", "output root (corpus/ and pathological/ are written below it)")
	verify := flag.Bool("verify", false, "regenerate into a temp dir and compare with --out")
	only := flag.String("only", "", "generate only entries whose id has this prefix (development)")
	flag.Parse()
	if *verify {
		tmp, err := os.MkdirTemp("", "gencorpus-")
		check(err)
		defer removeAll(tmp)
		check(generate(tmp, *only))
		diffs := compare(tmp, *out)
		if len(diffs) > 0 {
			for i, d := range diffs {
				if i == 20 {
					fmt.Printf("… %d more\n", len(diffs)-20)
					break
				}
				fmt.Println(d)
			}
			fmt.Printf("{\"command\":\"gencorpus-verify\",\"identical\":false,\"differences\":%d}\n", len(diffs))
			os.Exit(1)
		}
		fmt.Printf("{\"command\":\"gencorpus-verify\",\"identical\":true}\n")
		return
	}
	for _, d := range []string{"corpus", "pathological"} {
		removeAll(filepath.Join(*out, d))
	}
	check(generate(*out, *only))
	fmt.Printf("{\"command\":\"gencorpus\",\"out\":%q,\"entries\":%d}\n", *out, len(synth.Corpus()))
}

func generate(root, only string) error {
	cd := filepath.Join(root, "corpus")
	for _, e := range synth.Corpus() {
		if only != "" && !bytes.HasPrefix([]byte(e.ID), []byte(only)) {
			continue
		}
		if err := e.Build().Write(cd); err != nil {
			return fmt.Errorf("%s: %w", e.ID, err)
		}
	}
	if only != "" {
		return nil
	}
	return synth.Pathological().Write(filepath.Join(root, "pathological"))
}

func files(root string) map[string][]byte {
	out := map[string][]byte{}
	for _, d := range []string{"corpus", "pathological"} {
		filepath.WalkDir(filepath.Join(root, d), func(p string, de fs.DirEntry, err error) error {
			if err == nil && de.Type().IsRegular() {
				rel, _ := filepath.Rel(root, p)
				b, _ := os.ReadFile(p)
				out[filepath.ToSlash(rel)] = b
			}
			return nil
		})
	}
	return out
}

func compare(a, b string) []string {
	fa, fb := files(a), files(b)
	var diffs []string
	for p, x := range fa {
		y, ok := fb[p]
		switch {
		case !ok:
			diffs = append(diffs, "missing in "+b+": "+p)
		case !bytes.Equal(x, y):
			diffs = append(diffs, "differs: "+p)
		}
	}
	for p := range fb {
		if _, ok := fa[p]; !ok {
			diffs = append(diffs, "not generated: "+p)
		}
	}
	sort.Strings(diffs)
	return diffs
}

func removeAll(dir string) {
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil {
			os.Chmod(p, 0o755)
		}
		return nil
	})
	os.RemoveAll(dir)
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "gencorpus:", err)
		os.Exit(1)
	}
}
