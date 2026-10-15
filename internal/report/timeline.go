package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Timeline geometry: integer SVG coordinates only (§15 K6).
const (
	nsPerPx   = 50_000_000 // 20 px per second
	leftW     = 200        // label column
	rightPad  = 24
	axisH     = 26
	rowH      = 14
	faultRowH = 26 // label line above the phase band
	laneH     = 10
	chartH    = 70
	chartGap  = 18
	tickEvery = 5_000_000_000 // ns
)

var palette = []string{"#1f77b4", "#ff7f0e", "#2ca02c", "#9467bd", "#8c564b", "#e377c2", "#17becf", "#7f7f7f", "#bcbd22", "#393b79"}

func px(t int64) int64 { return leftW + t/nsPerPx }

// laneOrder returns the clients in ascending order — or, in the sabotage
// build, in map iteration order (K6.1's checker-level negative control).
func (m *Model) laneOrder() []int64 {
	if sabotageNondeterminism {
		seen := map[int64]bool{}
		for _, o := range m.Ops {
			seen[o.Client] = true
		}
		out := make([]int64, 0, len(seen))
		for c := range seen { // deliberately unsorted (sabotage)
			out = append(out, c)
		}
		return out
	}
	return m.Clients()
}

// gatewayRows returns the per-gateway metrics in gateway order — or, in
// the sabotage build, in map iteration order.
func (m *Model) gatewayRows(gs []GatewayMetric) []GatewayMetric {
	if !sabotageNondeterminism {
		return gs
	}
	by := map[string]GatewayMetric{}
	for _, g := range gs {
		by[g.GW] = g
	}
	out := make([]GatewayMetric, 0, len(by))
	for _, g := range by { // deliberately unsorted (sabotage)
		out = append(out, g)
	}
	return out
}

// faultRows packs faults into rows so that overlapping windows (composite
// steps) do not overlap on screen: first fit, in fault order.
func (m *Model) faultRows() [][]*Fault {
	var rows [][]*Fault
	var ends []int64
	for _, f := range m.Faults {
		hi := f.E
		if t := f.Events[len(f.Events)-1].T; t > hi {
			hi = t
		}
		placed := false
		for i := range rows {
			if f.I > ends[i]+4_000_000_000 { // ≥ 4 s apart: room for the label
				rows[i] = append(rows[i], f)
				ends[i] = hi
				placed = true
				break
			}
		}
		if !placed {
			rows = append(rows, []*Fault{f})
			ends = append(ends, hi)
		}
	}
	return rows
}

func (m *Model) laneLabel(c int64) string {
	wls, gws := map[string]bool{}, map[string]bool{}
	for _, o := range m.Ops {
		if o.Client == c {
			wls[o.WL] = true
			if o.GW != "" {
				gws[o.GW] = true
			}
		}
	}
	return clip(fmt.Sprintf("c%d %s %s", c, strings.Join(sortedKeys(wls), "/"), strings.Join(sortedKeys(gws), ",")), 34)
}

func (m *Model) sectionTimeline(w io.Writer) {
	fmt.Fprint(w, "<h2 id=\"timeline\">Timeline</h2>\n")
	fmt.Fprint(w, "<p class=\"legend\"><span><span class=\"sw\" style=\"background:var(--ok)\"></span>ok</span><span><span class=\"sw\" style=\"background:var(--fail)\"></span>fail</span><span><span class=\"sw\" style=\"background:var(--info)\"></span>info (unknown outcome)</span><span><span class=\"sw\" style=\"background:#e9b44c\"></span>inject → confirmed</span><span><span class=\"sw\" style=\"background:#c0392b\"></span>confirmed → heal</span><span><span class=\"sw\" style=\"background:#6c8ebf\"></span>heal → healed</span><span><span class=\"sw\" style=\"background:#1f4e9c\"></span>primary</span><span><span class=\"sw\" style=\"background:#9bb7e0\"></span>standby</span><span><span class=\"sw\" style=\"background:#d62828\"></span>status probe error</span></p>\n")
	frows := m.faultRows()
	nodes := sortedKeys(m.Nodes)
	slots := sortedKeys(m.Slots)
	lanes := m.laneOrder()
	gws := m.Gateways()
	// Vertical layout.
	y := int64(axisH)
	yMarkers := y
	y += rowH
	yFaults := y
	y += int64(max(len(frows), 1)) * faultRowH
	yLeader := y
	y += rowH + 4
	yNodes := y
	y += int64(len(nodes)) * (rowH + 2)
	ySlots := y
	y += int64(len(slots)) * (rowH + 2)
	yCobs := y
	if len(m.ClientObs) > 0 {
		y += rowH + 4
	}
	yLanes := y + 4
	y = yLanes + int64(len(lanes))*laneH + 8
	yThr := y + chartGap
	y = yThr + chartH
	hasWAL, hasLat := false, false
	for _, n := range nodes {
		for _, o := range m.Nodes[n] {
			hasWAL = hasWAL || o.WAL >= 0
			hasLat = hasLat || o.Latency >= 0
		}
	}
	yWAL := y + chartGap
	if hasWAL {
		y = yWAL + chartH
	}
	yLat := y + chartGap
	if hasLat {
		y = yLat + chartH
	}
	height := y + 10
	width := px(m.End) + rightPad
	fmt.Fprintf(w, "<div class=\"scroll\"><svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\" role=\"img\" aria-label=\"timeline of %s\">\n", width, height, width, height, esc(m.Run.ID))
	// Axis and grid.
	fmt.Fprintf(w, "<line class=\"axis\" x1=\"%d\" y1=\"%d\" x2=\"%d\" y2=\"%d\"/>\n", leftW, axisH-6, width-rightPad, axisH-6)
	for t := int64(0); t <= m.End; t += tickEvery {
		x := px(t)
		fmt.Fprintf(w, "<line class=\"grid\" x1=\"%d\" y1=\"%d\" x2=\"%d\" y2=\"%d\"/>", x, axisH-6, x, height-10)
		if t%(2*tickEvery) == 0 {
			fmt.Fprintf(w, "<text x=\"%d\" y=\"%d\">%d s</text>", x+2, axisH-10, t/1_000_000_000)
		}
		fmt.Fprint(w, "\n")
	}
	label := func(yy int64, s string) {
		fmt.Fprintf(w, "<text class=\"lbl\" x=\"4\" y=\"%d\">%s</text>\n", yy, esc(s))
	}
	// Fault bands behind the lanes.
	for _, f := range m.Faults {
		fmt.Fprintf(w, "<rect class=\"fb\" x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\"/>\n", px(f.I), yFaults, max(px(f.E)-px(f.I), 1), height-10-yFaults)
	}
	// Markers: one dashed line per marker event.
	label(yMarkers+10, "markers")
	for _, ev := range m.Markers {
		v, _ := ev.Value.(map[string]any)
		x := px(ev.T)
		fmt.Fprintf(w, "<line id=\"e%d\" data-ev=\"%d\" class=\"mk\" x1=\"%d\" y1=\"%d\" x2=\"%d\" y2=\"%d\"><title>i=%d marker %s at %s s</title></line>\n",
			ev.I, ev.I, x, yMarkers, x, height-10, ev.I, esc(str(v, "name")), secs(ev.T))
	}
	// Faults: phase bands and one element per nemesis event.
	label(yFaults+10, "faults")
	for ri, row := range frows {
		yy := yFaults + int64(ri)*faultRowH
		for _, f := range row {
			fmt.Fprintf(w, "<text x=\"%d\" y=\"%d\">%s %s %s</text>\n", px(f.I), yy+9, esc(f.ID), esc(f.Kind), esc(strings.Join(f.Targets, ",")))
			yy := yy + 11
			seg := func(a, b, cls string) {
				ta, oka := f.Phase[a]
				tb, okb := f.Phase[b]
				if oka && okb {
					fmt.Fprintf(w, "<rect class=\"%s\" x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\"/>", cls, px(ta), yy+3, max(px(tb)-px(ta), 1), rowH-6)
				}
			}
			seg("inject", "confirmed", "f-a")
			seg("confirmed", "heal", "f-b")
			seg("heal", "healed", "f-c")
			for _, ev := range f.Events {
				ph := str(ev.Fault, "phase")
				cls := "ph"
				if ph == "not-applied" || ph == "not-healed" {
					cls = "ph-bad"
				}
				fmt.Fprintf(w, "<rect id=\"e%d\" data-ev=\"%d\" data-fault=\"%s\" data-kind=\"%s\" data-phase=\"%s\" class=\"%s\" x=\"%d\" y=\"%d\" width=\"2\" height=\"%d\"><title>i=%d %s %s %s at %s s (targets %s)</title></rect>\n",
					ev.I, ev.I, esc(f.ID), esc(f.Kind), esc(ph), cls, px(ev.T), yy, rowH, ev.I, esc(f.ID), esc(f.Kind), esc(ph), secs(ev.T), esc(strings.Join(f.Targets, ",")))
			}
		}
	}
	// Observed leader: at t, the leader of the highest rule first seen ≤ t.
	label(yLeader+10, "leader (observed)")
	if len(m.Rules) > 0 {
		byT := append([]Rule{}, m.Rules...)
		sort.SliceStable(byT, func(a, b int) bool { return byT[a].FirstSeen < byT[b].FirstSeen })
		var cur *Rule
		var segStart int64
		flush := func(until int64) {
			if cur == nil {
				return
			}
			ci := 0
			for i, n := range nodes {
				if n == cur.Leader {
					ci = i
				}
			}
			fmt.Fprintf(w, "<rect x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\" fill=\"%s\" fill-opacity=\"0.55\"><title>rule %s leader %s from %s s</title></rect><text x=\"%d\" y=\"%d\">%s %s</text>\n",
				px(segStart), yLeader+2, max(px(until)-px(segStart), 1), rowH-2, palette[ci%len(palette)], esc(cur.Key()), esc(cur.Leader), secs(segStart), px(segStart)+3, yLeader+rowH-3, esc(cur.Leader), esc(cur.Key()))
		}
		for i := range byT {
			r := byT[i]
			if cur != nil && (r.Term < cur.Term || r.Term == cur.Term && r.Sub <= cur.Sub) {
				continue
			}
			flush(r.FirstSeen)
			cur, segStart = &byT[i], r.FirstSeen
		}
		flush(m.End)
	}
	// Node rows: pooler status (or node-sql) per observation.
	for ni, n := range nodes {
		yy := yNodes + int64(ni)*(rowH+2)
		label(yy+10, "node "+n)
		obs := m.Nodes[n]
		for i, o := range obs {
			end := o.T + 2_000_000_000
			if i+1 < len(obs) && obs[i+1].T < end {
				end = obs[i+1].T
			}
			wal := "—"
			if o.WAL >= 0 {
				wal = fmt.Sprint(o.WAL)
			}
			fmt.Fprintf(w, "<rect class=\"st-%s\" x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\"><title>%s %s at %s s: %s; WAL %s; probe %d ms</title></rect>\n",
				o.State, px(o.T), yy, max(px(end)-px(o.T), 1), rowH-2, esc(n), o.State, secs(o.T), esc(clip(o.Detail, 160)), wal, o.Latency)
		}
	}
	for si, n := range slots {
		yy := ySlots + int64(si)*(rowH+2)
		label(yy+10, "cdc slot on "+n)
		obs := m.Slots[n]
		for i, o := range obs {
			end := o.T + 2_000_000_000
			if i+1 < len(obs) && obs[i+1].T < end {
				end = obs[i+1].T
			}
			cls := "sl-not"
			if o.Ready {
				cls = "sl-ready"
			}
			fmt.Fprintf(w, "<rect class=\"%s\" x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\"><title>slot on %s at %s s: ready %t (%s)</title></rect>\n",
				cls, px(o.T), yy, max(px(end)-px(o.T), 1), rowH-2, esc(n), secs(o.T), o.Ready, esc(clip(o.Detail, 160)))
		}
	}
	if len(m.ClientObs) > 0 {
		label(yCobs+10, "client errors (obs)")
		for _, c := range m.ClientObs {
			fmt.Fprintf(w, "<rect class=\"cobs\" x=\"%d\" y=\"%d\" width=\"1\" height=\"%d\"><title>%s s op %d: %s</title></rect>\n", px(c.T), yCobs+2, rowH-4, secs(c.T), c.Op, esc(clip(c.Message, 200)))
		}
	}
	// Operation lanes.
	laneOf := map[int64]int64{}
	for i, c := range lanes {
		laneOf[c] = int64(i)
		label(yLanes+int64(i)*laneH+8, m.laneLabel(c))
	}
	for _, o := range m.Ops {
		yy := yLanes + laneOf[o.Client]*laneH
		x1 := px(o.TInv)
		wd := max(px(o.TComp)-x1, 1)
		e := ""
		if o.Type != "ok" {
			e = " " + esc(str(o.Err, "sqlstate")+" "+str(o.Err, "class")+" "+str(o.Err, "phase"))
		}
		fmt.Fprintf(w, "<rect id=\"e%d\" data-ev=\"%d %d\" class=\"%s\" x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\"><title>i=%d→%d c%d p%d %s %s %s %s %s ms at %s s%s</title></rect>\n",
			o.Inv, o.Inv, o.Comp, o.Type, x1, yy, wd, laneH-2, o.Inv, o.Comp, o.Client, o.Proc, esc(o.GW), esc(o.WL), esc(o.F), o.Type, msec(o.Latency()), secs(o.TInv), e)
	}
	// Throughput: ok completions per second per gateway; non-ok as bars.
	label(yThr+10, "ok completions/s per gateway")
	label(yThr+24, "non-ok/s (bars)")
	okPer := map[string]map[int64]int64{}
	bad := map[int64]int64{}
	var maxC int64 = 1
	for _, o := range m.Ops {
		s := o.TComp / 1_000_000_000
		if o.Type == "ok" {
			if okPer[o.GW] == nil {
				okPer[o.GW] = map[int64]int64{}
			}
			okPer[o.GW][s]++
			maxC = max(maxC, okPer[o.GW][s])
		} else {
			bad[s]++
			maxC = max(maxC, bad[s])
		}
	}
	base := yThr + chartH
	fmt.Fprintf(w, "<line class=\"axis\" x1=\"%d\" y1=\"%d\" x2=\"%d\" y2=\"%d\"/><text class=\"lbl\" x=\"%d\" y=\"%d\">max %d/s</text>\n", leftW, base, width-rightPad, base, leftW+4, yThr+8, maxC)
	secsN := m.End/1_000_000_000 + 1
	for s := int64(0); s < secsN; s++ {
		if n := bad[s]; n > 0 {
			h := n * (chartH - 4) / maxC
			fmt.Fprintf(w, "<rect class=\"bar\" x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\"><title>%d s: %d non-ok completions</title></rect>\n", px(s*1_000_000_000), base-h, max(int64(1_000_000_000/nsPerPx)-2, 1), max(h, 1), s, n)
		}
	}
	for gi, g := range gws {
		var pts []string
		for s := int64(0); s < secsN; s++ {
			n := okPer[g][s]
			pts = append(pts, fmt.Sprintf("%d,%d", px(s*1_000_000_000+500_000_000), base-n*(chartH-4)/maxC))
		}
		fmt.Fprintf(w, "<polyline class=\"series\" stroke=\"%s\" points=\"%s\"><title>%s ok/s</title></polyline><text x=\"%d\" y=\"%d\" fill=\"%s\">%s</text>\n",
			palette[gi%len(palette)], strings.Join(pts, " "), esc(g), leftW-60, yThr+36+int64(gi)*12, palette[gi%len(palette)], esc(g))
	}
	if hasWAL {
		m.nodeChart(w, nodes, yWAL, width, "WAL position (bytes)", func(o NodeObs) int64 { return o.WAL })
	}
	if hasLat {
		m.nodeChart(w, nodes, yLat, width, "status probe latency (ms)", func(o NodeObs) int64 { return o.Latency })
	}
	fmt.Fprint(w, "</svg></div>\n")
	fmt.Fprintf(w, "<p class=\"small\">%d operations in %d lanes, %d nemesis events, %d markers; scale 1 px = %d ms.</p>\n", len(m.Ops), len(lanes), m.Counts["nemesis"], len(m.Markers), nsPerPx/1_000_000)
}

// nodeChart draws one polyline per node of value(o) ≥ 0, scaled to the
// observed range.
func (m *Model) nodeChart(w io.Writer, nodes []string, y0, width int64, title string, value func(NodeObs) int64) {
	lo, hi := int64(-1), int64(-1)
	for _, n := range nodes {
		for _, o := range m.Nodes[n] {
			if v := value(o); v >= 0 {
				if lo < 0 || v < lo {
					lo = v
				}
				hi = max(hi, v)
			}
		}
	}
	base := y0 + chartH
	fmt.Fprintf(w, "<text class=\"lbl\" x=\"4\" y=\"%d\">%s</text><text class=\"lbl\" x=\"4\" y=\"%d\">%d … %d</text>\n", y0+10, esc(title), y0+24, lo, hi)
	fmt.Fprintf(w, "<line class=\"axis\" x1=\"%d\" y1=\"%d\" x2=\"%d\" y2=\"%d\"/>\n", leftW, base, width-rightPad, base)
	span := hi - lo
	for ni, n := range nodes {
		// One polyline per run of observations with a value, broken where the
		// node had none (probe error) or none for more than 2 s: no line is
		// drawn across time without data.
		var runs [][]string
		var pts []string
		last := int64(-1)
		flush := func() {
			if len(pts) > 0 {
				runs = append(runs, pts)
			}
			pts = nil
		}
		for _, o := range m.Nodes[n] {
			v := value(o)
			if v < 0 {
				flush()
				continue
			}
			if last >= 0 && o.T-last > 2_000_000_000 {
				flush()
			}
			last = o.T
			yy := base - (chartH-4)/2
			if span > 0 {
				yy = base - (v-lo)*(chartH-4)/span
			}
			pts = append(pts, fmt.Sprintf("%d,%d", px(o.T), yy))
		}
		flush()
		if len(runs) == 0 {
			continue
		}
		for _, r := range runs {
			fmt.Fprintf(w, "<polyline class=\"series\" stroke=\"%s\" points=\"%s\"><title>%s %s</title></polyline>\n", palette[ni%len(palette)], strings.Join(r, " "), esc(n), esc(title))
		}
		fmt.Fprintf(w, "<text x=\"%d\" y=\"%d\" fill=\"%s\">%s</text>\n", leftW-60, y0+36+int64(ni)*10, palette[ni%len(palette)], esc(n))
	}
}
