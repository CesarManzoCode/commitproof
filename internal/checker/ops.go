package checker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Never is the completion time of an op without a known completion (info):
// its effect may happen at any time after its invocation.
const Never = int64(math.MaxInt64)

// Op is one invoke paired with its completion (CONTRACT §6.1).
type Op struct {
	Index    int64 // i of the invoke event
	Proc     int64
	Client   int64
	GW       string
	WL       string
	F        string
	Iso      string
	Invoke   int64 // t of the invoke
	Complete int64 // t of the completion
	Type     string
	// Value is the completion's value for ok ops (the observed result) and
	// the invoke's value otherwise.
	Value   any
	InValue any
	Err     map[string]any
	CompI   int64
	Notices []any
	Done    bool
}

// End is the latest time the op's effect may have happened: its completion
// for ok and fail ops, Never for info ops (CONTRACT §10.4, open-ended).
func (o *Op) End() int64 {
	if o.Type == history.TypeInfo {
		return Never
	}
	return o.Complete
}

// SQLState returns err.sqlstate ("" if absent).
func (o *Op) SQLState() string {
	s, _ := o.Err["sqlstate"].(string)
	return s
}

// History is the input of every checker: the paired client operations in
// invoke order, nemesis events and markers, and the plan.
type History struct {
	RunID    string
	SealRoot string
	Ops      []*Op
	Nemesis  []history.Event
	Markers  []history.Event
	Plan     map[string]any
	// Profile and bank setup come from the plan (CONTRACT §5.5, P22).
	Profile      string
	BankAccounts int64
	BankInitial  int64
	// End is the t of the last history event.
	End int64
}

// FromEvents pairs invokes and completions. The events must form a
// well-formed history (history.WellFormed); an unanswered invoke is an
// error here, because cpcheck refuses runs whose seal check failed.
func FromEvents(evs []history.Event, plan map[string]any) (*History, error) {
	h := &History{Plan: plan, Profile: "baseline", BankAccounts: params.BankAccounts, BankInitial: params.BankInitialBalance}
	if plan != nil {
		if p, ok := plan["profile"].(string); ok && p != "" {
			h.Profile = p
		}
		if s, ok := plan["setup"].(map[string]any); ok {
			if n, ok := Int(s["bank_accounts"]); ok {
				h.BankAccounts = n
			}
			if n, ok := Int(s["bank_initial"]); ok {
				h.BankInitial = n
			}
		}
	}
	byInvoke := map[int64]*Op{}
	for _, ev := range evs {
		if ev.T > h.End {
			h.End = ev.T
		}
		switch ev.Type {
		case history.TypeInvoke:
			if ev.Proc == nil || ev.Client == nil {
				return nil, fmt.Errorf("i=%d: invoke without proc/client", ev.I)
			}
			o := &Op{Index: ev.I, Proc: *ev.Proc, Client: *ev.Client, GW: ev.GW, WL: ev.WL, F: ev.F, Iso: ev.Iso,
				Invoke: ev.T, Complete: -1, Value: ev.Value, InValue: ev.Value}
			byInvoke[ev.I] = o
			h.Ops = append(h.Ops, o)
		case history.TypeOK, history.TypeFail, history.TypeInfo:
			if ev.Op == nil {
				return nil, fmt.Errorf("i=%d: completion without op", ev.I)
			}
			o, ok := byInvoke[*ev.Op]
			if !ok || o.Done {
				return nil, fmt.Errorf("i=%d: completion of unknown or completed op %d", ev.I, *ev.Op)
			}
			o.Done, o.Complete, o.Type, o.CompI, o.Err, o.Notices = true, ev.T, ev.Type, ev.I, ev.Err, ev.Notices
			if ev.Type == history.TypeOK {
				o.Value = ev.Value
			}
		case history.TypeNemesis:
			h.Nemesis = append(h.Nemesis, ev)
		case history.TypeMarker:
			h.Markers = append(h.Markers, ev)
		}
	}
	for _, o := range h.Ops {
		if !o.Done {
			return nil, fmt.Errorf("invoke i=%d has no completion", o.Index)
		}
	}
	return h, nil
}

// Load verifies a run directory's seal and loads its history and plan. A
// seal problem is returned as an error: no checker runs on an unverified
// history (CONTRACT §5.4).
func Load(dir string) (*History, *history.SealReport, error) {
	rep := history.VerifySeal(dir)
	if !rep.Valid {
		return nil, rep, fmt.Errorf("seal does not verify: %v", rep.Problems)
	}
	raw, err := os.ReadFile(filepath.Join(dir, history.FilePlan))
	if err != nil {
		return nil, rep, fmt.Errorf("read plan: %w", err)
	}
	pv, err := canonjson.Decode(bytes.TrimSpace(raw))
	if err != nil {
		return nil, rep, fmt.Errorf("plan: %w", err)
	}
	plan, _ := pv.(map[string]any)
	h, err := FromEvents(rep.History, plan)
	if err != nil {
		return nil, rep, err
	}
	h.SealRoot = rep.MerkleRoot
	h.RunID = filepath.Base(filepath.Clean(dir))
	if mb, err := os.ReadFile(filepath.Join(dir, history.FileManifest)); err == nil {
		if mv, err := canonjson.Decode(bytes.TrimSpace(mb)); err == nil {
			if m, ok := mv.(map[string]any); ok {
				if id, ok := m["run_id"].(string); ok && id != "" {
					h.RunID = id
				}
			}
		}
	}
	return h, rep, nil
}

// Int converts a decoded JSON integer (json.Number, int64, int).
func Int(v any) (int64, bool) {
	switch x := v.(type) {
	case json.Number:
		n, err := strconv.ParseInt(string(x), 10, 64)
		return n, err == nil
	case int64:
		return x, true
	case int:
		return int64(x), true
	}
	return 0, false
}

// IntList converts a decoded JSON array of integers; ok is false for null
// (nil, true is returned for an empty array).
func IntList(v any) ([]int64, bool) {
	switch x := v.(type) {
	case []any:
		out := make([]int64, 0, len(x))
		for _, e := range x {
			n, ok := Int(e)
			if !ok {
				return nil, false
			}
			out = append(out, n)
		}
		return out, true
	case []int64:
		return x, true
	}
	return nil, false
}

func obj(v any) map[string]any { m, _ := v.(map[string]any); return m }
func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

// opsOf returns the ops of workload wl, excluding final reads.
func (h *History) opsOf(wl string) []*Op {
	var out []*Op
	for _, o := range h.Ops {
		if o.WL == wl && o.F != "final-read" {
			out = append(out, o)
		}
	}
	return out
}

// Gateways returns the sorted set of gateways used by any operation.
func (h *History) Gateways() []string {
	seen := map[string]bool{}
	for _, o := range h.Ops {
		if o.GW != "" {
			seen[o.GW] = true
		}
	}
	out := make([]string, 0, len(seen))
	for g := range seen {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

// FinalWorkloads are the tables read by the final reads (CONTRACT §6.4).
var FinalWorkloads = []string{"append", "register", "bank", "ledger"}

// finalReads returns every ok final read of wl, in history order.
func (h *History) finalReads(wl string) []*Op {
	var out []*Op
	for _, o := range h.Ops {
		if o.WL == wl && o.F == "final-read" && o.Type == history.TypeOK {
			out = append(out, o)
		}
	}
	return out
}

// Stable is the stable final read of one workload through one gateway: the
// value of its last P08 consecutive identical ok final reads.
type Stable struct {
	Value any
	Ops   []int64
}

// StableFinal computes, for every gateway used by the history and every
// final-read workload, the stable final read (P08: the last
// params.FinalReadRepeats ok final reads through that gateway are
// identical). It returns the stable reads and the list of (gateway,
// workload) pairs without one. It recomputes stability from the history; it
// never trusts the runner's final-read-stable markers.
func (h *History) StableFinal() (map[string]map[string]Stable, []string) {
	out := map[string]map[string]Stable{}
	var missing []string
	for _, gw := range h.Gateways() {
		out[gw] = map[string]Stable{}
		for _, wl := range FinalWorkloads {
			var reads []*Op
			for _, o := range h.finalReads(wl) {
				if o.GW == gw {
					reads = append(reads, o)
				}
			}
			n := params.FinalReadRepeats
			if len(reads) < n {
				missing = append(missing, gw+"/"+wl)
				continue
			}
			last := reads[len(reads)-n:]
			first, err := canonjson.Marshal(last[0].Value)
			stable := err == nil
			var ops []int64
			for _, o := range last {
				b, err := canonjson.Marshal(o.Value)
				if err != nil || !bytes.Equal(b, first) {
					stable = false
				}
				ops = append(ops, o.Index)
			}
			if !stable {
				missing = append(missing, gw+"/"+wl)
				continue
			}
			out[gw][wl] = Stable{Value: last[len(last)-1].Value, Ops: ops}
		}
	}
	return out, missing
}

// faultWindows returns the nemesis fault intervals [first event, last event]
// per fault id, sorted by start. A fault without a closing event lasts until
// the end of the history.
type faultWindow struct {
	ID         string
	Kind       string
	Start, End int64
	Phases     map[string]int64
}

func (h *History) faultWindows() []faultWindow {
	byID := map[string]*faultWindow{}
	var order []string
	for _, ev := range h.Nemesis {
		id := fmt.Sprint(ev.Fault["id"])
		w, ok := byID[id]
		if !ok {
			w = &faultWindow{ID: id, Kind: str(ev.Fault, "kind"), Start: ev.T, End: ev.T, Phases: map[string]int64{}}
			byID[id] = w
			order = append(order, id)
		}
		w.End = max(w.End, ev.T)
		w.Phases[str(ev.Fault, "phase")] = ev.T
	}
	out := make([]faultWindow, 0, len(order))
	for _, id := range order {
		w := byID[id]
		if _, healed := w.Phases["healed"]; !healed {
			if _, nh := w.Phases["not-healed"]; !nh && w.Kind != "F5a" && w.Kind != "F5b" {
				w.End = h.End
			}
		}
		out = append(out, *w)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}
