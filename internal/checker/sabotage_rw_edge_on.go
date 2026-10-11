//go:build cp_sabotage_rw_edge

package checker

// Deliberately broken rule (CONTRACT §15 K2.6): never in product builds.
const sabotageRWEdge = true

// cpSabotageActive_rw_edge marks the binary as sabotaged (see sabotage.go).
var cpSabotageActive_rw_edge = registerSabotage("rw_edge")
