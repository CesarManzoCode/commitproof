# K4 — Predictions for D2′, written before any D2′ run

Written and committed before the first D2′ run (ADR-0027, contract 1.0.7; plan identifier `D2p`), as the owner's decision on ADR-0027 requires. No D2′ plan had been generated or run when this file was committed; its commit precedes every `*-D2p-*` run in `evidence/K4/runs/` (each run's manifest carries the commitproof commit it ran on).

Schedule (CONTRACT §9.2, ADR-0027): F5a(n4) → F3a(base-follower:0), F3a(base-follower:1) held 40 s under W1/W4 load → F2a(leader), F2b(same:3), F2b(n4) → heal both F3a → the old leader and n4 stay down 30 s → F2b(leader), F2a(leader), F2b(n4) healed → recovery → final reads. Seeds 1–5, on M2 and on the real build.

## Predictions (the owner's, verbatim)

- **M2 on D2′: FAIL with lost-write in ≥ 4 of 5 runs** (the writes acknowledged only by n4 are lost: the recorded cohort is {n1,n2,n3}, n2 and n3 suffice, a follower without those writes is elected, n4 is rewound on return).
- **Real build on D2′: PASS in 5 of 5** (cohort {n1..n4}, n2+n3 are 2 of 4 < 3 recruited, no failover until n1 or n4 returns, unavailability only). If the real build loses an acknowledged write, that is a finding, recorded verbatim as always.

## What would show the mechanism (observations, not verdicts)

These are read from each run's observations and nemesis records after the runs; they explain a verdict, they never decide one (ADR-0016).

- M2: while the old leader and n4 are down, a base follower (n2 or n3) reports itself leader of a newer rule whose cohort is {n1, n2, n3}; n4 is never leader after the kill.
- Real build: no leader of a newer rule appears before the old leader's (or n4's) pooler returns; the new leader, if the rule changes at all, holds the writes acknowledged during the 40 s.

## Decision rule (fixed in advance)

K4.3 counts M2 as DETECTED iff ≥ 4 of the 5 valid D2′ runs on M2 are FAIL with C1 `lost-write` (P14). Otherwise M2 is UNDETECTED with all its runs, D2′ is not changed again, and K4 stops BLOCKED on K4.3 (ADR-0027, "one amendment only"). The real build's five D2′ results are recorded whatever they are; a real-build FAIL is minimized, replayed 5× and triaged (K4.5, K4.7). Every new FAIL run is minimized and replayed (K4.5).
