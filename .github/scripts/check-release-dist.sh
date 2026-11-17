#!/bin/sh
# check-release-dist.sh: check the archives and checksum manifest GoReleaser
# wrote to dist/, and how each executable was built (.goreleaser.yaml).
#
#   check-release-dist.sh DIST ARCHIVE_VERSION BINARY_VERSION REVISION
#
#   DIST             GoReleaser's output directory, usually dist
#   ARCHIVE_VERSION  the version in asset names, for example 0.1.1
#   BINARY_VERSION   what "garage version" must report, for example v0.1.1
#   REVISION         the full commit the release is built from
#
# It checks that:
#   - there are exactly six archives, one per target, with the expected names;
#   - each holds exactly the executable, LICENSE, and README.md;
#   - the checksum manifest lists exactly those six archives, and matches them;
#   - each executable records CGO_ENABLED=0, -trimpath, and a clean build of
#     REVISION (vcs.modified=false), which is what makes installed skills
#     carry REVISION; is stripped (-s -w); and embeds BINARY_VERSION.
#
# It needs Go, tar, and unzip, and runs on macOS and Linux.

set -eu

if [ "$#" -ne 4 ]; then
	echo "usage: check-release-dist.sh DIST ARCHIVE_VERSION BINARY_VERSION REVISION" >&2
	exit 2
fi
dist=$1
aver=$2
bver=$3
rev=$4

targets="darwin_amd64 darwin_arm64 linux_amd64 linux_arm64 windows_amd64 windows_arm64"

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() {
	printf 'FAIL %s\n' "$1"
	failures=$((failures + 1))
}

sha256_check() { # sha256_check MANIFEST (run in the manifest's directory)
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum -c "$1"
	else
		shasum -a 256 -c "$1"
	fi
}

# stripped BINARY: true when BINARY has no symbol table (-s). Only undefined
# imports, which a macOS executable keeps for libSystem, may remain.
stripped() {
	if ! syms=$(go tool nm "$1" 2>&1); then
		case $syms in
		*"no symbol"*) return 0 ;;
		*) return 1 ;;
		esac
	fi
	printf '%s\n' "$syms" | awk '$1 != "U" { found = 1 } END { exit found }'
}

work=$(mktemp -d "${TMPDIR:-/tmp}/garage-dist-check.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

# Archives: names and contents.
expected_archives=
for t in $targets; do
	case $t in
	windows_*) name=garage_${aver}_$t.zip exe=garage.exe ;;
	*) name=garage_${aver}_$t.tar.gz exe=garage ;;
	esac
	expected_archives="$expected_archives $name"
	archive=$dist/$name
	if [ ! -f "$archive" ]; then
		fail "archive $name exists"
		continue
	fi
	pass "archive $name exists"
	case $name in
	*.zip) entries=$(unzip -Z1 "$archive") ;;
	*) entries=$(tar -tzf "$archive") ;;
	esac
	got=$(printf '%s\n' "$entries" | sort | tr '\n' ' ')
	want=$(printf '%s\n' "$exe" LICENSE README.md | sort | tr '\n' ' ')
	if [ "$got" = "$want" ]; then pass "$name holds exactly $want"; else fail "$name holds exactly $want (got: $got)"; fi
done

found=$(cd "$dist" && ls -1 | grep -E '\.(tar\.gz|zip)$' | sort | tr '\n' ' ')
want=$(printf '%s\n' $expected_archives | sort | tr '\n' ' ')
if [ "$found" = "$want" ]; then pass "dist holds no other archives"; else fail "dist holds no other archives (got: $found)"; fi

# Checksum manifest.
manifest=garage_${aver}_checksums.txt
if [ -f "$dist/$manifest" ]; then
	pass "manifest $manifest exists"
	listed=$(awk '{ sub(/^\*/, "", $2); print $2 }' "$dist/$manifest" | sort | tr '\n' ' ')
	if [ "$listed" = "$want" ]; then pass "manifest lists exactly the six archives"; else fail "manifest lists exactly the six archives (got: $listed)"; fi
	if (cd "$dist" && sha256_check "$manifest" >/dev/null); then pass "manifest matches the archives"; else fail "manifest matches the archives"; fi
else
	fail "manifest $manifest exists"
fi

# Executables: build settings.
count=0
for t in $targets; do
	case $t in
	windows_*) exe=garage.exe ;;
	*) exe=garage ;;
	esac
	case $t in
	windows_*) archive=$dist/garage_${aver}_$t.zip ;;
	*) archive=$dist/garage_${aver}_$t.tar.gz ;;
	esac
	[ -f "$archive" ] || continue
	mkdir -p "$work/$t"
	case $archive in
	*.zip) unzip -q -o "$archive" "$exe" -d "$work/$t" ;;
	*) tar -xzf "$archive" -C "$work/$t" "$exe" ;;
	esac
	info=$(go version -m "$work/$t/$exe")
	count=$((count + 1))
	for want_line in "-trimpath=true" "CGO_ENABLED=0" "vcs.revision=$rev" "vcs.modified=false"; do
		if printf '%s\n' "$info" | grep -F -- "	$want_line" >/dev/null; then pass "$t: $want_line"; else fail "$t: $want_line"; fi
	done
	# Go omits -ldflags from the build information of a -trimpath build, so
	# the link-time settings are checked in the executable itself.
	if stripped "$work/$t/$exe"; then pass "$t: stripped (-s -w)"; else fail "$t: stripped (-s -w)"; fi
	# A static check of the embedded version string. Each archive's
	# "garage version" is run on its own platform by check-release-archive.sh.
	if grep -a -F -- "$bver" "$work/$t/$exe" >/dev/null; then pass "$t: embeds $bver"; else fail "$t: embeds $bver"; fi
done
if [ "$count" -eq 6 ]; then pass "six executables checked"; else fail "six executables checked (got $count)"; fi

if [ "$failures" -ne 0 ]; then
	echo "$failures check(s) failed"
	exit 1
fi
echo "all release dist checks passed"
