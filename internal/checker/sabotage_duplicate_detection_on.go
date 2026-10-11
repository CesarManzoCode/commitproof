//go:build cp_sabotage_duplicate_detection

package checker

// Deliberately broken rule (CONTRACT §15 K2.6): never in product builds.
const sabotageDuplicateDetection = true

// cpSabotageActive_duplicate_detection marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_duplicate_detection = registerSabotage("duplicate_detection")
