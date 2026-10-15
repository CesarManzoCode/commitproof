//go:build cp_sabotage_report_nondeterminism

package report

// Deliberately broken rule (CONTRACT §15 K6, checker-level sabotage of
// K6.1): client lanes and per-gateway rows in map iteration order. Never in
// product builds.
const sabotageNondeterminism = true

// cpSabotageActive_report_nondeterminism marks the binary as sabotaged
// (looked for by the independence test).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_report_nondeterminism = "report_nondeterminism"

// ActiveSabotage returns the sabotage rules compiled into this binary.
func ActiveSabotage() []string { return []string{cpSabotageActive_report_nondeterminism} }
