//go:build cp_sabotage_cdc_gap

package checker

// Deliberately broken rule (CONTRACT §15 K5.1): never in product builds.
const sabotageCDCGap = true

// cpSabotageActive_cdc_gap marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_cdc_gap = registerSabotage("cdc_gap")
