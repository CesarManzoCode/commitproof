//go:build !cp_sabotage_report_nondeterminism

package report

const sabotageNondeterminism = false

// ActiveSabotage returns the sabotage rules compiled into this binary
// (none in product builds).
func ActiveSabotage() []string { return nil }
