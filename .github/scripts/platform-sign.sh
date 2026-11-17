#!/bin/sh
# platform-sign.sh: the platform-signing hook GoReleaser runs after each build,
# before archiving (.goreleaser.yaml).
#
#   platform-sign.sh MODE GOOS BINARY
#
# MODE is GARAGE_RELEASE_MODE:
#   snapshot, dry-run  leave the binary unchanged. Nothing is signed.
#   release            sign the binary for its platform. Platform signing is
#                      track 2 of #120 (#122) and is not implemented yet, so
#                      release mode fails closed: no unsigned binary can reach
#                      a release, even if credentials are configured.
# Any other MODE, including an unset one, fails.

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
snapshot | dry-run)
	echo "platform-sign: $mode build; $goos binary left unsigned: $binary"
	;;
release)
	case $goos in
	darwin | windows)
		echo "platform-sign: platform signing for $goos is not implemented (#122); refusing to build a release" >&2
		exit 1
		;;
	*)
		echo "platform-sign: $goos binaries need no platform signature: $binary"
		;;
	esac
	;;
*)
	echo "platform-sign: GARAGE_RELEASE_MODE must be snapshot, dry-run, or release; got '$mode'" >&2
	exit 1
	;;
esac
