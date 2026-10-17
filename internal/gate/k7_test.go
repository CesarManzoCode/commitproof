package gate

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

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

// TestK7NegativeControlsLinks: each control's copy reaches every other
// sprint's evidence through its links — claims.yaml names artifacts under
// K2, K3 and pre-K7, not only the cited K4/K5 — also when --evidence is a
// relative path (relative link targets would dangle inside the copy).
func TestK7NegativeControlsLinks(t *testing.T) {
	root := t.TempDir()
	files := []string{"evidence/K7/commands.jsonl", "evidence/K2/gate.json", "evidence/K3/logs/a.stdout", "evidence/K4/runs/r.tar.zst",
		"evidence/pre-K7/logs/b.stdout"}
	for _, f := range files {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)
	reach := Criterion{ID: "T.1", Apply: func(string) error { return nil }, Eval: func(e *Evidence) (string, []string, []string) {
		var p []string
		for _, f := range files {
			if b, err := os.ReadFile(filepath.Join(k7Root(e), f)); err != nil || string(b) != f {
				p = append(p, f+" not reachable from the copy")
			}
		}
		return "", p, nil
	}}
	ctl := K7NegativeControls("evidence/K7", []Criterion{reach})
	if len(ctl) != 1 || ctl[0].Error != "" || len(ctl[0].Failed) != 0 {
		t.Fatalf("control %+v: want no error and no failed criterion", ctl)
	}
}

// TestK7LastTagCheck: K7.7 reads the latest tag-check attempt, as it reads
// the latest CI attempt; a record whose name only starts with tag-check is
// not an attempt.
func TestK7LastTagCheck(t *testing.T) {
	rec := func(names ...string) *Evidence {
		e := &Evidence{}
		for _, n := range names {
			e.Records = append(e.Records, Record{Name: n})
		}
		return e
	}
	cases := []struct {
		e    *Evidence
		want string
	}{
		{rec(), "tag-check"},
		{rec("ci", "tag-check"), "tag-check"},
		{rec("ci", "tag-check", "ci-2", "tag-check-2"), "tag-check-2"},
		{rec("tag-check", "tag-check-2", "tag-check-notes", "tag-check-2x"), "tag-check-2"},
	}
	for _, c := range cases {
		if got := lastTagCheck(c.e); got != c.want {
			t.Errorf("lastTagCheck(%v) = %q, want %q", c.e.Records, got, c.want)
		}
	}
}

// TestK7SabotageK72ClaimHash: K7.2's control forges the campaign page so
// that only a reading of its contents can tell — the render record's
// output hash and the hash a claim names for the page (claims.yaml) are
// rewritten to the forged page's; a claim naming another artifact keeps its
// hash.
func TestK7SabotageK72ClaimHash(t *testing.T) {
	root := t.TempDir()
	ev := filepath.Join(root, "evidence", "K7")
	page := []byte(`<script type="application/json">{"cells":[{"n":10}]}</script>`)
	other := sha([]byte("another artifact"))
	stdout := []byte(`{"command":"render","outputs":[{"file":"index.html","sha256":"` + sha(page) + `"}]}` + "\n")
	files := map[string][]byte{
		"evidence/K7/report/k7/index.html":  page,
		"evidence/K7/logs/render-k7.stdout": stdout,
		"evidence/K7/logs/render-k7.stderr": {},
		"claims.yaml": []byte("claims:\n  - id: CLM-0021\n    artifact: \"evidence/K7/report/k7/index.html\"\n    sha256: \"" + sha(page) +
			"\"\n  - id: CLM-0025\n    artifact: \"evidence/K7/results.json\"\n    sha256: \"" + other + "\"\n"),
		"evidence/K7/commands.jsonl": []byte(fmt.Sprintf(`{"name":"render-k7","argv":["bin/cpreport","render"],"exit_code":0,"stdout":"evidence/K7/logs/render-k7.stdout","stdout_sha256":%q,"stderr":"evidence/K7/logs/render-k7.stderr","stderr_sha256":%q}`+"\n",
			sha(stdout), sha(nil))),
	}
	for f, b := range files {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := sabotageK72(ev); err != nil {
		t.Fatal(err)
	}
	forged, _ := os.ReadFile(filepath.Join(ev, "report", "k7", "index.html"))
	if !bytes.Contains(forged, []byte(`"n":11`)) {
		t.Fatalf("page not forged: %s", forged)
	}
	cy, _ := os.ReadFile(filepath.Join(root, "claims.yaml"))
	if !bytes.Contains(cy, []byte(sha(forged))) || bytes.Contains(cy, []byte(sha(page))) || !bytes.Contains(cy, []byte(other)) {
		t.Fatalf("claims.yaml after the forgery:\n%s", cy)
	}
	if _, out, err := LoadEvidence(ev).Get("render-k7"); err != nil || !bytes.Contains(out, []byte(sha(forged))) {
		t.Fatalf("render record after the forgery: %s %v", out, err)
	}
}
