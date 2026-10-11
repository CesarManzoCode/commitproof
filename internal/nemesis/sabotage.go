package nemesis

import (
	"sort"
	"strings"
)

// Nemesis-level sabotage (CONTRACT §15 K3.2, K3.3). Building commitproof
// with the tag cp_sabotage_inject_<kind> (kind lower-case, e.g. f1a) turns
// the injector of that fault kind into a no-op; cp_sabotage_heal_<kind>
// does the same for its heal. The confirmation and heal predicates are
// untouched, so a sabotaged run must end with not-applied / not-healed and
// be INVALID. Only the sabotage_*_on.go files call registerSabotage;
// product builds never set these tags, and a test asserts the release
// binaries contain no cpSabotageActive symbol.
var sabotageRules = []string{}

// registerSabotage records an active sabotage; called only from _on files.
//
//lint:ignore U1000 called only by the sabotage builds (cp_sabotage_* tags)
func registerSabotage(rule string) string {
	sabotageRules = append(sabotageRules, rule)
	sort.Strings(sabotageRules)
	return rule
}

// ActiveSabotage returns the sabotage rules compiled into this binary
// (empty in product builds), e.g. "inject_f1a".
func ActiveSabotage() []string { return append([]string{}, sabotageRules...) }

func sabotageTag(rule string) string {
	for _, r := range sabotageRules {
		if r == rule {
			return "cp_sabotage_" + r
		}
	}
	return ""
}

// injectNoop returns the active tag that disables kind's injector, or "".
func injectNoop(kind string) string { return sabotageTag("inject_" + strings.ToLower(kind)) }

// healNoop returns the active tag that disables kind's heal, or "".
func healNoop(kind string) string { return sabotageTag("heal_" + strings.ToLower(kind)) }
