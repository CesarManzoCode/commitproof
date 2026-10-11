//go:build cp_sabotage_heal_f3a

package nemesis

// Deliberately broken heal (CONTRACT §15 K3.3): never in product builds.
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_heal_f3a = registerSabotage("heal_f3a")
