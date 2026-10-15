# K6 NOTES — Observability and reproducible report

Contract **1.1.1** (ADR-0036, a PATCH clarification committed as `bf9461f`
before any K6 evidence render). Evidence produced by `scripts/k6-evidence.sh`
(phases setup, render, fresh, ci, gate), every step recorded in
`commands.jsonl` (argv, exit code, stdout/stderr sha256), 2026-10-15
23:19:20Z onwards, on clean committed trees (the `build` record's ldflags
carry `Dirty=false`). No `claims.yaml` exists yet (K7): every statement names
its record, file or run; "design says" cites RESEARCH or contract ids,
"system did" cites the sealed reference run (observations explain, never
judge, ADR-0016).

**Status: DONE** — see §1.

## 1. Gate

`cpgate K6 --evidence evidence/K6 --negative-controls`: record `cpgate-k6`
(2026-10-15 23:40:41–23:40:44Z), **exit 0**, stdout sha256
`7a37134223253ffb93e6f8a798c63db9c0a48983067ea10e6b12547a876bd5e7`;
`gate.json` sha256 `560a299f…197b1c`, `negative-controls.json` sha256
`01fe4d78…729ebe` (`negative_controls_ok: true`).

| Criterion | Result | Basis |
|---|---|---|
| K6.1 Byte reproducibility | PASS | records `render-1`, `render-2`, `render-3`: one output each, sha256 `35dc3e48…c94c`, equal to the committed example `report/20261011T231036Z-C2-3-real-f4505b54.html`; the sabotaged build's three renders (`sabotage-render-1..3`) have 3 distinct hashes (§5) |
| K6.2 Forced rebuild | PASS | record `fresh-clone`: `git clone` of `2927f46` (the commit whose clean `make build`, record `build`, made the renders), `go clean -cache -modcache` exit 0 with both caches empty after it, build and render exit 0, render sha256 `35dc3e48…c94c` = the example's |
| K6.3 Completeness | PASS | the 7 862 history events of the reference run in 3 946 elements, each index exactly once (3 916 operations as invoke + completion, 12 nemesis and 18 marker events alone); 12 fault phase elements with their fault, kind and phase; the 3 observed rules and the 1 leader change cpgate derives from the observations; the 4 checks.json anomalies, each op index linked to the element holding it |
| K6.4 Seal enforcement | PASS | record `seal-enforcement`: a run directory with one byte of `history.jsonl` flipped and the same directory re-archived: cpreport exit 2 ("seal does not verify"), no output directory created, cpcheck verify-seal invalid on both; the untampered control renders one file |
| K6.5 Failover metrics | PASS | the data block's metrics of the 3 faults equal cpgate's own computation from the raw history and observations, field by field (windows, gaps and non-ok counts per gateway; n, p50, p95, p99 before/during/after; rule and leader changes and the rules' identities) |
| K6.6 CI-equivalent | PASS | record `ci`: `make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke ci-mutant-smoke ci-report` exit 0 on `2927f46` (23:24:22–23:40:31Z), the first attempt; `lint-workflow` exit 0 |

**Negative controls** — each failed its own criterion and no other:

- K6.1 (checker-level, `cp_sabotage_report_nondeterminism`): the three render
  records replaced by the sabotaged build's renders → "the 3 renders produced
  3 different outputs", and the example is not their output;
- K6.2: the fresh-clone render's recorded hash altered → it differs from the
  K6.1 output;
- K6.3: one operation's completion index removed from the example, with every
  recorded render and fresh-clone hash rewritten to the forged file's, so K6.1
  and K6.2 still pass → "element e4 holds the invoke event alone", "history
  event i=17 is in no element of the report";
- K6.4: the seal-enforcement record rewritten as if the tampered directory had
  rendered → "want exit 2 and nothing written";
- K6.5: the first `gap_ns` of the data block increased by 1 ns, hashes
  rewritten likewise (K6.3 does not read metric values) → "fault
  f0.gateways.gw-z1.gap_ns: report 19951812936, recomputed 19951812935";
- K6.6: `ci`'s exit code replaced by 1.

## 2. Order of work

1. **Precondition.** A development check at `c994767` (K5's closing commit,
   before any change) ran `cpgate K1`…`K5` on copies of the closed
   evidence: every criterion PASS. The recorded precondition,
   `k6-precondition` (exit 0, 23:19:21–23:21:14Z), repeated it with this
   sprint's binaries: K1–K5 PASS. K6 changed two things earlier gates use:
   the run-archive reader moved from `gate` to `history` (`ac0abf1`, no
   behaviour change), and K5's gate now pins D3′'s runs to the version K5
   closed with (`params.K5ContractVersion` = 1.1.0) instead of the current
   one, which ADR-0036 raised to 1.1.1 (`bf9461f`).
2. **Choosing the reference and the definitions.** Before ADR-0036 the
   sealed K4 runs were inspected with ad-hoc scripts (outside evidence):
   the fault kinds of every real K4 archive's plan (no run has both F2a and
   F1c, §4.1), and the reference candidate's events, rules and candidate
   window definitions — the §10.10 window by completion times, the same
   ending at the first `ok` after the window start, and the longest ok gap.
   That inspection informed ADR-0036's choice (the §10.10 wording kept, as
   C9 measures it, with the gap beside it) and is why the ADR lists the
   alternatives it rejects. ADR-0036 was committed before any recorded
   render; the code followed (`ac0abf1`…`e3b7785`, `2927f46`).
3. **Development renders** (scratch, outside evidence) of K4 and K5 runs of
   every shape — C1, C2, C9, C10, C11 (18 551 operations in 205 lanes),
   D2′ (composite step), DH1 (profile h1: `node-sql` observations, no rule
   model) and a stub (refused, exit 2) — and a complete dry run of the
   evidence flow into `runs/.k6-dry/` (gate: K6.1–K6.5 PASS, each control
   failing its own criterion; K6.6 without a CI record). One renderer
   defect found that way was fixed before the recorded renders (`2927f46`:
   node charts joined points across time without observations).

## 3. What the report shows and how to read it

`cpreport render <run-dir | <run-id>.tar.zst> --out <dir>` writes
`<dir>/<run-id>.html`, one self-contained file (inline CSS and SVG; the only
`<script>` element is a JSON data block, `type="application/json"`, never
executed; no external reference — the SVG namespace URI is a name, not a
fetch). Sections:

- **Run.** Manifest fields, seal root, sealed file count, checks.json
  sha256, history and observation line counts; history events by type;
  operations by workload, function and outcome; observations by source.
- **Timeline** (one SVG, 1 px = 50 ms, integer coordinates). From top:
  markers (dashed lines through the whole chart, one element per marker
  event); faults (label above a band: inject → confirmed amber, confirmed →
  heal red, heal → healed blue; one tick element per nemesis event; a grey
  window inject → healed behind the lanes); the observed leader (the leader
  of the highest decided rule first seen so far); one row per node from the
  pooler status observations (dark blue primary, light blue standby, red
  probe error, dark grey PostgreSQL not running); CDC slot readiness per node
  when present; client error observations as ticks; **one lane per client**
  with every operation as a bar from invoke to completion, green `ok`, red
  `fail` (proven not to take effect), amber `info` (unknown outcome; §6.2);
  then charts: ok completions per second per gateway with non-ok
  completions as bars, WAL position per node, status-probe latency per node.
  Every element has a tooltip (index, client, process, gateway, workload,
  outcome, latency, SQLSTATE).
- **Faults.** Per fault id: kind, targets, every phase with its time and a
  link to its element, and the inject→confirmed, confirmed→heal and
  heal→healed durations.
- **Failover metrics per fault** (ADR-0036; measured, not judged, §3): per
  gateway the §10.10 unavailability window (first `fail`/`info` completion
  at or after the injection → first `ok` completion at or after the heal's
  confirmation; `none` if no failure, `open` if no ok followed), the longest
  interval without an ok completion over [inject − 1 s, healed + 10 s] and
  the non-ok completions in it; ok-latency n, p50, p95, p99 (nearest rank)
  before [inject − 10 s, inject), during [inject, healed] and after (healed,
  healed + 10 s]; the rule changes first observed from the injection to the
  next injection (cascade indicator) and how many changed the leader.
- **Rules and leaders.** Every decided rule observed (term.subterm, leader,
  cohort, first seen, by which pooler) and every leader change.
- **Checkers.** cpcheck's verdict and each checker's validity; every
  anomaly with its explanation and **one link per operation index** to the
  operation's bar (a completion index links to its operation's invoke), the
  checker observations likewise, and each checker's stats.
- **Markers** table and **How to read** (including the absence of `/metrics`
  scrapes, §6).

Reading tips: click an anomaly's operation link — the browser scrolls the
timeline to that bar and outlines it (CSS `:target`). Compare the grey fault
windows with the lanes: a lane that turns amber is a client blocked until
its 10 s timeout (P01); bars that stay green under a fault were served.

`cpreport render <campaign-dir> --out <dir>` writes `<dir>/index.html`: for
each cell (campaign, profile, build) the valid runs N, INVALID runs (listed,
never counted), PASS/FAIL/UNKNOWN, every violation class k/N with its Wilson
95 % interval, the zero-failure bound when k = 0, and the exposure
(operations, ok writes, confirmed faults by kind, fault-time); the §12.3
clean-result sentence for real-build cells whose valid runs all PASS and are
held in full; the runs of each cell with their seal roots; minimization
candidates and replays apart. Statistics come from `internal/stats` in
integer arithmetic (10⁻¹² fixed point, lower ends rounded down, upper ends
up; N = 10 → 0.259 as §12.3 states, unit-tested).

## 4. The reference run

### 4.1 Which run, and the prompt's condition that could not be met

The prompt asks for "one real K4 failover run with at least one fault of
each of F2a and F1c". No real K4 run has both (ADR-0036 §Context 1: the
real runs with F1c are D1 ×5 and C2 seed 3, and their follow-ups; those
with F2a are C1 seed 2, C4 seeds 2–3, C5, C6, C7, D2, D2′; K5's C10 runs
have both but are K5 runs). §15 K6 requires "one real K4 failover", and the
contract wins (AGENTS.md §1): the reference is
**`20261011T231036Z-C2-3-real-f4505b54`** (K4 C2 seed 3, baseline, real
build, three F1c, one failover, cpcheck FAIL with C3 `incompatible-order`
×2 and C4 `non-linearizable` ×2 — FND-C2-2a534585 of K4). Record
`stage-reference` copied its archive (674 734 bytes, sha256
`4635a5fa…193a`) and checks.json (`2a520b5d…d066`) byte for byte from
`evidence/K4/runs/` into `reference-run/`; cpcheck verify-seal on the copy:
valid, seal root `a5472bd9…4969`, the root checks.json names.

### 4.2 What the report shows (system did; observations explain)

- **f0, F1c on leader n1** (inject 10.652 s, confirmed 11.881 s, heal
  41.882 s, healed 42.184 s). n2 reported `PROMOTING` at 35.36 s and rule
  2.0 led by n2 was first observed at **36.349 s, 25.7 s after the
  injection** (the only leader change of the run). n1 reported itself a replica from
  45.85 s and its PostgreSQL not running from 46.34 s, still at the end of
  the run. Unavailability windows (§10.10): gw-z1 31.512 s,
  gw-z2 21.468 s, gw-z3 31.506 s; longest ok gaps 19.952, 10.054 and
  20.052 s. ok latency p50/p95/p99: before 6.0/11.9/14.1 ms (1 445 ops),
  during 6.8/16.0/26.1 ms (233), after 6.3/11.7/15.0 ms (1 381).
- **f1, F1c on the new leader n2** (55.714–87.221 s). No failover: the next
  decided rule, 4.0, again led by n2, was first observed at 102.350 s (n2
  reported its PostgreSQL not running at 99.56 s, then standby, promoting,
  primary). From 55.9 s to the end of the workload (96.3 s) 18 operations
  completed ok through the three gateways, none after 86.3 s, and no ok
  operation was invoked in the 10 s after the heal (phase *after*: n = 0).
  Windows: gw-z1 61.161 s, gw-z2 61.161 s, gw-z3 51.129 s — each ends at the
  recovery writes at 116.9 s, the first operations after the workload, so
  the windows are bounded by when an operation was next issued, not only by
  the system (§6). Longest ok gaps: 21.334, 10.971, 30.053 s.
  Hypothesis, not verified: with n1's PostgreSQL down and n2 isolated from
  n3, no synchronous standby could acknowledge a commit under AT_LEAST_2,
  so writes blocked and no recruitment that excluded n2 succeeded until
  after the heal.
- **f2, F1c on n2** (105.320–116.886 s), after the workload: windows
  `none` (no operation failed through any gateway after its injection);
  the only operations near it are the three recovery writes after its heal
  (phase *after*: 3 ops, p50 20.3 ms).
- **Checkers.** The four anomalies link to their operations: C3 key 8 (i =
  3298, 3591) and key 14 (3298, 3541) — reads at 30.7 s and 41.4–41.6 s,
  inside f0; C4 registers 1 (cas i = 3370) and 4 (read i = 3412), at 40.9
  and 41.0 s. K4 NOTES §7 gives the mechanism.
- The C9 checker's own `unavailability_ms` statistics in checks.json agree
  with the report's windows to the millisecond (C9 truncates, the report
  rounds half up: 21 467 vs 21.468 s); for f2 C9 writes 0 where the report
  writes `none`.

## 5. Hashes of the renders

| Record | Binary | Output | sha256 |
|---|---|---|---|
| `render-1` | `bin/cpreport` (`make build`, `2927f46`, clean) | `report/20261011T231036Z-C2-3-real-f4505b54.html` (841 275 bytes, the committed example) | `35dc3e484d12349a16546967d953001065287c0f015c5ac0a878c1c67118c94c` |
| `render-2` | same | scratch | `35dc3e484d12349a16546967d953001065287c0f015c5ac0a878c1c67118c94c` |
| `render-3` | same | scratch | `35dc3e484d12349a16546967d953001065287c0f015c5ac0a878c1c67118c94c` |
| `fresh-clone` | built in a `git clone --no-local` of `2927f46`, Go caches inside the clone's temporary directory emptied by `go clean -cache -modcache` (0 files left in either), toolchain and modules downloaded again (go1.26.9) | scratch | `35dc3e484d12349a16546967d953001065287c0f015c5ac0a878c1c67118c94c` |
| `sabotage-render-1` | `-tags cp_sabotage_report_nondeterminism` | scratch | `7d605d94089117ac29f0b8c8fb8a4f80b9d2be0d85f3e85f36234bbe786f32f8` |
| `sabotage-render-2` | same | scratch | `5a8716021dcc37cb3ebb9ca50b2f8afeb080de4205bd3eb249b027788de1839c` |
| `sabotage-render-3` | same | scratch | `480d9248cd7e3d23ece9af56e9e2752fd4ee3cb49840779bed2246bc7b843bb8` |
| `render-campaign-k4` | `bin/cpreport` | `campaign-k4/index.html` (88 650 bytes) | `f4763186a9e93cacbec01c755efacc022fba46c4b26e5bff40a0bd65e4108a9a` |

The fresh clone used isolated caches on purpose: `go clean -modcache` on
the owner's own cache would delete data outside the repository used by other
work (AGENTS.md §3); an empty cache of the clone's own is the stronger
condition (nothing cached can be reused) and leaves the owner's untouched.
The sabotaged build emits the client lanes and the per-gateway rows in map
iteration order: same length, three different byte sequences.

`make ci-report` (in record `ci`) re-renders the reference run and
compares the bytes with the committed example with `cmp`, without
normalization.

## 6. Metrics that could not be computed, and why

- **`/metrics` timelines (P23).** No recorded run of K1–K5 holds a
  `/metrics` scrape: the observer records pooler status RPCs (P24), the
  HEALTHY evaluations, `cdc-slot`, `node-sql` and client errors only
  (`evidence/K1/NOTES.md` §4 deferred metric timelines to K6). The report
  cannot draw what the sealed observations do not hold; every page says so,
  and its observation rows and charts show role, reachability, WAL position
  and status-probe latency instead. LIMITATIONS LIM-40 (PENDING: adding the
  scraper before K7, or amending §5.4/P23, is the owner's decision).
- **Latency in phases without ok operations.** f1's *after* phase and f2's
  *before* and *during* phases hold no ok operation (n = 0): no percentile
  exists; the page shows "—" and the data block only `n: 0`.
- **Windows bounded by issued operations.** A window ends at the first ok
  completion after the heal; when the workload has ended (f1) the next
  operation is the runner's recovery write, so the measured end is when an
  operation was next issued (116.9 s), not when the gateway became able to
  serve. The report cannot know the latter from client-observed history.
- **Per-gateway latency percentiles** are not computed: ADR-0036 pools the
  ok operations of all gateways per phase (K6.5 asks for latency per fault,
  windows per gateway).
- **Fault-time of a kill.** The campaign page's fault-time is Σ (heal −
  confirmed) (ADR-0036, as `k4obs`), the time the nemesis held a fault; for
  F2a the runner's `heal` follows `confirmed` at once (PostgreSQL restarts
  by itself), so kills add ~0 (C6's cell: 0.000 s for 12 F2a).
- **Exposure of stub runs.** A stub (manifest, plan, seal) has no history;
  the campaign page counts its verdict but not its exposure and says over how
  many runs the exposure is computed (LIM-30, LIM-37). Every primary run of
  the K4 example is held in full; its stubs are follow-ups.

## 7. The per-campaign example (K4)

Record `render-campaign-k4` rendered `evidence/K4/runs` (220 sealed runs
verified: 114 full archives and 106 stubs; 76 primary runs — 66 valid, 10
INVALID listed and not counted, all held in full — and 144 minimization
candidates and replays listed apart) into `campaign-k4/index.html`. Its
cells reproduce K4's verdicts (e.g. C2: N = 3, incompatible-order 1/3 and
non-linearizable 1/3, Wilson [0.061, 0.793]; D1 M1: lost-write 5/5 [0.565,
1.000]; D2′ M2: lost-write 4/5 [0.375, 0.964]; C5: 9 INVALID attempts
listed, not counted). With N = 3 the zero-failure bound is 0.632: these are
not K7 statements (K7 runs 10 per cell), and the page is an example of the
format, not a result claimed by K6.

## 8. Evidence size (P25)

`evidence/K6` holds 1 665 130 bytes before this file's final edits: the
example report 841 275, the reference run's archive and checks.json 677 920
(an exact copy of K4's; the K4 copy stays where it was), the campaign
example 88 650, records and logs. `evidence/` totals 217 009 107 bytes
(`find evidence -type f`), within P25 = 250 MB; about 33 MB remain for K7.

## 9. Demonstrated / not demonstrated

Demonstrated (records above): a static, self-contained report of a real K4
failover run rendered byte-identically three times and by a fresh clone
built with emptied Go caches; a build with unsorted map iteration renders
three different outputs, which K6.1 detects; a run whose seal does not
verify is refused with exit 2 and nothing written, as a directory and as an
archive; every history event of the reference run is in exactly one element
of the page, every fault phase, every observed rule and leader change, and
every checker anomaly with one link per operation; every per-fault metric
equals cpgate's independent recomputation from the raw history and
observations; a per-campaign summary with the §12.3 statistics computed in
integer arithmetic; all CI targets including `ci-report`.

Not demonstrated: `/metrics` timelines (no scrape exists, LIM-40); a
reference run holding both F2a and F1c (none exists in K4, §4.1); the K6
report on a remote CI runner (LIM-16); that the report's unavailability
windows measure the system rather than the workload's issuing of operations
after the workload ends (§6); the cause of f1's 41 s of write
unavailability (hypothesis only, §4.2).
