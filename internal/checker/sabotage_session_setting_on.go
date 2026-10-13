//go:build cp_sabotage_session_setting

package checker

// Deliberately broken rule (CONTRACT §15 K5.1): never in product builds.
const sabotageSessionSetting = true

// cpSabotageActive_session_setting marks the binary as sabotaged (see sabotage.go).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_session_setting = registerSabotage("session_setting")
