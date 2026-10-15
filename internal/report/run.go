package report

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
)

// RenderRun renders the run page (ADR-0036). It is a pure function of the
// model.
func RenderRun(m *Model) ([]byte, error) {
	met := m.Metrics()
	data, err := dataBlock(m.data(met))
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	w := &b
	r := m.Run
	verdict := r.Checks.Verdict
	fmt.Fprintf(w, "<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<title>CommitProof report %s</title>\n<style>%s</style>\n</head>\n<body>\n", esc(r.ID), css)
	fmt.Fprintf(w, "<header><h1>Incident report — <span class=\"mono\">%s</span></h1>\n", esc(r.ID))
	fmt.Fprintf(w, "<p>Campaign <b>%s</b> · seed %s · profile %s · build %s · verdict <span class=\"verdict v-%s\">%s</span> (cpcheck)</p>\n",
		esc(str(r.Manifest, "campaign")), esc(str(r.Manifest, "seed")), esc(str(r.Manifest, "profile")), esc(str(r.Manifest, "mutant")), esc(verdict), esc(verdict))
	fmt.Fprintf(w, "<p class=\"small\">%s · rendered from the sealed run only: every number below is computed from <code>history.jsonl</code>, <code>observations.jsonl</code> and cpcheck's <code>checks.json</code>; observations explain, they never decide a verdict (ADR-0016).</p></header>\n", esc(Format))
	fmt.Fprint(w, "<nav><a href=\"#run\">Run</a><a href=\"#timeline\">Timeline</a><a href=\"#faults\">Faults</a><a href=\"#failover\">Failover metrics</a><a href=\"#rules\">Rules and leaders</a><a href=\"#checkers\">Checkers</a><a href=\"#markers\">Markers</a><a href=\"#reading\">How to read</a></nav>\n")
	m.sectionRun(w)
	m.sectionTimeline(w)
	m.sectionFaults(w)
	m.sectionFailover(w, met)
	m.sectionRules(w)
	m.sectionCheckers(w)
	m.sectionMarkers(w)
	m.sectionReading(w)
	fmt.Fprintf(w, "<script type=\"application/json\" id=\"cp-data\">%s</script>\n</body>\n</html>\n", data)
	return b.Bytes(), nil
}

func (m *Model) sectionRun(w io.Writer) {
	r := m.Run
	fmt.Fprint(w, "<h2 id=\"run\">Run</h2>\n<table>\n")
	row := func(k, v string) { fmt.Fprintf(w, "<tr><th>%s</th><td class=\"mono\">%s</td></tr>\n", esc(k), esc(v)) }
	row("run id", r.ID)
	row("seal root (Merkle, seal.json)", r.SealRoot)
	row("sealed files", fmt.Sprint(r.SealedFiles))
	row("checks.json sha256", r.ChecksSHA256)
	row("history lines", fmt.Sprint(len(r.History)))
	row("observation lines", fmt.Sprint(len(r.Observations)))
	for _, k := range sortedKeys(r.Manifest) {
		v := r.Manifest[k]
		s, ok := v.(string)
		if !ok {
			s = canonText(v)
		}
		row("manifest."+k, s)
	}
	fmt.Fprint(w, "</table>\n<h3>History events by type</h3>\n<table><tr>")
	for _, k := range sortedKeys(m.Counts) {
		fmt.Fprintf(w, "<th>%s</th>", esc(k))
	}
	fmt.Fprint(w, "<th>total</th></tr>\n<tr>")
	for _, k := range sortedKeys(m.Counts) {
		fmt.Fprintf(w, "<td class=\"n\">%d</td>", m.Counts[k])
	}
	fmt.Fprintf(w, "<td class=\"n\">%d</td></tr></table>\n", len(r.History))
	// Operations by workload, function and outcome.
	type key struct{ wl, f string }
	by := map[key]map[string]int64{}
	for _, o := range m.Ops {
		k := key{o.WL, o.F}
		if by[k] == nil {
			by[k] = map[string]int64{}
		}
		by[k][o.Type]++
	}
	keys := make([]key, 0, len(by))
	for k := range by {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		return keys[a].wl < keys[b].wl || keys[a].wl == keys[b].wl && keys[a].f < keys[b].f
	})
	fmt.Fprint(w, "<h3>Operations by workload and outcome</h3>\n<table><tr><th>wl</th><th>f</th><th>ok</th><th>fail</th><th>info</th></tr>\n")
	for _, k := range keys {
		fmt.Fprintf(w, "<tr><td>%s</td><td>%s</td><td class=\"n\">%d</td><td class=\"n\">%d</td><td class=\"n\">%d</td></tr>\n", esc(k.wl), esc(k.f), by[k]["ok"], by[k]["fail"], by[k]["info"])
	}
	fmt.Fprint(w, "</table>\n<h3>Observations by source</h3>\n<table><tr>")
	for _, k := range sortedKeys(m.ObsBySrc) {
		fmt.Fprintf(w, "<th>%s</th>", esc(k))
	}
	fmt.Fprint(w, "</tr>\n<tr>")
	for _, k := range sortedKeys(m.ObsBySrc) {
		fmt.Fprintf(w, "<td class=\"n\">%d</td>", m.ObsBySrc[k])
	}
	fmt.Fprint(w, "</tr></table>\n")
}

func (m *Model) sectionFaults(w io.Writer) {
	fmt.Fprint(w, "<h2 id=\"faults\">Faults</h2>\n")
	if len(m.Faults) == 0 {
		fmt.Fprint(w, "<p>No nemesis event in this run.</p>\n")
		return
	}
	fmt.Fprint(w, "<p>Times in seconds since run start (the runner's monotonic clock). Phases link to their event in the timeline. <i>Confirmed</i> means the §8 predicate held; <i>healed</i> that the heal predicate held.</p>\n")
	fmt.Fprint(w, "<table><tr><th>fault</th><th>kind</th><th>targets</th><th>phases (t, s)</th><th>inject→confirmed</th><th>confirmed→heal</th><th>heal→healed</th></tr>\n")
	for _, f := range m.Faults {
		var ph []string
		for _, ev := range f.Events {
			ph = append(ph, fmt.Sprintf("<a href=\"#e%d\">%s %s</a>", ev.I, esc(str(ev.Fault, "phase")), secs(ev.T)))
		}
		d := func(a, b string) string {
			ta, oka := f.Phase[a]
			tb, okb := f.Phase[b]
			if !oka || !okb {
				return "—"
			}
			return secs(tb-ta) + " s"
		}
		fmt.Fprintf(w, "<tr id=\"fault-%s\"><td class=\"mono\">%s</td><td>%s</td><td>%s</td><td>%s</td><td class=\"n\">%s</td><td class=\"n\">%s</td><td class=\"n\">%s</td></tr>\n",
			esc(f.ID), esc(f.ID), esc(f.Kind), esc(strings.Join(f.Targets, ", ")), strings.Join(ph, "<br>"), d("inject", "confirmed"), d("confirmed", "heal"), d("heal", "healed"))
	}
	fmt.Fprint(w, "</table>\n")
}

func (m *Model) sectionFailover(w io.Writer, met []FaultMetrics) {
	fmt.Fprint(w, "<h2 id=\"failover\">Failover metrics per fault</h2>\n")
	if len(met) == 0 {
		fmt.Fprint(w, "<p>No fault, no failover metric.</p>\n")
		return
	}
	fmt.Fprint(w, "<p>Definitions (ADR-0036; measured, not judged — CONTRACT §3). Operations exclude final reads. <b>Unavailability window</b> (§10.10): from the first <code>fail</code>/<code>info</code> completion through the gateway at or after the injection to the first <code>ok</code> completion at or after the heal's confirmation; every failure counts (40001 included), and the window cannot close before the heal is confirmed. <b>Longest ok gap</b>: the longest interval without an <code>ok</code> completion through the gateway over [inject − 1 s, healed + 10 s]. <b>Latency</b>: ok operations by invoke time — before [inject − 10 s, inject), during [inject, healed], after (healed, healed + 10 s]; nearest-rank percentiles. <b>Rule changes</b>: decided rules first observed from the injection until the next injection (cascade indicator).</p>\n")
	for _, fm := range met {
		f := fm.Fault
		fmt.Fprintf(w, "<h3 id=\"metrics-%s\">%s — %s on %s (inject %s s, end %s s)</h3>\n", esc(f.ID), esc(f.ID), esc(f.Kind), esc(strings.Join(f.Targets, ", ")), secs(f.I), secs(f.E))
		fmt.Fprint(w, "<table><tr><th>gateway</th><th>unavailability window</th><th>from (s)</th><th>to (s)</th><th>longest ok gap</th><th>non-ok completions in gap interval</th></tr>\n")
		for _, g := range m.gatewayRows(fm.Gateways) {
			win, from, to := "none", "—", "—"
			switch g.State {
			case WindowClosed:
				win, from, to = secs(g.R-g.S)+" s", secs(g.S), secs(g.R)
			case WindowOpen:
				win, from, to = "open (no ok after the heal)", secs(g.S), "—"
			}
			fmt.Fprintf(w, "<tr><td>%s</td><td class=\"n\">%s</td><td class=\"n\">%s</td><td class=\"n\">%s</td><td class=\"n\">%s s</td><td class=\"n\">%d</td></tr>\n", esc(g.GW), win, from, to, secs(g.Gap), g.NonOK)
		}
		fmt.Fprint(w, "</table>\n<table><tr><th>phase</th><th>ok ops</th><th>p50 (ms)</th><th>p95 (ms)</th><th>p99 (ms)</th></tr>\n")
		for _, p := range []struct {
			name string
			ph   Phase
		}{{"before", fm.Before}, {"during", fm.During}, {"after", fm.After}} {
			if p.ph.N == 0 {
				fmt.Fprintf(w, "<tr><td>%s</td><td class=\"n\">0</td><td class=\"n\">—</td><td class=\"n\">—</td><td class=\"n\">—</td></tr>\n", p.name)
				continue
			}
			fmt.Fprintf(w, "<tr><td>%s</td><td class=\"n\">%d</td><td class=\"n\">%s</td><td class=\"n\">%s</td><td class=\"n\">%s</td></tr>\n", p.name, p.ph.N, msec(p.ph.P50), msec(p.ph.P95), msec(p.ph.P99))
		}
		fmt.Fprint(w, "</table>\n")
		var rs []string
		for _, r := range fm.Rules {
			s := fmt.Sprintf("<a href=\"#rule-%s\">%s</a> (%s, first seen %s s)", esc(r.Key()), esc(r.Key()), esc(r.Leader), secs(r.FirstSeen))
			if r.LeaderChange() {
				s += " — leader change"
			}
			rs = append(rs, s)
		}
		if len(rs) == 0 {
			rs = []string{"none observed"}
		}
		fmt.Fprintf(w, "<p>Rule changes: <b>%d</b> (leader changes %d): %s</p>\n", len(fm.Rules), fm.LeaderChanges, strings.Join(rs, "; "))
	}
}

func (m *Model) sectionRules(w io.Writer) {
	fmt.Fprint(w, "<h2 id=\"rules\">Decided rules and leader changes (observations)</h2>\n")
	if len(m.Rules) == 0 {
		fmt.Fprint(w, "<p>No decided rule in the observations (a run of profile h1 has no rule model, ADR-0025).</p>\n")
		return
	}
	fmt.Fprint(w, "<p>A rule is observed when a pooler status observation (ok) or the HEALTHY evaluation reports it; ordered by (coordinator term, leader subterm). A leader change is a rule whose leader differs from the previous rule's (ADR-0036).</p>\n")
	fmt.Fprint(w, "<table><tr><th>rule</th><th>leader</th><th>cohort</th><th>first seen (s)</th><th>first reported by</th></tr>\n")
	for _, r := range m.Rules {
		fmt.Fprintf(w, "<tr id=\"rule-%s\" data-rule=\"%s\" data-leader=\"%s\"><td class=\"mono\">%s</td><td>%s</td><td>%s</td><td class=\"n\">%s</td><td>%s</td></tr>\n",
			esc(r.Key()), esc(r.Key()), esc(r.Leader), esc(r.Key()), esc(r.Leader), esc(strings.Join(r.Cohort, ", ")), secs(r.FirstSeen), esc(r.SeenBy))
	}
	fmt.Fprint(w, "</table>\n<h3>Leader changes</h3>\n")
	n := 0
	for _, r := range m.Rules {
		if r.LeaderChange() {
			n++
		}
	}
	if n == 0 {
		fmt.Fprint(w, "<p>None.</p>\n")
		return
	}
	fmt.Fprint(w, "<table><tr><th>rule</th><th>from</th><th>to</th><th>first seen (s)</th></tr>\n")
	for _, r := range m.Rules {
		if r.LeaderChange() {
			fmt.Fprintf(w, "<tr data-leader-change=\"%s\" data-leader=\"%s\"><td class=\"mono\">%s</td><td>%s</td><td>%s</td><td class=\"n\">%s</td></tr>\n",
				esc(r.Key()), esc(r.Leader), esc(r.Key()), esc(r.LeaderPrev), esc(r.Leader), secs(r.FirstSeen))
		}
	}
	fmt.Fprint(w, "</table>\n")
}

// target is the element an index links to: its operation's invoke, or the
// event itself (nemesis, marker).
func (m *Model) target(i int64) int64 {
	if o, ok := m.OpOf[i]; ok {
		return o.Inv
	}
	return i
}

func (m *Model) opLinks(ops []int64) string {
	var out []string
	for _, i := range ops {
		if i < 0 || i >= int64(len(m.Run.History)) {
			out = append(out, fmt.Sprintf("i=%d (not in the history)", i))
			continue
		}
		label := fmt.Sprintf("i=%d", i)
		if o, ok := m.OpOf[i]; ok {
			label = fmt.Sprintf("i=%d (c%d %s %s %s, %s s)", i, o.Client, o.WL, o.F, o.Type, secs(o.TInv))
		}
		out = append(out, fmt.Sprintf("<a href=\"#e%d\">%s</a>", m.target(i), esc(label)))
	}
	return strings.Join(out, ", ")
}

func (m *Model) sectionCheckers(w io.Writer) {
	c := m.Run.Checks
	fmt.Fprintf(w, "<h2 id=\"checkers\">Checkers (cpcheck) — verdict <span class=\"verdict v-%s\">%s</span></h2>\n", esc(c.Verdict), esc(c.Verdict))
	fmt.Fprint(w, "<p>Each anomaly links to the operations it names (history line indices; a link selects the operation in the timeline). checks.json lists at most 20 anomalies per class; <code>stats.counts</code> holds the totals.</p>\n")
	fmt.Fprint(w, "<table><tr><th>checker</th><th>valid</th><th>anomalies listed</th><th>observations listed</th></tr>\n")
	for _, ch := range c.Checkers {
		fmt.Fprintf(w, "<tr><td><a href=\"#checker-%s\">%s</a></td><td>%s</td><td class=\"n\">%d</td><td class=\"n\">%d</td></tr>\n", esc(ch.ID), esc(ch.ID), esc(ch.Valid), len(ch.Anomalies), len(ch.Observations))
	}
	fmt.Fprint(w, "</table>\n")
	for _, ch := range c.Checkers {
		fmt.Fprintf(w, "<h3 id=\"checker-%s\">%s — valid: %s</h3>\n", esc(ch.ID), esc(ch.ID), esc(ch.Valid))
		if len(ch.Anomalies) > 0 {
			fmt.Fprint(w, "<ol class=\"anoms\">\n")
			for j, a := range ch.Anomalies {
				fmt.Fprintf(w, "<li id=\"anomaly-%s-%d\" data-checker=\"%s\" data-class=\"%s\" data-ops=\"%s\"><b>%s</b> — %s<br>ops: %s</li>\n",
					esc(ch.ID), j, esc(ch.ID), esc(a.Class), joinInts(a.Ops), esc(a.Class), esc(a.Explanation), m.opLinks(a.Ops))
			}
			fmt.Fprint(w, "</ol>\n")
		}
		if len(ch.Observations) > 0 {
			fmt.Fprint(w, "<p>Observations (reported, never a violation):</p>\n<ol class=\"anoms\">\n")
			for j, a := range ch.Observations {
				fmt.Fprintf(w, "<li id=\"observation-%s-%d\" data-observation-checker=\"%s\" data-observation-class=\"%s\"><b>%s</b> — %s<br>ops: %s</li>\n",
					esc(ch.ID), j, esc(ch.ID), esc(a.Class), esc(a.Class), esc(a.Explanation), m.opLinks(a.Ops))
			}
			fmt.Fprint(w, "</ol>\n")
		}
		fmt.Fprintf(w, "<details><summary>stats</summary><pre>%s</pre></details>\n", esc(canonText(ch.Stats)))
	}
}

func (m *Model) sectionMarkers(w io.Writer) {
	fmt.Fprint(w, "<h2 id=\"markers\">Markers</h2>\n<table><tr><th>i</th><th>t (s)</th><th>name</th><th>value</th></tr>\n")
	for _, ev := range m.Markers {
		v, _ := ev.Value.(map[string]any)
		rest := map[string]any{}
		for k, x := range v {
			if k != "name" {
				rest[k] = x
			}
		}
		fmt.Fprintf(w, "<tr><td class=\"n\"><a href=\"#e%d\">%d</a></td><td class=\"n\">%s</td><td>%s</td><td class=\"mono\">%s</td></tr>\n", ev.I, ev.I, secs(ev.T), esc(str(v, "name")), esc(clip(canonText(rest), 400)))
	}
	fmt.Fprint(w, "</table>\n")
}

func (m *Model) sectionReading(w io.Writer) {
	fmt.Fprint(w, "<h2 id=\"reading\">How to read this report</h2>\n<ul>\n")
	fmt.Fprint(w, "<li><b>Timeline.</b> One lane per client; each bar is one operation from its invoke to its completion, coloured by outcome (<span class=\"sw ok\" style=\"background:var(--ok)\"></span>ok, <span class=\"sw\" style=\"background:var(--fail)\"></span>fail: proven not to take effect, <span class=\"sw\" style=\"background:var(--info)\"></span>info: unknown outcome; CONTRACT §6.2). Hover for details. Shaded bands are fault windows (inject → healed); dashed lines are markers.</li>\n")
	fmt.Fprint(w, "<li><b>Observations</b> (rows above the lanes and charts below) come from the runner's polling of pooler status RPCs every second (P24) and other diagnosis sources; they explain what the system did, never a verdict.</li>\n")
	_, scraped := m.ObsBySrc["metrics"]
	if scraped {
		fmt.Fprint(w, "<li><b>/metrics scrapes</b> are present in this run's observations.</li>\n")
	} else {
		fmt.Fprint(w, "<li><b>/metrics scrapes (P23)</b>: none in this run's observations — the observer records pooler status (P24) only, so no Prometheus metric timeline can be drawn (ADR-0036, LIMITATIONS LIM-40). The node rows and charts show what the observations hold: role and reachability, WAL position, status-probe latency.</li>\n")
	}
	fmt.Fprint(w, "<li><b>Verdicts</b> are cpcheck's, from the sealed history alone (CONTRACT §5.3); this page re-states them and links each anomaly to its operations.</li>\n")
	fmt.Fprint(w, "<li><b>Data.</b> The per-fault metrics are embedded as canonical JSON in the element <code>#cp-data</code>; cpgate recomputes them from the raw history (K6.5).</li>\n</ul>\n")
}

// data is the embedded data block (K6.5 reads it).
func (m *Model) data(met []FaultMetrics) map[string]any {
	faults := []any{}
	for _, fm := range met {
		f := fm.Fault
		phases := map[string]any{}
		for k, t := range f.Phase {
			phases[k] = t
		}
		gws := map[string]any{}
		for _, g := range fm.Gateways {
			x := map[string]any{"window": g.State, "gap_ns": g.Gap, "non_ok": g.NonOK}
			if g.State != WindowNone {
				x["window_start"] = g.S
			}
			if g.State == WindowClosed {
				x["window_end"], x["window_ns"] = g.R, g.R-g.S
			}
			gws[g.GW] = x
		}
		lat := map[string]any{}
		for name, p := range map[string]Phase{"before": fm.Before, "during": fm.During, "after": fm.After} {
			x := map[string]any{"n": p.N}
			if p.N > 0 {
				x["p50"], x["p95"], x["p99"] = p.P50, p.P95, p.P99
			}
			lat[name] = x
		}
		rules := []any{}
		for _, r := range fm.Rules {
			rules = append(rules, r.Key())
		}
		targets := make([]any, len(f.Targets))
		for i, t := range f.Targets {
			targets[i] = t
		}
		faults = append(faults, map[string]any{"id": f.ID, "kind": f.Kind, "targets": targets, "phases": phases,
			"inject": f.I, "end": f.E, "gateways": gws, "latency": lat, "rule_changes": int64(len(fm.Rules)),
			"leader_changes": fm.LeaderChanges, "rules": rules})
	}
	rules := []any{}
	changes := []any{}
	for _, r := range m.Rules {
		cohort := make([]any, len(r.Cohort))
		for i, c := range r.Cohort {
			cohort[i] = c
		}
		rules = append(rules, map[string]any{"rule": r.Key(), "leader": r.Leader, "cohort": cohort, "first_seen": r.FirstSeen})
		if r.LeaderChange() {
			changes = append(changes, map[string]any{"rule": r.Key(), "from": r.LeaderPrev, "to": r.Leader, "t": r.FirstSeen})
		}
	}
	counts := map[string]any{}
	for k, v := range m.Counts {
		counts[k] = v
	}
	return map[string]any{"format": Format, "run_id": m.Run.ID, "seal_root": m.Run.SealRoot, "checks_sha256": m.Run.ChecksSHA256,
		"verdict": m.Run.Checks.Verdict, "history_lines": int64(len(m.Run.History)), "observation_lines": int64(len(m.Run.Observations)),
		"events_by_type": counts, "operations": int64(len(m.Ops)), "faults": faults, "rules": rules, "leader_changes": changes}
}
