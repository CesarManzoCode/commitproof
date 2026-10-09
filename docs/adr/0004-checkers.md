# ADR-0004 — Reimplement an Elle subset in Go, wrap Porcupine, use elle-cli only as an oracle

Status: Accepted (K0, contract 1.0.0)

## Context

Needed checkers: lost acknowledged writes, aborted reads, list-append dependency cycles (G0, G1a/b/c, G-single, G-nonadjacent, G2-item, duplicates, incompatible order), linearizability of registers, bank invariants, CDC gap/phantom/duplicate. Facts: Elle is Clojure under EPL-2.0 or GPL-2.0-or-later with Classpath exception (S70); elle-cli 0.1.11 wraps Elle 0.2.7 and needs Java 21 (S73); Porcupine v1.3.1 is Go, MIT, with partitioning and timeouts (S74); a Go port of Elle exists inside PingCAP tipocket, derived from Elle's code and carrying Elle's license (S75).

## Decision

- **C3 (list-append):** reimplement in Go from the Elle paper (S71) and Adya's definitions (S72), without reading or copying Elle's or tipocket's code into the repository. Only the subset in CONTRACT §10.3.
- **C4 (linearizability):** wrap Porcupine v1.3.1 as a library with a register+CAS model partitioned by key.
- **C1, C2, C5, C6, C7, C8, C9:** written in Go; small set-difference and invariant logic.
- **Oracle:** elle-cli built from pinned commit inside a pinned `clojure` container, run on exported histories in K2 (agreement gate K2.4) and on every C3 finding in K7. Never vendored, never redistributed.

## Alternatives rejected

- **Run Elle as the checker (JVM in the loop).** Adds a JVM to every run and to CI; makes the verdict depend on a tool whose license differs from the project's; the brief asks for a Go project in Multigres' language.
- **Vendor tipocket's Go Elle.** Derived from EPL/GPL code (licensing ambiguity for an Apache-2.0 project), unmaintained relative to Elle 0.2.7, and a reviewer would still ask whether it is correct.
- **Reimplement linearizability checking.** Porcupine is the established Go implementation, MIT-licensed; wrapping it is cheaper and more trustworthy.
- **Knossos.** JVM, slower than Porcupine for register histories.

## Consequences

- K2 must show the reimplementation agrees with Elle on hundreds of histories; disagreements go to a registry with evidence.
- Elle's full feature set (predicates, rw-register version inference, all consistency models) is not reimplemented; LIMITATIONS lists the subset.

## Sources

S70–S75.
