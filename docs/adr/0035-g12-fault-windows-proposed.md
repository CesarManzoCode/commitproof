# ADR-0035 — G12's ok fraction excludes operations invoked inside fault windows (proposed clarification of CONTRACT §10.9, for after K5)

Status: **Proposed (K5) — not applied.** Written at the owner's request (decision of 2026-10-14, item 3) for the owner to consider after K5. K5 keeps G12's literal rule, C11 seed 2's FAIL and its §11 classification (`PROTOCOL_DEVIATION`, severity `medium`, FND-C11-17a344c1). Nothing in this ADR changes a K5 verdict, a checker or the contract.

## Owner decision (verbatim)

> 3. C11 seed 2 / G12: keep the literal rule, the FAIL, and severity "medium" as the contract maps it. Add a finding or note that all refused operations fall inside the injected F2a window, with a proposed contract clarification for the owner to consider after K5. Do not apply it to K5 verdicts.

## Context

- **G12** (§3; S40, S22): under a connection storm by one role, another role with steady demand still obtains connections. Overload alone does not trigger a failover.
- **C8's rule** (§10.9; kept literal by ADR-0030; LIM-25): over each storm window (the first invoke to the last completion of one phase's storm operations), ≥ 90 % of the `cp_steady` operations invoked in it complete `ok`. Every storm window counts, also one a nemesis fault overlaps.
- **C11** (§15 K5; ADR-0030): even seeds inject F2a(leader) at t = 10 s, inside the first storm phase. That is the campaign's "one run with F2a during a storm".

**What C11 seed 2 shows.** Run `20261013T232750Z-C11-2-real-1f5423ed`, record `c11-window-check` (stdout sha256 `203d4d24…7f3f`; times from workload start):

| | Storm window 1 (5.00–27.49 s), with the F2a window 10.12–13.84 s inside it | Storm window 2 (45.00–67.05 s), no fault |
|---|---|---|
| `cp_steady` operations invoked | 886 | 872 |
| ok | 766 (0.865) | 872 (1.0) |
| non-ok | 120: all invoked inside the F2a window (10.17–13.25 s) and completed inside it | 0 |
| ok fraction of the operations invoked outside every fault window | 744 of 744 (1.0) | 872 of 872 (1.0) |

The 120 are 114 connection refusals with `57P03`, 4 with `28P01` (a valid role, while PostgreSQL restarted) and 2 `57P01` after the statement was sent (triage record). They are the unavailability of the failover the plan injected, not unfair sharing of the pool: outside the fault window the steady role completed everything, as it did in both storm windows of seeds 1 and 3.

G10's judgement already excludes fault windows. C8 judges a NOTIFY only if no fault window overlaps its interval (ADR-0030). Misses inside reconnect windows are the observation `notify-window-loss`, by the K5 rejection condition. G12's rule has no such exclusion, so a campaign whose definition puts a failover inside a storm cannot separate fairness from availability.

## Proposal (not applied)

1. **§10.9, G12:** the ok fraction of a storm window counts only the `cp_steady` operations invoked outside every fault window (first to last nemesis event of a fault id, as for G10). A storm window with fewer than one judged operation per second of storm outside fault windows is reported as not judged (an observation), never as PASS.
2. **New observation `overload-fault-overlap`:** the operations invoked inside fault windows during a storm, with their completions. It is measured, not judged, as `notify-window-loss` is for G10.
3. **§11:** a G12 failure under the clarified rule stays `PROTOCOL_DEVIATION` (§11.1 maps G12 so). The owner decides whether runs judged under the literal rule are re-classified. Their sealed histories would be re-checked by `cpcheck` (no re-run), and the original verdicts stay recorded.

If approved after K5, it needs:

- an ADR applying it with a contract PATCH (a clarification that does not relax the fault-free judgement);
- corpus cases written from the rule: a fault-overlapped storm that the literal rule fails and the clarified rule passes, and an unfair storm outside faults that both fail;
- a sabotage rule for the exclusion.

## Alternatives

- **Keep the literal rule** (what K5 does): a failover inside a storm always counts as unfair sharing.
- **Exclude whole storm windows that a fault overlaps:** this loses the fairness evidence of the rest of the window (744 operations in C11 seed 2).
- **Lower the class to `LIVENESS_OBSERVATION`** without changing the rule: a classification would then contradict its checker; the owner kept `PROTOCOL_DEVIATION`.

## Sources

CONTRACT §3 (G12), §10.9, §11.1, §11.2, §15 K5; S22, S40; ADR-0030; LIM-25; record `c11-window-check` (evidence/K5/commands.jsonl); evidence/K5/triage/20261013T232750Z-C11-2-real-1f5423ed.json.
