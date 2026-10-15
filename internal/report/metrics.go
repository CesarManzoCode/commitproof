package report

import (
	"sort"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Per-fault failover metrics (CONTRACT §10.10, §15 K6.5; definitions fixed
// by ADR-0036). cpgate recomputes every value independently.

// Window states of a gateway's unavailability window.
const (
	WindowClosed = "window" // S and R exist
	WindowOpen   = "open"   // S exists, no ok completion at or after max(S, E)
	WindowNone   = "none"   // no fail/info completion at or after I
)

// GatewayMetric is one gateway's availability around one fault.
type GatewayMetric struct {
	GW    string
	State string
	S, R  int64 // window start and end (completion times); valid per State
	Gap   int64 // longest ok gap over [I − 1 s, E + 10 s]
	NonOK int64 // non-ok completions in that interval
}

// Phase holds nearest-rank percentiles of ok latencies (ns).
type Phase struct {
	N             int64
	P50, P95, P99 int64
}

// FaultMetrics are the K6.5 values of one fault.
type FaultMetrics struct {
	Fault         *Fault
	Gateways      []GatewayMetric
	Before        Phase
	During        Phase
	After         Phase
	Rules         []Rule // rules first seen in [I, I′)
	LeaderChanges int64
}

const (
	gapLead     = int64(time.Second)     // ADR-0036: the gap interval starts 1 s before I
	phaseWindow = int64(params.FaultGap) // P05's 10 s: before/after phases and the gap's tail
)

// Metrics computes ADR-0036's per-fault metrics.
func (m *Model) Metrics() []FaultMetrics {
	var ops []*Op // ADR-0036's O: every operation except final reads
	for _, o := range m.Ops {
		if o.F != "final-read" {
			ops = append(ops, o)
		}
	}
	gws := m.Gateways()
	out := make([]FaultMetrics, 0, len(m.Faults))
	for _, f := range m.Faults {
		fm := FaultMetrics{Fault: f}
		for _, g := range gws {
			fm.Gateways = append(fm.Gateways, gatewayMetric(ops, g, f))
		}
		var before, during, after []int64
		for _, o := range ops {
			if o.Type != "ok" {
				continue
			}
			switch t := o.TInv; {
			case t >= f.I-phaseWindow && t < f.I:
				before = append(before, o.Latency())
			case t >= f.I && t <= f.E:
				during = append(during, o.Latency())
			case t > f.E && t <= f.E+phaseWindow:
				after = append(after, o.Latency())
			}
		}
		fm.Before, fm.During, fm.After = phase(before), phase(during), phase(after)
		next := int64(-1) // I′: the least inject time greater than I
		for _, g := range m.Faults {
			if g.I > f.I && (next < 0 || g.I < next) {
				next = g.I
			}
		}
		for _, r := range m.Rules {
			if r.FirstSeen >= f.I && (next < 0 || r.FirstSeen < next) {
				fm.Rules = append(fm.Rules, r)
				if r.LeaderChange() {
					fm.LeaderChanges++
				}
			}
		}
		out = append(out, fm)
	}
	return out
}

func gatewayMetric(ops []*Op, g string, f *Fault) GatewayMetric {
	gm := GatewayMetric{GW: g, State: WindowNone}
	S := int64(-1)
	for _, o := range ops {
		if o.GW == g && o.Type != "ok" && o.TComp >= f.I && (S < 0 || o.TComp < S) {
			S = o.TComp
		}
	}
	if S >= 0 {
		gm.State, gm.S = WindowOpen, S
		from := max(S, f.E)
		R := int64(-1)
		for _, o := range ops {
			if o.GW == g && o.Type == "ok" && o.TComp >= from && (R < 0 || o.TComp < R) {
				R = o.TComp
			}
		}
		if R >= 0 {
			gm.State, gm.R = WindowClosed, R
		}
	}
	lo, hi := f.I-gapLead, f.E+phaseWindow
	var oks []int64
	for _, o := range ops {
		if o.GW != g || o.TComp < lo || o.TComp > hi {
			continue
		}
		if o.Type == "ok" {
			oks = append(oks, o.TComp)
		} else {
			gm.NonOK++
		}
	}
	sort.Slice(oks, func(a, b int) bool { return oks[a] < oks[b] })
	last := lo
	for _, t := range oks {
		gm.Gap = max(gm.Gap, t-last)
		last = t
	}
	gm.Gap = max(gm.Gap, hi-last)
	return gm
}

// phase: nearest rank — the value of rank ⌈q·n/100⌉ among the ascending
// latencies.
func phase(ls []int64) Phase {
	p := Phase{N: int64(len(ls))}
	if p.N == 0 {
		return p
	}
	sort.Slice(ls, func(a, b int) bool { return ls[a] < ls[b] })
	rank := func(q int64) int64 { return ls[(q*p.N+99)/100-1] }
	p.P50, p.P95, p.P99 = rank(50), rank(95), rank(99)
	return p
}
