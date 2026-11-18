#!/bin/sh
# install.sh: install garage for the current user, built from this checkout or
# downloaded as a verified published release.
#
#   ./install.sh [--install-dir PATH]                       from a checkout: build
#   ./install.sh --release [--version vX.Y.Z] [--install-dir PATH]
#   curl -fsSL https://raw.githubusercontent.com/bdonaldharris/builder-garage/vX.Y.Z/install.sh |
#     sh -s -- --version vX.Y.Z [--install-dir PATH]        a published release
#
# It installs garage in ~/.local/bin, or in the directory given with
# --install-dir. It may create that directory. It never edits shell startup
# files or PATH, and it never runs "garage skills install".
#
# From source (the default when this script runs as a file in a checkout of
# the builder-garage repository): it builds garage with the same flags as
# "make build". The version "garage version" reports is derived from this
# checkout's Git state, as with "make build"
# (docs/adr/0002-semantic-versioning.md): the tag at a clean tagged commit,
# such as v0.1.0; Git's descriptive form after it, such as v0.1.0-3-gabc1234;
# the abbreviated commit when no version tag is reachable; "-dirty" appended
# when the checkout has local changes; and "dev" without Git metadata. Set
# VERSION to override it, as with "make build VERSION=1.2.3". Apart from Go's
# own build and module caches and a temporary build directory it removes, it
# writes nothing outside the install directory: not the checkout, not any
# project. It requires the Go version that go.mod declares, and it never lets
# Go download a toolchain (GOTOOLCHAIN=local).
#
# A published release (with --release or --version, or when the script is not
# running as a file in a checkout, for example piped from curl): it downloads
# the release's archive for this platform, its checksum manifest, and the
# manifest's Sigstore bundle. Before installing anything, it verifies the
# manifest's signature with cosign, pinned to this repository's release
# workflow at that tag, then the archive's SHA-256 against the manifest
# (docs/release-verification.md). If any check fails or cannot run, including
# when cosign is missing, it installs nothing. It requires curl, tar, cosign
# v3 or later, and sha256sum or shasum. Without --version, it installs the
# latest release.

set -eu

prog=install.sh
repo=bdonaldharris/builder-garage

die() {
	printf '%s: %s\n' "$prog" "$*" >&2
	exit 1
}

usage() {
	cat <<'EOF'
Usage: ./install.sh [--install-dir PATH]
       ./install.sh --release [--version vX.Y.Z] [--install-dir PATH]
       curl -fsSL https://raw.githubusercontent.com/bdonaldharris/builder-garage/vX.Y.Z/install.sh |
         sh -s -- --version vX.Y.Z [--install-dir PATH]

Install garage for the current user: built from this checkout, or downloaded
as a published release and verified before it is installed.

Options:
  --install-dir PATH   install garage in PATH (default: ~/.local/bin)
  --release            install a published release instead of building
  --version vX.Y.Z     the release to install (implies --release; default:
                       the latest release)
  -h, --help           show this help

Environment (building from source only):
  VERSION              version reported by "garage version" (default: derived
                       from this checkout's Git tags, for example v0.1.0;
                       "dev" without Git metadata)

A release is installed only after its checksum manifest's signature verifies
with cosign (v3 or later), pinned to this repository's release workflow at
that tag, and the archive matches the manifest. Otherwise nothing is
installed.

The installer may create the install directory. It never edits shell startup
files or PATH, and it never runs "garage skills install".
EOF
}

# Parse arguments.
install_dir=
release=
want_version=
while [ $# -gt 0 ]; do
	case $1 in
	--install-dir)
		[ $# -ge 2 ] || die "--install-dir needs a PATH"
		install_dir=$2
		[ -n "$install_dir" ] || die "--install-dir needs a non-empty PATH"
		shift 2
		;;
	--install-dir=*)
		install_dir=${1#--install-dir=}
		[ -n "$install_dir" ] || die "--install-dir needs a non-empty PATH"
		shift
		;;
	--release)
		release=1
		shift
		;;
	--version)
		[ $# -ge 2 ] || die "--version needs a version, for example v0.2.1"
		want_version=$2
		release=1
		shift 2
		;;
	--version=*)
		want_version=${1#--version=}
		release=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		printf '%s: unknown argument: %s\n\n' "$prog" "$1" >&2
		usage >&2
		exit 2
		;;
	esac
done

if [ -z "$install_dir" ]; then
	[ -n "${HOME:-}" ] || die "HOME is not set; use --install-dir PATH"
	install_dir=$HOME/.local/bin
fi

# Make the install directory absolute, relative to where the installer ran,
# and drop any trailing slashes.
case $install_dir in
/*) ;;
*) install_dir=$(pwd)/$install_dir ;;
esac
while [ "$install_dir" != / ] && [ "${install_dir%/}" != "$install_dir" ]; do
	install_dir=${install_dir%/}
done

# Which mode: build from this checkout, or install a published release. Piped
# through sh, $0 is the shell, not this file, so there is no checkout.
root=
if [ -f "$0" ] && [ "${0##*/}" = install.sh ]; then
	dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd) || dir=
	if [ -n "$dir" ] && [ -f "$dir/go.mod" ] && [ -d "$dir/cmd/garage" ]; then
		root=$dir
	fi
fi
if [ -n "$release" ] || [ -z "$root" ]; then
	mode=release
else
	mode=source
fi

build_dir=
staged=
cleanup() {
	[ -z "$staged" ] || rm -f -- "$staged"
	[ -z "$build_dir" ] || rm -rf -- "$build_dir"
}

if [ "$mode" = source ]; then
	# Check the Go version before anything is built or installed.
	required=$(sed -n 's/^go[[:space:]]\{1,\}\([0-9][0-9.]*\).*$/\1/p' "$root/go.mod" | head -n 1)
	[ -n "$required" ] || die "cannot read the Go version from $root/go.mod"

	command -v go >/dev/null 2>&1 ||
		die "Go $required or later is required, and no go command is on PATH. Install Go $required, then run install.sh again."

	# GOTOOLCHAIN=local keeps Go from downloading a newer toolchain. "go env" runs
	# outside the module, so go.mod's requirement cannot stop it from reporting.
	found=$(cd / && GOTOOLCHAIN=local go env GOVERSION 2>/dev/null) || found=
	found=${found#go}

	# version_at_least FOUND REQUIRED: succeed when FOUND >= REQUIRED, comparing
	# numeric dot-separated components. A suffix such as "rc1" is ignored.
	version_at_least() {
		f=$1
		r=$2
		while [ -n "$f" ] || [ -n "$r" ]; do
			fc=${f%%.*}
			rc=${r%%.*}
			fc=${fc%%[!0-9]*}
			rc=${rc%%[!0-9]*}
			fc=${fc:-0}
			rc=${rc:-0}
			[ "$fc" -gt "$rc" ] && return 0
			[ "$fc" -lt "$rc" ] && return 1
			case $f in *.*) f=${f#*.} ;; *) f= ;; esac
			case $r in *.*) r=${r#*.} ;; *) r= ;; esac
		done
		return 0
	}

	case $found in
	[0-9]*) ;;
	*) die "Go $required or later is required, and the go command on PATH did not report a usable version. Install Go $required, then run install.sh again." ;;
	esac
	version_at_least "$found" "$required" ||
		die "Go $required or later is required, and the go command on PATH is Go $found. Install Go $required, then run install.sh again."

	# Build into a temporary directory, so nothing is written into the checkout.
	trap cleanup EXIT
	trap 'exit 1' HUP INT TERM

	build_dir=$(mktemp -d "${TMPDIR:-/tmp}/garage-install.XXXXXX") ||
		die "cannot create a temporary build directory"

	# derive_version prints the version to report when VERSION is unset. It
	# matches the Makefile's derivation, and never writes to the checkout:
	# "git describe --dirty" would rewrite the index, so modifications are
	# detected with "git status" and GIT_OPTIONAL_LOCKS=0 instead. Untracked
	# files count as modifications, as they do for Go's own vcs.modified.
	derive_version() {
		command -v git >/dev/null 2>&1 || {
			echo dev
			return 0
		}
		top=$(git -C "$root" rev-parse --show-toplevel 2>/dev/null) || {
			echo dev
			return 0
		}
		# A checkout without its own Git metadata, such as an extracted source
		# archive, must not report the version of an enclosing repository.
		[ "$(cd "$top" && pwd -P)" = "$(cd "$root" && pwd -P)" ] || {
			echo dev
			return 0
		}
		v=$(git -C "$root" describe --tags --match 'v[0-9]*' --always 2>/dev/null) || {
			echo dev
			return 0
		}
		if [ -n "$(GIT_OPTIONAL_LOCKS=0 git -C "$root" status --porcelain 2>/dev/null)" ]; then
			v=$v-dirty
		fi
		echo "$v"
	}

	version=${VERSION:-$(derive_version)}
	printf 'Building garage %s with Go %s\n' "$version" "$found"
	(
		cd "$root"
		CGO_ENABLED=0 GOTOOLCHAIN=local go build -trimpath \
			-ldflags "-X main.version=$version" -o "$build_dir/garage" ./cmd/garage
	) || die "the build failed; nothing was installed"

	"$build_dir/garage" version >/dev/null 2>&1 ||
		die "the built garage does not run; nothing was installed"
else
	# A published release, verified before anything is installed.
	need() {
		command -v "$1" >/dev/null 2>&1 || die "$2 Nothing was installed."
	}
	need curl "curl is required to download a release."
	need tar "tar is required to unpack a release."
	need uname "uname is required to detect this platform."
	need cosign "cosign v3 or later is required to verify the release's signature, and it is not on PATH. Install it (https://docs.sigstore.dev/cosign/system_config/installation/), or build from source with ./install.sh from a checkout."
	if command -v sha256sum >/dev/null 2>&1; then
		sha256() { sha256sum "$1" | cut -d' ' -f1; }
	elif command -v shasum >/dev/null 2>&1; then
		sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
	else
		die "sha256sum or shasum is required to check the release. Nothing was installed."
	fi

	cosign_version=$(cosign version 2>/dev/null | sed -n 's/^GitVersion:[[:space:]]*v\{0,1\}\([0-9][0-9]*\)\..*/\1/p' | head -n 1)
	case $cosign_version in
	'' | *[!0-9]*) die "cannot tell which cosign version is on PATH; cosign v3 or later is required. Nothing was installed." ;;
	esac
	[ "$cosign_version" -ge 3 ] ||
		die "cosign v3 or later is required; the cosign on PATH is v$cosign_version. Nothing was installed."

	case $(uname -s) in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*) die "release archives are published for macOS and Linux; this is $(uname -s). Nothing was installed." ;;
	esac
	case $(uname -m) in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) die "release archives are published for amd64 and arm64; this is $(uname -m). Nothing was installed." ;;
	esac

	# GARAGE_INSTALL_BASE_URL is for testing only: it serves release files from
	# elsewhere. It cannot weaken verification, because the signature is still
	# pinned to this repository's release workflow at the tag.
	base=${GARAGE_INSTALL_BASE_URL:-https://github.com/$repo/releases}
	case $base in
	file://*) proto='=file' ;;
	*) proto='=https' ;;
	esac

	is_version_tag() {
		printf '%s\n' "$1" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
	}
	if [ -n "$want_version" ]; then
		tag=$want_version
		is_version_tag "$tag" || die "--version must be a release tag of the form vMAJOR.MINOR.PATCH, such as v0.2.1; got '$tag'"
	else
		# The latest release, from the redirect GitHub serves (no API).
		location=$(curl -fsS --proto "$proto" -o /dev/null -w '%{redirect_url}' "$base/latest" 2>/dev/null) ||
			die "cannot find the latest release at $base/latest. Nothing was installed."
		tag=${location##*/}
		is_version_tag "$tag" || die "cannot find the latest release: $base/latest did not lead to a version tag. Nothing was installed."
	fi
	version=${tag#v}

	trap cleanup EXIT
	trap 'exit 1' HUP INT TERM
	build_dir=$(mktemp -d "${TMPDIR:-/tmp}/garage-install.XXXXXX") ||
		die "cannot create a temporary directory"

	# The archive name is release-inventory.sh's rule for this platform.
	archive=garage_${version}_${os}_${arch}.tar.gz
	manifest=garage_${version}_checksums.txt
	bundle=$manifest.sigstore.json
	printf 'Downloading garage %s for %s/%s\n' "$tag" "$os" "$arch"
	for f in "$archive" "$manifest" "$bundle"; do
		curl -fsSL --proto "$proto" -o "$build_dir/$f" "$base/download/$tag/$f" 2>"$build_dir/curl.err" ||
			die "cannot download $f for $tag ($(head -n 1 "$build_dir/curl.err")). Nothing was installed."
	done

	identity=https://github.com/$repo/.github/workflows/release.yml@refs/tags/$tag
	issuer=https://token.actions.githubusercontent.com
	if cosign verify-blob "$build_dir/$manifest" --bundle "$build_dir/$bundle" \
		--certificate-identity "$identity" --certificate-oidc-issuer "$issuer" >"$build_dir/cosign.out" 2>&1; then
		printf 'Verified the checksum manifest: signed by %s\n' "$identity"
	else
		sed 's/^/  /' "$build_dir/cosign.out" >&2
		die "the checksum manifest for $tag is not signed by $identity. Nothing was installed."
	fi

	expected=$(awk -v n="$archive" '{ f = $2; sub(/^\*/, "", f); if (f == n) print $1 }' "$build_dir/$manifest")
	case $expected in
	'' | *[!0-9a-f]*) die "the verified manifest has no single SHA-256 for $archive. Nothing was installed." ;;
	esac
	actual=$(sha256 "$build_dir/$archive")
	[ "$actual" = "$expected" ] ||
		die "$archive does not match the verified manifest (SHA-256 $actual, expected $expected). Nothing was installed."
	printf 'Verified %s against the manifest\n' "$archive"

	mkdir "$build_dir/x"
	tar -xzf "$build_dir/$archive" -C "$build_dir/x" garage ||
		die "cannot unpack garage from $archive. Nothing was installed."
	mv -- "$build_dir/x/garage" "$build_dir/garage"
	got=$("$build_dir/garage" version 2>/dev/null) ||
		die "the downloaded garage does not run on this machine. Nothing was installed."
	[ "$got" = "garage version $tag" ] ||
		die "the downloaded garage reports '$got', not '$tag'. Nothing was installed."
fi

# Install: stage a copy in the install directory, then rename it over the
# target, so an interrupted run never leaves a partly written garage.
target=$install_dir/garage
if [ -e "$install_dir" ] && [ ! -d "$install_dir" ]; then
	die "$install_dir exists and is not a directory"
fi
if [ ! -d "$install_dir" ]; then
	mkdir -p -- "$install_dir" || die "cannot create $install_dir"
	printf 'Created %s\n' "$install_dir"
fi
if [ -d "$target" ]; then
	die "$target is a directory; nothing was installed"
fi
replacing=
if [ -e "$target" ] || [ -L "$target" ]; then
	replacing=1
fi

staged=$install_dir/.garage.install.$$
cp -- "$build_dir/garage" "$staged" || die "cannot write to $install_dir"
chmod 755 "$staged" || die "cannot make $staged executable"
mv -f -- "$staged" "$target" || die "cannot install $target"
staged=

if [ -n "$replacing" ]; then
	printf 'Replaced the existing %s\n' "$target"
else
	printf 'Installed %s\n' "$target"
fi
"$target" version

# Report, but never make, any PATH change.
case :${PATH:-}: in
*:"$install_dir":* | *:"$install_dir"/:*)
	resolved=$(command -v garage 2>/dev/null || true)
	if [ -n "$resolved" ] && [ "$resolved" != "$target" ]; then
		printf '\nNote: "garage" on your PATH runs %s, which comes before %s.\n' "$resolved" "$target"
	fi
	;;
*)
	printf '\n%s is not on your PATH. To run garage by name, add it to PATH in\n' "$install_dir"
	printf 'your shell startup file, for example:\n\n'
	printf '  export PATH="%s:$PATH"\n' "$install_dir"
	printf '\nThe installer does not change PATH or shell startup files itself.\n'
	;;
esac

printf '\nSkills are installed separately, per project. In a project with a\n'
printf '.garage/config.yaml, see "garage skills install --help".\n'
