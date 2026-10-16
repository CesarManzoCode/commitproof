# ADR-0041 — Campaign C12, profile `slotless` and the K7 campaign command, fixed before any run of them

Status: **Accepted (K6b, contract 1.4.1).** Applies the owner's decision 3 of 2026-10-15 and fixes how the K6b runs of decisions 3, 5 and 8 are made and labelled. Clarifies CONTRACT §5.5 (`slotless`), §5.6 (`commitproof campaign`), §15 K7 (C12, `make campaign-k7`). Written and committed before C12's code, the `slotless` overlay, the K7 matrix file and any run of them, as ADR-0024 and ADR-0030 were before K4's and K5's runs: a schedule must not look chosen after seeing results (ADR-0007).

## Owner decision (verbatim)

> 3. Implement campaign C12 (CONTRACT §15 definition) and the `slotless` profile (flag only: `--enable-slot-based-replication=false` on pooler and gateway). Fix their schedules, timing, seeds and client counts in an ADR committed before any run of them, as ADR-0024/0030 did. One valid smoke run each (not evidence; label it as such) to prove they work; each smoke run checked by cpcheck.

## Context (gaps)

§15 K7 defines C12 in one line: "all workloads W1–W4, nemesis drawing uniformly from {F1a–F1d, F2a–F2e, F3a, F3b, F4a, F6, F7}, plus one F5a/F5b pair per run". It leaves open how many faults a run draws, their targets and holds, where the F5a/F5b pair sits, and the seeds. §5.5 defines `slotless` as "pooler and gateway `--enable-slot-based-replication=false`" for V2 (C1, C2, C4). §13 names `make campaign-k7`, local only; §15 K7 asks for it "resumable, background-safe". The owner's decision 5 asks for a measurement of whether ADR-0038's exporter changes anything observable, and item 8 of the task for a dry run of the K7 matrix and at most two real runs labelled as smoke tests.

## Decision

### C12 (V1: seeds 1–10, profile `baseline`)

- **Workloads and clients:** W1–W4 with P02's ten clients (W1 0–3 serializable, W2 4–5, W3 6–7 serializable, W4 8–9; client i through `gw-z(i mod 3 + 1)`), P03 pacing, P04 90 s, P01. The runner's own clients keep their numbers (§6.1).
- **Steps** (P05: the first at t = 10 s, each later one 10 s + seeded jitter in [0, 5) s after the previous step's last heal is confirmed):
  1. F5a(n4), held 5 s (C4's hold);
  2. drawn fault A;
  3. drawn fault B;
  4. F5b(n4), held 5 s.
  So n4 is a member of the cohort during both drawn faults (a membership change in the run, then faults with four members), and its graceful removal follows them. With K3/K4's timings (F5a confirmed ≈ 12 s, F5b ≈ 1 s, typical holds 0–30 s) the drawn faults fall mostly inside the 90 s workload and F5b usually after it, as K4's third C2/C3 faults did.
- **Kinds drawn uniformly, without replacement across seeds:** the 14 kinds {F1a, F1b, F1c, F1d, F2a, F2b, F2c, F2d, F2e, F3a, F3b, F4a, F6, F7} are shuffled with the PCG stream of (C12, block b = (seed − 1) div 7, stream "nemesis-kinds"); seed s draws positions 2·((s − 1) mod 7) and 2·((s − 1) mod 7) + 1 of block b's permutation. Every kind has the same probability at every draw, and every 7 consecutive seeds from 1 (1–7, 8–14, …) cover each kind exactly once: V1's seeds 1–10 expose every kind at least once and six kinds twice.
- **Targets and holds per kind** (the campaign that already exercises the kind; choices drawn from the run's nemesis stream):

| Kind | Targets | Hold | As in |
|---|---|---|---|
| F1a | `leader` or `random-follower` | 10 s or 30 s | C2 |
| F1b | A = {`leader` or `random-follower`, `random-orch`}, B = rest | 10 s or 30 s | C2 |
| F1c, F1d | `leader` | 10 s or 30 s | C2 |
| F2a | `leader` or `random-node` | 0 (the kill is the fault) | C1 |
| F2b | `leader` or `random-node` | 5 s | C1 |
| F2c | `random-orch` | 5 s | C1 |
| F2d | `random-gw` | 5 s | C1 |
| F2e | `etcd` | 5 s | K3 |
| F3a | `leader` or `random-follower` | P16 short (3 s) or long (60 s) | C3 |
| F3b | `pooler:leader`, `random-orch` or `random-gw` | P16 short or long | C3 |
| F4a | `leader` or `random-follower` | 10 s | C5's F4a, without its F2a |
| F6 | `leader` | 5 s | C8, C9 |
| F7 | `leader` or `random-follower` | 10 s | K3 |

  After F5a, `random-follower` and `random-node` include n4 (running nodes, ADR-0024).
- **Minimization target** of a C12 failure: the first class of the first invalid checker (ADR-0024's default).

### Profile `slotless` (V2: C1, C2, C4, seeds 1–5)

- `deploy/compose-slotless.yaml`, an override of `deploy/compose.yaml` like `buffered`'s: it restates the command of `pooler-n1`…`pooler-n4` and `gw-z1`…`gw-z3` with `--enable-slot-based-replication=true` replaced by `=false` and nothing else changed. `deploy/DERIVATION.md` records it.
- Plans on `slotless` differ from the same campaign's `baseline` plans only in their `profile` field: V2's C1, C2 and C4 seeds 1–5 run the schedules and operations of V1's seeds 1–5, so the two profiles are compared run for run.
- Real build only (no mutant runs on `slotless`, as before).

### K7's campaign command

- `commitproof campaign --matrix <file> --out <dir> [--cpcheck <bin>] [--dry-run]`: `--dry-run` starts nothing; it prints one JSON line per cell (cell, campaign, profile, build, seed, plan sha256, the plan's fault kinds, and whether a valid sealed run of that plan is already in `--out`) and a summary with the cell count and the number of runs still to make. A matrix entry may carry a `cell` label (V1…), copied into the driver's events.
- `make campaign-k7` (`scripts/campaign-k7.sh`): `commitproof campaign --matrix campaigns/k7.json --out runs/k7 --cpcheck bin/cpcheck` detached from the terminal (`setsid nohup`), appending to `runs/k7/campaign-k7.log`, with a pid file; it refuses to start while another campaign holds the output directory (the driver's lock) or while `CP_OTEL_METRICS_EXPORTER` is set (below). Re-invoking it resumes: sealed runs are reused, never recomputed (§5.6). `make campaign-k7-status` prints the progress from the log; `make campaign-k7 DRY_RUN=1` runs the dry run. `K7_MATRIX` and `K7_OUT` override the matrix and output directory (the K6b smoke verification below). What K7 commits from `runs/k7/` is ADR-0039's storage rule.

### K6b runs: smoke and measurement, not evidence

- **Smoke (decision 3 and item 8):** `make campaign-k7 K7_MATRIX=campaigns/k6b-smoke.json K7_OUT=runs/.k6b/smoke` with two cells: C12 seed 1001 on `baseline` and C4 seed 1001 on `slotless` (F5a → F2a(leader), then F5b: membership and a failover without slot-based replication). Seeds outside every K7 cell, so no smoke run can be mistaken for a matrix run. Each run is checked by cpcheck (the driver's subprocess, then a recorded `cpcheck check`); a second invocation shows the resume. Their summaries (stub and `checks.json`) are kept under `evidence/K6b/smoke-not-evidence/` and are not K7 evidence: no K7 cell or claim may count or cite them. The running containers' commands are captured during the slotless run (the flag in force on all seven processes).
- **Exporter measurement (decision 5):** the CI smoke plan (`smoke`, seed 42: W1–W4, 45 s, no fault) run twice back to back, once as K7 runs it (exporter on) and once with `CP_OTEL_METRICS_EXPORTER=none` — the configuration of K1–K6, where the pinned code sets `none` when the variable is unset (ADR-0038) — with the containers' environment captured during each run. Compared: per-workload operation counts and outcomes, latency percentiles of `ok` operations, per-container CPU time (cgroup) and peak memory, scrape outcomes, the checkers' verdicts. Reported as a measurement of one pair (`evidence/K6b/exporter-ab.md`), not a conclusion; labelled not evidence like the smoke runs.
- `deploy/compose.yaml` reads `OTEL_METRICS_EXPORTER` from `CP_OTEL_METRICS_EXPORTER`, default `prometheus`, only for that measurement; `commitproof campaign` refuses to run while the variable is set, so no matrix run can be made without the exporter.

## Alternatives rejected

- **C12 kinds drawn with replacement.** With two draws per run and ten seeds, about 3 of the 14 kinds would be expected never to occur in V1's cell; drawing without replacement in blocks of seven keeps every draw uniform and covers every kind.
- **Three or more drawn faults per run.** With F5a first and F5b last, a third draw would start after the 90 s workload in most runs (K4: the third C2/C3 fault often did), exposing it to no client operation; the run would last ~30 s longer.
- **F5a/F5b around the drawn faults as one composite step.** Composite heals run in reverse order after the step (§8): the drawn fault would be healed only after F5b, and F5b's predicate cannot hold during a partition of the leader (ADR-0026).
- **Smoke runs on matrix seeds.** A smoke run of a matrix plan in another directory could later be mistaken for, or copied as, a matrix run.
- **The exporter measured across sprints** (K7-configuration runs against K4–K6 runs of the same plan). Code, host state and contract versions differ between those runs; a back-to-back pair isolates the exporter.

## Consequences

- Contract 1.4.0 → 1.4.1 (PATCH, clarification): §5.6 lists `--dry-run`; §15 K7 points to this ADR for C12, `slotless` and `make campaign-k7`. K1–K6 plans are unchanged.
- C12's plans are a pure function of (campaign, seed, profile, contract version) as §6.3 requires: the kind permutation depends on the seed's block, which is a function of the seed.

## Sources

CONTRACT §5.5, §5.6, §6.3, §8, §13, §15 K4/K5/K7, P02–P05, P16; ADR-0007, ADR-0024, ADR-0026, ADR-0028, ADR-0030, ADR-0038, ADR-0039; evidence/K3/NOTES.md (F5a ≈ 12 s, F5b ≈ 1 s); evidence/K4 C2/C3 runs (third fault after the workload).
