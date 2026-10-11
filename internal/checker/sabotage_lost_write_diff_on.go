//go:build cp_sabotage_lost_write_diff

package checker

// Deliberately broken rule (CONTRACT §15 K2.6): never in product builds.
const sabotageLostWriteDiff = true

// cpSabotageActive_lost_write_diff marks the binary as sabotaged (see sabotage.go).
var cpSabotageActive_lost_write_diff = registerSabotage("lost_write_diff")
