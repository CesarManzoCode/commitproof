# Runs set aside (CONTRACT §12.1): K5

## Interrupted by a host shutdown (not a harness defect, not a finding)

`20261014T044352Z-D5-4-M5-aa2a5fbd.unsealed.tar.zst` (sha256
`e5e3a64a861a9c46ee8502fe6929cb6f17ace84d0c1ec319dfb9842609b05f2e`) is the
fourth minimization candidate of the D5 FAIL run
`20261014T001355Z-D5-4-M5-c5f146f4` (manifest: `candidate_of` that run,
`plan_sha256` `aa2a5fbd6ade422dbc3717d443224fdfd4bd718d3bedc8183c91e07f2d6ca12b`,
mutant M5, seed 4, commitproof `40ca0f4`, started 04:43:52 UTC on
2026-10-14), made by record `campaign-k5-3`. The owner's machine shut down
while it ran: `last -x` shows "shutdown system down" at 22:44 CST
(04:44Z), `journalctl --list-boots` ends that boot at 04:44:32Z and starts
the next at 23:18:55Z. The cluster had reached HEALTHY at 04:44:18Z
(`logs/cluster-up.json`) and the runner had started its helper; the history
and observations are empty, there is no `seal.json`, no checks.json and no
`run` event in `logs/campaign-k5-3.stdout`. It is not a run and counts toward
nothing.

It was archived here before the campaign was resumed (record
`campaign-k5-4`, which re-ran the candidate from scratch with the same plan
and seed: `20261014T232452Z-D5-4-M5-aa2a5fbd`, same `plan_sha256`): a zstd tar
of the directory as found (entries sorted, owner 0, mtime 0), checked file by
file (37 files, identical names and sha256 list) against the directory before
that was removed from `runs/`. Nothing in it was changed. `cpcheck
verify-seal` cannot pass on it (no seal), by construction.

## Not archived here

The two INVALID attempts of C9 seed 3 (`20261013T230625Z-C9-3-real-bb8a276f`,
`20261013T230750Z-C9-3-real-bb8a276f`; F6 `not-healed`, fixed in `2b968c5`)
are sealed runs and stay in `runs/` (stubs): the campaign driver counts them
as attempts of their cell, whose third attempt was valid (NOTES §11 item 3).
