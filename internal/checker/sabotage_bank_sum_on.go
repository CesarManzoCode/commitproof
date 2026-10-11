//go:build cp_sabotage_bank_sum

package checker

// Deliberately broken rule (CONTRACT §15 K2.6): never in product builds.
const sabotageBankSum = true

// cpSabotageActive_bank_sum marks the binary as sabotaged (see sabotage.go).
var cpSabotageActive_bank_sum = registerSabotage("bank_sum")
