//go:build cp_sabotage_bank_sum

package checker

// Deliberately broken rule (CONTRACT §15 K2.6): never in product builds.
const sabotageBankSum = true

// cpSabotageActive_bank_sum marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_bank_sum = registerSabotage("bank_sum")
