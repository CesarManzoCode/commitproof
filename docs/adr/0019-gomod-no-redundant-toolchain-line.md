# ADR-0019 — go.mod has no `toolchain` line equal to its `go` line

Status: Accepted (K1, contract 1.0.1). Amends ADR-0014 and CONTRACT §14.

## Context (defect)

CONTRACT §14 and ADR-0014 require CommitProof's `go.mod` to contain both `go 1.26.9` and `toolchain go1.26.9`. The Go command at the pinned toolchain rejects that combination: a `toolchain` line equal to the `go` line is redundant, `go mod tidy` removes it, and every build in the default `-mod=readonly` mode fails until it is removed.

Evidence (minimal module, 2026-10-10, host `go1.27.2` downloading `go1.26.9` through `GOTOOLCHAIN`):

```
$ cat go.mod
module example.com/m

go 1.26.9

toolchain go1.26.9
$ GOTOOLCHAIN=go1.26.9 go build ./...
go: updates to go.mod needed; to update it:
	go mod tidy
exit=1
$ GOTOOLCHAIN=go1.26.9 go mod tidy -diff
--- current/go.mod
+++ tidy/go.mod
@@ -1,5 +1,3 @@
 module example.com/m

 go 1.26.9
-
-toolchain go1.26.9
exit=1
$ # same module without the toolchain line
$ GOTOOLCHAIN=go1.26.9 go build ./...
exit=0
$ GOTOOLCHAIN=go1.26.9 go version
go version go1.26.9 linux/amd64
```

The same failure was observed in the CommitProof repository itself (`go build ./...` and `go test ./...` exit 1 with the line present).

## Decision

- `go.mod` declares `go 1.26.9` and no `toolchain` line. The `go` directive already requires toolchain ≥ go1.26.9 and fixes the language version.
- The exact toolchain stays pinned the way ADR-0014 already pins it in practice: the Makefile and the CI workflow export `GOTOOLCHAIN=go1.26.9`, so every build uses exactly go1.26.9 whatever the host Go is.
- A unit test (`internal/params`, part of `make ci-unit`) asserts `go 1.26.9` in `go.mod` and `GOTOOLCHAIN := go1.26.9` in the Makefile, so the pin cannot drift silently.

## Alternatives rejected

- **Keep both lines and run builds with `-mod=mod`.** The go command would rewrite `go.mod` on every build (removing the line again); builds would mutate the repository.
- **`go 1.26` + `toolchain go1.26.9`.** Accepted by the go command, but lowers the minimum language/toolchain version the module declares; with `GOTOOLCHAIN=auto` a newer host Go would still be used, so the `toolchain` line does not pin anything that `GOTOOLCHAIN=go1.26.9` does not already pin.

## Consequences

- Nothing about which compiler builds CommitProof changes: go1.26.9 via `GOTOOLCHAIN` (Makefile, CI).
- `prompts/K1.md` still mentions `toolchain go1.26.9`; the contract wins (AGENTS.md §1) and the K1 closure report lists the deviation.

## Sources

S80; Go command behaviour observed above (go1.26.9).
