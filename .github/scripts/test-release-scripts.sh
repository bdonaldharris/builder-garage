#!/bin/sh
# test-release-scripts.sh: exercise the release workflow's refusals.
#
#   .github/scripts/test-release-scripts.sh
#
# It shows, without running a release, that:
#   - release-guard.sh refuses every ref but a vMAJOR.MINOR.PATCH tag, and any
#     unknown mode;
#   - release-inventory.sh selects inventory v1 up to v0.1.x and v2 from
#     v0.2.0, names every file exactly, and refuses a malformed version;
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

# release-inventory.sh
inv=$scripts/release-inventory.sh
count() { sh "$inv" "$@" | grep -c .; }
check_eq() { # check_eq NAME GOT WANT
	if [ "$2" = "$3" ]; then pass "$1"; else fail "$1 (got: $2, want: $3)"; fi
}
check_eq "inventory v1 for 0.1.1" "$(sh "$inv" 0.1.1 id)" v1
check_eq "inventory v1 for 0.1.99" "$(sh "$inv" 0.1.99 id)" v1
check_eq "inventory v2 for 0.2.0" "$(sh "$inv" 0.2.0 id)" v2
check_eq "inventory v2 for 1.0.0" "$(sh "$inv" 1.0.0 id)" v2
check_eq "inventory v1 has 8 assets" "$(count 0.1.1)" 8
check_eq "inventory v1 has no packages" "$(sh "$inv" 0.1.1 packages | grep -c . || true)" 0
check_eq "inventory v2 has 12 assets" "$(count 0.2.0)" 12
check_eq "inventory v2 manifest lists 10 files" "$(count 0.2.0 manifest)" 10
check_eq "inventory v2 packages" "$(sh "$inv" 0.2.0 packages | tr '\n' ' ')" \
	"garage-0.2.0-1.aarch64.rpm garage-0.2.0-1.x86_64.rpm garage_0.2.0_linux_amd64.deb garage_0.2.0_linux_arm64.deb "
check_eq "inventory v2 assets include the manifest and bundle" \
	"$(sh "$inv" 0.2.0 all | grep -c '^garage_0.2.0_checksums.txt')" 2
check_eq "a snapshot selects its base version's inventory" "$(sh "$inv" 0.2.0-snapshot+abc1234 id)" v2
check_eq "a snapshot rpm name writes the pre-release with ~" \
	"$(sh "$inv" 0.2.0-snapshot+abc1234 packages | grep x86_64)" "garage-0.2.0~snapshot+abc1234-1.x86_64.rpm"
refuses "inventory refuses a version with v" sh "$inv" v0.2.0
refuses "inventory refuses a partial version" sh "$inv" 0.2
refuses "inventory refuses two pre-release dashes" sh "$inv" 0.2.0-rc-1
refuses "inventory refuses an unknown selection" sh "$inv" 0.2.0 binaries

# install.sh's release mode names the archive with release-inventory.sh's
# rule: the piped installer has no checkout, so it carries its own copy.
installer=$scripts/../../install.sh
if grep -F 'archive=garage_${version}_${os}_${arch}.tar.gz' "$installer" >/dev/null; then
	pass "install.sh names the archive garage_<version>_<os>_<arch>.tar.gz"
else
	fail "install.sh names the archive garage_<version>_<os>_<arch>.tar.gz"
fi
for v in 0.1.1 0.2.1; do
	for os in darwin linux; do
		for arch in amd64 arm64; do
			check_eq "inventory for $v lists install.sh's ${os}/${arch} archive" \
				"$(sh "$inv" "$v" archives | grep -cFx "garage_${v}_${os}_${arch}.tar.gz")" 1
		done
	done
done

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
