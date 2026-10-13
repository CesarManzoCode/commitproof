// Command k2check is the verdict step of `make ci-checkers` (CONTRACT §13):
// it re-evaluates cpcheck corpus outputs with the gate's own comparison
// (internal/gate, independent of cpcheck): every corpus entry matches its
// expected.json, C3 agrees with elle-cli except registered divergences (K2
// corpus), and each checker sabotage fails exactly its dependent labels.
// --set k5 does the same for the K5 corpus (§15 K5.1) and its sabotage
// rules; that corpus has no Elle entries.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/gate"
)

func main() {
	set := flag.String("set", "k2", "corpus set: k2 or k5")
	corpus := flag.String("corpus", "", "corpus directory (default: the set's)")
	results := flag.String("results", "", "results.jsonl of the corpus run")
	raw := flag.String("raw", "", "elle raw output directory (k2)")
	divs := flag.String("divergences", "evidence/K2/divergences.json", "divergence registry (k2)")
	sab := flag.String("sabotage", "", "directory with <rule>/results.jsonl")
	list := flag.Bool("list-rules", false, "print the sabotage rules and exit")
	flag.Parse()
	deps, dir := gate.K2SabotageDeps, gate.K2Corpus
	switch *set {
	case "k2":
	case "k5":
		deps, dir = gate.K5SabotageDeps, gate.K5Corpus
	default:
		fmt.Fprintf(os.Stderr, "k2check: --set %q: want k2 or k5\n", *set)
		os.Exit(3)
	}
	if *corpus == "" {
		*corpus = dir
	}
	rules := gate.SabotageRules(deps)
	if *list {
		fmt.Println(strings.Join(rules, "\n"))
		return
	}
	var p []string
	rs, err := gate.LoadK2Results(*results)
	if err != nil {
		p = append(p, err.Error())
	}
	ids, _ := gate.CorpusIDs(*corpus)
	if len(rs) != len(ids) {
		p = append(p, fmt.Sprintf("%d results for %d corpus entries", len(rs), len(ids)))
	}
	for _, r := range rs {
		e, err := gate.LoadK2Expected(*corpus, r)
		if err != nil {
			p = append(p, err.Error())
			continue
		}
		if d := gate.K2Diff(r, e); len(d) > 0 {
			p = append(p, r.ID+": "+strings.Join(d, "; "))
		}
	}
	var st gate.K2ElleStats
	if *set == "k2" {
		var pp []string
		st, pp = gate.K2ElleCompare(*corpus, *results, *raw, *divs)
		p = append(p, pp...)
	}
	for _, rule := range rules {
		d, pp := gate.CheckSabotage(deps, rule, *corpus, gate.ResultsFile(filepath.Join(*sab, rule)))
		p = append(p, pp...)
		fmt.Fprintln(os.Stderr, d)
	}
	for _, x := range p {
		fmt.Fprintln(os.Stderr, "PROBLEM", x)
	}
	fmt.Printf("{\"command\":\"k2check\",\"set\":%q,\"entries\":%d,\"elle_compared\":%d,\"elle_disagreements\":%d,\"sabotage_rules\":%d,\"problems\":%d}\n",
		*set, len(rs), st.Compared, len(st.Disagreements), len(rules), len(p))
	if len(p) > 0 {
		os.Exit(1)
	}
}
