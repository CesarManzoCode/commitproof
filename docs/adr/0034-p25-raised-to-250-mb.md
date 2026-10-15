# ADR-0034 — P25 raised to 250 MB; K5 stored as ADR-0030 prescribes (owner decision on ADR-0032)

Status: **Accepted by the owner (K5, contract 1.1.0).** The owner decided ADR-0032 on 2026-10-14 (see "Owner decision"). P25 is a fixed parameter (§5.7) whose change relaxes a requirement, so only the owner can make it (§18). It is a changed requirement, hence the MINOR version.

## Owner decision

The decision's storage item, verbatim:

> 2. Evidence budget: raise the limit to 250 MB, with an ADR amending it.
>    - First try lossless compression of the K5 runs. Raise the limit only if they still do not fit.
>    - Replace the K5 stubs with full runs as ADR-0030 requires, so the gate re-checks K5.3 and K5.5 from git.
>    - The D3′ runs count toward the 250 MB. If the total exceeds 250 MB, stop and report BLOCKED with numbers.
>    - Do not delete runs/k5-full/.

## Context: lossless compression tried first

Record `compress-measure` (`scripts/k5-compress-measure.sh runs/.k5-compress-measure`, stdout sha256 `5b93e27a…f4d`) measured the same bytes on a staged ADR-0030 copy, built outside `evidence/` from the committed stubs and the local full archives:

| What | As stored (zstd -19, the format cpgate reads) | `zstd --ultra -22 --long=27` (cpgate reads it) | xz -9e / one solid archive (cpgate reads neither) |
|---|---|---|---|
| The 12 full archives ADR-0030 requires | 19 981 318 B | 19 667 139 B (−1.6 %) | 19 405 960 B (−2.9 %, xz) |
| The other 144 runs as stubs | 6 879 969 B | 6 879 533 B (−0.0 %) | 1 466 740 B (all stubs in one archive) |

`evidence/` outside K5 holds 183 082 177 B; the staged K5 copy holds 28 566 935 B. Even with the best figure in every row, including formats cpgate cannot read, `evidence/` would hold **205 660 525 B, over the 200 MB budget**, before any D3′ run. The 12 full archives alone (≥ 19.4 MB) exceed the 16.92 MB P25 leaves. Lossless compression therefore does not make ADR-0030's storage fit.

## Decision

- **P25 = ≤ 250 MB total under `evidence/`** (bytes/10⁶ of the files under `evidence/` in git; the counting is unchanged).
- **K5 is stored as ADR-0030 prescribes:**
  - every C10 primary run, and the primary and minimized runs of every real-build FAIL, in full;
  - every other run as a stub;
  - the store step moves the 12 full archives from `runs/k5-full/` in place of their committed stubs, and stores any new run (the D3′ runs included) by the same rule.
- **zstd -19 is kept**, the format the store step writes and cpgate reads. The stronger level saves 1.6 % at most; the other formats would need a new reader in cpgate.
- **The D3′ runs count toward the 250 MB.** The store step refuses (exit 2, nothing moved) to make `evidence/` exceed 250 MB; that refusal is a BLOCKED condition, reported with its numbers.
- **`runs/k5-full/` is kept** on the owner's machine; nothing in it is deleted.

## Alternatives rejected

- **Stronger lossless compression only:** measured above, insufficient.
- **ADR-0032's options 2–5** (C10 primaries as stubs, all stubs, partial archives, storage outside git): each leaves a gate criterion unable to re-read its runs from git, or publishes; the owner chose option 1.

## Consequences

- **Contract 1.1.0:** §5.7 P25 reads ≤ 250 MB. `params.EvidenceBudgetMB` and the K5 store and staging scripts follow, and the parameters-table test ties them.
- **ADR-0032** is decided (option 1, by this ADR).
- **LIM-37** (K5 runs committed as stubs) is resolved once the store step has run with the ADR-0030 policy and cpgate K5 re-reads K5.3 and K5.5 from git.
- **What remains for K6 and K7** is 250 MB minus what `evidence/` holds at K5's close (stated in the K5 closure). K7's matrix still needs §13's summarising policy for uncited runs (ADR-0032).

## Sources

CONTRACT §5.7 (P25), §13, §17, §18; ADR-0010, ADR-0030, ADR-0032; record `compress-measure` (evidence/K5/commands.jsonl).
