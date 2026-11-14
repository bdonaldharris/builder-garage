#!/bin/sh
# install.sh: build garage from this checkout and install it for the current user.
#
#   ./install.sh [--install-dir PATH]
#
# The installer builds garage with the same flags as "make build", and
# installs it in ~/.local/bin, or in the directory given with --install-dir.
# Set VERSION to choose the version "garage version" reports, as with
# "make build VERSION=1.2.3". The default is "dev".
#
# It may create the install directory. Apart from Go's own build and module
# caches and a temporary build directory it removes, it writes nothing outside
# the install directory: not the checkout, not any project, and not shell
# startup files or PATH. It never runs "garage skills install".
#
# It requires the Go version that go.mod declares, and it never lets Go
# download a toolchain (GOTOOLCHAIN=local).

set -eu

prog=install.sh

die() {
	printf '%s: %s\n' "$prog" "$*" >&2
	exit 1
}

usage() {
	cat <<'EOF'
Usage: ./install.sh [--install-dir PATH]

Build garage from this checkout and install it for the current user.

Options:
  --install-dir PATH   install garage in PATH (default: ~/.local/bin)
  -h, --help           show this help

Environment:
  VERSION              version reported by "garage version" (default: dev)

The installer may create the install directory. It never edits shell startup
files or PATH, and it never runs "garage skills install".
EOF
}

# Parse arguments.
install_dir=
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

# Find the checkout this script belongs to.
root=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd) || die "cannot find the checkout"
[ -f "$root/go.mod" ] && [ -d "$root/cmd/garage" ] ||
	die "run install.sh from a checkout of the builder-garage repository"

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
build_dir=
staged=
cleanup() {
	[ -z "$staged" ] || rm -f -- "$staged"
	[ -z "$build_dir" ] || rm -rf -- "$build_dir"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

build_dir=$(mktemp -d "${TMPDIR:-/tmp}/garage-install.XXXXXX") ||
	die "cannot create a temporary build directory"

version=${VERSION:-dev}
printf 'Building garage %s with Go %s\n' "$version" "$found"
(
	cd "$root"
	CGO_ENABLED=0 GOTOOLCHAIN=local go build -trimpath \
		-ldflags "-X main.version=$version" -o "$build_dir/garage" ./cmd/garage
) || die "the build failed; nothing was installed"

"$build_dir/garage" version >/dev/null 2>&1 ||
	die "the built garage does not run; nothing was installed"

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
