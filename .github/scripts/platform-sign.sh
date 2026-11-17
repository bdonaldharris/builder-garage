#!/bin/sh
# platform-sign.sh: the platform-signing hook GoReleaser runs after each build,
# before archiving (.goreleaser.yaml).
#
#   platform-sign.sh MODE GOOS BINARY
#
# MODE is GARAGE_RELEASE_MODE: snapshot, dry-run, or release. Any other MODE,
# including an unset one, fails, so GoReleaser never builds without a mode.
#
# Platform signing (macOS Developer ID signing and notarization, Windows
# Authenticode) is track 2 of #120 (#122) and is not implemented yet. In every
# mode the binary is left unchanged. Releases publish unsigned macOS and
# Windows executables, and their notes say so (amended #120 decision 4). Until
# #122 signs every platform together, nothing is signed, so no release mixes
# signed and unsigned platforms.

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
snapshot | dry-run | release)
	case $goos in
	darwin | windows)
		echo "platform-sign: $mode build; $goos binary is not platform-signed (#122): $binary"
		;;
	*)
		echo "platform-sign: $mode build; $goos binaries need no platform signature: $binary"
		;;
	esac
	;;
*)
	echo "platform-sign: GARAGE_RELEASE_MODE must be snapshot, dry-run, or release; got '$mode'" >&2
	exit 1
	;;
esac
