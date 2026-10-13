//go:build cp_sabotage_overload_failover

package checker

// Deliberately broken rule (CONTRACT §15 K5.1): never in product builds.
const sabotageOverloadFailover = true

// cpSabotageActive_overload_failover marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_overload_failover = registerSabotage("overload_failover")
