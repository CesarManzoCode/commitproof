//go:build cp_sabotage_prepared_statement

package checker

// Deliberately broken rule (CONTRACT §15 K5.1): never in product builds.
const sabotagePreparedStatement = true

// cpSabotageActive_prepared_statement marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_prepared_statement = registerSabotage("prepared_statement")
