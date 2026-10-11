//go:build cp_sabotage_inject_f2d

package nemesis

// Deliberately broken injector (CONTRACT §15 K3.2): never in product builds.
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_inject_f2d = registerSabotage("inject_f2d")
