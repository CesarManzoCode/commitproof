//go:build cp_sabotage_g12_fault_exclusion

package checker

// Deliberately broken rule (ADR-0037): never in product builds.
const sabotageG12FaultExclusion = true

// cpSabotageActive_g12_fault_exclusion marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_g12_fault_exclusion = registerSabotage("g12_fault_exclusion")
