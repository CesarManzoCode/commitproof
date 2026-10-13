//go:build cp_sabotage_fair_share

package checker

// Deliberately broken rule (CONTRACT §15 K5.1): never in product builds.
const sabotageFairShare = true

// cpSabotageActive_fair_share marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_fair_share = registerSabotage("fair_share")
