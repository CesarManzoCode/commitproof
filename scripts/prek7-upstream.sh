#!/usr/bin/env bash
# prek7-upstream.sh — read-only lookups on github.com/multigres/multigres
# (owner decision D-C of 2026-10-15: issues, pull requests, commits and
# files since the pinned commit; nothing written or posted). Every lookup
# appends one line to <out>/lookups.jsonl: number, UTC date, method (GET or
# git clone/fetch), URL, what was read, the saved result file and its
# sha256. Results are saved under <out>/. The pinned commit is never
# changed: the clone is a separate partial clone under runs/.prek7.
#
# usage: prek7-upstream.sh search <out>          clone/fetch, compare files, search
#        prek7-upstream.sh read <out> <n>...     read issues/PRs by number
set -u -o pipefail
mode=${1:?mode} out=${2:?out}
PIN=e3cbdbbb0b94453ca2ec43c9c40c05c98ed7bb73
REPO=multigres/multigres
CLONE=runs/.prek7/upstream
FILES=(go/services/multigateway/poolergateway/notification_adapter.go go/services/multipooler/internal/pools/regular/regular_conn.go docs/query_serving/listen_notify_design.md)
mkdir -p "$out"
LOG=$out/lookups.jsonl
n=$( [ -f "$LOG" ] && wc -l <"$LOG" || echo 0)
log() { # method url read file
	n=$((n + 1))
	jq -cn --argjson n "$n" --arg date "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg m "$1" --arg u "$2" --arg r "$3" --arg f "$4" \
		--arg s "$( [ -f "$out/$4" ] && sha256sum "$out/$4" | cut -d' ' -f1)" \
		'{n: $n, date: $date, method: $m, url: $u, read: $r, file: $f, sha256: $s}' >>"$LOG"
}
get() { # name path read  (GitHub REST, GET only)
	timeout 60 gh api --method GET -H "Accept: application/vnd.github+json" "$2" >"$out/$1.json" || return 1
	log GET "https://api.github.com/$2" "$3" "$1.json"
}
case "$mode" in
search)
	if [ -d "$CLONE" ]; then
		timeout 600 git -C "$CLONE" fetch -q origin || exit 1
		log "git fetch" "https://github.com/$REPO.git" "refs and commits (blobs on demand)" ""
	else
		timeout 600 git clone -q --filter=blob:none --no-checkout "https://github.com/$REPO.git" "$CLONE" || exit 1
		log "git clone --filter=blob:none" "https://github.com/$REPO.git" "refs and commits (blobs on demand)" ""
	fi
	head=$(git -C "$CLONE" rev-parse origin/main)
	git -C "$CLONE" cat-file -e "$PIN^{commit}" || { echo "pinned commit not in upstream"; exit 1; }
	{
		echo "origin/main $head ($(git -C "$CLONE" show -s --format=%cI "$head"))"
		echo "pinned $PIN ($(git -C "$CLONE" show -s --format=%cI "$PIN"))"
		echo "commits $PIN..origin/main: $(git -C "$CLONE" rev-list --count "$PIN..$head")"
		echo "pinned is an ancestor of main: $(git -C "$CLONE" merge-base --is-ancestor "$PIN" "$head" && echo yes || echo no)"
	} >"$out/heads.txt"
	log "git (local, after fetch)" "https://github.com/$REPO/compare/$PIN...main" "main's head, commit count since the pinned commit" heads.txt
	git -C "$CLONE" log --format='%H %cs %s' "$PIN..$head" -- "${FILES[@]}" >"$out/commits-touching-files.txt"
	log "git (local, after fetch)" "https://github.com/$REPO/commits/main" "commits since the pinned commit touching ${FILES[*]}" commits-touching-files.txt
	for f in "${FILES[@]}"; do
		b=$(basename "$f" | tr . _)
		git -C "$CLONE" diff "$PIN" "$head" -- "$f" >"$out/diff-$b.txt"
		log "git (local, blobs fetched on demand)" "https://github.com/$REPO/blob/main/$f" "diff of $f, pinned → main" "diff-$b.txt"
	done
	git -C "$CLONE" log --format='%H %cs %s' "$PIN..$head" | grep -iE 'listen|notif|pubsub|cancel|synchronous|sync.?rep|rewind|fair|pool|overload|57P03|stale|visib|durab|lost|loss' >"$out/commits-keywords.txt" || true
	log "git (local, after fetch)" "https://github.com/$REPO/commits/main" "subjects of commits since the pinned commit matching listen|notif|pubsub|cancel|synchronous|sync rep|rewind|fair|pool|overload|57P03|stale|visib|durab|lost|loss" commits-keywords.txt
	i=0
	for q in "LISTEN NOTIFY" "notification failover" "notification stream" "synchronous replication cancel" "canceling wait for synchronous replication" "cancel commit" "pg_rewind" "rewind lost" "read after failover" "fair pool" "per-user pool" "overload" "label:alpha-known-gap"; do
		i=$((i + 1))
		enc=$(jq -rn --arg q "repo:$REPO $q" '$q|@uri')
		get "search-$i" "search/issues?q=$enc&per_page=50&sort=created&order=desc" "issues and pull requests matching: $q" || exit 1
		jq -c --arg q "$q" '{query: $q, total: .total_count, items: [.items[] | {number, title, state, pr: (.pull_request != null), created: .created_at[0:10], closed: (.closed_at // "" | .[0:10]), labels: [.labels[].name]}]}' "$out/search-$i.json" >>"$out/search-summary.jsonl"
		sleep 3
	done
	;;
read)
	shift 2
	for num in "$@"; do
		get "issue-$num" "repos/$REPO/issues/$num" "issue/PR #$num: title, body, state, labels" || exit 1
		get "issue-$num-comments" "repos/$REPO/issues/$num/comments?per_page=100" "issue/PR #$num: comments" || exit 1
		if jq -e '.pull_request != null' "$out/issue-$num.json" >/dev/null; then
			get "pr-$num-files" "repos/$REPO/pulls/$num/files?per_page=100" "PR #$num: changed files" || exit 1
		fi
		sleep 2
	done
	;;
*) echo "usage: $0 search|read <out> [n...]" >&2; exit 3 ;;
esac
echo "{\"command\":\"prek7-upstream\",\"mode\":\"$mode\",\"lookups\":$n,\"exit_code\":0}"
