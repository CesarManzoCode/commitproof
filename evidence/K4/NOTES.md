# K4 NOTES — Consensus and failover campaigns, minimization, mutants

Contract **1.0.6** (1.0.4 at sprint start; ADR-0024, ADR-0025 → 1.0.5 before
any K4 run; ADR-0026 → 1.0.6 after a development run showed a contract
defect). ADR-0028 amends ADR-0024's C5 shape (no contract change); ADR-0027
is a **proposal awaiting the owner** and is not applied. Evidence produced by
`scripts/k4-evidence.sh` (phases setup, campaign, rebuild, campaign, oracle,
ci, store, gate) and `scripts/k4-analysis.sh`, every step recorded in
`commands.jsonl` (argv, exit code, stdout/stderr sha256), 2026-10-11 21:57 –
2026-10-12 UTC, one cluster at a time, clean trees (`commitproof_dirty:
false` in every run manifest; cpgate checks it). No `claims.yaml` exists yet
(K7): every statement names its record, run id or file; "design says" cites
RESEARCH ids, "system did" cites runs (under `evidence/K4/runs/`, zstd tars
of sealed runs or stubs, §13) and the diagnosis records `k3obs` / `k4obs`
(observations, ADR-0016: never a verdict).

**Status: BLOCKED on K4.3** — mutant M2 is not detected by its directed
schedule D2 (0/5). The cause is in D2's design, not in the checkers or the
mutant (§5 below); changing a directed schedule needs the owner (§9.1, §18).
Everything else of the sprint is done.

## 1. Gate

`bin/cpgate K4 --evidence evidence/K4 --negative-controls` → **exit 1**
(record `cpgate-k4`, 03:54:20–03:59:21 UTC, stdout sha256 `71408c91…`).
`gate.json` (sha256 `a05048c3…`): **K4.1, K4.2, K4.4, K4.5, K4.6, K4.7, K4.8
PASS; K4.3 FAIL** with one problem, `M2 via D2: 0/5 runs FAIL with
lost-write, need 4` (M1 via D1 DETECTED 5/5, M4 via P1 DETECTED 5/5 with the
real build passing P1 5/5).

`negative-controls.json` (sha256 `305a6c26…`): every sabotage failed its own
criterion — K4.1: one row's upstream column flipped (recorded table hash kept
consistent) → "row [4 2 2] … mismatches recomputed 1"; K4.2: a C1 verdict
flipped in results.json; K4.3: M1's D1 checks.json for seeds 1–2 rewritten as
PASS; K4.4: multiorch's hash changed in the M1 lock entry; K4.5: the last
replay of M1 seed 1's minimized run removed; K4.6: h1.json's outcome
replaced; K4.7: one triage record removed; K4.8: the latest CI record's exit
code set to 1. Because K4.3 fails without sabotage, every control also lists
K4.3 among the failed criteria, so `failed_only_target` is true only for
K4.3's own control; no control failed any other criterion. The gate cannot
pass while K4.3 fails (status BLOCKED, §0).

Precondition: record `k4-precondition` (exit 0) ran `cpgate K1`, `K2`, `K3`
on copies of the closed evidence: all criteria PASS.

## 2. What K4 added (design says / system did)

- **Schedules** (§15 K4, §9.2, §9.4; ADR-0024 committed as `d1a66e9` before
  any K4 run). Plans gained element holds, explicit heal sequences, a
  new-leader anchor (C6) and deferred confirmation (ADR-0026); K1–K3 plans
  stay byte-identical (K3's `k3-f1b`, `k3-f5`, `nsmoke-f2a` regenerate to
  `9ac4fcb0…`, `021a2161…`, `834e32e5…`). Roles `random-node`,
  `base-follower:<k>`, `same:<j>`, and an F1b set `followers`.
- **Campaign driver** (`commitproof campaign|minimize|replay`,
  `internal/campaign`). Resumable: every run it needs is first looked up
  among the sealed runs with the same plan bytes, profile, build and
  provenance; INVALID runs are re-run up to three attempts (§12.1). Verdicts
  come from `cpcheck check` run as a subprocess; the driver computes none.
  Minimization per §12.2 and ADR-0024 (empty schedule first, ddmin over
  steps, halving while ≥ 10 s, clients in the anomaly + 1); replays per P13.
- **Mutants** (`mutants/M1.patch`, `M2.patch`, `M4.patch`, one function or
  declaration each at the §9.2 anchors, header with intent, failure mode,
  sources). `images build --mutant M<n>` builds `commitproof/multigres:M<n>`
  from a git worktree of the pinned commit with exactly the patch applied and
  fails unless every binary without a patched package equals the real build.
- **Probes** (G7). `commitproof probe --suite claims` is a sealed run: one
  probe client per gateway issuing P1, P1-show, P2; cpcheck's new checker P
  (§10.12) judges them.
- **quorumxcheck** (§10.8): a separate binary, the only importer of a
  Multigres package besides the observation code (`TestMultigresImporters`).
- **Profile h1** (§5.5, §9.4; ADR-0025): the historical commit's own
  Dockerfiles (FROM lines only), its single-cell demo topology
  (`deploy/compose-h1.yaml`, `deploy/DERIVATION-h1.md`), a HEALTHY predicate
  and leader view read from PostgreSQL.
- **cpgate K4** with eight negative controls; **`make ci-mutant-smoke`** in
  the Makefile and the workflow.

## 3. Quorum table (K4.1)

Record `quorumxcheck` (exit 0); table `quorum-table.json` (sha256
`fad7b8d8…`). For n ∈ [1,9], AT_LEAST_N with N ∈ [1,n], r ∈ [0,n]: 330
rows, **0 mismatches** between sufficient = (a) term uniqueness ∧ (b) no
rogue quorum and upstream `consensus.CheckSufficientRecruitment` at
`e3cbdbb`; (b) ⇔ (c) (FlexPaxos intersection r + N > n) holds in every row.
The minimal sufficient r equals max(⌊n/2⌋+1, n−N+1) everywhere (S07).

The majority term ⌊n/2⌋+1 and the revocation term n−N+1 **coincide** for
(n, N) = (1,1), (2,1), (3,2), (4,2), (5,3), (6,3), (7,4), (8,4), (9,5). For
`AT_LEAST_2` they coincide exactly for cohorts of 3 and 4: inference I3 is
now a measured fact, and §9.3's rejection of "drop one recruitment term" as
equivalent for the cohorts CommitProof can run holds.

## 4. Campaigns C1–C7 (K4.2)

Record `campaign-k4-1` (exit 2: incomplete) then, after two harness fixes
(§9), `build-2`, `images-build-2`, `images-verify-2`, `campaign-k4-2` (exit
1: matrix complete, FAIL present). 56 cells, each with a valid run: 42 PASS,
14 FAIL. Per run: verdict and classes from checks.json; exposure from
`k4obs` (ok writes = ok ledger inserts + appended elements of ok
transactions + ok register writes/CAS + ok transfers).

| Campaign (shape, ADR-0024) | Seed: run (`…` = `evidence/K4/runs/`) | Verdict | ok writes | Confirmed faults |
|---|---|---|---|---|
| C1 kill | 1: `20261011T225737Z-C1-1-real-c0ce92da` | PASS | 10 730 | F2d ×4 |
| | 2: `20261011T225954Z-C1-2-real-b139aac2` | PASS | 9 352 | F2a ×2, F2b, F2c |
| | 3: `20261011T230211Z-C1-3-real-ed8b194d` | PASS | 8 812 | F2b ×2, F2c ×2 |
| C2 partition | 1: `20261011T230429Z-C2-1-real-9811d19d` | PASS | 6 047 | F1b ×2, F1d |
| | 2: `20261011T230739Z-C2-2-real-9f3f45cb` | PASS | 7 182 | F1a ×3 |
| | 3: `20261011T231036Z-C2-3-real-f4505b54` | **FAIL** C3 incompatible-order, C4 non-linearizable | 3 174 | F1c ×3 |
| C3 pause | 1: `20261011T231328Z-C3-1-real-44a99ac9` (serializable) | PASS | 11 015 | F3a, F3b ×2 |
| | 2: `20261011T231604Z-C3-2-real-52940370` (repeatable-read) | PASS | 9 504 | F3a ×2 (one 60 s), F3b |
| | 3: `20261011T232034Z-C3-3-real-9608c3b8` (serializable) | PASS | 6 581 | F3a ×2, F3b |
| C4 membership | 1: `20261011T232310Z-C4-1-real-4d9e8fd0` (F5a; F5b) | PASS | 9 025 | F5a, F5b |
| | 2: `20261012T031145Z-C4-2-real-1b6468fb` (F5a→F2a; F1a→F5b) | PASS | 8 483 | F5a, F2a, F1a, F5b |
| | 3: `20261011T233126Z-C4-3-real-5c45606a` (D2 shape; F5b) | PASS | 5 996 | F5a, F3a ×2, F2a, F5b |
| C5 slow disk | 1: `20261012T031403Z-C5-1-real-6a391306` | PASS | 6 464 | F4a ×3, F2a ×3 |
| | 2: `20261012T031623Z-C5-2-real-25a6e5fe` | PASS | 5 176 | F4a ×3, F2a ×3 |
| | 3: `20261012T031844Z-C5-3-real-3cbb1b04` | PASS | 4 652 | F4a ×3, F2a ×3 |
| C6 rewind | 1: `20261011T235852Z-C6-1-real-ade478c0` | PASS | 7 682 | F2a ×4 |
| | 2: `20261012T000109Z-C6-2-real-585e78f0` | PASS | 7 265 | F2a ×4 |
| | 3: `20261012T000328Z-C6-3-real-e49b6d52` | PASS | 8 183 | F2a ×4 |
| C7 commit-cancel | 1: `20261012T000546Z-C7-1-real-f98a0215` | PASS | 2 214 | F1b ×2, F2a ×2 |
| | 2: `20261012T000816Z-C7-2-real-afcdcb27` | PASS | 2 275 | F1b ×2, F2a ×2 |
| | 3: `20261012T001043Z-C7-3-real-b11856f8` | PASS | 1 389 | F1b ×2, F2a ×2 |

INVALID attempts, kept and counted nowhere (§12.1): nine C5 attempts under
ADR-0024's original heal order and three C4 seed-2 attempts (harness defects,
§9); one C6 seed-1 attempt `20261011T235642Z-C6-1-real-ade478c0`, INVALID
because after the third leader kill PostgreSQL on n2 neither restarted by
itself within P28 nor started through pgctld within another P28
(`PostgreSQL failed to become ready: … postmaster.pid: no such file`): a
system behaviour recorded as an observation, not a harness defect; the
re-run was valid.

What PASS means here: in 3 valid runs per campaign (≤ 90 s of workload,
2–4 faults each) no checker found an anomaly. These are not §12.3
statements (K7 runs the matrix with 10 per cell and states the bounds).

## 5. Mutants side by side with the real build (K4.3)

| Mutant | Directed schedule | Mutant runs (seeds 1–5) | Real build, same plans |
|---|---|---|---|
| M1 sync-standby-cleared | D1: F1c(leader) at 10 s for 30 s | **5/5 FAIL** C1 lost-write (also C3 incompatible-order); `…T215900Z-D1-1-M1-59a95336` … `…T220810Z-D1-5-M1-3319a30c` → **DETECTED** | 3/5 FAIL C3 incompatible-order (seeds 1, 3, 4; §7), 2/5 PASS; no lost-write |
| M2 cohort-add-local-view | D2 (ADR-0024) | **0/5**: all PASS (`…T222206Z-D2-1-M2-56b84e41` … `…T223310Z-D2-5-M2-7b36cafd`) → **UNDETECTED** | 5/5 PASS |
| M4 guard-removed | probe P1 | **5/5 FAIL** P p1-not-rejected (all three gateways accept `SET synchronous_commit = off`) → **DETECTED** | 5/5 PASS (P1 rejected with 0A000, P2 rejected) |

**Why M2 is not detected (investigation, as §2 of the prompt asks).**
Design says (§9.2): under M2 "writes acked only by n4 are lost when failover
recruits from the old cohort". System did: in every M2 run M2's patch took
effect — after F5a the leader's `synchronous_standby_names` named n4 while
the recorded rule kept the cohort {n1,n2,n3}, and with both original
followers paused 1 751–4 011 operations per run completed ok, their writes acknowledged
with n4 as the only reachable standby (history). Then, 16.6–19.1 s after the leader's PostgreSQL and
pooler were killed, a pooler reported **n4 as leader of rule 2.0 whose
recorded cohort is {n1, n2, n3}** (observations; all five runs). n4 held
every acknowledged write, so none was lost. Code at the pinned commit:
`AppointLeader` recruits every pooler of the shard, not the recorded cohort
(`go/services/multiorch/recovery/actions/appoint_leader.go`: `cohort[i] =
p.Health()` over `shard.Poolers`; `consensus/coordinator.go`: "It operates on
a cohort of nodes (all nodes in the shard)"); only the quorum check is
restricted to the recorded cohort, while the leader is chosen among all
recruited poolers at the most advanced position
(`go/common/consensus/proposals.go` `buildProposalCore`,
`discoverMostAdvancedRulePositions`, `narrowByLSN`). Classification of the
cause: not a checker blind spot (nothing acknowledged was lost: C1 valid,
final reads complete), not equivalence (M2 *is* detectable when n4 is
unavailable at failover: the old cohort's 2 of 3 then suffice and a follower
without the n4-only writes is elected), but **D2 not reaching the defect**:
its design assumes a recruitment set multiorch does not use. ADR-0027
proposes D2′ (also kill n4's pooler with the leader); per §9.1 and the
prompt it is not applied, and K4.3 fails.

The real build on D2 is also informative: n4 or a base follower became
leader 13–17 s after the kill, with cohort {n1,n2,n3,n4}; no anomaly.

## 6. Historical control H1 (K4.6)

Records `images-build-h1` (the historical commit's images from its own
Dockerfiles, FROM-only, builder `golang:1.25-alpine3.23@sha256:27b3069b…`,
ADR-0025), `h1-cluster-up` (HEALTHY after 19.3 s under the h1 predicate:
n1 out of recovery, `ANY 1 ("zone1_n2", "zone1_n3")`, both streaming,
writes through gw-z1..3), `h1-cluster-verify`, `h1-cluster-down`. DH1 (F3a
on the leader's PostgreSQL for 120 s, workload 90 s): five valid runs
`20261012T001313Z-DH1-1-real-d643274e` … `20261012T002441Z-DH1-5-real-a8683424`,
**all PASS → H1 = NOT_DETECTED** (`h1.json`). System did: in no run was a new
primary observed while the primary was frozen (clients waited the whole
pause, longest gap ≈ 134 s, `k3obs`), which matches #778's "SIGSTOP-frozen
primary not detected as failed" (S51); the thaw lost no acknowledged write
in five runs. Not demonstrated: S51's data loss, which its report obtained
on a three-zone local-provisioner setup; h1 is the commit's single-cell
Kubernetes demo (§5.5), one orchestrator.

## 7. Real-build failures and their triage (K4.5, K4.7)

Four real-build runs FAIL; each has a triage record `triage/<run>.json`
(checker output, oracle cross-check, §6.2 review of every reviewed
completion, §11 classification, finding id per §11.3):

| Run | Minimized run (reductions) | Replays with the class | Reproducibility | Oracle | Finding |
|---|---|---|---|---|---|
| D1 seed 1 `20261011T221027Z-D1-1-real-59a95336` | `20261012T014955Z-D1-1-real-6625432a` (45 s, clients 0–2) | 4/5 | REPRODUCED | confirmed (recorded elle-cli validity pass) | `FND-D1-fb8d0f8c` |
| D1 seed 3 `20261011T221506Z-D1-3-real-da159fa0` | `20261012T020127Z-D1-3-real-fb2e3778` (45 s) | 4/5 | REPRODUCED | oracle-disagreement (elle adds G-single) | `FND-D1-2e860fd2` |
| D1 seed 4 `20261011T221728Z-D1-4-real-ad3fca18` | `20261012T021947Z-D1-4-real-eb537f55` (45 s, clients 0–3) | 5/5 | REPRODUCED | oracle-disagreement (elle-cli out of memory) | `FND-D1-a8eb9857` |
| C2 seed 3 `20261011T231036Z-C2-3-real-f4505b54` | `20261012T025629Z-C2-3-real-3fb018bf` (one step F1c 30 s, 45 s) | 2/5 | INTERMITTENT | confirmed (recorded elle-cli validity pass) | `FND-C2-2a534585` |

Classification (all four): SAFETY_VIOLATION · G2 · claim level EXPLICIT ·
origin UNDETERMINED · known status NEW · severity critical (derived, §11.2).
Nothing stronger is claimed: **no acknowledged write was lost** (C1 valid in
all four); what G2's observable catches is **a read that observed a write
that later disappeared**.

Mechanism (design says / system did; D1 seed 1 as the example, the others
alike). F1c isolates leader n1 from its followers, orchestrators and etcd
but not the gateways (10.7–42.2 s). COMMITs wait for a synchronous standby.
Txn op 2150 (appends 16←1) blocks at COMMIT; at 20.22 s its client gives up
(P01; completion `info`, §6.2 row 9). At that instant two serializable
transactions blocked on key 16's row lock fail with 40001, and op 2220 reads
key 16 = `[1]` (`ok`). Multigres' pooler cancels the backend statement when
the request context ends (`go/services/multipooler/internal/pools/regular/regular_conn.go`
`execWithContextCancel` → `handleContextCancellation`), and PostgreSQL ends a
cancelled synchronous-replication wait by completing the commit locally
(S61). ~25 s into the isolation a follower becomes leader (`k3obs`); the old
leader is rewound and value 1 is gone (op 2299 reads `[7 5]`; final read
`[7 5 6 …]`). Every vanished value in the four runs was written by a
transaction whose completion was `info` (client timeout), and appeared in an
`ok` read on the isolated leader. The C2 run also shows C4 non-linearizable
on registers 1 and 4 by the same mechanism (register 4 read 242 at 30.9 s,
227 at 41.0 s).

Origin hypothesis UNDETERMINED: the local visibility of a commit whose
sync-rep wait was cancelled is documented PostgreSQL behaviour (S61), the
cancel is issued by Multigres' pooler, the loss follows a Multigres failover;
no reproduction on plain PostgreSQL streaming replication was run (R15).
Known status NEW: no match among the alpha-known-gap issues named in S50
(#1059, #1060); #1065 and #1126–#1128 are not described in the register, so a
match there is not excluded; nothing was looked up online (AGENTS.md §6).

Oracle (§10.11). cpcheck's oracle runs three elle-cli passes; on three of the
four histories (~18 000 operations) elle-cli exits 255 with
`java.lang.OutOfMemoryError` in its process-order cycle search (records
`oracle-…`, `elle-process-…`; also with `-Xmx9g`). The validity pass — the
one that decides validity and classes — completes on D1 seed 1 and C2 seed 3
and confirms `incompatible-order` (records `elle-validity-…`). On D1 seed 4 it
also runs out of memory (default heap and `-Xmx9g`, record
`elle-validity-xmx9g-…`): not confirmed → `oracle-disagreement`. On D1 seed 3
cpcheck's three passes completed; elle-cli agrees on validity and on
`incompatible-order` and additionally reports a G-single cycle whose
anti-dependency edge starts at the read that observed the vanished value
(t = 20.75 s). Adya's cycle classes presuppose one version order per key
(S72); these reads admit none, and §10.3 defines C3's order as the longest
read with non-prefix reads reported as `incompatible-order`. Labelled
`oracle-disagreement` with that triage; checker logic unchanged (K4 forbids
checker changes without a K2 corpus case; this is not a C3 defect by §10.3).

Mutant failures (M1 ×5, M4 ×5) are minimized and replayed too: every M1 run
REPRODUCED 5/5 (minimal plans: D1's F1c with the workload halved three times
to 11.25 s, and for seeds 1, 3, 5 the clients reduced to four); every M4 run
REPRODUCED 5/5 (a
probe run has nothing to reduce: no faults, a 10 s workload, all three
clients involved).

## 8. C6 and C7: the PostgreSQL mechanisms (S44, S61) and what the system did

**C6 (pg_rewind same-timeline divergence, S44).** Design says: TLA+ found that
pg_rewind treats equal timeline IDs as "no divergence", so a rejoining standby
may keep writes after two independent promotions to the same timeline. C6
kills the leader, then the new leader 3–8 s after it is observed, twice per
run. System did (`k3obs`): in the three valid runs every one of the twelve
kills was followed by a newer decided rule led by a *different* node within
1.8–7.0 s (four promotions per run, e.g. seed 1: n2@1.0 → n3@2.0 → n2@3.0 →
n3@4.0 → n2@5.0); all three PASS (no lost write, no incompatible order,
linearizable registers). One attempt was
INVALID because PostgreSQL did not come back on n2 within 2 × P28 after the
third kill (§4). Not demonstrated: that the S44 divergence pattern (two
promotions to the same timeline ID) actually occurred — CommitProof does not
read timeline IDs into verdicts.

**C7 (commit cancel, S61).** Design says: PostgreSQL acknowledges a COMMIT
whose sync-rep wait is cancelled, with WARNING `canceling wait for synchronous
replication` (S61); the gateway enforces `statement_timeout` by cancelling
backend queries (S16) and routes client CancelRequests (S17). C7 isolates
every follower (F1b, commits block) for 15 s, then kills the leader's
PostgreSQL, twice per run; W4 sessions set `statement_timeout = 2000` (P19)
and W1 clients send a CancelRequest 1 s after COMMIT. System did (`k4obs`):
**no NOTICE or WARNING reached any client in any K4 run**; every cancelled
COMMIT (95, 93 and 45 CancelRequests) and every P19 timeout (36, 36, 18)
completed with SQLSTATE 57014 (plus a few XX000), i.e. `info`, never `ok`.
Of those unknown-outcome writes, many are present after recovery (e.g. seed
2: 146 of 156 appended elements, 40 of 45 ledger rows) and the rest absent;
both are allowed for `info`. After each leader kill a newer rule was decided
within 3.4–9.3 s, sometimes re-electing the same node (seed 1: n1@1.0 →
n1@2.0) and sometimes another (seed 1: n1@2.0 → n2@3.0) (`k3obs`). No
acknowledged write was lost (C1 valid): the "acknowledged but unreplicated
commit surfaces through the gateway" question of §1 is answered no for C7's
schedule (the gateway never returned success after a cancel). D1/C2's
finding (§7) shows the other face of the same PostgreSQL mechanism: the
locally committed value is not acknowledged but is visible to other sessions
until a failover removes it.

## 9. Harness defects found and fixed in K4 (none is a Multigres finding, §11.1)

1. **Variant rebuild dropped the lock entry** (evidence attempt 1, record
   `images-build-M1` exit 2 in `runs/k4-attempt1/`, outside evidence): a
   rebuilt mutant whose entry the real build preserved lost its new entry,
   so isolation found no hashes. Fixed in `f4a5a0f`; evidence restarted from
   scratch.
2. **C4 "F5b during F1a(leader)" could only end INVALID** (development run
   `20261011T213422Z-C4-2-real-e1f5552e`, not evidence): a contract defect,
   ADR-0026 (F5b confirmed after F1a heals; contract 1.0.6).
3. **F1a's heal predicate probed a namespace F5b had stopped** (three C4
   seed-2 attempts with `f4a5a0f`, archived with their checks in
   `harness-defect-runs/`, README there): fixed in `f84bf4f` (heals probe the
   namespaces that still exist and list the vanished ones); re-run valid.
4. **C5's heal order** (nine attempts, F2a not-healed): crash recovery on a
   200 ms-per-write disk cannot finish within P28; ADR-0028 heals the slow
   disk first (`c1d1604`); re-runs valid.
5. **cpgate compared plan.json's file hash** (with its trailing newline) to
   `plan_sha256` (development dry run): fixed in `64d51d5`.
6. **A refused `cluster up` could tear down another cluster** (found by
   inspection after a development command was refused): `RunPlan` now
   refuses before creating a run directory and never runs `cluster down`
   for a run that created nothing (`f64b653`).
7. **CI attempt `ci` failed on staticcheck** (an unused type in `k4obs`):
   fixed in `14c5045`; the latest attempt is `ci-2` (K4.8 reads the latest).

Runs therefore come from two commitproof commits, each a clean tree: 173
from `f4a5a0f` (`campaign-k4-1`) and 4 from `ccda4a4` (`campaign-k4-2`: C4
seed 2 and C5 seeds 1–3); the fixes between them touch only the code paths
named above. The CI record ran on `14c5045`.

## 10. Guarantees G1–G7: demonstrated / not demonstrated

- **G1** (acked writes survive): no lost-write in any valid real-build run of
  K4 (C1–C7 ×3, D1 ×5, D2 ×5, DH1 ×5 on h1). The detector works: M1 → 5/5
  lost-write. Not demonstrated: any statistical bound (K7), membership-change
  loss under M2 (undetected, §5), power loss (LIM-03).
- **G2** (no split brain): **violated by G2's own observable** — reads
  observed writes that later disappeared, with incompatible list orders, in
  D1 (3/5 real) and C2 seed 3 (§7). No two leaders committed divergent
  *acknowledged* histories in K4 (C1 valid everywhere on the real build).
- **G3** (membership changes keep acked writes): C4 ×3 and D2 ×5 real-build
  runs PASS, including F5b during an isolated leader and the D2 shape. Not
  demonstrated: that a membership defect would be caught (M2 undetected).
- **G4** (no aborted reads): C2 g1a never fired on the real build.
- **G5** (serializable / SI): no G0–G2 cycle reported by C3 on the real build;
  C3 seed 2 ran W1 at repeatable-read. The incompatible orders of §7 are G2
  observables; elle-cli's extra G-single on D1 seed 3 is triaged in §7.
- **G6** (linearizable registers): C4 non-linearizable on two registers in C2
  seed 3, same mechanism as §7; otherwise linearizable.
- **G7** (guards): P1 rejected with 0A000 and P2 rejected on the real build
  5/5; M4 accepted P1 5/5 (P detects it).

## 11. Evidence size (P25, §13)

Primary runs and every run of a real-build failure are committed in full;
minimization candidates and replays of mutant and H1 failures are committed
as stubs (manifest, plan, seal; cpgate checks them against the seal and
checks.json), their full archives kept on this machine under `runs/k4-full/`
(LIMITATIONS: not re-checkable from git alone). Stored by record `store-runs`: 104 runs in full, 73 stubs; `evidence/K4` is
92 MB and `evidence/` 166 MB in total (P25: ≤ 200 MB), which leaves about
34 MB for K5–K7: K7's policy of summarizing uncited runs (§13) will have to
apply from K5 on.

## 12. Demonstrated / not demonstrated

Demonstrated (artifacts above): the quorum table with 0 mismatches and the
coincidence list; 56 matrix cells with valid runs, every one checked;
M1 and M4 detected (5/5) with isolation of their builds; every FAIL
minimized and replayed with its reproducibility class; H1 attempted and
classified NOT_DETECTED; four real-build failures triaged; all CI targets
including `ci-mutant-smoke`.

Not demonstrated: M2's detection (K4.3; owner decision on ADR-0027);
statistics (K7); H1's data loss; S44's same-timeline promotion pattern;
plain-PostgreSQL reproduction of §7; remote CI (LIM-16).
