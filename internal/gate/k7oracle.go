package gate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/CesarManzoCode/commitproof/internal/checker/output"
)

// K7Oracle is ADR-0046's record of the elle-cli cross-check of one run's C3
// anomalies (evidence/K7/oracle/<run-id>.json).
type K7Oracle struct {
	RunID       string           `json:"run_id"`
	SealRoot    string           `json:"seal_root"`
	Source      string           `json:"source"` // K7 cell, or the cited M1 D1 cell
	C3Classes   []string         `json:"c3_classes"`
	C3Counts    map[string]int64 `json:"c3_counts"`
	Result      string           `json:"result"` // confirmed | oracle-disagreement
	ElleValid   string           `json:"elle_valid"`
	ElleClasses []string         `json:"elle_classes"`
	DecidedBy   string           `json:"decided_by"` // cpcheck --elle-oracle | elle-cli validity pass | none
	Records     []string         `json:"records"`
	Note        string           `json:"note,omitempty"`
	Triage      string           `json:"triage,omitempty"`
}

// lastRecord returns the latest record named name or name-<n> (an attempt
// re-run under the next name), or false.
func (e *Evidence) lastRecord(name string) (Record, bool) {
	re := regexp.MustCompile("^" + regexp.QuoteMeta(name) + "(-[0-9]+)?$")
	var got Record
	ok := false
	for _, r := range e.Records {
		if re.MatchString(r.Name) {
			got, ok = r, true
		}
	}
	return got, ok
}

// DecideK7Oracle derives ADR-0046's result for a run from its checks.json
// (C3's stats.elle when cpcheck --elle-oracle completed) and, otherwise, the
// recorded elle-cli validity pass elle-validity-<run-id> of evidence e.
// Triage and Source are left to the caller.
func DecideK7Oracle(e *Evidence, checksJSON []byte) (*K7Oracle, error) {
	c, err := output.Decode(checksJSON)
	if err != nil {
		return nil, err
	}
	o := &K7Oracle{RunID: c.RunID, SealRoot: c.SealRoot, C3Counts: map[string]int64{}}
	var c3 *output.Checker
	for i := range c.Checkers {
		if c.Checkers[i].ID == "C3" {
			c3 = &c.Checkers[i]
		}
	}
	if c3 == nil || c3.Valid != output.ValidFalse {
		return nil, fmt.Errorf("%s: C3 is not invalid; K7.3 concerns C3 anomalies only", c.RunID)
	}
	cm, _ := c3.Stats["counts"].(map[string]any)
	for k, v := range cm {
		n, _ := v.(json.Number).Int64()
		if n > 0 {
			o.C3Counts[k] = n
			o.C3Classes = append(o.C3Classes, k)
		}
	}
	sort.Strings(o.C3Classes)
	if r, ok := e.lastRecord("oracle-" + c.RunID); ok {
		o.Records = append(o.Records, r.Name)
	}
	// 1. cpcheck --elle-oracle's result.
	if el, ok := c3.Stats["elle"].(map[string]any); ok {
		o.ElleValid = fmt.Sprint(el["valid"])
		o.ElleClasses = strListOf(el["classes"])
		agree := fmt.Sprint(el["agree"]) == "true"
		switch {
		case o.ElleValid == "false" && agree:
			o.Result, o.DecidedBy = "confirmed", "cpcheck --elle-oracle"
			return o, nil
		case o.ElleValid != "unknown":
			o.Result, o.DecidedBy = "oracle-disagreement", "cpcheck --elle-oracle"
			return o, nil
		}
		o.Note = "cpcheck's oracle: elle-cli answered unknown, not a confirmation (ADR-0046)"
	}
	// 2. The validity pass on its own (K4's approach).
	r, ok := e.lastRecord("elle-validity-" + c.RunID)
	if !ok {
		if o.ElleValid == "" {
			return nil, fmt.Errorf("%s: no stats.elle in checks.json and no elle-validity record", c.RunID)
		}
		o.Result, o.DecidedBy = "oracle-disagreement", "none"
		return o, nil
	}
	o.Records = append(o.Records, r.Name)
	if pr, ok := e.lastRecord("elle-process-" + c.RunID); ok {
		o.Records = append(o.Records, pr.Name)
	}
	if !argvHas(r, "commitproof/elle-oracle", c.RunID+".elle") {
		return nil, fmt.Errorf("record %s is not elle-cli on this run's export", r.Name)
	}
	_, out, err := e.Get(r.Name)
	if err != nil {
		return nil, err
	}
	var v map[string]any
	if (r.ExitCode != 0 && r.ExitCode != 1) || json.Unmarshal(out, &v) != nil {
		o.Result, o.DecidedBy = "oracle-disagreement", "none"
		o.ElleValid, o.ElleClasses = "no answer", nil
		o.Note = strings.TrimSpace(o.Note + fmt.Sprintf("; elle-cli's validity pass did not complete (exit %d)", r.ExitCode))
		return o, nil
	}
	o.ElleValid = fmt.Sprint(v["valid?"])
	o.ElleClasses = nil
	for _, a := range strListOf(v["anomaly-types"]) {
		if n := elleName(a); n != "" {
			o.ElleClasses = append(o.ElleClasses, n)
		}
	}
	sort.Strings(o.ElleClasses)
	o.DecidedBy = "elle-cli validity pass"
	if o.ElleValid == "false" && strings.Join(o.ElleClasses, ",") == strings.Join(o.C3Classes, ",") {
		o.Result = "confirmed"
	} else {
		o.Result = "oracle-disagreement"
	}
	return o, nil
}
