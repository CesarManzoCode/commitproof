# CommitProof — all entry points are make targets (CONTRACT §13).
SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

# ADR-0014: any host Go >= 1.21 downloads and uses exactly go1.26.9.
export GOTOOLCHAIN := go1.26.9

EVIDENCE_K1 := evidence/K1

.PHONY: prereq-check
## prereq-check: verify docs/PREREQUISITES.md; writes evidence/K1/prereq.json; exit 3 on any FAIL
prereq-check:
	@scripts/prereq-check.sh $(EVIDENCE_K1)/prereq.json
