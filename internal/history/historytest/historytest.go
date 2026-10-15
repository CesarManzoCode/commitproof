// Package historytest writes sealed run directories whose events carry
// chosen times, for tests of the readers of sealed runs (report, gate).
// Production runs are written by history.Writer, which assigns t itself.
package historytest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/CesarManzoCode/commitproof/internal/canonjson"
	"github.com/CesarManzoCode/commitproof/internal/history"
)

// WriteChain writes events as a canonical, hash-chained stream: each event
// must carry its "t"; i and prev are assigned.
func WriteChain(path string, events []map[string]any) error {
	var out []byte
	prev := history.ZeroHash
	for i, e := range events {
		if _, ok := e["t"]; !ok {
			return fmt.Errorf("event %d has no t", i)
		}
		ev := map[string]any{}
		for k, v := range e {
			ev[k] = v
		}
		ev["i"], ev["prev"] = int64(i), prev
		line, err := canonjson.Marshal(ev)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(line)
		prev = hex.EncodeToString(sum[:])
		out = append(append(out, line...), '\n')
	}
	return os.WriteFile(path, out, 0o644)
}

// WriteRun writes and seals dir (named by manifest's run_id) with the given
// history and observation events, and returns the seal's Merkle root.
func WriteRun(dir string, manifest map[string]any, events, obs []map[string]any) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	mb, err := canonjson.Marshal(manifest)
	if err != nil {
		return "", err
	}
	files := map[string][]byte{history.FileManifest: append(mb, '\n'), history.FilePlan: []byte("{}\n"), history.FileCommands: nil}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return "", err
		}
	}
	if err := WriteChain(filepath.Join(dir, history.FileHistory), events); err != nil {
		return "", err
	}
	if err := WriteChain(filepath.Join(dir, history.FileObservations), obs); err != nil {
		return "", err
	}
	s, err := history.SealDir(dir)
	if err != nil {
		return "", err
	}
	return s.MerkleRoot, nil
}

// ms is a test time in nanoseconds.
func ms(v int64) int64 { return v * 1_000_000 }

// FailoverRunID names the run of Failover.
const FailoverRunID = "20260101T000000Z-T1-1-real-00000000"

// Failover is a small sealed-run scenario for the K6 report and its gate
// cross-check (ADR-0036): two clients on gw-z1 and gw-z2, one F2a fault
// (inject 10 s, confirmed and heal 10.3 s, healed 13 s), a fail and an
// info during it, a final read, and pooler observations of rules 1.0 (n1),
// 2.0 (n2) and 2.1 (n2). Tests state the expected metrics by hand.
func Failover() (manifest map[string]any, events, obs []map[string]any) {
	manifest = map[string]any{"run_id": FailoverRunID, "campaign": "T1", "profile": "baseline", "mutant": "real", "seed": int64(1),
		"multigres_commit": "e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73"}
	inv := func(t, proc, client int64, gw, f string) map[string]any {
		return map[string]any{"t": ms(t), "type": history.TypeInvoke, "proc": proc, "client": client, "gw": gw, "wl": "ledger", "f": f, "value": map[string]any{"id": t}}
	}
	done := func(t int64, typ string, op, proc, client int64, gw, f string) map[string]any {
		e := map[string]any{"t": ms(t), "type": typ, "op": op, "proc": proc, "client": client, "gw": gw, "wl": "ledger", "f": f}
		switch typ {
		case history.TypeFail:
			e["err"] = map[string]any{"sqlstate": "57P03", "class": "57", "phase": "connect"}
		case history.TypeInfo:
			e["err"] = map[string]any{"sqlstate": "", "class": "timeout", "phase": "client-timeout"}
		}
		return e
	}
	nem := func(t int64, phase string) map[string]any {
		return map[string]any{"t": ms(t), "type": history.TypeNemesis, "fault": map[string]any{"id": "f0", "kind": "F2a", "phase": phase, "targets": []any{"n1"}, "evidence": map[string]any{}}}
	}
	mark := func(t int64, name string) map[string]any {
		return map[string]any{"t": ms(t), "type": history.TypeMarker, "value": map[string]any{"name": name}}
	}
	events = []map[string]any{
		mark(0, "run-start"),                                           // 0
		inv(1000, 0, 0, "gw-z1", "insert"),                             // 1
		done(1010, history.TypeOK, 1, 0, 0, "gw-z1", "insert"),         // 2
		inv(2000, 1, 1, "gw-z2", "insert"),                             // 3
		done(2020, history.TypeOK, 3, 1, 1, "gw-z2", "insert"),         // 4
		nem(10000, "inject"),                                           // 5
		inv(10100, 0, 0, "gw-z1", "insert"),                            // 6
		done(10200, history.TypeFail, 6, 0, 0, "gw-z1", "insert"),      // 7
		nem(10300, "confirmed"),                                        // 8
		nem(10300, "heal"),                                             // 9
		inv(10400, 1, 1, "gw-z2", "insert"),                            // 10
		inv(11000, 0, 0, "gw-z1", "insert"),                            // 11
		done(11030, history.TypeOK, 11, 0, 0, "gw-z1", "insert"),       // 12
		done(12400, history.TypeInfo, 10, 1, 1, "gw-z2", "insert"),     // 13
		nem(13000, "healed"),                                           // 14
		inv(13500, 13, 1, "gw-z2", "insert"),                           // 15
		done(13540, history.TypeOK, 15, 13, 1, "gw-z2", "insert"),      // 16
		inv(14000, 0, 0, "gw-z1", "insert"),                            // 17
		done(14050, history.TypeOK, 17, 0, 0, "gw-z1", "insert"),       // 18
		inv(30000, 14, 10, "gw-z1", "final-read"),                      // 19
		done(30010, history.TypeOK, 19, 14, 10, "gw-z1", "final-read"), // 20
		mark(31000, "run-end"),                                         // 21
	}
	rule := func(term, sub int64, leader string) map[string]any {
		return map[string]any{"coordinator_term": term, "leader_subterm": sub, "leader": leader, "cohort": []any{"n1", "n2", "n3"}}
	}
	status := func(t int64, node string, d map[string]any) map[string]any {
		v := map[string]any{"src": "pooler-status", "node": node, "ok": true, "decision": d, "pooler_type": "REPLICA",
			"postgres_running": true, "postgres_status": "POSTGRES_STATUS_STANDBY", "wal_position": "0/4000000", "latency_ms": int64(5)}
		return map[string]any{"t": ms(t), "type": history.TypeObs, "value": v}
	}
	obs = []map[string]any{
		{"t": ms(500), "type": history.TypeObs, "value": map[string]any{"src": "healthy", "healthy": true, "rule": rule(1, 0, "n1")}},
		status(10500, "n2", rule(2, 0, "n2")),
		status(11500, "n3", rule(2, 0, "n2")),
		{"t": ms(12000), "type": history.TypeObs, "value": map[string]any{"src": "pooler-status", "node": "n1", "ok": false, "error": "unavailable", "latency_ms": int64(0)}},
		status(20000, "n1", rule(2, 1, "n2")),
	}
	return manifest, events, obs
}

// FailoverChecks is a checks.json for Failover naming one anomaly whose ops
// are an invoke (1) and a completion (12, of the op invoked at 11).
func FailoverChecks(sealRoot string) []byte {
	b, _ := canonjson.Marshal(map[string]any{"run_id": FailoverRunID, "seal_root": sealRoot, "verdict": "FAIL",
		"checkers": []any{map[string]any{"id": "C1", "valid": false, "stats": map[string]any{"observations": []any{}},
			"anomalies": []any{map[string]any{"class": "lost-write", "ops": []any{int64(1), int64(12)}, "explanation": "test"}}}}})
	return append(b, '\n')
}
