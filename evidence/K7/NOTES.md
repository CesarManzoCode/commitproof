# K7 NOTES — Full campaign, statistics and release

K7's matrix ran under contract **1.8.1** (every matrix plan carries it). After the matrix, the owner's decision on K7's BLOCKED closure raised P25 to 275 MB (ADR-0047, contract **1.9.0**). K7.4's replays ran under 1.9.0 with their packages' plans unchanged. Evidence produced by `scripts/k7-evidence.sh` (groups setup, m1oracle, m1triage, images, start, wait, oracle, store, results, render, triage, cited, c3audit, pkgreports, pkgreplay, liveness, index, packages, p25, ci, gate, tagcheck), the resume steps of §2 and the fixes of §3. Every command is recorded in `commands.jsonl` (argv, exit code, stdout/stderr sha256), 2026-10-15 22:03 CST – 2026-10-17 UTC, one cluster at a time. Every statement that reports a result carries its `claims.yaml` id. "Design says" cites RESEARCH ids; "system did" cites runs and records.

## 1. Gate

`cpgate K7 --evidence evidence/K7 --negative-controls` (records `cpgate-k7…`): its result is `gate.json` and `negative-controls.json`, and the closure (`closure.yaml`) reports it. `cpgate K7` was written before the matrix (`c017e83`, `1b66d40`). Its K7.4 replay check was corrected in `9807b0c` to the P13 replay command (§13). After K7.4's last replay, its index of `package-replays/` was corrected in `9bf9005` to the contract version those replays carry, 1.9.0 (ADR-0047), instead of the matrix's 1.8.1 (§2.3 item 6).

The first run with negative controls (`cpgate-k7`, exit 1, on `5a69aa9`) passed K7.1–K7.7 on the evidence, but no control isolated its target: every control's copy also failed K7.1–K7.5. The controls' harness had three defects. Its links to the other sprints' evidence dangled under a relative `--evidence`, and it linked only K4, K5 and K6b while `claims.yaml` cites K2, K3 and pre-K7 artifacts. K7.2's forgery left CLM-0021's hash of the forged page unchanged, so K7.5 caught it too. K7.7 accepted only one tag check. All three were fixed with regression tests in `9056ede` (§2.4). [CLM-0067]

With the fixes (binaries of `a4352af`), `cpgate-k7-3` passed: every criterion on the evidence, and each negative control failed exactly its target. It ran with `TMPDIR` on disk and `GOMEMLIMIT=6GiB` (§2.5). A run before it, `cpgate-k7-2`, was stopped during the controls to keep the host's memory from running out. The gate was then run once more on the final documents of this file, README, LIMITATIONS and `claims.yaml` (`cpgate-k7-4`; its result is in `gate.json` and the closure). [CLM-0072]

## 2. Resume after the host power-off

The owner's machine was powered off during K7's matrix: `last -x` shows "shutdown system down" at 2026-10-15 22:39 CST and `journalctl --list-boots` ends that boot at 2026-10-16T04:39:31Z. State found before anything was touched:

- **Git**: `main`, 67 commits ahead of `origin/main`, nothing pushed; ADR-0045 (`d07fca6`) and ADR-0046 (`6d06b73`, `49c541b`) committed before the matrix started.
- **Campaign**: `runs/k7` held seven sealed runs (V1 C1 seeds 1–7) and one unsealed directory, C1 seed 8's first attempt `20261016T043817Z-C1-8-real-9f75e28e` (no seal; its history ends in `run-end` `{"reason":"nemesis interrupted","status":"invalid"}`).
- **Docker**: 26 exited project containers, the network `commitproof_cpnet` and seven project volumes.

Resume protocol (as in K4, K5 and K6b):

1. `prereq-check-resume` exit 2: 25 of 26 checks, D5 failing on this project's own leftover `commitproof_cpnet`. `docker-cleanup-resume` removed only the resources labelled `com.docker.compose.project=commitproof` (images kept, no sudo). `prereq-check-resume-2`: 26 of 26 (`prereq-resume-2.json`). `evidence/K1/prereq.json` was restored to its committed content both times. [CLM-0031]
2. `precondition-resume`: cpgate K1–K6 on copies of the closed evidence, every criterion PASS; `images-verify-resume`: the local images equal the lock. [CLM-0032]
3. The unsealed attempt was archived unchanged (`harness-defect-runs/`) and re-run with the same plan and seed (`20261016T170819Z-C1-8-real-9f75e28e`, same `plan_sha256`).

The `docker-cleanup-resume` record was first written with raw newlines in its argv; the line was rejoined with escaped newlines and `record-command.sh` fixed (`748e549`).

### 2.1 The second host power-off (during K7.4) and its resume

The owner's machine was powered off a second time, during K7.4 (§8): `last -x` shows "shutdown system down" at 2026-10-16 22:57 CST, and `journalctl -b -1` ends that boot with systemd unmounting the file systems at 2026-10-17T04:57:02Z. The next boot started at 16:59:11Z. State found before anything was touched:

- **Git**: `main` at `43d7d1d`, nothing pushed. Uncommitted: K7.4's records, logs and replay summaries of five packages; `claims.yaml` and each package's `claim.yaml` (in progress); wording corrections in the three C8 packages' READMEs and in V3 C8 seed 3's triage record. Each correction was checked against the sealed histories: seed 3's i=2411 ran on gw-z2, seed 6's i=2408 on gw-z3, seed 10's i=2409 and i=2410 on gw-z1 and i=2411 on gw-z3.
- **Campaign**: the matrix is complete. 130 valid primaries (19 FAIL, 111 PASS) carry 1.8.1 plans [CLM-0020]. A seal sweep of the 248 run directories under `runs/k7` and `runs/k7-pkg` found every seal valid; it was not recorded then, and §2.3's `seal-sweep-resume-6` is the recorded one over the same directories [CLM-0061]. A dry run with today's binaries lists 130 runs to make. That is only because those binaries make 1.9.0 plans (ADR-0047), whose sha256 differs from the matrix's 1.8.1 plans. The matrix is not re-run.
- **K7.4**: `pkg-replay-<id>` exited 0 for FND-C11-17a344c1, FND-C2-2a534585, FND-C2-6806aeca, FND-C2-8215101c and FND-C2-c7475a73. `pkg-replay-FND-C8-5254b5cb` (started 04:46:56Z) has logs and no record. Its replays 1 and 2 are sealed and checked (PASS, PASS). Replay 3, `20261017T045333Z-C8-10-real-cb16d052`, was sealed at 04:56:49Z (verify-seal valid, `run-end` complete) but never checked. Its cluster was being taken down when the host shut down. No run directory is unsealed, so nothing is archived. `commitproof replay` reuses the sealed replays of the same plan and provenance, and checks the unchecked one. The command is re-executed under the next record name (`pkg-replay-FND-C8-5254b5cb-2`; cpgate K7.4 reads the latest). Eight packages had not started. [CLM-0057] [CLM-0062]
- **Docker**: 8 exited project containers, `commitproof_cpnet` and seven project volumes. No project process was running, and no FUSE mount was left over.

Resume protocol:

1. `prereq-check-resume-3` exited 2 with 24 of 26 checks: D5 failed on this project's leftover `commitproof_cpnet`, and R1 failed with MemAvailable 10.61 GiB against at least 12 GiB. `docker-cleanup-resume-2` removed only the resources labelled `com.docker.compose.project=commitproof`: 8 containers, 1 network, 7 volumes. Images were kept, and no sudo was used. `prereq-check-resume-4` passed 25 of 26. **R1 still failed**: MemAvailable 10.62 GiB of 14.91 GiB, with the owner's browser holding about 5.6 GiB RSS. `evidence/K1/prereq.json` was restored to its committed content both times (copies `prereq-resume-3.json`, `prereq-resume-4.json`). [CLM-0054]
2. `precondition-resume-2`: cpgate K1–K6 on copies of the closed evidence, every criterion PASS. `images-verify-resume-2`: the local images equal the lock. [CLM-0055]
3. R1 is a prerequisite of the owner's machine (AGENTS.md §3). K7 stopped BLOCKED before any further cluster run (closure at `14688db`).

### 2.2 The resume after the owner freed memory, and the kernel upgrade

The owner closed the browser and other heavy applications and re-invoked the resume on 2026-10-17. State found before anything was touched: `main` at `14688db`, clean, nothing pushed. No project container, network or volume; no project process; no FUSE mount but the desktop's own `portal`. K7.4 as §2.1 left it. MemAvailable 12.87 GiB.

1. `prereq-check-resume-5`: 26 of 26 (R1: MemAvailable 12.83 GiB). `evidence/K1/prereq.json` restored to its committed content (copy `prereq-resume-5.json`). `precondition-resume-3`: cpgate K1–K6 on copies, every criterion PASS. `images-verify-resume-3`: the local images equal the lock. [CLM-0056]
2. Seal sweep (not recorded at this resume; §2.3's `seal-sweep-resume-6` is the recorded one): every seal of the 248 run directories under `runs/k7` and `runs/k7-pkg` valid [CLM-0061]. The only one without a `checks.json` was FND-C8-5254b5cb's replay 3, checked by the next replay command [CLM-0057]. The matrix was not re-run.
3. The group `pkgreplay` (from a fresh copy of `scripts/k7-evidence.sh`) re-verified each extracted minimized run (`verify-seal-pkg-<id>-2`) and executed `pkg-replay-FND-C8-5254b5cb-2`. It reused replays 1–3, checked replay 3 (PASS), then made replay 4. That replay was **INVALID three times within 12 s**: every attempt's `docker compose up` failed with "failed to set up container networking: … failed to add the host (veth…) <=> sandbox (veth…) pair interfaces: operation not supported". No attempt ran an operation (one history line each), the cluster was taken down with no leftover, and the command exited 2. [CLM-0057]
4. The cause is on the host (record `host-kernel-modules`, exit 1). The `linux` package was upgraded from 7.2.9 to 7.2.10 at 2026-10-17T18:09:26Z (`/var/log/pacman.log`), while kernel 7.2.9-arch1-1 kept running. `/lib/modules` holds only 7.2.10-arch1-1, veth is not loaded and `modinfo` finds no veth module. Docker's bridge networks need veth, so no cluster can start until the host boots the installed kernel. `make prereq-check` does not check veth (its kernel items are nf_tables, nft_compat, sch_netem and `/dev/fuse`, all loaded before the upgrade), so it passed after the upgrade. [CLM-0058]
5. The three attempts are not runs of the system under test. They were stored as summaries in `package-replays/` (`store-pkg-invalid-FND-C8-5254b5cb`) and set aside, as V1 C2 seed 3's were (§3): `setaside-c8-10-r4-chmod`, `setaside-c8-10-r4`, `setaside-c8-10-r4-restore` moved them to `runs/k7-invalid/k7-pkg-FND-C8-5254b5cb/` on this machine. No file inside changed, and every seal still verifies. The driver counts the INVALID attempts it sees for a replay index, so after the owner's reboot `pkg-replay-FND-C8-5254b5cb-3` makes replay 4 afresh with the same plan (`harness-defect-runs/README.md`). [CLM-0061]
6. P25 after this store step (`p25-count-resume`): 261 879 954 B, margin 13 120 046 B [CLM-0064]. Projection: plus 45 replay summaries at the mean size of the stored ones, 59 559 B (≈ 2.68 MB), and ADR-0045 §5's 5 MB allowance, ≈ 269.6 MB, within 275 MB.
7. A kernel module missing on the owner's machine is a prerequisite the agent never resolves (AGENTS.md §3). K7 stopped BLOCKED again (closure at `ddb9822`, `12cdaef`).

### 2.3 The resume after the reboot into the installed kernel

The owner rebooted the host into kernel 7.2.10-arch1-1, freed memory and re-invoked the resume. `journalctl --list-boots` shows the current boot from 2026-10-17 12:55:10 CST (18:55:10Z). State found before anything was touched: `main` at `12cdaef`, clean, nothing pushed. No project container, network or volume; no project process; no FUSE mount but the desktop's own `portal`. MemAvailable 13.4 GiB. K7.4 as §2.2 left it.

1. Before any cluster run, `host-kernel-modules-2` (exit 0): the running kernel 7.2.10-arch1-1 is the installed `linux` 7.2.10.arch1-1; `/lib/modules` holds that tree only; veth is loaded and `modinfo` finds it. `bridge-probe` (exit 0): a throwaway container (`commitproof/nemesis:real`, `ip -brief addr show eth0`) on a bridge network labelled `com.docker.compose.project=commitproof` (`commitproof_vethprobe`) came up with its veth interface; the network was removed. [CLM-0059]
2. `prereq-check-resume-6`: 26 of 26 (R1: MemAvailable 13.45 GiB). `evidence/K1/prereq.json` restored to its committed content (copy `prereq-resume-6.json`). `precondition-resume-4`: cpgate K1–K6 on copies of the closed evidence, every criterion PASS. `images-verify-resume-4`: the local images equal the lock. [CLM-0060]
3. Seal sweep (`seal-sweep-resume-6`, exit 1 because it also swept `runs/k7-invalid/`): all 248 run directories under `runs/k7` and `runs/k7-pkg` pass `cpcheck verify-seal`. Under `runs/k7-invalid/`, the three attempts of FND-C8-5254b5cb's replay 4 pass. The other three are V1 C2 seed 3's harness-defect attempts, whose histories verify-seal has rejected since they were made ("proc N invokes after an info completion", §3 item 1). The matrix was not re-run. [CLM-0061]
4. The group `pkgreplay` (from a fresh copy of `scripts/k7-evidence.sh`, 19:03–20:27Z, no other cluster) re-verified each extracted minimized run (`verify-seal-pkg-<id>-3`, or the first such record for the eight packages not started), executed `pkg-replay-FND-C8-5254b5cb-3` (replays 1–3 reused, replays 4 and 5 made), then the eight packages not started. Every replay command exited 0, and no new attempt was INVALID or unsealed, so nothing was archived under `harness-defect-runs/`. The results are §8. [CLM-0062]
5. P25 after this store step (`p25-count-resume-2`, 45 replay summaries added): 264 326 805 B, margin 10 673 195 B. [CLM-0065]
6. `cpgate K7` on a scratch copy then reported every K7.4 replay as "contract version 1.9.0 is not one this sprint's runs carry": its index of `package-replays/` used the matrix's check (1.8.1), while ADR-0047 made the replays 1.9.0. That is a defect in the gate, not in the contract or the evidence. It was fixed with a regression test (`9bf9005`, `TestK7IndexContract`): `runs/` must still carry 1.8.1 and `package-replays/` 1.9.0, nothing else. It was built and tested only after the last replay, so no binary changed under K7.4.
7. The groups `liveness` (the two P06 attempts of K7.4, `liveness-summary-2`), `index` (`k7results-2`: `runs-index.json` lists the package replays; `results.json` byte-identical) and `packages` (`k7findings-3`, `pkg-sections-<id>`) ran from the committed script (`6bf4cc2`). [CLM-0016] [CLM-0028]

### 2.4 The release attempt, the controls' defects and the stop for memory

1. The release CI (§14) passed on `77c5f6c`. `v1.0.0` was tagged (annotated, local) on the release commit `5a69aa9`, and `tag-check` found no code difference from the CI's commit and no remote-tracking ref holding K7's first commit. [CLM-0066]
2. `cpgate-k7` with negative controls: every criterion PASS on the evidence; every control's copy also failed K7.1–K7.5 besides its target, so the run exited 1. Its `gate.json` and `negative-controls.json` were committed as written (`f3f465c`). [CLM-0067]
3. The causes were in the gate's control harness, not in the evidence or the contract (§1). They were fixed with regression tests in `9056ede`, which change code. The CI that K7.7 compares the tag with must therefore run again on a commit with the fix. K7.7 now reads the latest tag check, as it reads the latest CI attempt, so the tag can be checked again.
4. A verification of the fixed gate on a scratch copy of this tree had six of seven controls failing only their target. K7.2's also failed K7.5, which led to the third fix. The second verification was stopped by the session because the system ran critically low on memory. `/tmp` is tmpfs on this host (7.5 GB of RAM); at that moment it held this session's scratch copies (914 MB) and the gate's extracted runs (650 directories, about 1.3 GB). Both were removed (this project's own temporaries only), and memory recovered (`host-memory-reap`). The gate group now runs cpgate with `TMPDIR` on disk (`36d25e6`). [CLM-0068]
5. The session's instruction after a stop for low memory is not to restart such work unasked, and every remaining step is heavy: a CI run with clusters, then the gate's eight evaluations. Memory on the owner's machine is a prerequisite the agent does not resolve (AGENTS.md §3), so K7 stopped BLOCKED here. The local tag `v1.0.0` was withdrawn (`tag-withdraw`; it was never pushed) so that no release tag stands on a commit whose gate did not pass. It is to be created again on the release commit after the next CI.

### 2.5 The resume after the stop for memory: CI, tag and gate

The owner freed memory, kept it free, and authorized restarting the stopped gate verification. State found before anything was touched: `main` at `0415b52`, clean, nothing pushed, no tag. No project container, network, volume, process or FUSE mount (only the desktop's `portal`). No cpgate temporary in `/tmp` or `runs/.k7/tmp`. MemAvailable 13.3 GiB.

1. `host-kernel-modules-3`: the running kernel 7.2.10-arch1-1 is the installed `linux` package, and veth is loaded. `prereq-check-resume-7`: 26 of 26 (R1: MemAvailable 13.33 GiB). `evidence/K1/prereq.json` restored (copy `prereq-resume-7.json`). [CLM-0069]
2. A sampler wrote MemAvailable every 5 s to `memory-resume-8.log` for the rest of the resume. The heavy steps ran one at a time, each starting only with MemAvailable at least 12 GiB. [CLM-0073]
3. `ci-2` (`make ci-unit ci-checkers ci-cluster-smoke ci-nemesis-smoke ci-mutant-smoke ci-report`, after `build-10`) exited 0 on the clean build of `a4352af`, the commit with the gate fixes. No target was skipped. MemAvailable stayed at least 10.11 GiB, its dips being CI's own tests and cluster. `lint-workflow` (exit 0 on `77c5f6c`) stands: the workflow file and the Makefile are unchanged since. The runner image's new lock entry was committed (`3ac20a6`). [CLM-0070] [CLM-0073]
4. As the owner asked, the annotated tag `v1.0.0` was created on `a4352af`, the commit CI built. `tag-check-2`: object type tag, the tag's commit equal to the CI's, no code difference, no remote-tracking ref holding K7's first commit. [CLM-0071]
5. `cpgate-k7-2` (`TMPDIR` on disk, so `/tmp` stayed at 13 MB) passed every criterion in its evaluation of the evidence (`gate.json` as written). During the controls' evaluations, cpgate's resident set grew to about 8.6 GiB, and MemAvailable fell from 13.0 to 3.42 GiB. The gate's evaluations run one after another, with no parallelism to reduce. It was stopped with SIGTERM between two evaluations (exit 143), before the system reached critical pressure, and its temporaries on disk were removed. [CLM-0073]
6. `cpgate-k7-3` ran with `GOMEMLIMIT=6GiB`, the Go runtime's soft memory limit, which makes the collector work harder and changes no verdict. cpgate stayed near 5.7 GiB, MemAvailable at least 5.76 GiB, 15 min. Result: every criterion on the evidence, and each negative control failing exactly its target, exit 0. [CLM-0072] [CLM-0073]
7. `p25-final-2`: 264 512 538 B of 275 000 000 B. The claims and documents of this resume were written, and the gate was run once more on them (`cpgate-k7-4`, same settings) before the closure. [CLM-0019]

## 3. Harness defects found in K7 (none is a Multigres finding, §11.1)

1. **The runner's final reads reused a retired process.** After a recovery insert ended `info`, the recovery loop moved to a fresh process (§6.1), but `finalReads` restarted on process 10 + g; `verify-seal` rejected such a history ("proc 10 invokes after an info completion"). V1 C2 seed 3 was INVALID three times for this reason, a persistent HARNESS_DEFECT (§12.1). Fixed in `1f1f60b`, with a regression test that fails on the old behaviour. The driver was stopped at a run boundary, the runner image rebuilt (`build-5`, `images-build-2`), and the three attempts set aside as summaries (`harness-defect-runs/README.md`). The twelve runs sealed before the fix had no `info` recovery insert and were reused. [CLM-0029]
2. **C3 dropped one transaction of every reported cycle.** A reported cycle lacked the anchor's endpoint, so the anomaly's ops and explanation were wrong and the shape step ran on the shortened list. Whether a cycle exists, hence a FAIL under the serializable model, did not depend on it. Fixed in `32b660a` with three regression tests. `make ci-checkers` with the fix is clean (`ci-checkers-c3fix`). The 15 K7 runs sealed before it were re-checked before any was committed (`recheck-c3fix`): no verdict or class set changed, and one `checks.json` (V1 C2 seed 3's cycle explanations) was rewritten. What it changes in the committed evidence is §10. [CLM-0030]

## 4. The stop, the owner's decision and the resumption (ADR-0047)

K7 first closed BLOCKED at its store step (record `store`, exit 4; closure at `fb5d22f`): the P25 projection after storing every K7 run was 251 678 804 bytes, above 250 MB. Nothing stored was dropped. The owner then approved P25 = 275 000 000 bytes, asked that the cited D3′/D5 runs be kept in full, and asked for an audit of the claims the C3 correction affects. ADR-0047 records the decision verbatim (contract 1.9.0, `566d1af`). K7 resumed from the store step: `store-2` stored nothing new and reported a projection of 251 729 547 bytes against 275 000 000 (exit 0). [CLM-0014] [CLM-0033]

## 5. The matrix (§15 K7; `campaigns/k7.json`, `make campaign-k7`)

Records `campaign-start` … `campaign-start-4`, `campaign-wait`, `campaign-wait-2`. The final dry run reported 130 valid and 0 to make. Runs were made with commitproof `8788818` (12 primaries), `1f1f60b` (3 primaries, re-checked with the fixed C3) and `32b660a` (every other run). Every manifest says contract 1.8.1, clean tree, and every run had ADR-0038's metrics exporter (LIM-41). `k7results` wrote `results.json` and `runs-index.json` (215 stored K7 runs with their seal roots); `render-k7` wrote the campaign page with §12.3's statistics. [CLM-0020]

| Cell | Campaign / profile | Valid runs | Failing runs | Classes of the failing runs |
|---|---|---|---|---|
| V1 | C1 / baseline | 10 | 0 | — [CLM-0020] |
| V1 | C2 / baseline | 10 | 4 | seed 3: G-nonadjacent, G-single, G2-item, incompatible-order; seed 7: non-linearizable; seed 8: incompatible-order; seed 9: incompatible-order, non-linearizable [CLM-0020] |
| V1 | C3, C4, C5, C6, C7, C10, C12 / baseline | 10 each | 0 | — [CLM-0020] |
| V1 | C9 / baseline | 10 | 10 | notify-not-resumed (all) [CLM-0020] |
| V2 | C1, C4 / slotless | 5 each | 0 | — [CLM-0020] |
| V2 | C2 / slotless | 5 | 2 | seed 1: non-linearizable; seed 3: incompatible-order, non-linearizable [CLM-0020] |
| V3 | C8 / buffered | 10 | 3 | planned-failover-error (seeds 3, 6, 10) [CLM-0020] |
| V5 | C11 / baseline | 5 | 0 | — [CLM-0020] |

**No acknowledged write was lost in any valid K7 run**: C1 is valid in all 130 valid primaries. [CLM-0025]

INVALID attempts were re-run with the same seed (§12.1) and none is counted; each is stored as a summary with its checks.json, and `results.json` lists each one with its cause [CLM-0020]:

| Cause | Attempts |
|---|---|
| harness defect: final reads on a retired process (§3 item 1) | V1 C2 seed 3 ×3 |
| F6 not confirmed within 60 s (the decided rule's leader did not change) | V1 C9 seeds 2, 4; V1 C10 seed 4 ×2 [CLM-0020] |
| F5b not confirmed within 120 s | V1 C4 seed 2; V1 C12 seed 7; V2 C4 seed 2 ×2 [CLM-0020] |
| no recovery within P06 (final reads not completed; §11) | V1 C9 seed 3 ×2; V1 C9 seed 5 |
| F3a not heal-confirmed within 30 s | V1 C3 seed 8 [CLM-0020] |

Every cell reached a valid run within its three attempts.

## 6. Failures: repeats and new failures (ADR-0045)

ADR-0045's predicates (`internal/repeats`, fixed before any K7 run) classified the 19 failing runs [CLM-0026]:

- **14 repeats**, counted in their cells, stored as summaries and not minimized: V1 C2 seed 7 (R-G6), seed 8 (R-G2b), seed 9 (R-G2b and R-G6); V1 C9 seeds 1–10 (R-G10); V2 C2 seed 3 (R-G2b and R-G6). Each has a triage record (`triage/<run>.json`, `scripts/k7-triage-final.py`: §6.2 review, oracle record for C3, §11 classification), and each is listed with its matched fields as a recurrence in the packages its predicates stand for. [CLM-0026]
- **5 new failures**, minimized and replayed 5× by the driver, original and minimized runs stored in full, triaged by hand (`triage/<run>.json`) and packaged as new findings (§9) [CLM-0026]:

| Run | Finding | Why no predicate matched | Minimized run | Recorded replays with the class |
|---|---|---|---|---|
| V1 C2 seed 3 `20261016T173617Z-C2-3-real-9375b700` | FND-C2-c7475a73 (G5) | its C3 cycle groups have no predicate (its incompatible-order group matches R-G2b) | the original (11 candidates) | 0 of 5 G-nonadjacent [CLM-0046] |
| V2 C2 seed 1 `20261016T221153Z-C2-1-real-c8781715` | FND-C2-8215101c (G6) | R-G6's condition 5: F1c not among the confirmed faults (F1b, F1d) | `20261017T002205Z-C2-1-real-316f3877` (F1d alone, 22.5 s) | 5 of 5 [CLM-0045] |
| V3 C8 seed 3 `20261016T225050Z-C8-3-real-537e02d1` | FND-C8-ff4475d8 (G8) | G8 has no predicate | the original (11 candidates) | 2 of 5 [CLM-0047] |
| V3 C8 seed 6 `20261016T230045Z-C8-6-real-9601ddce` | FND-C8-e6b7ca52 (G8) | as above | the original | 0 of 5 [CLM-0048] |
| V3 C8 seed 10 `20261016T231406Z-C8-10-real-cb16d052` | FND-C8-5254b5cb (G8) | as above | the original | 1 of 5 [CLM-0049] |

The triage, in short:

- **FND-C2-c7475a73.** Every cycle runs through the `info` transaction i=3178, whose key-12 element was read by `ok` transactions and is absent from the final reads after the failover to n3. C3 infers key 12's version order from the transactions' reads (LIM-22). Origin UNDETERMINED: the cycles may be the vanish's effect on that order; the replays reproduced the vanish without cycles. [CLM-0046]
- **FND-C2-8215101c.** FND-C2-6806aeca's mechanism with the leader reachable through F1d instead of F1c: a `cas` whose client timed out became visible, was read by an `ok` read, and was removed by the failover. POSTGRES_INHERITED. [CLM-0045]
- **The three C8 findings.** Explicit transactions that began 15–20 ms after a judged switchover's injection received `57P03` on a statement after `BEGIN`, not `40001`. S14 lists the errors the gateway buffers, and `57P03` is not one of them. Origin hypothesis MULTIGRES; the routing is not traced. [CLM-0047] [CLM-0048] [CLM-0049]

## 7. K7.3: elle-cli on C3 anomalies (ADR-0046)

Group `oracle` (no cluster running). Four matrix runs have C3 anomalies; each is labelled oracle-disagreement with a triage in `oracle/<run-id>.json` [CLM-0027]:

| Run | elle-cli | Result |
|---|---|---|
| V1 C2 seed 3 | validity pass: invalid; incompatible-order and the same G-single as C3 (keys 12 and 13); cycle search timed out, no answer for G-nonadjacent and G2-item | oracle-disagreement [CLM-0027] |
| V1 C2 seed 8 | out of memory (default heap and 9 GB) | oracle-disagreement, no answer (LIM-31) [CLM-0027] |
| V1 C2 seed 9 | invalid; incompatible-order plus a G-single through key 42, whose reads are incompatible | oracle-disagreement [CLM-0027] |
| V2 C2 seed 3 | out of memory (default heap and 9 GB) | oracle-disagreement, no answer (LIM-31) [CLM-0027] |

M1's five D1 runs (records of 2026-10-16 04:12–04:21Z): three oracle-disagreement because elle-cli did not complete (seeds 1, 2, 5), two because elle-cli adds G0 and G-single cycles through incompatible keys (seeds 3, 4). [CLM-0027]

## 8. K7.4: each package's replay command executed once more

Group `pkgreplay`, no other cluster running. Each package's minimized run was extracted from its committed archive (seal verified against `finding.json`), and `commitproof replay <minimized run> --times 5` was executed (record `pkg-replay-<id>`; P13's command, the only count the CLI accepts). CommitProof `e8eca97`, contract 1.9.0, ran with the metrics exporter. The replays are stored as summaries in `package-replays/`, and `k7findings` wrote their results into each `finding.json` (`k7_replay`). The recorded reproducibility stands; this is reported beside it.

State at the stop of §2.1: 5 of 14 packages replayed. FND-C8-5254b5cb's replay command was interrupted after its third replay. FND-C8-e6b7ca52, FND-C8-ff4475d8, the three C9 packages and the three D1 packages had not started. The class counts of each replay command's summary line are its own target class (for FND-C2-6806aeca, the shared minimized run's `incompatible-order`). The package's class is recounted into `finding.json` by `k7findings` and by cpgate K7.4, and no package's `k7_replay` is written yet. Replay 3 of FND-C2-c7475a73 was INVALID twice (`20261017T042636Z…`, `20261017T043111Z…`: `run-end` "recovery deadline P06 exceeded", as in §11). Per §12.1 it was re-run with the same seed and was valid on its third attempt. [CLM-0062] [CLM-0016]

State at the stop of §2.2: still 5 of 14. `pkg-replay-FND-C8-5254b5cb-2` exited 2. Its replays 1–3 are sealed and checked (PASS, PASS, PASS). Replay 4 failed three times because no cluster could start on the host, and those attempts were set aside. Eight packages have not started. [CLM-0057]

At this resume (§2.3) all 14 packages were replayed. Every record exited 0 with five valid replays. Each count is of the package's own anomaly class in FAIL replays, as `k7findings` writes it into `finding.json` and cpgate K7.4 recomputes it from `package-replays/` [CLM-0062]:

| Package | Class | Recorded | K7.4 record | K7.4 verdicts | K7.4 with the class | §11.1 on K7.4's replays | Other classes in K7.4's replays |
|---|---|---|---|---|---|---|---|
| FND-D1-fb8d0f8c | incompatible-order | REPRODUCED (4 of 5) | `pkg-replay-FND-D1-fb8d0f8c` | 5 FAIL | 5 of 5 | REPRODUCED | — [CLM-0062] |
| FND-D1-2e860fd2 | incompatible-order | REPRODUCED (4 of 5) | `pkg-replay-FND-D1-2e860fd2` | 4 FAIL, 1 PASS | 4 of 5 | REPRODUCED | — [CLM-0062] |
| FND-D1-a8eb9857 | incompatible-order | REPRODUCED (5 of 5) | `pkg-replay-FND-D1-a8eb9857` | 4 FAIL, 1 PASS | 3 of 5 | REPRODUCED | replay 5: G0, G1c, G-single, G-nonadjacent, G2-item (C3 only) [CLM-0062] |
| FND-C2-2a534585 | incompatible-order | INTERMITTENT (2 of 5) | `pkg-replay-FND-C2-2a534585` | 4 FAIL, 1 PASS | 4 of 5 | REPRODUCED | — [CLM-0062] |
| FND-C2-6806aeca | non-linearizable | REPRODUCED (3 of 5) | `pkg-replay-FND-C2-6806aeca` | 5 FAIL | 0 of 5 | NOT_REPRODUCED | incompatible-order in all five; replay 1 also G0, G1c, G-single, G-nonadjacent, G2-item [CLM-0062] |
| FND-C2-8215101c | non-linearizable | REPRODUCED (5 of 5) | `pkg-replay-FND-C2-8215101c` | 5 FAIL | 5 of 5 | REPRODUCED | — [CLM-0062] |
| FND-C2-c7475a73 | G-nonadjacent | NOT_REPRODUCED (0 of 5) | `pkg-replay-FND-C2-c7475a73` | 5 FAIL | 0 of 5 | NOT_REPRODUCED | incompatible-order in all five; replay 1 also non-linearizable [CLM-0062] |
| FND-C8-ff4475d8 | planned-failover-error | INTERMITTENT (2 of 5) | `pkg-replay-FND-C8-ff4475d8` | 5 PASS | 0 of 5 | NOT_REPRODUCED | — [CLM-0062] |
| FND-C8-e6b7ca52 | planned-failover-error | NOT_REPRODUCED (0 of 5) | `pkg-replay-FND-C8-e6b7ca52` | 5 PASS | 0 of 5 | NOT_REPRODUCED | — [CLM-0062] |
| FND-C8-5254b5cb | planned-failover-error | INTERMITTENT (1 of 5) | `pkg-replay-FND-C8-5254b5cb-3` | 5 PASS | 0 of 5 | NOT_REPRODUCED | — [CLM-0062] |
| FND-C9-40b6f05a | notify-not-resumed | REPRODUCED (4 of 5) | `pkg-replay-FND-C9-40b6f05a` | 4 FAIL, 1 PASS | 4 of 5 | REPRODUCED | — [CLM-0062] |
| FND-C9-2fe9a9fc | notify-not-resumed | REPRODUCED (5 of 5) | `pkg-replay-FND-C9-2fe9a9fc` | 5 FAIL | 5 of 5 | REPRODUCED | — [CLM-0062] |
| FND-C9-90da5532 | notify-not-resumed | INTERMITTENT (1 of 5) | `pkg-replay-FND-C9-90da5532` | 1 FAIL, 4 PASS | 1 of 5 | INTERMITTENT | — [CLM-0062] |
| FND-C11-17a344c1 | fair-share-starvation | INTERMITTENT (2 of 5) | `pkg-replay-FND-C11-17a344c1` | 3 FAIL, 2 PASS | 3 of 5 | REPRODUCED | — [CLM-0062] |

- The recorded classifications stand. K7.4's result is written beside them in each package (`finding.json` `k7_replay`, README "K7 replay (K7.4)", `claim.yaml`) and in README's findings table, not instead of them. Under §11.1, 8 packages are REPRODUCED on K7.4's replays, 1 INTERMITTENT and 5 NOT_REPRODUCED. Of these, 5 differ from the recorded label: FND-C2-2a534585 and FND-C11-17a344c1 (INTERMITTENT recorded, REPRODUCED here); FND-C2-6806aeca (REPRODUCED recorded), FND-C8-ff4475d8 and FND-C8-5254b5cb (INTERMITTENT recorded), all three NOT_REPRODUCED here. [CLM-0062]
- The three C8 packages produced no `planned-failover-error` in their 15 replays (all PASS). [CLM-0062]
- FND-C2-6806aeca and FND-C2-2a534585 share one minimized run. The replay command's own summary line counts that run's target class, `incompatible-order`, which every replay of FND-C2-6806aeca shows; its package class, `non-linearizable`, is in none of them. [CLM-0062]
- Two replays report C3 cycle classes: FND-C2-6806aeca's replay 1 (with `incompatible-order`) and FND-D1-a8eb9857's replay 5 `20261017T201755Z-D1-4-real-eb537f55` (C3 is its only failing checker: G0 1, G1c 1, G-single 1, G-nonadjacent 2, G2-item 2). They are recorded verbatim in `finding.json`, the package READMEs and `runs-index.json`. K7.3 and ADR-0046 cross-check the matrix's C3 anomalies with elle-cli, and K7's triage covers the matrix's FAILs. K7.4's replays are recorded, neither triaged nor cross-checked, and no finding is derived from them (LIM-46). [CLM-0062] [CLM-0028]
- Whether those cycles run through an `info` transaction whose element the next failover removed, as FND-C2-c7475a73's do, is a hypothesis, not verified. [HYP-0001]
- K7.4 had 5 INVALID attempts, each re-run with the same plan (§12.1) and counted toward nothing: FND-C2-c7475a73's replay 3 ×2 (no recovery within P06, §11) and FND-C8-5254b5cb's replay 4 ×3 (no cluster could start, §2.2). Each is stored as a summary and listed in its package's README. [CLM-0028] [CLM-0016] [CLM-0057]
- C1 found no anomaly in any of K7.4's replays: valid in all 70 valid replays. [CLM-0063]

## 9. Findings packages (§11.4)

Fourteen packages under `findings/` (written, not published): the nine recorded findings, with the classifications ADR-0042 and ADR-0043 gave them and the recorded classification beside them, and the five new findings of §6. Each holds README (upstream issue format, S50; first line G1), `finding.json`, `plan.json` (minimized plan), `checks.json` (original run's), `report-excerpt.md` (from cpreport's page of the original run, `render-pkg-<id>`, `excerpt-<id>`) and `claim.yaml`. [CLM-0040] [CLM-0041] [CLM-0042] [CLM-0043] [CLM-0044] [CLM-0045] [CLM-0046] [CLM-0047] [CLM-0048] [CLM-0049] [CLM-0050] [CLM-0051] [CLM-0052] [CLM-0053]

## 10. What the C3 correction changes (`c3-correction-audit.md`)

`scripts/k7-c3-audit.sh` re-checked, in scratch copies, every committed run held in full that has a C3 result, with the corrected cpcheck (record `c3-audit-2`). Of 172 runs, none changed its verdict, C3 validity, anomaly classes, class totals or observation classes; 166 are identical. In 6 K4 runs only the operations and explanations of cycles changed, all of them `-process`, `-realtime` or SI observations that no verdict reads. One of them is FND-D1-a8eb9857's original run, whose package `checks.json` must stay byte-identical; its README now says so. No README, LIMITATIONS row, finding or claim quotes a C3 cycle's operations from before the fix. LIM-44 records it. [CLM-0015]

## 11. Observation: no recovery within P06 in six INVALID attempts

Six attempts ended INVALID because no recovery insert succeeded within P06 (120 s) after the last heal: V1 C9 seed 3 ×2 and V1 C9 seed 5 (matrix), the driver's replay 4 of V1 C2 seed 3 (minimization), and K7.4's replay 3 of FND-C2-c7475a73 ×2, which replays V1 C2 seed 3's plan as well. Records `liveness-<run>` (`scripts/k7-liveness.py`) and `liveness-summary-2` show the same state in all six. One node is PRIMARY of the latest rule with its WAL position moving; every other node reports REPLICA with its WAL position frozen from the last heal to the end; all 33 recovery inserts (11 per gateway) end `info` at the client deadline. With `AT_LEAST_2`, a commit then waits for a standby that does not replicate. §11.1 names this a LIVENESS_OBSERVATION; §12.1 makes the run INVALID, so these attempts count toward nothing and no criterion packages them (LIM-45). Each cell and each replay reached a valid run on a later attempt. Three of the six are attempts of the same plan, V1 C2 seed 3's. Why replication stopped is not determined; the full archives are in `runs/k7/` and `runs/k7-pkg/` on this machine. [CLM-0016]

## 12. Storage and P25 (ADR-0039, ADR-0045, ADR-0047)

- Store step (`store`): 212 run directories stored once, 6 in full (the five new failures' originals and V2 C2 seed 1's minimized run) and 206 as summaries (stub and `checks.json`). [CLM-0014]
- The cited D3′ and D5 real-build runs in full (`store-cited`): 10 archives from `runs/k5-full/` into `cited/`, each manifest, plan and seal byte-identical to K5's stub, its seal verified and its root equal to K5's `checks.json`. K5's stubs are untouched. Their campaign page (`render-cited-full`) gives both cells exposure from the histories and §12.3's statement. [CLM-0018]
- K7.4's 70 valid replays and 5 INVALID attempts are stored as summaries in `package-replays/` (stub and `checks.json`) and listed in `runs-index.json` under `package_replays` (`k7results-2`; `results.json` byte-identical). Their full archives stay in `runs/k7-pkg/` and `runs/k7-invalid/` on this machine. [CLM-0028]
- P25 after K7.4's store steps: `p25-count-resume` 261 879 954 B [CLM-0064], `p25-count-resume-2` 264 326 805 B [CLM-0065].

- Final count (`p25-final-2`, after K7.4, the release CI's records and the gate's): 264 512 538 B of 275 000 000 B, margin 10 487 462 B (K7's share 43 594 880 B). The earlier `p25-final` read 264 452 426 B. What is committed after it (the last gate run's records, the closure) is a few kilobytes of text. [CLM-0019]

## 13. Claims, overclaims, public documents

- `claims.yaml` (§17) holds every claim of README, LIMITATIONS and this file, each with contract section, target, command, artifact, the artifact's sha256 (a run archive: its seal root), result and limitations. Each package's `claim.yaml` is its entry there.
- `overclaim-review.md` lists every statement changed from earlier sprints' drafts, K7's BLOCKED draft included, and why.
- README: the §4 matrix verbatim, results per cell with §12.3's statement for every clean cell (K7's 11 and the cited D2′, P1, DH1, D3′ and D5 real-build cells), mutants beside the real build, H1, the findings ordered by severity after the G1 line, and the limitations summary.
- LIMITATIONS: no row PENDING; LIM-16 OPEN (owner action: the first remote CI run); new rows LIM-44 (C3 correction), LIM-45 (liveness is recorded, not judged) and LIM-46 (K7.4's replays are recorded, not triaged).

## 14. CI and release

`make lint-workflow` (record `lint-workflow`) and every `make ci-*` target (`ci-unit`, `ci-checkers`, `ci-cluster-smoke`, `ci-nemesis-smoke`, `ci-mutant-smoke`, `ci-report`; record `ci`) exited 0 on the clean build of `77c5f6c` (record `build-9`, `Dirty=false`), 20:43–21:00Z, with no target skipped. [CLM-0066]

The first release attempt tagged `v1.0.0` on `5a69aa9` (record `tag-check`). `cpgate-k7` then exited 1 because of the controls' defects (§1, §2.4), whose fix changed code, so that tag was withdrawn. [CLM-0067]

The release: every `make ci-*` target exited 0 on the clean build of `a4352af`, which contains the fixes (record `ci-2`, after `build-10`) [CLM-0070]. The annotated local tag `v1.0.0` is on `a4352af` itself; `tag-check-2` found no code difference and no remote-tracking ref with K7's commits [CLM-0071]. `cpgate-k7-3` passed with every negative control failing exactly its target [CLM-0072]. The commits after `a4352af` change only evidence, documents, `claims.yaml` and `images.lock.json` (`3ac20a6`, the runner image CI rebuilt). Nothing was pushed.

## 15. Deviations from the prompt

- K7 stopped once BLOCKED at the store step (owner decision 1) and resumed after the owner raised P25 to 275 MB (ADR-0047, contract 1.9.0). The version changed after the matrix, not while it ran. K7.4's replays record 1.9.0 in their manifests and replay their packages' plans unchanged.
- K7.4 executes each package's P13 replay command (`--times 5`): the CLI accepts no other count, and cpgate K7's check was corrected to it (`9807b0c`).
- Two harness defects were fixed during the matrix (§3); the driver was stopped twice at a run boundary, and the sealed runs were re-checked, not recomputed.
- `k7findings` counted an INVALID replay attempt as a recorded replay; fixed (`65ff46a`) before any package was written from it.
- The four "no recovery within P06" attempts are reported as an observation (§11, LIM-45): the contract gives no package to INVALID runs.
- A second host power-off interrupted K7.4 (§2.1): one replay command had no record, and its three replays were sealed. The command is resumable and is re-executed under the next record name; no sealed run is recomputed. At this resume `make prereq-check` failed R1 (available memory) after the cleanup, so K7 stopped BLOCKED again before any further cluster run.
- At the next resume (§2.2), with R1 passing, the host's kernel package had been upgraded while the old kernel ran. No cluster could start (veth unavailable), and replay 4 of FND-C8-5254b5cb was INVALID three times for that reason. The three attempts were stored as summaries and set aside so the replay can be made afresh after a reboot. K7 stopped BLOCKED a third time. [CLM-0057] [CLM-0058]
- The owner rebooted into the installed kernel, and K7 resumed a fourth time (§2.3). Kernel, veth and a bridge probe were checked before any cluster run. K7.4 was finished without relaunching the matrix: one replay command resumed, eight run for the first time. [CLM-0059] [CLM-0062]
- `cpgate K7` held K7.4's replays to the matrix's contract version. The gate was corrected with a regression test (`9bf9005`) after the last replay; the criterion is not relaxed: each runs directory must carry exactly its own version.
- K7.4's replays are recorded, not triaged: two of them report C3 cycle classes that no criterion cross-checks (§8, LIM-46). [CLM-0062]
- `v1.0.0` tags `a4352af`, the commit the release CI built, as the owner asked, not the final commit. The commits after it add only evidence, documents, `claims.yaml` and the runner image's lock entry. The closure records the CI, the tag check and the gate, so it cannot precede them.
- A fifth resume, after the stop for memory (§2.5): CI on the fixed commit, the tag, and the gate. `cpgate-k7-2` was stopped during its controls when MemAvailable fell to 3.42 GiB. The gate was re-run with the Go runtime's soft memory limit (`GOMEMLIMIT=6GiB`) and passed (`cpgate-k7-3`), then ran once more on the final documents (`cpgate-k7-4`). [CLM-0072] [CLM-0073]
- The first gate run with negative controls exited 1 because of three defects in the controls' harness, fixed with regression tests (`9056ede`; §2.4). No criterion, threshold or control was relaxed: the copies now hold the evidence every criterion reads, and K7.2's forgery is made consistent so that only K7.2's recomputation can catch it. K7.7 reads the latest tag check as it reads the latest CI. [CLM-0067]
- The local tag `v1.0.0` (on `5a69aa9`, never pushed) was withdrawn after the gate run, because its gate did not pass and the fix changes code. A session stop for low memory then ended the sprint BLOCKED before the CI and the gate could run again (§2.4). [CLM-0068]
- The owner's decision text of 2026-10-15 gives P25 as 250 MB. The owner's later decision on K7's first BLOCKED closure (2026-10-16, ADR-0047) raised it to 275 MB, and K7 enforces 275 MB.
