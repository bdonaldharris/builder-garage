#!/bin/sh
# check-channel-cask.sh: check the Homebrew cask GoReleaser generated against
# a release's checksum manifest, before it is published to the tap (#124).
#
#   check-channel-cask.sh CASK MANIFEST VERSION [REPOSITORY]
#
#   CASK        the generated cask, usually dist/homebrew/Casks/garage.rb
#   MANIFEST    the release's checksum manifest (garage_<version>_checksums.txt);
#               in the release workflow, the published, cosign-verified one
#   VERSION     the release version without its "v", for example 0.2.1
#   REPOSITORY  owner/name; defaults to bdonaldharris/builder-garage
#
# It checks that:
#   - the cask is "garage", at VERSION, installing the "garage" binary;
#   - it has Linux and macOS blocks;
#   - its downloads are exactly the release inventory's Linux and macOS
#     archives (release-inventory.sh), each from this repository's release
#     at vVERSION, each with the SHA-256 the manifest lists for it.

set -eu

if [ "$#" -lt 3 ] || [ "$#" -gt 4 ]; then
	echo "usage: check-channel-cask.sh CASK MANIFEST VERSION [REPOSITORY]" >&2
	exit 2
fi
cask=$1
manifest=$2
version=$3
repo=${4:-bdonaldharris/builder-garage}

scripts=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() {
	printf 'FAIL %s\n' "$1"
	failures=$((failures + 1))
}

for f in "$cask" "$manifest"; do
	if [ ! -f "$f" ]; then
		echo "check-channel-cask: $f does not exist" >&2
		exit 2
	fi
done

if grep -Eq '^cask "garage" do$' "$cask"; then pass "the cask is named garage"; else fail "the cask is named garage"; fi
got_version=$(sed -n 's/^  version "\(.*\)"$/\1/p' "$cask")
if [ "$got_version" = "$version" ]; then pass "the cask is at version $version"; else fail "the cask is at version $version (got: '$got_version')"; fi
if grep -Eq '^  binary "garage"$' "$cask"; then pass "the cask installs the garage binary"; else fail "the cask installs the garage binary"; fi
if grep -Eq '^  on_linux do$' "$cask"; then pass "the cask has an on_linux block"; else fail "the cask has an on_linux block"; fi
if grep -Eq '^  on_macos do$' "$cask"; then pass "the cask has an on_macos block"; else fail "the cask has an on_macos block"; fi

# Every download, as "<url> <sha256>", with the cask's #{version} expanded.
got=$(awk -v v="$version" '
	/^ *sha256 "/ { s = $2; gsub(/"/, "", s) }
	/^ *url "/ { u = $2; gsub(/"/, "", u); gsub(/#\{version\}/, v, u); print u, s }
' "$cask" | LC_ALL=C sort)

# What it must be: each Linux and macOS archive of the inventory, with its
# manifest checksum.
base=https://github.com/$repo/releases/download/v$version
want=
for name in $(sh "$scripts/release-inventory.sh" "$version" archives | grep -E '_(linux|darwin)_'); do
	sum=$(awk -v n="$name" '{ f = $2; sub(/^\*/, "", f); if (f == n) print $1 }' "$manifest")
	if [ -z "$sum" ]; then
		fail "the manifest lists $name"
		continue
	fi
	want="$want$base/$name $sum
"
done
want=$(printf '%s' "$want" | LC_ALL=C sort)

if [ -n "$got" ] && [ "$got" = "$want" ]; then
	pass "the cask's downloads are exactly the Linux and macOS archives, with the manifest's checksums"
else
	fail "the cask's downloads are exactly the Linux and macOS archives, with the manifest's checksums"
	printf 'want:\n%s\ngot:\n%s\n' "$want" "$got"
fi

if [ "$failures" -ne 0 ]; then
	echo "$failures check(s) failed"
	exit 1
fi
echo "the cask is valid for $version"
