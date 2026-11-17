#!/bin/sh
# check-release-archive.sh: run a release archive's garage on this machine.
#
#   check-release-archive.sh ARCHIVE BINARY_VERSION REVISION
#
# The archive must be for this machine's platform. It checks that:
#   - "garage version" reports BINARY_VERSION, for example v0.1.1;
#   - "garage skills install", in a new project, installs skills whose
#     metadata.garage-revision is REVISION, the tagged commit, and never a
#     development revision;
#   - "garage skills verify" then reports every skill Current.
#
# Everything is written to a temporary directory used as the home directory,
# which is removed afterwards. It runs on macOS, Linux, and Windows (Git Bash),
# and needs git and tar, and unzip or PowerShell for a .zip archive.

set -eu

if [ "$#" -ne 3 ]; then
	echo "usage: check-release-archive.sh ARCHIVE BINARY_VERSION REVISION" >&2
	exit 2
fi
archive=$1
bver=$2
rev=$3

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() {
	printf 'FAIL %s\n' "$1"
	failures=$((failures + 1))
}

work=$(mktemp -d "${TMPDIR:-/tmp}/garage-archive-check.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

# A path as the garage executable sees it: Windows form under Git Bash.
native() {
	if command -v cygpath >/dev/null 2>&1; then cygpath -w "$1"; else printf '%s\n' "$1"; fi
}

mkdir -p "$work/bin"
case $archive in
*.zip)
	exe=garage.exe
	if command -v unzip >/dev/null 2>&1; then
		unzip -q "$archive" -d "$work/bin"
	else
		powershell -NoProfile -Command "Expand-Archive -LiteralPath '$(native "$archive")' -DestinationPath '$(native "$work/bin")'"
	fi
	;;
*.tar.gz)
	exe=garage
	tar -xzf "$archive" -C "$work/bin"
	;;
*)
	echo "check-release-archive: unsupported archive $archive" >&2
	exit 2
	;;
esac
garage=$work/bin/$exe

# 1. The version.
out=$("$garage" version)
if [ "$out" = "garage version $bver" ]; then pass "garage version reports $bver"; else fail "garage version reports $bver (got: $out)"; fi

# 2. Skills install into a sandboxed home, from a new project.
home=$work/home
proj=$work/project
mkdir -p "$home" "$proj"
HOME=$(native "$home")
USERPROFILE=$HOME
APPDATA=$(native "$home/AppData/Roaming")
LOCALAPPDATA=$(native "$home/AppData/Local")
XDG_CONFIG_HOME=$(native "$home/.config")
export HOME USERPROFILE APPDATA LOCALAPPDATA XDG_CONFIG_HOME

git -C "$proj" init -q
if (cd "$proj" && "$garage" init --discovery implementer-judgment --workflow-owner "lead maintainer" \
	--change-acceptor maintainer --product-intent-acceptor "Workflow Owner" \
	--skill-target claude-code@0.0.0@2026-01-01 </dev/null >"$work/init.out" 2>&1); then
	pass "garage init creates a project configuration"
else
	cat "$work/init.out"
	fail "garage init creates a project configuration"
fi

if (cd "$proj" && "$garage" skills install --target claude-code </dev/null >"$work/install.out" 2>&1); then
	pass "garage skills install succeeds without --development"
else
	cat "$work/install.out"
	fail "garage skills install succeeds without --development"
fi

find "$home/.claude/skills" -name SKILL.md 2>/dev/null | sort >"$work/skills.txt" || true
n=$(grep -c . "$work/skills.txt" || true)
if [ "$n" -gt 0 ]; then pass "installed $n skills"; else fail "installed at least one skill"; fi
bad=0
while IFS= read -r f; do
	got=$(sed -n 's/^  garage-revision: "\(.*\)"$/\1/p' "$f")
	if [ "$got" != "$rev" ]; then
		echo "  $f: garage-revision is '$got'"
		bad=$((bad + 1))
	fi
done <"$work/skills.txt"
if [ "$n" -gt 0 ] && [ "$bad" -eq 0 ]; then pass "every installed skill carries revision $rev"; else fail "every installed skill carries revision $rev"; fi

if (cd "$proj" && "$garage" skills verify --target claude-code </dev/null >"$work/verify.out" 2>&1); then
	pass "garage skills verify reports every skill Current"
else
	cat "$work/verify.out"
	fail "garage skills verify reports every skill Current"
fi

if [ "$failures" -ne 0 ]; then
	echo "$failures check(s) failed"
	exit 1
fi
echo "all archive checks passed for $archive"
