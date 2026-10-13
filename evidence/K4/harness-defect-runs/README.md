# Runs set aside by a harness defect (CONTRACT §11.1, §12.1)

These three sealed runs are the K4 attempts of matrix cell C4 / baseline /
real / seed 2 made with commitproof `f4a5a0f` (record `campaign-k4-1`). Each
ended INVALID with `fault f1.0 (F1a) not-healed: heal predicate did not hold
within 30s`: in C4's "F5b during F1a(leader)" (ADR-0026) F5b stops n4 while
F1a holds, and F1a's heal predicate still required connects to and from n4
and `iptables-save` in n4's stopped helper (`problems: n3->n4 did not
connect, n4->n3 did not connect, n4: iptables-save failed`). A HARNESS_DEFECT
(§11.1): fixed in `f84bf4f` (F1 heals probe only the namespaces that still
exist and list the vanished ones), and the cell re-run with the fixed build
(records `build-2`, `images-build-2`, `campaign-k4-2`).

They are archived here, outside `runs/`, because the campaign driver counts
every sealed run of a cell's plan as an attempt and stops at three; the
re-run uses the same plan. Their checks.json (cpcheck: INVALID, reason
not-healed) are kept next to them. Nothing in them was changed: each archive
is the sealed run directory (zstd tar, entries sorted, owner 0, mtime 0) and
`cpcheck verify-seal` passes on the extracted directory.

The nine INVALID C5 attempts (F2a not-healed under the original C5 heal
order, ADR-0028) stay in `runs/`: C5's plans changed with the fix, so the
re-runs are different plans and the old attempts do not block them.

## Interrupted by a host power-off (not a harness defect, not a finding)

`20261012T045005Z-D2p-2-M2-2f9705c6.unsealed.tar.zst` (sha256
`e5cfcc57eb8cfa2ba060dbe0c4e364eee9da98441ef497353517b0239725c429`) is the
second minimization candidate of the D2′ FAIL run
`20261012T042125Z-D2p-2-M2-f3aaba7f` (manifest: `candidate_of` that run,
`plan_sha256` `2f9705c6…`, mutant M2, seed 2, commitproof `6232f91`, started
04:50:05 UTC), made by record `campaign-k4-d2p-1`. The owner's machine was
powered off while it ran: `journalctl --list-boots` ends the previous boot
at 2026-10-12T04:52:16Z (`last -x`: "shutdown system down" at that minute),
the directory's last writes are at 04:52 UTC, and the next boot started at
22:41:37Z. The run was never sealed: no
`seal.json`, no checks.json, no `run` event in
`logs/campaign-k4-d2p-1.stdout`; its history ends with the runner's marker
`run-end` `{"reason":"nemesis interrupted","status":"invalid"}`. It is not a
run and counts toward nothing.

It was archived here before the campaign was resumed (record
`campaign-k4-d2p-2`, which re-ran the candidate from scratch with the same
plan and seed): a zstd tar of the directory as found (entries sorted, owner
0, mtime 0), checked file by file (296 files, identical sha256 list) against
the directory before that was removed from `runs/`. Nothing in it was
changed. `cpcheck verify-seal` cannot pass on it (no seal), by construction.
