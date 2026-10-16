# Runs set aside in K7 (CONTRACT §11.1, §12.1)

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
