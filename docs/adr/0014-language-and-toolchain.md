# ADR-0014 — English documentation; Go 1.26.9 via GOTOOLCHAIN

Status: Accepted (K0, contract 1.0.0)

## Context

The audience is the Multigres team (English, Go). Multigres `go.mod` says `go 1.26`; its CI resolves the latest 1.26 patch; the latest on 2026-10-09 is go1.26.9 (S80). Arch ships go 1.27.1 (S87).

## Decision

- All repository text in English. Chat with the owner may be in Spanish.
- CommitProof `go.mod`: `go 1.26.9` and `toolchain go1.26.9`; Makefile exports `GOTOOLCHAIN=go1.26.9` so any host Go ≥ 1.21 downloads and uses exactly 1.26.9 (from the Go module proxy, $0). Multigres images are built with `golang:1.26.9-alpine` pinned by digest.

## Alternatives rejected

- **Go 1.27:** newer than Multigres' toolchain line; importing Multigres packages and building its images should use the same compiler line it targets.
- **Host Go as-is:** Arch updates would silently change the compiler between sprints.

## Sources

S02, S80, S87.
