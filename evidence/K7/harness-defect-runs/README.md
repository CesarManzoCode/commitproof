# Runs set aside in K7 (CONTRACT §11.1, §12.1)

## V1 C2 seed 3: INVALID three times by a harness defect (fixed)

The first three attempts of matrix cell V1 / C2 / baseline / real / seed 3
(plan `9375b700…`, contract 1.8.1, commitproof `8788818`):
`20261016T172043Z-C2-3-real-9375b700` (seal root `61dd0673…`),
`20261016T172408Z-C2-3-real-9375b700` (`634d81c4…`) and
`20261016T172745Z-C2-3-real-9375b700` (`40999f1f…`), made by record
`campaign-start-2`. All three ended INVALID with the same cause:
`cpcheck verify-seal` found the files' hashes and chains intact but 36
history problems "proc 10/11/12 invokes after an info completion". After the
plan's third fault (F1c on n2, healed at 112.8 s), the first recovery insert
through every gateway timed out (`info` at 122.8 s). The runner's recovery
loop retired its process as §6.1 requires, but its final reads restarted on
process 10 + g, the retired one. A **HARNESS_DEFECT** (§11.1), persistent
(3×, §12.1), never a Multigres finding. It is fixed in `1f1f60b` (recovery
and final reads share each gateway's process), with a regression test that
fails on the old behaviour with the same message. Binaries and the runner
image were rebuilt (records `build-5`, `images-build-2`; lock commit
`940e31f`). Only the runner image changed.

The campaign driver was stopped with SIGTERM right after the third
attempt's cpcheck event (the next run, C2 seed 4, stopped in its preflight
before creating a run directory: `campaign-k7.log`). The three attempts
were moved from `runs/k7/` to `runs/k7-invalid/` on this machine (records
`setaside-c2-3`: only their checks.json moved, the sealed 0555 directories
could not be renamed; `setaside-c2-3-chmod`, `setaside-c2-3-2`,
`setaside-c2-3-restore`: owner write bit added to the three directories, the
directories moved, mode 0555 restored; no file inside changed). They were
stored as summaries in `evidence/K7/runs/` (record `store-invalid-2`,
`scripts/k7-store-invalid.sh`: seal roots equal checks.json's, verdict
INVALID, every verify-seal problem of that one kind; `store-invalid`
exited 2 on the first, wrong move). They are listed with their causes in
`results.json` and count toward nothing. The resumed campaign
(`campaign-start-3`, commitproof `1f1f60b`) re-ran the cell with the same
plan and seed. The defect cannot change a run without an `info` recovery
insert, so the runs sealed before the fix (V1 C1 seeds 1–10, C2 seeds 1–2)
are unaffected and were reused.

## K7.4, FND-C8-5254b5cb replay 4: INVALID three times because no cluster could start (the host, not a harness defect, not a finding)

`20261017T183701Z-C8-10-real-cb16d052` (seal root `6df355ae…`),
`20261017T183704Z-C8-10-real-cb16d052` (`43d87470…`) and
`20261017T183709Z-C8-10-real-cb16d052` (`27266480…`) are the three attempts
of replay 4 of FND-C8-5254b5cb's minimized run (`replay_of`
`20261016T231406Z-C8-10-real-cb16d052`, `replay_index` 4, same plan,
contract 1.9.0, commitproof `e8eca97`, clean), made by record
`pkg-replay-FND-C8-5254b5cb-2` on 2026-10-17 between 18:37:00 and
18:37:12 UTC. Each one ended INVALID before any operation (one history
line, `run-end` invalid): `docker compose up` failed with "failed to set up
container networking: failed to create endpoint … on network
commitproof_cpnet: failed to add the host (veth…) <=> sandbox (veth…) pair
interfaces: operation not supported". The cluster was taken down with no
leftover.

The cause is on the host (record `host-kernel-modules`). The `linux`
package was upgraded from 7.2.9 to 7.2.10 at 18:09:26 UTC while kernel
7.2.9-arch1-1 kept running. Its module tree is gone from `/lib/modules`,
so the veth module Docker's bridge networks load on demand cannot be
loaded. A missing kernel module on the owner's machine is a prerequisite
(AGENTS.md §3): K7 stopped BLOCKED, and the owner's action is to boot the
installed kernel.

The three attempts were stored as summaries in
`evidence/K7/package-replays/` (record
`store-pkg-invalid-FND-C8-5254b5cb`: seal verified, verdict INVALID; stub
and checks.json, as K7.4 stores replays). They were then moved from
`runs/k7-pkg/FND-C8-5254b5cb/` to
`runs/k7-invalid/k7-pkg-FND-C8-5254b5cb/` on this machine, as V1 C2 seed
3's were: `setaside-c8-10-r4-chmod` added the owner write bit to the three
sealed directories, `setaside-c8-10-r4` moved them with their checks.json,
and `setaside-c8-10-r4-restore` restored mode 0555. No file inside changed,
and each seal still verifies. The driver counts every sealed INVALID attempt
of a replay index it finds under its output directory. Left there, the
attempts would make the re-executed command stop at replay 4 at once. After
the reboot, `pkg-replay-FND-C8-5254b5cb-3` makes replay 4 afresh with the
same plan, and reuses replays 1–3. The attempts count toward nothing.

## Interrupted by a host power-off (not a harness defect, not a finding)

`20261016T043817Z-C1-8-real-9f75e28e.unsealed.tar.zst` (sha256
`318ed11ed576b246707df1e802df8caaac5f12930bed708ba96daf9f81c6f3be`) is the
first attempt of matrix cell V1 / C1 / baseline / real / seed 8 (manifest:
campaign C1, seed 8, `plan_sha256` `9f75e28e…`, contract 1.8.1, commitproof
`8788818`, clean, started 04:38:17 UTC on 2026-10-16), made by record
`campaign-start` (`make campaign-k7`). The owner's machine was powered off
while it ran: `last -x` shows "shutdown system down" at 22:39 CST
(04:39 UTC) and `journalctl --list-boots` ends that boot at
2026-10-16T04:39:31Z; the next boot started at 16:56:25Z. The directory's
last writes are at 04:39:20 UTC. The run was never sealed: no `seal.json`,
no checks.json, no `run` event in `runs/k7/campaign-k7.log`; its history
ends with the runner's markers `nemesis-end` `{"ok":false,"reason":"nemesis
interrupted"}` and `run-end` `{"reason":"nemesis interrupted","status":"invalid"}`;
`cpcheck verify-seal` on it exits 2. It is not a run and counts toward nothing.

It was archived as found (zstd tar, entries sorted, owner 0, mtime 0; record
`archive-interrupted-20261016T043817Z-C1-8-real-9f75e28e`; the extracted
archive is identical to the directory, `diff -r`), moved out of `runs/k7/`
(to the git-ignored `runs/.k7/interrupted/`) so the campaign driver does not
see it, and re-run from scratch with the same plan and seed by the resumed
campaign. Nothing in it was changed.
