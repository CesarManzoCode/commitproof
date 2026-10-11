package gate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Criterion is one gate criterion with its negative control.
type Criterion struct {
	ID       string
	Name     string
	Eval     func(e *Evidence) (evidence string, problems, details []string)
	Sabotage string
	Apply    func(dir string) error
}

var (
	hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)
	dig   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// K1Binaries are the six binaries of K1.2 (image tag, path).
var K1Binaries = [][2]string{
	{"commitproof/multigres:real", "/multigres/bin/multigres"},
	{"commitproof/multigres:real", "/multigres/bin/multiadmin"},
	{"commitproof/multigres:real", "/multigres/bin/multigateway"},
	{"commitproof/multigres:real", "/multigres/bin/multiorch"},
	{"commitproof/multigres:real", "/multigres/bin/multipooler"},
	{"commitproof/pgctld:real", "/usr/local/bin/pgctld"},
}

func readJSON(p string) (map[string]any, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
	}
	return m, nil
}

func obj(m map[string]any, k string) map[string]any { x, _ := m[k].(map[string]any); return x }
func str(m map[string]any, k string) string         { x, _ := m[k].(string); return x }

func exitOK(e *Evidence, name string, words ...string) (Record, []byte, []string) {
	r, out, err := e.Get(name)
	if err != nil {
		return r, nil, []string{err.Error()}
	}
	var p []string
	if r.ExitCode != 0 {
		p = append(p, fmt.Sprintf("%s exited %d", name, r.ExitCode))
	}
	if !argvHas(r, words...) {
		p = append(p, fmt.Sprintf("%s argv %q lacks %q", name, r.Argv, words))
	}
	return r, out, p
}

// K1 returns the eight criteria of CONTRACT §15 K1 with their sabotages.
func K1() []Criterion {
	return []Criterion{
		{ID: "K1.1", Name: "Lock", Eval: evalLock,
			Sabotage: "remove the base_images field from images.lock.build1.json",
			Apply: func(dir string) error {
				return editJSON(filepath.Join(dir, "images.lock.build1.json"), func(m map[string]any) { delete(m, "base_images") })
			}},
		{ID: "K1.2", Name: "Binary reproducibility", Eval: evalRepro,
			Sabotage: "alter the multigateway sha256 in images.lock.build2.json (second-build record)",
			Apply: func(dir string) error {
				return editJSON(filepath.Join(dir, "images.lock.build2.json"), func(m map[string]any) {
					bins := obj(obj(obj(m, "images"), "commitproof/multigres:real"), "binaries")
					bins["/multigres/bin/multigateway"] = strings.Repeat("0", 64)
				})
			}},
		{ID: "K1.3", Name: "Derivation", Eval: evalDerivation,
			Sabotage: "add a RUN line to the derived build/Dockerfile copy",
			Apply: func(dir string) error {
				p := filepath.Join(dir, "derivation", "build", "Dockerfile")
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				i := bytes.IndexByte(b, '\n')
				out := append(append(append([]byte{}, b[:i+1]...), []byte("RUN echo sabotage\n")...), b[i+1:]...)
				return os.WriteFile(p, out, 0o644)
			}},
		{ID: "K1.4", Name: "Up/down from zero", Eval: evalUpDown,
			Sabotage: "add a leftover volume to the second teardown record (hashes kept consistent)",
			Apply: func(dir string) error {
				return editSummary(dir, "updown-2-down", func(m map[string]any) {
					obj(m, "leftover")["volumes"] = []any{"commitproof_sabotage"}
				})
			}},
		{ID: "K1.5", Name: "Memory", Eval: evalMemory,
			Sabotage: "set one recorded sample's first container to 7GiB (above P10)",
			Apply:    sabotageMemory},
		{ID: "K1.6", Name: "Plan determinism", Eval: evalPlan,
			Sabotage: "alter the plan_sha256 recorded by plan-42-b (hashes kept consistent)",
			Apply: func(dir string) error {
				return editSummary(dir, "plan-42-b", func(m map[string]any) {
					m["plan_sha256"] = strings.Repeat("f", 64)
				})
			}},
		{ID: "K1.7", Name: "History integrity", Eval: evalHistory,
			Sabotage: "flip one byte in the copied history.jsonl of the smoke run",
			Apply:    sabotageHistory},
		{ID: "K1.8", Name: "CI-equivalent", Eval: evalCI,
			Sabotage: "replace the recorded exit code of `make ci-unit ci-cluster-smoke` with 1",
			Apply: func(dir string) error {
				return editRecords(dir, func(r map[string]any) {
					if r["name"] == "ci" {
						r["exit_code"] = 1
					}
				})
			}},
	}
}

func evalLock(e *Evidence) (string, []string, []string) {
	const f = "images.lock.build1.json"
	_, _, p := exitOK(e, "images-build-1", "images", "build")
	if r, _, err := e.Get("images-build-1"); err == nil && argvHas(r, "--no-cache") {
		p = append(p, "images-build-1 used --no-cache")
	}
	lock, err := readJSON(filepath.Join(e.Dir, f))
	if err != nil {
		return f, append(p, err.Error()), nil
	}
	for _, k := range []string{"lock_version", "contract_version", "multigres", "commitproof", "dockerfiles", "base_images", "images", "mutant", "patch_sha256"} {
		if _, ok := lock[k]; !ok {
			p = append(p, "lock lacks field "+k)
		}
	}
	mg := obj(lock, "multigres")
	if str(mg, "commit") != params.MultigresCommit || str(mg, "commit_date") != params.MultigresCommitDate {
		p = append(p, fmt.Sprintf("multigres source %q %q != pinned", str(mg, "commit"), str(mg, "commit_date")))
	}
	if !hex40.MatchString(str(obj(lock, "commitproof"), "commit")) {
		p = append(p, "commitproof.commit missing")
	}
	df := obj(lock, "dockerfiles")
	for _, d := range []string{"build/Dockerfile", "build/Dockerfile.pgctld", "build/Dockerfile.runner"} {
		x := obj(df, d)
		if !hex64.MatchString(str(x, "sha256")) {
			p = append(p, "dockerfiles["+d+"].sha256 missing")
		}
		if d != "build/Dockerfile.runner" && !hex40.MatchString(str(x, "upstream_blob")) {
			p = append(p, "dockerfiles["+d+"].upstream_blob missing")
		}
	}
	bases := obj(lock, "base_images")
	resolved := map[string]string{}
	if _, out, err := e.Last("resolve-digests"); err != nil {
		p = append(p, err.Error())
	} else {
		var res struct {
			Images []struct {
				Ref    string `json:"ref"`
				Digest string `json:"resolved_digest"`
			} `json:"images"`
		}
		if err := json.Unmarshal(bytes.TrimSpace(out), &res); err != nil {
			p = append(p, "digest resolution output: "+err.Error())
		}
		for _, i := range res.Images {
			resolved[i.Ref] = i.Digest
		}
	}
	for _, ref := range []string{"golang:1.26.9-alpine", "debian:bookworm-slim", "postgres:17.7", "alpine:3.24.2", "quay.io/coreos/etcd:v3.6.4"} {
		d := str(bases, ref)
		switch {
		case !dig.MatchString(d):
			p = append(p, fmt.Sprintf("base image %s digest %q not resolved", ref, d))
		case resolved[ref] != d:
			p = append(p, fmt.Sprintf("base image %s pinned %s, re-resolved %s", ref, d, resolved[ref]))
		}
	}
	for ref := range bases {
		if strings.Contains(ref, "latest") {
			p = append(p, "base image uses latest: "+ref)
		}
	}
	imgs := obj(lock, "images")
	for _, tag := range []string{"commitproof/multigres:real", "commitproof/pgctld:real", "commitproof/runner:real"} {
		x := obj(imgs, tag)
		if !dig.MatchString(str(x, "id")) {
			p = append(p, tag+": local image id missing")
		}
		bins := obj(x, "binaries")
		if len(bins) == 0 {
			p = append(p, tag+": no binary hashes")
		}
		for path, v := range bins {
			if s, _ := v.(string); !hex64.MatchString(s) {
				p = append(p, fmt.Sprintf("%s %s: bad sha256", tag, path))
			}
		}
		if tag != "commitproof/runner:real" && strings.TrimSpace(str(x, "pgbackrest_version")) == "" {
			p = append(p, tag+": pgbackrest version output missing")
		}
	}
	for _, b := range K1Binaries {
		if _, ok := obj(obj(imgs, b[0]), "binaries")[b[1]]; !ok {
			p = append(p, fmt.Sprintf("lock lacks %s:%s", b[0], b[1]))
		}
	}
	return f, p, []string{fmt.Sprintf("%d base images, %d images", len(bases), len(imgs))}
}

func evalRepro(e *Evidence) (string, []string, []string) {
	const f = "images.lock.build2.json"
	b1, _, p := exitOK(e, "images-build-1", "images", "build")
	pr, _, p2 := exitOK(e, "builder-prune", "docker", "builder", "prune", "-af")
	b2, _, p3 := exitOK(e, "images-build-2", "images", "build", "--no-cache")
	p = append(append(p, p2...), p3...)
	if len(p) == 0 && !(b1.Line < pr.Line && pr.Line < b2.Line) {
		p = append(p, "order must be images-build-1 < builder-prune < images-build-2")
	}
	l1, err1 := readJSON(filepath.Join(e.Dir, "images.lock.build1.json"))
	l2, err2 := readJSON(filepath.Join(e.Dir, f))
	if err1 != nil || err2 != nil {
		return f, append(p, fmt.Sprint(err1, err2)), nil
	}
	if nc, _ := obj(l2, "build")["no_cache"].(bool); !nc {
		p = append(p, "second lock does not record a no-cache build")
	}
	var details []string
	for _, b := range K1Binaries {
		h1 := str(obj(obj(obj(l1, "images"), b[0]), "binaries"), b[1])
		h2 := str(obj(obj(obj(l2, "images"), b[0]), "binaries"), b[1])
		if !hex64.MatchString(h1) || h1 != h2 {
			p = append(p, fmt.Sprintf("%s differs: build1 %s build2 %s", b[1], h1, h2))
		}
		details = append(details, fmt.Sprintf("%s %s", filepath.Base(b[1]), h2))
	}
	return f, p, details
}

func evalDerivation(e *Evidence) (string, []string, []string) {
	const f = "derivation/derivation.json"
	_, _, p := exitOK(e, "check-dockerfile-derivation", "make", "check-dockerfile-derivation")
	d, err := readJSON(filepath.Join(e.Dir, f))
	if err != nil {
		return f, append(p, err.Error()), nil
	}
	if str(d, "pin") != params.MultigresCommit {
		p = append(p, "derivation pin != pinned commit")
	}
	files, _ := d["files"].([]any)
	if len(files) != 2 {
		p = append(p, fmt.Sprintf("%d derived Dockerfiles, want 2", len(files)))
	}
	var details []string
	for _, x := range files {
		m, _ := x.(map[string]any)
		name := str(m, "upstream")
		up, err1 := os.ReadFile(filepath.Join(e.Dir, "derivation", "upstream", name))
		der, err2 := os.ReadFile(filepath.Join(e.Dir, "derivation", "build", name))
		if err1 != nil || err2 != nil {
			p = append(p, fmt.Sprint(err1, err2))
			continue
		}
		if GitBlobID(up) != str(m, "upstream_blob") {
			p = append(p, name+": upstream copy is not the pinned blob "+str(m, "upstream_blob"))
		}
		r := CheckDerivation(up, der)
		for _, v := range r.Violations {
			p = append(p, name+": "+v)
		}
		details = append(details, fmt.Sprintf("%s: %d FROM lines changed", name, len(r.Changed)))
	}
	return f, p, details
}

func evalUpDown(e *Evidence) (string, []string, []string) {
	var p, details []string
	prevDown := -1
	for k := 1; k <= 3; k++ {
		up, upOut, p1 := exitOK(e, fmt.Sprintf("updown-%d-up", k), "cluster", "up", "--profile", "baseline")
		ve, veOut, p2 := exitOK(e, fmt.Sprintf("updown-%d-verify", k), "cluster", "verify")
		dn, dnOut, p3 := exitOK(e, fmt.Sprintf("updown-%d-down", k), "cluster", "down")
		p = append(append(append(p, p1...), p2...), p3...)
		if len(p1)+len(p2)+len(p3) > 0 {
			continue
		}
		if !(prevDown < up.Line && up.Line < ve.Line && ve.Line < dn.Line) {
			p = append(p, fmt.Sprintf("attempt %d: records out of order", k))
		}
		prevDown = dn.Line
		us, err1 := LastJSONLine(upOut)
		vs, err2 := LastJSONLine(veOut)
		ds, err3 := LastJSONLine(dnOut)
		if err1 != nil || err2 != nil || err3 != nil {
			p = append(p, fmt.Sprintf("attempt %d: %v %v %v", k, err1, err2, err3))
			continue
		}
		if us["healthy"] != true {
			p = append(p, fmt.Sprintf("attempt %d: up did not reach HEALTHY", k))
		}
		el, _ := strconv.ParseInt(fmt.Sprint(us["elapsed_ms"]), 10, 64)
		if el <= 0 || el > params.HealthyDeadline.Milliseconds() {
			p = append(p, fmt.Sprintf("attempt %d: HEALTHY after %d ms (P09 %d ms)", k, el, params.HealthyDeadline.Milliseconds()))
		}
		if !strings.Contains(str(us, "version"), params.MultigresCommit[:7]) {
			p = append(p, fmt.Sprintf("attempt %d: multigres.version() %q does not report %s", k, str(us, "version"), params.MultigresCommit[:7]))
		}
		for _, kind := range []string{"containers", "volumes", "networks"} {
			if l, _ := obj(us, "preexisting")[kind].([]any); l == nil || len(l) != 0 {
				p = append(p, fmt.Sprintf("attempt %d: up did not start from zero %s (%v)", k, kind, obj(us, "preexisting")[kind]))
			}
			if l, _ := obj(ds, "leftover")[kind].([]any); l == nil || len(l) != 0 {
				p = append(p, fmt.Sprintf("attempt %d: %s left after down: %v", k, kind, obj(ds, "leftover")[kind]))
			}
		}
		if vs["healthy"] != true {
			p = append(p, fmt.Sprintf("attempt %d: verify not HEALTHY", k))
		}
		details = append(details, fmt.Sprintf("attempt %d: HEALTHY in %d ms, version %q", k, el, str(us, "version")))
	}
	ups := 0
	for _, r := range e.Records {
		if strings.HasPrefix(r.Name, "updown-") && argvHas(r, "cluster", "up") {
			ups++
		}
	}
	if ups != 3 {
		p = append(p, fmt.Sprintf("%d cluster up records, want exactly 3 (no retries inside an attempt)", ups))
	}
	return "commands.jsonl", p, details
}

var memRe = regexp.MustCompile(`^\s*([0-9.]+)\s*([KMGTP]?i?B)\s*/`)

// ParseMem parses the used part of docker stats' MemUsage ("12.3MiB / 15GiB").
func ParseMem(s string) (int64, error) {
	m := memRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("unparseable MemUsage %q", s)
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, err
	}
	mult := map[string]float64{"B": 1, "KiB": 1 << 10, "MiB": 1 << 20, "GiB": 1 << 30, "TiB": 1 << 40,
		"kB": 1e3, "KB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12}[m[2]]
	if mult == 0 {
		return 0, fmt.Errorf("unknown unit in %q", s)
	}
	return int64(v * mult), nil
}

func evalMemory(e *Evidence) (string, []string, []string) {
	const f = "memory-samples.jsonl"
	run, _, p := exitOK(e, "smoke-run", "run", "--campaign", "smoke", "--seed", "42")
	start, err1 := time.Parse(time.RFC3339Nano, run.Start)
	end, err2 := time.Parse(time.RFC3339Nano, run.End)
	if len(p) == 0 && (err1 != nil || err2 != nil) {
		p = append(p, "smoke-run start/end unparseable")
	}
	fh, err := os.Open(filepath.Join(e.Dir, f))
	if err != nil {
		return f, append(p, err.Error()), nil
	}
	defer fh.Close()
	type sample struct {
		at  time.Time
		sum int64
	}
	var in []sample
	var peak int64
	perPeak := map[string]int64{}
	bad := 0
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1<<20), 1<<22)
	for sc.Scan() {
		var s struct {
			Wall       string `json:"wall"`
			Error      string `json:"error"`
			Containers []struct {
				Name string `json:"name"`
				Mem  string `json:"mem_usage"`
			} `json:"containers"`
		}
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			return f, append(p, "memory sample: "+err.Error()), nil
		}
		if s.Error != "" || len(s.Containers) == 0 {
			bad++
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, s.Wall)
		if err != nil {
			return f, append(p, "sample wall time: "+err.Error()), nil
		}
		var sum int64
		for _, c := range s.Containers {
			b, err := ParseMem(c.Mem)
			if err != nil {
				return f, append(p, err.Error()), nil
			}
			sum += b
			perPeak[c.Name] = max(perPeak[c.Name], b)
		}
		peak = max(peak, sum)
		if !at.Before(start) && !at.After(end) {
			in = append(in, sample{at, sum})
		}
	}
	if len(in) < int(params.SmokeDuration/time.Second) {
		p = append(p, fmt.Sprintf("%d valid samples during the smoke run, want ≥ %d", len(in), int(params.SmokeDuration/time.Second)))
	}
	for i := 1; i < len(in); i++ {
		if gap := in[i].at.Sub(in[i-1].at); gap > 10*time.Second {
			p = append(p, fmt.Sprintf("sampling gap of %s at %s", gap, in[i].at.Format(time.RFC3339)))
			break
		}
	}
	if peak > params.MemoryBudgetBytes {
		p = append(p, fmt.Sprintf("peak sum %d bytes > P10 %d bytes", peak, int64(params.MemoryBudgetBytes)))
	}
	names := make([]string, 0, len(perPeak))
	for n := range perPeak {
		names = append(names, n)
	}
	sort.Strings(names)
	details := []string{fmt.Sprintf("peak sum %.1f MiB over %d samples in the run window (%d failed samples overall)", float64(peak)/(1<<20), len(in), bad)}
	for _, n := range names {
		details = append(details, fmt.Sprintf("%s peak %.1f MiB", n, float64(perPeak[n])/(1<<20)))
	}
	return f, p, details
}

func evalPlan(e *Evidence) (string, []string, []string) {
	var p, details []string
	hashes := map[string]string{}
	lines := map[string]int{}
	for _, n := range []string{"plan-42-a", "plan-42-b", "plan-42-c", "plan-42-d", "plan-43"} {
		seed := "42"
		if n == "plan-43" {
			seed = "43"
		}
		r, out, pp := exitOK(e, n, "plan", "--campaign", "smoke", "--seed", seed)
		if len(pp) > 0 {
			p = append(p, pp...)
			continue
		}
		lines[n] = r.Line
		parts := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
		if len(parts) != 2 {
			p = append(p, fmt.Sprintf("%s: %d stdout lines, want plan + summary", n, len(parts)))
			continue
		}
		s, err := LastJSONLine(out)
		if err != nil {
			p = append(p, n+": "+err.Error())
			continue
		}
		got := sha(parts[0])
		if str(s, "plan_sha256") != got {
			p = append(p, fmt.Sprintf("%s: recorded plan_sha256 %s != sha256 of the printed plan %s", n, str(s, "plan_sha256"), got))
		}
		hashes[n] = got
		details = append(details, fmt.Sprintf("%s %s", n, got))
	}
	cl, _, p1 := exitOK(e, "go-clean-cache", "go", "clean", "-cache")
	rb, _, p2 := exitOK(e, "rebuild", "make", "build")
	p = append(append(p, p1...), p2...)
	if len(p) > 0 {
		return "commands.jsonl", p, details
	}
	if !(lines["plan-42-c"] < cl.Line && cl.Line < rb.Line && rb.Line < lines["plan-42-d"]) {
		p = append(p, "order must be plan-42-a..c < go clean -cache < make build < plan-42-d")
	}
	h := hashes["plan-42-a"]
	for _, n := range []string{"plan-42-b", "plan-42-c", "plan-42-d"} {
		if hashes[n] != h {
			p = append(p, fmt.Sprintf("%s %s != plan-42-a %s", n, hashes[n], h))
		}
	}
	if hashes["plan-43"] == h {
		p = append(p, "seed 43 produced the seed 42 plan")
	}
	return "commands.jsonl", p, details
}

func smokeRunDir(e *Evidence) (string, map[string]any, []string) {
	_, out, p := exitOK(e, "smoke-run", "run", "--campaign", "smoke", "--seed", "42")
	if len(p) > 0 {
		return "", nil, p
	}
	s, err := LastJSONLine(out)
	if err != nil {
		return "", nil, []string{"smoke-run: " + err.Error()}
	}
	rd := str(s, "run_dir")
	if rd == "" {
		return "", s, []string{"smoke-run summary has no run_dir"}
	}
	return e.Path(rd), s, nil
}

func evalHistory(e *Evidence) (string, []string, []string) {
	dir, s, p := smokeRunDir(e)
	if len(p) > 0 {
		return "runs/", p, nil
	}
	if str(s, "status") != "complete" {
		p = append(p, "smoke run status "+str(s, "status"))
	}
	vr, _, pv := exitOK(e, "verify-seal", "verify-seal")
	p = append(p, pv...)
	if len(pv) == 0 && !strings.Contains(strings.Join(vr.Argv, " "), filepath.Base(dir)) {
		p = append(p, "verify-seal was not run on the smoke run directory")
	}
	rel, _ := filepath.Rel(e.Dir, filepath.Join(dir, history.FileSeal))
	r := history.VerifySeal(dir)
	p = append(p, r.Problems...)
	n := map[string]int{}
	var end map[string]any
	for _, ev := range r.History {
		if ev.Type == history.TypeOK && ev.Client != nil && *ev.Client < params.ClientsTotal && ev.F != "final-read" {
			n[ev.WL]++
		}
		if ev.Type == history.TypeMarker {
			if v, _ := ev.Value.(map[string]any); str(v, "name") == "run-end" {
				end = v
			}
		}
	}
	for _, wl := range []string{"append", "register", "bank", "ledger"} {
		if n[wl] == 0 {
			p = append(p, "no ok "+wl+" operation")
		}
	}
	if str(end, "status") != "complete" {
		p = append(p, fmt.Sprintf("run-end marker status %q", str(end, "status")))
	}
	return rel, p, []string{fmt.Sprintf("seal %s, %d history lines, %d observation lines, ok counts %v",
		r.MerkleRoot, r.HistoryLines, r.ObservationLines, n)}
}

func evalCI(e *Evidence) (string, []string, []string) {
	r, _, p := exitOK(e, "ci", "make", "ci-unit", "ci-cluster-smoke")
	return "commands.jsonl", p, []string{fmt.Sprintf("argv %q exit %d", r.Argv, r.ExitCode)}
}
