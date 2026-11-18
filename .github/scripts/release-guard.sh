#!/bin/sh
# release-guard.sh: decide whether a release workflow run may build anything.
#
#   release-guard.sh REF_TYPE REF_NAME MODE RELEASED_REF
#
#   REF_TYPE      github.ref_type: branch or tag
#   REF_NAME      github.ref_name, for example v0.1.1
#   MODE          the workflow's mode input: dry-run or release
#   RELEASED_REF  the ref of the public released-code branch, for example
#                 origin/main
#
# A run is accepted only when it was dispatched from a version tag of the form
# vMAJOR.MINOR.PATCH, the mode is known, and the tag's commit is reachable
# from RELEASED_REF: main is the public released-code branch, so a tag on any
# other commit is refused (#138). Anything else is refused before anything is
# built (docs/adr/0003-release-distribution.md). The reachability check runs
# in the current Git repository, which must hold the tag and RELEASED_REF;
# when it cannot be decided, the run is refused.
#
# On acceptance it prints "tag=<tag>" and "version=<version without v>"
# lines, for $GITHUB_OUTPUT.

set -eu

if [ "$#" -ne 4 ]; then
	echo "usage: release-guard.sh REF_TYPE REF_NAME MODE RELEASED_REF" >&2
	exit 2
fi
ref_type=$1
ref_name=$2
mode=$3
released_ref=$4

refuse() {
	echo "release-guard: $1" >&2
	exit 1
}

[ "$ref_type" = tag ] ||
	refuse "the release workflow runs only from a version tag; it was dispatched from the $ref_type '$ref_name'"

printf '%s\n' "$ref_name" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' ||
	refuse "'$ref_name' is not a version tag of the form vMAJOR.MINOR.PATCH"

case $mode in
dry-run | release) ;;
*) refuse "mode must be dry-run or release; got '$mode'" ;;
esac

[ -n "$released_ref" ] ||
	refuse "no released-code ref was given, so the tag's commit cannot be checked"

commit=$(git rev-parse --verify --quiet "refs/tags/$ref_name^{commit}") ||
	refuse "the tag '$ref_name' is not present in this repository, so its commit cannot be checked"

git rev-parse --verify --quiet "$released_ref^{commit}" >/dev/null ||
	refuse "the released-code ref '$released_ref' is not present in this repository, so the tag's commit cannot be checked"

if git merge-base --is-ancestor "$commit" "$released_ref"; then
	:
else
	status=$?
	[ "$status" -eq 1 ] ||
		refuse "cannot decide whether the tag '$ref_name' (commit $commit) is on '$released_ref' (git merge-base exited $status)"
	refuse "the tag '$ref_name' points to commit $commit, which is not on '$released_ref'; a release is built only from a commit on the released-code branch"
fi

echo "tag=$ref_name"
echo "version=${ref_name#v}"
