package checker

import (
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

// c3op is a serializable W1 transaction of its own process, invoked at
// i (as t) and completed ok at 100 + i: all transactions overlap, so there
// are no process or realtime edges.
func c3op(i int64, mops ...[]any) *Op {
	v := make([]any, len(mops))
	for j, m := range mops {
		v[j] = m
	}
	return &Op{Index: i, Proc: i, Client: i, GW: "gw-z1", WL: "append", F: "txn", Iso: "serializable",
		Invoke: i, Complete: 100 + i, Type: history.TypeOK, Value: v, InValue: v, Done: true}
}

func ap(k, v int64) []any { return []any{"append", k, v} }

func rd(k int64, vals ...int64) []any {
	if vals == nil {
		return []any{"r", k, nil}
	}
	l := make([]any, len(vals))
	for i, x := range vals {
		l[i] = x
	}
	return []any{"r", k, l}
}

func onlyAnomaly(t *testing.T, r Result, class string) Anomaly {
	t.Helper()
	if r.Valid != False || len(r.Anomalies) != 1 || r.Anomalies[0].Class != class {
		t.Fatalf("want exactly one %s, got valid %v anomalies %+v", class, r.Valid, r.Anomalies)
	}
	return r.Anomalies[0]
}

// Regression (K7, V1 C2 seed 3): a reported cycle names every transaction of
// the cycle. Before the fix, the anchor's last node was dropped: this
// two-transaction G-single was reported as "cycle i=2 -rw-> i=2" with ops
// [2]. Expected from the definitions (§10.3): T1 appends 10 to key 1 and 20
// to key 2; T2 reads key 1 = [10] (wr T1 → T2) and key 2 empty, and the
// version order of key 2 is [20] (T3's read), so T2 → T1 is rw on key 2;
// one rw edge: G-single over {T1, T2}. The anchor of G-single is its rw
// edge, so the cycle starts at T2.
func TestC3TwoNodeGSingleNamesBothTransactions(t *testing.T) {
	h := &History{Ops: []*Op{
		c3op(1, ap(1, 10), ap(2, 20)),
		c3op(2, rd(1, 10), rd(2)),
		c3op(3, rd(2, 20)),
	}}
	a := onlyAnomaly(t, CheckListAppend(h), "G-single")
	if want := []int64{1, 2}; !equalOps(a.Ops, want) {
		t.Errorf("ops %v, want %v", a.Ops, want)
	}
	if want := "cycle i=2 -rw(k=2)-> i=1 -wr(k=1)-> i=2"; a.Explanation != want {
		t.Errorf("explanation %q, want %q", a.Explanation, want)
	}
}

// A four-transaction G-nonadjacent: T1 -rw(1)-> T2 -wr(2)-> T3 -rw(3)-> T4
// -wr(4)-> T1 (two rw edges, not consecutive). Keys 1 and 3 get their
// version orders from the reads of T5 and T6.
func TestC3FourNodeGNonadjacentNamesEveryTransaction(t *testing.T) {
	h := &History{Ops: []*Op{
		c3op(1, rd(1), rd(4, 40)),
		c3op(2, ap(1, 10), ap(2, 20)),
		c3op(3, rd(2, 20), rd(3)),
		c3op(4, ap(3, 30), ap(4, 40)),
		c3op(5, rd(1, 10)),
		c3op(6, rd(3, 30)),
	}}
	a := onlyAnomaly(t, CheckListAppend(h), "G-nonadjacent")
	if want := []int64{1, 2, 3, 4}; !equalOps(a.Ops, want) {
		t.Errorf("ops %v, want %v", a.Ops, want)
	}
	if want := "cycle i=1 -rw(k=1)-> i=2 -wr(k=2)-> i=3 -rw(k=3)-> i=4 -wr(k=4)-> i=1"; a.Explanation != want {
		t.Errorf("explanation %q, want %q", a.Explanation, want)
	}
}

var cycleNode = regexp.MustCompile(`i=(\d+) -`)

// On every W1 history of the K2 corpus, every reported cycle (anomaly or
// observation) is simple and names one transaction per edge, at least two:
// the transactions of its explanation are exactly its ops.
func TestC3CorpusCyclesNameEveryTransaction(t *testing.T) {
	dirs, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "corpus", "w1-*", "run"))
	if len(dirs) == 0 {
		t.Skip("no K2 corpus")
	}
	cycles := 0
	for _, d := range dirs {
		h, _, err := Load(d)
		if err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		r := CheckListAppend(h)
		for _, a := range append(append([]Anomaly{}, r.Anomalies...), r.Observations...) {
			if !strings.HasPrefix(a.Explanation, "cycle ") {
				continue
			}
			cycles++
			var nodes []string
			seen := map[string]bool{}
			for _, m := range cycleNode.FindAllStringSubmatch(a.Explanation, -1) {
				if seen[m[1]] {
					t.Errorf("%s: %s is not simple: %q", d, a.Class, a.Explanation)
				}
				seen[m[1]] = true
				nodes = append(nodes, m[1])
			}
			sort.Strings(nodes)
			var ops []string
			for _, o := range a.Ops {
				ops = append(ops, strconv.FormatInt(o, 10))
			}
			sort.Strings(ops)
			if len(nodes) < 2 || strings.Join(nodes, ",") != strings.Join(ops, ",") {
				t.Errorf("%s: %s names %v, ops %v: %q", d, a.Class, nodes, a.Ops, a.Explanation)
			}
		}
	}
	if cycles == 0 {
		t.Fatal("no cycle in the corpus's W1 histories")
	}
}

func equalOps(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
