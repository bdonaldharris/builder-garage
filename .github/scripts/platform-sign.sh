#!/bin/sh
# platform-sign.sh: the platform-signing hook GoReleaser runs after each build,
# before archiving (.goreleaser.yaml).
#
#   platform-sign.sh MODE GOOS BINARY
#
# MODE is GARAGE_RELEASE_MODE: snapshot, dry-run, or release. Any other MODE,
# including an unset one, fails, so GoReleaser never builds without a mode.
#
# Platform signing is track 2 of #120 (#122). It ships one platform at a time
# (#122, D2), so a release may carry signed macOS and unsigned Windows
# executables, and its notes say which is which.
#
# - macOS: in release mode, GoReleaser's notarize.macos signs each darwin
#   binary with the Developer ID Application identity (hardened runtime,
#   secure timestamp) and notarizes it, after this hook and before archiving.
#   This hook refuses a darwin release build without every signing credential
#   (check-macos-signing-env.sh), because GoReleaser would otherwise sign
#   without notarizing (#122, D3). Snapshot and dry-run builds are unsigned.
# - Windows: not Authenticode-signed yet (#122, phase 2), in any mode.
# - Linux: needs no platform signature.
#
# The hook never changes a binary itself.

set -eu

if [ "$#" -ne 3 ]; then
	echo "usage: platform-sign.sh MODE GOOS BINARY" >&2
	exit 2
fi
mode=$1
goos=$2
binary=$3

if [ ! -f "$binary" ]; then
	echo "platform-sign: $binary does not exist" >&2
	exit 1
fi

case $mode in
snapshot | dry-run | release) ;;
*)
	echo "platform-sign: GARAGE_RELEASE_MODE must be snapshot, dry-run, or release; got '$mode'" >&2
	exit 1
	;;
esac

case $mode/$goos in
release/darwin)
	sh "$(dirname -- "$0")/check-macos-signing-env.sh" release
	echo "platform-sign: release build; GoReleaser notarize.macos signs and notarizes this darwin binary next: $binary"
	;;
*/darwin)
	echo "platform-sign: $mode build; darwin binaries are signed and notarized only in release builds: $binary"
	;;
*/windows)
	echo "platform-sign: $mode build; windows binary is not platform-signed (#122, phase 2): $binary"
	;;
*)
	echo "platform-sign: $mode build; $goos binaries need no platform signature: $binary"
	;;
esac
