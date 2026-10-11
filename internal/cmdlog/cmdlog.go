// Package cmdlog runs external commands with a timeout and appends one
// canonical JSON record per command to a commands.jsonl (CONTRACT §5.4):
// argv, start/end CLOCK_MONOTONIC nanoseconds, exit code, and the sha256 of
// stdout and stderr. Outputs are kept next to the record so the hashes can
// be re-checked.
//
// CLOCK_MONOTONIC is the host kernel's; containers share it (Docker does not
// use time namespaces), so records written by the host and by the runner
// container are on one time axis. history.jsonl's run-start marker records
// the runner's CLOCK_MONOTONIC origin.
package cmdlog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
)

// MonoNS is CLOCK_MONOTONIC in nanoseconds.
func MonoNS() int64 {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		panic(fmt.Sprintf("clock_gettime: %v", err))
	}
	return ts.Nano()
}

// Recorder appends records to one commands.jsonl. Safe for concurrent use.
type Recorder struct {
	mu      sync.Mutex
	jsonl   string
	baseDir string // records store paths relative to this directory
	outDir  string // where stdout/stderr files are written
	prefix  string // distinguishes writers sharing one jsonl (host/runner)
	seq     int
}

// New returns a recorder writing to jsonl, keeping outputs under outDir.
// Paths in records are relative to baseDir.
func New(jsonl, baseDir, outDir, prefix string) (*Recorder, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("cmdlog: %w", err)
	}
	return &Recorder{jsonl: jsonl, baseDir: baseDir, outDir: outDir, prefix: prefix}, nil
}

// Opts adjusts one Run.
type Opts struct {
	Env    []string  // extra environment (appended to os.Environ())
	Dir    string    // working directory
	Stdin  io.Reader // optional
	Stdout string    // write stdout to this file instead of the default
	Echo   io.Writer // also copy stdout here (live progress)
}

// Result of one command.
type Result struct {
	ExitCode int
	Stdout   []byte // nil when Opts.Stdout redirected it to a file
	Stderr   []byte
	Err      error // start failure or timeout (ExitCode is then -1)
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// Run executes argv with timeout and records it. A non-zero exit is not an
// error of Run; callers decide (fail closed) from Result.
func (r *Recorder) Run(ctx context.Context, timeout time.Duration, name string, argv []string, o Opts) Result {
	r.mu.Lock()
	r.seq++
	seq := r.seq
	r.mu.Unlock()
	base := fmt.Sprintf("%s%04d-%s", r.prefix, seq, unsafeName.ReplaceAllString(name, "_"))
	stdoutPath := filepath.Join(r.outDir, base+".stdout")
	if o.Stdout != "" {
		stdoutPath = o.Stdout
	}
	stderrPath := filepath.Join(r.outDir, base+".stderr")

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, argv[0], argv[1:]...)
	cmd.WaitDelay = 5 * time.Second
	cmd.Env = append(os.Environ(), o.Env...)
	cmd.Dir = o.Dir
	if o.Stdin != nil {
		cmd.Stdin = o.Stdin
	}
	var res Result
	var stdoutBuf, stderrBuf bytes.Buffer
	outF, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		res.ExitCode, res.Err = -1, fmt.Errorf("cmdlog: %w", err)
		return res
	}
	var stdoutW io.Writer = outF
	if o.Stdout == "" {
		stdoutW = io.MultiWriter(outF, &stdoutBuf)
	}
	if o.Echo != nil {
		stdoutW = io.MultiWriter(stdoutW, o.Echo)
	}
	cmd.Stdout = stdoutW
	cmd.Stderr = &stderrBuf
	start := MonoNS()
	runErr := cmd.Run()
	end := MonoNS()
	outF.Close()
	if err := os.WriteFile(stderrPath, stderrBuf.Bytes(), 0o644); err != nil {
		res.Err = errors.Join(res.Err, fmt.Errorf("cmdlog: %w", err))
	}
	res.Stderr = stderrBuf.Bytes()
	if o.Stdout == "" {
		res.Stdout = stdoutBuf.Bytes()
	}
	res.ExitCode = 0
	errText := ""
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) && cctx.Err() == nil {
			res.ExitCode = ee.ExitCode()
		} else {
			res.ExitCode = -1
			if cctx.Err() != nil {
				runErr = fmt.Errorf("timeout after %s: %w", timeout, runErr)
			}
			res.Err = runErr
			errText = runErr.Error()
		}
	}
	outSum, _, _ := hashFile(stdoutPath)
	errSum := sha256.Sum256(res.Stderr)
	rel := func(p string) string {
		if q, err := filepath.Rel(r.baseDir, p); err == nil {
			return filepath.ToSlash(q)
		}
		return p
	}
	argvAny := make([]any, len(argv))
	for i, a := range argv {
		argvAny[i] = a
	}
	rec := map[string]any{
		"seq": seq, "name": r.prefix + name, "argv": argvAny,
		"start_mono_ns": start, "end_mono_ns": end, "exit_code": res.ExitCode,
		"stdout": rel(stdoutPath), "stdout_sha256": outSum,
		"stderr": rel(stderrPath), "stderr_sha256": hex.EncodeToString(errSum[:]),
		"timeout_ms": timeout.Milliseconds(),
	}
	if errText != "" {
		rec["error"] = errText
	}
	if err := r.append(rec); err != nil {
		res.Err = errors.Join(res.Err, err)
	}
	return res
}

// Note appends a record for an action that is not a subprocess (e.g. an
// in-process Docker or gRPC call), so commands.jsonl stays complete.
func (r *Recorder) Note(name string, fields map[string]any) error {
	rec := map[string]any{"name": r.prefix + name, "mono_ns": MonoNS()}
	for k, v := range fields {
		rec[k] = v
	}
	return r.append(rec)
}

func (r *Recorder) append(rec map[string]any) error {
	line, err := canonjson.Marshal(rec)
	if err != nil {
		return fmt.Errorf("cmdlog: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	f, err := os.OpenFile(r.jsonl, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("cmdlog: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return fmt.Errorf("cmdlog: %w", err)
	}
	return f.Close()
}

func hashFile(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}
