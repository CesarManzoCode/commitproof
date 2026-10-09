# ADR-0003 — History as canonical, hash-chained JSONL in sealed run directories

Status: Accepted (K0, contract 1.0.0)

## Context

The history is the evidence. It must be immutable once written, verifiable by a reviewer without CommitProof's runner, readable by other tools (elle-cli takes JSON), and byte-stable so reports regenerate byte-for-byte (K6).

## Decision

- One JSON object per line, canonical (sorted keys, integers only, no whitespace), Jepsen operation semantics (`invoke`/`ok`/`fail`/`info`, process retirement after `info`), nemesis events in the same stream, monotonic nanosecond timestamps from a single writer (ADR-0018).
- Each line carries `prev` = sha256 of the previous line; the run directory is sealed with per-file hashes and a Merkle root, then made read-only.
- Completion classification is a closed table (CONTRACT §6.2), the single most important source of false positives (R6).
- Observations (system internals) go in a separate chained stream, never read by checkers (ADR-0016).

## Alternatives rejected

- **EDN** (Jepsen's native format): needs a Clojure-side reader for every consumer; JSON is accepted by elle-cli.
- **SQLite or Parquet**: not line-diffable, harder to hash canonically, extra dependencies.
- **Wall-clock timestamps**: subject to NTP adjustments during a run; monotonic time is what ordering needs.
- **Unchained JSONL with only a file hash**: detects tampering but not where; the chain localizes it and supports partial verification.

## Consequences

- Any consumer verifies the seal first and refuses otherwise.
- Exporters to Elle JSON and Porcupine operations are pure transformations.

## Sources

S70, S73, S74; Jepsen history conventions.
