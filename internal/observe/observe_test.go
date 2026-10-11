package observe

import "testing"

func st(node string, term int64, leader string, cohort ...string) PoolerStatus {
	return PoolerStatus{Node: node, OK: true, Name: node,
		Decision: &Rule{CoordinatorTerm: term, LeaderSubterm: 0, Leader: leader, Cohort: cohort}}
}

func TestConsensus(t *testing.T) {
	want := []string{"n1", "n2", "n3"}
	c := []string{"n1", "n2", "n3"}
	if r, a, b := Consensus([]PoolerStatus{st("n1", 1, "n2", c...), st("n2", 1, "n2", c...), st("n3", 1, "n2", c...)}, want); r == nil || a != "" || b != "" {
		t.Fatalf("healthy cluster rejected: %v %q %q", r, a, b)
	}
	cases := map[string][]PoolerStatus{
		"no leader":       {st("n1", 1, "n4", c...), st("n2", 1, "n4", c...), st("n3", 1, "n4", c...)},
		"two leaders":     {st("n1", 1, "n1", c...), st("n2", 2, "n2", c...), st("n3", 2, "n2", c...)},
		"stale follower":  {st("n1", 1, "n2", c...), st("n2", 2, "n2", c...), st("n3", 2, "n2", c...)},
		"rpc failed":      {{Node: "n1", Error: "unavailable"}, st("n2", 1, "n2", c...), st("n3", 1, "n2", c...)},
		"no decided rule": {{Node: "n1", OK: true, Name: "n1"}, st("n2", 1, "n2", c...), st("n3", 1, "n2", c...)},
	}
	for name, sts := range cases {
		if _, a, _ := Consensus(sts, want); a == "" {
			t.Errorf("%s: part (a) satisfied", name)
		}
	}
	small := []string{"n1", "n2"}
	if _, a, b := Consensus([]PoolerStatus{st("n1", 1, "n2", small...), st("n2", 1, "n2", small...), st("n3", 1, "n2", small...)}, want); a != "" || b == "" {
		t.Errorf("cohort {n1,n2} accepted: a=%q b=%q", a, b)
	}
}
