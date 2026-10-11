# K2 NOTES — Checkers validated before touching Multigres

Contract **1.0.3** (unchanged in K2; no ADR added). Evidence produced by
`scripts/k2-evidence.sh` on commit `a6669e2` with a clean tree, 2026-10-11
04:53–05:00 UTC; the gate run (`cpgate-k2`) followed on the same tree. No
`claims.yaml` exists yet (K7): every statement below names its command
record (in `commands.jsonl`), artifact and sha256, or is marked as not
demonstrated. No campaign touched Multigres in K2.

## 1. Gate

`bin/cpgate K2 --evidence evidence/K2 --negative-controls` → exit 0 (record
`cpgate-k2`, stdout sha256 `abf313d0…cec9cc`). `gate.json` (sha256
`7011cff5…59abf`): K2.1–K2.7 PASS. `negative-controls.json` (sha256
`44ed8253…34c6`): each of the seven sabotages failed exactly its own
criterion, for the intended reason (target problems recorded in the file).

Precondition: `scripts/k1-precondition.sh` (record `k1-precondition`) ran
`cpgate K1` on a copy of `evidence/K1` under the git-ignored `runs/` → K1.1–K1.8
PASS. It runs on a copy because cpgate writes `gate.json` into the directory
it evaluates; see §6.

## 2. Design says / system did, per criterion

### K2.1 Planted anomalies
- **Design says:** for every class of §10.1–§10.7 and §10.9–§10.10, ≥ 3
  planted histories, reported classes equal to the expected set.
- **System did:** `bin/cpcheck corpus testdata/corpus --elle-oracle --out
  evidence/K2/corpus` (record `corpus`, exit 0) checked 807 sealed corpus runs;
  cpgate recomputed every comparison from `corpus/results.jsonl` (sha256
  `225a0d1e…f44c`) against the committed `expected.json` files and re-verified
  every referenced seal. 282 planted histories, all 38 classes covered (3 to
  33 matching histories each; counts in `gate.json`), 0 mismatches.
  Comparison is set equality per checker of anomaly classes and of
  observation classes, plus validity and verdict.

### K2.2 Clean histories
- **System did:** 525 clean histories, all nine checkers `valid: true`;
  per workload W1 195, W2 65, W3 65, W4 130, W5 55, W6 55, W7 55 (W8 55, not
  required). Counts include histories mixing workloads (`mixed-*`, `w7-*`,
  `g8-*` contain W1/W4 operations).

### K2.3 Unknown is not pass
- **Design says:** a register history exceeding P11 → `unknown`, verdict
  UNKNOWN, exit 2.
- **System did:** `bin/cpcheck check evidence/K2/runs/porcupine-timeout`
  (record `pathological`, exit 2, 60.08 s): C4 and C6 `unknown`, every other
  checker `true`, verdict UNKNOWN (`runs/porcupine-timeout.checks.json`,
  sha256 `befb4441…4aff`). The history (copied from
  `testdata/pathological/`, seal root `bfd97f06…c7c8`) has 30 concurrent
  info writes to one register and a read of a value nobody wrote.

### K2.4 Elle agreement
- **Design says:** ≥ 300 W1 histories (≥ 100 clean, ≥ 200 planted, both
  isolation models); C3 validity and mapped class set agree with elle-cli
  wherever elle-cli is not `unknown`.
- **System did:** 350 histories compared (130 clean, 220 planted; 184
  serializable, 166 repeatable-read): **350 agree, 0 disagree, 0 elle-cli
  unknown**. `divergences.json` is `[]` (sha256 `37517e5f…b570`). cpgate
  recomputed Elle's validity and classes from the raw analyses in
  `corpus/elle/raw/` with its own name mapping; the exported inputs are in
  `corpus/elle/export/`.
- The agreement covers every C3 class in both models, including the
  `-process` / `-realtime` variants and G2-item (forbidden under
  serializable, allowed under snapshot isolation by both tools).
- Oracle: `make elle-oracle` (record `elle-oracle`) built
  `commitproof/elle-oracle:9c2a18b` from `build/Dockerfile.elle` = the pinned
  `clojure:temurin-21-lein-2.13.0-bookworm@sha256:724a7f58…c34f` + elle-cli
  `9c2a18b3…5225673` (cloned into the git-ignored `.cache/elle-cli`, never
  vendored); image `sha256:2d51fa76…3d6e`, jar sha256 `8916c46d…6209`,
  `elle-cli 0.1.11`; the corpus run used that image ID.
- Method (from black-box probes of elle-cli, not from Elle's source):
  each history is checked three times — `-c <model>` alone (its `valid?` is
  Elle's validity), and with the base anomalies plus either the `-process`
  or the `-realtime` variants (their `anomaly-types` are Elle's classes).
  Requesting extra anomalies makes elle-cli reject them in `valid?`, and
  requesting both variant families in one run makes Elle label a cycle that
  needs only process order as `-realtime`. Model mapping: serializable →
  `serializable`, repeatable-read → `snapshot-isolation`. Name mapping:
  `G-single-item` → G-single, `G-nonadjacent-item` → G-nonadjacent,
  `G1a`/`G1b` → g1a/g1b; `empty-transaction-graph` is informational.
- elle-cli defect worked around: with explicit `index` fields, elle-cli
  deadlocks (thread dump: nested folds in jepsen.history waiting on each
  other) on any history containing a `fail` op; the exporter therefore omits
  `index` and elle-cli assigns dense indices in file order.

### K2.5 Classification table
- **System did:** `go test -count=1 -json -run '^TestWire'
  ./internal/workload` (record `wire-tests`, exit 0, stdout sha256
  `c09abd6a…a449`): 13 tests pass — one per row of §6.2 plus the S61 COMMIT
  WARNING case — each driving the real `Session` code (pgx) against a
  pgproto3 fake server that answers statements with success, a SQLSTATE, a
  WARNING, a closed connection or silence. During development, breaking
  row 2 in `Classify` failed exactly `TestWireRow02Class40BeforeCommit` (not a
  recorded artifact).

### K2.6 Checker sabotage
- **Design says:** for each `cp_sabotage_<rule>`, the corpus run fails
  exactly the classes depending on the rule.
- **System did:** nine rules (`scripts/sabotage-corpus.sh`, records
  `sabotage-<rule>`, each exit 1, each binary reporting exactly its own
  active sabotage). The expected dependent labels were committed in
  `internal/gate/k2deps.go` (commit `e5582ac`) before any sabotage build ran;
  cpgate recomputed each run's failing labels from
  `sabotage/<rule>/results.jsonl`: every one equals its map entry — rw_edge →
  the nine rw-dependent cycle classes and g1b; prefix_check →
  incompatible-order; duplicate_detection → duplicate-elements;
  lost_write_diff → lost-write; g1a → g1a, dirty-update; porcupine_cas →
  non-linearizable; bank_sum → bank-sum; cdc_phantom → cdc-phantom;
  cdc_duplicate_bound (≤ weakened to <) → cdc-duplicate. No clean history
  failed under any sabotage.
- Product binaries (cpcheck, cpgate, commitproof) contain no
  `cpSabotageActive` symbol; a `cp_sabotage_bank_sum` build does (record
  `no-sabotage-symbols`, `TestProductBinariesHaveNoSabotage` PASS).

### K2.7 CI-equivalent
- `make ci-unit ci-checkers` (record `ci`, exit 0, 2 min 51 s): gofmt, vet,
  staticcheck, all unit tests; then the corpus regenerated byte for byte,
  the corpus + oracle run, the nine sabotage runs and the UNKNOWN control,
  judged by `internal/tools/k2check` (gate logic). `make lint-workflow`
  (record `lint-workflow`) passes on the workflow with the new
  `ci-checkers` job. Not run on GitHub (nothing pushed, LIM-16).

## 3. Checkers: what each detects (corpus ids) and blind spots

Corpus ids are directories of `testdata/corpus/`; `NN` ranges over the
planted copies. Class names are exactly §10's.

| Checker | Demonstrated on | Known blind spots |
|---|---|---|
| C1 lost-acked-writes | lost-write: `w1-ser-lost-write-01..03`, `w4-lost-write-01..03`; final-read-divergence: `w1-ser-divergence-01..02`, `w3-divergence-01`, `w4-divergence-01..02` | Stable final reads are recomputed from the history (last P08 reads identical per gateway and table), never from the runner's markers. info writes are not judged. Register final-read divergence also makes C4 fail (not planted alone). |
| C2 aborted-visible | g1a: `w1-{ser,rr}-g1a-NN`, `w1-ser-g1a-final-01..03` (seen only by final reads, which C3 does not read), `w2-g1a-01..03`, `w4-g1a-01..03` (M3's 23505 shape), and inside `w1-*-dirty-update-NN` | Bank transfers carry no traceable value. CDC deliveries are not reads for C2. |
| C3 list-append | `w1-{ser,rr}-<class>-NN` for G0, G1c, G-single, G-nonadjacent, G2-item (anomaly under serializable, observation under repeatable-read), g1a, g1b (+ G-single: Elle's per-element versions), dirty-update (+ g1a), internal, duplicate-elements, incompatible-order; observations `w1-*-<class>-process-NN` (+ the -realtime class: process order between completed transactions is also realtime order) and `w1-*-<class>-realtime-NN` | Transactions only (LIM-22). Search is exact for G0, G1c, G-single, G2-item; G-nonadjacent and the -process/-realtime variants use a product-state BFS whose walks are checked for simplicity, with a DFS fallback under P12 → `unknown` if the budget runs out (no C3 result in the corpus was `unknown`; whether the fallback ever ran is not recorded). Keys with duplicate or incompatible reads get no edges. A process or realtime edge is not added where a dependency edge already orders the pair. |
| C4 linearizability | non-linearizable: `w2-phantom-value-01..02`, `w2-cas-applied-01..02`, `w2-cas-not-applied-01`, and in `w2-stale-read*`, `w2-g1a-*`; `unknown`: K2.3 | One Porcupine call per key with its own P11; info ops open-ended, info reads dropped, failed ops dropped. Memory under an undecided partition (LIM-23). |
| C5 bank | bank-sum: `w3-sum-read-01..02`, `w3-sum-final-01`; bank-negative: `w3-negative-01..03` | Totals from the plan's setup (P22). |
| C6 stale-read | `w2-stale-read-01..02`, `w2-stale-read-initial-01..02`; not stale: `w2-phantom-value-*`, `w2-cas-*`, `w2-g1a-*` | Only the earliest offending op per key (binary search on prefixes, each bounded by P11) is classified. |
| C7 CDC | `w7-gap-01..03`, `w7-phantom-01..03` (an info insert streamed but never committed, M5's shape), `w7-duplicate-boundary-01..02` (lsn = confirmed flush), `w7-duplicate-below-03`, `w7-order-01..03`; clean `w7-clean-*` include reconnects with allowed re-deliveries above the confirmed flush | Gaps only for ok writes and only after a `caught-up` event (else `unknown`). Final state = stable final read through the first gateway. Synthetic forms only (LIM-24). |
| C8 session-state | `w5-setting-lost-01..03`, `w5-prepared-26000-01..03`, `w5-prepared-row-01`, `w6-not-resumed-01..03`, observation `w6-window-loss-01..03`, `w8-starvation-01..03`, `w8-failover-01..03`; clean `w8-clean-*` include faults inside and between storm windows | Checked at every ok SHOW/EXECUTE, not only after a leader change (§10.9's condition is a subset; leader changes are visible only in observations). A NOTIFY counts as received if a wait that started within 10 s returned it. Literal G12 rule (LIM-25). Synthetic forms only (LIM-24). |
| C9 availability / G8 | planned-failover-error: `g8-autocommit-info-01`, `g8-autocommit-fail-02`, `g8-explicit-info-03`, `g8-explicit-deadlock-04`; clean `g8-clean-*` have explicit transactions failing with 40001 inside the window | G8 only for profile `buffered`; window = F6 inject → healed, an op is inside if [invoke, completion] overlaps it. Literal "only 40001" (LIM-25). Unavailability and latency are stats, not judged. |

History encodings of W5–W8 (defined in K2, read by C7–C9; documented in
`internal/checker/c5c7.go` and `c8c9.go`): `session` set/show/prepare/execute
with a `conn` id; `notify` notify/listen/wait (waits return payload ids);
`cdc` connect/recv/ack/caught-up (`recv` carries table, row, commit lsn, xid
and the confirmed flush at receipt) plus `stream-end`; `overload`
storm/steady with `role`, storm `phase`, steady `server`. K5's clients must
emit these.

checks.json: exactly `{run_id, seal_root, checkers: [{id, valid, anomalies:
[{class, ops, explanation}], stats}], verdict}`; `valid` is `true`, `false`
or `"unknown"`; observations (classes the expected model allows) are under
`stats.observations` with the same shape and never change validity.

## 4. The corpus

- `internal/synth`: an in-memory store executes every operation at one
  instant inside its [invoke, complete] interval, in that instant's order
  (serial, hence strictly serializable and linearizable); outcomes ok / fail
  (no effect) / info (effect with probability ½, process retired). Plants
  add gadget transactions on fresh keys and processes (concurrent with each
  other, or exactly ordered for -process/-realtime plants) or alter recorded
  observations, and state their expected classes, written from §10's
  definitions per plant. `internal/synth` cannot link `internal/checker`
  (independence test).
- `go run ./internal/tools/gencorpus --verify` (record `gencorpus-verify`)
  regenerated all 807 entries and the pathological history byte for byte.
  `testdata/` is about 41 MB, mostly hash-chained history lines.

## 5. Design says / system did on a real history (informational)

`bin/cpcheck check --elle-oracle` on a copy of K1's sealed smoke run (record
`k1-smoke-check`, exit 0; `k1-smoke.checks.json`, sha256 `749389fb…e3e`, seal
root `77cc860c…b690`): verdict PASS, all nine checkers `true`; C3 built a
graph of 2 355 transactions (ww 2 298, wr 2 509, rw 2 592, process 1 694,
realtime 4 826 edges) with no non-trivial SCC; elle-cli agreed (serializable,
no anomaly). This is a 45 s fault-free smoke run of the real build (K1); it
demonstrates nothing about faults.

## 6. Provenance notes

- **Two checker defects were found by the corpus during development and
  fixed before the evidence run** (commits `71221c2`, `5800dde`): C3 gave
  no rw edge to an append that no read observed, so every rw-dependent class
  was missed; C9 judged an info op whose error the client saw before an F6
  window as inside it. Both rules are now stated from the definitions in the
  code comments.
- During this session my first verification of the K1 precondition ran
  `cpgate K1 --evidence evidence/K1`, which rewrote `evidence/K1/gate.json`
  (only the `commands.jsonl` hash changed). It was restored with `git
  checkout` before any commit; K1's evidence is unchanged. The recorded
  precondition runs on a copy.
- elle-cli behaviour (output format, validity semantics, variant labelling,
  the index deadlock, how unobserved appends are ordered) was learnt by
  running it on small handcrafted histories; Elle's and tipocket's source
  were not read (ADR-0004).

## 7. Demonstrated / not demonstrated

Demonstrated (artifacts above): C1–C9 classify 807 synthetic histories
exactly as planted; C3 agrees with elle-cli on 350 histories with 0
disagreements; UNKNOWN is exit 2, never PASS; every §6.2 row classified as
the table says through the real client code; each of nine sabotaged rules is
caught by exactly the expected corpus labels; product binaries carry no
sabotage.

Not demonstrated in K2:
- any checker verdict on a faulted run of Multigres (K3/K4);
- C7–C9 on histories from real W5–W8 clients (K5; LIM-24);
- C3 on histories with predicate reads, or rw-register workloads (LIM-08);
- the oracle image's reproducibility across clean rebuilds (LIM-26);
- the CI workflow on GitHub-hosted runners (LIM-16).
