#!/bin/sh
# release-guard.sh: decide whether a release workflow run may build anything.
#
#   release-guard.sh REF_TYPE REF_NAME MODE
#
#   REF_TYPE  github.ref_type: branch or tag
#   REF_NAME  github.ref_name, for example v0.1.1
#   MODE      the workflow's mode input: dry-run or release
#
# A run is accepted only when it was dispatched from a version tag of the form
# vMAJOR.MINOR.PATCH and the mode is known. Anything else is refused before
# anything is built (docs/adr/0003-release-distribution.md). On acceptance it
# prints "tag=<tag>" and "version=<version without v>" lines, for
# $GITHUB_OUTPUT.

set -eu

if [ "$#" -ne 3 ]; then
	echo "usage: release-guard.sh REF_TYPE REF_NAME MODE" >&2
	exit 2
fi
ref_type=$1
ref_name=$2
mode=$3

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

echo "tag=$ref_name"
echo "version=${ref_name#v}"
