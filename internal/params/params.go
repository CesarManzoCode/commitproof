// Package params mirrors the fixed parameters of CONTRACT §5.7.
//
// Every number a gate or a run uses comes from here. Table returns the
// contract's rows verbatim; params_test.go parses the Markdown table in
// docs/CONTRACT.md and fails if the text or any typed constant drifts.
package params

import (
	"fmt"
	"time"
)

// ContractVersion is the docs/CONTRACT.md version this code implements
// (checked against the contract header by params_test.go). K6 closed with
// 1.1.1 and made no run; 1.2.0 (ADR-0037, the owner's decision on
// ADR-0035) changes G12's verdict rule for plans carrying 1.2.0 or later,
// and every recorded run keeps the version it carries.
const ContractVersion = "1.2.0"

// K4RunsContractVersion is the contract version of the K4 runs made before
// ADR-0027. Contract 1.0.7 changes only M2's directed schedule, and as a new
// campaign (D2′, "D2p"); every other K4 plan is unchanged apart from its
// contract_version field, so those runs stay K4 evidence, as the K2 corpus
// keeps CorpusContractVersion. D2′ runs carry K4ContractVersion.
const K4RunsContractVersion = "1.0.6"

// K4ContractVersion is the contract version K4 closed with (D2′ and the
// runs after ADR-0027). K5's corrections and clarifications (1.0.8, 1.0.9:
// ADR-0029, ADR-0030) change no K4 plan; K4's evidence keeps the versions
// its runs carry.
const K4ContractVersion = "1.0.7"

// K5RunsContractVersion is the contract version of the K5 runs made before
// ADR-0033. Contract 1.0.10 changes only M3's directed schedule, and as a
// new campaign (D3′, "D3p"); contract 1.1.0 changes only P25 (ADR-0034).
// Every other K5 plan is unchanged apart from its contract_version field,
// so those runs stay K5 evidence; D3′ runs carry ContractVersion.
const K5RunsContractVersion = "1.0.9"

// K5ContractVersion is the contract version K5 closed with: D3′'s runs
// carry it (ADR-0033, ADR-0034). K6's clarifications (1.1.1, ADR-0036)
// change no plan's schedule; K5's evidence keeps the versions its runs
// carry.
const K5ContractVersion = "1.1.0"

// Typed values. Each one is tied to its row's text by Param.Derived.
const (
	ClientOpTimeout = 10 * time.Second // P01

	ClientsAppend   = 4 // P02
	ClientsRegister = 2
	ClientsBank     = 2
	ClientsLedger   = 2
	ClientsTotal    = ClientsAppend + ClientsRegister + ClientsBank + ClientsLedger

	Pacing = 50 * time.Millisecond // P03

	CampaignDuration = 90 * time.Second // P04
	SmokeDuration    = 45 * time.Second

	FirstFaultAt   = 10 * time.Second // P05
	FaultGap       = 10 * time.Second
	FaultJitterMax = 5 * time.Second

	RecoveryDeadline = 120 * time.Second // P06

	Quiescence = 10 * time.Second // P07

	FinalReadRepeats  = 3 // P08
	FinalReadInterval = 2 * time.Second
	FinalReadDeadline = 60 * time.Second

	HealthyDeadline = 300 * time.Second // P09

	MemoryBudgetGiB   = 6 // P10
	MemoryBudgetBytes = MemoryBudgetGiB << 30

	PorcupineTimeout = 60 * time.Second // P11

	CycleSearchBudget = 10 * time.Second // P12

	Replays           = 5 // P13
	ReproducedAtLeast = 3

	DirectedRuns    = 5 // P14
	DetectedAtLeast = 4

	RepsV1V3   = 10 // P15
	RepsV2V4V5 = 5

	PauseShort = 3 * time.Second // P16
	PauseLong  = 60 * time.Second

	SlowfsDelay           = 200 * time.Millisecond // P17
	SlowfsConfirmMedian   = 150 * time.Millisecond
	SlowfsBaselineMedian  = 50 * time.Millisecond
	NetemDelay            = 50 * time.Millisecond // P18
	NetemConfirmIncrease  = 40 * time.Millisecond
	C7StatementTimeout    = 2000 * time.Millisecond // P19
	AppendActiveKeys      = 8                       // P20
	AppendKeyRetireAt     = 64
	RegisterKeys          = 5 // P21
	BankAccounts          = 5 // P22
	BankInitialBalance    = 100
	MetricsScrapeInterval = 1 * time.Second  // P23
	ObservationInterval   = 1 * time.Second  // P24
	EvidenceBudgetMB      = 250              // P25 (ADR-0034)
	DockerSubnet          = "10.77.0.0/24"   // P26
	FaultConfirmDeadline  = 15 * time.Second // P27
	HealConfirmDeadline   = 30 * time.Second // P28
)

// Param is one row of CONTRACT §5.7.
type Param struct {
	ID    string
	Name  string
	Value string
	// Derived are fragments rendered from the typed constants above; each must
	// occur in Value, so a constant cannot change without the contract text.
	Derived []string
}

func s(d time.Duration) string  { return fmt.Sprintf("%d s", int64(d/time.Second)) }
func ms(d time.Duration) string { return fmt.Sprintf("%d ms", int64(d/time.Millisecond)) }

// Table returns the §5.7 rows, in order. A fresh slice on every call: no
// package-level mutable state.
func Table() []Param {
	return []Param{
		{"P01", "Client operation timeout", "10 s",
			[]string{s(ClientOpTimeout)}},
		{"P02", "Clients per run", "10: list-append 4, register 2, bank 2, ledger 2",
			[]string{fmt.Sprintf("%d: list-append %d, register %d, bank %d, ledger %d",
				ClientsTotal, ClientsAppend, ClientsRegister, ClientsBank, ClientsLedger)}},
		{"P03", "Pacing per client", "one operation every 50 ms; if the previous op is still running, the next starts on completion (closed loop)",
			[]string{"every " + ms(Pacing)}},
		{"P04", "Workload duration", "90 s (campaigns), 45 s (CI smoke, K1 smoke)",
			[]string{s(CampaignDuration) + " (campaigns)", s(SmokeDuration) + " (CI smoke, K1 smoke)"}},
		{"P05", "Fault cadence", "first fault at t = 10 s; each fault lasts the plan's duration; next fault starts 10 s + seeded jitter in [0, 5) s after the previous heal is confirmed",
			[]string{"t = " + s(FirstFaultAt), s(FaultGap) + " + seeded jitter", fmt.Sprintf("[0, %d) s", int64(FaultJitterMax/time.Second))}},
		{"P06", "Recovery deadline", "120 s from final heal to a successful write through each gateway",
			[]string{s(RecoveryDeadline) + " from final heal"}},
		{"P07", "Quiescence", "10 s after recovery before final reads",
			[]string{s(Quiescence) + " after recovery"}},
		{"P08", "Final read stability", "per gateway, 3 consecutive identical final reads 2 s apart, within 60 s",
			[]string{fmt.Sprintf("%d consecutive", FinalReadRepeats), s(FinalReadInterval) + " apart", "within " + s(FinalReadDeadline)}},
		{"P09", "Cluster HEALTHY deadline", "300 s",
			[]string{s(HealthyDeadline)}},
		{"P10", "Memory budget", "peak sum of container memory usage ≤ 6 GiB",
			[]string{fmt.Sprintf("≤ %d GiB", MemoryBudgetGiB)}},
		{"P11", "Porcupine timeout per partition", "60 s → UNKNOWN",
			[]string{s(PorcupineTimeout)}},
		{"P12", "Cycle search budget per strongly connected component", "10 s → UNKNOWN",
			[]string{s(CycleSearchBudget)}},
		{"P13", "Replays per minimized failure", "5; REPRODUCED if ≥ 3",
			[]string{fmt.Sprintf("%d; REPRODUCED if ≥ %d", Replays, ReproducedAtLeast)}},
		{"P14", "Directed runs per mutant", "5; DETECTED if ≥ 4",
			[]string{fmt.Sprintf("%d; DETECTED if ≥ %d", DirectedRuns, DetectedAtLeast)}},
		{"P15", "K7 repetitions", "10 per cell of V1 and V3; 5 per cell of V2, V4, V5; mutants and H1 per P14 (5)",
			[]string{fmt.Sprintf("%d per cell of V1 and V3", RepsV1V3), fmt.Sprintf("%d per cell of V2, V4, V5", RepsV2V4V5)}},
		{"P16", "Pause durations", "short 3 s; long 60 s",
			[]string{"short " + s(PauseShort), "long " + s(PauseLong)}},
		{"P17", "slowfs injected latency", "200 ms per `write`/`fsync`; confirmed if median probe ≥ 150 ms and baseline median < 50 ms",
			[]string{ms(SlowfsDelay) + " per", "≥ " + ms(SlowfsConfirmMedian), "< " + ms(SlowfsBaselineMedian)}},
		{"P18", "netem delay", "50 ms one-way on egress of the target netns; confirmed if measured TCP connect RTT increase ≥ 40 ms",
			[]string{ms(NetemDelay) + " one-way", "≥ " + ms(NetemConfirmIncrease)}},
		{"P19", "C7 session `statement_timeout`", "2000 ms",
			[]string{ms(C7StatementTimeout)}},
		{"P20", "List-append keys", "8 active keys; a key retires at 64 elements and a fresh key replaces it",
			[]string{fmt.Sprintf("%d active keys", AppendActiveKeys), fmt.Sprintf("retires at %d elements", AppendKeyRetireAt)}},
		{"P21", "Register keys", "5 keys; written values unique per run",
			[]string{fmt.Sprintf("%d keys", RegisterKeys)}},
		{"P22", "Bank", "5 accounts × 100",
			[]string{fmt.Sprintf("%d accounts × %d", BankAccounts, BankInitialBalance)}},
		{"P23", "Metrics scrape interval", "1 s (`/metrics` on HTTP ports 15000, 16000, 17000)",
			[]string{s(MetricsScrapeInterval) + " ("}},
		{"P24", "Observation interval", "1 s",
			[]string{s(ObservationInterval)}},
		{"P25", "Evidence size budget in git", "≤ 250 MB total under `evidence/`",
			[]string{fmt.Sprintf("≤ %d MB", EvidenceBudgetMB)}},
		{"P26", "Docker subnet", "10.77.0.0/24",
			[]string{DockerSubnet}},
		{"P27", "Fault confirmation deadline", "15 s after the inject command returns",
			[]string{s(FaultConfirmDeadline) + " after"}},
		{"P28", "Heal confirmation deadline", "30 s",
			[]string{s(HealConfirmDeadline)}},
	}
}

// Source pins of CONTRACT §5.2 and §14 (checked against the contract text by
// params_test.go).
const (
	MultigresRepository = "https://github.com/multigres/multigres.git"
	MultigresCommit     = "e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73"
	MultigresCommitDate = "2026-10-09T14:17:55+02:00"
	// H1Commit is the historical control source (§5.5 profile h1, §9.4,
	// §14).
	H1Commit = "4bca1d5770a6bd425bde5a61eaad240431242cd5"
)
