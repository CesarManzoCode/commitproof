//go:build cp_sabotage_cdc_phantom

package checker

// Deliberately broken rule (CONTRACT §15 K2.6): never in product builds.
const sabotageCDCPhantom = true

// cpSabotageActive_cdc_phantom marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_cdc_phantom = registerSabotage("cdc_phantom")
