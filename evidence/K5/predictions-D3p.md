# K5 — Predictions for D3′, written before any D3′ run

Written and committed before the first D3′ run (ADR-0033, contract 1.0.10; plan identifier `D3p`), as the owner's decision of 2026-10-14 requires. When this file was committed:

- no D3′ plan had been generated or run;
- the plan generator had no D3′ campaign.

Its commit precedes every `*-D3p-*` run in `evidence/K5/runs/`: each run's manifest names the commitproof commit it ran on, and that commit descends from this one.

## Schedule (CONTRACT §9.2, ADR-0033)

Profile `buffered`; W1 autocommit appends (clients 0–3, P03) and W4 inserts at 25 ms (clients 8–9). Seeds 1–5, on M3 and on the real build.

Three steps: the first at t = 10 s, each later one 15 s after the previous step's last heal is confirmed. Each step:

1. F7(base-follower:0), then F7(base-follower:1): 50 ms egress delay on each follower node (P18);
2. F2a(leader), as soon as the second F7 is confirmed;
3. heals: both F7s at once, then F2a (PostgreSQL restarted by its pooler).

The gateway buffers only the first kill of each run (1 min timing guard, ADR-0029).

## Predictions

### M3 on D3′: FAIL with C2 `g1a` in ≥ 4 of 5 runs (DETECTED)

Expected sequence in each run:

1. While both F7s hold, every commit waits ≥ 50 ms for its standby acknowledgement, with its WAL already flushed on both followers. Each client is in that wait most of the time (the statements otherwise take 3–8 ms, as in D3). At the first kill each W4 client (8, 9) therefore most likely has an insert in that state.
2. Its backend exits without a reply when the postmaster dies, and the gateway gets an error after execution.
3. M3 buffers it. The followers are responsive, so the first failover of the run recruits them at once (the killed leader cannot be recruited, record `d3-diagnosis`) and elects one that holds the row.
4. When that leader serves, the buffer drains and M3 retries the insert: `23505` on each of the four attempts. The client sees `fail` (§6.2 row 4) while the row is present in the final state: C2 `g1a`.

Further expectations:

- **Interrupted appends:** M3 retries them successfully (an upsert). Their elements appear twice in final reads, which no checker judges (LIM-22). C3 `duplicate-elements` is not expected in any run: as in D3, the W1 variant issues no reads.
- **Second and third kills:** not buffered. M3's three immediate retries reach the dead PostgreSQL and end in a `57P03` other than "no writable primary" (`info`, as at D3's later kills), or in "no writable primary" (`fail`, another `g1a`). These are additional chances only.

### Real build on D3′: PASS in 5 of 5

- The interrupted statements end `info` (`XX000`, §6.2 row 11, as in the D3 real runs); present or absent, both are allowed.
- Statements sent after the first kill fail before executing and are buffered, then run once on the new leader.
- No `ok` write is lost: every `ok` commit was flushed by a standby, and the elected follower holds all of them.

If the real build FAILs, that is a finding: it is recorded verbatim, minimized, replayed 5× and triaged (K5.5).

## Risks to the M3 prediction, stated in advance

1. **The failover is too slow.** If no leader serves before the interrupted inserts' P01 deadline (10 s after their invoke, about 9.9 s after the kill), their retries end as client timeouts (`info`) and the first kill does not detect. In the ten D3 runs a new leader was observed 1.9–7.0 s after the first kill (record `k3obs`).
2. **No insert is in flight.** If neither W4 client has an insert waiting at the first kill, only the unbuffered later kills can detect.
3. **Later appointments are delayed.** After the later kills, appointments may be delayed by recruitment backoff (up to 34.8 s after a kill in D3). This affects only the additional chances.

## What would show the mechanism (observations, not verdicts)

These are read after the runs, from histories, observations and logs. They explain a verdict; they never decide one (ADR-0016).

- Ok W4 and W1 latencies of about 50–60 ms while the F7s hold, against 3–8 ms outside them.
- At each kill: the statements in flight and how each ended (`XX000` `info` on the real build; on M3, the `g1a` insert's `23505` completion after the new leader's first SERVING broadcast).
- Multiorch's first appointment after the first kill: both followers recruited, the killed leader "unknown WAL position", a follower promoted.

## Decision rule (fixed in advance)

- **Detection.** K5.4 counts M3 as DETECTED iff ≥ 4 of the 5 valid D3′ runs on M3 are FAIL with C3 `duplicate-elements` or C2 `g1a` (§9.2's two classes, ADR-0030; P14). The superseded D3 runs (M3 2/5, real 5/5 PASS) stay recorded and do not count.
- **Otherwise:**
  - M3 is UNDETECTED with every D3 and D3′ run;
  - D3′ is not changed again;
  - the investigation is reported;
  - K5 stops BLOCKED on K5.4 ("one amendment only", ADR-0033).
- **Real build:** its five D3′ results are recorded whatever they are.
- **Follow-ups:** every D3′ FAIL run is minimized (target `duplicate-elements` if present, else `g1a`) and its minimal plan replayed 5× (K5.5).
