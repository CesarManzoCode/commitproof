package gate

// K2SabotageDeps is, for each checker sabotage rule (build tag
// cp_sabotage_<rule>, CONTRACT §15 K2.6), the set of corpus labels whose
// histories must stop matching their expected.json when that rule is
// broken — and no others. Written from the rules and the planted
// expectations, before any sabotage build ran:
//
//   - rw_edge: no rw edges, so no cycle with an anti-dependency: G-single,
//     G-nonadjacent, G2-item and their -process/-realtime variants, and g1b
//     (an intermediate read also yields G-single). G0 and G1c need no rw.
//   - prefix_check: incompatible reads are not reported (the remaining edges
//     of the planted key form no cycle).
//   - duplicate_detection: duplicate elements are not reported.
//   - lost_write_diff: C1 never finds a lost write.
//   - g1a: neither C2 nor C3 reports an aborted read: every g1a plant, and
//     dirty-update (whose expectation includes g1a).
//   - porcupine_cas: cas operations constrain nothing; only the plants whose
//     violation is a cas are non-linearizable through cas (label
//     non-linearizable); stale-read and g1a register plants have no cas.
//   - bank_sum: sums are not checked.
//   - cdc_phantom: phantoms are not reported.
//   - cdc_duplicate_bound: the bound becomes lsn < flushed; the plants that
//     re-deliver at exactly the confirmed flush are missed (label
//     cdc-duplicate); legitimate re-deliveries above it stay allowed.
var K2SabotageDeps = map[string][]string{
	"rw_edge": {"G-single", "G-nonadjacent", "G2-item", "G-single-process", "G-nonadjacent-process", "G2-item-process",
		"G-single-realtime", "G-nonadjacent-realtime", "G2-item-realtime", "g1b"},
	"prefix_check":        {"incompatible-order"},
	"duplicate_detection": {"duplicate-elements"},
	"lost_write_diff":     {"lost-write"},
	"g1a":                 {"g1a", "dirty-update"},
	"porcupine_cas":       {"non-linearizable"},
	"bank_sum":            {"bank-sum"},
	"cdc_phantom":         {"cdc-phantom"},
	"cdc_duplicate_bound": {"cdc-duplicate"},
}

// K2Classes are the anomaly classes of CONTRACT §10.1–§10.7 and
// §10.9–§10.10 that K2.1 requires at least three planted histories of
// (observation classes included).
var K2Classes = []string{
	"lost-write", "final-read-divergence", // §10.1
	"g1a",                                                                         // §10.2
	"duplicate-elements", "incompatible-order", "g1b", "internal", "dirty-update", // §10.3
	"G0", "G1c", "G-single", "G-nonadjacent", "G2-item", // §10.3 cycles
	"G0-process", "G1c-process", "G-single-process", "G-nonadjacent-process", "G2-item-process",
	"G0-realtime", "G1c-realtime", "G-single-realtime", "G-nonadjacent-realtime", "G2-item-realtime",
	"non-linearizable",          // §10.4
	"bank-sum", "bank-negative", // §10.5
	"stale-read",                                           // §10.6
	"cdc-gap", "cdc-phantom", "cdc-duplicate", "cdc-order", // §10.7
	"session-setting-lost", "prepared-statement-lost", "notify-not-resumed", "notify-window-loss",
	"fair-share-starvation", "overload-failover", // §10.9
	"planned-failover-error", // §10.10
}
