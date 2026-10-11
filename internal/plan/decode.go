package plan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Decode reads plan.json bytes back into a Plan. Re-encoding the result
// with Canonical yields the input bytes (checked), so a replayed or reduced
// plan is exactly the recorded one.
func Decode(b []byte) (*Plan, error) {
	b = bytes.TrimSpace(b)
	v, err := canonjson.Decode(b)
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("plan: not an object")
	}
	p := &Plan{}
	p.Campaign, _ = m["campaign"].(string)
	p.ContractVersion, _ = m["contract_version"].(string)
	p.Directed, _ = m["directed"].(string)
	p.Profile, _ = m["profile"].(string)
	seed, err := toUint(m["seed"])
	if err != nil {
		return nil, fmt.Errorf("plan seed: %w", err)
	}
	p.Seed = seed
	for k, dst := range map[string]*int64{"duration_ms": &p.DurationMS, "pacing_ms": &p.PacingMS, "client_timeout_ms": &p.ClientTimeoutMS} {
		if *dst, err = toInt(m[k]); err != nil {
			return nil, fmt.Errorf("plan %s: %w", k, err)
		}
	}
	if p.Steps, err = ParseSteps(m["nemesis"]); err != nil {
		return nil, err
	}
	if p.ReducedFrom, _ = m["reduced_from"].(string); p.ReducedFrom != "" {
		p.Reductions = strs(m["reductions"])
	}
	cl, _ := m["clients"].([]any)
	for i, x := range cl {
		cm, _ := x.(map[string]any)
		c := Client{}
		n, err := toInt(cm["client"])
		if err != nil {
			return nil, fmt.Errorf("plan clients[%d]: %w", i, err)
		}
		c.Client = int(n)
		c.WL, _ = cm["wl"].(string)
		c.GW, _ = cm["gw"].(string)
		c.Iso, _ = cm["iso"].(string)
		if v, ok := cm["statement_timeout_ms"]; ok {
			if c.StatementTimeoutMS, err = toInt(v); err != nil {
				return nil, fmt.Errorf("plan clients[%d]: %w", i, err)
			}
		}
		if v, ok := cm["cancel_commit_after_ms"]; ok {
			if c.CancelCommitAfterMS, err = toInt(v); err != nil {
				return nil, fmt.Errorf("plan clients[%d]: %w", i, err)
			}
		}
		ops, _ := cm["ops"].([]any)
		for _, o := range ops {
			om, _ := o.(map[string]any)
			f, _ := om["f"].(string)
			c.Ops = append(c.Ops, Op{F: f, Value: om["value"]})
		}
		p.Clients = append(p.Clients, c)
	}
	out, err := p.Canonical()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(out, b) {
		return nil, fmt.Errorf("plan: re-encoding differs from the input (unknown or non-canonical fields)")
	}
	return p, nil
}

func toUint(v any) (uint64, error) {
	if x, ok := v.(interface{ String() string }); ok {
		var u uint64
		_, err := fmt.Sscan(x.String(), &u)
		return u, err
	}
	i, err := toInt(v)
	return uint64(i), err
}

// SHA256 is the hex sha256 of the plan's canonical bytes.
func (p *Plan) SHA256() (string, error) {
	b, err := p.Canonical()
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

// Reductions for minimization (CONTRACT §12.2). Each returns a new plan
// whose ReducedFrom names the parent plan; the parent is not modified.

func (p *Plan) derive(what string) (*Plan, error) {
	sum, err := p.SHA256()
	if err != nil {
		return nil, err
	}
	q := *p
	q.Clients = append([]Client{}, p.Clients...)
	q.Steps = append([]Step{}, p.Steps...)
	q.ReducedFrom = sum
	q.Reductions = append(append([]string{}, p.Reductions...), what)
	return &q, nil
}

// KeepSteps keeps the nemesis steps whose indices are listed (in order) and
// renumbers them. A step that becomes the first follows P05's first-fault
// anchor; a step anchored on a removed predecessor's new leader keeps its
// anchor relative to the step now before it.
func (p *Plan) KeepSteps(keep []int) (*Plan, error) {
	q, err := p.derive(fmt.Sprintf("keep-steps %v", keep))
	if err != nil {
		return nil, err
	}
	q.Steps = nil
	for i, k := range keep {
		if k < 0 || k >= len(p.Steps) {
			return nil, fmt.Errorf("keep-steps: no step %d", k)
		}
		s := p.Steps[k]
		s.Index = i
		if i == 0 {
			s.After, s.DelayMS = AfterWorkloadStart, params.FirstFaultAt.Milliseconds()
		}
		q.Steps = append(q.Steps, s)
	}
	return q, nil
}

// HalveDuration halves the workload duration and truncates every client's
// operations to the rounds that fit (P03 pacing).
func (p *Plan) HalveDuration() (*Plan, error) {
	q, err := p.derive("halve-duration")
	if err != nil {
		return nil, err
	}
	q.DurationMS = p.DurationMS / 2
	n := int(q.DurationMS / p.PacingMS)
	for i, c := range q.Clients {
		if len(c.Ops) > n {
			c.Ops = append([]Op{}, c.Ops[:n]...)
		}
		q.Clients[i] = c
	}
	return q, nil
}

// KeepClients keeps the listed clients (by client number).
func (p *Plan) KeepClients(keep []int) (*Plan, error) {
	q, err := p.derive(fmt.Sprintf("keep-clients %v", keep))
	if err != nil {
		return nil, err
	}
	set := map[int]bool{}
	for _, k := range keep {
		set[k] = true
	}
	q.Clients = nil
	for _, c := range p.Clients {
		if set[c.Client] {
			q.Clients = append(q.Clients, c)
		}
	}
	if len(q.Clients) == 0 {
		return nil, fmt.Errorf("keep-clients %v: no client left", keep)
	}
	return q, nil
}
