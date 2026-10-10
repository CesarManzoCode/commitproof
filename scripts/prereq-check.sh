#!/usr/bin/env bash
# prereq-check.sh — checks every row of docs/PREREQUISITES.md (§1–§4, §6).
#
# Prints one line per item: RESULT  ID  ITEM  observed=<value>
# Writes the same results as JSON to the path given as $1.
# Exit codes follow CONTRACT §5.6: 0 = all PASS, 3 = prerequisite missing.
#
# Fail-closed (AGENTS.md §2): an item that cannot be verified is FAIL.
# Nothing is installed, loaded or worked around here; that is the owner's
# action (PREREQUISITES §7).
set -u -o pipefail

OUT="${1:?usage: prereq-check.sh <output.json>}"

# Every external probe is bounded by P01 (client operation timeout, 10 s).
PROBE_TIMEOUT=10s

GIB=$((1024 * 1024 * 1024))
REQ_RAM_GIB=12
REQ_CPU=4
REQ_DOCKER_DISK_GIB=40
REQ_REPO_DISK_GIB=5
REQ_DOCKER_VERSION=29.9.0
REQ_BUILDX_VERSION=0.38.0
REQ_COMPOSE_VERSION=5.6.0
REQ_GO_VERSION=1.21
SUBNET_PREFIX='10.77.'
SUBNET_CIDR='10.77.0.0/24'

IDS=() ITEMS=() REQS=() CHECKS=() OBSERVED=() RESULTS=()

record() { # id item requirement check observed result
	IDS+=("$1") ITEMS+=("$2") REQS+=("$3") CHECKS+=("$4") OBSERVED+=("$5") RESULTS+=("$6")
	printf '%-4s  %-4s  %-28s  observed=%s\n' "$6" "$1" "$2" "$5"
}

run() { # bounded probe; stdout+stderr merged, trailing newlines stripped
	timeout "$PROBE_TIMEOUT" "$@" 2>&1
}

has() { command -v "$1" >/dev/null 2>&1; }

ver_ge() { # $1 >= $2 (version sort)
	[ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]
}

oneline() { tr '\n' ' ' | sed -e 's/[[:space:]]\+/ /g' -e 's/^ //' -e 's/ $//'; }

json_str() {
	local s
	s=$(printf '%s' "$1" | tr -d '\000-\010\013\014\016-\037')
	s=${s//\\/\\\\}
	s=${s//\"/\\\"}
	s=${s//$'\n'/\\n}
	s=${s//$'\t'/\\t}
	s=${s//$'\r'/\\r}
	printf '"%s"' "$s"
}

pass_if() { if "$@"; then echo PASS; else echo FAIL; fi; }

DOCKER_OK=false
if has docker && run docker info --format '{{.ServerVersion}}' >/dev/null; then
	DOCKER_OK=true
fi
docker_unreachable() {
	if has docker; then
		printf 'docker daemon not reachable: %s' "$(run docker info --format '{{.ServerVersion}}' | oneline | cut -c1-200)"
	else
		printf 'docker: command not found'
	fi
}

# ---------------------------------------------------------------- §1 Resources

avail_kib=$(awk '/^MemAvailable:/{print $2}' /proc/meminfo)
total_kib=$(awk '/^MemTotal:/{print $2}' /proc/meminfo)
avail_b=$((avail_kib * 1024))
obs=$(awk -v a="$avail_kib" -v t="$total_kib" 'BEGIN{printf "MemAvailable=%.2f GiB (MemTotal=%.2f GiB)", a/1048576, t/1048576}')
record R1 "RAM available" ">= ${REQ_RAM_GIB} GiB available" "free -g -> available (MemAvailable in /proc/meminfo)" \
	"$obs" "$(pass_if test "$avail_b" -ge $((REQ_RAM_GIB * GIB)))"

ncpu=$(nproc)
record R2 "CPU cores" ">= ${REQ_CPU} cores" "nproc" "$ncpu" "$(pass_if test "$ncpu" -ge "$REQ_CPU")"

if $DOCKER_OK; then
	docker_root=$(run docker info --format '{{.DockerRootDir}}')
else
	docker_root=/var/lib/docker
fi
if [ -d "$docker_root" ]; then
	free_b=$(df -B1 --output=avail "$docker_root" | tail -n1 | tr -d ' ')
	mnt=$(df --output=target "$docker_root" | tail -n1)
	obs=$(awk -v f="$free_b" -v d="$docker_root" -v m="$mnt" 'BEGIN{printf "%s on %s: %.1f GiB free", d, m, f/1073741824}')
	res=$(pass_if test "$free_b" -ge $((REQ_DOCKER_DISK_GIB * GIB)))
else
	obs="$docker_root: No such file or directory"
	res=FAIL
fi
record R3 "Disk (Docker root)" ">= ${REQ_DOCKER_DISK_GIB} GiB free on the filesystem holding the Docker root" \
	"df /var/lib/docker (docker info DockerRootDir when reachable)" "$obs" "$res"

free_b=$(df -B1 --output=avail . | tail -n1 | tr -d ' ')
obs=$(awk -v f="$free_b" 'BEGIN{printf "repository filesystem: %.1f GiB free", f/1073741824}')
record R4 "Disk (repository)" ">= ${REQ_REPO_DISK_GIB} GiB free" "df ." "$obs" \
	"$(pass_if test "$free_b" -ge $((REQ_REPO_DISK_GIB * GIB)))"

# The PREREQUISITES row has no check command ("owner's choice of power
# settings"). Verified fail-closed: sleep.target (required by every
# suspend/hibernate target) must be masked so the host cannot suspend.
sleep_state=$(run systemctl is-enabled sleep.target | oneline)
record R5 "No suspend during campaigns" "machine must not suspend during K7" \
	"systemctl is-enabled sleep.target == masked" "sleep.target=${sleep_state:-unknown}" \
	"$(pass_if test "$sleep_state" = masked)"

# ----------------------------------------------------------------- §2 Packages

pkg() { # package name, for observed values
	if has pacman; then
		pacman -Q "$1" 2>&1 | oneline
	else
		echo "pacman unavailable"
	fi
}

if has docker && $DOCKER_OK; then
	v=$(run docker version --format '{{.Server.Version}}' | oneline)
	res=FAIL
	[[ $v =~ ^([0-9]+\.[0-9]+\.[0-9]+) ]] && ver_ge "${BASH_REMATCH[1]}" "$REQ_DOCKER_VERSION" && res=PASS
	obs="server $v; $(pkg docker)"
else
	obs="$(docker_unreachable); $(pkg docker)"
	res=FAIL
fi
record P1 "docker" ">= ${REQ_DOCKER_VERSION}" "docker version --format '{{.Server.Version}}'" "$obs" "$res"

if has docker; then
	v=$(run docker buildx version | oneline)
	res=FAIL
	# Upstream prints "github.com/docker/buildx v0.38.0 <commit>"; the Arch
	# package prints "github.com/docker/buildx 0.38.0 <commit>" (no "v").
	[[ $v =~ v?([0-9]+\.[0-9]+\.[0-9]+) ]] && ver_ge "${BASH_REMATCH[1]}" "$REQ_BUILDX_VERSION" && res=PASS
	obs="$v; $(pkg docker-buildx)"
else
	obs="docker: command not found; $(pkg docker-buildx)"
	res=FAIL
fi
record P2 "docker-buildx" ">= ${REQ_BUILDX_VERSION}" "docker buildx version" "$obs" "$res"

if has docker; then
	v=$(run docker compose version | oneline)
	res=FAIL
	[[ $v =~ v?([0-9]+\.[0-9]+\.[0-9]+) ]] && ver_ge "${BASH_REMATCH[1]}" "$REQ_COMPOSE_VERSION" && res=PASS
	obs="$v; $(pkg docker-compose)"
else
	obs="docker: command not found; $(pkg docker-compose)"
	res=FAIL
fi
record P3 "docker-compose" ">= ${REQ_COMPOSE_VERSION}" "docker compose version" "$obs" "$res"

if has go; then
	v=$(GOTOOLCHAIN=local run go version | oneline)
	res=FAIL
	[[ $v =~ go([0-9]+\.[0-9]+(\.[0-9]+)?) ]] && ver_ge "${BASH_REMATCH[1]}" "$REQ_GO_VERSION" && res=PASS
	obs="$v; $(pkg go)"
else
	obs="go: command not found"
	res=FAIL
fi
record P4 "go" ">= ${REQ_GO_VERSION} (bootstraps go1.26.9)" "GOTOOLCHAIN=local go version" "$obs" "$res"

tool() { # id name check-argv...
	local id=$1 name=$2
	shift 2
	if has "$1"; then
		local out rc v
		out=$(run "$@")
		rc=$?
		v=$(printf '%s\n' "$out" | head -n1 | oneline)
		if [ "$rc" -eq 0 ]; then
			record "$id" "$name" "present" "$*" "$v" PASS
		else
			record "$id" "$name" "present" "$*" "exit ${rc}: $v" FAIL
		fi
	else
		record "$id" "$name" "present" "$*" "$1: command not found; $(pkg "$name")" FAIL
	fi
}
tool P5 git git --version
tool P6 make make --version
tool P7 zstd zstd --version
tool P8 jq jq --version

# ------------------------------------------------- §3 Docker daemon and access

if has systemctl; then
	st=$(run systemctl is-active docker | oneline)
	en=$(run systemctl is-enabled docker 2>&1 | oneline)
else
	st="systemctl unavailable" en=""
fi
record D1 "Docker daemon running" "active (enabled)" "systemctl is-active docker" \
	"is-active=${st}; is-enabled=${en}" "$(pass_if test "$st" = active)"

groups=$(id -nG)
in_group=false
grep -qw docker <<<"$groups" && in_group=true
if $in_group && $DOCKER_OK; then
	res=PASS
else
	res=FAIL
fi
obs="groups: ${groups}; docker info without sudo: $($DOCKER_OK && echo ok || docker_unreachable)"
record D2 "User in docker group" "member of docker; docker info without sudo" \
	"id -nG | grep -qw docker && docker info" "$obs" "$res"

if $DOCKER_OK; then
	cg=$(run docker info --format '{{.CgroupVersion}}' | oneline)
	record D3 "cgroup v2" "2" "docker info --format '{{.CgroupVersion}}'" "$cg" "$(pass_if test "$cg" = 2)"
	drv=$(run docker info --format '{{.Driver}}' | oneline)
	record D4 "Storage driver" "overlay2" "docker info --format '{{.Driver}}'" "$drv" "$(pass_if test "$drv" = overlay2)"
else
	record D3 "cgroup v2" "2" "docker info --format '{{.CgroupVersion}}'" "$(docker_unreachable)" FAIL
	record D4 "Storage driver" "overlay2" "docker info --format '{{.Driver}}'" "$(docker_unreachable)" FAIL
fi

routes=$(ip route | grep -c "^${SUBNET_PREFIX//./\\.}" || true)
if $DOCKER_OK; then
	using=""
	while read -r net; do
		[ -n "$net" ] || continue
		subs=$(run docker network inspect --format '{{range .IPAM.Config}}{{.Subnet}} {{end}}' "$net" | oneline)
		if grep -q "${SUBNET_PREFIX//./\\.}" <<<"$subs"; then
			using+="${net}(${subs}) "
		fi
	done < <(run docker network ls --format '{{.Name}}')
	obs="ip routes in ${SUBNET_CIDR}: ${routes}; docker networks using it: ${using:-none}"
	res=$(pass_if test "$routes" -eq 0 -a -z "$using")
else
	obs="ip routes in ${SUBNET_CIDR}: ${routes}; docker networks: unverifiable ($(docker_unreachable))"
	res=FAIL
fi
record D5 "Bridge subnet free" "${SUBNET_CIDR} unused" \
	"ip route | grep -c '^10\\.77\\.' == 0 and no docker network uses it" "$obs" "$res"

# --------------------------------------------------------- §4 Kernel facilities

kernel=$(uname -r)
moddir_note=""
if [ ! -d "/lib/modules/$kernel" ]; then
	installed=$(ls /lib/modules 2>/dev/null | oneline)
	moddir_note="; /lib/modules/${kernel} missing (installed: ${installed:-none}) — modprobe cannot load modules until reboot"
fi
module() { # id name
	local m=$2 how=""
	if lsmod | awk -v m="$m" '$1 == m {found=1} END {exit !found}'; then
		how="loaded (lsmod)"
	elif grep -qs "/${m}\.ko" "/lib/modules/${kernel}/modules.builtin"; then
		how="built-in (modules.builtin)"
	fi
	if [ -n "$how" ]; then
		record "$1" "kernel module $m" "available in running kernel" "lsmod | grep '^$m '" "$how; kernel ${kernel}" PASS
	else
		record "$1" "kernel module $m" "available in running kernel" "lsmod | grep '^$m '" "not loaded; kernel ${kernel}${moddir_note}" FAIL
	fi
}
module K1 nf_tables
module K2 nft_compat
module K3 sch_netem

if [ -c /dev/fuse ]; then
	record K4 "/dev/fuse" "character device present" "test -c /dev/fuse" "$(ls -l /dev/fuse | oneline)" PASS
else
	record K4 "/dev/fuse" "character device present" "test -c /dev/fuse" "absent" FAIL
fi

# ---------------------------------------------------- §6 Repository and tools

gs=$(run git -C . status --porcelain=v1 --branch)
rc=$?
gs=$(printf '%s\n' "$gs" | head -n1 | oneline)
if [ "$rc" -eq 0 ]; then
	record G1 "Repository" "git repository" "git -C . status" "$gs" PASS
else
	record G1 "Repository" "git repository" "git -C . status" "exit ${rc}: $gs" FAIL
fi

gname=$(git config user.name || true)
gemail=$(git config user.email || true)
record G2 "Git identity" "user.name and user.email set" "git config user.email" \
	"user.name=${gname:-<unset>}; user.email=${gemail:-<unset>}" "$(pass_if test -n "$gemail" -a -n "$gname")"

if has claude; then
	cv=$(run claude --version)
	cv=$(printf '%s\n' "$cv" | head -n1 | oneline)
	record G3 "Claude Code" "installed" "claude --version" "$cv" "$(pass_if test -n "$cv")"
else
	record G3 "Claude Code" "installed" "claude --version" "claude: command not found" FAIL
fi

remotes=$(git remote | wc -l | tr -d ' ')
record G4 "GitHub remote" "optional (never pushed by K1-K7)" "git remote" "${remotes} remote(s) configured" PASS

# ------------------------------------------------------------------- summary

n=${#IDS[@]} fails=0
for r in "${RESULTS[@]}"; do [ "$r" = FAIL ] && fails=$((fails + 1)); done
overall=PASS
[ "$fails" -gt 0 ] && overall=FAIL

mkdir -p "$(dirname "$OUT")"
{
	printf '{\n'
	printf '  "checked_at": %s,\n' "$(json_str "$(date -u +%Y-%m-%dT%H:%M:%SZ)")"
	printf '  "contract_version": "1.0.0",\n'
	printf '  "host_kernel": %s,\n' "$(json_str "$kernel")"
	printf '  "items": [\n'
	for ((i = 0; i < n; i++)); do
		printf '    {"check": %s, "id": %s, "item": %s, "observed": %s, "requirement": %s, "result": %s}' \
			"$(json_str "${CHECKS[i]}")" "$(json_str "${IDS[i]}")" "$(json_str "${ITEMS[i]}")" \
			"$(json_str "${OBSERVED[i]}")" "$(json_str "${REQS[i]}")" "$(json_str "${RESULTS[i]}")"
		[ $((i + 1)) -lt "$n" ] && printf ','
		printf '\n'
	done
	printf '  ],\n'
	printf '  "result": %s,\n' "$(json_str "$overall")"
	printf '  "source": "docs/PREREQUISITES.md",\n'
	printf '  "summary": {"fail": %d, "pass": %d, "total": %d}\n' "$fails" $((n - fails)) "$n"
	printf '}\n'
} >"$OUT"

printf '{"command":"prereq-check","fail":%d,"output":%s,"pass":%d,"result":%s,"total":%d}\n' \
	"$fails" "$(json_str "$OUT")" $((n - fails)) "$(json_str "$overall")" "$n"

[ "$overall" = PASS ] && exit 0
exit 3
