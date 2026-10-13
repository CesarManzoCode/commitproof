package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/buildinfo"
	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/cmdlog"
	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/host"
	"github.com/CesarManzoCode/commitproof/internal/images"
	"github.com/CesarManzoCode/commitproof/internal/nemesis"
	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/plan"
)

// runnerTimeout bounds the runner process: workload, in-flight ops, the
// nemesis schedule's worst case, recovery, quiescence and final reads, plus
// one P09 of slack.
func runnerTimeout(p *plan.Plan) time.Duration {
	d := time.Duration(p.DurationMS)*time.Millisecond + params.ClientOpTimeout + plan.NemesisBound(p.Steps) +
		params.RecoveryDeadline + params.Quiescence + params.FinalReadDeadline + 2*params.ClientOpTimeout + params.HealthyDeadline
	// K5's own waits (ADR-0029, ADR-0030), only for plans that have them.
	if p.Publication != "" {
		d += 2*plan.SlotReadyDeadline + plan.CatchUpDeadline
	}
	if p.Profile == "buffered" {
		d += plan.BufferGuardWait
	}
	for _, c := range p.Clients {
		if c.Mode == plan.ModeListener {
			d += plan.ListenerTail
			break
		}
	}
	return d
}

// usesKind reports whether the plan contains a fault of kind.
func usesKind(p *plan.Plan, kind string) bool {
	for _, s := range p.Steps {
		for _, e := range s.Elements {
			if e.Kind == kind {
				return true
			}
		}
	}
	return false
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// RunOpts are the optional parts of a run: the mutant, and provenance
// recorded in the manifest (a replay's original run, a minimization
// candidate's parent run).
type RunOpts struct {
	Mutant     string
	Provenance map[string]any
	Command    string // summary "command" field ("run" if empty)
}

// RunPlan executes one run (CONTRACT §5.4): cluster up from zero, the runner
// writes the history, logs are captured, the cluster is torn down and the
// run directory sealed. Exit code: 0 complete, 2 INVALID, 3 prerequisite.
func RunPlan(ctx context.Context, root, profile string, o RunOpts, p *plan.Plan, outDir string) (map[string]any, int) {
	mutant := o.Mutant
	command := o.Command
	if command == "" {
		command = "run"
	}
	sum := map[string]any{"command": command, "campaign": p.Campaign, "seed": p.Seed, "profile": profile, "mutant": "real"}
	if mutant != "" {
		sum["mutant"] = mutant
	}
	if p.Profile != profile {
		sum["error"] = fmt.Sprintf("plan is for profile %q, run requested on %q", p.Profile, profile)
		return sum, 3
	}
	fail := func(code int, msg string) (map[string]any, int) {
		sum["error"] = msg
		return sum, code
	}
	planBytes, err := p.Canonical()
	if err != nil {
		return fail(2, err.Error())
	}
	ps := sha256.Sum256(planBytes)
	planSHA := hex.EncodeToString(ps[:])
	// Prerequisites before anything is created (CONTRACT §5.6).
	scratch, err := os.MkdirTemp("", "commitproof-preflight-")
	if err != nil {
		return fail(2, err.Error())
	}
	defer os.RemoveAll(scratch)
	pre, err := cmdlog.New(filepath.Join(scratch, "commands.jsonl"), scratch, scratch, "")
	if err != nil {
		return fail(2, err.Error())
	}
	pc, err := New(root, profile, pre)
	if err != nil {
		return fail(3, err.Error())
	}
	if err := pc.SetMutant(mutant); err != nil {
		return fail(3, err.Error())
	}
	if missing := pc.Preflight(ctx); len(missing) > 0 {
		sum["missing"] = missing
		return fail(3, "prerequisite missing: "+strings.Join(missing, "; "))
	}
	// What cluster up would refuse (exit 3) is refused here, before a run
	// directory exists: a refused start is not a run.
	if mm, _, err := (&images.Builder{Root: root, Rec: pre}).Verify(ctx, pc.Variant()); err != nil || len(mm) > 0 {
		return fail(3, fmt.Sprintf("image lock: %v %v (run `commitproof images build`)", err, mm))
	}
	if res, err := pc.Resources(ctx); err != nil || !empty(res) {
		return fail(3, fmt.Sprintf("project resources already exist (another cluster?): %v %v", res, err))
	}
	lock, err := images.ReadLock(root)
	if err != nil {
		return fail(3, "image lock: "+err.Error())
	}
	cp, _ := lock["commitproof"].(map[string]any)
	if lc, _ := cp["commit"].(string); lc != buildinfo.Commit {
		return fail(3, fmt.Sprintf("runner image built from commitproof %s, this binary is %s: run `commitproof images build`", lc, buildinfo.Commit))
	}
	imgs, _ := lock["images"].(map[string]any)
	variants, _ := lock["variants"].(map[string]any)
	var variantRec map[string]any
	if v := pc.Variant(); v != images.VariantReal {
		variantRec, _ = variants[v].(map[string]any)
		if variantRec == nil {
			return fail(3, fmt.Sprintf("images.lock.json has no %s build: run `commitproof images build %s`", v, map[bool]string{true: "--mutant " + v, false: "--profile " + v}[images.IsMutant(v)]))
		}
	}
	nemEntry, _ := imgs["commitproof/nemesis:real"].(map[string]any)
	nemImage, _ := nemEntry["id"].(string)
	if len(p.Steps) > 0 && nemImage == "" {
		return fail(3, "images.lock.json has no commitproof/nemesis:real: run `commitproof images build`")
	}
	prefix, err := NetPrefix()
	if err != nil {
		return fail(3, err.Error())
	}
	// A binary built with a nemesis sabotage tag (K3.2, K3.3) runs the
	// runner image built with the same tags from the same commit; the image
	// labels are checked (fail closed). Such runs are never mutant runs.
	sab := nemesis.ActiveSabotage()
	runnerTag := images.RunnerTag(sab)
	runnerImage := ""
	sum["sabotage"] = []any{}
	if len(sab) > 0 {
		var full []string
		for _, t := range sab {
			full = append(full, "cp_sabotage_"+t)
		}
		b := &images.Builder{Root: root, Rec: pre}
		ref := "commitproof/runner:" + runnerTag
		id, err := b.ImageID(ctx, ref)
		if err != nil {
			return fail(3, fmt.Sprintf("sabotage runner image %s missing: %v", ref, err))
		}
		lc, err1 := b.ImageLabel(ctx, ref, "org.commitproof.commit")
		lt, err2 := b.ImageLabel(ctx, ref, "org.commitproof.build-tags")
		if err1 != nil || err2 != nil || lc != buildinfo.Commit || lt != strings.Join(full, ",") {
			return fail(3, fmt.Sprintf("sabotage runner image %s: commit %q tags %q, want %q %q", ref, lc, lt, buildinfo.Commit, strings.Join(full, ",")))
		}
		runnerImage = id
		sum["sabotage"] = strsAny(full)
	}

	mut := "real"
	if mutant != "" {
		mut = mutant
	}
	start := time.Now()
	id := fmt.Sprintf("%s-%s-%d-%s-%s", host.UTCStamp(start), p.Campaign, p.Seed, mut, planSHA[:8])
	dir, err := filepath.Abs(filepath.Join(outDir, id))
	if err != nil {
		return fail(2, err.Error())
	}
	sum["run_id"] = id
	sum["run_dir"] = filepath.Join(outDir, id)
	if err := os.MkdirAll(filepath.Join(dir, history.DirLogs), 0o755); err != nil {
		return fail(2, err.Error())
	}
	rec, err := cmdlog.New(filepath.Join(dir, history.FileCommands), dir, filepath.Join(dir, history.DirLogs, "cmd"), "host-")
	if err != nil {
		return fail(2, err.Error())
	}
	c, _ := New(root, profile, rec)
	c.RunnerTag = runnerTag
	c.SetMutant(mutant)
	if err := os.WriteFile(filepath.Join(dir, history.FilePlan), append(planBytes, '\n'), 0o644); err != nil {
		return fail(2, err.Error())
	}
	lockRaw, err := os.ReadFile(filepath.Join(root, images.LockFile))
	if err != nil {
		return fail(2, err.Error())
	}
	ls := sha256.Sum256(lockRaw)
	dv := rec.Run(ctx, host.QuickTimeout, "docker-version", []string{"docker", "version", "--format", "{{.Server.Version}}"}, cmdlog.Opts{})
	kernel, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	manifest := map[string]any{
		"contract_version":    params.ContractVersion,
		"run_id":              id,
		"campaign":            p.Campaign,
		"profile":             profile,
		"seed":                p.Seed,
		"mutant":              mut,
		"directed":            p.Directed,
		"images_lock_sha256":  hex.EncodeToString(ls[:]),
		"plan_sha256":         planSHA,
		"commitproof_commit":  buildinfo.Commit,
		"commitproof_dirty":   buildinfo.Dirty,
		"runner_image_commit": cp["commit"],
		"runner_image_dirty":  cp["dirty"],
		"multigres_commit":    params.MultigresCommit,
		"variant":             pc.Variant(),
		"sabotage":            sum["sabotage"],
		"sabotage_runner_id":  runnerImage,
		"host_kernel":         strings.TrimSpace(string(kernel)),
		"docker_version":      strings.TrimSpace(string(dv.Stdout)),
		"start_wall_time":     start.UTC().Format(time.RFC3339),
	}
	if profile == "h1" {
		manifest["multigres_commit"] = params.H1Commit
	}
	if variantRec != nil {
		if ps, ok := variantRec["patch_sha256"]; ok {
			manifest["patch_sha256"] = ps
		}
	}
	for k, v := range o.Provenance {
		manifest[k] = v
	}
	mb, err := canonjson.Marshal(manifest)
	if err != nil {
		return fail(2, err.Error())
	}
	if err := os.WriteFile(filepath.Join(dir, history.FileManifest), append(mb, '\n'), 0o644); err != nil {
		return fail(2, err.Error())
	}

	status, reason := "invalid", ""
	up := c.Up(ctx)
	writeJSON(filepath.Join(dir, history.DirLogs, "cluster-up.json"), up.Summary)
	sum["cluster_up"] = map[string]any{"exit": up.ExitCode, "healthy": up.Summary["healthy"], "elapsed_ms": up.Summary["elapsed_ms"]}
	if up.ExitCode == 0 && usesKind(p, "F5a") {
		// F5a starts n4's containers from the runner (ADR-0022): they are
		// created here, from the Compose definitions, not started.
		r := c.run(ctx, host.ComposeTimeout, "compose-create-n4",
			c.compose("--profile", "n4", "create", "--no-recreate", "pg-n4", "pooler-n4", "nem-pg-n4"), cmdlog.Opts{})
		if err := ok(r); err != nil {
			up.ExitCode, up.Summary["error"] = 2, "create n4: "+err.Error()
		}
	}
	if up.ExitCode != 0 {
		reason = fmt.Sprintf("cluster up exit %d: %v", up.ExitCode, up.Summary["error"])
		hostOnlyHistory(dir, reason)
	} else {
		rel, err := filepath.Rel(root, dir)
		if err != nil || strings.HasPrefix(rel, "..") {
			// The output directory is outside the repository: the mount
			// source must then be absolute.
			rel = dir
		} else {
			rel = "./" + filepath.ToSlash(rel)
		}
		r := c.run(ctx, runnerTimeout(p), "runner-run",
			c.runnerArgs(nemesis.RunnerContainer, []string{rel + ":/run-dir"}, "__runner", "run", "--run-dir=/run-dir",
				"--nemesis-image="+nemImage, "--net-prefix="+prefix), cmdlog.Opts{})
		rs, err := LastJSON(r.Stdout)
		switch {
		case r.Err != nil:
			reason = "runner: " + r.Err.Error()
		case err != nil:
			reason = fmt.Sprintf("runner exit %d without summary", r.ExitCode)
		default:
			status, _ = rs["status"].(string)
			reason, _ = rs["reason"].(string)
		}
		if _, err := os.Stat(filepath.Join(dir, history.FileHistory)); err != nil {
			status = "invalid"
			hostOnlyHistory(dir, "runner wrote no history: "+reason)
		}
	}
	bg := context.Background() // teardown even if ctx was cancelled
	if up.ExitCode == 3 {
		// cluster up refused before creating anything (a prerequisite,
		// the image lock, or project resources that already exist — another
		// cluster's): there is nothing of this run to capture or remove.
		sum["cluster_down"] = map[string]any{"exit": 0, "skipped": "cluster up created nothing"}
	} else {
		if err := c.CaptureLogs(bg, filepath.Join(dir, history.DirLogs)); err != nil {
			status, reason = "invalid", join(reason, "log capture: "+err.Error())
		}
		down, code := c.Down(bg)
		writeJSON(filepath.Join(dir, history.DirLogs, "cluster-down.json"), down)
		sum["cluster_down"] = map[string]any{"exit": code, "leftover": down["leftover"]}
		if code != 0 {
			status, reason = "invalid", join(reason, fmt.Sprintf("cluster down exit %d", code))
		}
	}
	seal, err := history.SealDir(dir)
	if err != nil {
		return fail(2, "seal: "+err.Error())
	}
	v := history.VerifySeal(dir)
	sum["status"] = status
	sum["reason"] = reason
	sum["seal_root"] = seal.MerkleRoot
	sum["seal_valid"] = v.Valid
	sum["history_lines"] = v.HistoryLines
	sum["observation_lines"] = v.ObservationLines
	sum["ok_counts"] = OKCounts(v.History)
	if !v.Valid {
		sum["seal_problems"] = v.Problems
		return sum, 2
	}
	if status != "complete" {
		return sum, 2
	}
	return sum, 0
}

func strsAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// hostOnlyHistory writes the history and observation streams when the
// runner never ran, so the INVALID run can still be sealed. The host is then
// the only writer (ADR-0018 holds: there is never a second writer).
func hostOnlyHistory(dir, reason string) {
	now := time.Now()
	for _, f := range []string{history.FileHistory, history.FileObservations} {
		p := filepath.Join(dir, f)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		w, err := history.Create(p, now)
		if err != nil {
			continue
		}
		if f == history.FileHistory {
			w.Append(map[string]any{"type": history.TypeMarker, "value": map[string]any{"name": "run-end", "status": "invalid", "reason": reason, "writer": "host"}})
		}
		w.Close()
	}
}

// OKCounts counts ok completions of planned workload operations per wl
// (final reads and recovery writes excluded).
func OKCounts(evs []history.Event) map[string]any {
	n := map[string]int64{"append": 0, "register": 0, "bank": 0, "ledger": 0}
	for _, ev := range evs {
		if ev.Type == history.TypeOK && ev.Client != nil && *ev.Client < params.ClientsTotal && ev.F != "final-read" {
			n[ev.WL]++
		}
	}
	out := map[string]any{}
	for k, v := range n {
		out[k] = v
	}
	return out
}
