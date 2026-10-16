#!/usr/bin/env bash
# k7-tag-check.sh <ci-commit> <first-k7-commit> — the facts K7.7 needs
# about the local tag v1.0.0 (CONTRACT §15 K7.7): its object type (an
# annotated tag is "tag"), the commit it points to, the files outside the
# release's text and evidence that differ between that commit and the
# commit the CI record built (the code must be the CI's), and the
# remote-tracking refs that contain K7's first commit (nothing of K7 is
# pushed, AGENTS.md §6; the owner pushed up to K6 — what a remote holds
# beyond its tracking refs is not queried: no network). One JSON line;
# cpgate K7 judges it.
set -u -o pipefail
ci=${1:?the commit the recorded CI built} first=${2:?the first K7 commit}
type=$(git cat-file -t v1.0.0 2>/dev/null || echo missing)
tc=$(git rev-parse --verify --quiet 'v1.0.0^{commit}' || echo "")
diff='null'
if [ -n "$tc" ]; then
	diff=$(git diff --name-only "$ci" "$tc" -- . ':(exclude)evidence' ':(exclude)findings' ':(exclude)docs' ':(exclude)prompts' \
		':(exclude)README.md' ':(exclude)CHANGELOG.md' ':(exclude)claims.yaml' ':(exclude)images.lock.json' | jq -R . | jq -sc .)
fi
remotes=$(git remote | jq -R . | jq -sc .)
rrefs=$(git for-each-ref --format='%(refname) %(objectname)' refs/remotes | jq -R . | jq -sc .)
onremote=$(git for-each-ref --format='%(refname)' refs/remotes | while read -r r; do
	git merge-base --is-ancestor "$first" "$r" 2>/dev/null && echo "$r"
done | jq -R . | jq -sc .)
printf '{"ci_commit":"%s","code_diff":%s,"command":"k7-tag-check","exit_code":0,"first_k7_commit":"%s","head":"%s","k7_on_remote":%s,"object_type":"%s","remote_refs":%s,"remotes":%s,"tag":"v1.0.0","tag_commit":"%s"}\n' \
	"$ci" "$diff" "$(git rev-parse "$first")" "$(git rev-parse HEAD)" "$onremote" "$type" "$rrefs" "$remotes" "$tc"
