**Acknowledged writes lost (G1) in this finding's original run: no.**

# multipooler: steady-role operations refused while the leader's PostgreSQL restarts inside a connection storm (G12 literal rule)

| Dimension | Value (contract 1.9.0; as recorded in K5, contract 1.0.9) |
|---|---|
| Finding | FND-C11-17a344c1 |
| Guarantee | **G12** — per-user pools share fairly: under a connection storm by one role, another role with steady demand still obtains connections and completes operations; overload alone does not trigger a failover (§3) |
| Class | PROTOCOL_DEVIATION |
| Claim level | DESIGN (S40, S22) |
| Severity (§11.2, derived) | medium |
| Origin | MULTIGRES (the refusals come from Multigres' gateway and pooler while PostgreSQL restarts) |
| Reproducibility | INTERMITTENT (2 of 5 recorded replays) |
| Known status | NEW |
| Rule | G12's **literal** rule of contract ≤ 1.1.x, kept for this finding by the owner (decision D-B, LIM-25): every `cp_steady` operation of a storm window counts, also those inside a fault window |
| Both fractions, storm window 1 | **literal: 766 of 886 ok (0.865 < 0.9)**; **outside fault windows: 744 of 744 ok** (record `c11-window-check` in `evidence/K5/commands.jsonl`) |

## Summary

In C11 (overload) seed 2 on the real build, the plan's one fault — `kill -9` of the leader's PostgreSQL (F2a) — fell inside storm phase 1, while role `cp_storm` held 200 connections against a global pool capacity of 50. Every one of the 120 `cp_steady` operations that did not complete `ok` was invoked inside the F2a window, while PostgreSQL restarted. G12's literal rule counts them, so storm window 1 has 766 of 886 `ok` (0.865 < 0.9): `fair-share-starvation`. Outside the fault window all 744 `cp_steady` operations of the storm completed `ok`. The fault-free storm phase and the fault-free runs (seeds 1 and 3) starved nothing. **What the run shows is the injected failover's unavailability counted by the literal rule, not unfair pooling.** From contract 1.2.0 (ADR-0037) a run is judged on the operations outside fault windows. Under that rule this run's storm windows would show no starvation. The recorded verdict and finding are kept as the owner decided.

## Version and environment

- Multigres `e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73` built from source with upstream's Dockerfiles (only `FROM` lines changed, §5.2), PostgreSQL 17.7 (`postgres:17.7@sha256:2006493727bd5277eece187319af1ef82b4cf82cf4fc1ed00da0775b646ac2a4`), durability policy `AT_LEAST_2`, cohort {n1, n2, n3}, three cells with one multiorch and one multigateway each (CONTRACT §5.1).
- Profile `baseline` (gateway buffering off, `--enable-slot-based-replication=true` as in upstream's Kubernetes demo); no metrics exporter in the original run (contract 1.0.9; LIM-41).
- Host: Arch Linux, kernel 7.2.9-arch1-1, Docker 29.9.0, one bridge network 10.77.0.0/24, all containers on one host (LIM-02, LIM-03).
- Original run: CommitProof `2b968c5` (clean), contract 1.0.9, started 2026-10-13T23:27:50Z; minimized run `20261013T232750Z-C11-2-real-1f5423ed` (CommitProof `2b968c5`) and its five recorded replays (K5).
- Client: pgx v5.11.0; P01 deadline 10 s per operation (§6.2).

## Reproduction

From a clone of this repository at tag `v1.0.0` (Linux host meeting `docs/PREREQUISITES.md`):

```sh
make prereq-check                       # 26 checks; exit 0 required
make build
bin/commitproof images build            # pinned images; writes images.lock.json
bin/commitproof cluster up --profile baseline && bin/commitproof cluster verify && bin/commitproof cluster down   # optional: HEALTHY check
mkdir -p runs/k7-pkg/FND-C11-17a344c1
tar --zstd -C runs/k7-pkg/FND-C11-17a344c1 -xf evidence/K5/runs/20261013T232750Z-C11-2-real-1f5423ed.tar.zst
bin/commitproof replay runs/k7-pkg/FND-C11-17a344c1/20261013T232750Z-C11-2-real-1f5423ed --times 5 --out runs/k7-pkg/FND-C11-17a344c1 --cpcheck bin/cpcheck
```

Each replay brings the cluster up, runs the minimized plan (`plan.json` here; sha256 of its canonical bytes `1f5423ed108353240aa692294d9682f640f1d3bcb0e54b748d69808a97eba0c9`), tears the cluster down, seals and checks the run; the last line is a JSON summary with the replays and how many show the class the driver targets. `tar --zstd -xf evidence/K5/runs/20261013T232750Z-C11-2-real-1f5423ed.tar.zst` and `bin/cpcheck check <run-dir>` reproduce the original run's `checks.json` here byte for byte.

The plan carries contract 1.0.9, so cpcheck judges its replays by the literal rule, as the recorded run.

## Expected vs actual

- **Expected** (G12, DESIGN; §10.9 literal rule): in every storm window the fraction of `ok` `cp_steady` operations is at least 0.9, and no leader change happens without a nemesis fault.
- **Actual**: storm window 1 (5.4–27.9 s): 766 of 886 `cp_steady` operations `ok` (0.865). Of the 120 others, 114 were refused at connect with `57P03` (PostgreSQL starting up), 4 with `28P01` (password authentication failed, for a valid role, while PostgreSQL restarted), and 2 ended `info` with `57P01`. All 120 were invoked inside the F2a window (10.55–13.63 s; fault 10.50 s inject, 14.21 s healed). Storm window 2 (no fault): 872 of 872 `ok`. No leader change without a fault (no `overload-failover`).

## Analysis

**Design says** (S40 "per-user pools that share fairly", S22): a storm by one role does not starve another role's steady demand, and overload alone does not cause a failover.

**System did** (K5 triage `evidence/K5/triage/20261013T232750Z-C11-2-real-1f5423ed.json`, `report-excerpt.md`): the only server change `cp_steady` saw is the restart after F2a (its inserts return `pg_postmaster_start_time()`); latency inside storms p50/p99 4.3–4.6/7.1–8.1 ms against 4.3–4.6/7.0–7.6 ms outside, in the fault-free runs; 1 824 of 1 824 `cp_steady` operations `ok` inside the storms of seeds 1 and 3. The `28P01` refusals of a valid role during the restart are an observation (evidence/K5/NOTES.md §9).

**Harness review** (§6.2): all 120 non-`ok` completions reviewed. 118 were refused at connection startup, with no statement sent: `fail`, row 10. Two got `57P01` after sending: `info`. Every `ok` is an acknowledged insert. The fault was confirmed and healed, and the run is valid. C8 applied §10.9 as written for the plan's contract version. Not a HARNESS_DEFECT.

**Minimization and replays** (§12.2, K5): the empty schedule PASSes (the storm alone does not starve `cp_steady`), the halved 45 s plan PASSes, the client-reduced plan PASSes; the original plan is the minimum. Its five recorded replays FAIL with `fair-share-starvation` in 2 (INTERMITTENT): whether the outage's failures exceed 10 % of the storm window depends on how long PostgreSQL takes to restart.

<!-- K7-RECURRENCES -->

<!-- K7-REPLAY -->

## Suggested direction (optional; not verified)

None for Multigres from this run: refusing connections while the leader's PostgreSQL restarts is expected unavailability during a failover (S48: promotion "within seconds"). The `28P01` answers for a valid role during the restart may deserve a look (a client would read them as a wrong password); that is an observation, not this finding.

## Limitations

- The finding exists only under the literal rule; with ADR-0037's rule (from contract 1.2.0, used by K7's cell V5) this run would show no starvation outside fault windows (LIM-25).
- One storm role against one steady role, one fault inside one storm (LIM-24); one pinned commit, one host (LIM-01, LIM-02); no metrics exporter in this run (LIM-41).
- The recorded classification comes from K5's runs; K7 re-executed the replay command once with a later build that runs the metrics exporter (section above).
