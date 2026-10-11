//go:build cp_sabotage_prefix_check

package checker

// Deliberately broken rule (CONTRACT §15 K2.6): never in product builds.
const sabotagePrefixCheck = true

// cpSabotageActive_prefix_check marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_prefix_check = registerSabotage("prefix_check")
