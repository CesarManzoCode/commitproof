//go:build cp_sabotage_cdc_duplicate_bound

package checker

// Deliberately broken rule (CONTRACT §15 K2.6): never in product builds.
const sabotageCDCDuplicateBound = true

// cpSabotageActive_cdc_duplicate_bound marks the binary as sabotaged (see sabotage.go).
var cpSabotageActive_cdc_duplicate_bound = registerSabotage("cdc_duplicate_bound")
