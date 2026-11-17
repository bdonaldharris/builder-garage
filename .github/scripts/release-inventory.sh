#!/bin/sh
# release-inventory.sh: the single definition of the files a garage release
# publishes (docs/adr/0003-release-distribution.md, "Release inventory").
#
#   release-inventory.sh VERSION [all | manifest | archives | packages | id]
#
# VERSION is the release version without its "v", for example 0.2.0. A
# snapshot version such as 0.2.0-snapshot+abc1234 selects the inventory of
# its base version.
#
#   all       every release asset: the manifest entries, the checksum
#             manifest, and its Sigstore bundle (the default)
#   manifest  the files the checksum manifest lists
#   archives  the six platform archives
#   packages  the Linux packages (none in inventory v1)
#   id        the inventory's name: v1 or v2
#
# Inventories:
#   v1  up to v0.1.x: six archives, the manifest, and its bundle (8 files)
#   v2  from v0.2.0: v1, plus .deb and .rpm packages for linux/amd64 and
#       linux/arm64 (12 files)
#
# Names are printed one per line, sorted. Every check that needs a release's
# file names reads them from here, so no other list exists.

set -eu

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
	echo "usage: release-inventory.sh VERSION [all|manifest|archives|packages|id]" >&2
	exit 2
fi
version=$1
what=${2:-all}

# One optional pre-release or build suffix, without a further "-": rpm turns
# the first "-" into "~", and the rule here must match it exactly.
if ! printf '%s\n' "$version" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)([-+][0-9A-Za-z.+]*)?$'; then
	echo "release-inventory: '$version' is not a version of the form MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]" >&2
	exit 2
fi

base=$(printf '%s\n' "$version" | sed 's/[-+].*$//')
major=${base%%.*}
rest=${base#*.}
minor=${rest%%.*}

if [ "$major" -eq 0 ] && [ "$minor" -lt 2 ]; then
	id=v1
else
	id=v2
fi

archives() {
	for t in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
		echo "garage_${version}_$t.tar.gz"
	done
	for t in windows_amd64 windows_arm64; do
		echo "garage_${version}_$t.zip"
	done
}

packages() {
	[ "$id" = v2 ] || return 0
	# rpm versions cannot contain "-": nfpm writes a pre-release as "~".
	rpm_version=$(printf '%s\n' "$version" | sed 's/-/~/')
	echo "garage_${version}_linux_amd64.deb"
	echo "garage_${version}_linux_arm64.deb"
	echo "garage-${rpm_version}-1.x86_64.rpm"
	echo "garage-${rpm_version}-1.aarch64.rpm"
}

manifest=garage_${version}_checksums.txt

case $what in
all) { archives; packages; echo "$manifest"; echo "$manifest.sigstore.json"; } | LC_ALL=C sort ;;
manifest) { archives; packages; } | LC_ALL=C sort ;;
archives) archives | LC_ALL=C sort ;;
packages) packages | LC_ALL=C sort ;;
id) echo "$id" ;;
*)
	echo "release-inventory: unknown selection '$what'" >&2
	exit 2
	;;
esac
