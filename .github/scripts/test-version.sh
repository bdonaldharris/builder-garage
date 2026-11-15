#!/bin/sh
# test-version.sh: exercise the version that "make build" and install.sh derive
# from Git (docs/adr/0002-semantic-versioning.md).
#
#   .github/scripts/test-version.sh
#
# Each case runs in a temporary Git repository built from this checkout's
# current files, so no tag is ever created in this repository. It needs Go
# (the version go.mod declares), git, and make, and runs on macOS and Linux.
# CI runs it on both.

set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)

work=$(mktemp -d "${TMPDIR:-/tmp}/garage-version-test.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

# Keep Go's caches where they already are.
GOCACHE=$(go env GOCACHE)
GOMODCACHE=$(go env GOMODCACHE)
export GOCACHE GOMODCACHE

# Commits in the temporary repositories need an identity, and must not depend
# on the user's Git configuration.
GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.invalid
GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.invalid
export GIT_AUTHOR_NAME GIT_AUTHOR_EMAIL GIT_COMMITTER_NAME GIT_COMMITTER_EMAIL
unset VERSION || true

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() {
	printf 'FAIL %s\n' "$1"
	failures=$((failures + 1))
}
expect() { # expect NAME GOT WANT
	if [ "$2" = "$3" ]; then pass "$1"; else fail "$1: got '$2', want '$3'"; fi
}
expect_match() { # expect_match NAME GOT PATTERN (a case pattern)
	# shellcheck disable=SC2254
	case $2 in
	$3) pass "$1" ;;
	*) fail "$1: got '$2', want a match for '$3'" ;;
	esac
}

# copy_source DIR: copy this checkout's current files, without .git, into DIR.
copy_source() {
	mkdir -p "$1"
	(cd "$root" && git ls-files -z --cached --others --exclude-standard | xargs -0 tar -cf -) | (cd "$1" && tar -xf -)
}

# new_repo DIR: a repository holding this checkout's current files, in one commit.
new_repo() {
	copy_source "$1"
	git -C "$1" init -q
	git -C "$1" add -A
	git -C "$1" commit -q -m "test source"
}

make_version() { make -s -C "$1" version; }

# install_version DIR: the version install.sh reports when run from DIR.
install_version() {
	sh "$1/install.sh" --install-dir "$work/bin" >"$work/install.out" 2>&1 || {
		cat "$work/install.out" >&2
		echo "install.sh failed"
		return 0
	}
	"$work/bin/garage" version | sed 's/^garage version //'
}

# 1. No version tag reachable: the abbreviated commit.
repo=$work/repo
new_repo "$repo"
short=$(git -C "$repo" rev-parse --short HEAD)
expect "no tag: make reports the abbreviated commit" "$(make_version "$repo")" "$short"

# 2. A tag that is not a version tag is ignored.
git -C "$repo" tag not-a-version
expect "non-version tag is ignored" "$(make_version "$repo")" "$short"

# 3. Exactly at a version tag, clean: the tag. install.sh agrees.
git -C "$repo" tag v0.1.0
expect "tagged: make reports the tag" "$(make_version "$repo")" v0.1.0
expect "tagged: install.sh reports the tag" "$(install_version "$repo")" v0.1.0
expect "tagged: the checkout stays clean" "$(git -C "$repo" status --porcelain)" ""

# 4. An explicit VERSION overrides derivation, in both.
expect "override: make" "$(make -s -C "$repo" version VERSION=1.2.3)" 1.2.3
expect "override: install.sh" "$(VERSION=1.2.3 install_version "$repo")" 1.2.3

# 5. A commit after the tag: Git's descriptive form.
echo "after the tag" >>"$repo/README.md"
git -C "$repo" commit -q -a -m "after the tag"
after=v0.1.0-1-g$(git -C "$repo" rev-parse --short HEAD)
expect "after the tag: make" "$(make_version "$repo")" "$after"

# 6. A modified tracked file: marked -dirty, in both.
echo "local change" >>"$repo/README.md"
expect "modified: make" "$(make_version "$repo")" "$after-dirty"
expect "modified: install.sh" "$(install_version "$repo")" "$after-dirty"
git -C "$repo" checkout -q -- README.md

# 7. An untracked file also counts as a local change.
echo "untracked" >"$repo/untracked.txt"
expect "untracked: make" "$(make_version "$repo")" "$after-dirty"
rm "$repo/untracked.txt"
expect "clean again: make" "$(make_version "$repo")" "$after"

# 8. Deriving the version leaves the index unrewritten.
touch "$repo/README.md"
index=$(git -C "$repo" rev-parse --git-path index)
case $index in /*) ;; *) index=$repo/$index ;; esac
before=$(ls -l "$index")
sleep 1
make_version "$repo" >/dev/null
expect "derivation leaves the index unchanged" "$(ls -l "$index")" "$before"

# 9. No Git metadata, inside an enclosing repository: dev, in both.
outer=$work/outer
git init -q "$outer"
copy_source "$outer/archive"
expect "no Git metadata: make reports dev" "$(make_version "$outer/archive")" dev
expect "no Git metadata: install.sh reports dev" "$(install_version "$outer/archive")" dev

# 10. No git command: dev. make runs simple recipes without a shell, so the
# restricted PATH keeps echo.
tools=$work/tools
mkdir -p "$tools"
for t in sh make echo; do
	# "command -v" names a builtin, such as echo, without a path; link the
	# executable instead.
	p=$(command -v "$t")
	case $p in
	/*) ;;
	*) for d in /bin /usr/bin; do
		if [ -x "$d/$t" ]; then
			p=$d/$t
			break
		fi
	done ;;
	esac
	ln -s "$p" "$tools/$t"
done
expect "no git command: make reports dev" "$(PATH=$tools make_version "$repo")" dev

if [ "$failures" -ne 0 ]; then
	printf '\n%d check(s) failed\n' "$failures"
	exit 1
fi
printf '\nall version checks passed\n'
