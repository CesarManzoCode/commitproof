// Command k4orch summarises multiorch's leader appointments in sealed D2 and
// D2′ runs, for evidence/K4/NOTES.md (ADR-0027): every failover attempt the
// orchestrators logged after the shard's initial election — orchestrator,
// proposed term, reason, poolers recruited (with the LSN they reported) and
// refused, outcome, new primary — with times relative to the first drop of
// the health stream of the pooler of the node the plan's F2a(leader) killed
// (negative: before the kill). It reads the seal-verified history and the
// orchestrators' logs of each run (a directory or a full archive
// <id>.tar.zst); derived runs (candidates, replays) are skipped. It is
// diagnosis only (ADR-0016): no verdict depends on it.
//
//	go run ./internal/tools/k4orch <run-dir|run-archive>... > summary.json
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/gate"
	"github.com/CesarManzoCode/commitproof/internal/history"
)

// orchLine is one JSON log line of multiorch (slog).
type orchLine struct {
	Time       time.Time `json:"time"`
	Msg        string    `json:"msg"`
	TraceID    string    `json:"trace_id"`
	Term       *int64    `json:"proposed_term"`
	Reason     string    `json:"reason"`
	Outcome    string    `json:"outcome"`
	Error      string    `json:"error"`
	NewPrimary string    `json:"new_primary"`
	Pooler     string    `json:"pooler"`
	PoolerID   string    `json:"pooler_id"`
	LSN        string    `json:"lsn"`
}

type recruit struct {
	Pooler string `json:"pooler"`
	LSN    string `json:"lsn,omitempty"`
	Error  string `json:"error,omitempty"`
}

type attempt struct {
	Orch       string    `json:"orch"`
	TS         float64   `json:"t_s"` // seconds after the anchor
	Term       int64     `json:"term"`
	Reason     string    `json:"reason"`
	Outcome    string    `json:"outcome"`
	Error      string    `json:"error,omitempty"`
	NewPrimary string    `json:"new_primary,omitempty"`
	Recruited  []recruit `json:"recruited"`
	Refused    []recruit `json:"refused"`
	start      time.Time
}

func short(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// killedLeader is the node of the first confirmed F2a whose role was
// "leader" (D2, D2′: element 3).
func killedLeader(evs []history.Event) string {
	for _, ev := range evs {
		if ev.Type != history.TypeNemesis || fmt.Sprint(ev.Fault["kind"]) != "F2a" || fmt.Sprint(ev.Fault["phase"]) != "confirmed" {
			continue
		}
		if ts, ok := ev.Fault["targets"].([]any); ok && len(ts) == 1 {
			return fmt.Sprint(ts[0])
		}
	}
	return ""
}

func readOrchLogs(dir string) (map[string][]orchLine, error) {
	files, err := filepath.Glob(filepath.Join(dir, history.DirLogs, "orch-*.log"))
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("%s: no orchestrator logs", dir)
	}
	out := map[string][]orchLine{}
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		orch := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "orch-"), ".log")
		for sc.Scan() {
			_, js, ok := strings.Cut(sc.Text(), " ")
			var l orchLine
			if !ok || json.Unmarshal([]byte(js), &l) != nil || l.Time.IsZero() {
				continue
			}
			out[orch] = append(out[orch], l)
		}
		err = sc.Err()
		fh.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
	}
	return out, nil
}

func summarize(dir string) (map[string]any, error) {
	rep := history.VerifySeal(dir)
	if !rep.Valid {
		return nil, fmt.Errorf("%s: seal: %v", dir, rep.Problems)
	}
	var man map[string]any
	b, err := os.ReadFile(filepath.Join(dir, history.FileManifest))
	if err != nil || json.Unmarshal(b, &man) != nil {
		return nil, fmt.Errorf("%s: manifest: %v", dir, err)
	}
	if man["replay_of"] != nil || man["candidate_of"] != nil {
		return nil, nil
	}
	leader := killedLeader(rep.History)
	if leader == "" {
		return nil, fmt.Errorf("%s: no confirmed F2a(leader)", dir)
	}
	logs, err := readOrchLogs(dir)
	if err != nil {
		return nil, err
	}
	// Anchor: the first drop of the killed leader's pooler health stream
	// seen by any orchestrator (D2 and D2′ touch that pooler only there).
	var anchor time.Time
	for _, ls := range logs {
		for _, l := range ls {
			if l.Msg == "health stream disconnected" && strings.HasSuffix(l.PoolerID, "-"+leader) && (anchor.IsZero() || l.Time.Before(anchor)) {
				anchor = l.Time
				break
			}
		}
	}
	if anchor.IsZero() {
		return nil, fmt.Errorf("%s: no health-stream drop of %s's pooler", dir, leader)
	}
	var atts []*attempt
	for orch, ls := range logs {
		byTrace := map[string]*attempt{}
		for _, l := range ls {
			if l.TraceID == "" {
				continue
			}
			a := byTrace[l.TraceID]
			switch l.Msg {
			case "primary.promotion":
				if l.Reason == "ShardInit" {
					continue
				}
				if a == nil {
					a = &attempt{Orch: orch, Recruited: []recruit{}, Refused: []recruit{}, start: l.Time}
					byTrace[l.TraceID] = a
					atts = append(atts, a)
				}
				if l.Term != nil {
					a.Term = *l.Term
				}
				a.Reason, a.Outcome = l.Reason, l.Outcome
				if l.Outcome != "started" {
					a.Error, a.NewPrimary = short(l.Error, 200), l.NewPrimary
				}
			case "recruited pooler":
				if a != nil {
					a.Recruited = append(a.Recruited, recruit{Pooler: l.Pooler, LSN: l.LSN})
				}
			case "recruit failed":
				if a != nil {
					a.Refused = append(a.Refused, recruit{Pooler: l.Pooler, Error: short(l.Error, 160)})
				}
			}
		}
	}
	sort.SliceStable(atts, func(i, j int) bool { return atts[i].start.Before(atts[j].start) })
	var first map[string]any
	before, split := 0, 0
	for _, a := range atts {
		a.TS = float64(a.start.Sub(anchor).Milliseconds()) / 1000
		if a.Outcome == "success" && first == nil {
			first = map[string]any{"t_s": a.TS, "orch": a.Orch, "term": a.Term, "reason": a.Reason, "new_primary": a.NewPrimary}
		}
		if first == nil && a.Outcome != "success" {
			before++
		}
		if strings.Contains(a.Error, "insufficient outgoing cohort recruitment") {
			split++
		}
	}
	return map[string]any{
		"run": filepath.Base(dir), "campaign": man["campaign"], "mutant": man["mutant"], "seed": man["seed"],
		"killed_leader": leader, "anchor": anchor.UTC().Format(time.RFC3339Nano),
		"attempts": atts, "first_success": first, "failed_attempts_before_first_success": before,
		"attempts_failed_insufficient_recruitment": split,
	}, nil
}

func main() {
	res := []any{}
	args := os.Args[1:]
	sort.Strings(args)
	tmp, err := os.MkdirTemp("", "k4orch-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer os.RemoveAll(tmp)
	for _, d := range args {
		if id, ok := strings.CutSuffix(filepath.Base(d), gate.ArchiveSuffix); ok {
			if err := gate.ExtractRun(d, tmp); err != nil {
				fmt.Fprintln(os.Stderr, d+":", err)
				os.RemoveAll(tmp)
				os.Exit(2)
			}
			d = filepath.Join(tmp, id)
		}
		s, err := summarize(d)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.RemoveAll(tmp)
			os.Exit(2)
		}
		if s != nil {
			res = append(res, s)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(res)
}
