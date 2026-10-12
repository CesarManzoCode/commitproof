# ADR-0027 — D2′: D2 makes n4 unavailable when the failover recruits (amendment of CONTRACT §9.2 D2)

Status: **Accepted (K4, contract 1.0.7)**: proposed after K4 stopped BLOCKED on K4.3 and approved by the owner on 2026-10-11 (see "Owner decision"). Changing a directed schedule after seeing mutant results needs the owner (CONTRACT §9.1, §15 K4 rejection conditions, §18); this ADR records that approval.

## Context (defect in D2's design)

§9.2 models M2 (`cohort-add-local-view`, S41) as: n4's acks count, the rule is not written, "writes acked only by n4 are lost when failover recruits from the old cohort". D2 = F5a → F3a(other two followers) → 40 s load → F2a + F2b(leader) → heal F3a → recovery.

K4 evidence: the five M2 runs of D2 (evidence/K4/runs/*-D2-*-M2-*) all PASS; in each, 17–19 s after the leader's kill a pooler reports n4 as leader of rule 2.0 with recorded cohort {n1, n2, n3} (observations), i.e. the member M2 never wrote into the cohort was recruited and elected. The real build on D2 also PASSes (5/5), electing n4 or a base follower with cohort {n1, n2, n3, n4}.

At the pinned commit, multiorch does not recruit "from the old cohort": `AppointLeader` recruits every pooler of the shard (`go/services/multiorch/recovery/actions/appoint_leader.go`: `cohort[i] = p.Health()` for `shard.Poolers`; `consensus/coordinator.go`: "It operates on a cohort of nodes (all nodes in the shard)", "All poolers in the cohort are recruited regardless of their current health state"). Only the quorum check is restricted to the recorded cohort (`go/common/consensus/proposals.go` `buildProposalCore`: `CheckSufficientRecruitment(..., filterCohortStatuses(cohort, recruitedStatuses))`); the leader is chosen among *all* recruited poolers at the most advanced position (`discoverMostAdvancedRulePositions`, `narrowByLSN`). Under M2, n4 holds the 40 s of writes only it acknowledged, is recruited, has the highest LSN and is elected (observed in all five runs); nothing acknowledged is lost. D2 therefore never reaches M2's defect.

## Decision

D2′: F5a (admission predicate) → F3a(base-follower:0), F3a(base-follower:1) held 40 s → F2a(leader), F2b(same:3) **and F2b(n4)** → heal both F3a → the old leader and n4 stay down 30 s → heals → recovery → final reads.

- M2: the recorded cohort is {n1, n2, n3}; n2 and n3 (2 of 3 ≥ max(2, 2)) suffice, a follower without the n4-only writes is elected, n4 is rewound on return → C1 lost-write (S41's "orchestrator B fails over with the old membership view").
- Real build: the cohort is {n1, n2, n3, n4}; n2 and n3 are 2 of 4 < 3; no failover until n1 or n4 returns with the writes → no loss (unavailability only).

Details fixed before any D2′ run:

- Elements, in injection order: 0 F5a(n4), 1 F3a(base-follower:0), 2 F3a(base-follower:1) held 40 s, 3 F2a(leader), 4 F2b(same:3), 5 F2b(n4). F2b(n4) kills n4's pooler; n4's PostgreSQL keeps running, as the leader's pooler-less PostgreSQL does not (F2a).
- Heal sequence: F3a(base-follower:1), F3a(base-follower:0); 30 s after the second F3a heal is confirmed, F2b(leader), then F2a(leader) (as in D2, ADR-0024), then F2b(n4). n4 is down at least as long as the old leader.
- Workloads, step anchor, durations and the target class (C1 `lost-write`) are D2's. Thresholds are unchanged: K4.3 needs M2 via D2′ FAIL with `lost-write` in ≥ 4 of 5 runs (P14); the real build runs D2′ five times (seeds 1–5) and its results are recorded whatever they are.
- Identifier: plans, matrices and run IDs name D2′ `D2p` (`campaign: "D2p"`, `directed: "D2p"`); the prime is not used in file names.

This does not make detection easier than the modelled failure mode warrants: S41's scenario requires that the failover not see the new member; D2′ makes n4 unreachable instead of relying on a membership view multiorch does not use.

## Owner decision

Approved by the owner on 2026-10-11, with these conditions (verbatim):

> Owner decision on ADR-0027: APPROVED, with the conditions below. Do exactly this, end to end. Do not push anything.
>
> 1. Apply the amendment.
>    - Change ADR-0027 status to Accepted. Add an "Owner decision" section: approved by the owner on 2026-10-11, with these conditions verbatim.
>    - Amend CONTRACT §9.2 D2 to D2′ exactly as the ADR specifies (F5a → F3a(base-follower:0), F3a(base-follower:1) held 40 s → F2a(leader), F2b(same:3) and F2b(n4) → heal both F3a → old leader and n4 stay down 30 s → heals → recovery → final reads). Bump the contract to 1.0.7, update the ADR index, and commit separately: docs(contract): D2′ makes n4 unavailable when the failover recruits (ADR-0027).
>    - Thresholds and expected class do not change: K4.3 stays ≥ 4/5 with lost-write for M2.
>    - Add a limitation (LIM): D2′ models the correlated crash of the leader and the new member; it does not model an orchestrator failing over with a stale membership view, because multiorch does not use one (appoint_leader.go recruits every pooler of the shard).
>
> 2. Write the predictions BEFORE any run, in a committed file (docs or evidence/K4, your choice) with its own commit:
>    - M2 on D2′: FAIL with lost-write in ≥ 4 of 5 runs (the writes acknowledged only by n4 are lost: the recorded cohort is {n1,n2,n3}, n2 and n3 suffice, a follower without those writes is elected, n4 is rewound on return).
>    - Real build on D2′: PASS in 5 of 5 (cohort {n1..n4}, n2+n3 are 2 of 4 < 3 recruited, no failover until n1 or n4 returns, unavailability only). If the real build loses an acknowledged write, that is a finding, recorded verbatim as always.
>
> 3. Run D2′ 5 times on M2 and 5 times on the real build (seeds 1–5, same as before). Update the plan generator, the gate expectations and tests for D2′ without changing any threshold, repetition count or expected class.
>
> 4. Keep the original D2 runs (M2 0/5, real 5/5 PASS) in evidence/K4/ as the superseded schedule. Do not delete or edit them. Record in NOTES §5 that the original D2 did not detect M2 because multiorch recruits every pooler of the shard and elects the most advanced one, with the upstream files you already cite.
>
> 5. One amendment only. If M2 is not detected in ≥ 4 of 5 under D2′, record M2 as UNDETECTED with all its runs, do not change the schedule again, and stop with BLOCKED on K4.3.
>
> 6. After the runs: minimize and replay any new FAIL run per K4.5, re-run `cpgate K4 --evidence evidence/K4 --negative-controls`, regenerate closure.yaml and NOTES.md. The negative controls must now fail only their own criterion (failed_only_target true for all eight); if any does not, say why.
>
> 7. Close the sprint per AGENTS.md §7: DONE if the gate exits 0, BLOCKED otherwise. Commit everything. Print the full closure report. Do not start K5.

## Alternatives rejected

- Keep D2 and record M2 as UNDETECTED (its detection gate K4.3 then fails permanently): the owner chose the amendment.
- Pause n4 (F3a) instead of killing its pooler: Recruit RPCs to a pooler whose PostgreSQL is paused may hang until the coordinator proceeds without it; F2b is deterministic.

## Consequences

- One amendment only: if M2 is not detected in ≥ 4 of 5 D2′ runs, M2 is recorded UNDETECTED with all its runs, D2′ is not changed again, and K4 stops BLOCKED on K4.3.
- The original D2 is superseded, not deleted: its ten K4 runs (M2 0/5, real 5/5 PASS) stay in evidence/K4/ unedited and cpgate still checks their integrity and reports them next to D2′. The `D2` campaign stays in the plan generator so the superseded matrix (`campaigns/k4.json`) still reads.
- Contract 1.0.7 changes only M2's directed schedule, as a new campaign identifier; every other K4 plan (C1–C7, D1, D2, DH1, claims) is unchanged apart from its `contract_version` field. The K4 runs made under contract 1.0.6 therefore stay K4 evidence, as the K2 corpus keeps its own version (`CorpusContractVersion`): cpgate accepts contract 1.0.6 for every K4 run except D2′ runs, which must be 1.0.7. They are not re-run.
- D2′ models a correlated crash of the leader and the new member, not an orchestrator failing over with a stale membership view (LIM-34).

## Sources

CONTRACT §9.1, §9.2, §15 K4, §18; S24, S41; ADR-0007, ADR-0024.
