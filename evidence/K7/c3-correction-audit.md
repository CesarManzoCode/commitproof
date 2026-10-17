# K7 — what the C3 correction changes in the evidence and its claims

The owner's decision on ADR-0047 asks for an audit of the claims affected by the C3 correction. The defect, found during K7's matrix and fixed in `32b660a` (`evidence/K7/NOTES.md` §3): C3 built a reported cycle without one of its transactions. The anomaly's `ops` and `explanation` were therefore wrong, and the cycle-shape step ran on the shortened list. Whether a cycle exists, and hence any verdict, did not depend on it.

## 1. Re-checks

| What | Command (record in `evidence/K7/commands.jsonl`) | Result |
|---|---|---|
| The K2 corpus (807 entries, 350 compared with elle-cli), the K5 corpus (157) and the G12 corpus, with the fix | `make ci-checkers` (`ci-checkers-c3fix`, exit 0) | No expected classification changed; 0 disagreements with elle-cli |
| The 15 K7 runs sealed before the fix (in `runs/k7`, before any was committed) | `scripts/k7-recheck.sh runs/k7 runs/.k7/recheck-c3fix-before` (`recheck-c3fix`, exit 0) | 0 verdicts and 0 class sets changed; 1 `checks.json` changed (V1 C2 seed 3's cycle explanations), committed in its corrected form |
| Every committed run held in full that has a C3 result: 172 runs of K3, K4, K5, K6, pre-K7 and K7 under `evidence/` (none under `findings/`) | `scripts/k7-c3-audit.sh runs/.k7/c3audit` (`c3-audit-2`, exit 0; stdout sha256 `7ed6d9a4baaace4db7a02a7c59482acce8bf7eb9890166d26e594944caecb03c`) | 172 compared, 0 errors: **0 runs with a changed verdict, C3 validity, anomaly classes, class totals or observation classes**; 166 identical; 6 runs whose cycles changed only their `ops` and `explanation` (table below) |

`c3-audit-2` re-checks a scratch copy of each run with this commit's cpcheck and never writes into `evidence/`. The first attempt, `c3-audit`, printed C3's `valid: false` as `null` (a `jq` `//` mistake, fixed in the script). Its counts were the same.

## 2. The six runs whose committed cycle reports name one transaction too few

All six are K4 runs. Every changed entry is a cycle; the classes (`-process`, `-realtime` and SI `G2-item` observations, which no verdict reads, §10.3) are unchanged.

| Run (evidence/K4/runs) | Verdict (unchanged) | Cycles changed | Classes of the changed cycles | Example: committed → corrected |
|---|---|---|---|---|
| `20261011T215900Z-D1-1-M1-59a95336` (D1, M1) | FAIL | 10 | G0, G1c, G-single, G-nonadjacent, G2-item (`-process`, `-realtime`) | `i=7249 -rw-> i=7356 …` → `i=7249 -rw(k=47)-> i=7334 -ww(k=47)-> i=7356 …` |
| `20261011T220810Z-D1-5-M1-3319a30c` (D1, M1) | FAIL | 6 | G-single, G-nonadjacent, G2-item (`-process`, `-realtime`) | `i=5094 -rw-> i=5107 …` → `i=5094 -rw(k=30)-> i=5095 -process-> i=5107 …` |
| `20261011T221728Z-D1-4-real-ad3fca18` (D1, real; FND-D1-a8eb9857's original run) | FAIL | 2 | G-single-realtime, G2-item-realtime | `i=2481 -rw-> i=2438 …` → `i=2481 -rw(k=13)-> i=2425 -wr(k=13)-> i=2438 …` |
| `20261011T231604Z-C3-2-real-52940370` (C3, real) | PASS | 4 | G2-item (SI observation), G2-item-process, G2-item-realtime | `i=973 -rw(k=5)-> i=974 -rw-> i=973` → `i=973 -rw(k=5)-> i=974 -rw(k=1)-> i=972 -ww(k=1)-> i=973` |
| `20261012T022121Z-D1-4-real-eb537f55` (replay of FND-D1-a8eb9857's minimized plan) | FAIL | 3 | G-single (`-process`, `-realtime`), G-nonadjacent-realtime | `i=1660 -rw-> i=1668 …` → `i=1660 -rw(k=16)-> i=1662 -ww(k=16)-> i=1668 …` |
| `20261012T022439Z-D1-4-real-eb537f55` (replay, same plan) | FAIL | 3 | as above | as above |

These `checks.json` files are closed K4 evidence and keep their recorded text. Copies of them carry the same text: `findings/FND-D1-a8eb9857/checks.json`, which §11.4 and cpgate K7 require to be byte-identical to the original run's. K7's oracle records of M1's D1 runs (`evidence/K7/oracle/*-D1-*-M1-*.checks.json`) also carry it: they were written by `cpcheck check --elle-oracle` on 2026-10-16 at 04:12–04:17Z, before the fix, and D1 seeds 1 and 5 hold the cycles above.

## 3. The claims reviewed

| Claim (where) | Reads | Outcome |
|---|---|---|
| Every verdict and k/N of K4, K5, K6 and K7, and every §12.3 statement (closures, README, `claims.yaml`) | verdicts, class sets | Unaffected: 0 changes in 172 runs held in full. Runs held as summaries keep their recorded `checks.json`; the defect could not change whether a cycle is found (NOTES §3), so their classes stand. |
| K4 NOTES "G5: no G0–G2 cycle reported by C3 on the real build" | C3 anomalies on the real build | Unaffected: no C3 cycle anomaly appears in any real-build K4 run, before or after (the only real-build cycles are observations). |
| K2.1, K5.1 and the G12 corpus (CLM-0002, CLM-0004) | class sets | Unaffected: `ci-checkers-c3fix`. |
| The findings packages (README, `finding.json`) | anomaly classes; no README quotes a C3 cycle's operations | Unaffected. `findings/FND-D1-a8eb9857/README.md` now states, under Limitations, that its `checks.json`'s two realtime observations name one transaction too few. |
| K7.3 (`evidence/K7/oracle/`): M1's D1 records | elle-cli against C3 on `incompatible-order`; observations are not compared | Unaffected; the copies' cycle explanations are pre-fix (§2). |
| FND-C2-c7475a73 (K7's G5 finding) and K7.3's record of its run | the run's cycles | Written from the corrected `checks.json` (`recheck-c3fix`). The C3 G-single matches elle-cli's (i=3230 -rw(k=13)-> i=3178 -ww(k=12)-> i=3230). |
| LIMITATIONS | — | LIM-44 added: the checks.json files written before `32b660a` keep cycle reports that name one transaction too few; classes and verdicts unchanged. |

Nothing else in README, LIMITATIONS, NOTES, `claims.yaml` or the findings quotes a C3 cycle's operations or explanation: `grep -rn 'cycle i='` over those files finds only this audit, NOTES §3 and FND-C2-c7475a73.
