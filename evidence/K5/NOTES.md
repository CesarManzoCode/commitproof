# K5 NOTES — Pooler, gateway and CDC under faults

Contract **1.0.9** for the K5 campaign runs (ADR-0029 and ADR-0030 fixed K5's
contract defects, schedules, client behaviours and storage before any K5 run);
**1.1.0** for D3′ and the storage of the runs (ADR-0033 and ADR-0034, both
approved by the owner on 2026-10-14; every D3′ manifest carries 1.1.0). Evidence
produced by `scripts/k5-evidence.sh` (phases setup, images, campaign, rebuild,
campaign, oracle, analysis, ci, store, gate), every step recorded in
`commands.jsonl` (argv, exit code, stdout/stderr sha256). No `claims.yaml`
exists yet (K7): every statement names its record, run id or file; "design
says" cites RESEARCH ids, "system did" cites runs under `evidence/K5/runs/`
and the diagnosis records `k3obs`, `k5obs` (observations, ADR-0016: never a
verdict).

**Status: DONE** — see §1. `cpgate K5 --evidence evidence/K5
--negative-controls` passes on the committed evidence, after the owner
decided the two conditions K5 had first closed BLOCKED on (commit `4254719`):
M3 is detected by D3′ (4/5; ADR-0033), and the runs ADR-0030 requires in
full are committed in full under P25 = 250 MB (ADR-0034).

## 1. Gate

`cpgate K5 --evidence evidence/K5 --negative-controls`: record `cpgate-k5-2`
(2026-10-15 22:24:48–22:29:45 UTC), exit 0, stdout sha256
`ff39d250b55ce17ac68d722ddbb51cc74c77bec888c5b55cd45ba9fb2b90e7fd`. Its
outputs are `gate.json` and `negative-controls.json`. It evaluates the
evidence as committed: the 12 runs ADR-0030 requires in full are full
archives, every other run is a stub (record `store-runs-2`).

| Criterion | Result | Basis |
|---|---|---|
| K5.1 Synthetic validation | PASS | 157 K5 corpus entries exact (≥ 3 plants per class); K2 corpus 807/807 under the K5 checkers; 12 sabotage rules fail exactly their labels; records before every K5 run (§4) |
| K5.2 Campaign runs | PASS | C8 ×3 PASS; C9 ×3 FAIL `notify-not-resumed`; C10 ×5 PASS; C11 PASS, FAIL `fair-share-starvation`, PASS; all valid, results verbatim (§6–§9) |
| K5.3 CDC slot precondition | PASS | 5 C10 primaries read in full, 0 INVALID: slot failover-ready at 10.3–33.1 s, before the first fault 10.1–10.2 s later (§8) |
| K5.4 Mutant detection | PASS | M3 via D3′ (buffered): 4/5 DETECTED, `g1a`; M5 via D5: 5/5 DETECTED, `cdc-phantom`; real build PASS ×5 on both; superseded D3 reported (M3 2/5, real PASS ×5); isolation holds (§5) |
| K5.5 Minimize/replay/triage | PASS | 15 FAIL runs minimized and replayed ×5; 4 real-build triage records (§10) |
| K5.6 CI-equivalent | PASS | `ci-5`: `make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke ci-mutant-smoke` exit 0 on `9be7087` (clean code tree). Earlier attempts: `ci`, `ci-2`, `ci-3` exit 0; `ci-4` interrupted, no record (§2.3) |

**Negative controls.** Each of the six sabotages fails its own criterion and
only that one (`negative_controls_ok: true`):

- K5.1: a planted `cdc-order` entry dropped;
- K5.2: the first C9 verdict flipped;
- K5.3: one C10 run's slot observations rewritten as not synced, re-sealed;
- K5.4: M3's D3′ seeds 1 and 2 rewritten as PASS;
- K5.5: the last replay of M3's first minimized D3′ run deleted;
- K5.6: `ci-5`'s exit code replaced with 1.

**Earlier gate evaluations, kept as recorded.** Records `cpgate-k5` and
`cpgate-k5-staged-adr0030` (both exit 1; `gate-staged-adr0030.json`,
`negative-controls-staged-adr0030.json`) are the BLOCKED closure's. There, M3
was judged on D3 (2/5), and either every run was a stub or the staged copy sat
outside `evidence/`. `cpgate-k5-2` replaces them as the gate of record. Its
`gate.json` and `negative-controls.json` overwrite the earlier files of those
names, whose content stays in git at `4254719`.

**Evidence budget (P25 = 250 MB, ADR-0034).** At K5's close `evidence/`
holds 215.34 MB (`find evidence -type f`, the bytes committed by K5's
last commit). That leaves 34.66 MB for K6 and K7. `store-runs-2`
reported 215 226 458 bytes when it ran; later records, logs and these notes
added the rest.

## 2. Resume of the interrupted session and order of work

The previous session ended (power or session loss) after commit `0b0610e`
(WIP: W5–W8 clients, K5 plans, C8/C9 completions, the K5 corpus generator).
State found: the K5 corpus generated but untracked (`testdata/corpus-k5`,
157 entries; development record `runs/.k5-corpus.out`: 157/157 matched), no
`evidence/K5/`, no K5 run anywhere (`runs/` newest entries are K4's and the
dry precondition checks `runs/.k5-dry-precondition-K1…K4`, all PASS), no
project container, network or volume, no stash or branch. Nothing was
sealed, partial or interrupted, so nothing was archived or re-run.

Order kept (prompt §1, resume protocol §3): **K5.1 before any live K5 run.**
The corpus and the C7–C9 sabotage dependency map (`internal/gate/k5deps.go`,
written from the rules and plants before any K5 sabotage build ran) were
committed (`f6b2746`), then record `k5-precondition` (cpgate K1, K2, K3, K4
on copies of the closed evidence: every criterion PASS) and the K5.1 records
(`gencorpus-k5-verify`, `gencorpus-k2-verify`, `corpus-k5`, `corpus-k2`, 12
`sabotage-<rule>` records, `no-sabotage-symbols`) ended at 22:11:24Z on
2026-10-13. The first live K5 run of any kind — development runs included —
started at 22:16:31Z (`runs/dev-k5/`, outside evidence); the first evidence
run at 22:51:39Z. cpgate K5.1 checks that ordering against every run in
`runs/`. A first start of the setup phase ran the precondition with the
binaries of the WIP tree; it was stopped before any record was written, its
logs moved to `runs/.k5-logs/aborted-setup-1/` (outside evidence), the order
changed to build first (`a0240ce`) and the phase re-run.

### 2.1 Second resume (host shut down during the M5 follow-ups)

The owner's machine shut down at 2026-10-14T04:44:32Z (`last -x`: "shutdown
system down" at 22:44 CST; `journalctl --list-boots`: the boot ends then, the
next starts 2026-10-14T23:18:55Z) while record `campaign-k5-3` ran M5's
follow-ups. State found from evidence: the last commit `9241832`; `evidence/K5/`
untracked; 141 sealed run directories (every primary run, every follow-up of
C9, C11, D3 and of D5 seeds 1–3, three candidates of D5 seed 4) and one
unsealed directory, `20261014T044352Z-D5-4-M5-aa2a5fbd` — a minimization
candidate of D5 seed 4 (manifest `candidate_of`
`20261014T001355Z-D5-4-M5-c5f146f4`, plan `aa2a5fbd…`), started 04:43:52Z,
cluster HEALTHY at 04:44:18Z, empty history and observations, no seal, no
checks.json, no `run` event; the project's 26 containers (exited), its network
and its 7 volumes left over; no stash or branch; `campaign-k5-3` with logs and
no record (interrupted).

Order of the resume, every step recorded in `commands.jsonl`:

1. `prereq-check-resume` (exit 2): 25/26, D5 FAIL — the only occupant of
   10.77.0.0/24 was this project's `commitproof_cpnet`
   (`prereq-resume-1.json`). As in K4, the project-labelled containers,
   network and volumes were removed (images kept, nothing else touched, no
   sudo) and `prereq-check-resume-2` passed 26/26 (`prereq-resume-2.json`).
   `make prereq-check` writes `evidence/K1/prereq.json`; that file was
   restored to its committed content both times.
2. The unsealed candidate was archived unchanged in
   `harness-defect-runs/` (K4's convention; README there) and removed from
   `runs/`.
3. `k5-precondition-resume`: cpgate K1, K2, K3, K4 on copies of the closed
   evidence, every criterion PASS. `images-verify-resume`: the local images
   equal the lock.
4. The campaign resumed as `campaign-k5-4` with the binaries of `40ca0f4`
   (`make build` of `build-4`; `images-build-4`), the build that made every
   `campaign-k5-3` run; nothing in Go changed since (`git diff --stat 40ca0f4
   HEAD`: `images.lock.json`, `scripts/k5-store-runs.sh`). The driver
   re-derived the finished follow-ups from their sealed runs and re-ran the
   interrupted candidate from scratch with the same plan and seed:
   `20261014T232452Z-D5-4-M5-aa2a5fbd`, `plan_sha256`
   `aa2a5fbd6ade422dbc3717d443224fdfd4bd718d3bedc8183c91e07f2d6ca12b`, as in the
   archived manifest. No sealed run was recomputed. `campaign-k5-4` ended
   complete (34 cells: 23 PASS, 11 FAIL; 156 sealed run directories, no
   unsealed one, no project container left).
5. Phases `oracle` (nothing to cross-check: no real-build FAIL has a C3
   anomaly), `analysis` (records `k3obs`, `k5obs` over the 36 primary run
   directories), `ci` (records `lint-workflow`, `ci`: exit 0, on `9241832`
   with uncommitted documentation only). While `ci` ran, `k5-evidence.sh` was
   edited in place (its store phase gained a policy variable); the running
   bash read the changed file once its `case` returned, after `ci`'s record
   was written, and stopped at a syntax error on its first token — nothing
   else ran (`runs/.k5-logs/ci.log`). The edit was committed afterwards
   (`ca9943b`) and CI re-run on the committed tree (§1).

### 2.2 Third resume: the owner's decisions of 2026-10-14

K5 had closed BLOCKED (commit `4254719`) on two decisions: M3 undetected by
D3 (ADR-0031) and P25 (ADR-0032). The owner decided both and three review
items. Items 1–3 are quoted verbatim in ADR-0033 (M3), ADR-0034 (storage)
and ADR-0035 (C11). The G10 harness review (item 4) and the instruction to
keep the prior deviations recorded are summarized below (items 4 and 6). Their
verbatim text was in the third session, which was interrupted, and is not in
the repository (fourth resume, §2.3). State found:

- a clean tree at `4254719`;
- no project container, network or volume;
- the 156 full archives in `runs/k5-full/` (kept, never deleted).

Order of work, every step recorded in `commands.jsonl`:

1. **Storage first: lossless compression, as asked** (record
   `compress-measure`, script `62d1657`). Even at the best measured
   compression, formats cpgate cannot read included, `evidence/` would
   hold 205.66 MB before any D3′ run. So P25 was raised to 250 MB
   (ADR-0034, contract 1.1.0, commit `f7dab87`), keeping zstd -19 (§1).
2. **D3′ designed before any D3′ run.** The record `d3-diagnosis` (script
   `8bcc706`) checked ADR-0031's composition against the ten D3 runs and
   the pinned source, and found it cannot reach M3's defect:
   - the killed leader cannot be recruited ("unknown WAL position"), so a
     follower without the in-flight rows is elected about 1 s after the
     problem is seen;
   - held longer, F1b leads to a ≥ 10 s recruitment backoff, past P01.

   ADR-0033 (commit `bdd6bb2`, contract 1.0.10) therefore accepts D3′ with
   F7 on both followers before each F2a. Every commit then waits ≥ 50 ms
   for its acknowledgement, its WAL already on both followers, when the
   leader dies. Schedule only: workloads, seeds, gaps, expected classes and
   P14 are D3's.
3. **Predictions committed before any D3′ plan existed**
   (`evidence/K5/predictions-D3p.md`, commit `eea9d37`). Then:
   - the code: plan `6f611ac`, gate `f4941db`, matrix and storage
     `e2b8ff4`;
   - records `build-5`, `images-build-5`, `images-verify-5` on `e2b8ff4`;
   - the lock `16241cf`;
   - record `campaign-k5-d3p`.

   Every D3′ run's manifest names `e2b8ff4`, a descendant of `eea9d37`
   (the manifests take the commit and the dirty flag from the binary's
   build).
4. **G10 harness review** (record `g10-listener-check`; test `a375174`):
   the listener is not the cause (§7).
5. **C11 seed 2** (record `c11-window-check`): every refused operation lies
   inside the injected F2a window (§9). ADR-0035 proposes the
   clarification, not applied.
6. **Prior deviations** (the script edited during CI; the staging-script
   bug and its records) stay recorded as deviations, as the owner decided.
7. **After the campaign:**
   - records `k3obs-2`, `k5obs-2` (analysis of the ten D3′ primaries),
     `d3p-diagnosis` and `d3p-halving-check`;
   - `store-runs-2`: 58 D3′ stubs; the 12 ADR-0030 runs replaced from
     `runs/k5-full/` (manifest, plan and seal compared byte for byte);
     evidence/ at 215 226 458 bytes (budget 250 000 000);
   - `results-2`: 214 runs, 15 follow-ups.
8. **CI ran twice in this resume.** `ci-3` (exit 0) started on the clean
   `a760628`, but 2 min into it I created the untracked script
   `scripts/k5-d3p-halving-check.sh`. CI's `images build` reads `git status`
   when it runs, so it built the runner image with `CP_DIRTY=true`. The host
   binaries, built before the file existed, were clean. `ci-3` stays
   recorded. The script was committed (`9be7087`), and CI re-ran on that
   clean tree as `ci-4`. No evidence run used either CI build. `ci-4` was
   interrupted by the host shutdown (§2.3).

### 2.3 Fourth resume (host shut down during `ci-4`)

The owner's machine shut down at 2026-10-15T04:56:43Z (`last -x`: "shutdown
system down" at 22:56 CST; `journalctl --list-boots`: that boot ends then,
the next starts 2026-10-15T22:03:02Z) while `ci-4` ran `ci-nemesis-smoke`.
State found from evidence:

- the last commit `9be7087`, everything since `4254719` under
  `evidence/K5/` uncommitted (records `compress-measure` to
  `d3p-halving-check`, the 58 D3′ runs, `store-runs-2`'s twelve full
  archives in place of their stubs, `results-2`, NOTES);
- `images.lock.json` rewritten by `ci-4`'s `images build` (runner at
  `9be7087`, clean);
- every K5 run sealed and archived: 214 archives in `evidence/K5/runs/`
  (12 full, 202 stubs), each with its checks.json, 15 minimization
  journals, no run directory left unarchived;
- `campaign-k5-d3p` complete (record exit 1: FAIL verdicts present; its
  last event `"complete":true`, 10 cells);
- `ci-4` with logs and no record (interrupted). It had passed `ci-unit`,
  `ci-checkers`, `ci-cluster-smoke` and two of the nemesis smokes; the
  third, CI scratch run `runs/20261015T045556Z-nsmoke-f3a-1-real-75b58a56`
  (outside evidence, git-ignored; `plan_sha256` `75b58a56…be353`, history
  689 478 bytes, no seal), was cut off;
- the project's 5 exited containers, its network and its 7 volumes left
  over; no stash or branch.

No K5 evidence run was interrupted, so nothing was archived in
`harness-defect-runs/` and no evidence run was re-run. Order of this
resume, every step recorded in `commands.jsonl`:

1. `prereq-check-resume-3` (exit 2): 25/26, D5 FAIL — the only occupant of
   10.77.0.0/24 was this project's `commitproof_cpnet`
   (`prereq-resume-3.json`). As before, the project-labelled containers,
   network and volumes were removed (images kept, nothing else touched, no
   sudo); `prereq-check-resume-4` passed 26/26 (`prereq-resume-4.json`).
   `evidence/K1/prereq.json` was restored to its committed content both
   times.
2. `build-6` (`make build` on `9be7087`), then `k5-precondition-resume-2`:
   cpgate K1, K2, K3, K4 on copies of the closed evidence, every criterion
   PASS. `images-verify-resume-2`: the local images equal the lock.
3. CI re-ran from scratch as `ci-5` (exit 0; the script's next name after
   an interrupted attempt) on the same clean code tree. The interrupted
   `ci-4`'s logs stay as they are, and K5.6 reads `ci-5`. Its nemesis smoke
   re-ran the cut-off CI run with the same plan and seed:
   `runs/20261015T221726Z-nsmoke-f3a-1-real-75b58a56`, verdict PASS, the same
   `plan_sha256` (`75b58a56…be353`). The unsealed scratch run stays unchanged in `runs/`.
   It is a CI run, not K5 evidence, so it was not archived under
   `evidence/` (decision for review in the closure). `ci-5`'s `images build`
   rewrote `images.lock.json` byte for byte as `ci-4`'s (sha256
   `83881518…5f60`); that lock is committed as the CI build's.
4. `cpgate-k5-2` (exit 0, §1), this NOTES file, LIMITATIONS and the closure.

## 3. What K5 added (design says / system did)

- **Workloads W5–W8** (§7; ADR-0029, ADR-0030), from the WIP commit, exercised
  live for the first time in this session: W5 session settings and named
  prepared statements set up again on every connection; W6 one listener and
  one notifier per gateway, listeners waiting until 11 s after the workload;
  W7 a `pglogrepl` consumer (commit `662581bb…`) of a non-temporary failover
  slot `cp_slot` through gw-z1, status updates every 1 s at the last commit
  end LSN, reconnects with the same slot at its confirmed flush; W8 200
  `cp_storm` connections in two 20 s phases and `cp_steady` W4 inserts
  returning the server's postmaster start time.
- **K5.3 precondition** in the runner: warm-up writes and slot polling until
  every follower holds `cp_slot` failover-ready (synced, not temporary, not
  invalidated) and the primary's has `failover = true`, within 60 s; C10
  not ready → INVALID.
- **Profile buffered** runs wait 60 s after HEALTHY (ADR-0029).
- **D3′** (ADR-0033, contract 1.1.0; third resume): plan shape `D3p` in
  `internal/plan/k5.go` (F7 on both followers, then F2a on the leader, per
  step; D3's gaps, seeds and workloads), the matrix `campaigns/k5-d3p.json`,
  cpgate K5.4 judging M3 on D3′ with the predictions file required and the
  superseded D3 checked and reported (`f4941db`), and the storage step that
  replaces committed stubs by full archives (`e2b8ff4`).
- **Mutants** `mutants/M3.patch` (classifyError, pooler_gateway.go:96) and
  `mutants/M5.patch` (setSynchronizedStandbySlots, pg_replication.go:600):
  one function, one hunk each, headers with intent, failure mode and
  sources. `images build --mutant` checked isolation: M3 changes only
  `multigateway`, M5 only `multipooler`; every other binary equals the real
  build (records `images-build-M3`, `images-build-M5`).
- **cpgate K5** with six criteria and six negative controls; `make
  ci-checkers` also regenerates and judges the K5 corpus and its 12 sabotage
  rules; tools `k5results`, `k5obs`; `scripts/k5-store-runs.sh`.

## 4. K5.1 — the synthetic validation

Record `corpus-k5`: 157 entries, 157 match their expected.json (written per
plant from §10, never from running cpcheck). Planted histories per class:
session-setting-lost 3, prepared-statement-lost 5, notify-not-resumed 4,
fair-share-starvation 3, overload-failover 3, planned-failover-error 4,
cdc-gap 3, cdc-phantom 3, cdc-duplicate 3, cdc-order 3, and the observation
classes notify-window-loss 7, planned-failover-unbuffered 4. Clean
histories, valid for every checker: W5 25, W6 25, W7 25, W8 25, G8 15.
Record `corpus-k2`: the 807 K2 entries classified with the K5 checkers, 0
mismatches (the completions change no K2 classification). Each of the 12
sabotage builds fails exactly the labels written in `k5deps.go`
(`notify_anchor` fails both notify-not-resumed and the reconnect-window
plants, as predicted from the plants' observations). Stored compressed
(P25); cpgate ties each file to its record by the summary's
`results_sha256`.

## 5. Mutants side by side with the real build (K5.4)

| Mutant (directed schedule) | Mutant seeds 1–5 | Detected | Real build seeds 1–5 |
|---|---|---|---|
| M3 `gateway-retry-any` (**D3′**, profile buffered; ADR-0033, contract 1.1.0) | FAIL `g1a`, FAIL `g1a`, PASS, FAIL `g1a`, FAIL `g1a` | **4/5 — DETECTED** (P14) | PASS ×5 |
| M3 (superseded D3, contract 1.0.9; kept, not counted) | PASS, FAIL `g1a`, PASS, FAIL `g1a`, PASS | 2/5 | PASS ×5 |
| M5 `standby-slots-cleared` (D5) | FAIL `cdc-phantom` ×5 (2, 6, 2, 2, 5 phantom changes) | **5/5 — DETECTED** | PASS ×5 |

D3′ runs (`evidence/K5/runs/`):

- M3: `20261015T015122Z-D3p-1-M3-84dccb5a`, `…015445Z-D3p-2-M3-f75f6098`, `…015811Z-D3p-3-M3-bc177144`, `…020135Z-D3p-4-M3-c83b3282`, `…020452Z-D3p-5-M3-87fb0b83`;
- real: `…020809Z-D3p-1-real-84dccb5a`, `…021130Z-D3p-2-real-f75f6098`, `…021446Z-D3p-3-real-bc177144`, `…021802Z-D3p-4-real-c83b3282`, `…022122Z-D3p-5-real-87fb0b83`.

Record `campaign-k5-d3p` (exit 1: FAIL verdicts present) holds these 10 primaries and 48 follow-up runs: 28 minimization candidates and 20 replays (§10). None of the 58 was INVALID. All ran on `e2b8ff4` (`commitproof_dirty: false`, contract 1.1.0 in every manifest), after the predictions commit `eea9d37` (01:43:22Z; the first D3′ run started 01:51:22Z on 2026-10-15).

**M3 on D3′: outcome against the predictions** (`predictions-D3p.md`, committed before any D3′ run; record `d3p-diagnosis` for the mechanism, diagnosis only):

- **Verdicts.** Predicted: M3 FAIL with C2 `g1a` in ≥ 4 of 5, and real PASS 5/5. Observed: M3 4 of 5 (seeds 1, 2, 4, 5), DETECTED; real 5 of 5 PASS. No C3 `duplicate-elements` occurred, as predicted.
- **Commit wait while both F7s hold.** p50 ok latency was 54–55 ms (outside faults 4.4–5.3 ms) in all ten runs, as predicted.
- **First appointment after the first kill.** In all ten runs the killed leader was "unknown WAL position", both followers were recruited, and a follower was promoted (first attempt 0.5–4.4 s after the kill), as predicted.
- **Real build.** Every statement interrupted at a kill ended `info` (`XX000`, or `57P03`/`57P01` after sending), present or absent, both allowed. No final read held an element twice. As predicted.
- **M3 seeds 1, 2, 5.** The interrupted W4 inserts of the first, buffered kill were retried on the new leader and failed with `23505`, their rows present: C2 `g1a` (2, 1 and 2 inserts). This is the predicted route.
- **M3 seed 4.** The gateways drained the first kill's buffer at 02:03:17.26, before multiorch's appointment began (17.73). M3's immediate retries ended in "no writable primary" (`fail`) with the rows present, at the first kill and again at the third: `g1a` (8 writes). This is the alternative route the predictions named.
- **M3 seed 3 (PASS).** The appointment began 4.4 s after the kill (0.5–2.4 s in the other M3 runs). The gateways drained at 01:59:55.48, 0.50 s before the new leader's promotion completed (55.99). M3's four attempts for the two interrupted inserts ended in `57P03` from the not-yet-writable leader, which is `info` (§6.2 row 11), so no `g1a`; the later kills ended `info` as predicted. This route was *not* among the risks stated in advance, which named a too-slow failover and no insert in flight. The defect still acted: 3 appends were executed twice and appear twice in seed 3's final reads. No checker judges final reads for duplicates (§10.3, LIM-22), so this is an observation, not a detection.
- **Duplicates in final reads.** Elements present twice in cp.append: M3 3, 1, 3, 0, 3 in seeds 1–5; real 0 in all five.

Superseded D3 runs (`evidence/K5/runs/`): D3 M3 `20261013T233223Z-D3-1-M3-44f0b044`,
`…233549Z-D3-2-M3-ad08cd75`, `…233915Z-D3-3-M3-c422c7a4`,
`…234245Z-D3-4-M3-6b74e887`, `…234606Z-D3-5-M3-457577b2`; D3 real
`…234932Z-D3-1-real-44f0b044`, `…235250Z-D3-2-real-ad08cd75`,
`…235615Z-D3-3-real-c422c7a4`, `…235947Z-D3-4-real-6b74e887`,
`20261014T000305Z-D3-5-real-457577b2`; D5 M5 `20261014T000625Z-D5-1-M5-ad688688`,
`…000854Z-D5-2-M5-aca068c8`, `…001126Z-D5-3-M5-d95fb90a`,
`…001355Z-D5-4-M5-c5f146f4`, `…001628Z-D5-5-M5-e9fd481e`; D5 real
`…001925Z-D5-1-real-ad688688`, `…002154Z-D5-2-real-aca068c8`,
`…002426Z-D5-3-real-d95fb90a`, `…002722Z-D5-4-real-c5f146f4`,
`…002954Z-D5-5-real-e9fd481e`.

**M5 (design says / system did).** S13: in slot-based mode the primary lists
the cohort's physical slots in `synchronized_standby_slots`, so logical
decoding does not run ahead of the cohort. D5 isolates the followers (F1b) for
20 s, so commits wait for a synchronous standby; then kills the leader's
PostgreSQL and pooler and keeps it down 30 s. On M5 the consumer received
changes of commits the cohort never had (e.g. seed 1: `cp.ledger` id 410 at
lsn 67452288), which the failover discarded: C7 `cdc-phantom` in every run.
On the real build the consumer received none of them (C7 valid ×5). In every
D5 run the slot was failover-ready before F1b (marker `cdc-slot-ready`, 9.8–
21.7 s after creation on M5, 9.8–33.6 s on the real build; record `k5obs`);
the consumer's stream ended once, at the leader's death, and it reconnected
with the same slot (2 successful connects, 49–69 refused attempts while no
primary served); no change was re-delivered in any D5 run.

**M3 on the superseded D3 (design says / system did) — not detected, investigated; kept as recorded.** S14: the
gateway buffers and retries only errors that prove the statement did not
run. M3 retries every error on a PRIMARY target. D3 kills the leader's
PostgreSQL three times. Per kill (diagnosis of the histories, every
operation in flight at the kill): statements already executing completed
`ok` for ~30–40 ms after `kill -9` (a backend notices the postmaster's
death only at its next wait); statements sent later failed without having
executed (`57P03`, absent from the final state) — on M3 as on the real
build. Only a statement whose commit is in progress when its backend exits
gets an error after execution, the one case in which M3 differs. That
happened once in each detecting run, both times at the first (buffered)
kill: seed 2 an autocommit append sent 39 ms after the kill, seed 4 a ledger
insert sent 40 ms after it; each committed on the old primary, M3's retry
ended in "no writable primary", the client saw `fail` and the final state
held the row → C2 `g1a`. With ~6 statements of 3–8 ms in flight per 25–50
ms, three kills per run hit that window in about two runs of five.
§9.2's first expected class, C3 `duplicate-elements`, cannot occur in D3:
C3 judges reads inside W1 transactions only and final reads are not its
input (§10.3, LIM-22), while D3's W1 variant issues no reads; no final read
of the ten D3 runs held a duplicated element. Per §9.1 M3 was reported
UNDETECTED by D3 and K5 closed BLOCKED on K5.4; ADR-0031 recorded the
investigation and proposed D3′. The owner approved D3′ on 2026-10-14; the
composition ADR-0031 proposed was found unable to reach the defect before
any D3′ run (§2.2), and ADR-0033 fixed D3′ as run above. D3's runs stay
recorded unedited; cpgate checks them and reports them next to D3′, and they
no longer decide M3's detection.

Isolation (as K4.4, records `images-build-M3`, `images-build-M5`): M3 changes
only `/multigres/bin/multigateway`, M5 only `/multigres/bin/multipooler`;
every other binary, and pgBackRest, equals the real build.

## 6. C8 — planned switchovers with buffering (G8)

Three runs (profile buffered; W1 serializable explicit transactions with
micro-ops in key order, W4 autocommit; F6 ×3), all valid, all **PASS**:
`20261013T225139Z-C8-1-real-27ef0b9a`, `…225455Z-C8-2-real-bfac6b42`,
`…225818Z-C8-3-real-026b358c`. Every switchover moved the leadership within
1.4–2.0 s (`k3obs`). Per run C9 judged exactly one window — the first, ≥ 60 s
after bootstrap's buffering (marker `buffer-guard-wait` 59.8 s) — and in it
no autocommit statement failed and every explicit transaction that did not
commit failed with 40001 (604, 545, 560 explicit-transaction failures
judged, none other than 40001). The second and third switchovers come within
the documented 1 min timing guard (ADR-0029), are not buffered by design, and
their errors (57P03 and transport) are the observation
`planned-failover-unbuffered` (342, 336, 366 operations). Design says (S14,
S15): zero application-visible errors for single statements in a buffered
planned failover; explicit transactions get 40001. System did: exactly that,
in 3 of 3 judged switchovers.

## 7. C9 — pooler state: session settings, prepared statements, LISTEN (G9, G10)

Three valid runs, all **FAIL** with C8 `notify-not-resumed` (G10):
`20261013T230136Z-C9-1-real-21458975`, `…230358Z-C9-2-real-b0861f34`,
`…230909Z-C9-3-real-bb8a276f` (seed 3 after two INVALID attempts, §10).

**G9 held.** Each W5 client kept one connection to its gateway through every
fault (F2a, F2b, F6; failovers in two of the three faults per run); C8
checked 1 772–1 973 SHOWs and 2 275–2 363 EXECUTEs per run against the
values and statements set up on that connection: no session-setting-lost, no
prepared-statement-lost. Statements failed during the faults (e.g. seed 1:
354 EXECUTE, 84 SET with 57P03) and the session state was intact afterwards.
Design says (S18): the gateway is authoritative for the session state and
re-applies it on other backends. System did: as designed, in 3 of 3 runs.

**G10 violated.** Each listener (one per gateway; one LISTEN per connection)
kept a single session for the whole run and received every notification
committed before the first fault (600, 605, 600) and **none** of the
3 322–3 726 committed after it, through the rest of the 90 s workload and the
11 s tail — whatever the first fault was (F2b on the leader's pooler, F2a
with a failover, F6). C8 judged only NOTIFYs committed after the listener's
first successful operation post-recovery and outside every fault window
(ADR-0030); all of those were missed (notify-not-resumed); the misses inside
reconnect windows are the observation `notify-window-loss` (4 356, 5 013,
6 044), as S19 documents. Mechanism, from the pinned source
(`go/services/multigateway/poolergateway/notification_adapter.go`): a
session's notification stream is (re)built only inside `update`, i.e. when
the client issues LISTEN or UNLISTEN; `recvLoop` ends on a stream error and
nothing restarts it, and after a failover an unbroken stream stays attached
to the old leader's pooler, where nothing is notified any more (seed 2: F2a
moved the leadership n1 → n3 at 3.8 s; n1's pooler logged `pubsub: reader
error, will reconnect`; the listeners received nothing more). S19 says the
stream "is automatically re-established with a 2-second backoff" and that
only notifications inside the reconnect window are lost. Classification in
§9 (triage).

**Measured LISTEN loss windows** (record `k5obs`, `notify`; observation,
never a verdict). Every listener kept the one session it opened (no reconnect,
no failed wait) and received every notification committed before the first
fault; from the first NOTIFY committed after that fault to the end of the
workload it received none, so the loss window is the rest of the run:

| C9 run | First fault (window) | Listener loss window (each of 26, 27, 28) | Received / missed per listener |
|---|---|---|---|
| seed 1 `…230136Z-C9-1-real-21458975` | F2b 10.4–16.3 s | 20.9 s → 90.4 s (69.4 s) | 600 / 3 726 |
| seed 2 `…230358Z-C9-2-real-b0861f34` | F2a 10.5–12.9 s | 13.5 s → 90.3 s (76.8 s) | 605 / 3 630 |
| seed 3 `…230909Z-C9-3-real-bb8a276f` | F6 10.4–17.4 s | 12.2 s → 90.3 s (78.1 s) | 600 / 3 322 |

(Between the fault's start and the first missed NOTIFY the notifiers' own
transactions failed or were not yet retried; the two INVALID attempts of seed
3 show the same pattern up to their abort.) S19's reconnect windows would be
a few seconds per fault; the windows measured here have no end.

Observed while healing F6 (§10): with listener sessions open, the TERM'd
leader pooler kept running ~30 s (its graceful shutdown waits for the open
notification streams up to its shutdown timeout) instead of ~1 s.

**Harness review: the W6 listener is not the cause** (owner's decision
of 2026-10-14, item 4). Record `g10-listener-check`
(`scripts/k5-g10-listener-check.sh`, stdout sha256 `3ec13385…5222f`):

- **No listener reconnected.** The record covers all 42 C9 runs
  (primaries, the two INVALID attempts, every minimization candidate and
  replay): 121 listener sessions, one LISTEN each, 0 reconnects.
- **The sessions stayed healthy.** In the 39 runs with a fault, 0
  notifications reached any listener after the first fault, and every wait
  after it ended `ok` on an open connection. The 6 failed waits after a
  fault are the two INVALID attempts' aborts.
- **The same loop receives without faults.** In the 3 fault-free
  candidates the same listener code received 48 036 notifications (about
  5 337 per listener) over the whole run. The wait loop does not stop
  receiving over time.
- **The reconnect path works.** On every (re)connect the listener issues
  LISTEN (`internal/runner/k5.go` runListener; `internal/workload/k5.go`
  Listen), and a failed wait drops the connection, so the next step
  reconnects. The unit test `TestListenerRelistensAfterReconnect` (commit
  `a375174`, run inside the record) shows it on a fake server: waits on an
  idle open session end normally at their 1 s deadline; after the session
  breaks, the next LISTEN, on a new session, receives again. pgx sends a
  CancelRequest only when a broken session is closed, never at a wait's
  deadline.
- **The pinned gateway code explains the loss.** It rebuilds a session's
  stream only in `update`, i.e. on the client's LISTEN or UNLISTEN, where a
  failed send triggers `restartLocked`; `recvLoop` returns on a stream error
  and nothing restarts it (`notification_adapter.go` 138–175, 188–228,
  255–270, printed by the record).

A client that kept its session, as S19 implies it can, therefore never gets
the stream back. The loss is the gateway's, not the listener's. The finding
stands; no harness change was needed, and no run was re-run.

One harness detail surfaced by the review, with no effect on any K5 result:
a wait that ends in an error drops the payloads it had collected before the
error (`Listener.Wait`), so they would count as missed. No wait failed after
a fault in any judged C9 run (the only failed waits are the INVALID
attempts'), so no K5 verdict or measured window depends on it (LIM-39).

## 8. C10 — CDC across failover (G11) and the slot precondition (K5.3)

Five valid runs, all **PASS** (`20261013T231132Z-C10-1-real-4e2010a8`,
`…231500Z-C10-2-real-01af5fe0`, `…231736Z-C10-3-real-018e2dd9`,
`…232032Z-C10-4-real-0370610b`, `…232303Z-C10-5-real-ed9c988d`); each saw
F2a, F2b, F1c (10 s) and F6 on the leader once, in a seeded order.

- **Slot sync timing (K5.3).** `cp_slot` became failover-ready on both
  followers 9.8–32.7 s after its creation (marker `cdc-slot-ready`
  `waited_ms`), always before the workload and so before the first fault
  (cpgate K5.3 recomputes it from the observations). After a fault the
  followers held it failover-ready again after every F2b (0.07–0.7 s after
  the heal) and every F1c (0.3–3.7 s), after F2a in two runs (0.07, 0.7 s)
  and after F6 in four (5.1–10.6 s). In the other cases — F2a in seeds 1, 4
  and 5 (seed 1's after the workload), F6 in seed 1 — the first polling
  round after the heal showed a node unreachable, a follower's copy present
  but not synced, or the old leader's copy absent after its rewind, and
  readiness was not observed again before the next fault or the end of
  polling (k5obs `slots.after_faults`). K5.3 judges only readiness
  before the first fault; after a failover the consumer still received
  every change (below).
- **Reconnects.** The consumer's stream ended 2–3 times per run (transport
  error or 57P01 at the leader's death or switchover) and reconnected with the
  same slot at its confirmed flush: 3–4 successful connects, 13–28 refused
  attempts while no primary served.
- **Duplicates relative to the confirmed flush.** No change was received
  twice in any C10 run, neither above the confirmed flush (allowed) nor at or
  below it (`cdc-duplicate`). No gap, no phantom, no order violation; the
  consumer caught up to the final LSN in every run.
- **Leadership.** F2a moved the leader in 1.8–4.7 s; F6 in 2.1–15.7 s; F2b
  and the 10 s F1c never did.

Design says (S12, S13): failover slots are synced to standbys and logical
decoding does not run ahead of the cohort, "no loss of data during a
failover". System did: in 5 of 5 runs, no gap, phantom, duplicate or order
violation. Claim level DESIGN at most (§1, S48: logical replication is not a
supported platform feature).

## 9. C11 — overload (G12)

Three valid runs: seeds 1 and 3 without nemesis **PASS**
(`20261013T232532Z-C11-1-real-3cc39b6f`, `…233007Z-C11-3-real-73d5b2c8`),
seed 2 with F2a(leader) at 10 s inside the first storm phase **FAIL**
`fair-share-starvation` (`…232750Z-C11-2-real-1f5423ed`).

- **Connection acquisition under the storm.** 200 cp_storm connections per
  20 s phase against `--connpool-global-capacity` 50: ~2 730 `pg_sleep(0.5)`
  statements completed per run (~68/s; 50 backends at 0.5 s each allow
  100/s); no storm connection was refused in the fault-free runs. cp_steady completed 100 % of its
  inserts in every fault-free storm window (seeds 1 and 3: 1 824 of 1 824
  inside storms; seed 2 phase 2: 872 of 872), with p50/p99 latency 4.3–4.6 /
  7.0–8.1 ms inside storms against 4.3–4.6 / 7.0–7.6 ms outside.
- **Failovers.** No leader change without a fault in any run (one server
  identity for cp_steady in seeds 1 and 3); in seed 2 the only change
  followed the F2a (k5obs `server_changes`, `after_a_fault`).
- **Seed 2's FAIL.** Record `c11-window-check`
  (`scripts/k5-c11-window-check.sh`, stdout sha256 `203d4d24…7f3f`; times
  from workload start; C8's storm window, first invoke to last completion
  of a phase's storm operations):
  - Storm window 1 (5.00–27.49 s) held 886 cp_steady operations; 766
    completed ok (0.865).
  - All 120 non-ok operations were invoked (10.17–13.25 s) and completed
    inside the F2a window (10.12–13.84 s, PostgreSQL killed and
    restarted): 114 refused at connect with 57P03, 4 refused at connect
    with **28P01** (password authentication failed — for a valid role,
    while PostgreSQL restarted), 2 `info` 57P01.
  - Outside the fault window the steady role completed **744 of 744**.
    The earlier text of this note said "766 of 766": 766 is the window's
    total ok, 22 of them inside the F2a window; corrected from the record.
  - Storm window 2 (no fault) was 872 of 872 ok.

  The checker applies G12's literal rule (§10.9: ok fraction over the
  whole storm window; ADR-0030 kept it, LIM-25), so the run is FAIL. The
  triage (§10) attributes it to the failover's unavailability, not to
  unfair pooling.
- **Owner's decision (item 3):** the literal rule, the FAIL and severity
  `medium` stay as the contract maps them. ADR-0035 (proposed, not applied)
  gives a contract clarification for the owner to consider after K5: G12's
  ok fraction would count only operations invoked outside every fault
  window, as G10's judgement already does, with the fault-overlapped ones
  as an observation. It is not applied to any K5 verdict.

## 10. Minimization, replays and triage (K5.5)

Every FAIL primary run was minimized (§12.2; ADR-0024's procedure, ADR-0030's
targets) and its minimal plan replayed 5 times (P13); journals
`runs/<run>.minimize.json`, replays recorded in results.json.

| FAIL run | Target class | Minimized run (reductions) | Replays with the class | Reproducibility |
|---|---|---|---|---|
| C9 seed 1 `20261013T230136Z-C9-1-real-21458975` | notify-not-resumed | `20261014T004127Z-C9-1-real-e8381b69` (F2b alone; 90 → 22.5 s) | 4/5 | REPRODUCED |
| C9 seed 2 `20261013T230358Z-C9-2-real-b0861f34` | notify-not-resumed | `20261014T010524Z-C9-2-real-b8b35825` (F2a alone; 22.5 s; clients 8, 23–28) | 5/5 | REPRODUCED |
| C9 seed 3 `20261013T230909Z-C9-3-real-bb8a276f` | notify-not-resumed | `20261014T012149Z-C9-3-real-52de89e0` (F6 alone; 22.5 s) | 1/5 | INTERMITTENT |
| C11 seed 2 `20261013T232750Z-C11-2-real-1f5423ed` | fair-share-starvation | the original (no fault → PASS; 45 s → PASS; fewer clients → PASS) | 2/5 | INTERMITTENT |
| M3 D3 seed 2 `20261013T233549Z-D3-2-M3-ad08cd75` (superseded D3) | g1a | `20261014T020756Z-D3-2-M3-c877bac8` (the second and third kills, unbuffered by the timing guard) | 2/5 | INTERMITTENT |
| M3 D3 seed 4 `20261013T234245Z-D3-4-M3-6b74e887` (superseded D3) | g1a | the original (no subset of the kills, no halving, no client reduction kept it) | 0/5 | NOT_REPRODUCED |
| M3 D3′ seed 1 `20261015T015122Z-D3p-1-M3-84dccb5a` | g1a | `20261015T024129Z-D3p-1-M3-6d77285f` (the first step alone; 22.5 s; clients 0, 8, 9) | 5/5 | REPRODUCED |
| M3 D3′ seed 2 `20261015T015445Z-D3p-2-M3-f75f6098` | g1a | `20261015T031126Z-D3p-2-M3-e6d69f18` (the first step alone; 22.5 s; clients 0, 8, 9) | 5/5 | REPRODUCED |
| M3 D3′ seed 4 `20261015T020135Z-D3p-4-M3-c83b3282` | g1a | `20261015T034122Z-D3p-4-M3-8bf2b453` (the first step alone; 22.5 s; clients 0, 9) | 4/5 | REPRODUCED |
| M3 D3′ seed 5 `20261015T020452Z-D3p-5-M3-87fb0b83` | g1a | `20261015T041113Z-D3p-5-M3-3c258252` (the first step alone; 22.5 s; clients 0, 8, 9) | 5/5 | REPRODUCED |
| M5 D5 seed 1 `20261014T000625Z-D5-1-M5-ad688688` | cdc-phantom | `20261014T033002Z-D5-1-M5-af21930b` (D5's step; 22.5 s; clients 8, 29) | 5/5 | REPRODUCED |
| M5 D5 seed 2 `20261014T000854Z-D5-2-M5-aca068c8` | cdc-phantom | `20261014T035023Z-D5-2-M5-ac2559f5` (22.5 s; all clients) | 3/5 | REPRODUCED |
| M5 D5 seed 3 `20261014T001126Z-D5-3-M5-d95fb90a` | cdc-phantom | `20261014T042135Z-D5-3-M5-48de96e0` (22.5 s; clients 8, 29) | 0/5 | NOT_REPRODUCED |
| M5 D5 seed 4 `20261014T001355Z-D5-4-M5-c5f146f4` | cdc-phantom | `20261014T232452Z-D5-4-M5-aa2a5fbd` (45 s; clients 8, 29; re-run after the host shut down, §2.1) | 3/5 | REPRODUCED |
| M5 D5 seed 5 `20261014T001628Z-D5-5-M5-e9fd481e` | cdc-phantom | `20261014T234830Z-D5-5-M5-d365585c` (45 s; clients 8, 29) | 2/5 | INTERMITTENT |

Reading the table. Every C9 minimum is a single fault — F2b, F2a or F6 —
and the empty schedule PASSes in all three: one pooler restart or leader
change is enough to stop a LISTEN session's notifications. F6 alone
reproduces only 1 of 5 times: after a lone switchover the drained pooler is
restarted once it exits (§11 item 3) and the loss does not always fall in
the interval C8 judges. C11 seed 2 needs its F2a and the full 90 s; whether
the restart's refusals exceed 10 % of the storm window varies (2/5). M3's
`g1a` needs a statement caught mid-commit at a kill (§5): minimization and
replays confirm it is a coincidence of timing (2/5 and 0/5); seed 2's
minimum shows that M3's unsafe retry acts without buffering too (the second
and third kills are within the timing guard and retried at once). Under
D3′ (record `campaign-k5-d3p`: 58 runs, none INVALID) each of the four
M3 failures kept its class with the first step alone, i.e. both F7s and
the one buffered kill, over 22.5 s, with the W1 client and one or two W4
insert clients. The empty schedule PASSed in all four, and so did halving
again to 11.25 s. Record `d3p-halving-check`
(`scripts/k5-d3p-halving-check.sh`) shows why: at 11.25 s the workload's
last write is invoked at 11.24–11.25 s, before the F2a at 11.83–11.84 s
(F2a follows both F7 confirmations), so no write is in flight at the kill
(0, against 6 at 45 s and 22.5 s).
The minima reproduced `g1a` in 5, 5, 4 and 5 of 5 replays. D3′ makes M3's
case a property of the schedule, not of timing luck. M5's
`cdc-phantom` needs D5's step (the empty schedule PASSes for all five seeds)
but not the full 90 s nor the second W4 client; its minimized plans
reproduced it 5/5, 3/5, 3/5, 2/5 and 0/5. Hypothesis, not verified: a
phantom needs a change decoded and sent while F1b isolates the followers and
then discarded by the failover, and the shorter plans leave fewer such
changes (seed 1's original had 2 phantom changes, seed 3's 2).

Triage records (`triage/<run>.json`: checker output, §6.2 review of every
completion of the anomaly's operations, §11 classification, finding id per
§11.3, harness review). No real-build FAIL has a C3 anomaly, so no elle-cli
cross-check applies (§10.11).

- **C9 ×3 — G10, PROTOCOL_DEVIATION, DESIGN, origin MULTIGRES, known status
  NEW, severity medium**: `FND-C9-40b6f05a` (seed 1, REPRODUCED),
  `FND-C9-2fe9a9fc` (seed 2, REPRODUCED), `FND-C9-90da5532` (seed 3,
  INTERMITTENT). One mechanism (§7); the ids differ because §11.3 hashes the
  minimized plan. Known status NEW: S19 documents loss *during* reconnect
  windows (measured as notify-window-loss) and states that the stream is
  re-established automatically, which the runs and the pinned source
  contradict; no alpha-known-gap issue in S50 matches; nothing was looked up
  online (AGENTS.md §6).
- **C11 seed 2 — G12, PROTOCOL_DEVIATION, DESIGN, origin MULTIGRES, known
  status NEW, severity medium**: `FND-C11-17a344c1` (INTERMITTENT). Classified
  as §11.1 maps the guarantee (G12 → PROTOCOL_DEVIATION). The evidence (§9)
  shows the injected failover's unavailability window counted by G12's
  literal rule (LIM-25), which in §11.1's terms would be a
  LIVENESS_OBSERVATION (severity info, id `FND-C11-88fc04ee`) — the previous
  session's draft classification. The owner decided (2026-10-14, item 3) to
  keep the literal rule, the FAIL and severity medium, and to add the note
  that every refused operation lies inside the injected F2a window (§9,
  record `c11-window-check`). The proposed clarification for after K5 is
  ADR-0035, not applied. The run stays FAIL in results.json; no rule was
  changed.

## 11. Harness defects found and fixed in K5 (none is a Multigres finding, §11.1)

Development runs (`runs/dev-k5/`, outside evidence, after K5.1's records)
exercised every K5 campaign once before the evidence campaign: C9 seed 1,
C10 seed 1, C11 seed 2, C8 seed 1, D3 seed 1 on M3, D5 seed 1 on M5.

1. **Fresh process numbers collided with K5 client numbers** (development C9
   run, seal well-formedness: "proc 26 invokes after an info completion").
   After an `info` a client continues as a new process numbered from 13; K5's
   clients use their own numbers (14, 20–29, 100–299), so a W4 client's new
   process could be a listener's. Fixed in `9d129ea` (fresh numbers start
   above every planned number; K1–K4 plans keep 13).
2. **A W8 storm connection restarted its retired process in the second
   phase** (development C11 run, same check). Fixed in `5bcb6fe`.
3. **F6's heal started a pooler that was still draining** (campaign-k5-1:
   two INVALID attempts of C9 seed 3, `20261013T230625Z-C9-3-real-bb8a276f`
   and `20261013T230750Z-C9-3-real-bb8a276f`, `fault f0 (F6) not-healed`).
   F6 was the first fault there, with the W6 listeners' notification streams
   open on the leader's pooler; the TERM'd pooler kept running ~30 s
   (graceful shutdown waiting for the streams) and `docker start` 5 s after
   the confirmation was a no-op; the pooler then exited. Fixed in `2b968c5`:
   the heal waits (≤ P28, every inspect recorded) for the container to exit,
   then starts it; the predicate is unchanged. The campaign was stopped while
   cpcheck checked a sealed run (no run interrupted), rebuilt (records
   `build-3`, `images-build-3`, `images-verify-3`) and resumed
   (`campaign-k5-2`); the third attempt of C9 seed 3 had already been valid.
   Every valid run made before the fix was checked: in each, every TERM'd
   pooler exited within 1–3 s and was restarted by its heal (pooler logs).
   The two INVALID attempts stay in `runs/` (stubs) and count toward nothing
   (§12.1).
4. **The minimizer's client reduction ignored K5 client numbers**
   (campaign-k5-2, C9 seed 1's follow-up): `involvedClients` kept only
   numbers below 10, so the candidate dropped the listeners and notifiers
   involved and PASSed. Fixed in `40ca0f4` (anomaly operations map to the
   plan's clients; a storm connection to the storm client, ADR-0030); stopped
   and resumed the same way (`build-4`, `images-build-4`, `images-verify-4`,
   `campaign-k5-3`). Follow-ups are re-derived from their sealed candidates,
   so C9 seed 1's follow-up gained the corrected candidate (which did not
   keep the failure) and kept its result.
5. Tools: `k5obs` read observation counts from the wrong place (`9adfcdc`);
   the setup phase ran the precondition before rebuilding the binaries — that
   first start was stopped before any record was written (§2, `a0240ce`).

K5's evidence runs therefore come from four runner builds, each from a
clean tree (`commitproof_dirty: false` in every manifest):

- `eff004a`: campaign-k5-1 (C8, C9, C10 seed 1);
- `2b968c5`: campaign-k5-2 (C10 seeds 2–5, C11, D3, D5 and the first
  follow-ups);
- `40ca0f4`: campaign-k5-3 and, after the host shut down, campaign-k5-4
  (the remaining follow-ups);
- `e2b8ff4`: campaign-k5-d3p (all 58 D3′ runs).

Between the first three only F6's heal and the host-side minimizer
changed. From `40ca0f4` to `e2b8ff4` the Go changes are D3′'s plan
(`internal/plan/k5.go`), the contract parameters (1.1.0, P25), the
campaign's target class for D3′ and the gate and results tools
(`git diff --stat 40ca0f4 e2b8ff4 -- cmd internal`). The runner, nemesis,
workloads and checkers are unchanged.

6. **The staging script selected nothing** (second resume): the first
   records `stage-adr0030`, `stage-adr0032-option2`, `stage-partial` exited
   0 with no run kept in full — `field()` read an argument its callers never
   pass. Fixed in `c716bcc` (it also fails closed now); re-run as
   `stage-*-2`. Tooling for ADR-0032's measurements, not a run or a verdict.
7. **A failed W6 wait drops the payloads it collected** (found by the G10
   review, §7). Latent: no wait failed after a fault in any judged C9 run,
   so no K5 result depends on it. Not changed in K5, since a harness change
   now would put the CI build on code no evidence run used for nothing
   K5 needs (LIM-39).

## 12. Guarantees G8–G12: demonstrated / not demonstrated

Claim levels per §3: every K5 guarantee is DESIGN; G11 (CDC) is DESIGN at
most (S48: logical replication is not a supported platform feature; the path
is the upstream flag `--enable-slot-based-replication`, LIM-13). No
`claims.yaml` yet (K7): each line names its runs and records.

| Guarantee | Result on the real build | Runs | Demonstrated | Not demonstrated |
|---|---|---|---|---|
| G8 buffered planned failover (S14, S15) | held | C8 seeds 1–3, all PASS (§6) | in each run's judged switchover (the first, ≥ 60 s after bootstrap's buffering) no autocommit statement failed and every failed explicit transaction got 40001 (no other error; no deadlock with W1 in key order) | the second and third switchovers of each run: within the documented 1 min guard, not buffered by design (`planned-failover-unbuffered`, observation); unplanned failovers are not G8's |
| G9 session state (S18) | held | C9 seeds 1–3 (FAIL for G10 only), no `session-setting-lost` / `prepared-statement-lost` (§7) | every SHOW and EXECUTE on the same client connection after F2a, F2b and F6 (failovers in two of three faults per run) returned the values and rows set up on it | a client reconnect: each W5 client kept its one gateway connection through every fault, so re-applying state on a new connection was never needed |
| G10 LISTEN resumes (S19) | **violated** | C9 seeds 1–3 FAIL `notify-not-resumed`; minimized and replayed (§10): FND-C9-40b6f05a, FND-C9-2fe9a9fc, FND-C9-90da5532 | after one fault (F2b, F2a or F6) a listener's session keeps working but receives no notification for the rest of the run (measured windows 69–78 s, §7); mechanism in the pinned source (§7) | loss bounded to reconnect windows, as S19 documents: never observed, because the stream never resumed |
| G11 CDC across failover (S12, S13) | held | C10 seeds 1–5 PASS, K5.3 met in 5/5 (§8); D5 real seeds 1–5 PASS (§5) | no gap, phantom, duplicate or order violation through F2a, F2b, F1c and F6; the consumer reconnected through gw-z1 with the same slot after each stream end and caught up; M5 (`standby-slots-cleared`) shows C7 detects the phantoms S13 guards against (5/5) | slot readiness after every fault (not re-established before the next fault in four cases, §8); several consumers, other plugins, consumers through other gateways |
| G12 fair pools under a storm (S40, S22) | held without faults; **FAIL** with the F2a inside a storm (literal rule) | C11 seeds 1, 3 PASS; seed 2 FAIL `fair-share-starvation`, FND-C11-17a344c1 (§9, §10) | under 200 `cp_storm` connections against a global capacity of 50, `cp_steady` completed every insert inside the storms (p50 as outside them, p99 at most 0.5 ms higher, §9); no leader change without a fault | whether pool sharing is fair when a failover and a storm coincide: the FAIL counts the failover's refusals (LIM-25); other roles, more than two roles, per-user pool limits |
