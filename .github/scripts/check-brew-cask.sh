#!/bin/sh
# check-brew-cask.sh: install the garage cask with Homebrew on Linux, check
# the installed garage, and uninstall it (#124).
#
#   GARAGE_PACKAGE_CHECK=1 check-brew-cask.sh --local CASK DIST BINARY_VERSION REVISION
#   GARAGE_PACKAGE_CHECK=1 check-brew-cask.sh --tap TAP BINARY_VERSION REVISION
#
#   --local  install CASK from a new local test tap, with its download URLs
#            pointed at the archives in DIST (file://), as pull-request CI does
#            before anything is published
#   --tap    install the cask garage from the published tap TAP, for example
#            bdonaldharris/tap, as a user would
#
# Both modes trust the tap explicitly (brew trust --tap), as the
# documentation tells users to; tap trust is never bypassed. They check that:
#   - brew install --cask succeeds, and puts garage on Homebrew's bin;
#   - that garage passes check-release-archive.sh: it reports BINARY_VERSION,
#     and installs skills whose revision is REVISION;
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
	echo "usage: check-brew-cask.sh --local CASK DIST BINARY_VERSION REVISION" >&2
	echo "       check-brew-cask.sh --tap TAP BINARY_VERSION REVISION" >&2
	exit 2
}
[ "$#" -ge 1 ] || usage
mode=$1
shift
case $mode in
--local) [ "$#" -eq 4 ] || usage ;;
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

case $mode in
--local)
	cask=$1
	dist=$(CDPATH='' cd -- "$2" && pwd)
	bver=$3
	rev=$4
	tap=garage-check/local
	brew untap "$tap" >/dev/null 2>&1 || true
	if brew tap-new --no-git "$tap" >"$work/tap.out" 2>&1; then pass "creates the local tap $tap"; else
		cat "$work/tap.out"
		fail "creates the local tap $tap"
	fi
	tapdir=$(brew --repository "$tap")
	mkdir -p "$tapdir/Casks"
	# Point every release download at the archive of the same name in DIST.
	sed -E "s|url \"https://github.com/[^\"]*/([^/\"]*)\"|url \"file://$dist/\\1\"|" "$cask" >"$tapdir/Casks/garage.rb"
	if grep -q 'url "file://' "$tapdir/Casks/garage.rb"; then pass "the test cask downloads from $dist"; else fail "the test cask downloads from $dist"; fi
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
if [ -x "$garage" ]; then
	pass "garage is on Homebrew's bin ($garage)"
	tar -czf "$work/installed.tar.gz" -C "$(dirname -- "$garage")" garage
	# The archive check runs garage from a copy, so the link is resolved here.
	if sh "$scripts/check-release-archive.sh" "$work/installed.tar.gz" "$bver" "$rev"; then
		pass "the installed garage passes check-release-archive.sh"
	else
		fail "the installed garage passes check-release-archive.sh"
	fi
else
	fail "garage is on Homebrew's bin ($garage)"
fi

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
