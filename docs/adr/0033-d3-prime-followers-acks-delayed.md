# ADR-0033 — D3′: the leader is killed while every commit waits for its delayed standby acknowledgement (amendment of CONTRACT §9.2 D3)

Status: **Accepted by the owner (K5, contract 1.0.10)**. The owner approved D3′ on 2026-10-14 (see "Owner decision"); ADR-0031 proposed it. Before any D3′ run, ADR-0031's composition was checked against the ten D3 runs and the pinned source, and found unable to reach M3's defect. This ADR fixes the composition that can, together with every detail, before the first D3′ run. Checkers, verdict rules, the expected classes and P14 are unchanged.

## Owner decision

Approved by the owner on 2026-10-14. The decision's M3 item, verbatim:

> 1. M3 / D3′: APPROVED.
>    - Write the ADR accepting D3′ (next free ADR number) and a predictions file BEFORE any D3′ run, including the expected outcome for M3 and for the real build. Commit both. Every D3′ run must start after that commit.
>    - D3′ must deliberately align the kill with an in-flight commit, in profile `buffered`. It may change the schedule only, not checkers, verdicts or thresholds. Detection threshold stays ≥ 4/5.
>    - Keep the original D3 results (M3 2/5) in the evidence and in NOTES. D3′ adds to them and replaces nothing.
>    - Run M3 ×5 and the real build ×5 under D3′. If the real build fails, triage it as a real finding.
>    - Run D3′ once. If M3 is still below the threshold, record M3 as undetected, investigate and report, and do not iterate on schedules.

## Context

**What M3 needs.** M3 (`gateway-retry-any`) differs from the real build only for a statement that fails *after* it began executing (S14; ADR-0031). It is detectable (C2 `g1a`) only if:

1. the statement's commit is durable on the node that leads next;
2. the gateway gets an error for it;
3. M3's retry then ends in a definite failure the client classifies `fail` (§6.2: `23505`, "no writable primary") before the client's P01 deadline (10 s from its invoke).

In D3, condition 1 with condition 2 happens only when a kill catches a 3–8 ms commit (2 of 5 runs; ADR-0031).

**What happens after an F2a**, from the ten primary D3 runs (record `d3-diagnosis`: `scripts/k5-d3-diagnosis.sh`, stdout sha256 `efb7e145…3bd64`; record `k3obs`, stdout sha256 `f64be1b7…3596f`) and the pinned source (printed by the same record):

- **The killed leader cannot be recruited.** In all ten runs the first appointment after the first kill logs `recruit failed {pooler: <killed leader>, "cannot accept revocation: unknown WAL position"}` (`go/common/consensus/revocation.go:358`). The two followers are recruited at the same LSN and one is promoted about 1 s after the orchestrator sees the problem. A new leader is observed 1.9–7.0 s after the kill (k3obs `new_leader_after_inject_ms`); the old leader's PostgreSQL is back 1.3–5.7 s after it (`healed_after_inject_ms`).
- **The old leader's pooler restarts PostgreSQL as a standby.** `MonitorPostgres` starts it (`postgres_monitor.go:1374–1384`). If the rule still names it, it resigns rather than promoting itself: "rule names us leader but postgres is a standby; resigning" (`postgres_monitor.go:1338–1349`; D3 seed 2, second kill).
- **Recruitment backs off.** Once any pooler holds a revocation (or a resignation) relevant to the current decision, an orchestrator's next failover attempt waits at least 10 s, plus up to 25 % (`failover_backoff.go:68–77`, `ha/backoff.go:57–63`). After D3's second and third kills new leaders appeared up to 34.8 s after the kill.
- **The gateway stops buffering** when a leader becomes SERVING: a new one or the same one again (`load_balancer.go:559–570`). M3 buffers only the first kill of a run (the 1 min timing guard, ADR-0029); later kills are retried at once, at most 3 times (`MaxBufferingRetries`).

**Why ADR-0031's composition cannot detect M3.** ADR-0031 proposed F1b(A = followers, B = rest) held 5 s, then F2a(leader). F1b *drops* packets, so the in-flight commits' WAL never reaches the followers; only the old leader holds those rows. Then:

- **With ADR-0031's heal order** (F1b healed right after F2a is confirmed), the orchestrators reach the followers at once. They recruit them while the dead leader cannot be recruited, and elect a follower without the rows. M3's retry succeeds there, and the first execution is rewound with the old leader: no anomaly.
- **With the order ADR-0031's rationale needs** (F1b held until the old leader's PostgreSQL is back), the restarted standby accepts a revocation while the followers are unreachable. Recruitment fails, and the next attempt waits ≥ 10 s. The interrupted statements (invoked ≥ 1.3 s before the kill, F1b's confirmation time, D5 k3obs) reach P01 first and end as client timeouts (`info`): no anomaly.

A schedule that reaches M3's defect must instead leave the in-flight commits' WAL on the followers, keep the followers responsive, and kill the leader while the commits still wait for their acknowledgements.

## Decision

**D3′** (identifier `D3p`; profile `buffered`). Workloads, pacing, clients, seeds 1–5, three steps, the first at t = 10 s (P05), each later one 15 s after the previous step's last heal is confirmed: all as D3 (ADR-0030). Each step:

- **Elements, in injection order:**
  - 0: F7(base-follower:0);
  - 1: F7(base-follower:1);
  - 2: F2a(leader), injected as soon as element 1 is confirmed (no hold).
- **Heals:** F7(base-follower:0) and F7(base-follower:1) immediately after F2a is confirmed, then F2a (PostgreSQL restarted by its pooler; §8's manual start otherwise). The step's duration is 0.

**Mechanism.** F7 is P18's 50 ms egress delay in a follower node's namespace, which its pooler shares. Each follower receives the WAL at once, flushes it and replies, but the reply reaches the leader ≥ 50 ms later. With both followers delayed, every commit spends ≥ 50 ms in its synchronous-replication wait with its WAL already flushed on both followers. W1's and W4's statements otherwise take 3–8 ms, so at the kill nearly every one of the six clients has a commit in that state.

At the postmaster's death those backends exit without a reply; the gateway reports an error after execution (`XX000` on the real build, `info`, as in D3). The followers stay responsive, so the first failover of the run recruits them at once and elects one that holds the rows. On M3, the first (buffered) kill's interrupted W4 inserts are retried on that leader and fail with `23505` while their rows are present: C2 `g1a`.

**Fixed details:**

- 50 ms is P18.
- `base-follower:0/1` are resolved at each step from the current leader.
- Expected classes: §9.2's two, either detects (ADR-0030): C3 `duplicate-elements`, C2 `g1a`.
- Minimization target: `duplicate-elements` if present, else `g1a`.
- K5.4 needs M3 via D3′ FAIL with an expected class in ≥ 4 of 5 runs (P14).
- The real build runs D3′ five times; its results are recorded whatever they are, and a real-build FAIL is minimized, replayed and triaged as a finding (K5.5).

**Not changed:**

- §9.2's class list. ADR-0031's proposed correction (listing only `g1a`) is not applied: the owner limited D3′ to the schedule. C3 `duplicate-elements` stays unreachable in D3′ as in D3: an interrupted autocommit append that M3 retries succeeds (it is an upsert) and appears twice only in final reads, which C3 does not judge (§10.3, LIM-22).
- Checkers, verdict rules, P01, P14, P18.

**Why this does not make detection easier than the failure mode warrants.** A synchronous standby 50 ms away (another zone) and a leader crash while commits wait for it are the ordinary situation in which an unsafe retry executes a statement twice. D3 reached it only when a kill and a 3–8 ms commit coincided.

**One amendment only.** If M3 is not detected in ≥ 4 of 5 D3′ runs:

- M3 is recorded UNDETECTED with every D3 and D3′ run;
- D3′ is not changed again;
- the investigation is reported;
- K5 stops BLOCKED on K5.4.

## Alternatives rejected

- **ADR-0031's composition** (F1b, held 5 s, with either heal order): cannot detect M3 at the pinned commit (Context).
- **F3a on both followers' PostgreSQL**:
  - the WAL would wait in the paused followers' socket buffers;
  - each confirmation takes ≥ 3 s;
  - multiorch's first recruitment, about 1 s after the kill, races the two F3a heals;
  - a follower that resumes in time accepts a revocation while the other is still paused, which triggers the ≥ 10 s backoff.
- **F4a on both followers**: each WAL write and fsync on the standby takes 200 ms. The walreceiver falls behind, so the in-flight WAL may still be unwritten when the elected follower is promoted and its walreceiver is stopped.
- **Keep D3 and record M3 UNDETECTED**: the owner approved D3′.

## Consequences

- **Contract 1.0.10:** §9.2 M3's directed schedule is D3′; §15 K5 (deliverables, K5.4, the buffering rejection condition) names D3′.
- **D3 is superseded, not deleted.** Its ten runs (M3 2/5, real 5/5 PASS), their minimizations and replays stay in `evidence/K5/` unedited. cpgate still checks them and reports them next to D3′; the `D3` campaign stays in the plan generator for `campaigns/k5.json`.
- **K5 runs made under contract 1.0.9 stay K5 evidence** except as D3′ runs, as ADR-0027 did for K4: contract 1.0.10 changes only M3's directed schedule, as a new campaign.
- **Predictions** are committed before the first D3′ run (`evidence/K5/predictions-D3p.md`), in their own commit; every D3′ run's manifest names a later commit.
- **ADR-0031:** accepted with this composition; its status points here.

## Sources

CONTRACT §6.2, §8 (F2a, F7), §9.1, §9.2, §10.2, §10.3, §12.4, §15 K5, P01, P14, P18; S14, S21; ADR-0007, ADR-0027, ADR-0029, ADR-0030, ADR-0031; records `d3-diagnosis`, `k3obs` (evidence/K5/commands.jsonl); evidence/K3/NOTES.md (F7 confirmed ≈ 0.6 s, healed ≈ 0.13 s); evidence/K5/NOTES.md §5.
