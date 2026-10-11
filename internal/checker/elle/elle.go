// Package elle runs elle-cli as a differential oracle for C3 (CONTRACT §10.11,
// ADR-0004). elle-cli 0.1.11 (Elle 0.2.7) is built from its pinned commit in
// the pinned clojure image (build/Dockerfile.elle) and only executed in a
// container; nothing of Elle is vendored or linked.
//
// Each history is checked three times: with only the expected consistency
// model (its valid? is Elle's validity), and with the base anomalies plus
// either the -process or the -realtime variants requested (their
// anomaly-types, mapped to CONTRACT §10.3 names, are Elle's class set).
// Requesting extra anomalies makes elle-cli reject them in valid?, hence the
// separate validity run; requesting process and realtime variants in one run
// makes Elle label a cycle that needs only process order as -realtime, hence
// the separate class runs.
package elle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/checker"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Pins (CONTRACT §14).
const (
	CLICommit   = "9c2a18b3719f573dab06ae05c955c9c9d5225673"
	BaseImage   = "clojure:temurin-21-lein-2.13.0-bookworm@sha256:724a7f5840394bf9e659730e5039bf7498f31185e747e3586dbab626b717c34f"
	Image       = "commitproof/elle-oracle:9c2a18b"
	BatchSize   = 50
	dockerSlack = 120 * time.Second
)

// Requested anomalies of the two class runs.
var (
	base          = []string{"G0", "G1a", "G1b", "G1c", "G-single", "G-nonadjacent", "G2-item", "incompatible-order", "dirty-update"}
	ordered       = []string{"G0", "G1c", "G-single", "G-nonadjacent", "G2-item"}
	ProcessAnoms  = withSuffix("-process")
	RealtimeAnoms = withSuffix("-realtime")
	ElleModelOf   = map[string]string{"serializable": "serializable", "repeatable-read": "snapshot-isolation"}
	informational = map[string]bool{"empty-transaction-graph": true}
)

func withSuffix(s string) []string {
	out := append([]string{}, base...)
	for _, a := range ordered {
		out = append(out, a+s)
	}
	return out
}

// Map converts an Elle anomaly name to the CONTRACT §10.3 name: Elle 0.2.7
// names item-level cycles G-single-item and G-nonadjacent-item, and
// G1a/G1b in upper case. Informational names map to "" (dropped); names
// without a §10.3 counterpart map to "elle:<name>" so that they always
// count as a disagreement.
func Map(name string) string {
	if informational[name] {
		return ""
	}
	suffix := ""
	for _, s := range []string{"-process", "-realtime"} {
		if strings.HasSuffix(name, s) {
			suffix, name = s, strings.TrimSuffix(name, s)
		}
	}
	m := map[string]string{
		"G0": "G0", "G1a": "g1a", "G1b": "g1b", "G1c": "G1c", "G-single-item": "G-single",
		"G-nonadjacent-item": "G-nonadjacent", "G2-item": "G2-item",
		"duplicate-elements": "duplicate-elements", "incompatible-order": "incompatible-order",
		"internal": "internal", "dirty-update": "dirty-update",
	}
	if v, ok := m[name]; ok && (suffix == "" || strings.HasPrefix(v, "G")) {
		return v + suffix
	}
	return "elle:" + name + suffix
}

// Export converts the W1 operations of a history to elle-cli JSON: one array
// of op maps {type, f, process, time, value} in history order. index is
// omitted on purpose: with explicit indices, elle-cli deadlocks on any
// history containing a fail op (K2 NOTES); without them it assigns dense
// indices in file order.
func Export(h *checker.History) ([]byte, string, error) {
	ops := checker.W1Txns(h)
	type ev struct {
		line int64
		m    map[string]any
	}
	var evs []ev
	isos := map[string]bool{}
	for _, o := range ops {
		if o.Iso != "" {
			isos[o.Iso] = true
		}
		evs = append(evs, ev{o.Index, map[string]any{"type": "invoke", "f": "txn", "process": o.Proc, "time": o.Invoke, "value": o.InValue}})
		v := o.InValue
		if o.Type == "ok" {
			v = o.Value
		}
		evs = append(evs, ev{o.CompI, map[string]any{"type": o.Type, "f": "txn", "process": o.Proc, "time": o.Complete, "value": v}})
	}
	sort.Slice(evs, func(i, j int) bool { return evs[i].line < evs[j].line })
	model := "serializable"
	if isos["repeatable-read"] {
		model = "repeatable-read"
	}
	for iso := range isos {
		if ElleModelOf[iso] == "" {
			return nil, "", fmt.Errorf("isolation %q has no Elle model", iso)
		}
	}
	var b bytes.Buffer
	b.WriteString("[\n")
	for i, e := range evs {
		line, err := canonjson.Marshal(e.m)
		if err != nil {
			return nil, "", err
		}
		b.Write(line)
		if i+1 < len(evs) {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("]\n")
	return b.Bytes(), model, nil
}

// Run is one elle-cli invocation over a batch of files.
type Run struct {
	Kind    string // validity, process, realtime
	Args    []string
	Outputs [][]byte // one JSON analysis per file, in order
}

// Result is the oracle's verdict on one history.
type Result struct {
	Model    string   // CommitProof model (serializable, repeatable-read)
	Valid    string   // "true", "false", "unknown"
	Classes  []string // mapped classes from the process and realtime runs
	Unmapped []string
	Raw      map[string][]byte // kind → raw analysis
}

// ImageID returns the local ID of the oracle image (fail-closed if absent).
func ImageID(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", Image).Output()
	if err != nil {
		return "", fmt.Errorf("oracle image %s not available (make elle-oracle): %w", Image, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// args returns elle-cli arguments for one run kind (files appended later).
func args(kind, elleModel string) []string {
	a := []string{"-m", "list-append", "-c", elleModel, "--output", "json",
		"--cycle-search-timeout", strconv.FormatInt(params.CycleSearchBudget.Milliseconds(), 10)}
	switch kind {
	case "process":
		a = append(a, "-a", strings.Join(ProcessAnoms, ","))
	case "realtime":
		a = append(a, "-a", strings.Join(RealtimeAnoms, ","))
	}
	return a
}

// Kinds are the three runs per history.
var Kinds = []string{"validity", "process", "realtime"}

// CheckFiles runs the oracle on exported files (all of one CommitProof
// model) in dir, in batches, and returns one Result per file.
func CheckFiles(ctx context.Context, dir string, files []string, model string) ([]Result, []Run, error) {
	em := ElleModelOf[model]
	if em == "" {
		return nil, nil, fmt.Errorf("no Elle model for %q", model)
	}
	res := make([]Result, len(files))
	for i := range res {
		res[i] = Result{Model: model, Raw: map[string][]byte{}}
	}
	var runs []Run
	for start := 0; start < len(files); start += BatchSize {
		end := min(start+BatchSize, len(files))
		for _, kind := range Kinds {
			a := args(kind, em)
			outs, err := runBatch(ctx, dir, a, files[start:end])
			if err != nil {
				return nil, runs, fmt.Errorf("%s batch %d-%d: %w", kind, start, end, err)
			}
			runs = append(runs, Run{Kind: kind, Args: a, Outputs: outs})
			for j, out := range outs {
				res[start+j].Raw[kind] = out
			}
		}
	}
	for i := range res {
		if err := res[i].interpret(); err != nil {
			return nil, runs, fmt.Errorf("%s: %w", files[i], err)
		}
	}
	return res, runs, nil
}

// Analysis is the part of Elle's result map the comparison reads.
type Analysis struct {
	Valid        any      `json:"valid?"`
	AnomalyTypes []string `json:"anomaly-types"`
}

// ParseValid normalizes Elle's valid? (true, false or "unknown").
func ParseValid(v any) string {
	switch x := v.(type) {
	case bool:
		return strconv.FormatBool(x)
	case string:
		return x
	}
	return "unknown"
}

func (r *Result) interpret() error {
	r.Valid = "true"
	classes := map[string]bool{}
	for _, kind := range Kinds {
		var a Analysis
		if err := json.Unmarshal(r.Raw[kind], &a); err != nil {
			return fmt.Errorf("%s output: %w", kind, err)
		}
		v := ParseValid(a.Valid)
		if v == "unknown" {
			r.Valid = "unknown"
		}
		if kind == "validity" && r.Valid != "unknown" {
			r.Valid = v
		}
		if kind == "validity" {
			continue
		}
		for _, t := range a.AnomalyTypes {
			m := Map(t)
			if m == "" {
				continue
			}
			if strings.HasPrefix(m, "elle:") {
				r.Unmapped = append(r.Unmapped, t)
			}
			classes[m] = true
		}
	}
	for c := range classes {
		r.Classes = append(r.Classes, c)
	}
	sort.Strings(r.Classes)
	sort.Strings(r.Unmapped)
	return nil
}

// runBatch runs elle-cli in the oracle container on files (names relative
// to dir, mounted read-only, no network). The container is named so that a
// timed-out run is killed, not left running. Timeout: P11 per file plus a
// fixed allowance for JVM start-up.
func runBatch(ctx context.Context, dir string, a []string, files []string) ([][]byte, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("cp-elle-%d-%d", os.Getpid(), time.Now().UnixNano())
	argv := []string{"run", "--rm", "--name", name, "--network", "none", "--read-only", "--tmpfs", "/tmp",
		"-v", abs + ":/h:ro", Image}
	argv = append(argv, a...)
	for _, f := range files {
		argv = append(argv, "/h/"+f)
	}
	timeout := dockerSlack + time.Duration(len(files))*params.PorcupineTimeout
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "docker", argv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	if cctx.Err() != nil {
		kctx, kcancel := context.WithTimeout(context.Background(), 30*time.Second)
		exec.CommandContext(kctx, "docker", "kill", name).Run()
		kcancel()
		return nil, fmt.Errorf("elle-cli timed out after %s", timeout)
	}
	// elle-cli exits 1 when some history is invalid; any other failure is an
	// error.
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() != 1 {
		return nil, fmt.Errorf("elle-cli exited %d: %s", ee.ExitCode(), lastLines(stderr.String()+stdout.String(), 10))
	} else if err != nil && !ok {
		return nil, err
	}
	dec := json.NewDecoder(&stdout)
	var outs [][]byte
	for dec.More() {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("parse elle-cli output (object %d): %w", len(outs), err)
		}
		c, err := json.Marshal(raw) // compact
		if err != nil {
			return nil, err
		}
		outs = append(outs, c)
	}
	if len(outs) != len(files) {
		return nil, fmt.Errorf("elle-cli printed %d analyses for %d files", len(outs), len(files))
	}
	return outs, nil
}

func lastLines(s string, n int) string {
	ls := strings.Split(strings.TrimSpace(s), "\n")
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	return strings.Join(ls, " | ")
}
