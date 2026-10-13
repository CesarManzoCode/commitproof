//go:build cp_sabotage_planned_failover

package checker

// Deliberately broken rule (CONTRACT §15 K5.1): never in product builds.
const sabotagePlannedFailover = true

// cpSabotageActive_planned_failover marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_planned_failover = registerSabotage("planned_failover")
