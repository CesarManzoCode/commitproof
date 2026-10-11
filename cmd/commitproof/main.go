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
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/CesarManzoCode/commitproof/internal/buildinfo"
	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/cluster"
	"github.com/CesarManzoCode/commitproof/internal/cmdlog"
	"github.com/CesarManzoCode/commitproof/internal/host"
	"github.com/CesarManzoCode/commitproof/internal/images"
	"github.com/CesarManzoCode/commitproof/internal/nemesis"
	"github.com/CesarManzoCode/commitproof/internal/observe"
	"github.com/CesarManzoCode/commitproof/internal/plan"
	"github.com/CesarManzoCode/commitproof/internal/runner"
)

const usage = `usage:
  commitproof images build [--mutant M<n>] [--no-cache]
  commitproof images verify
  commitproof cluster up|down|verify --profile <p> [--mutant M<n>]
  commitproof plan --campaign <C> --seed <u64> [--profile <p>]
  commitproof run  --campaign <C> --seed <u64> [--profile <p>] [--mutant M<n>] [--directed D<n>] --out runs/
  (probe, minimize, replay, campaign: later sprints)`

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
	case "probe", "minimize", "replay", "campaign":
		return summary(map[string]any{"command": args[0], "error": "not implemented in K1 (CONTRACT §15: K4)"}, 3)
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
	mutant := fs.String("mutant", "", "mutant M<n> (K4)")
	noCache := fs.Bool("no-cache", false, "build without the builder cache")
	if err := fs.Parse(args[1:]); err != nil {
		return usageErr("images", err.Error())
	}
	cmd := "images " + args[0]
	if *mutant != "" {
		return summary(map[string]any{"command": cmd, "error": "mutant builds are K4"}, 3)
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
		lock, err := b.Build(ctx, *noCache)
		if err != nil {
			return summary(map[string]any{"command": cmd, "error": err.Error()}, 2)
		}
		raw, _ := os.ReadFile(root + "/" + images.LockFile)
		sum := sha256.Sum256(raw)
		ids := map[string]any{}
		imgs, _ := lock["images"].(map[string]any)
		for t, e := range imgs {
			m, _ := e.(map[string]any)
			ids[t] = m["id"]
		}
		return summary(map[string]any{"command": cmd, "no_cache": *noCache, "lock": images.LockFile,
			"lock_sha256": hex.EncodeToString(sum[:]), "images": ids}, 0)
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
		mm, _, err := b.Verify(ctx)
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
	if *mutant != "" {
		return summary(map[string]any{"command": cmd, "error": "mutant clusters are K4"}, 3)
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
	if *mutant != "" {
		return summary(map[string]any{"command": "run", "error": "mutant runs are K4"}, 3)
	}
	seed, err := parseSeed(*seedS)
	if err != nil {
		return usageErr("run", err.Error())
	}
	p, err := plan.Generate(*campaign, seed, *profile, *directed)
	if err != nil {
		return usageErr("run", err.Error())
	}
	root, err := host.RepoRoot()
	if err != nil {
		return summary(map[string]any{"command": "run", "error": err.Error()}, 3)
	}
	return summary(cluster.RunPlan(ctx, root, *profile, *mutant, p, *out))
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
	if err := fs.Parse(args[1:]); err != nil {
		return usageErr("__runner", err.Error())
	}
	switch args[0] {
	case "healthy":
		evals := 0
		h, n := runner.WaitHealthy(ctx, msDuration(*deadlineMS), func(h runner.Health) {
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
