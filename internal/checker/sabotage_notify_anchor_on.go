//go:build cp_sabotage_notify_anchor

package checker

// Deliberately broken rule (CONTRACT §15 K5.1): never in product builds.
const sabotageNotifyAnchor = true

// cpSabotageActive_notify_anchor marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_notify_anchor = registerSabotage("notify_anchor")
