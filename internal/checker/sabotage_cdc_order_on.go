//go:build cp_sabotage_cdc_order

package checker

// Deliberately broken rule (CONTRACT §15 K5.1): never in product builds.
const sabotageCDCOrder = true

// cpSabotageActive_cdc_order marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_cdc_order = registerSabotage("cdc_order")
