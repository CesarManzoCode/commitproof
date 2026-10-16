//go:build cp_sabotage_g12_judged_minimum

package checker

// Deliberately broken rule (ADR-0037): never in product builds.
const sabotageG12JudgedMinimum = true

// cpSabotageActive_g12_judged_minimum marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_g12_judged_minimum = registerSabotage("g12_judged_minimum")
