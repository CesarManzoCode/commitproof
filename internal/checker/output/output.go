// Package output holds the types of cpcheck's checks.json (CONTRACT §15
// K2: {run_id, seal_root, checkers: [{id, valid, anomalies: [{class, ops,
// explanation}], stats}], verdict}) for readers that must not link the
// checkers themselves: cpreport imports "checker output types" only
// (CONTRACT §5.3, ADR-0036). It decodes; cpcheck's encoder stays in
// package checker.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// Validity values of a checker (checks.json: true, false or "unknown").
const (
	ValidTrue    = "true"
	ValidFalse   = "false"
	ValidUnknown = "unknown"
)

// Anomaly is one anomaly or observation of a checker. Ops are history line
// indices.
type Anomaly struct {
	Class       string
	Ops         []int64
	Explanation string
}

// Checker is one checker's result.
type Checker struct {
	ID        string
	Valid     string // ValidTrue, ValidFalse or ValidUnknown
	Anomalies []Anomaly
	// Observations are stats.observations (reported, never a violation).
	Observations []Anomaly
	// Stats is the checker's stats object without observations, numbers as
	// json.Number.
	Stats map[string]any
}

// Checks is a decoded checks.json.
type Checks struct {
	RunID    string
	SealRoot string
	Verdict  string
	Checkers []Checker
}

// Decode parses checks.json strictly: every required field present with its
// type, ops integers. Anything else is an error (fail closed).
func Decode(b []byte) (*Checks, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("checks.json: %w", err)
	}
	c := &Checks{}
	var ok bool
	if c.RunID, ok = m["run_id"].(string); !ok {
		return nil, fmt.Errorf("checks.json: run_id missing")
	}
	if c.SealRoot, ok = m["seal_root"].(string); !ok {
		return nil, fmt.Errorf("checks.json: seal_root missing")
	}
	if c.Verdict, ok = m["verdict"].(string); !ok {
		return nil, fmt.Errorf("checks.json: verdict missing")
	}
	cs, ok := m["checkers"].([]any)
	if !ok {
		return nil, fmt.Errorf("checks.json: checkers missing")
	}
	for i, x := range cs {
		cm, ok := x.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("checks.json: checkers[%d] is not an object", i)
		}
		ch := Checker{Stats: map[string]any{}}
		if ch.ID, ok = cm["id"].(string); !ok {
			return nil, fmt.Errorf("checks.json: checkers[%d].id missing", i)
		}
		switch v := cm["valid"].(type) {
		case bool:
			ch.Valid = strconv.FormatBool(v)
		case string:
			if v != ValidUnknown {
				return nil, fmt.Errorf("checks.json: %s.valid = %q", ch.ID, v)
			}
			ch.Valid = v
		default:
			return nil, fmt.Errorf("checks.json: %s.valid missing", ch.ID)
		}
		as, ok := cm["anomalies"].([]any)
		if !ok {
			return nil, fmt.Errorf("checks.json: %s.anomalies missing", ch.ID)
		}
		var err error
		if ch.Anomalies, err = anomalies(ch.ID+".anomalies", as); err != nil {
			return nil, err
		}
		st, ok := cm["stats"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("checks.json: %s.stats missing", ch.ID)
		}
		for k, v := range st {
			if k == "observations" {
				obs, ok := v.([]any)
				if !ok {
					return nil, fmt.Errorf("checks.json: %s.stats.observations is not an array", ch.ID)
				}
				if ch.Observations, err = anomalies(ch.ID+".observations", obs); err != nil {
					return nil, err
				}
				continue
			}
			ch.Stats[k] = v
		}
		c.Checkers = append(c.Checkers, ch)
	}
	return c, nil
}

func anomalies(where string, as []any) ([]Anomaly, error) {
	out := make([]Anomaly, 0, len(as))
	for j, x := range as {
		am, ok := x.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("checks.json: %s[%d] is not an object", where, j)
		}
		a := Anomaly{}
		if a.Class, ok = am["class"].(string); !ok {
			return nil, fmt.Errorf("checks.json: %s[%d].class missing", where, j)
		}
		if a.Explanation, ok = am["explanation"].(string); !ok {
			return nil, fmt.Errorf("checks.json: %s[%d].explanation missing", where, j)
		}
		ops, ok := am["ops"].([]any)
		if !ok {
			return nil, fmt.Errorf("checks.json: %s[%d].ops missing", where, j)
		}
		for _, o := range ops {
			n, ok := o.(json.Number)
			if !ok {
				return nil, fmt.Errorf("checks.json: %s[%d].ops holds a non-number", where, j)
			}
			v, err := strconv.ParseInt(string(n), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("checks.json: %s[%d].ops: %w", where, j, err)
			}
			a.Ops = append(a.Ops, v)
		}
		out = append(out, a)
	}
	return out, nil
}
