//go:build cp_sabotage_buffer_guard

package checker

// Deliberately broken rule (CONTRACT §15 K5.1): never in product builds.
const sabotageBufferGuard = true

// cpSabotageActive_buffer_guard marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_buffer_guard = registerSabotage("buffer_guard")
