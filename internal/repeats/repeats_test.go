package repeats

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/CesarManzoCode/commitproof/internal/checker/output"
)

// The expectations below are written from ADR-0045's definitions and the
// recorded triage of each run (evidence/K4/triage, evidence/K5/triage), not
// by running Classify.

func load(t *testing.T, path string) *output.Checks {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	c, err := output.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func set(ks ...string) map[string]bool {
	m := map[string]bool{}
	for _, k := range ks {
		m[k] = true
	}
	return m
}

// The recorded runs of the four mechanisms (plans before ADR-0044: F is
// each plan's fault kinds, every fault of a valid run being confirmed).
func TestRecordedFindings(t *testing.T) {
	k4, k5 := "../../evidence/K4/runs/", "../../evidence/K5/runs/"
	cases := []struct {
		run       string
		f         map[string]bool
		repeat    bool
		preds     []string // per group, in order ("" = none)
		guarantee []string
		target    string
	}{
		// G2b: D1 real seeds 1, 3, 4 (F1c only).
		{k4 + "20261011T221027Z-D1-1-real-59a95336", set("F1c"), true, []string{"R-G2b"}, []string{"G2b"}, ""},
		{k4 + "20261011T221506Z-D1-3-real-da159fa0", set("F1c"), true, []string{"R-G2b"}, []string{"G2b"}, ""},
		{k4 + "20261011T221728Z-D1-4-real-ad3fca18", set("F1c"), true, []string{"R-G2b"}, []string{"G2b"}, ""},
		// C2 seed 3: G2b (C3) and G6 (C4), F1c.
		{k4 + "20261011T231036Z-C2-3-real-f4505b54", set("F1c"), true, []string{"R-G2b", "R-G6"}, []string{"G2b", "G6"}, ""},
		// G10: C9 seeds 1–3 (F2a, F2b, F6).
		{k5 + "20261013T230136Z-C9-1-real-21458975", set("F2a", "F2b", "F6"), true, []string{"R-G10"}, []string{"G10"}, ""},
		{k5 + "20261013T230358Z-C9-2-real-b0861f34", set("F2a", "F2b", "F6"), true, []string{"R-G10"}, []string{"G10"}, ""},
		{k5 + "20261013T230909Z-C9-3-real-bb8a276f", set("F2a", "F2b", "F6"), true, []string{"R-G10"}, []string{"G10"}, ""},
		// G12: C11 seed 2 was judged by the literal rule; its checks.json
		// has no per-window fractions, so R-G12 cannot match (ADR-0045 §2).
		{k5 + "20261013T232750Z-C11-2-real-1f5423ed", set("F2a"), false, []string{""}, []string{"G12"}, "fair-share-starvation"},
		// M1 on D1: lost-write (G1, no predicate) and incompatible-order
		// with C1 invalid: new; target C1's lost-write.
		{k4 + "20261011T215900Z-D1-1-M1-59a95336", set("F1c"), false, []string{"", ""}, []string{"G1", "G2b"}, "lost-write"},
	}
	for _, tc := range cases {
		r := ClassifyWith(load(t, tc.run+".checks.json"), tc.f, true)
		if r.Repeat != tc.repeat || r.Target != tc.target || len(r.Groups) != len(tc.preds) {
			t.Errorf("%s: repeat %v target %q groups %d; want %v %q %d (%+v)", tc.run, r.Repeat, r.Target, len(r.Groups), tc.repeat, tc.target, len(tc.preds), r.Groups)
			continue
		}
		for i, g := range r.Groups {
			if g.Predicate != tc.preds[i] || g.Guarantee != tc.guarantee[i] {
				t.Errorf("%s group %d: %s %s, want %s %s (failed %v)", tc.run, i, g.Guarantee, g.Predicate, tc.guarantee[i], tc.preds[i], g.Failed)
			}
		}
	}
}

// Fields of R-G2b on D1 seed 1 (triage: value 1 of key 16 read at i=2220,
// absent from the longest read i=3292; 13 client timeouts).
func TestG2bFields(t *testing.T) {
	r := ClassifyWith(load(t, "../../evidence/K4/runs/20261011T221027Z-D1-1-real-59a95336.checks.json"), set("F1c"), true)
	f := r.Groups[0].Fields
	io := f["incompatible_order"].([]any)
	v := io[0].(map[string]any)
	if v["key"] != int64(16) || v["read_i"] != int64(2220) || v["longest_read_i"] != int64(3292) || len(v["absent_from_longest"].([]any)) != 1 || v["absent_from_longest"].([]any)[0] != int64(1) {
		t.Errorf("fields %v", v)
	}
	if f["timeouts"] != int64(13) || f["valid_C1"] != "true" || f["valid_C2"] != "true" {
		t.Errorf("fields %v", f)
	}
}

func checks(verdict string, checkers string) []byte {
	return []byte(`{"run_id":"r","seal_root":"s","verdict":"` + verdict + `","checkers":[` + checkers + `]}`)
}

const (
	okC1  = `{"id":"C1","valid":true,"anomalies":[],"stats":{"counts":{}}}`
	okC2  = `{"id":"C2","valid":true,"anomalies":[],"stats":{"counts":{}}}`
	okC6  = `{"id":"C6","valid":true,"anomalies":[],"stats":{"counts":{}}}`
	c9F1c = `{"id":"C9","valid":true,"anomalies":[],"stats":{"counts":{},"errors":{"info/timeout":2},"exposure":{"confirmed_faults":{"F1c":1,"F2a":0},"fault_time_ns":1,"ok_writes":1,"operations":1}}}`
)

func c3(expl ...string) string {
	var as []string
	for _, e := range expl {
		as = append(as, `{"class":"incompatible-order","ops":[1,2],"explanation":"`+e+`"}`)
	}
	return `{"id":"C3","valid":false,"anomalies":[` + strings.Join(as, ",") + `],"stats":{"counts":{"incompatible-order":` + strconv.Itoa(len(expl)) + `}}}`
}

func TestSynthetic(t *testing.T) {
	vanish := "reads of key 3 are not prefixes of each other: [1 2 9] (i=10) and [1 2 3 4] (i=20)"
	cases := []struct {
		name   string
		b      []byte
		repeat bool
		target string
		failed string // substring of a failed condition ("" = none expected)
	}{
		{"G2b vanish", checks("FAIL", okC1+","+okC2+","+c3(vanish)+","+c9F1c), true, "", ""},
		{"PASS is never a repeat", checks("PASS", okC1+","+okC2+","+c9F1c), false, "", ""},
		{"reordering, not a vanish", checks("FAIL", okC1+","+okC2+","+c3("reads of key 3 are not prefixes of each other: [2 1] (i=10) and [1 2 3] (i=20)")+","+c9F1c), false, "incompatible-order", "a reordering"},
		{"vanished read later than the longest", checks("FAIL", okC1+","+okC2+","+c3("reads of key 3 are not prefixes of each other: [1 2 9] (i=30) and [1 2 3 4] (i=20)")+","+c9F1c), false, "incompatible-order", "not earlier"},
		{"C2 invalid", checks("FAIL", okC1+`,{"id":"C2","valid":false,"anomalies":[{"class":"g1a","ops":[5],"explanation":"x"}],"stats":{"counts":{"g1a":1}}},`+c3(vanish)+","+c9F1c), false, "g1a", "valid(C2) false"},
		{"no F (plan before 1.7.0)", checks("FAIL", okC1+","+okC2+","+c3(vanish)+`,{"id":"C9","valid":true,"anomalies":[],"stats":{"counts":{},"errors":{"info/timeout":2}}}`), false, "incompatible-order", "no confirmed fault kinds"},
		{"no timeout", checks("FAIL", okC1+","+okC2+","+c3(vanish)+`,{"id":"C9","valid":true,"anomalies":[],"stats":{"counts":{},"errors":{},"exposure":{"confirmed_faults":{"F1c":1}}}}`), false, "incompatible-order", "no info/timeout"},
		{"F1a instead of F1c", checks("FAIL", okC1+","+okC2+","+c3(vanish)+`,{"id":"C9","valid":true,"anomalies":[],"stats":{"counts":{},"errors":{"info/timeout":2},"exposure":{"confirmed_faults":{"F1a":1}}}}`), false, "incompatible-order", "F1c not among"},
		{"C3 also G-single (own group)", checks("FAIL", okC1+","+okC2+`,{"id":"C3","valid":false,"anomalies":[{"class":"incompatible-order","ops":[1,2],"explanation":"`+vanish+`"},{"class":"G-single","ops":[3],"explanation":"c"}],"stats":{"counts":{"incompatible-order":1,"G-single":1}}},`+c9F1c), false, "G-single", ""},
		{"G6 vanish", checks("FAIL", okC1+","+okC2+`,{"id":"C4","valid":false,"anomalies":[{"class":"non-linearizable","ops":[7],"explanation":"register 1: x"}],"stats":{"counts":{"non-linearizable":1}}},`+okC6+","+c9F1c), true, "", ""},
		{"G6 with a stale read", checks("FAIL", okC1+","+okC2+`,{"id":"C4","valid":false,"anomalies":[{"class":"non-linearizable","ops":[7],"explanation":"register 1: x"}],"stats":{"counts":{"non-linearizable":1}}},{"id":"C6","valid":false,"anomalies":[{"class":"stale-read","ops":[7,3],"explanation":"y"}],"stats":{"counts":{"stale-read":1}}},`+c9F1c), false, "non-linearizable", "classes [non-linearizable stale-read]"},
		{"G10 one session per listener", checks("FAIL", `{"id":"C8","valid":false,"anomalies":[`+notify(26, 8)+","+notify(27, 9)+`],"stats":{"counts":{"notify-not-resumed":2}}},{"id":"C9","valid":true,"anomalies":[],"stats":{"counts":{},"exposure":{"confirmed_faults":{"F6":1}}}}`), true, "", ""},
		{"G10 listener re-established its session", checks("FAIL", `{"id":"C8","valid":false,"anomalies":[`+notify(26, 8)+","+notify(26, 900)+`],"stats":{"counts":{"notify-not-resumed":2}}},{"id":"C9","valid":true,"anomalies":[],"stats":{"counts":{},"exposure":{"confirmed_faults":{"F6":1}}}}`), false, "notify-not-resumed", "re-established"},
		{"G10 without a leader fault", checks("FAIL", `{"id":"C8","valid":false,"anomalies":[`+notify(26, 8)+`],"stats":{"counts":{"notify-not-resumed":1}}},{"id":"C9","valid":true,"anomalies":[],"stats":{"counts":{},"exposure":{"confirmed_faults":{"F7":1}}}}`), false, "notify-not-resumed", "none of F2a, F2b, F6"},
		{"G12 judged outside fault windows (1.2.0+)", checks("FAIL", `{"id":"C8","valid":false,"anomalies":[{"class":"fair-share-starvation","ops":[],"explanation":"storm phase 1: 50 of 100 cp_steady operations invoked outside fault windows completed ok (< 90%); literal fraction 60 of 120 (ADR-0037)"}],"stats":{"counts":{"fair-share-starvation":1},"storm_windows":[{"phase":1,"result":"fair-share-starvation","judged_ok":50,"judged_total":100,"literal_ok":60,"literal_total":120}]}},{"id":"C9","valid":true,"anomalies":[],"stats":{"counts":{},"exposure":{"confirmed_faults":{"F2a":1}}}}`), false, "fair-share-starvation", "outside fault windows ok (< 90 %)"},
		{"lost-write has no predicate", checks("FAIL", `{"id":"C1","valid":false,"anomalies":[{"class":"lost-write","ops":[4],"explanation":"w"}],"stats":{"counts":{"lost-write":1}}},`+okC2+","+c9F1c), false, "lost-write", ""},
	}
	for _, tc := range cases {
		r, err := Classify(tc.b)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if r.Repeat != tc.repeat || r.Target != tc.target {
			t.Errorf("%s: repeat %v target %q, want %v %q (%+v)", tc.name, r.Repeat, r.Target, tc.repeat, tc.target, r.Groups)
		}
		if tc.failed != "" {
			found := false
			for _, g := range r.Groups {
				for _, fs := range g.Failed {
					for _, f := range fs {
						found = found || strings.Contains(f, tc.failed)
					}
				}
			}
			if !found {
				t.Errorf("%s: no failed condition containing %q in %+v", tc.name, tc.failed, r.Groups)
			}
		}
	}
}

func notify(client, listen int) string {
	return `{"class":"notify-not-resumed","ops":[` + strconv.Itoa(listen) + `,1000],"explanation":"NOTIFY 5 (i=1000) was not received within 10s by listener client ` + strconv.Itoa(client) + `, whose session (LISTEN i=` + strconv.Itoa(listen) + `) was alive and past its first successful op after recovery"}`
}
