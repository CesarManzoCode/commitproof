// Command k2check is the verdict step of `make ci-checkers` (CONTRACT §13):
// it re-evaluates cpcheck corpus outputs with the gate's own comparison
// (internal/gate, independent of cpcheck): every corpus entry matches its
// expected.json, C3 agrees with elle-cli except registered divergences, and
// each checker sabotage fails exactly its dependent labels.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/gate"
)

func main() {
	corpus := flag.String("corpus", gate.K2Corpus, "corpus directory")
	results := flag.String("results", "", "results.jsonl of the corpus run")
	raw := flag.String("raw", "", "elle raw output directory")
	divs := flag.String("divergences", "evidence/K2/divergences.json", "divergence registry")
	sab := flag.String("sabotage", "", "directory with <rule>/results.jsonl")
	list := flag.Bool("list-rules", false, "print the sabotage rules and exit")
	flag.Parse()
	rules := make([]string, 0, len(gate.K2SabotageDeps))
	for r := range gate.K2SabotageDeps {
		rules = append(rules, r)
	}
	sort.Strings(rules)
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
	st, pp := gate.K2ElleCompare(*corpus, *results, *raw, *divs)
	p = append(p, pp...)
	for _, rule := range rules {
		d, pp := gate.K2CheckSabotage(rule, *corpus, filepath.Join(*sab, rule, "results.jsonl"))
		p = append(p, pp...)
		fmt.Fprintln(os.Stderr, d)
	}
	for _, x := range p {
		fmt.Fprintln(os.Stderr, "PROBLEM", x)
	}
	fmt.Printf("{\"command\":\"k2check\",\"entries\":%d,\"elle_compared\":%d,\"elle_disagreements\":%d,\"sabotage_rules\":%d,\"problems\":%d}\n",
		len(rs), st.Compared, len(st.Disagreements), len(rules), len(p))
	if len(p) > 0 {
		os.Exit(1)
	}
}
