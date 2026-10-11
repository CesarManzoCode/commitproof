// Package cluster brings the Compose topology of CONTRACT §5.1 up and down,
// checks the HEALTHY predicate through the runner container, and drives a
// run from the host (ADR-0018: the host only builds images and launches the
// runner).
package cluster

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/cmdlog"
	"github.com/CesarManzoCode/commitproof/internal/host"
	"github.com/CesarManzoCode/commitproof/internal/images"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Project is the Compose project name (CONTRACT §5.1).
const Project = "commitproof"

// ProjectLabel selects every Docker resource of the project.
const ProjectLabel = "com.docker.compose.project=" + Project

// Profiles implemented by deploy/ (K1: baseline only).
var implemented = map[string]bool{"baseline": true}

// Cluster is the host-side handle.
type Cluster struct {
	Root    string
	Profile string
	Rec     *cmdlog.Recorder
	Echo    io.Writer
}

// New validates the profile.
func New(root, profile string, rec *cmdlog.Recorder) (*Cluster, error) {
	if !implemented[profile] {
		return nil, fmt.Errorf("profile %q is not implemented yet (implemented: baseline)", profile)
	}
	return &Cluster{Root: root, Profile: profile, Rec: rec}, nil
}

// env returns the Compose interpolation variables (P26: CP_SUBNET).
func (c *Cluster) env() ([]string, error) {
	subnet := os.Getenv("CP_SUBNET")
	if subnet == "" {
		subnet = params.DockerSubnet
	}
	ip, ipnet, err := net.ParseCIDR(subnet)
	if err != nil || ip.To4() == nil {
		return nil, fmt.Errorf("CP_SUBNET %q: want an IPv4 /24", subnet)
	}
	if ones, _ := ipnet.Mask.Size(); ones != 24 {
		return nil, fmt.Errorf("CP_SUBNET %q: want an IPv4 /24", subnet)
	}
	v4 := ipnet.IP.To4()
	prefix := fmt.Sprintf("%d.%d.%d", v4[0], v4[1], v4[2])
	return []string{"CP_SUBNET=" + ipnet.String(), "CP_NET_PREFIX=" + prefix, "CP_IMAGE_TAG=real", "CP_RUNNER_TAG=real"}, nil
}

func (c *Cluster) compose(args ...string) []string {
	return append([]string{"docker", "compose", "--project-name", Project,
		"--file", filepath.Join(c.Root, "deploy", "compose.yaml"),
		"--project-directory", filepath.Join(c.Root, "deploy")}, args...)
}

func (c *Cluster) run(ctx context.Context, timeout time.Duration, name string, argv []string, o cmdlog.Opts) cmdlog.Result {
	env, err := c.env()
	if err != nil {
		return cmdlog.Result{ExitCode: -1, Err: err}
	}
	o.Env = append(o.Env, env...)
	if o.Dir == "" {
		o.Dir = c.Root
	}
	return c.Rec.Run(ctx, timeout, name, argv, o)
}

func ok(r cmdlog.Result) error {
	if r.Err != nil {
		return r.Err
	}
	if r.ExitCode != 0 {
		return fmt.Errorf("exit %d: %s", r.ExitCode, strings.TrimSpace(lastLines(r.Stderr, 5)))
	}
	return nil
}

func lastLines(b []byte, n int) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

// Preflight returns the missing prerequisites of CONTRACT §5.6 (Docker
// reachable, /dev/fuse, sch_netem and the nf_tables iptables backend
// available in the running kernel). Empty = all present.
func (c *Cluster) Preflight(ctx context.Context) []string {
	var missing []string
	if r := c.Rec.Run(ctx, host.QuickTimeout, "docker-version", []string{"docker", "version", "--format", "{{.Server.Version}}"}, cmdlog.Opts{}); ok(r) != nil {
		missing = append(missing, "Docker daemon unreachable: "+ok(r).Error())
	}
	if st, err := os.Stat("/dev/fuse"); err != nil || st.Mode()&os.ModeCharDevice == 0 {
		missing = append(missing, "/dev/fuse character device absent")
	}
	for _, m := range []string{"sch_netem", "nf_tables", "nft_compat"} {
		if !kernelModule(m) {
			missing = append(missing, "kernel module "+m+" not available (not loaded, not built in)")
		}
	}
	return missing
}

func kernelModule(name string) bool {
	if b, err := os.ReadFile("/proc/modules"); err == nil {
		sc := bufio.NewScanner(bytes.NewReader(b))
		for sc.Scan() {
			if f := strings.Fields(sc.Text()); len(f) > 0 && f[0] == name {
				return true
			}
		}
	}
	rel, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return false
	}
	b, err := os.ReadFile(filepath.Join("/lib/modules", strings.TrimSpace(string(rel)), "modules.builtin"))
	return err == nil && bytes.Contains(b, []byte("/"+name+".ko"))
}

// Resources lists Docker containers, volumes and networks labelled with the
// project (CONTRACT §15 K1.4).
func (c *Cluster) Resources(ctx context.Context) (map[string][]string, error) {
	out := map[string][]string{}
	for kind, argv := range map[string][]string{
		"containers": {"docker", "ps", "-a", "--filter", "label=" + ProjectLabel, "--format", "{{.Names}}"},
		"volumes":    {"docker", "volume", "ls", "--filter", "label=" + ProjectLabel, "--format", "{{.Name}}"},
		"networks":   {"docker", "network", "ls", "--filter", "label=" + ProjectLabel, "--format", "{{.Name}}"},
	} {
		r := c.Rec.Run(ctx, host.QuickTimeout, "list-"+kind, argv, cmdlog.Opts{})
		if err := ok(r); err != nil {
			return nil, fmt.Errorf("list %s: %w", kind, err)
		}
		names := []string{}
		for _, l := range strings.Split(strings.TrimSpace(string(r.Stdout)), "\n") {
			if l != "" {
				names = append(names, l)
			}
		}
		sort.Strings(names)
		out[kind] = names
	}
	return out, nil
}

func empty(res map[string][]string) bool {
	return len(res["containers"])+len(res["volumes"])+len(res["networks"]) == 0
}

// UpResult is the outcome of Up.
type UpResult struct {
	Summary  map[string]any
	ExitCode int
}

// Up starts the topology from zero and waits for HEALTHY within P09. It
// refuses (exit 3) when a prerequisite is missing, an image differs from the
// lock, or project resources already exist. The predicate is evaluated by
// the runner container; nothing is restarted or retried.
func (c *Cluster) Up(ctx context.Context) UpResult {
	start := time.Now()
	sum := map[string]any{"command": "cluster up", "profile": c.Profile}
	fail := func(code int, msg string) UpResult {
		sum["error"] = msg
		sum["healthy"] = false
		sum["elapsed_ms"] = time.Since(start).Milliseconds()
		return UpResult{Summary: sum, ExitCode: code}
	}
	if missing := c.Preflight(ctx); len(missing) > 0 {
		sum["missing"] = missing
		return fail(3, "prerequisite missing: "+strings.Join(missing, "; "))
	}
	b := &images.Builder{Root: c.Root, Rec: c.Rec}
	mm, _, err := b.Verify(ctx)
	if err != nil {
		return fail(3, "image lock: "+err.Error())
	}
	if len(mm) > 0 {
		var s []string
		for _, m := range mm {
			s = append(s, fmt.Sprintf("%s local %s lock %s", m.Tag, m.Local, m.Lock))
		}
		return fail(3, "local images differ from images.lock.json: "+strings.Join(s, "; "))
	}
	pre, err := c.Resources(ctx)
	if err != nil {
		return fail(2, err.Error())
	}
	sum["preexisting"] = pre
	if !empty(pre) {
		return fail(3, "project resources already exist (run `commitproof cluster down` first)")
	}
	r := c.run(ctx, host.ComposeTimeout, "compose-up", c.compose("up", "--detach", "--no-build", "--quiet-pull"), cmdlog.Opts{})
	sum["compose_up_exit"] = r.ExitCode
	if err := ok(r); err != nil {
		return fail(2, "docker compose up: "+err.Error())
	}
	remaining := params.HealthyDeadline - time.Since(start)
	h, err := c.Healthy(ctx, remaining)
	sum["elapsed_ms"] = time.Since(start).Milliseconds()
	if err != nil {
		return fail(2, "healthy check: "+err.Error())
	}
	sum["healthy"] = h["healthy"]
	sum["evaluations"] = h["evaluations"]
	sum["version"] = h["version"]
	sum["predicate"] = h["predicate"]
	sum["deadline_ms"] = params.HealthyDeadline.Milliseconds()
	healthy, _ := h["healthy"].(bool)
	if !healthy || time.Since(start) > params.HealthyDeadline {
		sum["healthy"] = false
		sum["error"] = "not HEALTHY within P09"
		return UpResult{Summary: sum, ExitCode: 1}
	}
	return UpResult{Summary: sum, ExitCode: 0}
}

// RunnerArgs builds `docker compose run` for the runner service as the
// invoking user (so files written into bind mounts are the user's).
func (c *Cluster) RunnerArgs(mounts []string, args ...string) []string {
	argv := c.compose("run", "--rm", "--no-deps", "--quiet-pull", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()))
	for _, m := range mounts {
		argv = append(argv, "--volume", m)
	}
	return append(append(argv, "runner"), args...)
}

// Healthy runs the predicate inside the runner container, re-evaluating
// every second until it holds or deadline passes. It returns the runner's
// JSON summary.
func (c *Cluster) Healthy(ctx context.Context, deadline time.Duration) (map[string]any, error) {
	if deadline < 0 {
		deadline = 0
	}
	argv := c.RunnerArgs(nil, "__runner", "healthy", fmt.Sprintf("--deadline-ms=%d", deadline.Milliseconds()))
	r := c.run(ctx, deadline+host.ComposeTimeout, "runner-healthy", argv, cmdlog.Opts{})
	if r.Err != nil {
		return nil, r.Err
	}
	m, err := LastJSON(r.Stdout)
	if err != nil {
		return nil, fmt.Errorf("runner healthy (exit %d): %w; stderr: %s", r.ExitCode, err, lastLines(r.Stderr, 5))
	}
	return m, nil
}

// LastJSON parses the last non-empty stdout line as a JSON object (every
// command's summary line, CONTRACT §5.6).
func LastJSON(out []byte) (map[string]any, error) {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	var m map[string]any
	dec := json.NewDecoder(strings.NewReader(last))
	dec.UseNumber() // integers stay integers (canonical JSON has no floats)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("no JSON summary line: %q", last)
	}
	return m, nil
}

// Down removes every container, volume and network of the project and
// reports what is left.
func (c *Cluster) Down(ctx context.Context) (map[string]any, int) {
	sum := map[string]any{"command": "cluster down", "profile": c.Profile}
	r := c.run(ctx, host.ComposeTimeout, "compose-down",
		c.compose("--profile", "n4", "--profile", "runner", "down", "--volumes", "--remove-orphans", "--timeout", "10"), cmdlog.Opts{})
	sum["compose_down_exit"] = r.ExitCode
	res, err := c.Resources(ctx)
	if err != nil {
		sum["error"] = err.Error()
		return sum, 2
	}
	sum["leftover"] = res
	if err := ok(r); err != nil {
		sum["error"] = "docker compose down: " + err.Error()
		return sum, 2
	}
	if !empty(res) {
		sum["error"] = "resources left after down"
		return sum, 1
	}
	return sum, 0
}

// Verify checks that running containers use the locked images and that
// the HEALTHY predicate holds now (one evaluation).
func (c *Cluster) Verify(ctx context.Context) (map[string]any, int) {
	sum := map[string]any{"command": "cluster verify", "profile": c.Profile}
	lock, err := images.ReadLock(c.Root)
	if err != nil {
		sum["error"] = err.Error()
		return sum, 3
	}
	want := map[string]string{}
	imgs, _ := lock["images"].(map[string]any)
	for tag, e := range imgs {
		m, _ := e.(map[string]any)
		want[tag], _ = m["id"].(string)
	}
	r := c.run(ctx, host.QuickTimeout, "running-images", []string{"docker", "ps", "--filter", "label=" + ProjectLabel,
		"--format", "{{.Names}} {{.Image}}"}, cmdlog.Opts{})
	if err := ok(r); err != nil {
		sum["error"] = err.Error()
		return sum, 2
	}
	var bad []string
	running := []string{}
	for _, l := range strings.Split(strings.TrimSpace(string(r.Stdout)), "\n") {
		f := strings.Fields(l)
		if len(f) != 2 {
			continue
		}
		running = append(running, f[0])
		if id, locked := want[f[1]]; locked {
			ri := c.run(ctx, host.QuickTimeout, "container-image", []string{"docker", "inspect", "--format", "{{.Image}}", f[0]}, cmdlog.Opts{})
			if got := strings.TrimSpace(string(ri.Stdout)); ok(ri) != nil || got != id {
				bad = append(bad, fmt.Sprintf("%s runs %s, lock %s", f[0], got, id))
			}
		} else if strings.HasPrefix(f[1], "commitproof/") {
			bad = append(bad, fmt.Sprintf("%s runs unlocked image %s", f[0], f[1]))
		}
	}
	sort.Strings(running)
	sum["running"] = running
	sum["image_mismatches"] = bad
	if bad == nil {
		sum["image_mismatches"] = []string{}
	}
	h, err := c.Healthy(ctx, 0)
	if err != nil {
		sum["error"] = err.Error()
		return sum, 2
	}
	sum["healthy"] = h["healthy"]
	sum["version"] = h["version"]
	sum["predicate"] = h["predicate"]
	healthy, _ := h["healthy"].(bool)
	if !healthy || len(bad) > 0 {
		return sum, 1
	}
	return sum, 0
}

// Services lists the project's services that have (or had) a container.
func (c *Cluster) Services(ctx context.Context) ([]string, error) {
	r := c.run(ctx, host.QuickTimeout, "compose-ps", c.compose("--profile", "n4", "ps", "--all", "--format", "{{.Service}}"), cmdlog.Opts{})
	if err := ok(r); err != nil {
		return nil, err
	}
	var s []string
	for _, l := range strings.Split(strings.TrimSpace(string(r.Stdout)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			s = append(s, l)
		}
	}
	sort.Strings(s)
	return s, nil
}

// CaptureLogs writes `docker compose logs --timestamps` of every service to
// dir/<service>.log (CONTRACT §5.4).
func (c *Cluster) CaptureLogs(ctx context.Context, dir string) error {
	svcs, err := c.Services(ctx)
	if err != nil {
		return err
	}
	for _, s := range svcs {
		r := c.run(ctx, host.LogsTimeout, "logs-"+s, c.compose("logs", "--timestamps", "--no-color", "--no-log-prefix", s),
			cmdlog.Opts{Stdout: filepath.Join(dir, s+".log")})
		if err := ok(r); err != nil {
			return fmt.Errorf("logs %s: %w", s, err)
		}
	}
	return nil
}
