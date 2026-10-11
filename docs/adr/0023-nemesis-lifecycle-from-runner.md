# ADR-0023 — Nemesis lifecycle commands from the runner: n4 created at run start, started with `docker start`; nem-runner started by the runner

Status: Accepted (K3, contract 1.0.4). Corrects CONTRACT §8 F5a (injection command) and §5.1 (how the runner's helper is created).

## Context (defect)

ADR-0018 makes the runner container the nemesis controller: it reaches Docker through the mounted socket and every nemesis command is recorded in the run's `commands.jsonl`. CONTRACT §8 gives F5a's injection as `docker compose up -d pg-n4 pooler-n4 nem-pg-n4`. Run inside the runner, `docker compose` needs the Compose file and resolves its relative bind mounts (`./pg_hba_template.conf`, `./postgres-password`, `deploy/compose.yaml` lines with `./`) against its own project directory, and the Docker daemon then bind-mounts those paths **from the host**. The only way to make them resolve is to give compose the repository's absolute host path, which would then appear in the recorded argv of a sealed run — AGENTS.md §4 forbids host-specific absolute paths in evidence. The contract therefore asks for a command the runner cannot issue faithfully.

§5.1 lists a nemesis helper for the runner's namespace. The runner is started per run with `docker compose run`; Compose cannot create a service in the namespace of a one-off container that does not exist yet, and the host is blocked in `docker compose run` while the runner works.

## Decision

- When a plan contains F5a, `commitproof run` (host side, right after `cluster up`) executes `docker compose --profile n4 create --no-recreate pg-n4 pooler-n4 nem-pg-n4`: the three containers are created from the Compose definitions (same images, flags, volumes, network) and not started. F5a's injection is `docker start` of `pg-n4`, `pooler-n4` and `nem-pg-n4`, in that order, from the runner. The state reached is the one `docker compose up -d` reaches for created services; the confirmation predicate (§8, 120 s) is unchanged.
- The runner container of a run is named `commitproof-runner`; at the start of a run with faults, the runner starts `commitproof-nem-runner` with `docker run --network container:commitproof-runner --cap-drop ALL --cap-add NET_ADMIN <locked nemesis image ID> idle`, labelled with the Compose project, and removes it at the end (`cluster down` also removes it if a run was interrupted).
- The runner image carries the docker CLI (alpine's `docker-cli`, version locked in `images.lock.json`) and joins the Docker socket's group (`group_add`, from the socket's gid).
- CONTRACT §8 (F5a) and §5.1 (helpers) are corrected accordingly (1.0.3 → 1.0.4). The nemesis helper image also carries `cpnet`, CommitProof's probe tool (connect/HTTP probes with timings), in addition to iptables and iproute2.

## Alternatives rejected

- **Mount the repository at its host path inside the runner.** Puts the host path into recorded argv and sealed runs (AGENTS.md §4).
- **Move the bind-mounted files into images or volumes so Compose works from the runner.** Changes the K1 topology to serve one command; a second Compose client (the runner's) with a different version could also compute different service hashes.
- **Have the host perform F5a on the runner's request.** A second nemesis controller and an IPC channel; the history's single-writer rule (ADR-0018) would need a protocol to order host actions.
- **No helper in the runner's namespace.** §8 F1a installs rules in every peer's namespace, and the runner is a cpnet address.

## Consequences

- F5a's recorded commands are `docker start …` (three records) instead of one `docker compose up -d`; the evidence names them.
- `cluster up` alone does not create n4; only runs whose plan contains F5a do.

## Sources

CONTRACT §5.1, §5.4, §8; ADR-0005, ADR-0018; AGENTS.md §4.
