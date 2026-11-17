#!/bin/sh
# check-release-package.sh: install, upgrade, and remove a garage Linux
# package with its own package manager, and check it (#123).
#
#   GARAGE_PACKAGE_CHECK=1 check-release-package.sh PACKAGE BINARY_VERSION REVISION [OLDER_PACKAGE]
#
#   PACKAGE         a garage .deb (checked with apt and dpkg) or .rpm (checked
#                   with dnf and rpm), for this machine's architecture
#   BINARY_VERSION  what "garage version" must report, for example v0.2.0
#   REVISION        the full commit the release is built from
#   OLDER_PACKAGE   optional: an older package of the same format, installed
#                   first, so that PACKAGE is installed as an upgrade
#
# It checks that:
#   - the package is named garage, with the version and architecture its
#     file name states;
#   - it holds exactly /usr/bin/garage and its license file, and no
#     maintainer scripts;
#   - after installing (or upgrading to) it, /usr/bin/garage passes
#     check-release-archive.sh: it reports BINARY_VERSION, and installs
#     skills whose revision is REVISION;
#   - removing it removes both files;
#   - installing, upgrading, and removing it leave user data byte for byte
#     unchanged: a project's .garage/config.yaml, installed skills, and the
#     Builder Garage user configuration directory (manifest and history).
#
# It installs and removes system packages, as root or through sudo, and
# writes sentinel files under the home directory, so it runs only on a
# disposable machine or container: CI sets GARAGE_PACKAGE_CHECK=1.

set -eu

if [ "${GARAGE_PACKAGE_CHECK:-}" != 1 ]; then
	echo "check-release-package: this installs system packages; run it only on a disposable machine, with GARAGE_PACKAGE_CHECK=1" >&2
	exit 2
fi
if [ "$#" -lt 3 ] || [ "$#" -gt 4 ]; then
	echo "usage: check-release-package.sh PACKAGE BINARY_VERSION REVISION [OLDER_PACKAGE]" >&2
	exit 2
fi
abs() { (cd -- "$(dirname -- "$1")" && printf '%s/%s\n' "$(pwd)" "$(basename -- "$1")"); }
pkg=$(abs "$1")
bver=$2
rev=$3
older=
if [ "$#" -eq 4 ]; then older=$(abs "$4"); fi

scripts=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() {
	printf 'FAIL %s\n' "$1"
	failures=$((failures + 1))
}
as_root() {
	if [ "$(id -u)" -eq 0 ]; then "$@"; else sudo "$@"; fi
}

# The package version nfpm writes: no "v", and a pre-release "-" as "~".
pversion=$(printf '%s\n' "${bver#v}" | sed 's/-/~/')

name=$(basename -- "$pkg")
case $name in
*.deb)
	format=deb
	license=/usr/share/doc/garage/copyright
	case $name in
	*_linux_amd64.deb) arch=amd64 ;;
	*_linux_arm64.deb) arch=arm64 ;;
	*) fail "$name names a known architecture" ;;
	esac
	;;
*.rpm)
	format=rpm
	license=/usr/share/licenses/garage/LICENSE
	case $name in
	*.x86_64.rpm) arch=x86_64 ;;
	*.aarch64.rpm) arch=aarch64 ;;
	*) fail "$name names a known architecture" ;;
	esac
	;;
*)
	echo "check-release-package: $name is not a .deb or .rpm" >&2
	exit 2
	;;
esac

work=$(mktemp -d "${TMPDIR:-/tmp}/garage-package-check.XXXXXX")
sentinel_id=garage-package-check-$$
cleanup() {
	rm -rf -- "$work" "$HOME/.claude/skills/$sentinel_id" "$HOME/.config/builder-garage/$sentinel_id"
}
trap cleanup EXIT

# 1. Metadata and contents, before installing.
case $format in
deb)
	meta=$(dpkg-deb -f "$pkg" Package Version Architecture | tr '\n' ' ')
	files=$(dpkg-deb -c "$pkg" | awk '$1 !~ /^d/ { sub(/^\./, "", $6); print $6 }' | LC_ALL=C sort | tr '\n' ' ')
	scripts_in=$(dpkg-deb --ctrl-tarfile "$pkg" | tar -tf - | sed 's|^\./||' | grep -E '^(preinst|postinst|prerm|postrm|config|triggers)$' || true)
	;;
rpm)
	meta=$(rpm -qp --qf 'Package: %{NAME} Version: %{VERSION} Release: %{RELEASE} Architecture: %{ARCH}' "$pkg")
	files=$(rpm -qp --qf '[%{FILEMODES:perms} %{FILENAMES}\n]' "$pkg" | awk '$1 !~ /^d/ { print $2 }' | LC_ALL=C sort | tr '\n' ' ')
	scripts_in=$(rpm -qp --scripts --triggers "$pkg")
	;;
esac
case $format in
deb) want_meta="Package: garage Version: $pversion Architecture: $arch " ;;
rpm) want_meta="Package: garage Version: $pversion Release: 1 Architecture: $arch" ;;
esac
if [ "$meta" = "$want_meta" ]; then pass "$name metadata: $want_meta"; else fail "$name metadata: $want_meta (got: $meta)"; fi
want_files=$(printf '%s\n' /usr/bin/garage "$license" | LC_ALL=C sort | tr '\n' ' ')
if [ "$files" = "$want_files" ]; then pass "$name holds exactly $want_files"; else fail "$name holds exactly $want_files (got: $files)"; fi
if [ -z "$scripts_in" ]; then pass "$name has no maintainer scripts"; else fail "$name has no maintainer scripts (got: $scripts_in)"; fi

# 2. Sentinel user data, recorded before any package operation.
mkdir -p "$work/project/.garage" "$HOME/.claude/skills/$sentinel_id" "$HOME/.config/builder-garage/$sentinel_id"
printf 'schema_version: 2\n# sentinel\n' >"$work/project/.garage/config.yaml"
printf -- '---\nname: %s\n---\nsentinel\n' "$sentinel_id" >"$HOME/.claude/skills/$sentinel_id/SKILL.md"
printf '{"sentinel": true}\n' >"$HOME/.config/builder-garage/$sentinel_id/skills-manifest.json"
printf 'sentinel history\n' >"$HOME/.config/builder-garage/$sentinel_id/history.jsonl"
user_data() {
	for d in "$work/project/.garage" "$HOME/.claude/skills/$sentinel_id" "$HOME/.config/builder-garage/$sentinel_id"; do
		find "$d" -type f -exec cksum {} \;
	done | LC_ALL=C sort
}
before=$(user_data)
user_data_unchanged() { # user_data_unchanged STEP
	# Compared in the shell: minimal images such as Fedora's lack cmp.
	if [ "$(user_data)" = "$before" ]; then pass "user data unchanged after $1"; else fail "user data unchanged after $1"; fi
}

install_pkg() { # install_pkg FILE
	case $format in
	deb) as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y -q "$1" ;;
	rpm) as_root dnf install -y -q "$1" ;;
	esac
}
installed_version() {
	case $format in
	deb)
		# A removed package can still be known to dpkg; only "installed" counts.
		st=$(dpkg-query -W -f '${db:Status-Status} ${Version}' garage 2>/dev/null || true)
		case $st in installed\ *) printf '%s\n' "${st#installed }" ;; esac
		;;
	rpm)
		# rpm -q prints "package garage is not installed" on stdout, and fails.
		if rpm -q garage >/dev/null 2>&1; then rpm -q --qf '%{VERSION}' garage; fi
		;;
	esac
}

# 3. Install, or upgrade from OLDER_PACKAGE.
if [ -n "$older" ]; then
	if install_pkg "$older" >"$work/older.out" 2>&1; then pass "installs the older package $(basename -- "$older")"; else
		cat "$work/older.out"
		fail "installs the older package $(basename -- "$older")"
	fi
	old_version=$(installed_version)
	if [ -n "$old_version" ] && [ "$old_version" != "$pversion" ]; then pass "older version $old_version is installed"; else fail "an older version is installed (got: '$old_version')"; fi
	user_data_unchanged "installing the older package"
	step="upgrading to $name"
else
	step="installing $name"
fi
if install_pkg "$pkg" >"$work/install.out" 2>&1; then pass "$step"; else
	cat "$work/install.out"
	fail "$step"
fi
got_version=$(installed_version)
if [ "$got_version" = "$pversion" ]; then pass "installed version is $pversion"; else fail "installed version is $pversion (got: '$got_version')"; fi
if [ -f "$license" ]; then pass "$license is installed"; else fail "$license is installed"; fi
user_data_unchanged "$step"

# 4. The installed executable, through the archive check.
if [ -x /usr/bin/garage ]; then
	tar -czf "$work/installed.tar.gz" -C /usr/bin garage
	if sh "$scripts/check-release-archive.sh" "$work/installed.tar.gz" "$bver" "$rev"; then
		pass "/usr/bin/garage passes check-release-archive.sh"
	else
		fail "/usr/bin/garage passes check-release-archive.sh"
	fi
else
	fail "/usr/bin/garage is installed and executable"
fi
user_data_unchanged "running the installed garage"

# 5. Remove.
case $format in
deb) remove() { as_root env DEBIAN_FRONTEND=noninteractive apt-get remove -y -q garage; } ;;
rpm) remove() { as_root dnf remove -y -q garage; } ;;
esac
if remove >"$work/remove.out" 2>&1; then pass "removes the package"; else
	cat "$work/remove.out"
	fail "removes the package"
fi
if [ -z "$(installed_version)" ]; then pass "garage is no longer installed"; else fail "garage is no longer installed"; fi
if [ ! -e /usr/bin/garage ] && [ ! -e "$license" ]; then pass "removing it removes /usr/bin/garage and $license"; else
	fail "removing it removes /usr/bin/garage and $license"
fi
user_data_unchanged "removing the package"

if [ "$failures" -ne 0 ]; then
	echo "$failures check(s) failed"
	exit 1
fi
echo "all package checks passed for $name"
