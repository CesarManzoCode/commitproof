**Acknowledged writes lost (G1) in this finding's original run: no.**

# multigateway: a LISTEN session receives no notifications after a leader change (independent reproduction supporting upstream PR #1550)

This package is an **independent reproduction** supporting the open upstream pull request #1550 (S53, "fix(multigateway): re-establish LISTEN notification stream after leader change"), not a new issue (ADR-0043): the known status is `KNOWN`. Nothing was posted upstream.

| Dimension | Value (contract 1.9.0) | Recorded in K5 (contract 1.0.9) |
|---|---|---|
| Finding | FND-C9-2fe9a9fc | FND-C9-2fe9a9fc |
| Guarantee | **G10** — LISTEN subscriptions resume after a failover: notifications sent after recovery are delivered (§3) | G10 |
| Class | PROTOCOL_DEVIATION | PROTOCOL_DEVIATION |
| Claim level | DESIGN (S19) | DESIGN |
| Severity (§11.2, derived) | medium | medium |
| Origin | MULTIGRES (`notification_adapter.go` at e3cbdbb) | MULTIGRES |
| Reproducibility | REPRODUCED (5 of 5 recorded replays) | REPRODUCED |
| Known status | **KNOWN** — upstream PR #1550 (S53), opened 2026-10-15, not merged at upstream main `b2137f9` | NEW |

## Summary

In C9 (pooler state) seed 2 on the real build, every W6 listener (one per gateway, one LISTEN on its one connection) received every notification committed before the run's first fault, a `kill -9` of the leader's PostgreSQL (F2a) at 10.5 s, after which leadership moved n1 → n3 (rule 2.0, 3.8 s later), and **none** committed after it. That held through the rest of the 90 s workload and the 11 s tail, although each listener's session stayed alive and kept completing its waits. C8 judges only NOTIFYs committed after the listener's first successful operation following recovery and outside every fault window (ADR-0030); all of them were missed (`notify-not-resumed`). Losses inside reconnect windows are documented upstream (S19) and are counted as the observation `notify-window-loss`, not judged. Upstream's PR #1550 describes the same mechanism: an idle LISTEN client stops receiving notifications because the gateway replaces a session's stream only on the client's next LISTEN or UNLISTEN.

## Version and environment

- Multigres `e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73` built from source with upstream's Dockerfiles (only `FROM` lines changed, §5.2), PostgreSQL 17.7 (`postgres:17.7@sha256:2006493727bd5277eece187319af1ef82b4cf82cf4fc1ed00da0775b646ac2a4`), durability policy `AT_LEAST_2`, cohort {n1, n2, n3}, three cells with one multiorch and one multigateway each (CONTRACT §5.1).
- Profile `baseline` (gateway buffering off, `--enable-slot-based-replication=true` as in upstream's Kubernetes demo); no metrics exporter in the original run (contract 1.0.9; LIM-41).
- Host: Arch Linux, kernel 7.2.9-arch1-1, Docker 29.9.0, one bridge network 10.77.0.0/24, all containers on one host (LIM-02, LIM-03).
- Original run: CommitProof `eff004a` (clean), contract 1.0.9, started 2026-10-13T23:03:58Z; minimized run `20261014T010524Z-C9-2-real-b8b35825` (CommitProof `40ca0f4`) and its five recorded replays (K5).
- Client: pgx v5.11.0; P01 deadline 10 s per operation (§6.2).

## Reproduction

From a clone of this repository at tag `v1.0.0` (Linux host meeting `docs/PREREQUISITES.md`):

```sh
make prereq-check                       # 26 checks; exit 0 required
make build
bin/commitproof images build            # pinned images; writes images.lock.json
bin/commitproof cluster up --profile baseline && bin/commitproof cluster verify && bin/commitproof cluster down   # optional: HEALTHY check
mkdir -p runs/k7-pkg/FND-C9-2fe9a9fc
tar --zstd -C runs/k7-pkg/FND-C9-2fe9a9fc -xf evidence/K5/runs/20261014T010524Z-C9-2-real-b8b35825.tar.zst
bin/commitproof replay runs/k7-pkg/FND-C9-2fe9a9fc/20261014T010524Z-C9-2-real-b8b35825 --times 5 --out runs/k7-pkg/FND-C9-2fe9a9fc --cpcheck bin/cpcheck
```

Each replay brings the cluster up, runs the minimized plan (`plan.json` here; sha256 of its canonical bytes `b8b35825536d755a01ff0a80797ac10659ac46aba85655ebd7cca95ef04f6539`), tears the cluster down, seals and checks the run; the last line is a JSON summary with the replays and how many show the class the driver targets. `tar --zstd -xf evidence/K5/runs/20261013T230358Z-C9-2-real-b0861f34.tar.zst` and `bin/cpcheck check <run-dir>` reproduce the original run's `checks.json` here byte for byte.

## Expected vs actual

- **Expected** (G10, DESIGN): S19 states that LISTEN/NOTIFY uses a dedicated per-pooler listener and a per-session gRPC stream that "is automatically re-established" after it breaks, and that only notifications sent during the reconnect window are lost. Notifications committed after recovery are delivered.
- **Actual**: every listener received the 605 notifications committed before the fault and missed the 3 630 committed after it; C8 judged the post-recovery ones `notify-not-resumed` (5 013 more are `notify-window-loss`). n1's pooler logged `pubsub: reader error, will reconnect`.

## Analysis

**Design says** (S19, `docs/query_serving/listen_notify_design.md` at e3cbdbb): one gRPC NotificationStream per client session from the gateway to the pooler, re-established automatically with a 2 s backoff when it breaks; loss only inside reconnect windows.

**System did**: F2a on the leader's PostgreSQL at 10.5 s; leadership n1 → n3 at rule 2.0 about 3.8 s later (observations). The listeners' sessions stayed open and their waits kept completing `ok` with no notification after the fault (`report-excerpt.md`; evidence/K5/NOTES.md §7).

**Mechanism in the source** (e3cbdbb, `go/services/multigateway/poolergateway/notification_adapter.go`): a session's stream is built and rebuilt only inside `update()`, that is when the client issues LISTEN or UNLISTEN (`restartLocked` → `startLocked` re-subscribes the active channels). `recvLoop` returns on a stream error after posting it to a channel that only the next `update` reads, so nothing else re-establishes the stream. After a leader change an unbroken stream stays on the old leader's pooler, which is now a standby where nothing is notified (PostgreSQL refuses LISTEN during recovery). PR #1550 (S53) says the background reconnect of the design document was dropped in upstream #1222. It changes `notification_adapter.go`, `pubsub/listener.go` and `grpcpoolerservice/service.go` and adds a failover test. This package does not test that pull request.

**Harness review** (§6.2, K5 triage `evidence/K5/triage/20261013T230358Z-C9-2-real-b0861f34.json`): every reviewed completion is the LISTEN that opened the listener's session or a NOTIFY transaction that was missed, both `ok` (row 1). The listener received notifications on the same connection before the fault, so the client could receive them. The run is valid and every fault was confirmed and healed. Not a HARNESS_DEFECT. A W6 wait that ends in an error drops the notifications it had collected (LIM-39); no wait failed after a fault in any judged C9 run of K5.

**Minimization and replays** (§12.2, K5): F2a alone, 22.5 s, clients 8 and 23–28 (the W5 clients and client 9 removed); minimized run `20261014T010524Z-C9-2-real-b8b35825`; its five recorded replays FAIL with `notify-not-resumed` in 5 (REPRODUCED).

<!-- K7-RECURRENCES -->
## Recurrences in K7 (ADR-0045)

10 valid run(s) of K7's matrix repeat this finding's mechanism: every guarantee group of their anomalies matches ADR-0045's predicate R-G10, which reads only `checks.json`. Each is counted in its cell's k/N and interval (§12.3), triaged (`evidence/K7/triage/`), committed as a summary (stub and `checks.json`, `evidence/K7/runs/`), and not minimized. Its full archive stays on the machine that ran it (`runs/k7/`). `cpgate K7` re-evaluates the predicate for every one.

| Run | Cell | Seed | Seal root | Predicate | Matched fields |
|---|---|---|---|---|---|
| `20261016T201832Z-C9-1-real-d61643d0` | V1 C9/baseline | 1 | `56e09ff799f1eb3d…` | R-G10 | classes notify-not-resumed (notify-not-resumed 6834); F {F2a, F2b, F6}; listener sessions: client 26 LISTEN i=8; client 27 LISTEN i=9; client 28 LISTEN i=11 |
| `20261016T202317Z-C9-2-real-5c3d3a46` | V1 C9/baseline | 2 | `7f5edfd4548bbce6…` | R-G10 | classes notify-not-resumed (notify-not-resumed 6858); F {F2a, F2b, F6}; listener sessions: client 26 LISTEN i=9; client 27 LISTEN i=11; client 28 LISTEN i=13 |
| `20261016T203413Z-C9-3-real-c2918eba` | V1 C9/baseline | 3 | `f98e8071c4aa1c04…` | R-G10 | classes notify-not-resumed (notify-not-resumed 4590); F {F2a, F2b, F6}; listener sessions: client 26 LISTEN i=11; client 27 LISTEN i=13; client 28 LISTEN i=12 |
| `20261016T203843Z-C9-4-real-9f68293a` | V1 C9/baseline | 4 | `7e8c71b65c21e6e3…` | R-G10 | classes notify-not-resumed (notify-not-resumed 4204); F {F2a, F2b, F6}; listener sessions: client 26 LISTEN i=9; client 27 LISTEN i=10; client 28 LISTEN i=12 |
| `20261016T204512Z-C9-5-real-9f2b2904` | V1 C9/baseline | 5 | `08afe29e7811d27a…` | R-G10 | classes notify-not-resumed (notify-not-resumed 4882); F {F2a, F2b, F6}; listener sessions: client 26 LISTEN i=8; client 27 LISTEN i=7; client 28 LISTEN i=9 |
| `20261016T204734Z-C9-6-real-c5c9999a` | V1 C9/baseline | 6 | `4256064d45a5eb42…` | R-G10 | classes notify-not-resumed (notify-not-resumed 7092); F {F2a, F2b, F6}; listener sessions: client 26 LISTEN i=4; client 27 LISTEN i=10; client 28 LISTEN i=13 |
| `20261016T204952Z-C9-7-real-bfbc67fd` | V1 C9/baseline | 7 | `e9930fce8839c869…` | R-G10 | classes notify-not-resumed (notify-not-resumed 5735); F {F2a, F2b, F6}; listener sessions: client 26 LISTEN i=7; client 27 LISTEN i=8; client 28 LISTEN i=13 |
| `20261016T205220Z-C9-8-real-93c00b1b` | V1 C9/baseline | 8 | `866c4b386a27d96b…` | R-G10 | classes notify-not-resumed (notify-not-resumed 5449); F {F2a, F2b, F6}; listener sessions: client 26 LISTEN i=8; client 27 LISTEN i=9; client 28 LISTEN i=11 |
| `20261016T205448Z-C9-9-real-718765c7` | V1 C9/baseline | 9 | `4cb07c445a746602…` | R-G10 | classes notify-not-resumed (notify-not-resumed 934); F {F2a, F2b, F6}; listener sessions: client 26 LISTEN i=9; client 27 LISTEN i=10; client 28 LISTEN i=11 |
| `20261016T205709Z-C9-10-real-d9d891b4` | V1 C9/baseline | 10 | `b1ab3fad2a2fce9b…` | R-G10 | classes notify-not-resumed (notify-not-resumed 6810); F {F2a, F2b, F6}; listener sessions: client 26 LISTEN i=12; client 27 LISTEN i=8; client 28 LISTEN i=7 |
<!-- /K7-RECURRENCES -->

<!-- K7-REPLAY -->
## K7 replay (K7.4)

The package's replay command was executed once more in K7 (record `pkg-replay-FND-C9-2fe9a9fc` in `evidence/K7/commands.jsonl`; CommitProof `e8eca97`, contract 1.9.0 (ADR-0047), with ADR-0038's metrics exporter on every Multigres component, LIM-41). The plan replayed is this package's minimized plan, unchanged. Its five replays are stored as summaries in `evidence/K7/package-replays/`:

| Replay | Verdict | Classes | Seal root |
|---|---|---|---|
| `20261017T194328Z-C9-2-real-b8b35825` | FAIL | `notify-not-resumed` | `7804d50c1d9dd644…` |
| `20261017T194446Z-C9-2-real-b8b35825` | FAIL | `notify-not-resumed` | `5d47dd4d0fd7f3bc…` |
| `20261017T194604Z-C9-2-real-b8b35825` | FAIL | `notify-not-resumed` | `9d8acc42d8fbe5ac…` |
| `20261017T194722Z-C9-2-real-b8b35825` | FAIL | `notify-not-resumed` | `075f537076f768c6…` |
| `20261017T194840Z-C9-2-real-b8b35825` | FAIL | `notify-not-resumed` | `f1faec644cb7d565…` |

**5 of 5** show `notify-not-resumed` (§11.1 on these replays: REPRODUCED). The recorded classification stays REPRODUCED (5 of 5 recorded replays): this re-execution is reported beside it, not instead of it.
<!-- /K7-REPLAY -->

## Suggested direction (optional)

As PR #1550 proposes (S53): re-establish a session's notification stream after a stream error or a leader change without waiting for the client's next LISTEN/UNLISTEN, so that the loss is bounded to the reconnect window S19 documents.

## Limitations

- One listener per gateway on one channel, each on one connection for the whole run (LIM-24); a client that re-issues LISTEN would rebuild its stream and is not tested.
- A W6 wait that ends in an error drops the notifications it had collected (LIM-39); no K5 C9 result depends on it.
- One pinned commit, one host (LIM-01, LIM-02); no metrics exporter in this run (LIM-41).
- Seed 2's fault changes the leader: the stream stays on the old leader's pooler, now a standby.
- PR #1550 itself was read through GitHub's API in K6b (S53); whether it fixes this run's schedule is not tested.
