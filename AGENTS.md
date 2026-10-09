# AGENTS.md — Permanent instructions for coding agents in this repository

CommitProof is an independent, Jepsen-style verifier written in Go that tries to break Multigres' public durability guarantees (an acknowledged commit survives failovers, split brain and membership changes) using a reproducible local cluster, seeded fault schedules, history checkers and deliberately broken builds (mutants). The normative specification is `docs/CONTRACT.md`. Read it before changing anything.

## 1. Authority and reading order

1. `docs/CONTRACT.md` (normative; its version is in its header).
2. `docs/adr/` (decisions and their reasons; index in `0000-index.md`).
3. `docs/RESEARCH.md` (sources; every FACT cites an `Sxx`).
4. The sprint prompt in `prompts/K<n>.md`.

If a prompt and the contract disagree, the contract wins; record the disagreement under `deviations_from_prompt` in the closure report. If the contract is wrong, apply the BLOCKED protocol (§3).

## 2. Ground rules

- **One sprint = one K.** Do exactly the sprint you were given, end to end. Do not start the next sprint. Do not expand scope with features the contract does not ask for.
- **Allowed stops: only two.** (a) The sprint is DONE: gate passed with negative controls, closure report written, commits made. (b) BLOCKED per §3, with evidence. Stopping midway to ask whether to continue, to offer options, or to summarize progress is not allowed.
- **Never weaken verification.** Do not lower thresholds, repetitions or deadlines; do not skip, mark expected, or delete a failing test, criterion, negative control, run or finding; do not reclassify a real-system failure as a harness defect without the §6.2 review evidence the contract requires. A failing gate is either fixed by fixing the code or reported as BLOCKED.
- **Never hide results.** Results from the real Multigres build are recorded verbatim whatever they are. A violation found in Multigres is a result, not a problem to make go away.
- **Never trust the system under test for verdicts.** Verdicts come from `cpcheck` on sealed histories and `cpgate` on evidence files (CONTRACT §5.3, ADR-0016, ADR-0017).
- **Never generate expected outputs by running the code under test.** Expected classifications for synthetic histories are written from the definitions (Adya, Elle paper, contract §10), by hand or by an independent generator.
- **Sealed run directories are immutable.** Never edit a file under a sealed `runs/` or `evidence/` run directory. Re-run instead.
- **Fail closed.** A missing prerequisite, an unconfirmed fault, an unverifiable seal, an UNKNOWN checker result: never treated as success.
- **Evidence rule** (CONTRACT §17): every claim in README, NOTES, LIMITATIONS, findings or closure reports has `claim → contract section → target → command → artifact → sha256 → result → limitations`, or is written as a hypothesis. Notes always separate "design says" (with source ID) from "system did" (with artifact).
- **Values come from the contract.** Numbers, ports, flags, versions and digests come from CONTRACT §5.7 and §14 (or the files they generate). If reality differs from the contract (a flag missing at the pinned commit, a digest moved), that is a contract defect: §3.

## 3. BLOCKED protocol (verbatim from CONTRACT §18)

If you are blocked by an error **in the contract, an ADR or the research** — a contradiction between sections, a default value read wrongly, an outdated source, a flag that does not exist at the pinned commit, a digest that changed — resolve it yourself:

1. write a new ADR (`docs/adr/NNNN-<slug>.md`) stating the defect, the evidence (command + output), the correction and the alternatives considered;
2. bump the contract version (PATCH for corrections, MINOR for new requirements) and update the affected sections and the ADR index;
3. commit the fix separately (`docs(contract): <summary> (ADR-NNNN)`);
4. continue the sprint.

Never resolve by yourself — stop with **BLOCKED** and evidence instead:

- prerequisites missing on the owner's machine (packages, permissions, kernel modules, disk, memory);
- irreversible actions (deleting data outside the repository and its own Docker resources, rewriting published history, force-pushing);
- publishing anything outside the repository (push, issues, PRs, comments, releases, uploads, posting findings);
- any change that relaxes a gate, a criterion, a threshold, a test or a negative control — including lowering repetitions, widening tolerances, removing a fault from a campaign, reclassifying a failure as expected, or skipping a criterion.

A BLOCKED stop prints the closure report (CONTRACT §19) with `status: BLOCKED`, the exact blocking condition, the command and output that show it, what was completed, and the single action needed from the owner.

## 4. Repository conventions

- Language: English for code, comments, docs, commit messages.
- Go: toolchain `go1.26.9` via `GOTOOLCHAIN` (ADR-0014); `gofmt`, `go vet`, `staticcheck` clean; errors wrapped with context; every network or Docker call has a timeout derived from CONTRACT §5.7; no global mutable state in checkers; checkers are pure functions of their inputs.
- Binary independence (CONTRACT §5.3) is enforced by a test; do not add imports that break it.
- Commits: Conventional Commits (`feat(nemesis): …`, `fix(checker): …`, `docs(contract): …`, `test(gate): …`), DCO sign-off (`git commit -s`). Small, reviewable commits; each sprint ends with a commit that adds `evidence/K<n>/`.
- Never commit secrets, host-specific absolute paths in evidence, or files over 50 MB. Evidence size budget: P25.
- Layout inside packages is the implementer's choice; interfaces in CONTRACT §5.6 and §6 are fixed.

## 5. Environment hygiene

- Only touch Docker resources labelled `com.docker.compose.project=commitproof` or images named `commitproof/*`, plus the pinned base images. Never prune or remove other projects' containers, images or volumes. `docker builder prune -af` is allowed only where K1.2 requires it.
- Never use `sudo`. Never run privileged containers. Capabilities allowed: `NET_ADMIN` (helpers), `SYS_ADMIN` + `/dev/fuse` (pg nodes, slowfs).
- Long-running work (campaigns) runs through the resumable `commitproof campaign` command in the background with output to a log file; poll its status; a sealed run is never recomputed. Keep the memory budget P10 in mind: one cluster at a time.
- No outbound network access beyond what `docs/PREREQUISITES.md` §5 lists.

## 6. Publication

Nothing leaves this repository without the owner: no `git push`, no remote creation, no GitHub issues/PRs/comments/releases, no uploads, no messages to the Multigres project. Findings packages are written under `findings/` for the owner to decide.

## 7. Closure

Every sprint ends with the closure report (CONTRACT §19) printed in full and saved to `evidence/K<n>/closure.yaml`.
