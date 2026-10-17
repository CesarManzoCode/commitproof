# CommitProof


CommitProof searches for counterexamples to the durability and consistency guarantees that Multigres states publicly — an acknowledged commit survives failovers, split brain and membership changes — and reports bounded evidence, not proofs: every result is a count of valid runs of a stated fault schedule on one pinned commit (Multigres `e3cbdbb`), with a 95 % bound and the exact exposure, and every violation it found comes with a sealed, replayable history. It is an independent, Jepsen-style verifier written in Go: a reproducible multi-container cluster, seeded fault schedules whose every fault is checked by a predicate before it counts, history checkers validated on synthetic histories and against elle-cli, and deliberately broken builds (mutants) that show the checkers catch what they claim to catch. The normative specification is [`docs/CONTRACT.md`](docs/CONTRACT.md) (version 1.9.0); every number below comes from the evidence under [`evidence/`](evidence/) and is listed with its artifact and hash in [`claims.yaml`](claims.yaml).

<!-- README-FINDINGS -->
## Findings

**Acknowledged writes lost (G1): none** — in no finding's original run, in no valid K7 run and in none of K7.4's replays (C1 valid). [CLM-0025] [CLM-0034] [CLM-0063]

Fourteen packages under [`findings/`](findings/), written in the upstream issue format and not published; ordered by severity (§11.2, derived), each with its claim level and origin hypothesis. K7.4 executed each package's replay command once more; its result is reported beside the recorded reproducibility, not instead of it.

| Finding | Guarantee | Class | Claim level | Severity | Origin | Reproducibility | Known status | K7 recurrences | K7.4 replays | What it shows |
|---|---|---|---|---|---|---|---|---|---|---|
| [FND-D1-fb8d0f8c](findings/FND-D1-fb8d0f8c/) | G2b | SAFETY_VIOLATION | INFERRED | high | POSTGRES_INHERITED | REPRODUCED (4/5) | NEW | 3 | REPRODUCED (5/5) | reads of a write never acknowledged to its client, which the next failover removes: PostgreSQL-inherited behaviour (a cancelled synchronous-replication wait commits locally, S61) exposed by Multigres' cancel-on-timeout path [CLM-0040] [CLM-0062] |
| [FND-D1-2e860fd2](findings/FND-D1-2e860fd2/) | G2b | SAFETY_VIOLATION | INFERRED | high | POSTGRES_INHERITED | REPRODUCED (4/5) | NEW | 3 | REPRODUCED (4/5) | as FND-D1-fb8d0f8c (D1 seed 3) [CLM-0041] [CLM-0062] |
| [FND-D1-a8eb9857](findings/FND-D1-a8eb9857/) | G2b | SAFETY_VIOLATION | INFERRED | high | POSTGRES_INHERITED | REPRODUCED (5/5) | NEW | 3 | REPRODUCED (3/5) | as FND-D1-fb8d0f8c (D1 seed 4) [CLM-0042] [CLM-0062] |
| [FND-C2-2a534585](findings/FND-C2-2a534585/) | G2b | SAFETY_VIOLATION | INFERRED | high | POSTGRES_INHERITED | INTERMITTENT (2/5) | NEW | 3 | REPRODUCED (4/5) | as FND-D1-fb8d0f8c, under campaign C2's F1c [CLM-0043] [CLM-0062] |
| [FND-C2-6806aeca](findings/FND-C2-6806aeca/) | G6 | SAFETY_VIOLATION | DESIGN | high | POSTGRES_INHERITED | REPRODUCED (3/5) | NEW | 3 | NOT_REPRODUCED (0/5) | the same mechanism on registers: an `ok` read returns a value the next failover removes [CLM-0044] [CLM-0062] |
| [FND-C2-8215101c](findings/FND-C2-8215101c/) | G6 | SAFETY_VIOLATION | DESIGN | high | POSTGRES_INHERITED | REPRODUCED (5/5) | NEW | 0 | REPRODUCED (5/5) | the same register vanish under a one-way leader→follower drop (F1d), without F1c [CLM-0045] [CLM-0062] |
| [FND-C2-c7475a73](findings/FND-C2-c7475a73/) | G5 | SAFETY_VIOLATION | INFERRED | high | UNDETERMINED | NOT_REPRODUCED (0/5) | NEW | 0 | NOT_REPRODUCED (0/5) | list-append cycles that all run through a transaction whose commit the failover removed; whether they are more than the vanish's effect is not shown [CLM-0046] [CLM-0062] |
| [FND-C8-ff4475d8](findings/FND-C8-ff4475d8/) | G8 | PROTOCOL_DEVIATION | DESIGN | medium | MULTIGRES | INTERMITTENT (2/5) | NEW | 0 | NOT_REPRODUCED (0/5) | an explicit transaction started during a buffered planned switchover receives `57P03` instead of `40001` [CLM-0047] [CLM-0062] |
| [FND-C8-e6b7ca52](findings/FND-C8-e6b7ca52/) | G8 | PROTOCOL_DEVIATION | DESIGN | medium | MULTIGRES | NOT_REPRODUCED (0/5) | NEW | 0 | NOT_REPRODUCED (0/5) | as FND-C8-ff4475d8 (C8 seed 6) [CLM-0048] [CLM-0062] |
| [FND-C8-5254b5cb](findings/FND-C8-5254b5cb/) | G8 | PROTOCOL_DEVIATION | DESIGN | medium | MULTIGRES | INTERMITTENT (1/5) | NEW | 0 | NOT_REPRODUCED (0/5) | as FND-C8-ff4475d8 (C8 seed 10) [CLM-0049] [CLM-0062] |
| [FND-C9-40b6f05a](findings/FND-C9-40b6f05a/) | G10 | PROTOCOL_DEVIATION | DESIGN | medium | MULTIGRES | REPRODUCED (4/5) | KNOWN | 10 | REPRODUCED (4/5) | LISTEN notifications are not delivered after a failover until the client's next LISTEN; an independent reproduction supporting upstream PR #1550 (S53) [CLM-0050] [CLM-0062] |
| [FND-C9-2fe9a9fc](findings/FND-C9-2fe9a9fc/) | G10 | PROTOCOL_DEVIATION | DESIGN | medium | MULTIGRES | REPRODUCED (5/5) | KNOWN | 10 | REPRODUCED (5/5) | as FND-C9-40b6f05a (C9 seed 2) [CLM-0051] [CLM-0062] |
| [FND-C9-90da5532](findings/FND-C9-90da5532/) | G10 | PROTOCOL_DEVIATION | DESIGN | medium | MULTIGRES | INTERMITTENT (1/5) | KNOWN | 10 | INTERMITTENT (1/5) | as FND-C9-40b6f05a (C9 seed 3) [CLM-0052] [CLM-0062] |
| [FND-C11-17a344c1](findings/FND-C11-17a344c1/) | G12 | PROTOCOL_DEVIATION | DESIGN | medium | MULTIGRES | INTERMITTENT (2/5) | NEW | 0 | REPRODUCED (3/5) | the steady role's refusals during an injected failover counted by G12's literal rule: literal 766 of 886 ok in storm window 1, outside fault windows 744 of 744 [CLM-0053] [CLM-0062] |

K7.4's column counts each package's own class in the five replays of its minimized plan (`evidence/K7/NOTES.md` §8). FND-C2-6806aeca's five replays all FAIL with `incompatible-order`, the class of FND-C2-2a534585, whose minimized run they share, and none with `non-linearizable`; the three C8 packages' fifteen replays all PASS. Two K7.4 replays also report C3 cycle classes (FND-C2-6806aeca's replay 1, FND-D1-a8eb9857's replay 5); they are listed in the packages, not triaged, and no finding is derived from them (LIMITATIONS LIM-46). [CLM-0062]

G2b is not G2a: in these runs no acknowledged write was lost and no failed write became visible; what vanished had never been acknowledged to its client (ADR-0042). The G10 packages are independent reproductions supporting upstream PR #1550, not new issues (ADR-0043).
<!-- /README-FINDINGS -->

<!-- README-RESULTS -->
## Results: K7's matrix on the real build

130 valid runs on Multigres `e3cbdbb` (contract 1.8.1, with the metrics exporter, LIM-41): 19 FAIL, 111 PASS. INVALID attempts were re-run with the same seed and are listed in `evidence/K7/results.json`, never counted (§12.1). [CLM-0020]

**No acknowledged write was lost in any valid K7 run**: C1 (lost writes, final-read divergence) is valid in all 130. [CLM-0025]

| Cell | Campaign / profile | Valid runs | Failing runs | Violations k/N [Wilson 95 %] or bound for k = 0 | ok writes | Confirmed faults | Exposure over |
|---|---|---|---|---|---|---|---|
| V1 | C1 / baseline | 10 | 0 | none; bound 0.259 | 102620 | F2a ×9, F2b ×8, F2c ×10, F2d ×13 | 10 of 10 runs (0 full, 10 from checks.json) [CLM-0020] [CLM-0021] |
| V1 | C2 / baseline | 10 | 4 | G-nonadjacent 1/10 [0.017, 0.405]; G-single 1/10 [0.017, 0.405]; G2-item 1/10 [0.017, 0.405]; incompatible-order 3/10 [0.107, 0.604]; non-linearizable 2/10 [0.056, 0.510] | 54075 | F1a ×6, F1b ×8, F1c ×10, F1d ×6 | 10 of 10 runs (1 full, 9 from checks.json) [CLM-0020] [CLM-0021] |
| V1 | C3 / baseline | 10 | 0 | none; bound 0.259 | 93713 | F3a ×14, F3b ×16 | 10 of 10 runs (0 full, 10 from checks.json) [CLM-0020] [CLM-0021] |
| V1 | C4 / baseline | 10 | 0 | none; bound 0.259 | 81378 | F1a ×5, F2a ×6, F3a ×6, F5a ×10, F5b ×10 | 10 of 10 runs (0 full, 10 from checks.json) [CLM-0020] [CLM-0021] |
| V1 | C5 / baseline | 10 | 0 | none; bound 0.259 | 47945 | F2a ×30, F4a ×30 | 10 of 10 runs (0 full, 10 from checks.json) [CLM-0020] [CLM-0021] |
| V1 | C6 / baseline | 10 | 0 | none; bound 0.259 | 75680 | F2a ×40 | 10 of 10 runs (0 full, 10 from checks.json) [CLM-0020] [CLM-0021] |
| V1 | C7 / baseline | 10 | 0 | none; bound 0.259 | 35100 | F1b ×20, F2a ×20 | 10 of 10 runs (0 full, 10 from checks.json) [CLM-0020] [CLM-0021] |
| V1 | C9 / baseline | 10 | 10 | notify-not-resumed 10/10 [0.722, 1.000] | 28092 | F2a ×10, F2b ×10, F6 ×10 | 10 of 10 runs (0 full, 10 from checks.json) [CLM-0020] [CLM-0021] |
| V1 | C10 / baseline | 10 | 0 | none; bound 0.259 | 57949 | F1c ×10, F2a ×10, F2b ×10, F6 ×10 | 10 of 10 runs (0 full, 10 from checks.json) [CLM-0020] [CLM-0021] |
| V1 | C12 / baseline | 10 | 0 | none; bound 0.259 | 107917 | F1a ×2, F1b ×1, F1c ×1, F1d ×2, F2a ×1, F2b ×2, F2c ×2, F2d ×2, F2e ×1, F3a ×1, F3b ×1, F4a ×1, F5a ×10, F5b ×10, F6 ×1, F7 ×2 | 10 of 10 runs (0 full, 10 from checks.json) [CLM-0020] [CLM-0021] |
| V2 | C1 / slotless | 5 | 0 | none; bound 0.451 | 52879 | F2a ×5, F2b ×4, F2c ×6, F2d ×5 | 5 of 5 runs (0 full, 5 from checks.json) [CLM-0020] [CLM-0021] |
| V2 | C2 / slotless | 5 | 2 | incompatible-order 1/5 [0.036, 0.625]; non-linearizable 2/5 [0.117, 0.770] | 30088 | F1a ×4, F1b ×5, F1c ×4, F1d ×2 | 5 of 5 runs (1 full, 4 from checks.json) [CLM-0020] [CLM-0021] |
| V2 | C4 / slotless | 5 | 0 | none; bound 0.451 | 40757 | F1a ×2, F2a ×3, F3a ×2, F5a ×5, F5b ×5 | 5 of 5 runs (0 full, 5 from checks.json) [CLM-0020] [CLM-0021] |
| V3 | C8 / buffered | 10 | 3 | planned-failover-error 3/10 [0.107, 0.604] | 88780 | F6 ×30 | 10 of 10 runs (3 full, 7 from checks.json) [CLM-0020] [CLM-0021] |
| V5 | C11 / baseline | 5 | 0 | none; bound 0.451 | 17669 | F2a ×2 | 5 of 5 runs (0 full, 5 from checks.json) [CLM-0020] [CLM-0021] |

Exposure of a run held as a summary is the one cpcheck reported from the sealed history into its `checks.json`; it is re-derivable from the full archive, which is not in git. `cpgate K7` recomputes the exposure of every run held in full from its history and compares it (ADR-0044, LIM-43).

V5 (C11, G12, ADR-0037), both fractions over its 10 storm windows: literal 8868/8966 ok of invoked (windows below 90 %: 0); judged outside fault windows 8818/8818 (windows below 90 %: 0; not judged: 0). [CLM-0021]

§12.3's statement for each clean cell (cpreport's wording, recomputed by `cpgate K7`):

- In 10 valid runs of campaign C1 (profile baseline, Multigres e3cbdbb), exposing 102620 ok writes to 40 confirmed faults of kinds {F2a, F2b, F2c, F2d}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.259. Not tested: see docs/LIMITATIONS.md. [CLM-0021]
- In 5 valid runs of campaign C1 (profile slotless, Multigres e3cbdbb), exposing 52879 ok writes to 20 confirmed faults of kinds {F2a, F2b, F2c, F2d}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.451. Not tested: see docs/LIMITATIONS.md. [CLM-0021]
- In 10 valid runs of campaign C10 (profile baseline, Multigres e3cbdbb), exposing 57949 ok writes to 40 confirmed faults of kinds {F1c, F2a, F2b, F6}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.259. Not tested: see docs/LIMITATIONS.md. [CLM-0021]
- In 5 valid runs of campaign C11 (profile baseline, Multigres e3cbdbb), exposing 17669 ok writes to 2 confirmed faults of kinds {F2a}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.451. Not tested: see docs/LIMITATIONS.md. [CLM-0021]
- In 10 valid runs of campaign C12 (profile baseline, Multigres e3cbdbb), exposing 107917 ok writes to 40 confirmed faults of kinds {F1a, F1b, F1c, F1d, F2a, F2b, F2c, F2d, F2e, F3a, F3b, F4a, F5a, F5b, F6, F7}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.259. Not tested: see docs/LIMITATIONS.md. [CLM-0021]
- In 10 valid runs of campaign C3 (profile baseline, Multigres e3cbdbb), exposing 93713 ok writes to 30 confirmed faults of kinds {F3a, F3b}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.259. Not tested: see docs/LIMITATIONS.md. [CLM-0021]
- In 10 valid runs of campaign C4 (profile baseline, Multigres e3cbdbb), exposing 81378 ok writes to 37 confirmed faults of kinds {F1a, F2a, F3a, F5a, F5b}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.259. Not tested: see docs/LIMITATIONS.md. [CLM-0021]
- In 5 valid runs of campaign C4 (profile slotless, Multigres e3cbdbb), exposing 40757 ok writes to 17 confirmed faults of kinds {F1a, F2a, F3a, F5a, F5b}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.451. Not tested: see docs/LIMITATIONS.md. [CLM-0021]
- In 10 valid runs of campaign C5 (profile baseline, Multigres e3cbdbb), exposing 47945 ok writes to 60 confirmed faults of kinds {F2a, F4a}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.259. Not tested: see docs/LIMITATIONS.md. [CLM-0021]
- In 10 valid runs of campaign C6 (profile baseline, Multigres e3cbdbb), exposing 75680 ok writes to 40 confirmed faults of kinds {F2a}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.259. Not tested: see docs/LIMITATIONS.md. [CLM-0021]
- In 10 valid runs of campaign C7 (profile baseline, Multigres e3cbdbb), exposing 35100 ok writes to 40 confirmed faults of kinds {F1b, F2a}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.259. Not tested: see docs/LIMITATIONS.md. [CLM-0021]

The failing cells' runs are triaged in `evidence/K7/triage/`: 14 repeat a recorded finding's mechanism (ADR-0045's predicates over `checks.json`) and are cited by those packages as recurrences; 5 match no predicate and became the five new findings below. [CLM-0026]
<!-- /README-RESULTS -->

<!-- README-MUTANTS -->
## Mutants and the real build, side by side (cited directed cells, not re-run)

The directed cells K4 and K5 ran, cited by K7 with the contract versions their plans carry (ADR-0039); none ran with the metrics exporter (LIM-41).

| Mutant (§9) | Directed cell | Contract | Mutant build | Real build |
|---|---|---|---|---|
| M1 (`sync-standby-cleared`) | D1 (K4) | 1.0.6 | 5/5 FAIL (incompatible-order 5/5 [0.565, 1.000]; lost-write 5/5 [0.565, 1.000]) | 3/5 FAIL (incompatible-order 3/5 [0.230, 0.883]) [CLM-0022] |
| M2 (`cohort-add-local-view`) | D2′ (K4) | 1.0.7 | 4/5 FAIL (lost-write 4/5 [0.375, 0.964]) | 0/5 FAIL (bound 0.451) [CLM-0022] |
| M4 (`guard-removed`) | P1 probe, campaign `claims` (K4) | 1.0.6 | 5/5 FAIL (p1-not-rejected 5/5 [0.565, 1.000]) | 0/5 FAIL (bound 0.451) [CLM-0022] |
| M3 (`gateway-retry-any`) | D3′ (K5) | 1.1.0 | 4/5 FAIL (g1a 4/5 [0.375, 0.964]) | 0/5 FAIL (bound 0.451) [CLM-0023] [CLM-0024] |
| M5 (`standby-slots-cleared`) | D5 (K5) | 1.0.9 | 5/5 FAIL (cdc-phantom 5/5 [0.565, 1.000]) | 0/5 FAIL (bound 0.451) [CLM-0023] [CLM-0024] |

D1 on the real build failed in 3 of 5 runs with `incompatible-order`: those are three of the G2b findings below, not a mutant effect. [CLM-0022]

§12.3's statement for each clean cited real-build cell (D3′ and D5 from their runs committed in full under `evidence/K7/cited/`, ADR-0047):

- In 5 valid runs of campaign D2p (profile baseline, Multigres e3cbdbb), exposing 37825 ok writes to 30 confirmed faults of kinds {F2a, F2b, F3a, F5a}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.451. Not tested: see docs/LIMITATIONS.md. [CLM-0022]
- In 5 valid runs of campaign DH1 (profile h1, Multigres 4bca1d5), exposing 4982 ok writes to 5 confirmed faults of kinds {F3a}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.451. Not tested: see docs/LIMITATIONS.md. [CLM-0022]
- In 5 valid runs of campaign claims (profile baseline, Multigres e3cbdbb), exposing 15 ok writes to 0 confirmed faults of kinds {}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.451. Not tested: see docs/LIMITATIONS.md. [CLM-0022]
- In 5 valid runs of campaign D3p (profile buffered, Multigres e3cbdbb), exposing 53281 ok writes to 45 confirmed faults of kinds {F2a, F7}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.451. Not tested: see docs/LIMITATIONS.md. [CLM-0024]
- In 5 valid runs of campaign D5 (profile baseline, Multigres e3cbdbb), exposing 2654 ok writes to 15 confirmed faults of kinds {F1b, F2a, F2b}, no violation of {C1, C2, C3, C4, C5, C6, C7, C8, C9, P} was observed; at 95% confidence the per-run violation probability under this exact schedule distribution is below 0.451. Not tested: see docs/LIMITATIONS.md. [CLM-0024]
<!-- /README-MUTANTS -->

<!-- README-H1 -->
## Historical control

H1 (issue #778's commit `4bca1d5`, on that commit's own single-cell demo topology): **NOT_DETECTED** — 0 of 5 valid DH1 runs lost an acknowledged write (LIM-17). [CLM-0003]
<!-- /README-H1 -->

## Job bullet → evidence matrix (CONTRACT §4)

Reproduced verbatim from the contract:

| # | Bullet (S47) | Coverage | Evidence (gate / artifact) | What is explicitly not claimed |
|---|---|---|---|---|
| 1 | Query routing logic for sharded databases | **PARTIAL** (non-sharded only) | Leader routing under faults: G6 (no stale reads from superseded primary), G8 (buffered routing during switchover), mutant M3 (unsafe retry in the gateway's error classifier) — gates K5.4, K4.3 | Any sharded routing, query splitting, scatter/gather. Multigres has no sharding at the pinned commit (S48). |
| 2 | Consensus and replication for distributed durability | **COVERED** | G1–G3 campaigns C1–C7 (K4), mutants M1/M2 detection, historical control H1, minimized replayable findings — gates K4.2–K4.6, K7.2 | Proofs. Results are empirical over the §12 matrix. |
| 3 | Materialization pipelines for migrations and CDC | **PARTIAL** (CDC failover only) | G11 campaign C10, mutant M5 (logical decoding not gated by standby slots) — gate K5.3 | Migrations and materialization: the migrator exists only as a proto (S30). No pipeline is built or tested. |
| 4 | Connection pooling and workload isolation | **COVERED** (as fault behavior) | G8–G10, G12 campaigns C8, C9, C11; mutant M3 — gates K5.1, K5.2, K5.4 | Pool performance optimization; pooler internals beyond black-box behavior. |
| 5 | Open-source collaboration | **PARTIAL** | Upstream-format evidence packages for every finding (§11.4), DCO-ready patches for mutants, issue drafts — gate K7.4 | Any actual upstream interaction: publication is the owner's decision (AGENTS.md). |
| 6 | Sharding, relational algebra, Postgres internals | **PARTIAL** (internals only) | Internals exercised and explained: sync-rep cancel semantics (C7, S61), pg_rewind same-timeline divergence (C6, S44), WAL/LSN and timeline reasoning in findings; isolation checking (C3) | Sharding and relational algebra: nothing to test (S48). Visibility order and snapshot isolation on replicas: not tested (C13 removed, ADR-0040; LIM-42). |
| 7 | Consensus protocols (Raft, Paxos, FlexPaxos), distributed transactions | **PARTIAL** | FlexPaxos quorum-intersection table cross-checked against upstream `CheckSufficientRecruitment` (§10.8, gate K4.1); equivalence analysis of recruitment mutants (§9.3); consensus campaigns (K4) | Distributed transactions: single shard, no 2PC in scope; Raft/Paxos implementations are not built. |
| 8 | Stream processing, materialization, CDC | **PARTIAL** | CDC consumer with gap/phantom/duplicate checking across failover (C10, C7 checker) — gate K5.3 | Stream processing frameworks, materialized views. |
| 9 | Robust, high-performance distributed systems with strong observability | **COVERED** (for the harness) | The harness itself: fail-closed fault confirmation (K3), sealed hash-chained histories (K1), byte-reproducible incident timeline/report with metrics (K6), statistics (K7) | Production deployment of CommitProof; performance tuning of Multigres. |
| 10 | Low-latency infrastructure and network protocol optimization | **PARTIAL** (measurement only) | Wire-protocol-level outcome capture (SQLSTATE, NOTICE/WARNING), commit-latency vs injected RTT (F7, C1/C12), failover latency distributions (K6, K7) | Any optimization of Multigres or of a network protocol. |
| D78 | Discussion #78 scale expectations | **PARTIAL** | Load spikes/overload (C11), timeouts and retries (M3, C7, G8), cascading failures (rule changes per fault, observations), minimal-disruption deploys (C8 graceful switchover), observability (K6) | Cross-cluster and cross-cloud interaction: single host. |

Evidence for each row: the gates named in it are evaluated by `cpgate` over [`evidence/K3/`](evidence/K3/), [`evidence/K4/`](evidence/K4/), [`evidence/K5/`](evidence/K5/), [`evidence/K6/`](evidence/K6/) and [`evidence/K7/`](evidence/K7/) (each with `gate.json`, `negative-controls.json` and `closure.yaml`); the quorum table (K4.1) is [`evidence/K4/quorum-table.json`](evidence/K4/quorum-table.json); the findings packages are under [`findings/`](findings/); K7.2's statistics are in [`evidence/K7/results.json`](evidence/K7/results.json) and the campaign pages under [`evidence/K7/report/`](evidence/K7/report/).

## How to reproduce

Prerequisites: a Linux host as [`docs/PREREQUISITES.md`](docs/PREREQUISITES.md) lists (Docker 29.9 with buildx and Compose, cgroup v2, the kernel modules `nf_tables`, `nft_compat`, `sch_netem` and `/dev/fuse`, ≥ 12 GiB free RAM, the bridge subnet 10.77.0.0 (a /24 network) free, Go ≥ 1.21 to bootstrap go1.26.9). Every entry point is a `make` target:

```sh
make prereq-check          # the 26 checks of docs/PREREQUISITES.md
make build                 # bin/commitproof, bin/cpcheck, bin/cpgate, bin/cpreport, bin/quorumxcheck
bin/commitproof images build   # Multigres e3cbdbb and the helpers from source; images.lock.json
make ci-unit               # gofmt, go vet, staticcheck, unit tests, independence and parameters tests
make ci-checkers           # synthetic corpora, elle-cli differential, checker sabotage controls
make ci-cluster-smoke      # cluster up / verify / down, a 45 s smoke run, cpcheck
make ci-nemesis-smoke      # one run per fault kind F1a, F2a, F3a, F7 (F4a with /dev/fuse)
make ci-mutant-smoke       # mutant M1, directed schedule D1, expects C1 lost-write
make ci-report             # re-renders the K6 reference report and compares the bytes
make campaign-k7 DRY_RUN=1 # K7's 130 runs with their plans; starts nothing
make campaign-k7           # the K7 matrix, detached and resumable (several hours, local only)
```

One run by hand: `bin/commitproof run --campaign C2 --seed 3 --out runs/` writes a sealed run directory; `bin/cpcheck check runs/<run-id>` writes its verdict next to it; `bin/cpreport render runs/<run-id> --out report/` renders its incident report. Every finding package gives the exact commands that replay its minimized plan. A sprint's gate: `bin/cpgate K7 --evidence evidence/K7 --negative-controls`.

## How to read the findings

Each package under [`findings/`](findings/) is written in the upstream issue format (S50) and has not been posted anywhere. Its first line states whether any acknowledged write was lost in its original run. Then come the classification (CONTRACT §11) and the sections Summary, Version and environment, Reproduction, Expected vs actual, Analysis, Recurrences in K7, K7 replay, Suggested direction (optional) and Limitations. Beside them: `finding.json` (runs, seal roots, replays), `plan.json` (the minimized plan), `checks.json` (the original run's checker output), `report-excerpt.md` (faults, rules, WAL positions, windows) and `claim.yaml`. The dimensions:

- **Guarantee and claim level** (§3): EXPLICIT is a public product statement (S48, S49, S42); DESIGN is a repository design document or test intent; INFERRED is an expectation derived from PostgreSQL compatibility that no Multigres source states.
- **Class and severity** (§11.1, §11.2, derived): `critical` needs a SAFETY_VIOLATION of an EXPLICIT guarantee; `high` is a SAFETY_VIOLATION of a DESIGN or INFERRED one.
- **Origin**: MULTIGRES, POSTGRES_INHERITED (the behaviour reproduces on plain PostgreSQL streaming replication or is documented PostgreSQL behaviour) or UNDETERMINED.
- **Reproducibility** (§11.1): from 5 replays of the minimized plan — reproduced (at least 3 show the class), intermittent (1 or 2), not reproduced (none; the original history is the evidence).
- **Known status**: NEW, or KNOWN when an open upstream issue or unmerged pull request describes the same mechanism. A KNOWN package is an independent reproduction supporting that item, not a new issue.

## Limitations

The full list, with every status resolved, is [`docs/LIMITATIONS.md`](docs/LIMITATIONS.md). In short:

- One pinned commit of an alpha system, on one host, in a Compose topology derived from upstream's Kubernetes demo, not the operator or real zones (LIM-01, LIM-02).
- No power-loss or page-cache-loss faults (shared kernel), no clock skew, and a narrow, excluded fault list (LIM-03, LIM-04, LIM-20).
- Empirical, bounded evidence only: a clean cell says nothing about schedules outside the matrix (LIM-06). Narrow workloads (LIM-07) and a checker subset (LIM-08, LIM-22).
- Not covered: sharding, migrations, distributed transactions, replica-port reads (LIM-09, LIM-19, LIM-42); one durability policy (LIM-14); five mutants (LIM-15).
- Most runs are committed as summaries whose full archives stay on the machine that ran them; their exposure is cpcheck-reported from the sealed history and re-derivable only from that archive (LIM-30, LIM-37, LIM-43).
- K7's runs have the metrics exporter, the cited K4/K5 runs do not (LIM-41). There has been no remote CI run before the owner pushes (LIM-16).

## Repository

`cmd/` — the binaries (`commitproof`, `cpcheck`, `cpgate`, `cpreport`, `quorumxcheck`, and the helpers `slowfs`, `cpnet`). `internal/` — their packages. `build/`, `deploy/` — images and the Compose topology. `campaigns/` — the K7 matrix. `mutants/` — the five mutant patches. `docs/` — contract, ADRs, research register, limitations, prerequisites. `evidence/` — every sprint's records, runs and gate. `findings/` — the findings packages. `claims.yaml` — every claim of this README, LIMITATIONS and K7's NOTES with its artifact and hash. License: Apache-2.0.
