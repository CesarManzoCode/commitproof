package checker

import "sort"

// Checker-level sabotage (CONTRACT §15 K2.6). Each rule below can be broken
// deliberately by building with the tag cp_sabotage_<rule>; the files
// sabotage_<rule>_on.go / _off.go define the constant that the rule checks.
// Product builds never set these tags; a test asserts that the release
// binaries contain no cpSabotageActive symbol, which only the _on files
// define.
//
// Rules: rw_edge (C3 rw-edge inference), prefix_check (C3 prefix check),
// duplicate_detection (C3 duplicate elements), lost_write_diff (C1 set
// difference), g1a (C2/C3 aborted-read detection), porcupine_cas (C4 model's
// cas step), bank_sum (C5 sum), cdc_phantom (C7 phantom), cdc_duplicate_bound
// (C7 duplicate bound).
var sabotageRules = []string{}

// registerSabotage records an active sabotage; called only from _on files.
func registerSabotage(rule string) string {
	sabotageRules = append(sabotageRules, rule)
	sort.Strings(sabotageRules)
	return rule
}

// ActiveSabotage returns the sabotage rules compiled into this binary
// (empty in product builds).
func ActiveSabotage() []string { return append([]string{}, sabotageRules...) }
