# ADR-0024 — K4 schedules, directed-schedule timing, the probe checker and minimization details, fixed before any K4 run

Status: Accepted (K4, contract 1.0.5). Clarifies CONTRACT §9.2 (D2), §3 G7 and §10 (probe checker), §12.2 and the §15 K4 campaign table. Written and committed before any directed run of K4.

## Context (gaps)

The contract fixes the K4 campaigns' workloads and fault pools (§15 K4), the directed schedules' fault sequences (§9.2, §9.4), the probes P1/P2 (§15 K4) and the minimization order (§12.2), and leaves the rest to the implementer (§0: anything not specified is the implementer's choice). Five points cannot be left implicit because a reader must be able to see that they were not chosen after looking at results (CONTRACT §9.1, ADR-0007: a directed schedule changed after results needs an ADR):

1. **D2's heal timing.** §9.2 says "F5a → F3a on the other two followers → 40 s of W4/W1 load → F2a + F2b on the leader → heal F3a → recovery → final reads". It does not say when the leader's PostgreSQL (F2a) and pooler (F2b) come back, nor which node F2b targets once F2a has killed the leader's PostgreSQL (a role resolved again after F2a may already name a new leader). §8's default composite heals in reverse injection order, which would heal F2b and F2a *before* F3a — the opposite of §9.2.
2. **Campaign shapes**: how many faults per run, holds, which roles.
3. **G7's checker**: §3 names "probes P1, P2" as the checker; §10 defines no anomaly classes for it, and verdicts must come from `cpcheck` (§5.3).
4. **§12.2 details**: which class a FAIL is minimized for, how an INVALID candidate counts, what "remove faults" ranges over.
5. **C7's client behaviors**: when statement_timeout is set, which operations get the CancelRequest.

## Decision

### Plan extensions (all in plan.json, so a plan stays a pure function of its inputs, §6.3)

- An element may carry `hold_ms`: the nemesis waits that long after the element is confirmed before injecting the next one.
- A step may carry an explicit heal sequence `heals: [{element, delay_ms}]` replacing the default (reverse injection order, no delay).
- A step may be anchored `after: "new-leader"`: it starts `delay_ms` after a pooler reports itself leader of a decided rule newer than the rule its previous step resolved against (deadline 60 s, the F6 confirmation deadline; not observing one ends the run INVALID with the element `not-applied`).
- Roles: `random-node` (any running node); `base-follower:<k>` — the k-th (sorted) of n1–n3 other than the leader: D2's "other two followers", identical under every build whatever the recorded cohort says (M2 changes the recorded cohort); `same:<j>` — the node element j of the same step resolved to; `followers` inside an F1b set — every running follower of the leader's cohort.

### Directed schedules (§9.2, §9.4)

- Directed runs use seeds 1–5, for the mutant and for the real build alike (P14).
- **D1** exactly as §9.2: W1 + W4 clients only, F1c on the leader at t = 10 s, held 30 s, heal.
- **D2**: W1 + W4; one step of five elements: F5a(n4) [admission predicate of §8], F3a(base-follower:0), F3a(base-follower:1) held 40 s, F2a(leader), F2b(same:3). Heal sequence: both F3a immediately after F2b is confirmed ("heal F3a"), then **30 s later** F2b (`docker start` of the old leader's pooler), then F2a (its PostgreSQL restarts once its pooler runs; §8's manual start after P28 otherwise). The 30 s keep the old leader down while multiorch fails over: S21's grace period is 4 s + up to 8 s of jitter, and the K3 runs observed new leaders 2.7–12.9 s after a leader kill (evidence/K3/NOTES.md §10). This models S41's scenario as written ("the primary crashes, orchestrator B fails over"), where the crashed primary does not come back inside the failover; it does not make the mutant easier to detect than that failure mode warrants. F2a is injected before F2b, in the order §9.2 lists them.
- **DH1**: W1 + W4, F3a on the leader at t = 10 s for 120 s, `kill -CONT`; workload P04 (90 s), so the pause outlasts the workload, as in S51 where the thaw came after the writes.

### Campaign shapes (§15 K4; P04 90 s; first fault at 10 s, then P05 gaps)

| Campaign | Faults per run (drawn from the plan's nemesis stream) | Holds |
|---|---|---|
| C1 | 4 steps, kind uniform in {F2a, F2b, F2c, F2d}; F2a/F2b on `leader` or `random-node`; F2c `random-orch`; F2d `random-gw` | F2a 0 (the kill is the fault); F2b–F2d 5 s |
| C2 | 3 steps, kind uniform in {F1a, F1b, F1c, F1d}; F1a on `leader` or `random-follower`; F1b A = {`leader` or `random-follower`, `random-orch`}, B = rest; F1c, F1d on the leader | 10 s or 30 s |
| C3 | 3 steps: F3a on `leader` or `random-follower`, or F3b on `pooler:leader`, `random-orch` or `random-gw`; W1 serializable on odd seeds, repeatable-read on even seeds | P16 short (3 s) or long (60 s) |
| C4 | per seed s: admission (s−1) mod 3 = F5a / F5a→F2a(leader) / F5a→F3a(base-follower:0)→F3a(base-follower:1)→F2a(leader); removal (s−1) mod 2 = F5b / F1a(leader)→F5b. Seeds 1–3 cover every K4 sequence | 5 s per step |
| C5 | 3 composites F4a(`leader` or `random-follower`) → F2a(leader) | 10 s |
| C6 | F2a(leader); F2a(leader) 3–8 s after the new leader is observed; repeated (4 steps) | 0 |
| C7 | 2 composites: F1b with A = `followers`, B = rest, held 15 s, then F2a(leader); W4 sessions `SET statement_timeout = 2000` (P19) on every new connection; W1 clients send a CancelRequest when COMMIT has been outstanding 1 s | 0 after F2a |

### Probe suite and checker P (G7)

`commitproof probe --suite claims [--mutant M<n>] [--seed s] --out runs/` is a run (sealed, §5.4) with one probe client per gateway issuing P1 (`SET synchronous_commit = off`), P1-show (`SHOW synchronous_commit`, diagnosis only) and P2 (`ALTER SYSTEM SET work_mem = '8MB'`), then the usual recovery and final reads. `cpcheck` adds checker **P** after C1–C9: P1 completed ok → `p1-not-rejected`; P1 rejected with a SQLSTATE other than 0A000 → `p1-wrong-sqlstate`; P2 completed ok → `p2-not-rejected` (any SQLSTATE is a rejection); a probe without a SQLSTATE in its outcome (timeout, transport) or missing → `unknown`. Runs without probes are vacuously valid; the K2 corpus checkers C1–C9 are unchanged.

### Minimization and replay (§12.2, §11.1)

- Target class: the directed schedule's expected class when the run shows it (D1, D2, DH1 → `lost-write`; the probe suite → `p1-not-rejected`), else the first class of the first invalid checker in checks.json order. Replays reproduce iff they are FAIL with that class.
- "Over nemesis steps": the unit is a plan step (a composite is one step, §8). The empty schedule is tried first (a failure without faults is the most informative minimum), then Zeller's ddmin (subsets, complements, increasing granularity) down to 1-minimality. A step that becomes first takes P05's first-fault anchor.
- Then the duration is halved while ≥ 10 s (client operations truncated to the rounds that fit), then the clients are reduced to those whose operations appear in the target class's anomalies plus the lowest-numbered other client.
- A candidate is one run; an INVALID candidate is re-run like any run (§12.1) and, INVALID three times, counts as not keeping the failure (the larger plan is kept). The minimized run is the last candidate that kept the failure (the original if none did); its plan.json is replayed five times (P13), each replay a valid run.
- `commitproof campaign` is resumable: a cell, candidate or replay already present as a sealed run (same plan bytes, profile, build and provenance) is reused, never recomputed; verdicts come from `cpcheck check` run as a subprocess.

## Alternatives rejected

- **Heal D2 in default reverse order.** Brings the old leader back before its followers resume, contradicting §9.2's order.
- **D2 without a leader-down interval (heal F2b/F2a right after F3a).** The old leader's pooler would restart PostgreSQL within ~2 s (K3 F2a heals: 1.8–2.9 s) and could be re-recruited with all its WAL; that is not S41's crashed primary.
- **Resolve F2b's "leader" again after F2a.** A failover may already be under way; F2b could kill the *new* leader.
- **A K2-style corpus for checker P.** P has three classes over single-statement outcomes; hand-written unit cases from the G7 text cover them (internal/checker/probes_test.go); the K2 gate is untouched.
- **Minimize over individual composite elements.** §12.2 says "over nemesis steps"; element-level removal would also break composites' meaning (§8).

## Consequences

- Contract 1.0.4 → 1.0.5: §3 G7 names checker P; §9.2 D2 states the heal timing; §10.12 defines P; §5.6 lists the optional flags (`images build --profile`, `probe --seed`, `--cpcheck`); §15 K4 points to this ADR for the shapes.
- K1–K3 plans are byte-identical (the new fields are emitted only when used).

## Sources

CONTRACT §3, §6.3, §8, §9, §10, §12, §15 K4; S11, S21, S41, S51; evidence/K3/NOTES.md; ADR-0007, ADR-0009.
