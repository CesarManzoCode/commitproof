// Command gencorpus writes the synthetic corpora from internal/synth: with
// --set k2 (default) the K2 corpus (CONTRACT §15 K2),
// testdata/corpus/<id>/{run/ (sealed), expected.json} and
// testdata/pathological/; with --set k5 the K5 corpus (§15 K5.1, ADR-0030),
// testdata/corpus-k5/. Output is a pure function of the code: --verify
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
	set := flag.String("set", "k2", "corpus: k2 (corpus/, pathological/) or k5 (corpus-k5/)")
	flag.Parse()
	if *set != "k2" && *set != "k5" {
		check(fmt.Errorf("--set %q: want k2 or k5", *set))
	}
	dirs = map[string][]string{"k2": {"corpus", "pathological"}, "k5": {"corpus-k5"}}[*set]
	if *verify {
		tmp, err := os.MkdirTemp("", "gencorpus-")
		check(err)
		defer removeAll(tmp)
		check(generate(tmp, *only, *set))
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
	for _, d := range dirs {
		removeAll(filepath.Join(*out, d))
	}
	check(generate(*out, *only, *set))
	fmt.Printf("{\"command\":\"gencorpus\",\"set\":%q,\"out\":%q,\"entries\":%d}\n", *set, *out, len(entries(*set)))
}

// dirs are the output directories of the selected set.
var dirs []string

func entries(set string) []synth.Entry {
	if set == "k5" {
		return synth.K5Corpus()
	}
	return synth.Corpus()
}

func generate(root, only, set string) error {
	cd := filepath.Join(root, dirs[0])
	for _, e := range entries(set) {
		if only != "" && !bytes.HasPrefix([]byte(e.ID), []byte(only)) {
			continue
		}
		if err := e.Build().Write(cd); err != nil {
			return fmt.Errorf("%s: %w", e.ID, err)
		}
	}
	if only != "" || set != "k2" {
		return nil
	}
	return synth.Pathological().Write(filepath.Join(root, "pathological"))
}

func files(root string) map[string][]byte {
	out := map[string][]byte{}
	for _, d := range dirs {
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
