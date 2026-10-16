# ADR-0045 — K7 under P25: a real-build failure that repeats a recorded finding stays a summary; the repeat predicates, the store step and the D3′/D5 decision

Status: **Accepted by the owner (K7, contract 1.8.0).** Applies the owner's decisions 1 and 2 of 2026-10-15 on K6b's open items (option 1 of `evidence/K6b/k7-estimate.md` §4). Written and committed before any K7 run, before the matrix file's change and before the code that evaluates it.

## Owner decisions (verbatim)

> 1. P25 storage: Option 1 of evidence/K6b/k7-estimate.md, recorded as a new ADR committed BEFORE any K7 run. A real-build failure that repeats a recorded finding is counted in k/N and in every interval, cited, kept as a summary, and NOT minimized or kept in full. "Repeats" is defined mechanically in the ADR: same guarantee, same set of anomaly classes, and a per-mechanism predicate over the checker output and the fault kinds involved, one predicate for each recorded finding (G10 notification stream not resumed, G2b reads of unacknowledged writes that vanish after failover following a cancelled synchronous-replication wait, G6, G12). A failure that matches no predicate is always minimized, replayed 5× and kept in full as a new finding. The closure lists every repeat with the fields that matched, so the owner can audit them. After each store step measure evidence/ size; if the projection exceeds 250 MB, stop BLOCKED with the number of new findings and the options, and do not drop anything silently.
>
> 2. The 11.8 MB of K5 D3′ and D5 real-build runs in full: decided at the end. If, after K7's matrix and findings, evidence/ stays within 250 MB with them, commit them in full; otherwise those two cells keep the "runs held in full: 0 of 5" wording.

## Context

- Under ADR-0039 a K7 run that a finding package names as its artifact is committed in full, and §11.3 gives every minimized failure its own finding id. With K4's and K5's failure rates that ends near 277 MB (`k7-estimate.md` §4, S1), most of it repeats of mechanisms already recorded: C9's G10 failures (deterministic, K5 3 of 3) and C2's vanish failures (G2b, G6).
- Nine findings are recorded (K4, K5, K6b): FND-D1-fb8d0f8c, FND-D1-2e860fd2, FND-D1-a8eb9857, FND-C2-2a534585 (G2b by ADR-0042, `evidence/K6b/reclassification.md`); FND-C2-6806aeca (G6, ADR-0043); FND-C9-40b6f05a, FND-C9-2fe9a9fc, FND-C9-90da5532 (G10, KNOWN); FND-C11-17a344c1 (G12). They fall into four mechanisms, the four the owner names.
- What a summarized run keeps in git is its stub (manifest, plan, seal) and its `checks.json` (ADR-0039). A predicate that cpgate must re-evaluate for every repeat can therefore read only `checks.json` — the checkers' anomalies (cpcheck lists at most 20 per class, `stats.counts` holds every class's total) and their stats, among them C9's `exposure.confirmed_faults` for plans of contract 1.7.0 or later (ADR-0044).

## Decision

### 1. What a repeat is

A K7 matrix run (a valid primary run of V1, V2, V3 or V5 on the real build) whose verdict is FAIL is split into **guarantee groups**: every class with a nonzero total in `stats.counts` of a checker whose `valid` is `false` is put in the group of its guarantee (table below); a class not in the table is a group of its own, which never matches. A group **matches** predicate R when its guarantee is R's, its set of classes is exactly R's set, and every condition of R holds. The run is a **repeat** when every one of its groups matches a predicate; otherwise it is a **new failure**.

| Class (checker) | Guarantee group |
|---|---|
| `incompatible-order` (C3) | G2b (ADR-0042; one from an `ok` write is also C1 `lost-write`) |
| `non-linearizable` (C4), `stale-read` (C6) | G6 |
| `notify-not-resumed` (C8) | G10 |
| `fair-share-starvation`, `overload-failover` (C8) | G12 |
| `session-setting-lost`, `prepared-statement-lost` (C8) | G9 |
| `lost-write` (C1) | G1 (G1 or G2a by triage, ADR-0042) |
| `final-read-divergence` (C1) | G2a |
| `g1a` (C2) | G4 |
| `bank-sum`, `bank-negative` (C5) | G13 |
| `cdc-gap`, `cdc-phantom`, `cdc-duplicate`, `cdc-order` (C7) | G11 |
| `planned-failover-error` (C9) | G8 |
| `p1-not-rejected`, `p1-wrong-sqlstate`, `p2-not-rejected` (P) | G7 |
| any other class (C3's other classes among them) | its own group |

Notation for the conditions, all read from the run's `checks.json`: *valid(C)* is checker C's `valid`; *listed(class)* the anomalies of that class cpcheck listed; *timeouts* is C9's `stats.errors["info/timeout"]` (0 if absent: completions `info` because the client's P01 deadline expired, §6.2); *F* is the set of kinds with a count ≥ 1 in C9's `stats.exposure.confirmed_faults` (a run without that field — a plan before 1.7.0 — has no F and matches no predicate).

### 2. The predicates, one per recorded mechanism

**R-G2b — reads of unacknowledged writes that vanish after a failover following a cancelled synchronous-replication wait.** Stands for FND-D1-fb8d0f8c, FND-D1-2e860fd2, FND-D1-a8eb9857, FND-C2-2a534585.
1. guarantee G2b, classes exactly {`incompatible-order`};
2. every listed `incompatible-order` anomaly reads "reads of key *k* are not prefixes of each other: [*a*…] (i=*x*) and [*b*…] (i=*y*)" with *x* < *y* and at least one element of [*a*…] absent from [*b*…] — a value an earlier `ok` read returned is absent from the key's longest read, which came later (it vanished; not a reordering);
3. valid(C1) and valid(C2) are `true` — no acknowledged write was lost and no failed write is visible, so the vanished values came from writes whose completion was `info`;
4. timeouts ≥ 1;
5. F1c ∈ F (the leader cut from orchestrators, followers and etcd but not from the gateways: the fault of every recorded run of this mechanism).

**R-G6 — the same vanish on registers.** Stands for FND-C2-6806aeca.
1. guarantee G6, classes exactly {`non-linearizable`};
2. valid(C6) is `true` — no offending read is a stale read from a superseded primary (§10.6), the recorded finding's C6 result;
3. valid(C1) and valid(C2) are `true`;
4. timeouts ≥ 1;
5. F1c ∈ F.

**R-G10 — the LISTEN notification stream is not re-established.** Stands for FND-C9-40b6f05a, FND-C9-2fe9a9fc, FND-C9-90da5532.
1. guarantee G10, classes exactly {`notify-not-resumed`};
2. every listed `notify-not-resumed` anomaly reads "… by listener client *c*, whose session (LISTEN i=*l*) was alive and past its first successful op after recovery", and each listener client *c* appears with one LISTEN index *l* only — the listener kept the session it opened, so nothing re-established its stream (the recorded mechanism: the gateway rebuilds a session's stream only on LISTEN or UNLISTEN);
3. F ∩ {F2a, F2b, F6} ≠ ∅ (a kill of the leader's PostgreSQL or pooler, or a switchover: the kinds of the recorded runs).

**R-G12 — cp_steady failures only inside a fault window.** Stands for FND-C11-17a344c1.
1. guarantee G12, classes exactly {`fair-share-starvation`};
2. for every storm window of C8's stats whose result is `fair-share-starvation`, the fraction of `ok` operations among those invoked outside fault windows (`judged_ok`/`judged_total`) is at least 90 % and the literal fraction (`literal_ok`/`literal_total`) below 90 % — the recorded run's figures (literal 766 of 886, outside fault windows 744 of 744);
3. F2a ∈ F.

The recorded run's own `checks.json` (a plan of 1.0.9, judged by the literal rule) carries no per-window fractions; its figures come from its triage (`evidence/K5/triage/`). Condition 2 of R-G12 holds for no run of a plan of contract 1.2.0 or later: from 1.2.0 C8 reports `fair-share-starvation` only when the fraction outside fault windows is below 90 % (ADR-0037). So **every G12 failure of K7 is a new failure**; the predicate is stated so that the mechanism, not the class name, decides.

The predicates are fixed here, before any K7 run; they are not tuned to K7's results. They read only `checks.json`, so `cpgate K7` re-evaluates every one for every FAIL run, committed in full or as a summary.

### 3. What happens to each

- **Repeat:** counted in its cell's k/N and every interval and bound (§12.3), as any FAIL; triaged (§6.2 completion review on the local full archive, the oracle for C3 per ADR-0046, classification per §11 with G2a/G2b per ADR-0042); **cited** by the packages of the findings its predicates stand for (a "recurrences in K7" list: run id, seal root, cell, the matched fields), and **committed as a summary** (stub and `checks.json`, a row of `evidence/K7/runs-index.json`); **not minimized**, not replayed, not kept in full in git. Its full archive stays on the machine that ran it (`runs/k7/`, as `runs/k4-full/` and `runs/k5-full/`). ADR-0039's "a run a finding package names as its artifact is committed in full" does not apply to a recurrence: the package's artifacts are the recorded finding's runs; the recurrence is named with its summary.
- **New failure:** minimized (§12.2) for the first class, in `checks.json` order, of its first group that matches no predicate; its minimized plan replayed P13 = 5 times; a new finding (§11.3 id, §11.4 package), whose original and minimized runs are committed **in full**; its minimization candidates and replays are summaries. A run with a matched group and an unmatched group is a new failure (in full); its matched group is also listed as a repeat.
- The **closure** lists every repeat with the fields that matched (§19 `findings` carries the new ones; the repeats are a list beside it): run, cell, campaign, profile, seed, seal root, matched predicate(s) and finding ids, and per group: the class set and totals, and the values of each condition (the vanished elements and read indexes of R-G2b; valid(C1), valid(C2), valid(C6); timeouts; F; the listener clients and LISTEN indexes of R-G10).

### 4. The matrix file and the driver

`campaigns/k7.json` keeps `minimize_failures: true` and adds `skip_repeats: true`: after every cell has its valid run, `commitproof campaign` evaluates each FAIL with the predicates above (package `internal/repeats`, the code cpgate uses) and follows up only new failures, with the target class above; for a repeat it records an event with the matched fields. A dry run shows the setting.

### 5. The store step and P25

- K7's store step (`scripts/k7-store.sh`) writes each run of `runs/k7/` once: in full if it is the original or the minimized run of a new finding, otherwise as a summary; it refuses (exit 2, nothing written) when the result would make the P25 count (every byte under `evidence/` plus every run archive under `findings/`) exceed 250 MB.
- **After every store step** it measures that count and a **projection**: the count + the summaries of the K7 runs not yet stored (each estimated by the measured mean of K7's stored summaries, or 95 KB before there is one, `k7-estimate.md` §4) + 5 MB for K7's remaining records and text (`k7-estimate.md` §4 item B, upper end). If the projection exceeds 250 MB, K7 stops **BLOCKED** with the number of new findings, the measured sizes and the owner's options; nothing already stored is dropped or moved to a summary to make room.
- **D3′ and D5 (decision 2):** at the end — after the matrix, its follow-ups, the packages' replays and every other K7 store — the cited D3′ and D5 real-build runs (10 runs, ~11.8 MB, `runs/k5-full/`, each archive's manifest, plan and seal byte-identical to K5's stub) are committed in full under `evidence/K7/cited/` only if the count stays within 250 MB with them; then those cells get their exposure from the histories and, if clean, the §12.3 statement. Otherwise they keep "runs held in full: 0 of 5" without the clean template. K5's stubs are not touched either way.

### 6. Exposure of summarized runs (owner decision 3, ADR-0044 accepted)

ADR-0044 is accepted by the owner. Every README or report statement that uses the exposure of a summarized run says it is cpcheck-reported from the sealed history and re-derivable from the full archive, which is not in git; cpgate K7 recomputes the exposure independently from the history of every run held in full and compares it with its `checks.json`.

## Alternatives rejected

- **Option 2, P25 raised to ~300 MB; option 3, only minimized runs in full; option 4, fewer C9 runs:** the owner chose option 1.
- **Predicates on the history** (e.g. the completion of the vanished values' writes, notifications received after recovery). A summarized run's history is not in git, so cpgate could not re-evaluate them; the triage of each repeat still reviews the history on this machine.
- **"Same guarantee and class" without a mechanism predicate.** Would count a different mechanism of the same class (a stale read from a superseded primary; a listener that reconnected and still misses notifications; starvation outside fault windows) as a repeat.
- **Predicates per campaign.** C12 can reach the recorded mechanisms (it draws F1c, F2a, F2b, F6); the predicates follow the mechanism wherever it appears.

## Consequences

- Contract 1.7.0 → 1.8.0 (MINOR, owner-approved change): §13 (K7's storage of repeats, the projection after each store step, D3′/D5 at the end), §15 K7 (the matrix's repeats), P25's row unchanged.
- A repeat has no minimized plan and no reproducibility class of its own; the recorded finding's classes stand. The packages state how many K7 runs repeated each mechanism.
- K7's plans carry 1.8.0; nothing else in them changes.

## Sources

CONTRACT §3, §6.2, §10.3, §10.6, §10.9, §10.10, §11, §12.2, §12.3, §13, §15 K7, P13, P25; ADR-0024, ADR-0030, ADR-0036, ADR-0037, ADR-0039, ADR-0042, ADR-0043, ADR-0044; evidence/K6b/k7-estimate.md, readiness.md, reclassification.md, findings/FND-C2-6806aeca.json; evidence/K4/triage/, evidence/K5/triage/ (the recorded runs' checks.json satisfy their predicates: R-G2b's conditions on D1 seeds 1, 3, 4 and C2 seed 3; R-G6's on C2 seed 3; R-G10's on C9 seeds 1–3; with F from each plan, every fault of a valid run being confirmed).
