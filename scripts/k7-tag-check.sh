#!/usr/bin/env bash
# k7-tag-check.sh <ci-commit> — the facts K7.7 needs about the local tag
# v1.0.0 (CONTRACT §15 K7.7): its object type (an annotated tag is "tag"),
# the commit it points to, the files outside the release's text and
# evidence that differ between that commit and the commit the CI record
# built (the code must be the CI's), the remotes and remote refs (nothing
# is pushed, AGENTS.md §6). One JSON line; cpgate K7 judges it.
set -u -o pipefail
ci=${1:?the commit the recorded CI built}
type=$(git cat-file -t v1.0.0 2>/dev/null || echo missing)
tc=$(git rev-parse --verify --quiet 'v1.0.0^{commit}' || echo "")
diff='null'
if [ -n "$tc" ]; then
	diff=$(git diff --name-only "$ci" "$tc" -- . ':(exclude)evidence' ':(exclude)findings' ':(exclude)docs' ':(exclude)prompts' \
		':(exclude)README.md' ':(exclude)CHANGELOG.md' ':(exclude)claims.yaml' ':(exclude)images.lock.json' | jq -R . | jq -sc .)
fi
remotes=$(git remote | jq -R . | jq -sc .)
rrefs=$(git for-each-ref --format='%(refname)' refs/remotes | jq -R . | jq -sc .)
printf '{"ci_commit":"%s","code_diff":%s,"command":"k7-tag-check","exit_code":0,"head":"%s","object_type":"%s","remote_refs":%s,"remotes":%s,"tag":"v1.0.0","tag_commit":"%s"}\n' \
	"$ci" "$diff" "$(git rev-parse HEAD)" "$type" "$rrefs" "$remotes" "$tc"
