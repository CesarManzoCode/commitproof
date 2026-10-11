//go:build cp_sabotage_porcupine_cas

package checker

// Deliberately broken rule (CONTRACT §15 K2.6): never in product builds.
const sabotagePorcupineCAS = true

// cpSabotageActive_porcupine_cas marks the binary as sabotaged (see sabotage.go).
var cpSabotageActive_porcupine_cas = registerSabotage("porcupine_cas")
