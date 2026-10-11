# ADR-0027 — Proposed: D2 must make n4 unavailable when the failover recruits (amendment of CONTRACT §9.2 D2)

Status: **Proposed — awaiting the owner's approval (CONTRACT §9.1, §18). Not applied.** Changing a directed schedule after seeing mutant results is a relaxation unless the owner approves; K4 stopped BLOCKED on K4.3 instead.

## Context (defect in D2's design)

§9.2 models M2 (`cohort-add-local-view`, S41) as: n4's acks count, the rule is not written, "writes acked only by n4 are lost when failover recruits from the old cohort". D2 = F5a → F3a(other two followers) → 40 s load → F2a + F2b(leader) → heal F3a → recovery.

K4 evidence: the five M2 runs of D2 (evidence/K4/runs/*-D2-*-M2-*) all PASS; in each, 17–19 s after the leader's kill a pooler reports n4 as leader of rule 2.0 with recorded cohort {n1, n2, n3} (observations), i.e. the member M2 never wrote into the cohort was recruited and elected. The real build on D2 also PASSes (5/5), electing n4 or a base follower with cohort {n1, n2, n3, n4}.

At the pinned commit, multiorch does not recruit "from the old cohort": `AppointLeader` recruits every pooler of the shard (`go/services/multiorch/recovery/actions/appoint_leader.go`: `cohort[i] = p.Health()` for `shard.Poolers`; `consensus/coordinator.go`: "It operates on a cohort of nodes (all nodes in the shard)", "All poolers in the cohort are recruited regardless of their current health state"). Only the quorum check is restricted to the recorded cohort (`go/common/consensus/proposals.go` `buildProposalCore`: `CheckSufficientRecruitment(..., filterCohortStatuses(cohort, recruitedStatuses))`); the leader is chosen among *all* recruited poolers at the most advanced position (`discoverMostAdvancedRulePositions`, `narrowByLSN`). Under M2, n4 holds the 40 s of writes only it acknowledged, is recruited, has the highest LSN and is elected (observed in all five runs); nothing acknowledged is lost. D2 therefore never reaches M2's defect.

## Proposed decision

D2′: F5a (admission predicate) → F3a(base-follower:0), F3a(base-follower:1) held 40 s → F2a(leader), F2b(same:3) **and F2b(n4)** → heal both F3a → the old leader and n4 stay down 30 s → heals → recovery → final reads.

- M2: the recorded cohort is {n1, n2, n3}; n2 and n3 (2 of 3 ≥ max(2, 2)) suffice, a follower without the n4-only writes is elected, n4 is rewound on return → C1 lost-write (S41's "orchestrator B fails over with the old membership view").
- Real build: the cohort is {n1, n2, n3, n4}; n2 and n3 are 2 of 4 < 3; no failover until n1 or n4 returns with the writes → no loss (unavailability only).

This does not make detection easier than the modelled failure mode warrants: S41's scenario requires that the failover not see the new member; D2′ makes n4 unreachable instead of relying on a membership view multiorch does not use.

## Alternatives

- Keep D2 and record M2 as UNDETECTED (its detection gate K4.3 then fails permanently).
- Pause n4 (F3a) instead of killing its pooler: Recruit RPCs to a pooler whose PostgreSQL is paused may hang until the coordinator proceeds without it; F2b is deterministic.

## Sources

CONTRACT §9.1, §9.2, §18; S24, S41; ADR-0007, ADR-0024.
