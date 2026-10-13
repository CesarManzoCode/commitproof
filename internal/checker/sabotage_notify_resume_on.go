//go:build cp_sabotage_notify_resume

package checker

// Deliberately broken rule (CONTRACT §15 K5.1): never in product builds.
const sabotageNotifyResume = true

// cpSabotageActive_notify_resume marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_notify_resume = registerSabotage("notify_resume")
