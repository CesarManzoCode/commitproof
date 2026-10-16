package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Metric scrapes (CONTRACT P23, ADR-0038): one observation per scrape of a
// target's /metrics, reduced by the observer to ADR-0038's integer
// families. A run without them renders no metrics section, no navigation
// link and no data-block key (the K6 example stays byte-identical).

// MetricObs is one scrape of a target.
type MetricObs struct {
	T  int64
	OK bool
	V  map[string]int64
}

// MetricSummary is one family's summary over a target's ok scrapes.
type MetricSummary struct {
	Counter                  bool
	N, First, Last, Min, Max int64
	Increase                 int64 // counters: Σ (vᵢ − vᵢ₋₁, or vᵢ after a decrease)
	samples                  []MetricObs
}

func metricCounter(key string) bool {
	for _, f := range params.MetricFamilies {
		if f.Key == key {
			return f.Counter
		}
	}
	return false
}

// metricKeys returns the family keys present in the run: ADR-0038's order,
// then any other key in sorted order.
func (m *Model) metricKeys() []string {
	present := map[string]bool{}
	for _, t := range sortedKeys(m.Scrapes) {
		for _, o := range m.Scrapes[t] {
			for k := range o.V {
				present[k] = true
			}
		}
	}
	var out []string
	for _, f := range params.MetricFamilies {
		if present[f.Key] {
			out = append(out, f.Key)
			delete(present, f.Key)
		}
	}
	return append(out, sortedKeys(present)...)
}

// metricSamples returns target's ok scrapes holding key, in observation
// order — without the first one in the sabotage build (the metrics
// section's checker-level negative control, ADR-0038).
func (m *Model) metricSamples(target, key string) []MetricObs {
	var out []MetricObs
	skipped := !sabotageMetrics
	for _, o := range m.Scrapes[target] {
		if !o.OK {
			continue
		}
		if _, ok := o.V[key]; !ok {
			continue
		}
		if !skipped {
			skipped = true
			continue
		}
		out = append(out, o)
	}
	return out
}

// metricSummary summarizes key over target's samples (ADR-0038).
func (m *Model) metricSummary(target, key string) (MetricSummary, bool) {
	ss := m.metricSamples(target, key)
	if len(ss) == 0 {
		return MetricSummary{}, false
	}
	s := MetricSummary{Counter: metricCounter(key), N: int64(len(ss)), First: ss[0].V[key], Last: ss[len(ss)-1].V[key],
		Min: ss[0].V[key], Max: ss[0].V[key], samples: ss}
	for i, o := range ss {
		v := o.V[key]
		s.Min, s.Max = min(s.Min, v), max(s.Max, v)
		if i > 0 && s.Counter {
			if p := ss[i-1].V[key]; v >= p {
				s.Increase += v - p
			} else {
				s.Increase += v
			}
		}
	}
	return s, true
}

// metricsData is the data block's "metrics" object (ADR-0038).
func (m *Model) metricsData() map[string]any {
	out := map[string]any{}
	keys := m.metricKeys()
	for _, t := range sortedKeys(m.Scrapes) {
		var failed int64
		for _, o := range m.Scrapes[t] {
			if !o.OK {
				failed++
			}
		}
		fams := map[string]any{}
		for _, k := range keys {
			s, ok := m.metricSummary(t, k)
			if !ok {
				continue
			}
			x := map[string]any{"kind": "gauge", "n": s.N, "first": s.First, "last": s.Last, "min": s.Min, "max": s.Max}
			if s.Counter {
				x["kind"], x["increase"] = "counter", s.Increase
			}
			fams[k] = x
		}
		out[t] = map[string]any{"scrapes": int64(len(m.Scrapes[t])), "failed": failed, "families": fams}
	}
	return out
}

func (m *Model) sectionMetrics(w io.Writer) {
	fmt.Fprint(w, "<h2 id=\"metrics\">Metrics (/metrics scrapes, P23)</h2>\n")
	fmt.Fprint(w, "<p>Each target's OpenTelemetry Prometheus exporter was scraped every second (ADR-0038); each scrape is reduced to integer families (sums over their series). Gauges are drawn as values; counters as their per-second increase between consecutive scrapes (a decrease is a process restart: the increase is the new value). Lines break at failed scrapes and gaps over 2 s; shaded bands are fault windows (inject → healed). Observations explain; they never decide a verdict (ADR-0016).</p>\n")
	targets := sortedKeys(m.Scrapes)
	fmt.Fprint(w, "<table><tr><th>target</th><th>scrapes</th><th>failed</th><th>first (s)</th><th>last (s)</th></tr>\n")
	for _, t := range targets {
		obs := m.Scrapes[t]
		var failed int64
		for _, o := range obs {
			if !o.OK {
				failed++
			}
		}
		fmt.Fprintf(w, "<tr><td>%s</td><td class=\"n\">%d</td><td class=\"n\">%d</td><td class=\"n\">%s</td><td class=\"n\">%s</td></tr>\n",
			esc(t), len(obs), failed, secs(obs[0].T), secs(obs[len(obs)-1].T))
	}
	fmt.Fprint(w, "</table>\n")
	width := px(m.End) + rightPad
	for _, k := range m.metricKeys() {
		counter := metricCounter(k)
		unit := "value"
		if counter {
			unit = "per-second increase"
		}
		var ts []string
		for _, t := range targets {
			if _, ok := m.metricSummary(t, k); ok {
				ts = append(ts, t)
			}
		}
		fmt.Fprintf(w, "<h3 id=\"metric-%s\">%s — %s</h3>\n", esc(k), esc(k), unit)
		m.metricChart(w, k, ts, width, counter)
		fmt.Fprint(w, "<table><tr><th>target</th><th>samples</th><th>first</th><th>last</th><th>min</th><th>max</th><th>increase</th></tr>\n")
		for _, t := range ts {
			s, _ := m.metricSummary(t, k)
			inc := "—"
			if s.Counter {
				inc = fmt.Sprint(s.Increase)
			}
			fmt.Fprintf(w, "<tr data-metric=\"%s\" data-target=\"%s\"><td>%s</td><td class=\"n\">%d</td><td class=\"n\">%d</td><td class=\"n\">%d</td><td class=\"n\">%d</td><td class=\"n\">%d</td><td class=\"n\">%s</td></tr>\n",
				esc(k), esc(t), esc(t), s.N, s.First, s.Last, s.Min, s.Max, inc)
		}
		fmt.Fprint(w, "</table>\n")
	}
}

// metricChart draws one family: one polyline per target, scaled to the
// drawn range; x on the timeline's scale.
func (m *Model) metricChart(w io.Writer, key string, targets []string, width int64, counter bool) {
	type pt struct{ t, v int64 }
	runs := map[string][][]pt{}
	lo, hi := int64(0), int64(0)
	first := true
	for _, t := range targets {
		ss, _ := m.metricSummary(t, key)
		var cur []pt
		var prev *MetricObs
		lastT := int64(-1)
		// A failed scrape between two samples breaks the line.
		broken := func(a, b int64) bool {
			for _, o := range m.Scrapes[t] {
				if !o.OK && o.T > a && o.T < b {
					return true
				}
			}
			return b-a > 2_000_000_000
		}
		for i := range ss.samples {
			o := ss.samples[i]
			if lastT >= 0 && broken(lastT, o.T) {
				if len(cur) > 0 {
					runs[t] = append(runs[t], cur)
				}
				cur, prev = nil, nil
			}
			lastT = o.T
			v := o.V[key]
			if counter {
				if prev == nil {
					prev = &ss.samples[i]
					continue
				}
				d := v - prev.V[key]
				if d < 0 {
					d = v
				}
				v = d * 1_000_000_000 / max(o.T-prev.T, 1)
				prev = &ss.samples[i]
			}
			if first || v < lo {
				lo = v
			}
			if first || v > hi {
				hi = v
			}
			first = false
			cur = append(cur, pt{o.T, v})
		}
		if len(cur) > 0 {
			runs[t] = append(runs[t], cur)
		}
	}
	height := int64(chartH + 20)
	base := int64(chartH + 6)
	fmt.Fprintf(w, "<div class=\"scroll\"><svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\" role=\"img\" aria-label=\"%s\">\n", width, height, width, height, esc(key))
	for _, f := range m.Faults {
		fmt.Fprintf(w, "<rect class=\"fb\" x=\"%d\" y=\"0\" width=\"%d\" height=\"%d\"/>\n", px(f.I), max(px(f.E)-px(f.I), 1), base)
	}
	fmt.Fprintf(w, "<line class=\"axis\" x1=\"%d\" y1=\"%d\" x2=\"%d\" y2=\"%d\"/><text class=\"lbl\" x=\"4\" y=\"12\">%s</text><text class=\"lbl\" x=\"4\" y=\"26\">%d … %d</text>\n",
		leftW, base, width-rightPad, base, esc(clip(key, 30)), lo, hi)
	for t := int64(0); t <= m.End; t += 2 * tickEvery {
		fmt.Fprintf(w, "<text x=\"%d\" y=\"%d\">%d s</text>", px(t)+2, height-2, t/1_000_000_000)
	}
	fmt.Fprint(w, "\n")
	span := hi - lo
	for ti, t := range targets {
		color := palette[ti%len(palette)]
		for _, r := range runs[t] {
			var pts []string
			for _, p := range r {
				yy := base - (chartH-4)/2
				if span > 0 {
					yy = base - (p.v-lo)*(chartH-4)/span
				}
				pts = append(pts, fmt.Sprintf("%d,%d", px(p.t), yy))
			}
			fmt.Fprintf(w, "<polyline class=\"series\" stroke=\"%s\" points=\"%s\"><title>%s %s</title></polyline>\n", color, strings.Join(pts, " "), esc(t), esc(key))
		}
		fmt.Fprintf(w, "<text x=\"%d\" y=\"%d\" fill=\"%s\">%s</text>\n", leftW-90, 40+int64(ti)*10, color, esc(t))
	}
	fmt.Fprint(w, "</svg></div>\n")
}
