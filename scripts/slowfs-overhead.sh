#!/usr/bin/env bash
# slowfs-overhead.sh — K3 NOTES: the cost of the slowfs pass-through at
# delay 0 (ADR-0006: "overhead is uniform and recorded in K3 NOTES"). With a
# baseline cluster up, 20 write+fsync probes of 4 KiB per node through the
# pass-through (/data) and on the plain volume (/data-backing), same
# container, same moment. Prints one JSON line per probe; the cluster is
# torn down at the end.
set -euo pipefail
CP=bin/commitproof
cleanup() { "$CP" cluster down --profile baseline >/dev/null 2>&1 || true; }
trap cleanup EXIT
"$CP" cluster up --profile baseline | tail -n 1
for n in n1 n2 n3; do
	for dir in /data /data-backing; do
		echo "{\"node\":\"$n\",\"probe\":$(docker exec "commitproof-pg-$n-1" slowfs probe --dir "$dir" --size 4096 --count 20)}"
	done
done
