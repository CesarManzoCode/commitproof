#!/usr/bin/env bash
# check-flags.sh — every flag deploy/compose.yaml passes to a Multigres
# binary must exist in that binary's --help at the pinned commit (CONTRACT
# §5.1; K1 prompt: "check with each binary's --help"). Prints one line per
# flag and exits 1 if any is missing.
set -u -o pipefail
MG=commitproof/multigres:real PG=commitproof/pgctld:real
help() { # image entrypoint args...
	local img=$1 ep=$2
	shift 2
	timeout 60s docker run --rm --network none --label com.docker.compose.project=commitproof --entrypoint "$ep" "$img" "$@" --help 2>&1
}
declare -A H
H[multipooler]=$(help $MG /multigres/bin/multipooler)
H[multiorch]=$(help $MG /multigres/bin/multiorch)
H[multigateway]=$(help $MG /multigres/bin/multigateway)
H[createclustermetadata]=$(help $MG /multigres/bin/multigres createclustermetadata)
H[pgctld]=$(help $PG /usr/local/bin/pgctld server)
cfg=$(docker compose -p commitproof -f deploy/compose.yaml --project-directory deploy --profile n4 config --format json)
missing=0
for svc in $(jq -r '.services | keys[]' <<<"$cfg"); do
	cmd=$(jq -r --arg s "$svc" '[.services[$s].entrypoint // [] | .[]] + [.services[$s].command // [] | .[]] | join(" ")' <<<"$cfg")
	case "$cmd" in
	*/multipooler*) bin=multipooler ;;
	*/multiorch*) bin=multiorch ;;
	*/multigateway*) bin=multigateway ;;
	*createclustermetadata*) bin=createclustermetadata ;;
	*/pgctld*) bin=pgctld ;;
	*) continue ;;
	esac
	for f in $(grep -o -- '--[a-z0-9-]*' <<<"$cmd" | sort -u); do
		if grep -q -- "  $f\b" <<<"${H[$bin]}" || grep -q -- " $f[ =]" <<<"${H[$bin]}"; then
			echo "OK       $svc $bin $f"
		else
			echo "MISSING  $svc $bin $f"
			missing=1
		fi
	done
done
exit $missing
