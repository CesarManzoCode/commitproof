//go:build cp_sabotage_report_metrics

package report

// Deliberately broken metrics summary (ADR-0038): each target's first
// scrape is skipped. Never in product builds.
const sabotageMetrics = true

// cpSabotageActive_report_metrics marks the binary as sabotaged (the
// independence test looks for the prefix).
//
//lint:ignore U1000 marker symbol, looked for by the independence test
var cpSabotageActive_report_metrics = true
