# Upstream check of the real-build findings (pre-K7 readiness, step 3; owner decision D-C)

Lookups: 39, all read-only, on 2026-10-16 between 00:43:59Z and 00:45:55Z (UTC; 2026-10-15 in the owner's time zone), listed one per line in `upstream/lookups.jsonl` (UTC date, method, URL, what was read, the saved result and its sha256) and recorded as `upstream-search` and `upstream-read` in `commands.jsonl` (`scripts/prek7-upstream.sh`). Methods: one partial `git clone` of `https://github.com/multigres/multigres.git` into the git-ignored `runs/.prek7/upstream` (the build's own clone and the pinned commit are untouched; no campaign's pin changes), local `git log`/`git diff` on it, and `GET` requests to the GitHub REST API (13 issue/PR searches, 7 issues/PRs read with comments and changed files). Nothing was written, posted or commented. The raw API responses are in `upstream/raw-responses.tar.zst` (the hashes in `lookups.jsonl` are of the uncompressed files); `upstream/search-summary.jsonl` lists every hit.

## What upstream main holds

`upstream/heads.txt`: `main` = `b2137f9ba33411b3548e87631cd9c69a6e27185f` (2026-10-15T09:18:58+02:00), 12 commits after the pinned `e3cbdbb` (2026-10-09), which is its ancestor. **None of the 12 touches** `go/services/multigateway/poolergateway/notification_adapter.go`, `go/services/multipooler/internal/pools/regular/regular_conn.go` or `docs/query_serving/listen_notify_design.md` (`commits-touching-files.txt` and the three `diff-*.txt` are empty): at main these files are byte-identical to the pinned commit. Of the 12, `e201703` (#1510, below) touches the gateway's statement-timeout path.

## Per finding

Known status uses the task's words (NEW / KNOWN / FIXED-UPSTREAM). The contract's taxonomy (§11.1) has `NEW`, `KNOWN_GAP` (an `alpha-known-gap` issue or a documented limitation, cited) and `KNOWN_FIXED` (historical control) and no value for "an open upstream fix"; the recorded finding records are not changed (the mapping is a proposal in `findings-readiness.md`).

### G10 — LISTEN subscriptions do not resume after a failover (FND-C9-40b6f05a, FND-C9-2fe9a9fc, FND-C9-90da5532)

**KNOWN — fix proposed upstream, not merged; not FIXED-UPSTREAM.**

- Design says / system did (K5 triage): S19 states the stream "is automatically re-established"; the runs' listeners received nothing committed after the first fault; at e3cbdbb `notificationSession` rebuilds its gRPC stream only in `update()` (a client `LISTEN`/`UNLISTEN`, `restartLocked → startLocked`), and `recvLoop` ends on a stream error without re-establishing anything.
- Upstream: **PR #1550**, open, created 2026-10-15T01:06:33Z (after K5's runs of 2026-10-13), "fix(multigateway): re-establish LISTEN notification stream after leader change". It describes the same mechanism: "A client that runs `LISTEN` and then idles stops receiving notifications after a leader change … the stream is only replaced when the client's next `LISTEN`/`UNLISTEN` fails … The background reconnect described in the design doc was dropped in #1222" (#1222, merged 2026-07-08, before the pinned commit — S19 was stale at the pin). It also names a second, pooler-side cause (a deferred `Unsubscribe` on a stopped listener waits forever, so the stream never ends). It changes `notification_adapter.go` (+149 −58), `pubsub/listener.go`, `grpcpoolerservice/service.go`, the design doc and adds `listen_notify_failover_test.go`.
- Pinned vs main: `notification_adapter.go` is identical; the fix is not on main at `b2137f9`.
- No `alpha-known-gap` issue concerns LISTEN (the 8 items: #1059, #1060, #1065, #1126, #1127, #1128, PRs #1057, #1068). S19's documented limitation covers only reconnect-window loss (measured as `notify-window-loss`, not reported).

### G2 — reads of writes that later vanish (FND-D1-fb8d0f8c, FND-D1-2e860fd2, FND-D1-a8eb9857, FND-C2-2a534585)

**NEW — no upstream report or fix found.**

- Design says / system did (K4 triage): under F1c, COMMITs on the cut-off leader wait for a synchronous standby; a client giving up after P01 ends the request context; the pooler cancels the backend (`regular_conn.go`: `execWithContextCancel` → on `ctx.Done()`, `handleContextCancellation` → `adminPool.CancelBackend(ProcessID)`, i.e. `pg_cancel_backend`, force-closing the connection if that fails, then `drainOpAfterCancel`); PostgreSQL completes a commit whose synchronous-replication wait is cancelled locally with a WARNING (S61), other sessions read it, a failover rewinds it away. No acknowledged write was lost (C1 valid).
- Pinned vs main: `regular_conn.go` is identical; `execWithContextCancel` and `handleContextCancellation` behave the same at `b2137f9`.
- **#1510** (merged 2026-10-14, `e201703`, on main): the gateway now waits for the pooler's cleanup before reporting a *statement timeout*; it states that "explicit cancellation and client disconnect otherwise retain their existing immediate transport cancellation behavior". It changes when the timeout is reported, not whether the pooler cancels the backend; it does not address the visibility of a cancelled sync-rep commit.
- **#656** (open RFC, 2026-02-24, "Query Execution Timeouts") states the intended design: on client disconnect the pooler "detects cancellation, sends PG cancel, recycles connection", and names "queries waiting on Sync ACKs" among the queries to bound. It does not discuss that cancelling a COMMIT's sync-rep wait leaves the commit visible locally.
- **#1455** (merged 2026-09-25, at the pin): LeaderStuck detection through a quorum-commit watermark — failure detection, not commit visibility. **#601** (merged 2026-02-11): connection lifetime tied to the pool, not the caller's context — not the cancel path.
- Searches for `canceling wait for synchronous replication` (0 hits), `synchronous replication cancel`, `cancel commit`, `rewind lost`, `read after failover`, `pg_rewind` found nothing about reads of locally committed, unreplicated values.

### G12 — fair-share starvation during an injected failover (FND-C11-17a344c1)

**NEW — no upstream report found** (and, from contract 1.2.0, judged by ADR-0037's rule, under which C11 seed 2's storm windows would not fail; the recorded verdict stays as recorded).

- Upstream design: #546 (merged, "dynamic fair share pool allocation": max-min fairness over `--connpool-global-capacity`). Searches `fair pool`, `per-user pool`, `overload` found no issue about steady-role refusals during a failover inside a storm; the 120 refusals (57P03, 28P01, 57P01) were the restart of PostgreSQL that the plan injected.

## Side result (ADR-0038)

**Issue #406** (open, 2025-12-30, "feat(telemetry): Add always-on Prometheus /metrics endpoint for debugging") quotes the same `TODO(dweitzman)` in `telemetry.go` that ADR-0038 cites: upstream itself records that no component serves `/metrics` by default — independent support for ADR-0038's correction of P23.

## Limitations

Searches are keyword searches of public issues and PRs at one moment; a report worded differently, a closed duplicate the search ranks low, private reports or the internal Multiquake test (S29) would not be found. "Identical at main" is a file comparison, not a run of main.
