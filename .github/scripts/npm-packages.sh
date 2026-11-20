#!/bin/sh
# npm-packages.sh: generate the npm packages for one garage release (#158).
#
#   npm-packages.sh VERSION ARCHIVES OUT
#
#   VERSION   the release version without its "v", for example 0.4.0 (a
#             snapshot version, such as 0.4.0-snapshot+abc1234, in CI)
#   ARCHIVES  a directory holding the release's checksum manifest,
#             garage_<VERSION>_checksums.txt, and the archives it names
#   OUT       a directory to create, which must not exist yet
#
# It writes one directory per package to OUT:
#   - garage: the launcher, @buildergarage/garage, whose bin is npm/garage.js
#     and whose optionalDependencies are the platform packages, each pinned
#     to exactly VERSION;
#   - garage-<os>-<cpu>: one platform package per entry of NPM_PLATFORMS,
#     with npm's "os" and "cpu" fields, holding bin/garage from that
#     platform's release archive, byte for byte.
#
# Before extracting anything, it checks every archive it uses against the
# manifest, and refuses a missing or mismatched archive. No package has an
# install script. The packages are published with npm-publish.sh.

set -eu

if [ "$#" -ne 3 ]; then
	echo "usage: npm-packages.sh VERSION ARCHIVES OUT" >&2
	exit 2
fi
version=$1
archives=$2
out=$3

scripts=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
root=$(CDPATH='' cd -- "$scripts/../.." && pwd)

refuse() {
	echo "npm-packages: $1" >&2
	exit 1
}

# The npm platforms, one per line: npm os, npm cpu, release archive target.
# Windows is not published through npm until the clean Windows 11 test
# decides its status (#158; #120 decision 4 as amended):
#   win32 x64 windows_amd64
#   win32 arm64 windows_arm64
NPM_PLATFORMS='darwin arm64 darwin_arm64
darwin x64 darwin_amd64
linux arm64 linux_arm64
linux x64 linux_amd64'

scope=@buildergarage
repository=git+https://github.com/bdonaldharris/builder-garage.git
homepage=https://github.com/bdonaldharris/builder-garage

# release-inventory.sh refuses a malformed version.
sh "$scripts/release-inventory.sh" "$version" id >/dev/null ||
	refuse "'$version' is not a release version"
[ -d "$archives" ] || refuse "$archives is not a directory"
[ ! -e "$out" ] || refuse "$out already exists"

manifest=$archives/garage_${version}_checksums.txt
[ -f "$manifest" ] || refuse "the checksum manifest $manifest does not exist"

if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$1" | cut -d' ' -f1; }
else
	sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
fi

work=$(mktemp -d "${TMPDIR:-/tmp}/garage-npm-packages.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

# 1. Check every archive before using any of them.
echo "$NPM_PLATFORMS" | while read -r os cpu target; do
	archive=garage_${version}_${target}.tar.gz
	[ -f "$archives/$archive" ] || refuse "the archive $archive is missing from $archives"
	want=$(awk -v f="$archive" '$2 == f { print $1 }' "$manifest")
	[ -n "$want" ] || refuse "the manifest does not list $archive"
	got=$(sha256 "$archives/$archive")
	[ "$got" = "$want" ] || refuse "$archive does not match the manifest (got $got, want $want)"
done

# 2. The platform packages.
mkdir -p "$out"
echo "$NPM_PLATFORMS" | while read -r os cpu target; do
	name=garage-$os-$cpu
	dir=$out/$name
	mkdir -p "$dir/bin" "$work/$name"
	tar -xzf "$archives/garage_${version}_${target}.tar.gz" -C "$work/$name" garage LICENSE
	cp "$work/$name/garage" "$dir/bin/garage"
	chmod 0755 "$dir/bin/garage"
	cp "$work/$name/LICENSE" "$dir/LICENSE"
	cat >"$dir/package.json" <<EOF
{
  "name": "$scope/$name",
  "version": "$version",
  "description": "The garage executable for $os-$cpu, used by $scope/garage.",
  "license": "MIT",
  "homepage": "$homepage",
  "repository": {
    "type": "git",
    "url": "$repository"
  },
  "os": ["$os"],
  "cpu": ["$cpu"],
  "files": ["bin/garage"],
  "publishConfig": {
    "access": "public"
  }
}
EOF
	cat >"$dir/README.md" <<EOF
# $scope/$name

The \`garage\` executable of [Builder Garage]($homepage) $version for $os-$cpu, byte for byte as in the release archive \`garage_${version}_${target}.tar.gz\`.

Don't install this package directly. Install \`$scope/garage\`, which selects it.
EOF
done

# 3. The launcher, pinned to exactly these platform packages.
deps=$(echo "$NPM_PLATFORMS" | awk -v s="$scope" -v v="$version" '
	{ printf "%s    \"%s/garage-%s-%s\": \"%s\"", (NR > 1 ? ",\n" : ""), s, $1, $2, v }')
dir=$out/garage
mkdir -p "$dir/bin"
cp "$root/npm/garage.js" "$dir/bin/garage.js"
chmod 0755 "$dir/bin/garage.js"
cp "$root/npm/README.md" "$dir/README.md"
cp "$root/LICENSE" "$dir/LICENSE"
cat >"$dir/package.json" <<EOF
{
  "name": "$scope/garage",
  "version": "$version",
  "description": "The Builder Garage command-line interface.",
  "license": "MIT",
  "homepage": "$homepage",
  "repository": {
    "type": "git",
    "url": "$repository"
  },
  "bin": {
    "garage": "bin/garage.js"
  },
  "files": ["bin/garage.js"],
  "engines": {
    "node": ">=20"
  },
  "optionalDependencies": {
$deps
  },
  "publishConfig": {
    "access": "public"
  }
}
EOF

echo "npm-packages: wrote the $scope packages for $version to $out:"
ls -1 "$out" | sed 's/^/  /'
