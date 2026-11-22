#!/bin/sh
# macos-installer-identity.sh: select the Developer ID Installer identity that
# build-macos-package.sh signs with, from "security find-identity" output.
#
#   macos-installer-identity.sh VALID_LISTING [ALL_LISTING]
#
#   VALID_LISTING  a file holding "security find-identity -v -p basic KEYCHAIN"
#                  output: valid identities only
#   ALL_LISTING    optionally, a file holding the same command's output without
#                  -v, used only to explain a refusal
#
# It prints the name of the first valid "Developer ID Installer: ... (38397A2FG6)"
# identity in VALID_LISTING, and exits 0. Otherwise it exits 1 and says that
# no such identity exists; when ALL_LISTING holds a matching identity, it also
# says that one is present but not valid (for example untrusted or expired).
# Only identity names are read and printed: never certificates or keys.
#
# The selection is kept apart from the keychain so that it can be tested on
# any platform (test-release-scripts.sh, #182).

set -eu

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
	echo "usage: macos-installer-identity.sh VALID_LISTING [ALL_LISTING]" >&2
	exit 2
fi
valid=$1
all=${2:-}
[ -f "$valid" ] || {
	echo "macos-installer-identity: listing not found: $valid" >&2
	exit 2
}

team=38397A2FG6
# A listed identity is: N) HASH "NAME", optionally followed by a status.
pattern="s/^[[:space:]]*[0-9][0-9]*) [0-9A-F][0-9A-F]* \"\\(Developer ID Installer: [^\"]* ($team)\\)\".*/\\1/p"

identity=$(sed -n "$pattern" "$valid" | head -n 1)
if [ -n "$identity" ]; then
	printf '%s\n' "$identity"
	exit 0
fi

echo "macos-package: no Developer ID Installer identity for Team ID $team" >&2
if [ -n "$all" ] && [ -f "$all" ]; then
	present=$(sed -n "$pattern" "$all" | head -n 1)
	if [ -n "$present" ]; then
		echo "macos-package: '$present' is in the keychain but is not a valid identity (for example untrusted or expired)" >&2
	fi
fi
exit 1
