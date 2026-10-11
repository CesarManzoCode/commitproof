// Command k4obs summarises sealed K4 runs for evidence/K4/NOTES.md: the
// exposure of each run (operations, ok writes, confirmed faults by kind and
// fault time; the inputs of CONTRACT §12.3's statements), completions by
// workload and outcome, error SQLSTATEs and phases, server notices, client
// CancelRequests (C7), and for writes whose outcome the client could not
// know (info) whether they are present in the stable final read. It reads
// the history (and observations for CancelRequests); it judges nothing —
// verdicts are cpcheck's (ADR-0016).
//
//	go run ./internal/tools/k4obs <run-dir>... > summary.json
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/CesarManzoCode/commitproof/internal/history"
)

func num(v any) (int64, bool) {
	switch x := v.(type) {
	case json.Number:
		n, err := x.Int64()
		return n, err == nil
	case float64:
		return int64(x), true
	}
	return 0, false
}

type key struct{ wl, typ string }

func summarize(dir string) (map[string]any, error) {
	rep := history.VerifySeal(dir)
	if !rep.Valid {
		return nil, fmt.Errorf("%s: seal: %v", dir, rep.Problems)
	}
	inv := map[int64]history.Event{}
	counts := map[string]int64{}
	errs := map[string]int64{}
	notices := map[string]int64{}
	var okWrites int64
	infoLedger := map[int64]bool{}    // ids of ledger inserts completed info
	infoAppend := map[[2]int64]bool{} // (key, value) of appends in info txns
	faultTime := map[string]int64{}   // kind → ns between confirmed and heal
	confirmedAt := map[string]int64{} // fault id → t
	kinds := map[string]string{}      // fault id → kind
	confirmedByKind := map[string]int64{}
	var finalLedger []any
	var finalAppend []any
	for _, ev := range rep.History {
		switch ev.Type {
		case history.TypeInvoke:
			inv[ev.I] = ev
		case history.TypeNemesis:
			id, ph := fmt.Sprint(ev.Fault["id"]), fmt.Sprint(ev.Fault["phase"])
			kinds[id] = fmt.Sprint(ev.Fault["kind"])
			switch ph {
			case "confirmed":
				confirmedAt[id] = ev.T
				confirmedByKind[kinds[id]]++
			case "heal":
				if t, ok := confirmedAt[id]; ok {
					faultTime[kinds[id]] += ev.T - t
				}
			}
		case history.TypeOK, history.TypeFail, history.TypeInfo:
			if ev.Client == nil {
				continue
			}
			if ev.F == "final-read" {
				if ev.Type == history.TypeOK {
					switch ev.WL {
					case "ledger":
						finalLedger, _ = ev.Value.([]any)
					case "append":
						finalAppend, _ = ev.Value.([]any)
					}
				}
				continue
			}
			if *ev.Client >= 10 {
				continue // recovery writes
			}
			counts[ev.WL+"/"+ev.Type]++
			if ev.Err != nil {
				errs[fmt.Sprintf("%s/%s/%v/%v", ev.WL, ev.Type, ev.Err["sqlstate"], ev.Err["phase"])]++
			}
			for _, n := range ev.Notices {
				m, _ := n.(map[string]any)
				notices[fmt.Sprintf("%s/%s: %v %v", ev.WL, ev.Type, m["severity"], m["message"])]++
			}
			in := inv[*ev.Op]
			switch ev.WL {
			case "ledger":
				v, _ := in.Value.(map[string]any)
				id, _ := num(v["id"])
				if ev.Type == history.TypeOK {
					okWrites++
				} else if ev.Type == history.TypeInfo {
					infoLedger[id] = true
				}
			case "append":
				mops, _ := in.Value.([]any)
				for _, m := range mops {
					mo, _ := m.([]any)
					if len(mo) == 3 && mo[0] == "append" {
						k, _ := num(mo[1])
						v, _ := num(mo[2])
						if ev.Type == history.TypeOK {
							okWrites++
						} else if ev.Type == history.TypeInfo {
							infoAppend[[2]int64{k, v}] = true
						}
					}
				}
			case "register":
				if ev.Type == history.TypeOK && (ev.F == "write" || ev.F == "cas") {
					okWrites++
				}
			case "bank":
				if ev.Type == history.TypeOK && ev.F == "transfer" {
					okWrites++
				}
			}
		}
	}
	present := func() (int64, int64, int64, int64) {
		inL := map[int64]bool{}
		for _, x := range finalLedger {
			if n, ok := num(x); ok {
				inL[n] = true
			}
		}
		inA := map[[2]int64]bool{}
		for _, row := range finalAppend {
			r, _ := row.([]any)
			if len(r) != 2 {
				continue
			}
			k, _ := num(r[0])
			vals, _ := r[1].([]any)
			for _, v := range vals {
				n, _ := num(v)
				inA[[2]int64{k, n}] = true
			}
		}
		var lp, la, ap, aa int64
		for id := range infoLedger {
			if inL[id] {
				lp++
			} else {
				la++
			}
		}
		for kv := range infoAppend {
			if inA[kv] {
				ap++
			} else {
				aa++
			}
		}
		return lp, la, ap, aa
	}
	lp, la, ap, aa := present()
	cancels := map[string]int64{}
	if obs, err := history.ReadChain(filepath.Join(dir, history.FileObservations)); err == nil {
		for _, o := range obs {
			v, _ := o.Value.(map[string]any)
			if v["event"] == "cancel-request" {
				cancels[fmt.Sprintf("%v → %v", v["result"], v["completion"])]++
			}
		}
	}
	ft := map[string]any{}
	for k, v := range faultTime {
		ft[k] = v / 1_000_000
	}
	cb := map[string]any{}
	for k, v := range confirmedByKind {
		cb[k] = v
	}
	man := map[string]any{}
	if b, err := os.ReadFile(filepath.Join(dir, history.FileManifest)); err == nil {
		json.Unmarshal(b, &man)
	}
	return map[string]any{
		"run_id": man["run_id"], "campaign": man["campaign"], "seed": man["seed"], "mutant": man["mutant"], "profile": man["profile"],
		"completions": counts, "errors": errs, "notices": notices, "ok_writes": okWrites,
		"faults_confirmed_by_kind": cb, "fault_time_ms_by_kind": ft, "cancel_requests": cancels,
		"info_writes_in_final_read": map[string]any{"ledger_present": lp, "ledger_absent": la, "append_present": ap, "append_absent": aa},
	}, nil
}

func main() {
	var res []any
	args := os.Args[1:]
	sort.Strings(args)
	for _, d := range args {
		s, err := summarize(d)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		res = append(res, s)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(res)
}
