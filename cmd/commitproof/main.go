// Command commitproof builds and locks images, runs the cluster lifecycle,
// generates plans and executes runs (CONTRACT §5.3, §5.6).
//
// Exit codes: 0 success, 1 verdict FAIL, 2 INVALID/UNKNOWN, 3 usage or
// prerequisite error. Every command prints one JSON summary line last.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/CesarManzoCode/commitproof/internal/buildinfo"
	"github.com/CesarManzoCode/commitproof/internal/campaign"
	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/cluster"
	"github.com/CesarManzoCode/commitproof/internal/cmdlog"
	"github.com/CesarManzoCode/commitproof/internal/host"
	"github.com/CesarManzoCode/commitproof/internal/images"
	"github.com/CesarManzoCode/commitproof/internal/nemesis"
	"github.com/CesarManzoCode/commitproof/internal/observe"
	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/plan"
	"github.com/CesarManzoCode/commitproof/internal/runner"
)

const usage = `usage:
  commitproof images build [--mutant M<n>] [--no-cache]
  commitproof images verify
  commitproof cluster up|down|verify --profile <p> [--mutant M<n>]
  commitproof plan --campaign <C> --seed <u64> [--profile <p>]
  commitproof run  --campaign <C> --seed <u64> [--profile <p>] [--mutant M<n>] [--directed D<n>] --out runs/
  commitproof probe --suite claims [--mutant M<n>] [--seed <u64>] --out runs/
  commitproof minimize <run-dir> --out runs/ [--cpcheck bin/cpcheck]
  commitproof replay <run-dir> --times <k> --out runs/ [--cpcheck bin/cpcheck]
  commitproof campaign --matrix <file> --out runs/ [--cpcheck bin/cpcheck]`

// summary prints the final JSON line and returns code.
func summary(m map[string]any, code int) int {
	m["exit_code"] = code
	b, err := canonjson.Marshal(m)
	if err != nil {
		b = []byte(fmt.Sprintf(`{"command":"error","error":%q,"exit_code":2}`, err.Error()))
		code = 2
	}
	fmt.Fprintln(os.Stdout, string(b))
	return code
}

func usageErr(cmd, msg string) int {
	fmt.Fprintln(os.Stderr, msg)
	fmt.Fprintln(os.Stderr, usage)
	return summary(map[string]any{"command": cmd, "error": msg}, 3)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(dispatch(ctx, os.Args[1:]))
}

func dispatch(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return usageErr("commitproof", "missing command")
	}
	switch args[0] {
	case "images":
		return imagesCmd(ctx, args[1:])
	case "cluster":
		return clusterCmd(ctx, args[1:])
	case "plan":
		return planCmd(args[1:])
	case "run":
		return runCmd(ctx, args[1:])
	case "probe":
		return probeSuiteCmd(ctx, args[1:])
	case "minimize", "replay", "campaign":
		return driverCmd(ctx, args[0], args[1:])
	case "__runner":
		return runnerCmd(ctx, args[1:])
	case "__init":
		return initCmd(args[1:])
	case "__memsample":
		return memSampleCmd(ctx, args[1:])
	case "__probe":
		return probeCmd(ctx, args[1:])
	case "version", "--version":
		return summary(map[string]any{"command": "version", "commit": buildinfo.Commit, "dirty": buildinfo.Dirty,
			"sabotage": strsAny(nemesis.ActiveSabotage())}, 0)
	}
	return usageErr("commitproof", "unknown command "+args[0])
}

// hostRecorder records host commands of non-run commands under
// $CP_CMDLOG_DIR when set (evidence collection), else in a temp directory.
func hostRecorder(name string) (*cmdlog.Recorder, func(), error) {
	dir := os.Getenv("CP_CMDLOG_DIR")
	cleanup := func() {}
	if dir == "" {
		d, err := os.MkdirTemp("", "commitproof-"+name+"-")
		if err != nil {
			return nil, nil, err
		}
		dir = d
		cleanup = func() { os.RemoveAll(d) }
	}
	// Several commands may share one CP_CMDLOG_DIR: the pid keeps output
	// file names unique.
	rec, err := cmdlog.New(dir+"/commands.jsonl", dir, dir+"/cmd", fmt.Sprintf("%s-%d-", name, os.Getpid()))
	return rec, cleanup, err
}

func imagesCmd(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return usageErr("images", "missing images subcommand")
	}
	fs := flag.NewFlagSet("images "+args[0], flag.ContinueOnError)
	mutant := fs.String("mutant", "", "mutant M<n>: build commitproof/multigres:M<n> from the pinned commit + mutants/M<n>.patch")
	profile := fs.String("profile", "", "profile whose images to build (h1)")
	noCache := fs.Bool("no-cache", false, "build without the builder cache")
	if err := fs.Parse(args[1:]); err != nil {
		return usageErr("images", err.Error())
	}
	cmd := "images " + args[0]
	variant := ""
	switch {
	case *mutant != "" && *profile != "":
		return usageErr(cmd, "--mutant and --profile are exclusive")
	case *mutant != "":
		if !images.IsMutant(*mutant) {
			return usageErr(cmd, "--mutant wants M<n>")
		}
		variant = *mutant
	case *profile == images.VariantH1:
		variant = images.VariantH1
	case *profile != "" && *profile != "baseline":
		return usageErr(cmd, "--profile: only h1 has its own images")
	}
	root, err := host.RepoRoot()
	if err != nil {
		return summary(map[string]any{"command": cmd, "error": err.Error()}, 3)
	}
	rec, cleanup, err := hostRecorder("images")
	if err != nil {
		return summary(map[string]any{"command": cmd, "error": err.Error()}, 2)
	}
	defer cleanup()
	b := &images.Builder{Root: root, Rec: rec}
	switch args[0] {
	case "build":
		var lock map[string]any
		if variant != "" {
			lock, err = b.BuildVariant(ctx, variant, *noCache)
		} else {
			lock, err = b.Build(ctx, *noCache)
		}
		if err != nil {
			return summary(map[string]any{"command": cmd, "variant": variant, "error": err.Error()}, 2)
		}
		raw, _ := os.ReadFile(root + "/" + images.LockFile)
		sum := sha256.Sum256(raw)
		ids := map[string]any{}
		imgs, _ := lock["images"].(map[string]any)
		for t, e := range imgs {
			m, _ := e.(map[string]any)
			ids[t] = m["id"]
		}
		out := map[string]any{"command": cmd, "no_cache": *noCache, "lock": images.LockFile,
			"lock_sha256": hex.EncodeToString(sum[:]), "images": ids, "variant": variant}
		if vs, ok := lock["variants"].(map[string]any); ok && variant != "" {
			out["variant_record"] = vs[variant]
		}
		return summary(out, 0)
	case "sabotage-runner":
		// Hidden (K3.2, K3.3): a binary built with nemesis sabotage tags
		// builds the runner image carrying the same tags.
		tags := nemesis.ActiveSabotage()
		if len(tags) == 0 {
			return summary(map[string]any{"command": cmd, "error": "this binary has no cp_sabotage_* tag"}, 3)
		}
		id, err := b.BuildSabotageRunner(ctx, tags)
		if err != nil {
			return summary(map[string]any{"command": cmd, "error": err.Error()}, 2)
		}
		return summary(map[string]any{"command": cmd, "image": "commitproof/runner:" + images.RunnerTag(tags), "id": id,
			"sabotage": strsAny(tags)}, 0)
	case "verify":
		mm, _, err := b.Verify(ctx, "")
		if err != nil {
			return summary(map[string]any{"command": cmd, "error": err.Error()}, 3)
		}
		list := []any{}
		for _, m := range mm {
			list = append(list, map[string]any{"tag": m.Tag, "lock": m.Lock, "local": m.Local})
		}
		code := 0
		if len(mm) > 0 {
			code = 1
		}
		return summary(map[string]any{"command": cmd, "mismatches": list, "match": len(mm) == 0}, code)
	}
	return usageErr("images", "unknown images subcommand "+args[0])
}

func clusterCmd(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return usageErr("cluster", "missing cluster subcommand")
	}
	fs := flag.NewFlagSet("cluster "+args[0], flag.ContinueOnError)
	profile := fs.String("profile", "", "profile (CONTRACT §5.5)")
	mutant := fs.String("mutant", "", "mutant M<n> (K4)")
	if err := fs.Parse(args[1:]); err != nil {
		return usageErr("cluster", err.Error())
	}
	cmd := "cluster " + args[0]
	if *profile == "" {
		return usageErr(cmd, "--profile is required")
	}
	root, err := host.RepoRoot()
	if err != nil {
		return summary(map[string]any{"command": cmd, "error": err.Error()}, 3)
	}
	rec, cleanup, err := hostRecorder("cluster")
	if err != nil {
		return summary(map[string]any{"command": cmd, "error": err.Error()}, 2)
	}
	defer cleanup()
	c, err := cluster.New(root, *profile, rec)
	if err == nil {
		err = c.SetMutant(*mutant)
	}
	if err != nil {
		return summary(map[string]any{"command": cmd, "error": err.Error()}, 3)
	}
	switch args[0] {
	case "up":
		r := c.Up(ctx)
		return summary(r.Summary, r.ExitCode)
	case "down":
		return summary(c.Down(ctx))
	case "verify":
		return summary(c.Verify(ctx))
	}
	return usageErr("cluster", "unknown cluster subcommand "+args[0])
}

func parseSeed(s string) (uint64, error) {
	if s == "" {
		return 0, fmt.Errorf("--seed is required")
	}
	return strconv.ParseUint(s, 10, 64)
}

func planCmd(args []string) int {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	campaign := fs.String("campaign", "", "campaign")
	seedS := fs.String("seed", "", "seed (u64)")
	profile := fs.String("profile", "baseline", "profile")
	if err := fs.Parse(args); err != nil {
		return usageErr("plan", err.Error())
	}
	seed, err := parseSeed(*seedS)
	if err != nil {
		return usageErr("plan", err.Error())
	}
	p, err := plan.Generate(*campaign, seed, *profile, "")
	if err != nil {
		return usageErr("plan", err.Error())
	}
	b, err := p.Canonical()
	if err != nil {
		return summary(map[string]any{"command": "plan", "error": err.Error()}, 2)
	}
	fmt.Fprintln(os.Stdout, string(b))
	sum := sha256.Sum256(b)
	return summary(map[string]any{"command": "plan", "campaign": *campaign, "seed": seed, "profile": *profile,
		"plan_sha256": hex.EncodeToString(sum[:]), "bytes": len(b)}, 0)
}

func runCmd(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	campaign := fs.String("campaign", "", "campaign")
	seedS := fs.String("seed", "", "seed (u64)")
	profile := fs.String("profile", "baseline", "profile")
	mutant := fs.String("mutant", "", "mutant M<n> (K4)")
	directed := fs.String("directed", "", "directed schedule D<n> (K4)")
	out := fs.String("out", "", "output directory for run directories")
	if err := fs.Parse(args); err != nil {
		return usageErr("run", err.Error())
	}
	if *out == "" {
		return usageErr("run", "--out is required")
	}
	seed, err := parseSeed(*seedS)
	if err != nil {
		return usageErr("run", err.Error())
	}
	if *campaign == "" {
		*campaign = *directed // `run --directed D1`: the directed schedule is the campaign
	}
	p, err := plan.Generate(*campaign, seed, *profile, *directed)
	if err != nil {
		return usageErr("run", err.Error())
	}
	root, err := host.RepoRoot()
	if err != nil {
		return summary(map[string]any{"command": "run", "error": err.Error()}, 3)
	}
	return summary(cluster.RunPlan(ctx, root, *profile, cluster.RunOpts{Mutant: *mutant}, p, *out))
}

// runnerCmd is executed inside the runner container (ADR-0018).
func runnerCmd(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return usageErr("__runner", "missing runner subcommand")
	}
	fs := flag.NewFlagSet("__runner "+args[0], flag.ContinueOnError)
	deadlineMS := fs.Int64("deadline-ms", 0, "HEALTHY deadline")
	runDir := fs.String("run-dir", "", "run directory")
	nemImage := fs.String("nemesis-image", "", "image ID of the nemesis helper (nem-runner)")
	netPrefix := fs.String("net-prefix", "", "cpnet /24 prefix (a.b.c)")
	profile := fs.String("profile", "baseline", "profile (HEALTHY predicate variant)")
	if err := fs.Parse(args[1:]); err != nil {
		return usageErr("__runner", err.Error())
	}
	switch args[0] {
	case "healthy":
		evals := 0
		h, n := runner.WaitHealthy(ctx, msDuration(*deadlineMS), *profile, func(h runner.Health) {
			evals++
			fmt.Fprintf(os.Stderr, "healthy evaluation %d: healthy=%v a=%q b=%q c=%v d=%q\n", evals, h.Healthy, h.A, h.B, h.C, h.D)
		})
		code := 0
		if !h.Healthy {
			code = 1
		}
		return summary(map[string]any{"command": "runner healthy", "healthy": h.Healthy, "evaluations": n,
			"version": h.Version, "predicate": h.Map()}, code)
	case "run":
		if *runDir == "" {
			return usageErr("__runner run", "--run-dir is required")
		}
		status, reason, err := runner.Run(ctx, runner.Config{RunDir: *runDir, NemesisImage: *nemImage, NetPrefix: *netPrefix})
		m := map[string]any{"command": "runner run", "status": status, "reason": reason}
		if err != nil {
			m["error"] = err.Error()
			return summary(m, 2)
		}
		if status != runner.StatusComplete {
			return summary(m, 2)
		}
		return summary(m, 0)
	}
	return usageErr("__runner", "unknown runner subcommand "+args[0])
}

func strsAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// probeCmd is the runner's status probe (inside the runner container): the
// Status RPC of each listed pooler, printed as the raw probe output the
// nemesis records (CONTRACT §8 role resolution and F6/F5 predicates).
func probeCmd(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] != "status" {
		return usageErr("__probe", "usage: __probe status --nodes n1,n2 [--timeout-ms N]")
	}
	fs := flag.NewFlagSet("__probe status", flag.ContinueOnError)
	nodes := fs.String("nodes", "", "comma-separated nodes")
	timeoutMS := fs.Int64("timeout-ms", 3000, "per-RPC timeout")
	if err := fs.Parse(args[1:]); err != nil || *nodes == "" {
		return usageErr("__probe", "--nodes is required")
	}
	list := strings.Split(*nodes, ",")
	sts := make([]any, len(list))
	var wg sync.WaitGroup
	for i, n := range list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sts[i] = observe.StatusOnce(ctx, n, msDuration(*timeoutMS)).Map()
		}()
	}
	wg.Wait()
	return summary(map[string]any{"command": "probe status", "statuses": sts}, 0)
}

type multiFlag []string

func (m *multiFlag) String() string     { return fmt.Sprint(*m) }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// initCmd is the cluster-init job (deploy/compose.yaml).
func initCmd(args []string) int {
	fs := flag.NewFlagSet("__init", flag.ContinueOnError)
	certs := fs.String("certs", "", "certificate directory")
	var vols multiFlag
	fs.Var(&vols, "volume", "shared volume to prepare (repeatable)")
	if err := fs.Parse(args); err != nil {
		return usageErr("__init", err.Error())
	}
	for _, v := range vols {
		if err := runner.InitVolume(v); err != nil {
			return summary(map[string]any{"command": "init", "error": err.Error()}, 2)
		}
	}
	if *certs != "" {
		if err := runner.InitCerts(*certs); err != nil {
			return summary(map[string]any{"command": "init", "error": err.Error()}, 2)
		}
	}
	return summary(map[string]any{"command": "init", "certs": *certs, "volumes": len(vols)}, 0)
}

// probeSuiteCmd runs the claims probe suite as a run (CONTRACT §15 K4: P1,
// P2 recorded as runs; the verdict is cpcheck's probe checker).
func probeSuiteCmd(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	suite := fs.String("suite", "", "probe suite (claims)")
	mutant := fs.String("mutant", "", "mutant M<n>")
	seedS := fs.String("seed", "1", "seed (identifies the repetition; probes draw nothing from it)")
	out := fs.String("out", "", "output directory for run directories")
	if err := fs.Parse(args); err != nil {
		return usageErr("probe", err.Error())
	}
	if *suite != plan.ProbeSuite || *out == "" {
		return usageErr("probe", "want --suite "+plan.ProbeSuite+" and --out")
	}
	seed, err := parseSeed(*seedS)
	if err != nil {
		return usageErr("probe", err.Error())
	}
	p, err := plan.Generate(plan.ProbeSuite, seed, "baseline", "")
	if err != nil {
		return usageErr("probe", err.Error())
	}
	root, err := host.RepoRoot()
	if err != nil {
		return summary(map[string]any{"command": "probe", "error": err.Error()}, 3)
	}
	return summary(cluster.RunPlan(ctx, root, "baseline", cluster.RunOpts{Mutant: *mutant, Command: "probe"}, p, *out))
}

// driverCmd is `minimize`, `replay` and `campaign` (internal/campaign).
func driverCmd(ctx context.Context, name string, args []string) int {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	out := fs.String("out", "", "runs directory")
	cpcheck := fs.String("cpcheck", "", "cpcheck binary (default: next to this binary)")
	times := fs.Int("times", 0, "replays (replay)")
	matrix := fs.String("matrix", "", "matrix file (campaign)")
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return usageErr(name, err.Error())
		}
		args = fs.Args()
		if len(args) > 0 {
			pos, args = append(pos, args[0]), args[1:]
		}
	}
	if *out == "" {
		return usageErr(name, "--out is required")
	}
	if *cpcheck == "" {
		exe, err := os.Executable()
		if err != nil {
			return summary(map[string]any{"command": name, "error": err.Error()}, 3)
		}
		*cpcheck = filepath.Join(filepath.Dir(exe), "cpcheck")
	}
	if st, err := os.Stat(*cpcheck); err != nil || st.IsDir() {
		return usageErr(name, "cpcheck binary not found: "+*cpcheck)
	}
	root, err := host.RepoRoot()
	if err != nil {
		return summary(map[string]any{"command": name, "error": err.Error()}, 3)
	}
	d := &campaign.Driver{Root: root, Out: filepath.Clean(*out), CPCheck: *cpcheck, Log: os.Stdout}
	unlock, err := d.Lock()
	if err != nil {
		return summary(map[string]any{"command": name, "error": err.Error()}, 3)
	}
	defer unlock()
	sum := map[string]any{"command": name, "out": filepath.ToSlash(d.Out)}
	switch name {
	case "minimize", "replay":
		if len(pos) != 1 {
			return usageErr(name, "want exactly one run directory")
		}
		r, err := d.Find(pos[0])
		if err == nil {
			err = d.Check(ctx, r)
		}
		if err != nil {
			sum["error"] = err.Error()
			return summary(sum, 3)
		}
		if name == "minimize" {
			m, err := d.Minimize(ctx, r)
			if err != nil {
				sum["error"] = err.Error()
				return summary(sum, 2)
			}
			sum["original"], sum["target_class"], sum["minimized_run"] = r.ID, m.Class, m.Run.ID
			sum["minimized_run_dir"] = filepath.ToSlash(m.Run.Dir)
			sum["candidates"] = int64(len(m.Candidates))
			return summary(sum, 0)
		}
		if *times != params.Replays {
			return usageErr(name, fmt.Sprintf("--times %d: CONTRACT P13 says %d", *times, params.Replays))
		}
		class := campaign.TargetClass(r)
		reps, hits, err := d.Replay(ctx, r, *times, class)
		rl := []any{}
		for _, x := range reps {
			rl = append(rl, x.ID)
		}
		sum["of"], sum["class"], sum["replays"], sum["replays_with_class"] = r.ID, class, rl, int64(hits)
		sum["reproducibility"] = campaign.Reproducibility(hits)
		if err != nil {
			sum["error"] = err.Error()
			return summary(sum, 2)
		}
		return summary(sum, 0)
	}
	// campaign
	if *matrix == "" || len(pos) != 0 {
		return usageErr(name, "want --matrix <file> and no positional arguments")
	}
	m, err := campaign.ReadMatrix(*matrix)
	if err != nil {
		sum["error"] = err.Error()
		return summary(sum, 3)
	}
	cells, complete, err := d.RunMatrix(ctx, m)
	sum["matrix"], sum["cells"], sum["complete"] = m.Name, int64(len(cells)), complete
	verdicts := map[string]any{}
	failed := false
	for _, c := range cells {
		v := "INVALID"
		if c.Valid {
			v = c.Run.Verdict
		}
		verdicts[v] = toInt64(verdicts[v]) + 1
		failed = failed || v == "FAIL"
	}
	sum["verdicts"] = verdicts
	switch {
	case err != nil:
		sum["error"] = err.Error()
		return summary(sum, 2)
	case !complete:
		return summary(sum, 2)
	case failed:
		return summary(sum, 1)
	}
	return summary(sum, 0)
}

func toInt64(v any) int64 {
	n, _ := v.(int64)
	return n
}
