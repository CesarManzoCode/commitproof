package nemesis

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/CesarManzoCode/commitproof/internal/cmdlog"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Timeouts (AGENTS.md §4: derived from CONTRACT §5.7).
const (
	// cmdTimeout bounds one docker command or exec (P01).
	cmdTimeout = params.ClientOpTimeout
	// connectTimeout is the per-attempt TCP connect timeout of F1 probes
	// ("fails within 1 s", §8 F1a) and of F7's RTT probes.
	connectTimeout = time.Second
	// statusTimeout bounds one pooler status RPC of the role-resolution
	// probe: a fifth of P27, so a round over unreachable nodes still leaves
	// time to retry before the confirmation deadline.
	statusTimeout = params.FaultConfirmDeadline / 5
	// pollInterval separates predicate rounds.
	pollInterval = 500 * time.Millisecond
)

// Env is what the nemesis needs from the runner.
type Env struct {
	Rec      *cmdlog.Recorder // the run's commands.jsonl (runner prefix)
	Prefix   string           // cpnet /24 prefix, "10.77.0"
	Exe      string           // the commitproof binary (status probe)
	NemImage string           // image of nem-runner (the locked nemesis image ID)
	Emit     func(fault map[string]any) error
	Marker   func(name string, v map[string]any)
	// NodeStarted is called when F5a has started a node, so the runner
	// observes it.
	NodeStarted func(node string)
	// Profile is the run's profile; "h1" resolves the leader from
	// PostgreSQL itself (the historical pooler API predates rules,
	// ADR-0025).
	Profile string
}

// Probe is one executed command with its raw outputs.
type Probe struct {
	Argv []string
	Res  cmdlog.Result
}

// OK reports exit 0 without a start/timeout error.
func (p Probe) OK() bool { return p.Res.Err == nil && p.Res.ExitCode == 0 }

func hexSHA(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// rawText reports whether b survives canonical JSON unchanged (valid UTF-8
// in NFC); otherwise the evidence carries it base64-encoded.
func rawText(b []byte) bool { return utf8.Valid(b) && norm.NFC.IsNormal(b) }

// Evidence is the probe's entry in a fault event: the commands.jsonl record
// it corresponds to (name, seq), argv, exit code, and the raw stdout and
// stderr with their sha256 (CONTRACT §15 K3.4).
func (p Probe) Evidence() map[string]any {
	argv := make([]any, len(p.Argv))
	for i, a := range p.Argv {
		argv[i] = a
	}
	m := map[string]any{"name": p.Res.Name, "seq": p.Res.Seq, "argv": argv, "exit_code": p.Res.ExitCode,
		"stdout_sha256": hexSHA(p.Res.Stdout), "stderr_sha256": hexSHA(p.Res.Stderr)}
	for _, f := range []struct {
		k string
		b []byte
	}{{"stdout", p.Res.Stdout}, {"stderr", p.Res.Stderr}} {
		if rawText(f.b) {
			m[f.k] = string(f.b)
		} else {
			m[f.k+"_b64"] = base64.StdEncoding.EncodeToString(f.b)
		}
	}
	if p.Res.Err != nil {
		m["error"] = p.Res.Err.Error()
	}
	return m
}

func evidenceList(ps []Probe) []any {
	out := make([]any, len(ps))
	for i, p := range ps {
		out[i] = p.Evidence()
	}
	return out
}

// run executes argv, recorded under name.
func (e *Env) run(ctx context.Context, name string, timeout time.Duration, argv ...string) Probe {
	return Probe{Argv: argv, Res: e.Rec.Run(ctx, timeout, name, argv, cmdlog.Opts{})}
}

// docker runs the docker CLI.
func (e *Env) docker(ctx context.Context, name string, timeout time.Duration, args ...string) Probe {
	return e.run(ctx, name, timeout, append([]string{"docker"}, args...)...)
}

// exec runs a command inside a container.
func (e *Env) exec(ctx context.Context, name, container string, argv ...string) Probe {
	return e.docker(ctx, name, cmdTimeout, append([]string{"exec", container}, argv...)...)
}

// parallel runs f(0..n-1) concurrently and returns the probes in index
// order.
func parallel(n int, f func(i int) Probe) []Probe {
	out := make([]Probe, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() { defer wg.Done(); out[i] = f(i) }()
	}
	wg.Wait()
	return out
}

// lastJSON decodes the last non-empty stdout line of p into v.
func lastJSON(p Probe, v any) error {
	lines := strings.Split(strings.TrimSpace(string(p.Res.Stdout)), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "" {
		return fmt.Errorf("%s: empty output (exit %d)", p.Res.Name, p.Res.ExitCode)
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(last)))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", p.Res.Name, err)
	}
	return nil
}

// runningContainers lists the running containers of the project.
func (e *Env) runningContainers(ctx context.Context, name string) (Probe, map[string]bool) {
	p := e.docker(ctx, name, cmdTimeout, "ps", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.Names}}")
	set := map[string]bool{}
	if p.OK() {
		for _, l := range strings.Split(string(p.Res.Stdout), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				set[l] = true
			}
		}
	}
	return p, set
}

// containerState is the part of `docker inspect` State the predicates read.
type containerState struct {
	Status   string `json:"Status"`
	Running  bool   `json:"Running"`
	Paused   bool   `json:"Paused"`
	ExitCode int    `json:"ExitCode"`
	Pid      int    `json:"Pid"`
}

// inspectState returns a container's State.
func (e *Env) inspectState(ctx context.Context, name, container string) (Probe, containerState, error) {
	p := e.docker(ctx, name, cmdTimeout, "inspect", "--format", "{{json .State}}", container)
	var st containerState
	if !p.OK() {
		return p, st, fmt.Errorf("docker inspect %s: exit %d", container, p.Res.ExitCode)
	}
	return p, st, lastJSON(p, &st)
}
