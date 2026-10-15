# ADR-0036 — K6 report: the reference run, the failover metrics, the campaign statistics and the gate's cross-check, fixed before any K6 render

Status: Accepted (K6, contract 1.1.1). Clarifies CONTRACT §5.3 (cpreport's utility imports), §10.10 (the unavailability window), §12.3 (exposure and the arithmetic of the bounds) and §15 K6 (reference run, K6.3 and K6.5). Written and committed before any K6 evidence render, as ADR-0024 and ADR-0030 were before their sprints' runs: definitions that decide a cross-check must not look chosen after seeing the numbers.

## Context (gaps)

1. **Reference run.** §15 K6 asks that "the report of one real K4 failover is committed as an example". The K6 prompt adds: "with at least one fault of each of F2a and F1c". No real K4 run has both. Every real K4 archive's plan was listed (`zstd -dc <archive> | tar -xO <id>/plan.json`, fault kinds per run, development command, 2026-10-15): the real runs with F1c are D1 seeds 1–5 (one F1c each), C2 seed 3 (three F1c) and their minimization candidates and replays (F1c only); those with F2a are C1 seed 2, C4 seeds 2–3, C5 ×3 (and the nine INVALID C5 attempts), C6 ×4, C7 ×3, D2 ×5 and D2′ ×5 (no F1c). The only real runs with both are K5's C10 runs (F2a, F2b, F1c, F6), which are not K4 runs.
2. **§10.10** defines the unavailability window as "first failed/unknown op after a fault to first ok after heal" without saying whether "after" is judged by invocation or completion, whether "heal" is the heal command or its confirmation, or which operations count. K6.5 asks for latency "before/during/after" per fault and a "rule changes count" per fault; neither phase bounds, the percentile, nor the attribution of a rule change to a fault is defined. K6.3 asks for "every leader change in observations" without defining one.
3. **§12.3** names the exposure items (operations, ok writes, confirmed faults by kind, fault-time) and the bounds (Wilson 95 %, `1 − 0.05^(1/N)`) without defining ok writes or fault-time, and §15 K6 requires integer arithmetic for rendered numbers.
4. **P23.** No run of K1–K5 contains a `/metrics` scrape: the observer implements P24's pooler status observations only (`evidence/K1/NOTES.md` §4: "metric timelines are K6"). The report can only render what the sealed observations hold.
5. **§5.3** lists what cpreport may import (`history`, checker output types, `stats`, `report`); cpgate's row likewise omits the utility packages it uses (`canonjson`, `params`), and run archives (§13) need a zstd reader.

## Decision

### Reference run

**`20261011T231036Z-C2-3-real-f4505b54`** (K4 C2 seed 3; profile baseline; real build; contract 1.0.6; seal root `a5472bd9…4969`; archive `evidence/K4/runs/<id>.tar.zst`, 674 734 bytes), copied byte for byte with its `checks.json` into `evidence/K6/reference-run/`. It is a real K4 failover run, as §15 K6 requires: F1c on leader n1 (10.65–42.18 s) is followed by a new decided rule 2.0 led by n2, first observed at 36.35 s; F1c on the new leader n2 (55.71–87.22 s) by no failover — from 55.9 s to the end of the workload (96.3 s) 18 operations completed ok through the three gateways, none after 86.3 s, and the next decided rule (4.0, n2 again) was first observed at 102.35 s; a third F1c on n2 falls after the workload. It is the K4 failover run with the most faults of the kinds the prompt names, and its checker anomalies (C3 `incompatible-order` ×2, C4 `non-linearizable` ×2; FND-C2-2a534585) make K6.3's anomaly links non-vacuous. The prompt's F2a requirement cannot be met by a K4 run; the contract wins (AGENTS.md §1) and the closure records the deviation.

### Inputs, seal enforcement, output

- `cpreport render <run-dir | run-archive | campaign-dir> --out <dir>`. A run archive is `<run-id>.tar.zst` (the §13 form: a zstd tar of the sealed directory). The checker output is `<run-dir>.checks.json` (cpcheck's location) or `<run-id>.checks.json` next to the archive; it is required, and its `run_id` and `seal_root` must equal the run's.
- Fail closed: a run whose seal does not verify (`history.VerifySeal`: file hashes, Merkle root, both hash chains, well-formedness), a stub, a missing or mismatching checks file, or any run of a campaign directory doing so → exit 2 and **nothing written**: the report is rendered in memory and `--out` is created only afterwards.
- Output: a run → `<out>/<run-id>.html`; a campaign directory → `<out>/index.html`. One self-contained file: inline CSS and SVG, a canonical-JSON data block in `<script type="application/json" id="cp-data">`, no executable script, no external reference.
- Determinism: no time of rendering, host name, user, absolute or input path; only run ids and sealed content. Every map is iterated in sorted key order. Rendered numbers come from integers (ns → ms or s with fixed decimals, rounded half up); SVG coordinates are integers. The renderer's own commit is **not** printed, so an unchanged run renders to the same bytes at any later commit unless the report format changes; the page names its format (`cpreport-format 1`). The committed example is the regression reference of `make ci-report`.

### Events (K6.3)

- An operation (an invoke and its completion, §6.1) is one SVG element `id="e<invoke i>"` with `data-ev="<invoke i> <completion i>"`; every `nemesis` and `marker` event is one element `id="e<i>"` with `data-ev="<i>"`. Completeness: the union of `data-ev` over the page is {0, …, history lines − 1}, each index exactly once.
- A fault phase element carries `data-fault`, `data-kind` and `data-phase`.
- An anomaly of checks.json (checkers' `anomalies`; their `stats.observations` are rendered too but are not anomalies) is one element with `data-checker`, `data-class` and `data-ops` (its op indices in checks.json order), and one link `href="#e<j>"` per index, j being the invoke index of the operation the index belongs to (or the index itself for a nemesis or marker event).

### Observed rules and leader changes

- A decided rule is observed by a `pooler-status` observation with `ok: true` and a non-null `decision`, or a `healthy` observation with a non-null `rule`. Its identity is (`coordinator_term`, `leader_subterm`); `first_seen` is the least observation `t` reporting it; leader and cohort are those of that first observation.
- Rules are ordered by identity. A **leader change** is a rule whose leader differs from its predecessor's in that order; its time is its `first_seen`. Each rule is one element with `data-rule="<term>.<subterm>"` and `data-leader`; each leader change one element with `data-leader-change="<term>.<subterm>"` and `data-leader`.
- Other sources (`node-sql` of profile h1, `cdc-slot`, `client`) carry no rule; a run without rule observations has none.

### Per-fault failover metrics (K6.5, §10.10)

For fault f (the nemesis events sharing `fault.id`): I = its `inject` time; E = its `healed` time, else its `confirmed` time (F5a, F5b: no heal), else its last event's time. O = every operation except `f: final-read` (C9's set), gateway = the invoke's `gw`, latency = completion t − invoke t. The gateways are those of O, sorted.

1. **Unavailability window** of gateway g (§10.10, as C9 measures it): S = the least completion t ≥ I of an operation of O through g completed `fail` or `info`; R = the least completion t ≥ max(S, E) of one completed `ok`. Window R − S; `open` if S exists and R does not; `none` if there is no S. Every `fail` counts, 40001 included (§10.10 says "failed"; C9 does the same), and the window cannot end before the heal is confirmed — so for a fault the system rides through before its heal, the window spans to the heal. The next measure complements it.
2. **Longest ok gap** of gateway g (the measure of `k3obs`, used in K3–K5 NOTES): over [I − 1 s, E + 10 s], the largest difference between consecutive members of {I − 1 s} ∪ {completion t of `ok` operations of O through g in the interval} ∪ {E + 10 s}, and the number of non-`ok` completions in the interval.
3. **Latency percentiles**: the `ok` operations of O whose invoke t lies in *before* [I − 10 s, I), *during* [I, E] or *after* (E, E + 10 s] — 10 s is P05's gap between a confirmed heal and the next injection. Per phase n, and p50, p95, p99 by nearest rank (the value of rank ⌈q·n/100⌉ among the ascending latencies, q ∈ {50, 95, 99}); none when n = 0. Data block in ns; page in ms with one decimal.
4. **Rule changes** (cascade indicator, §3): the rules with `first_seen` in [I, I′), I′ being the least inject time greater than I among the run's faults (unbounded for the last), and how many of them are leader changes.

### Campaign summary (§12.3)

- The directory holds runs as subdirectories with `seal.json`, `<id>.tar.zst` archives and `<id>.stub.tar.zst` stubs (§13), each with `<id>.checks.json`; other entries are ignored. A run is verified (`VerifySeal`, a stub by `VerifyStub`) and its checks must name its id and seal root.
- Minimization candidates and replays (manifest `candidate_of`, `replay_of`; §12.2) are listed, not counted. A **cell** is (campaign, profile, mutant). N = its runs with verdict PASS, FAIL or UNKNOWN; INVALID runs are listed, never counted (§12.1). For each anomaly class c of the cell, k_c = the runs having c among the anomalies of a checker with `valid: false`; k = the FAIL runs.
- **Bounds**, integer arithmetic (`math/big`, 10⁻¹² fixed point, then three decimals): the Wilson 95 % interval (z = 1.96) of k_c/N with the lower end rounded down and the upper end rounded up; for k = 0, `1 − 0.05^(1/N)` from the greatest r with r^N ≤ 0.05 at that resolution, rounded up (N = 10 → 0.259, as §12.3 states).
- **Exposure** over the cell's valid runs held in full (otherwise "m of N runs": stubs are not re-checkable, LIM-30, LIM-37): operations = invokes except `final-read`; **ok writes** = `ok` ledger inserts + the `append` micro-ops of `ok` append operations + `ok` register `write` and `cas` + `ok` bank `transfer` (`k4obs`'s count in K4 NOTES; W5–W8 operations are counted as operations only); **confirmed faults** by kind = `confirmed` events; **fault-time** = Σ (heal t − confirmed t) over the faults with both (`k4obs`).
- The §12.3 clean statement is rendered for a cell with k = 0, no UNKNOWN run and its exposure complete; "Not tested" points to `docs/LIMITATIONS.md` (the renderer reads sealed runs only).

### Metrics from observations

Rendered per node: pooler status (PRIMARY / STANDBY / probe error), WAL position (an LSN `X/Y` as the integer X·2³² + Y) and the status probe's latency; `node-sql` (h1) recovery state and LSN; `cdc-slot` readiness; `client` error observations as ticks. `/metrics` timelines (P23) cannot be rendered for any recorded run (Context 4): the page says so, and LIMITATIONS LIM-40 records it.

### cpreport's imports and the gate's independence

cpreport links `history` (with the archive reader moved there from `gate`), `report`, `stats`, `checker/output` (the checks.json types; no checker logic, no Porcupine), and the utility packages `canonjson` and `params` that cpgate already uses; the independence test holds it to that list. cpgate's K6.3 and K6.5 recompute the elements' expected sets and every per-fault metric from the extracted reference run with `gate` code written from this ADR; they import neither `report` nor `stats`, and compare integers exactly.

### Sabotage

`cp_sabotage_report_nondeterminism` (in `internal/report`, a sabotage directory for the independence test) emits the client lanes and the per-gateway rows in map iteration order. K6.1's negative control replaces the three render records by the three renders of the sabotaged build; K6.1 also requires those three to differ, so a sabotage that failed to show would fail the gate.

## Alternatives rejected

- **A K5 C10 run as the reference** (F2a, F2b, F1c, F6): meets the prompt's fault kinds, not §15 K6's "real K4 failover".
- **A new run combining F2a and F1c**: a schedule no campaign defines, run for the report; §15 K6 asks for an existing K4 run.
- **Two reference runs**: K6.1–K6.5 name one; evidence doubled for no criterion.
- **Excluding 40001 from the window, or ending it at the first `ok` after S**: departs from §10.10's words and from C9; under F1c an isolated leader still serves reads while commits block, so "first ok" would end the window while writes are unavailable. The longest ok gap gives the system's view instead.
- **Interpolated percentiles**: floating point; nearest rank is exact.
- **Scraping `/metrics` in K6**: a runner change that no recorded run would show, with an unknown cost under P25; left to the owner (LIM-40).

## Consequences

- Contract 1.1.1 (PATCH, clarification): §5.3, §10.10, §12.3 and §15 K6 point here. `params.ContractVersion` = 1.1.1; D3′'s runs keep 1.1.0 (`params.K5ContractVersion`); every other K1–K5 run keeps the version it carries.
- The C9 checker's own `unavailability_ms` and `latency_us` statistics stay as they are (K2 interfaces) and are rendered as checker output; the report's per-fault values are computed by `report` and cross-checked by `gate`.

## Sources

CONTRACT §3, §5.3, §5.4, §10.10, §12.3, §13, §15 K6; ADR-0003, ADR-0016, ADR-0017, ADR-0024, ADR-0030; `internal/tools/k3obs`, `internal/tools/k4obs`; S48.
