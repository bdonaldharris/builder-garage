#!/bin/sh
# test-release-scripts.sh: exercise the release workflow's refusals.
#
#   .github/scripts/test-release-scripts.sh
#
# It shows, without running a release, that:
#   - release-guard.sh refuses every ref but a vMAJOR.MINOR.PATCH tag, and any
#     unknown mode;
#   - platform-sign.sh leaves every binary unchanged in every known mode,
#     since nothing is platform-signed until #122, and refuses an unset or
#     unknown mode and a missing binary.
# CI runs it on every pull request. It runs on macOS and Linux.

set -eu

scripts=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)

work=$(mktemp -d "${TMPDIR:-/tmp}/garage-release-scripts.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() {
	printf 'FAIL %s\n' "$1"
	failures=$((failures + 1))
}
accepts() { # accepts NAME COMMAND...
	name=$1
	shift
	if "$@" >"$work/out" 2>&1; then pass "$name"; else
		cat "$work/out"
		fail "$name"
	fi
}
refuses() { # refuses NAME COMMAND...
	name=$1
	shift
	if "$@" >"$work/out" 2>&1; then
		cat "$work/out"
		fail "$name"
	else pass "$name"; fi
}

guard=$scripts/release-guard.sh

# release-guard.sh
accepts "guard accepts tag v0.1.1 in dry-run mode" sh "$guard" tag v0.1.1 dry-run
accepts "guard accepts tag v10.20.30 in release mode" sh "$guard" tag v10.20.30 release
out=$(sh "$guard" tag v0.1.1 release)
if [ "$out" = "tag=v0.1.1
version=0.1.1" ]; then pass "guard outputs the tag and version"; else fail "guard outputs the tag and version (got: $out)"; fi
refuses "guard refuses the main branch" sh "$guard" branch main release
refuses "guard refuses a branch named like a tag" sh "$guard" branch v0.1.1 dry-run
refuses "guard refuses a tag without v" sh "$guard" tag 0.1.1 release
refuses "guard refuses a pre-release tag" sh "$guard" tag v0.1.1-rc.1 release
refuses "guard refuses a tag with leading zeros" sh "$guard" tag v0.01.1 release
refuses "guard refuses a partial version" sh "$guard" tag v0.1 release
refuses "guard refuses an unknown mode" sh "$guard" tag v0.1.1 publish
refuses "guard refuses an empty mode" sh "$guard" tag v0.1.1 ""

# platform-sign.sh
sign=$scripts/platform-sign.sh
bin=$work/garage
printf 'binary' >"$bin"
for mode in snapshot dry-run release; do
	for os in darwin linux windows; do
		accepts "hook passes a $os binary in $mode mode" sh "$sign" "$mode" "$os" "$bin"
	done
done
if [ "$(cat "$bin")" = binary ]; then pass "hook did not modify the binary"; else fail "hook did not modify the binary"; fi
refuses "hook refuses an unset mode" sh "$sign" unset linux "$bin"
refuses "hook refuses an unknown mode" sh "$sign" publish linux "$bin"
refuses "hook refuses a missing binary" sh "$sign" dry-run linux "$work/absent"

if [ "$failures" -ne 0 ]; then
	echo "$failures check(s) failed"
	exit 1
fi
echo "all release script checks passed"
