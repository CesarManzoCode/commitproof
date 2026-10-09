# PREREQUISITES (before K1)

Target machine: Arch Linux, 16 GB DDR4, most RAM free. Package versions were verified against the Arch repositories on 2026-10-09 (RESEARCH S87). Each item has a check command; K1's first step runs all of them (`make prereq-check`) and stops with BLOCKED, naming the failed item, if any fails. Nothing here can be installed by Claude Code itself (BLOCKED protocol: prerequisites are the owner's).

## 1. Resources

| Item | Requirement | Why | Check |
|---|---|---|---|
| RAM | ≥ 12 GiB available when campaigns run | cluster peak budget 6 GiB (P10) + image builds + Claude Code | `free -g` → `available` ≥ 12 |
| CPU | ≥ 4 cores | 15+ containers, Go builds | `nproc` ≥ 4 |
| Disk | ≥ 40 GB free on the filesystem holding `/var/lib/docker`, ≥ 5 GB free for the repo | builder cache (~5 GB), base + mutant images (~4 GB), evidence and run directories | `df -h /var/lib/docker .` |
| Sleep | the machine must not suspend during K7 (several hours of campaigns) | campaign runs are resumable, but a suspended Docker daemon invalidates the run in progress | owner's choice of power settings |

## 2. Packages (Arch `extra`)

| Package | Version verified 2026-10-09 | Use | Check |
|---|---|---|---|
| `docker` | 1:29.9.0-1 | Docker Engine | `docker version --format '{{.Server.Version}}'` |
| `docker-buildx` | 0.38.0-1 | image builds, digest inspection | `docker buildx version` |
| `docker-compose` | 5.6.0-1 | Compose plugin (`docker compose`) | `docker compose version` |
| `go` | 2:1.27.1-1 (any ≥ 1.21 works) | bootstraps the pinned toolchain go1.26.9 through `GOTOOLCHAIN` | `go version` |
| `git` | any current | repository, upstream clone at the pinned commit | `git --version` |
| `make` | any current (in `base-devel`) | all entry points are `make` targets | `make --version` |
| `zstd` | any current | evidence compression | `zstd --version` |
| `jq` | any current | readable evidence inspection (not used by gates) | `jq --version` |

Not needed on the host: PostgreSQL, Java, iptables userland, FUSE userland (all run inside containers).

Install (one line, for reference): `sudo pacman -S --needed docker docker-buildx docker-compose go git base-devel zstd jq`.

## 3. Docker daemon and permissions

| Item | Requirement | Check |
|---|---|---|
| Daemon running and enabled | `systemctl enable --now docker.service` done | `systemctl is-active docker` → `active` |
| User in `docker` group | so `make` targets and Claude Code run Docker without `sudo` (the group is root-equivalent; accepted, LIM-11) | `id -nG \| grep -qw docker` and `docker info` without sudo |
| cgroup v2 | Arch default; needed for `docker pause` (freezer) and `docker stats` | `docker info --format '{{.CgroupVersion}}'` → `2` |
| Storage driver | `overlay2` | `docker info --format '{{.Driver}}'` |
| Bridge subnet free | `10.77.0.0/24` (P26) must not overlap a host route or another Docker network | `ip route \| grep -c '^10\.77\.'` → `0` and `docker network ls` shows no network using it |

## 4. Kernel facilities

Containers can use these only if the host kernel provides them; module autoloading from inside a container network namespace is not reliable, so they are loaded on the host and persisted.

| Facility | Used by | Load / persist | Check |
|---|---|---|---|
| `nf_tables`, `nft_compat` | iptables (nft backend) inside nemesis helpers (F1a–F1d) | `sudo modprobe -a nf_tables nft_compat` and add both to `/etc/modules-load.d/commitproof.conf` | `lsmod \| grep -E '^(nf_tables\|nft_compat) '` |
| `sch_netem` | `tc netem` delay (F7) | `sudo modprobe sch_netem`, add to the same file | `lsmod \| grep '^sch_netem '` |
| `fuse` and `/dev/fuse` | slowfs (F4a) | built as a module in Arch's kernel; `sudo modprobe fuse`, add to the same file | `test -c /dev/fuse` |

## 5. Network access during sprints

Builds and tests download from: GitHub (`github.com/multigres/multigres` at the pinned commit, Go modules), `proxy.golang.org` and `sum.golang.org` (go1.26.9 toolchain and modules), Docker Hub (`golang`, `debian`, `postgres`, `alpine`, `clojure`), `quay.io` (etcd), `apt.postgresql.org` and `deb.debian.org` (pgBackRest in upstream Dockerfiles), Alpine package mirrors (iptables, iproute2, fuse3), Clojars and Maven Central (elle-cli dependencies, K2). All free; no accounts required.

## 6. Repository and tools

| Item | Requirement | Check |
|---|---|---|
| Empty repository | a new directory with this zip's contents at its root; `git init` done or left to K1 | `git -C <repo> status` |
| Git identity | `user.name` and `user.email` set; commits are DCO-signed (`-s`) | `git config user.email` non-empty |
| Claude Code | installed and authenticated, started from the repository root | `claude --version` |
| GitHub remote | optional; K1–K7 never push (AGENTS.md). Remote CI runs happen only when the owner pushes | — |

## 7. What K1 does if something is missing

`make prereq-check` prints one line per item above with PASS/FAIL and the observed value, writes `evidence/K1/prereq.json`, and K1 stops with `status: BLOCKED` and the failing item(s). No item is skipped, emulated or worked around.
