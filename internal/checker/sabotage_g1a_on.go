//go:build cp_sabotage_g1a

package checker

// Deliberately broken rule (CONTRACT §15 K2.6): never in product builds.
const sabotageG1a = true

// cpSabotageActive_g1a marks the binary as sabotaged (see sabotage.go).
var cpSabotageActive_g1a = registerSabotage("g1a")
