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
	"github.com/CesarManzoCode/commitproof/internal/params"
	"github.com/CesarManzoCode/commitproof/internal/plan"
)

// runnerTimeout bounds the runner process: workload, in-flight ops,
// recovery, quiescence and final reads, plus one P09 of slack.
func runnerTimeout(p *plan.Plan) time.Duration {
	return time.Duration(p.DurationMS)*time.Millisecond + params.ClientOpTimeout + params.RecoveryDeadline +
		params.Quiescence + params.FinalReadDeadline + 2*params.ClientOpTimeout + params.HealthyDeadline
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// RunPlan executes one run (CONTRACT §5.4): cluster up from zero, the runner
// writes the history, logs are captured, the cluster is torn down and the
// run directory sealed. Exit code: 0 complete, 2 INVALID, 3 prerequisite.
func RunPlan(ctx context.Context, root, profile, mutant string, p *plan.Plan, outDir string) (map[string]any, int) {
	sum := map[string]any{"command": "run", "campaign": p.Campaign, "seed": p.Seed, "profile": profile}
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
	if missing := pc.Preflight(ctx); len(missing) > 0 {
		sum["missing"] = missing
		return fail(3, "prerequisite missing: "+strings.Join(missing, "; "))
	}
	lock, err := images.ReadLock(root)
	if err != nil {
		return fail(3, "image lock: "+err.Error())
	}
	cp, _ := lock["commitproof"].(map[string]any)
	if lc, _ := cp["commit"].(string); lc != buildinfo.Commit {
		return fail(3, fmt.Sprintf("runner image built from commitproof %s, this binary is %s: run `commitproof images build`", lc, buildinfo.Commit))
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
		"host_kernel":         strings.TrimSpace(string(kernel)),
		"docker_version":      strings.TrimSpace(string(dv.Stdout)),
		"start_wall_time":     start.UTC().Format(time.RFC3339),
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
	if up.ExitCode != 0 {
		reason = fmt.Sprintf("cluster up exit %d: %v", up.ExitCode, up.Summary["error"])
		hostOnlyHistory(dir, reason)
	} else {
		r := c.run(ctx, runnerTimeout(p), "runner-run",
			c.RunnerArgs([]string{dir + ":/run-dir"}, "__runner", "run", "--run-dir=/run-dir"), cmdlog.Opts{})
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
	if err := c.CaptureLogs(bg, filepath.Join(dir, history.DirLogs)); err != nil {
		status, reason = "invalid", join(reason, "log capture: "+err.Error())
	}
	down, code := c.Down(bg)
	writeJSON(filepath.Join(dir, history.DirLogs, "cluster-down.json"), down)
	sum["cluster_down"] = map[string]any{"exit": code, "leftover": down["leftover"]}
	if code != 0 {
		status, reason = "invalid", join(reason, fmt.Sprintf("cluster down exit %d", code))
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
