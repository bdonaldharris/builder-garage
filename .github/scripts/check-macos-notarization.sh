#!/bin/sh
# check-macos-notarization.sh: check that a release archive's macOS garage is
# Developer ID signed and notarized, and that Gatekeeper lets a quarantined
# copy run (#122, D3).
#
#   check-macos-notarization.sh ARCHIVE TEAM_ID
#
#   ARCHIVE  a darwin .tar.gz release archive for this machine's architecture
#   TEAM_ID  the Apple Team ID the signature must carry, for example 38397A2FG6
#
# It checks that the extracted garage:
#   - passes "codesign --verify --strict";
#   - is signed by a Developer ID Application identity of TEAM_ID, with the
#     hardened runtime and a secure timestamp;
#   - is accepted by Gatekeeper as "Notarized Developer ID". A bare
#     executable cannot be stapled, so this is Apple's online notarization
#     record. "spctl -t execute" rejects every command-line tool, notarized or
#     not, so the check uses the primary-signature context instead;
#   - runs "garage version" when quarantined, as a browser download is.
#
# GoReleaser only logs a notarization that times out or stays pending, so this
# check is what keeps such an executable from being signed into a release. It
# runs on macOS only, in a temporary directory it removes.

set -eu

if [ "$#" -ne 2 ]; then
	echo "usage: check-macos-notarization.sh ARCHIVE TEAM_ID" >&2
	exit 2
fi
archive=$1
team=$2

if [ "$(uname -s)" != Darwin ]; then
	echo "check-macos-notarization: needs macOS" >&2
	exit 2
fi

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() {
	printf 'FAIL %s\n' "$1"
	failures=$((failures + 1))
}

work=$(mktemp -d "${TMPDIR:-/tmp}/garage-notarization-check.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

tar -xzf "$archive" -C "$work" garage
garage=$work/garage

# 1. The signature is intact.
if codesign --verify --strict --verbose=2 "$garage" >"$work/verify.out" 2>&1; then
	pass "codesign --verify --strict"
else
	cat "$work/verify.out"
	fail "codesign --verify --strict"
fi

# 2. Who signed it, and how.
codesign -dvv "$garage" >"$work/display.out" 2>&1 || true
grep -E '^(Authority|TeamIdentifier|Timestamp)=' "$work/display.out" || true
if grep -Eq '^CodeDirectory .*flags=0x[0-9a-f]*\(([a-z-]+,)*runtime(,[a-z-]+)*\)' "$work/display.out"; then
	pass "hardened runtime"
else
	fail "hardened runtime"
fi
if sed -n '1,/^Authority=/p' "$work/display.out" | grep -q '^Authority=Developer ID Application: '; then
	pass "signed by a Developer ID Application identity"
else
	fail "signed by a Developer ID Application identity"
fi
if grep -qx "TeamIdentifier=$team" "$work/display.out"; then
	pass "team identifier $team"
else
	fail "team identifier $team"
fi
if grep -q '^Timestamp=' "$work/display.out"; then
	pass "secure timestamp"
else
	fail "secure timestamp"
fi

# 3. Gatekeeper's verdict.
spctl -a -vvv -t open --context context:primary-signature "$garage" >"$work/spctl.out" 2>&1 || true
cat "$work/spctl.out"
if grep -q ': accepted$' "$work/spctl.out" && grep -qx 'source=Notarized Developer ID' "$work/spctl.out"; then
	pass "Gatekeeper accepts it as Notarized Developer ID"
else
	fail "Gatekeeper accepts it as Notarized Developer ID"
fi

# 4. A quarantined copy runs. macOS kills a quarantined executable that
# Gatekeeper rejects, even from a terminal.
xattr -w com.apple.quarantine "0081;$(printf '%x' "$(date +%s)");garage-release-check;" "$garage"
if out=$("$garage" version 2>&1); then
	pass "a quarantined copy runs ($out)"
else
	status=$?
	fail "a quarantined copy runs (exit $status: $out)"
fi

if [ "$failures" -ne 0 ]; then
	echo "$failures check(s) failed"
	exit 1
fi
echo "all notarization checks passed for $archive"
