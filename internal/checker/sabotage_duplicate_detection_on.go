//go:build cp_sabotage_duplicate_detection

package checker

// Deliberately broken rule (CONTRACT §15 K2.6): never in product builds.
const sabotageDuplicateDetection = true

// cpSabotageActive_duplicate_detection marks the binary as sabotaged (see sabotage.go).
var cpSabotageActive_duplicate_detection = registerSabotage("duplicate_detection")
