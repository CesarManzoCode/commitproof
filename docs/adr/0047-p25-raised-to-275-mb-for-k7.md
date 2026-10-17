# ADR-0047 — P25 raised to 275 MB; K7 resumes from its store step, with the cited D3′/D5 runs in full (owner decision on K7's BLOCKED closure)

Status: **Accepted by the owner (K7, contract 1.9.0).** The owner decided K7's BLOCKED closure (`evidence/K7/closure.yaml` at `fb5d22f`, option 1) on 2026-10-16 (see "Owner decision"). P25 is a fixed parameter (§5.7) whose change relaxes a requirement, so only the owner can make it (§18). It is a changed requirement, hence the MINOR version, as ADR-0034 did for 200 → 250 MB. K7's matrix is complete: no matrix run is made under this version (prompts/K7.md: the contract version is not bumped while the matrix runs).

## Owner decision

Verbatim:

> I approve increasing P25 from 250 MB to **275 MB (275,000,000 bytes)** for K7 through a new ADR, following the same formal process used by ADR-0034.
>
> Preserve the complete D3′/D5 evidence, the existing sealed runs, and all required findings and replay artifacts. Do not weaken any gate, threshold, negative control, reproduction requirement, or evidence-integrity rule.
>
> Resume K7 from the current store checkpoint. Complete the remaining finding triage and packages, re-execute the required package replays, audit claims affected by the C3 correction, and finish all K7 acceptance criteria.
>
> Measure the actual final storage usage and enforce the new P25 limit. If the final evidence still exceeds 275 MB, report BLOCKED rather than dropping required evidence.
>
> Run the required CI checks and gates, including the K7 negative controls. If all criteria pass, create the local annotated v1.0.0 tag and produce the final DONE closure report.
>
> Do not push, publish findings, or contact upstream. Preserve all existing valid evidence.

## Context

- K7 stopped BLOCKED after its store step (record `store` in `evidence/K7/commands.jsonl`, `scripts/k7-store.sh runs/k7 evidence/K7/runs --pending 0`, exit 4): every K7 run stored once (6 in full, 206 summaries), P25 count 246 678 804 B, ADR-0045 §5's projection 251 678 804 B > 250 000 000 B (`evidence/K7/NOTES.md` §8).
- Measured on the closure commit `fb5d22f` (`find evidence -type f -printf '%s\n' | awk '{s+=$1} END {print s}'`; no run archive under `findings/`): **246 729 547 B**.
- What K7 still has to store, measured or estimated on that tree:

| What | Bytes |
|---|---|
| K7.4: each package's P13 replay command executed once more, 14 packages (9 recorded + 5 new findings) × 5 replays stored as summaries (stub + checks.json; mean of K7's 209 stored summaries 85 420 B) | ≈ 5 980 000 |
| The cited D3′ and D5 real-build runs in full (10 archives in `runs/k5-full/`, `stat -c %s`, ADR-0045 §5) | 11 832 720 |
| K7's remaining records and text (results.json, runs-index.json, campaign pages — 253 631 B in a trial render —, triage records, five packages' text, claims.yaml, logs) | ≤ 5 000 000 (ADR-0045 §5's allowance) |
| **Projected end of K7** | **≈ 269 542 267** |

## Decision

- **P25 = ≤ 275 MB** (275 000 000 bytes) total under `evidence/` and the run archives under `findings/` (bytes/10⁶; the counting of ADR-0039 is unchanged).
- **K7 resumes from its store step.** `scripts/k7-store.sh` refuses (exit 2, nothing written) a store that would make the count exceed 275 MB, and after every store step exits 4 when ADR-0045 §5's projection exceeds 275 MB; that stop is BLOCKED, reported with its numbers, as before. Nothing already stored is dropped or moved to a summary.
- **The cited D3′ and D5 real-build runs are committed in full** under `evidence/K7/cited/` at K7's end (ADR-0045 §5, decision 2), each archive's manifest, plan and seal byte-identical to K5's stub, if the count stays within 275 MB with them; K5's stubs are not touched. If it does not, K7 stops BLOCKED instead of committing them partially or as summaries (the owner asks to preserve them).
- **The final count is measured and enforced**: after the last evidence is written and before the closure commit, the P25 count of the tree is recorded (`p25-final`, exit 4 if it exceeds 275 MB); a count above 275 MB is BLOCKED.
- **Everything else is unchanged**: ADR-0045's repeat predicates and storage of repeats (summaries), the new findings' minimization, replays and packages, every gate criterion, threshold, negative control and reproduction requirement.

## Alternatives rejected

- **260 MB** (the closure's other figure under option 1): enough for K7 without D3′/D5 in full; the owner chose to keep those runs in full, and 260 MB would not hold them.
- **The closure's options 2–4** (replays as checks.json only, one replay per package, reclassifying new failures as repeats): each removes evidence or changes a requirement the owner did not approve, and none fits 250 MB on its own (closure.yaml at `fb5d22f`).

## Consequences

- **Contract 1.9.0:** §5.7 P25 reads ≤ 275 MB; §13 states that the D3′/D5 runs are committed in full under it. `params.EvidenceBudgetMB` (the parameters-table test ties it to §5.7) and `scripts/k7-store.sh` follow.
- K7's matrix plans keep 1.8.1 (`k7MatrixVersion`); the package replays reuse their packages' plans (`commitproof replay`), so no plan changes. The runner records 1.9.0 in the manifests of runs made after this ADR (the K7.4 replays).
- The runner image is rebuilt from the commit carrying this change (images.lock.json records the commit its runner image was built from).
- ADR-0034's 250 MB and ADR-0045 §5's figures read 275 MB from this ADR on; ADR-0045 is otherwise unchanged.

## Sources

CONTRACT §5.7 (P25), §13, §15 K7, §18; ADR-0034, ADR-0039, ADR-0045; `evidence/K7/closure.yaml` and `NOTES.md` §8 at `fb5d22f`; record `store` (evidence/K7/commands.jsonl).
