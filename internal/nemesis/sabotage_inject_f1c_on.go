//go:build cp_sabotage_inject_f1c

package nemesis

// Deliberately broken injector (CONTRACT §15 K3.2): never in product builds.
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_inject_f1c = registerSabotage("inject_f1c")
