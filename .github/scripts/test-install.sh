#!/bin/sh
# test-install.sh: exercise install.sh from this checkout.
#
#   .github/scripts/test-install.sh
#
# Every install goes into a temporary directory, which is removed afterwards.
# It needs Go (the version go.mod declares) and git, and runs on macOS and
# Linux. CI runs it on both.

set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
installer=$root/install.sh

work=$(mktemp -d "${TMPDIR:-/tmp}/garage-install-test.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

# Keep Go's caches where they already are, even when a test changes HOME.
GOCACHE=$(go env GOCACHE)
GOMODCACHE=$(go env GOMODCACHE)
export GOCACHE GOMODCACHE

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() {
	printf 'FAIL %s\n' "$1"
	failures=$((failures + 1))
}
check() { # check NAME COMMAND...
	name=$1
	shift
	if "$@"; then pass "$name"; else fail "$name"; fi
}
contains() { grep -F -- "$2" "$1" >/dev/null; }

status_before=$(git -C "$root" status --porcelain)

# 1. Install into a new directory that is not on PATH.
dir="$work/new dir/bin"
out=$work/first.out
if sh "$installer" --install-dir "$dir" >"$out" 2>&1; then pass "installs into --install-dir"; else
	cat "$out"
	fail "installs into --install-dir"
fi
check "creates the install directory" test -d "$dir"
check "installed garage runs garage version" "$dir/garage" version
check "reports the version" contains "$out" "garage version dev"
check "explains the PATH change" contains "$out" "is not on your PATH"
check "says skills are installed separately" contains "$out" "Skills are installed separately, per project."
check "leaves only garage in the install directory" test "$(ls -A "$dir")" = garage

# 2. Run again: the binary is upgraded in place.
out=$work/second.out
if VERSION=9.9.9 sh "$installer" --install-dir="$dir/" >"$out" 2>&1; then pass "runs again"; else
	cat "$out"
	fail "runs again"
fi
check "reports the replacement" contains "$out" "Replaced the existing $dir/garage"
check "upgrades in place" sh -c '"$1" version | grep -F 9.9.9 >/dev/null' sh "$dir/garage"
check "leaves no staged file behind" test "$(ls -A "$dir")" = garage

# 3. Default directory, already on PATH: no PATH hint.
home=$work/home
mkdir -p "$home/.local/bin"
out=$work/default.out
if HOME=$home PATH="$home/.local/bin:$PATH" sh "$installer" >"$out" 2>&1; then pass "installs into ~/.local/bin by default"; else
	cat "$out"
	fail "installs into ~/.local/bin by default"
fi
check "default install runs" "$home/.local/bin/garage" version
check "no PATH hint when on PATH" sh -c '! grep -F "is not on your PATH" "$1" >/dev/null' sh "$out"

# 4. No go command on PATH: fail clearly, install nothing.
tools=$work/tools
mkdir -p "$tools"
for t in sh sed head dirname mktemp cp mv chmod mkdir rm cat; do
	ln -s "$(command -v "$t")" "$tools/$t"
done
dir=$work/nogo/bin
out=$work/nogo.out
if PATH=$tools sh "$installer" --install-dir "$dir" >"$out" 2>&1; then fail "fails without go"; else pass "fails without go"; fi
check "explains the missing go" contains "$out" "no go command is on PATH"
check "installs nothing without go" test ! -e "$work/nogo"

# 5. An older go: fail clearly, install nothing, never ask Go for a toolchain.
stub=$work/oldgo
mkdir -p "$stub"
cat >"$stub/go" <<'EOF'
#!/bin/sh
if [ "${GOTOOLCHAIN:-}" != local ]; then
	echo "stub go: GOTOOLCHAIN is not local" >&2
	exit 98
fi
if [ "$1 $2" = "env GOVERSION" ]; then
	echo go1.20.1
	exit 0
fi
echo "stub go: unexpected call: $*" >&2
exit 99
EOF
chmod 755 "$stub/go"
dir=$work/oldgo-install/bin
out=$work/oldgo.out
if PATH="$stub:$PATH" sh "$installer" --install-dir "$dir" >"$out" 2>&1; then fail "fails with an older go"; else pass "fails with an older go"; fi
check "explains the older go" contains "$out" "the go command on PATH is Go 1.20.1"
check "installs nothing with an older go" test ! -e "$work/oldgo-install"

# 6. An install directory that is a file: fail, change nothing.
file=$work/not-a-dir
echo keep >"$file"
if sh "$installer" --install-dir "$file" >"$work/file.out" 2>&1; then fail "rejects a file as the install directory"; else pass "rejects a file as the install directory"; fi
check "leaves that file unchanged" test "$(cat "$file")" = keep

# 7. Unknown arguments are rejected.
if sh "$installer" --bogus >/dev/null 2>&1; then fail "rejects unknown arguments"; else pass "rejects unknown arguments"; fi

# 8. The checkout is unchanged.
check "leaves the checkout unchanged" test "$(git -C "$root" status --porcelain)" = "$status_before"

if [ "$failures" -ne 0 ]; then
	printf '\n%d check(s) failed\n' "$failures"
	exit 1
fi
printf '\nall install.sh checks passed\n'
