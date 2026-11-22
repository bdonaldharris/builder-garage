#!/bin/sh
# check-brew-cask.sh: install the garage cask with Homebrew on macOS or Linux,
# check the installed garage, and uninstall it (#124).
#
#   GARAGE_PACKAGE_CHECK=1 check-brew-cask.sh --local CASK DIST BINARY_VERSION REVISION [OLDER_CASK OLDER_DIST OLDER_VERSION OLDER_REVISION]
#   GARAGE_PACKAGE_CHECK=1 check-brew-cask.sh --tap TAP BINARY_VERSION REVISION
#
#   --local  install CASK from a new local test tap, with its download URLs
#            pointed at the archives in DIST (file://), as pull-request CI does
#            before anything is published; optionally upgrade from OLDER_CASK
#   --tap    install the cask garage from the published tap TAP, for example
#            bdonaldharris/tap, as a user would
#
# Both modes trust the tap explicitly (brew trust --tap), as the
# documentation tells users to; tap trust is never bypassed. They check that:
#   - brew install --cask succeeds, and puts garage on Homebrew's bin;
#   - that garage passes check-release-archive.sh: it reports BINARY_VERSION,
#     and installs skills whose revision is REVISION;
#   - when an older cask is supplied, brew upgrade replaces it with the new
#     version and the upgraded binary passes the same check;
#   - brew uninstall --cask removes it.
#
# It changes the machine's Homebrew installation, so it runs only on a
# disposable machine or container: CI sets GARAGE_PACKAGE_CHECK=1.

set -eu

if [ "${GARAGE_PACKAGE_CHECK:-}" != 1 ]; then
	echo "check-brew-cask: this changes Homebrew; run it only on a disposable machine, with GARAGE_PACKAGE_CHECK=1" >&2
	exit 2
fi
usage() {
	echo "usage: check-brew-cask.sh --local CASK DIST BINARY_VERSION REVISION [OLDER_CASK OLDER_DIST OLDER_VERSION OLDER_REVISION]" >&2
	echo "       check-brew-cask.sh --tap TAP BINARY_VERSION REVISION" >&2
	exit 2
}
[ "$#" -ge 1 ] || usage
mode=$1
shift
case $mode in
--local) [ "$#" -eq 4 ] || [ "$#" -eq 8 ] || usage ;;
--tap) [ "$#" -eq 3 ] || usage ;;
*) usage ;;
esac

scripts=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() {
	printf 'FAIL %s\n' "$1"
	failures=$((failures + 1))
}

work=$(mktemp -d "${TMPDIR:-/tmp}/garage-brew-check.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

export HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_ENV_HINTS=1

write_local_cask() { # write_local_cask CASK DIST
	source_cask=$1
	source_dist=$2
	if [ ! -f "$source_cask" ]; then
		echo "check-brew-cask: $source_cask does not exist" >&2
		exit 2
	fi
	sed -E "s|url \"https://github.com/[^\"]*/([^/\"]*)\"|url \"file://$source_dist/\\1\"|" \
		"$source_cask" >"$tapdir/Casks/garage.rb"
	if grep -q 'url "file://' "$tapdir/Casks/garage.rb"; then
		pass "the test cask downloads from $source_dist"
	else
		fail "the test cask downloads from $source_dist"
	fi
}

case $mode in
--local)
	cask=$1
	dist=$(CDPATH='' cd -- "$2" && pwd)
	bver=$3
	rev=$4
	older_cask=${5:-}
	older_dist=
	older_bver=
	older_rev=
	if [ -n "$older_cask" ]; then
		older_dist=$(CDPATH='' cd -- "$6" && pwd)
		older_bver=$7
		older_rev=$8
	fi
	tap=garage-check/local
	brew untap "$tap" >/dev/null 2>&1 || true
	if brew tap-new --no-git "$tap" >"$work/tap.out" 2>&1; then pass "creates the local tap $tap"; else
		cat "$work/tap.out"
		fail "creates the local tap $tap"
	fi
	tapdir=$(brew --repository "$tap")
	mkdir -p "$tapdir/Casks"
	# Point each cask version at the archives from its own snapshot.
	if [ -n "$older_cask" ]; then
		write_local_cask "$older_cask" "$older_dist"
	else
		write_local_cask "$cask" "$dist"
	fi
	;;
--tap)
	tap=$1
	bver=$2
	rev=$3
	if brew tap "$tap" >"$work/tap.out" 2>&1; then pass "taps $tap"; else
		cat "$work/tap.out"
		fail "taps $tap"
	fi
	;;
esac

if brew trust --tap "$tap" >"$work/trust.out" 2>&1; then pass "trusts $tap"; else
	cat "$work/trust.out"
	fail "trusts $tap"
fi

if brew install --cask "$tap/garage" >"$work/install.out" 2>&1; then pass "brew install --cask $tap/garage"; else
	cat "$work/install.out"
	fail "brew install --cask $tap/garage"
fi

garage=$(brew --prefix)/bin/garage
check_installed() { # check_installed VERSION REVISION LABEL
	expected_version=$1
	expected_revision=$2
	label=$3
	if [ -x "$garage" ]; then
		pass "$label: garage is on Homebrew's bin ($garage)"
		tar -czf "$work/installed.tar.gz" -C "$(dirname -- "$garage")" garage
		# The archive check runs garage from a copy, so the link is resolved here.
		if sh "$scripts/check-release-archive.sh" "$work/installed.tar.gz" "$expected_version" "$expected_revision"; then
			pass "$label: installed garage passes check-release-archive.sh"
		else
			fail "$label: installed garage passes check-release-archive.sh"
		fi
	else
		fail "$label: garage is on Homebrew's bin ($garage)"
	fi
}

if [ -n "${older_cask:-}" ]; then
	check_installed "$older_bver" "$older_rev" "older cask"
	write_local_cask "$cask" "$dist"
	if brew outdated --cask "$tap/garage" >"$work/outdated.out" 2>&1 && [ -s "$work/outdated.out" ]; then
		pass "Homebrew reports the older cask as outdated"
	else
		cat "$work/outdated.out"
		fail "Homebrew reports the older cask as outdated"
	fi
	if brew upgrade --cask "$tap/garage" >"$work/upgrade.out" 2>&1; then pass "upgrades $tap/garage"; else
		cat "$work/upgrade.out"
		fail "upgrades $tap/garage"
	fi
fi
check_installed "$bver" "$rev" "current cask"

if brew uninstall --cask "$tap/garage" >"$work/uninstall.out" 2>&1; then pass "brew uninstall --cask $tap/garage"; else
	cat "$work/uninstall.out"
	fail "brew uninstall --cask $tap/garage"
fi
if [ ! -e "$garage" ]; then pass "uninstalling removes $garage"; else fail "uninstalling removes $garage"; fi

if [ "$mode" = --local ]; then brew untap "$tap" >/dev/null 2>&1 || true; fi

if [ "$failures" -ne 0 ]; then
	echo "$failures check(s) failed"
	exit 1
fi
echo "all Homebrew cask checks passed"
