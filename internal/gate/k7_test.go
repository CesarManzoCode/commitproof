package gate

import "testing"

// TestK7ReplayArgv: K7.4 accepts the package's replay command only with the
// minimized run as an argument and --times followed by P13 (5), matched as
// tokens: a "1" inside a path or a seed does not stand for --times 1.
func TestK7ReplayArgv(t *testing.T) {
	const run = "20261012T014955Z-D1-1-real-6625432a"
	cases := []struct {
		argv []string
		want bool
	}{
		{[]string{"bin/commitproof", "replay", "runs/k7-pkg/FND-D1-fb8d0f8c/" + run, "--times", "5", "--out", "runs/k7-pkg/FND-D1-fb8d0f8c", "--cpcheck", "bin/cpcheck"}, true},
		{[]string{"bin/commitproof", "replay", "runs/k7-pkg/x/" + run + "/", "--times", "5", "--out", "runs/k7-pkg/x"}, true},
		{[]string{"bin/commitproof", "replay", "runs/k7-pkg/x/" + run, "--times", "1", "--out", "runs/k7-pkg/1"}, false},
		{[]string{"bin/commitproof", "replay", "runs/k7-pkg/x/" + run, "--out", "runs/k7-pkg/5"}, false},
		{[]string{"bin/commitproof", "minimize", "runs/k7-pkg/x/" + run, "--times", "5"}, false},
		{[]string{"bin/commitproof", "replay", "runs/k7-pkg/x/" + run + "-other", "--times", "5"}, false},
		{[]string{"bin/commitproof", "replay", "runs/k7-pkg/x/" + run, "--times"}, false},
	}
	for i, c := range cases {
		if got := k7ReplayArgv(c.argv, run); got != c.want {
			t.Errorf("case %d %q: got %v, want %v", i, c.argv, got, c.want)
		}
	}
}

// TestK7IndexContract: K7's matrix runs carry the version the matrix ran
// under (1.8.1) and K7.4's package replays the version ADR-0047 set before
// they ran (1.9.0); each directory accepts its own version only. Before the
// fix, package-replays/ was held to 1.8.1, so every K7.4 replay was a
// problem in cpgate's index.
func TestK7IndexContract(t *testing.T) {
	cases := []struct {
		sub, version string
		want         bool
	}{
		{k7RunsSub, "1.8.1", true},
		{k7RunsSub, "1.9.0", false},
		{k7RunsSub, "1.8.0", false},
		{k7PkgSub, "1.9.0", true},
		{k7PkgSub, "1.8.1", false},
		{k7PkgSub, "1.9.1", false},
		{k7PkgSub, "", false},
	}
	for _, c := range cases {
		ok := k7IndexContract[c.sub]
		if ok == nil {
			t.Fatalf("no contract check for %s", c.sub)
		}
		if got := ok("C8", c.version); got != c.want {
			t.Errorf("%s accepts %q: got %v, want %v", c.sub, c.version, got, c.want)
		}
	}
}

// TestK7Repro: §11.1's classes from the hits of P13 replays.
func TestK7Repro(t *testing.T) {
	for hits, want := range map[int]string{0: "NOT_REPRODUCED", 1: "INTERMITTENT", 2: "INTERMITTENT", 3: "REPRODUCED", 5: "REPRODUCED"} {
		if got := k7Repro(hits); got != want {
			t.Errorf("k7Repro(%d) = %s, want %s", hits, got, want)
		}
	}
}
