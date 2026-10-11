package nemesis

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/netprobe"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// nsRules are the iptables rules one fault installs in one namespace, in
// two chains of its own (CPN-<id>-O jumped from OUTPUT, CPN-<id>-I from
// INPUT): drops towards and from peer namespaces, and, for F1d, rules that
// only count TCP SYNs arriving from a peer.
type nsRules struct {
	DropOut []string
	DropIn  []string
	CountIn []string
}

// partition implements F1a–F1d (CONTRACT §8).
type partition struct {
	e     *Env
	id    string
	kind  string
	rules map[string]*nsRules
	fail  [][2]string // directed (src, dst) namespace pairs whose connects must time out
	ok    [][2]string // pairs that must connect while the fault holds
}

func (p *partition) chains() (string, string) {
	c := "CPN-" + strings.ReplaceAll(p.id, ".", "_")
	return c + "-O", c + "-I"
}

func (p *partition) addRule(ns string) *nsRules {
	if p.rules[ns] == nil {
		p.rules[ns] = &nsRules{}
	}
	return p.rules[ns]
}

// cut drops all traffic between a and b, with rules in both namespaces.
func (p *partition) cut(a, b string) {
	ra, rb := p.addRule(a), p.addRule(b)
	ra.DropOut, ra.DropIn = append(ra.DropOut, b), append(ra.DropIn, b)
	rb.DropOut, rb.DropIn = append(rb.DropOut, a), append(rb.DropIn, a)
	p.fail = append(p.fail, [2]string{a, b}, [2]string{b, a})
}

func newPartition(e *Env, id, kind string) *partition {
	return &partition{e: e, id: id, kind: kind, rules: map[string]*nsRules{}}
}

// isolate is F1a: target cut from every other running namespace.
func isolate(e *Env, id, target string, running []string) *partition {
	p := newPartition(e, id, "F1a")
	for _, peer := range without(running, target) {
		p.cut(target, peer)
	}
	return p
}

// split is F1b: every A–B pair cut; A–A and B–B pairs must still connect.
func split(e *Env, id string, a, b []string) *partition {
	p := newPartition(e, id, "F1b")
	for _, x := range a {
		for _, y := range b {
			p.cut(x, y)
		}
	}
	for _, set := range [][]string{a, b} {
		for _, x := range set {
			for _, y := range set {
				if x != y {
					p.ok = append(p.ok, [2]string{x, y})
				}
			}
		}
	}
	return p
}

// asymmetric is F1c: the leader cut from orchestrators, followers and etcd,
// not from gateways.
func asymmetric(e *Env, id, leader string, running []string) *partition {
	p := newPartition(e, id, "F1c")
	for _, n := range running {
		switch {
		case n == leader, n == "runner":
		case strings.HasPrefix(n, "gw-"):
			p.ok = append(p.ok, [2]string{leader, n}, [2]string{n, leader})
		default: // orchestrators, other nodes, etcd
			p.cut(leader, n)
		}
	}
	return p
}

// oneWay is F1d: the leader's packets towards followers are dropped on the
// leader's OUTPUT only; SYNs from followers are counted on the leader.
func oneWay(e *Env, id, leader string, followers []string) *partition {
	p := newPartition(e, id, "F1d")
	r := p.addRule(leader)
	for _, f := range followers {
		r.DropOut = append(r.DropOut, f)
		r.CountIn = append(r.CountIn, f)
		// leader→follower SYNs are dropped; follower→leader SYNs arrive
		// (counted) but the SYN-ACKs are dropped: both connects time out.
		p.fail = append(p.fail, [2]string{leader, f}, [2]string{f, leader})
	}
	return p
}

func (p *partition) namespaces() []string {
	var out []string
	for n := range p.rules {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (p *partition) installScript(ns string) string {
	o, i := p.chains()
	r := p.rules[ns]
	var b strings.Builder
	fmt.Fprintf(&b, "set -e; iptables -w 5 -N %s; iptables -w 5 -N %s", o, i)
	for _, peer := range r.DropOut {
		fmt.Fprintf(&b, "; iptables -w 5 -A %s -d %s -j DROP", o, Addr(p.e.Prefix, peer))
	}
	for _, peer := range r.DropIn {
		fmt.Fprintf(&b, "; iptables -w 5 -A %s -s %s -j DROP", i, Addr(p.e.Prefix, peer))
	}
	for _, peer := range r.CountIn {
		fmt.Fprintf(&b, "; iptables -w 5 -A %s -s %s -p tcp --syn", i, Addr(p.e.Prefix, peer))
	}
	fmt.Fprintf(&b, "; iptables -w 5 -I OUTPUT 1 -j %s; iptables -w 5 -I INPUT 1 -j %s", o, i)
	return b.String()
}

func (p *partition) removeScript() string {
	o, i := p.chains()
	return fmt.Sprintf("iptables -w 5 -D OUTPUT -j %[1]s; iptables -w 5 -D INPUT -j %[2]s; "+
		"iptables -w 5 -F %[1]s; iptables -w 5 -F %[2]s; iptables -w 5 -X %[1]s; iptables -w 5 -X %[2]s; true", o, i)
}

func (p *partition) prepare(ctx context.Context) ([]Probe, map[string]any, error) {
	sets := map[string]any{}
	for _, n := range p.namespaces() {
		r := p.rules[n]
		sets[n] = map[string]any{"drop_out": strList(r.DropOut), "drop_in": strList(r.DropIn), "count_in": strList(r.CountIn)}
	}
	return nil, map[string]any{"rules": sets, "must_fail": pairs(p.fail), "must_connect": pairs(p.ok)}, nil
}

func pairs(ps [][2]string) []any {
	out := make([]any, len(ps))
	for i, x := range ps {
		out[i] = x[0] + "->" + x[1]
	}
	return out
}

func (p *partition) inject(ctx context.Context) []Probe {
	ns := p.namespaces()
	return parallel(len(ns), func(k int) Probe {
		return p.e.exec(ctx, p.id+"-inject-"+ns[k], Helper(ns[k]), "sh", "-c", p.installScript(ns[k]))
	})
}

// present splits the fault's namespaces into those whose owner container
// and helper still run and those that went away while the fault held (a
// composite's later element may stop a node, e.g. C4's F5b during F1a,
// ADR-0026): a vanished namespace holds no rules and cannot be probed.
func (p *partition) present(ctx context.Context, tag string) (map[string]bool, []string, Probe) {
	pr, running := p.e.runningContainers(ctx, p.id+"-"+tag+"-ps")
	alive := map[string]bool{}
	var gone []string
	for _, n := range p.namespaces() {
		if !pr.OK() || n == "runner" || (running[OwnerContainer(n)] && running[Helper(n)]) {
			alive[n] = true
		} else {
			gone = append(gone, n)
		}
	}
	return alive, gone, pr
}

func (p *partition) heal(ctx context.Context) []Probe {
	alive, _, pr := p.present(ctx, "heal")
	var ns []string
	for _, n := range p.namespaces() {
		if alive[n] {
			ns = append(ns, n)
		}
	}
	return append([]Probe{pr}, parallel(len(ns), func(k int) Probe {
		return p.e.exec(ctx, p.id+"-heal-"+ns[k], Helper(ns[k]), "sh", "-c", p.removeScript())
	})...)
}

// connectRound probes every pair, one `cpnet connect` per source helper.
func (p *partition) connectRound(ctx context.Context, tag string, prs [][2]string) ([]Probe, map[[2]string]*netprobe.Target) {
	bySrc := map[string][]string{}
	for _, x := range prs {
		bySrc[x[0]] = append(bySrc[x[0]], x[1])
	}
	var srcs []string
	for s := range bySrc {
		srcs = append(srcs, s)
	}
	sort.Strings(srcs)
	probes := parallel(len(srcs), func(k int) Probe {
		argv := []string{"cpnet", "connect", "--timeout", connectTimeout.String()}
		for _, d := range bySrc[srcs[k]] {
			argv = append(argv, ProbeAddr(p.e.Prefix, d))
		}
		return p.e.exec(ctx, p.id+"-"+tag+"-connect-"+srcs[k], Helper(srcs[k]), argv...)
	})
	res := map[[2]string]*netprobe.Target{}
	for k, s := range srcs {
		var c netprobe.Connect
		if !probes[k].OK() || lastJSON(probes[k], &c) != nil {
			continue
		}
		for _, d := range bySrc[s] {
			res[[2]string{s, d}] = c.Result(ProbeAddr(p.e.Prefix, d))
		}
	}
	return probes, res
}

var saveRule = regexp.MustCompile(`^\[(\d+):(\d+)\] -A (\S+) (.*)$`)

// counter is one rule of `iptables-save -c`.
type counter struct {
	Packets int64
	Chain   string
	Spec    string
}

func parseSave(out string) []counter {
	var cs []counter
	for _, l := range strings.Split(out, "\n") {
		m := saveRule.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		n, _ := strconv.ParseInt(m[1], 10, 64)
		cs = append(cs, counter{Packets: n, Chain: m[3], Spec: m[4]})
	}
	return cs
}

func (p *partition) saveRound(ctx context.Context, tag string) ([]Probe, map[string][]counter, map[string]string) {
	ns := p.namespaces()
	probes := parallel(len(ns), func(k int) Probe {
		return p.e.exec(ctx, p.id+"-"+tag+"-iptables-"+ns[k], Helper(ns[k]), "iptables-save", "-c", "-t", "filter")
	})
	out := map[string][]counter{}
	raw := map[string]string{}
	for k, n := range ns {
		if probes[k].OK() {
			out[n] = parseSave(string(probes[k].Res.Stdout))
			raw[n] = string(probes[k].Res.Stdout)
		}
	}
	return probes, out, raw
}

// packets returns the counter of the rule in chain whose spec names addr
// with flag ("-d"/"-s"), -1 if absent.
func packets(cs []counter, chain, flag, addr string, syn bool) int64 {
	for _, c := range cs {
		if c.Chain != chain || !strings.Contains(c.Spec, flag+" "+addr+"/32") {
			continue
		}
		if syn != strings.Contains(c.Spec, "tcp") {
			continue
		}
		return c.Packets
	}
	return -1
}

// confirm: every must-fail pair times out (probed from both helpers), every
// must-connect pair connects, every installed drop rule on OUTPUT and every
// SYN-counting rule has a packet counter > 0 (CONTRACT §8 F1a–F1d).
func (p *partition) confirm(ctx context.Context) (bool, []Probe, map[string]any) {
	cp, res := p.connectRound(ctx, "confirm", append(append([][2]string{}, p.fail...), p.ok...))
	sp, cs, _ := p.saveRound(ctx, "confirm")
	var problems []string
	for _, x := range p.fail {
		if !res[x].AllFailedWith(netprobe.KindTimeout) {
			problems = append(problems, fmt.Sprintf("%s->%s did not time out", x[0], x[1]))
		}
	}
	for _, x := range p.ok {
		if !res[x].AllOK() {
			problems = append(problems, fmt.Sprintf("%s->%s did not connect", x[0], x[1]))
		}
	}
	o, i := p.chains()
	counts := map[string]any{}
	for _, n := range p.namespaces() {
		r := p.rules[n]
		c, ok := cs[n]
		if !ok {
			problems = append(problems, n+": iptables-save failed")
			continue
		}
		nc := map[string]any{}
		for _, peer := range r.DropOut {
			v := packets(c, o, "-d", Addr(p.e.Prefix, peer), false)
			nc["out->"+peer] = v
			if v < 1 {
				problems = append(problems, fmt.Sprintf("%s: drop rule towards %s counted %d packets", n, peer, v))
			}
		}
		for _, peer := range r.CountIn {
			v := packets(c, i, "-s", Addr(p.e.Prefix, peer), true)
			nc["syn<-"+peer] = v
			if v < 1 {
				problems = append(problems, fmt.Sprintf("%s: SYN counter from %s is %d", n, peer, v))
			}
		}
		counts[n] = nc
	}
	return len(problems) == 0, append(cp, sp...), map[string]any{"problems": strList(problems), "counters": counts}
}

// healed: every pair between namespaces that still exist connects and no
// chain of this fault remains in them. Namespaces that went away while the
// fault held are listed, not probed.
func (p *partition) healed(ctx context.Context) (bool, []Probe, map[string]any) {
	alive, gone, pp := p.present(ctx, "healed")
	var prs [][2]string
	for _, x := range append(append([][2]string{}, p.fail...), p.ok...) {
		if alive[x[0]] && alive[x[1]] {
			prs = append(prs, x)
		}
	}
	cp, res := p.connectRound(ctx, "healed", prs)
	sp, _, raw := p.saveRound(ctx, "healed")
	var problems []string
	for _, x := range prs {
		if !res[x].AllOK() {
			problems = append(problems, fmt.Sprintf("%s->%s did not connect", x[0], x[1]))
		}
	}
	o, i := p.chains()
	for _, n := range p.namespaces() {
		if !alive[n] {
			continue
		}
		r, ok := raw[n]
		switch {
		case !ok:
			problems = append(problems, n+": iptables-save failed")
		case strings.Contains(r, o) || strings.Contains(r, i):
			problems = append(problems, n+": chains of the fault remain")
		}
	}
	det := map[string]any{"problems": strList(problems)}
	if len(gone) > 0 {
		det["namespaces_gone"] = strList(gone)
	}
	return len(problems) == 0, append(append([]Probe{pp}, cp...), sp...), det
}

// delay is F7: netem delay on the target's eth0 (P18).
type delay struct {
	e        *Env
	id       string
	target   string
	prober   string
	baseline int64 // median connect RTT before the fault, µs
}

func newDelay(e *Env, id, target string) *delay {
	prober := "etcd"
	if target == "etcd" {
		prober = "gw-z1"
	}
	return &delay{e: e, id: id, target: target, prober: prober}
}

// rttSamples is the number of connects whose median is the RTT.
const rttSamples = 10

func (d *delay) rtt(ctx context.Context, tag string) (Probe, int64, error) {
	pr := d.e.exec(ctx, d.id+"-"+tag+"-rtt", Helper(d.prober), "cpnet", "connect", "--timeout", connectTimeout.String(),
		"--count", strconv.Itoa(rttSamples), ProbeAddr(d.e.Prefix, d.target))
	var c netprobe.Connect
	if !pr.OK() {
		return pr, 0, fmt.Errorf("rtt probe exit %d", pr.Res.ExitCode)
	}
	if err := lastJSON(pr, &c); err != nil {
		return pr, 0, err
	}
	m, ok := c.Result(ProbeAddr(d.e.Prefix, d.target)).MedianUS()
	if !ok {
		return pr, 0, fmt.Errorf("not every RTT probe connected")
	}
	return pr, m, nil
}

func (d *delay) prepare(ctx context.Context) ([]Probe, map[string]any, error) {
	pr, m, err := d.rtt(ctx, "baseline")
	d.baseline = m
	return []Probe{pr}, map[string]any{"prober": d.prober, "baseline_rtt_us": m}, err
}

func (d *delay) inject(ctx context.Context) []Probe {
	return []Probe{d.e.exec(ctx, d.id+"-inject", Helper(d.target), "tc", "qdisc", "add", "dev", "eth0", "root", "netem",
		"delay", fmt.Sprintf("%dms", params.NetemDelay.Milliseconds()))}
}

func (d *delay) heal(ctx context.Context) []Probe {
	return []Probe{d.e.exec(ctx, d.id+"-heal", Helper(d.target), "tc", "qdisc", "del", "dev", "eth0", "root")}
}

// confirm: the median connect RTT from a peer helper rose by ≥ P18's 40 ms.
func (d *delay) confirm(ctx context.Context) (bool, []Probe, map[string]any) {
	pr, m, err := d.rtt(ctx, "confirm")
	det := map[string]any{"baseline_rtt_us": d.baseline, "rtt_us": m, "threshold_increase_us": params.NetemConfirmIncrease.Microseconds()}
	if err != nil {
		det["error"] = err.Error()
		return false, []Probe{pr}, det
	}
	return m-d.baseline >= params.NetemConfirmIncrease.Microseconds(), []Probe{pr}, det
}

// healedTolerance is §8 F7's heal predicate: RTT within 10 ms of baseline.
const healedTolerance = 10_000 // µs

func (d *delay) healed(ctx context.Context) (bool, []Probe, map[string]any) {
	pr, m, err := d.rtt(ctx, "healed")
	qs := d.e.exec(ctx, d.id+"-healed-qdisc", Helper(d.target), "tc", "qdisc", "show", "dev", "eth0")
	det := map[string]any{"baseline_rtt_us": d.baseline, "rtt_us": m, "tolerance_us": int64(healedTolerance)}
	ok := err == nil && m-d.baseline <= healedTolerance && d.baseline-m <= healedTolerance &&
		qs.OK() && !strings.Contains(string(qs.Res.Stdout), "netem")
	if err != nil {
		det["error"] = err.Error()
	}
	return ok, []Probe{pr, qs}, det
}
