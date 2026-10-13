# ADR-0031 — D3′: hold commits in synchronous replication when the leader is killed (proposed amendment of CONTRACT §9.2 D3)

Status: **Proposed (K5) — not applied.** K5 stops BLOCKED on K5.4: M3 via D3 FAILs with an expected class in 2 of 5 runs (P14 needs 4). Changing a directed schedule after seeing mutant results needs the owner (CONTRACT §9.1, §15 K5 rejection conditions, §18; ADR-0007); the contract, D3, the checkers and every threshold are unchanged. This ADR records the investigation and one proposal; the owner decides.

## Context (what the K5 runs show)

§9.2 models M3 (`gateway-retry-any`, S14) as: `classifyError` buffers and retries every error on PRIMARY targets, "including errors after execution began" — an unsafe retry that executes a statement twice. D3 (profile `buffered`): W1 autocommit-append variant and W4 at 2× pacing; F2a on the leader three times, 15 s after each heal (ADR-0030). Expected detection: C3 `duplicate-elements` or C2 `g1a`.

Evidence (evidence/K5/runs, campaign records `campaign-k5-1`, `campaign-k5-2`):

- M3 on D3, seeds 1–5: PASS, FAIL `g1a`, PASS, FAIL `g1a`, PASS → **2/5**. The real build on D3: recorded in evidence/K5/results.json.
- Mechanism, per kill (tool output in NOTES §5; diagnosis only): after F2a's `kill -9` of the postmaster, the statements already executing keep completing `ok` for ~30–40 ms (a backend notices the postmaster's death only at its next wait); statements arriving later fail without having executed (`57P03`; absent from the final state; "no writable primary" is an error the real build buffers and retries too). Only a statement whose commit is in progress when its backend exits gets an error *after* execution — the one case in which M3 differs from the real build. In each detecting run exactly one statement hit that window, at the first (buffered) kill: seed 2 an autocommit append sent 39 ms after the kill, seed 4 a ledger insert sent 40 ms after it; each committed on the old primary (present in the final state), M3's retry ended in "no writable primary" and the client saw `fail` → C2 `g1a`. With ~6 statements of 3–8 ms in flight per 25–50 ms, three kills per run hit that window in about 2 of 5 runs.
- **§9.2's first expected class is unreachable in D3.** C3 judges reads inside W1 transactions only; final reads are not C3 inputs (§10.3, LIM-22). D3's W1 variant issues autocommit appends and no reads, so an append executed twice — M3's duplicate, when the retry succeeds — appears only in final reads, which C1/C2 do not judge for duplicates. (In the ten D3 runs no final read held a duplicated element.) Only C2 `g1a` can detect M3 in D3.

## Proposal (D3′, one amendment)

D3′ (profile `buffered`; workloads, pacing, seeds 1–5, holds and the 15 s gaps of D3 unchanged): each of the three steps becomes **F1b(A = the followers' nodes, B = the rest) held 5 s, then F2a(leader)**; heals: F1b immediately after F2a is confirmed, then F2a (PostgreSQL restarts by itself; §8's manual start otherwise). While F1b holds, every commit on the leader waits in synchronous replication (as in C7 and D5), so when the postmaster dies each client's statement is in progress after execution: its commit record is flushed locally and survives the restart on the same node (the followers are unreachable for the orchestrators, so no failover can elect them meanwhile), and its client gets an error after execution.

Predictions, to be committed before any D3′ run (as for D2′, ADR-0027):

- M3 on D3′: FAIL with C2 `g1a` in ≥ 4 of 5 runs — M3 retries the interrupted W4 inserts once the restarted primary serves again; the retry fails with `23505` (or "no writable primary") while the first execution's row is present.
- Real build on D3′: PASS in 5 of 5 — the interrupted statements end `info` (an error after `COMMIT` was sent or a transport error, §6.2 rows 8/12), present or absent, both allowed.

Correction proposed with it (contract defect, independent of the schedule): §9.2 lists for D3 only the class D3's workloads can show, C2 `g1a`; `duplicate-elements` stays the class of an M3 retry observed by a read, which D3's workloads do not issue.

Why this does not make detection easier than the failure mode warrants: the unsafe retry matters exactly when a statement fails after it began executing; a primary crash while commits wait for the standbys (the usual state of a busy synchronous primary) is that situation, whereas a postmaster kill at an arbitrary instant on an idle-fast workload reaches it only by a millisecond coincidence. Thresholds (P14: ≥ 4/5), repetitions and the detecting class are unchanged.

## Alternatives

- **Keep D3 and record M3 as UNDETECTED** (K5.4 then fails permanently for M3; the owner accepts it with a LIMITATIONS entry): the evidence supports that D3 reaches M3's defect only by coincidence.
- **Raise the W1/W4 pacing** so that more statements are in flight at each kill: changes the workload, and still leaves detection a matter of millisecond timing.
- **Add autocommit reads to D3's W1 variant** so that C3 can see a duplicate append: makes `duplicate-elements` reachable, but M3's successful retries are rare under F2a for the same timing reason.

## Consequences if approved

As ADR-0027 for D2′: contract 1.0.10 (§9.2 D3′ and its expected class); predictions committed before the first D3′ run; M3 and the real build run D3′ five times each (seeds 1–5); the D3 runs stay recorded as the superseded schedule and cpgate keeps checking them; one amendment only — if M3 is not detected in ≥ 4 of 5 D3′ runs it is recorded UNDETECTED and K5 stops BLOCKED on K5.4.

## Sources

CONTRACT §6.2, §9.1, §9.2, §10.2, §10.3, §12.4, §15 K5; S14, S15, S61; ADR-0007, ADR-0027, ADR-0029, ADR-0030; evidence/K5/NOTES.md §5.
