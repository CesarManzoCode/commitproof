package synth

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/history"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// CorpusContractVersion is the contract version the K2 corpus was generated
// and validated under. The corpus is committed evidence referenced by seal
// root (gate K2), so a contract change that leaves the history model and
// the checkers alone (§6, §10) must not regenerate it; a change to either
// regenerates it and re-runs gate K2 (CONTRACT §15 K4 "Do not change").
const CorpusContractVersion = "1.0.3"

// Times are nanoseconds since run start. Workload operations sit on a
// microsecond grid; planted ("gadget") operations are offset by 1–999 ns so
// that no two events of different operations share a timestamp.
const (
	us = int64(1000)
	ms = 1000 * us
	s_ = 1000 * ms
)

// Gateways in cell order (CONTRACT §5.1).
var Gateways = []string{"gw-z1", "gw-z2", "gw-z3"}

// Op is one operation of a synthetic history.
type Op struct {
	Client    int64
	Proc      int64
	FixedProc bool
	GW, WL, F string
	Iso       string
	In        any
	Out       any
	Type      string
	Err       map[string]any
	Notices   []any
	Inv, Eff  int64
	Cmp       int64
	// Applied: the effect happened (ok: always; info: decided by the
	// generator; fail: never).
	Applied bool
	// Exec applies the effect at Eff and returns the ok value; nil for
	// operations whose value is preset (gadgets).
	Exec func(s *Store, at int64) any
}

// Expect is one checker's expectation in expected.json.
type Expect struct {
	Valid        any      `json:"valid"`
	Anomalies    []string `json:"anomalies"`
	Observations []string `json:"observations"`
}

// Run is one synthetic run under construction.
type Run struct {
	ID, Family, Label string
	Iso, Profile      string
	Workloads         []string
	Elle              bool
	Plant             string
	R                 *rand.Rand
	S                 *Store
	Ops               []*Op
	marks             []event
	nem               []event
	procNext          int64
	T0                int64
	// FinalOverride replaces final-read values per gateway and workload.
	FinalOverride map[string]map[string]any
	// Observers run after the recovery writes, with the time of the end of
	// the final reads, to generate operations that watch commits (W6, W7).
	Observers []func(r *Run, end int64)
	// PreFinal hooks run before the final-read snapshot; Post hooks run
	// after every operation exists (to alter recorded results).
	PreExec  []func(r *Run)
	PreFinal []func(r *Run)
	Post     []func(r *Run)
	// Meta holds generator bookkeeping per op (never written).
	Meta      map[*Op]map[string]any
	Expected  map[string]*Expect
	Verdict   string
	extraKeys map[int64]bool
	// Version is the contract version the entry is generated under (""
	// = CorpusContractVersion, the K2 corpus; K5CorpusContractVersion for
	// testdata/corpus-k5).
	Version string
}

func (r *Run) version() string {
	if r.Version != "" {
		return r.Version
	}
	return CorpusContractVersion
}

// CheckerIDs are the checkers of CONTRACT §10 in checks.json order.
var CheckerIDs = []string{"C1", "C2", "C3", "C4", "C5", "C6", "C7", "C8", "C9"}

func seedOf(id string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(id))
	return h.Sum64()
}

// NewRun starts a run whose randomness is a pure function of its id.
func NewRun(id, family, label string) *Run {
	r := &Run{ID: id, Family: family, Label: label, Profile: "baseline", Iso: "serializable",
		R: rand.New(rand.NewPCG(seedOf(id), 0x6b32636f72707573)), S: NewStore(params.BankAccounts, params.BankInitialBalance),
		procNext: 100, T0: 20 * ms, FinalOverride: map[string]map[string]any{}, Meta: map[*Op]map[string]any{}, Expected: map[string]*Expect{}, extraKeys: map[int64]bool{}}
	for _, c := range CheckerIDs {
		r.Expected[c] = &Expect{Valid: true, Anomalies: []string{}, Observations: []string{}}
	}
	return r
}

// Expect adds expected anomaly classes to a checker.
func (r *Run) Expect(checker string, classes ...string) {
	e := r.Expected[checker]
	e.Anomalies = append(e.Anomalies, classes...)
	e.Valid = len(e.Anomalies) == 0
}

// Observe adds expected observation classes to a checker.
func (r *Run) Observe(checker string, classes ...string) {
	e := r.Expected[checker]
	e.Observations = append(e.Observations, classes...)
}

// c3Forbidden is the expected model of CONTRACT §10.3 (G5), written here
// independently of the checker: under serializable every base class is
// forbidden; under repeatable-read (snapshot isolation) G2-item is allowed.
// Suffixed (-process, -realtime) classes are observations.
func c3Forbidden(iso, class string) bool {
	switch class {
	case "G0", "g1a", "g1b", "G1c", "G-single", "G-nonadjacent", "duplicate-elements", "incompatible-order", "internal", "dirty-update":
		return true
	case "G2-item":
		return iso == "serializable"
	}
	return false
}

// ExpectC3 files C3 classes as anomalies or observations by the model.
func (r *Run) ExpectC3(classes ...string) {
	for _, c := range classes {
		if c3Forbidden(r.Iso, c) {
			r.Expect("C3", c)
		} else {
			r.Observe("C3", c)
		}
	}
}

// NewProc allocates a fresh logical process.
func (r *Run) NewProc() int64 {
	p := r.procNext
	r.procNext++
	return p
}

// Mark adds a runner marker.
func (r *Run) Mark(t int64, name string, v map[string]any) {
	val := map[string]any{"name": name}
	for k, x := range v {
		val[k] = x
	}
	r.marks = append(r.marks, event{t: t, fields: map[string]any{"type": history.TypeMarker, "value": val}})
}

// Nemesis adds a nemesis event (CONTRACT §6.1 fault object).
func (r *Run) Nemesis(t int64, id, kind, phase string, targets ...string) {
	ts := make([]any, len(targets))
	for i, x := range targets {
		ts[i] = x
	}
	r.nem = append(r.nem, event{t: t, fields: map[string]any{"type": history.TypeNemesis,
		"fault": map[string]any{"id": id, "kind": kind, "phase": phase, "targets": ts, "evidence": map[string]any{"synthetic": true}}}})
}

// Loop schedules one client's closed-loop operations (P03 pacing) from start
// until end: gen fills each op's payload, outcome and Exec and returns
// false to stop.
func (r *Run) Loop(client int64, gw, wl string, start, end, dmin, dmax int64, gen func(o *Op) bool) {
	t := start + int64(r.R.IntN(40))*us
	for t < end {
		d := dmin + int64(r.R.Int64N((dmax-dmin)/us+1))*us
		o := &Op{Client: client, GW: gw, WL: wl, Inv: t, Cmp: t + d, Type: history.TypeOK, Applied: true}
		lo, hi := o.Inv+d/5, o.Cmp-d/5
		o.Eff = lo + int64(r.R.Int64N((hi-lo)/us+1))*us
		if !gen(o) {
			return
		}
		r.Ops = append(r.Ops, o)
		t = max(o.Inv+params.Pacing.Nanoseconds(), o.Cmp+us)
	}
}

// Fail marks o as failed with err (no effect).
func (o *Op) Fail(sqlstate, class, phase string) {
	o.Type, o.Applied = history.TypeFail, false
	o.Err = map[string]any{"sqlstate": sqlstate, "class": class, "phase": phase}
}

// Info marks o as indeterminate; applied decides whether it took effect.
func (o *Op) Info(applied bool) {
	o.Type, o.Applied = history.TypeInfo, applied
	o.Err = map[string]any{"sqlstate": "", "class": "timeout", "phase": "client-timeout"}
}

// Outcome draws a random outcome: fail with probability pf (err from
// failErr), info with probability pi (applied with probability 1/2).
func (r *Run) Outcome(o *Op, pf, pi float64, failErr [3]string) {
	x := r.R.Float64()
	switch {
	case x < pf:
		o.Fail(failErr[0], failErr[1], failErr[2])
	case x < pf+pi:
		o.Info(r.R.IntN(2) == 0)
	}
}

// Common errors.
var (
	Err40001Stmt   = [3]string{"40001", "40", "statement"}
	Err40001Commit = [3]string{"40001", "40", "commit"}
	ErrConnect     = [3]string{"", "connect", "connect"}
)

// execute applies effects in effect-time order: the store's serial order.
func (r *Run) execute(ops []*Op) {
	sorted := append([]*Op{}, ops...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Eff < sorted[j].Eff })
	for _, o := range sorted {
		if !o.Applied || o.Exec == nil {
			continue
		}
		out := o.Exec(r.S, o.Eff)
		if o.Type == history.TypeOK {
			o.Out = out
		}
	}
}

// assignProcs gives every client's operations its process number: the client
// index, then a fresh process after each info completion (Jepsen).
func (r *Run) assignProcs() {
	byClient := map[int64][]*Op{}
	for _, o := range r.Ops {
		if !o.FixedProc {
			byClient[o.Client] = append(byClient[o.Client], o)
		}
	}
	for _, c := range keys(byClient) {
		os := byClient[c]
		sort.SliceStable(os, func(i, j int) bool { return os[i].Inv < os[j].Inv })
		p := c
		for _, o := range os {
			o.Proc = p
			if o.Type == history.TypeInfo {
				p = r.NewProc()
			}
		}
	}
}

// Finish executes the workload, runs recovery writes, observers and final
// reads like the runner (CONTRACT §6.4, P06–P08), and returns the history
// events in order.
func (r *Run) Finish() []map[string]any {
	r.Mark(100*us, "run-start", map[string]any{"contract_version": r.version(), "synthetic": true})
	r.Mark(1*ms, "setup-begin", map[string]any{"gw": "gw-z1"})
	r.Mark(13*ms, "setup-end", map[string]any{"result": "ok"})
	r.Mark(r.T0-us, "workload-start", nil)
	for _, f := range r.PreExec {
		f(r)
	}
	r.execute(r.Ops)
	end := r.T0
	for _, o := range r.Ops {
		end = max(end, o.Cmp)
	}
	t := end + ms
	r.Mark(t, "workload-end", map[string]any{"open_ops_closed": int64(0)})
	r.Mark(t+us, "recovery-begin", nil)
	var rec []*Op
	for g, gw := range Gateways {
		id := 9_000_000_000 + int64(g)*1_000_000
		o := &Op{Client: 10 + int64(g), Proc: 10 + int64(g), FixedProc: true, GW: gw, WL: "ledger", F: "insert",
			In: map[string]any{"id": id, "payload": fmt.Sprintf("recovery-%s-0", gw)}, Type: history.TypeOK, Applied: true,
			Inv: t + 2*us + int64(g)*10*us, Eff: t + 1*ms + int64(g)*10*us, Cmp: t + 2*ms + int64(g)*10*us}
		in := o.In.(map[string]any)
		o.Exec = func(s *Store, at int64) any { return s.insert(at, o, in) }
		rec = append(rec, o)
	}
	r.execute(rec)
	r.Ops = append(r.Ops, rec...)
	t += 3 * ms
	r.Mark(t, "recovery-end", map[string]any{"recovered": []any{true, true, true}})
	r.Mark(t+us, "quiescence-begin", map[string]any{"ms": params.Quiescence.Milliseconds()})
	fr0 := t + params.Quiescence.Nanoseconds()
	frEnd := fr0 + int64(params.FinalReadRepeats-1)*params.FinalReadInterval.Nanoseconds() + 20*ms
	for _, obs := range r.Observers {
		obs(r, frEnd)
	}
	for _, f := range r.Post {
		f(r)
	}
	r.assignProcs()
	r.Mark(fr0, "final-reads-begin", nil)
	for _, f := range r.PreFinal {
		f(r)
	}
	final := r.S.Final()
	for round := 0; round < params.FinalReadRepeats; round++ {
		rt := fr0 + int64(round)*params.FinalReadInterval.Nanoseconds()
		for g, gw := range Gateways {
			for j, wl := range []string{"append", "register", "bank", "ledger"} {
				v := final[wl]
				if ov, ok := r.FinalOverride[gw][wl]; ok {
					v = ov
				}
				inv := rt + int64(g)*50*us + int64(j)*2*ms + us
				r.Ops = append(r.Ops, &Op{Client: 10 + int64(g), Proc: 10 + int64(g), FixedProc: true, GW: gw, WL: wl, F: "final-read",
					In: map[string]any{}, Out: v, Type: history.TypeOK, Applied: true, Inv: inv, Eff: inv + 500*us, Cmp: inv + ms})
			}
			if round == params.FinalReadRepeats-1 {
				r.Mark(rt+9*ms+int64(g)*us, "final-read-stable", map[string]any{"gw": gw, "rounds": int64(params.FinalReadRepeats)})
			}
		}
	}
	r.Mark(frEnd, "final-reads-end", map[string]any{"stable": []any{true, true, true}})
	r.Mark(frEnd+us, "run-end", map[string]any{"status": "complete", "reason": "", "closed_open_ops": int64(0)})
	return r.events()
}

type event struct {
	t      int64
	seq    int
	fields map[string]any
	op     *Op
	inv    *event
}

func (r *Run) events() []map[string]any {
	var evs []*event
	seq := 0
	add := func(e *event) *event {
		e.seq = seq
		seq++
		evs = append(evs, e)
		return e
	}
	for i := range r.marks {
		add(&r.marks[i])
	}
	for i := range r.nem {
		add(&r.nem[i])
	}
	for _, o := range r.Ops {
		must(o.Cmp > o.Inv, "%s op %s/%s: completion %d not after invoke %d", r.ID, o.WL, o.F, o.Cmp, o.Inv)
		inv := map[string]any{"type": history.TypeInvoke, "proc": o.Proc, "client": o.Client, "gw": o.GW, "wl": o.WL, "f": o.F, "value": o.In}
		comp := map[string]any{"type": o.Type, "proc": o.Proc, "client": o.Client, "gw": o.GW, "wl": o.WL, "f": o.F, "value": o.In}
		if o.Iso != "" {
			inv["iso"], comp["iso"] = o.Iso, o.Iso
		}
		if o.Type == history.TypeOK {
			comp["value"] = o.Out
		}
		if o.Err != nil {
			comp["err"] = o.Err
		}
		if len(o.Notices) > 0 {
			comp["notices"] = o.Notices
		}
		ie := add(&event{t: o.Inv, fields: inv, op: o})
		add(&event{t: o.Cmp, fields: comp, op: o, inv: ie})
	}
	sort.SliceStable(evs, func(i, j int) bool {
		if evs[i].t != evs[j].t {
			return evs[i].t < evs[j].t
		}
		return evs[i].seq < evs[j].seq
	})
	index := map[*event]int64{}
	out := make([]map[string]any, len(evs))
	for i, e := range evs {
		index[e] = int64(i)
		f := map[string]any{}
		for k, v := range e.fields {
			f[k] = v
		}
		f["t"] = e.t
		if e.inv != nil {
			f["op"] = index[e.inv]
		}
		out[i] = f
	}
	return out
}

// Write writes dir/<id>/run (sealed) and dir/<id>/expected.json.
func (r *Run) Write(dir string) error {
	evs := r.Finish()
	ed := filepath.Join(dir, r.ID)
	rd := filepath.Join(ed, "run")
	if err := os.MkdirAll(rd, 0o755); err != nil {
		return err
	}
	var hb bytes.Buffer
	prev := history.ZeroHash
	for i, ev := range evs {
		ev["i"] = int64(i)
		ev["prev"] = prev
		line, err := canonjson.Marshal(ev)
		if err != nil {
			return fmt.Errorf("%s line %d: %w", r.ID, i, err)
		}
		sum := sha256.Sum256(line)
		prev = hex.EncodeToString(sum[:])
		hb.Write(line)
		hb.WriteByte('\n')
	}
	plan := map[string]any{"campaign": "synthetic", "contract_version": r.version(), "profile": r.Profile,
		"seed": int64(seedOf(r.ID) >> 1), "directed": "", "clients": []any{}, "nemesis": []any{},
		"setup": map[string]any{"append_active_keys": params.AppendActiveKeys, "append_retire_at": params.AppendKeyRetireAt,
			"bank_accounts": params.BankAccounts, "bank_initial": params.BankInitialBalance, "register_keys": params.RegisterKeys},
		"synthetic": map[string]any{"family": r.Family, "label": r.Label, "generator": "internal/synth"}}
	pb, err := canonjson.Marshal(plan)
	if err != nil {
		return err
	}
	ps := sha256.Sum256(pb)
	man := map[string]any{"contract_version": r.version(), "run_id": r.ID, "campaign": "synthetic", "profile": r.Profile,
		"seed": plan["seed"], "mutant": "none", "plan_sha256": hex.EncodeToString(ps[:]), "synthetic": true,
		"generator": "internal/synth (cmd: go run ./internal/tools/gencorpus)"}
	mb, err := canonjson.Marshal(man)
	if err != nil {
		return err
	}
	files := map[string][]byte{history.FileManifest: append(mb, '\n'), history.FilePlan: append(pb, '\n'),
		history.FileHistory: hb.Bytes(), history.FileObservations: nil, history.FileCommands: nil}
	for _, name := range []string{history.FileManifest, history.FilePlan, history.FileHistory, history.FileObservations, history.FileCommands} {
		if err := os.WriteFile(filepath.Join(rd, name), files[name], 0o644); err != nil {
			return err
		}
	}
	if _, err := history.SealDir(rd); err != nil {
		return err
	}
	verdict := "PASS"
	for _, c := range CheckerIDs {
		e := r.Expected[c]
		sort.Strings(e.Anomalies)
		sort.Strings(e.Observations)
		e.Anomalies, e.Observations = dedup(e.Anomalies), dedup(e.Observations)
		if v, ok := e.Valid.(bool); ok && !v {
			verdict = "FAIL"
		}
	}
	for _, c := range CheckerIDs {
		if s, ok := r.Expected[c].Valid.(string); ok && s == "unknown" && verdict == "PASS" {
			verdict = "UNKNOWN"
		}
	}
	if r.Verdict != "" {
		verdict = r.Verdict
	}
	wls := append([]string{}, r.Workloads...)
	sort.Strings(wls)
	exp := map[string]any{"id": r.ID, "family": r.Family, "label": r.Label, "workloads": wls, "iso": r.Iso,
		"profile": r.Profile, "elle": r.Elle, "plant": r.Plant, "checkers": r.Expected, "verdict": verdict,
		"derivation": "expected classes are written per plant from CONTRACT §10 (internal/synth), never from running cpcheck"}
	eb, err := json.MarshalIndent(exp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ed, "expected.json"), append(eb, '\n'), 0o644)
}

func dedup(xs []string) []string {
	out := []string{}
	for i, x := range xs {
		if i == 0 || x != xs[i-1] {
			out = append(out, x)
		}
	}
	return out
}
