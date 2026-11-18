#!/bin/sh
# test-install-release.sh: exercise install.sh's release mode (#125).
#
#   .github/scripts/test-install-release.sh
#
# It installs published releases into temporary directories, which are
# removed afterwards, and checks that install.sh installs nothing when a
# release does not verify. It needs network access to github.com, cosign v3
# or later on PATH, curl, tar, and sha256sum or shasum. It runs on macOS and
# Linux; CI runs it on both. test-install.sh covers building from source.

set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
installer=$root/install.sh
repo=bdonaldharris/builder-garage

command -v cosign >/dev/null 2>&1 || {
	echo "test-install-release.sh: cosign is required" >&2
	exit 2
}

work=$(mktemp -d "${TMPDIR:-/tmp}/garage-install-release-test.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

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
installs() { # installs NAME OUT COMMAND...: the command succeeds
	name=$1
	out=$2
	shift 2
	if "$@" >"$out" 2>&1; then pass "$name"; else
		cat "$out"
		fail "$name"
	fi
}
refuses() { # refuses NAME OUT DIR COMMAND...: it fails, and DIR is not created
	name=$1
	out=$2
	dir=$3
	shift 3
	if "$@" >"$out" 2>&1; then
		cat "$out"
		fail "$name"
	else
		pass "$name"
	fi
	check "$name: installs nothing" test ! -e "$dir"
}

case $(uname -s) in Darwin) os=darwin ;; Linux) os=linux ;; *) echo "unsupported OS" >&2 && exit 2 ;; esac
case $(uname -m) in x86_64 | amd64) arch=amd64 ;; arm64 | aarch64) arch=arm64 ;; *) echo "unsupported arch" >&2 && exit 2 ;; esac

status_before=$(git -C "$root" status --porcelain)

# 1. A genuine release, by version.
dir=$work/v021/bin
out=$work/v021.out
installs "installs the published v0.2.1" "$out" sh "$installer" --version v0.2.1 --install-dir "$dir"
check "the installed garage reports v0.2.1" sh -c '"$1" version | grep -Fx "garage version v0.2.1" >/dev/null' sh "$dir/garage"
check "reports the verified signature" contains "$out" "Verified the checksum manifest: signed by https://github.com/$repo/.github/workflows/release.yml@refs/tags/v0.2.1"
check "reports the verified archive" contains "$out" "Verified garage_0.2.1_${os}_${arch}.tar.gz against the manifest"
check "leaves only garage in the install directory" test "$(ls -A "$dir")" = garage
check "says skills are installed separately" contains "$out" "Skills are installed separately, per project."

# 2. The latest release, without --version.
latest=$(curl -fsS -o /dev/null -w '%{redirect_url}' "https://github.com/$repo/releases/latest")
latest=${latest##*/}
dir=$work/latest/bin
installs "installs the latest release ($latest)" "$work/latest.out" sh "$installer" --release --install-dir "$dir"
check "the installed garage reports $latest" sh -c '"$1" version | grep -Fx "garage version $2" >/dev/null' sh "$dir/garage" "$latest"

# 3. Piped through sh from inside the checkout: release mode, not a build.
dir=$work/piped/bin
installs "piped through sh, installs the release" "$work/piped.out" sh -c 'cd "$1" && sh -s -- --version v0.2.1 --install-dir "$2" <"$3"' sh "$root" "$dir" "$installer"
check "piped through sh, does not build" sh -c '! grep -F "Building garage" "$1" >/dev/null' sh "$work/piped.out"
check "piped through sh, reports v0.2.1" sh -c '"$1" version | grep -Fx "garage version v0.2.1" >/dev/null' sh "$dir/garage"

# A local copy of real release files, served through the test-only
# GARAGE_INSTALL_BASE_URL, which never changes the pinned identity.
fetch() { # fetch TAG FILE DEST
	curl -fsSL -o "$3" "https://github.com/$repo/releases/download/$1/$2"
}
serve=$work/serve/download/v0.2.1
mkdir -p "$serve"
for f in "garage_0.2.1_${os}_${arch}.tar.gz" garage_0.2.1_checksums.txt garage_0.2.1_checksums.txt.sigstore.json; do
	fetch v0.2.1 "$f" "$serve/$f"
done
base=file://$work/serve
genuine=$work/genuine
cp -R "$serve" "$genuine"
reset() { rm -rf "$serve" && cp -R "$genuine" "$serve"; }

dir=$work/served/bin
installs "installs genuine files served locally" "$work/served.out" env GARAGE_INSTALL_BASE_URL="$base" sh "$installer" --version v0.2.1 --install-dir "$dir"

# 4. A tampered archive: the checksum does not match the signed manifest.
reset
printf 'tampered' >>"$serve/garage_0.2.1_${os}_${arch}.tar.gz"
dir=$work/tampered-archive/bin
refuses "refuses a tampered archive" "$work/ta.out" "$work/tampered-archive" env GARAGE_INSTALL_BASE_URL="$base" sh "$installer" --version v0.2.1 --install-dir "$dir"
check "explains the archive mismatch" contains "$work/ta.out" "does not match the verified manifest"

# 5. A tampered manifest: its signature no longer verifies.
reset
sed 's/^\(.\)/0\1/' "$genuine/garage_0.2.1_checksums.txt" >"$serve/garage_0.2.1_checksums.txt"
dir=$work/tampered-manifest/bin
refuses "refuses a tampered manifest" "$work/tm.out" "$work/tampered-manifest" env GARAGE_INSTALL_BASE_URL="$base" sh "$installer" --version v0.2.1 --install-dir "$dir"
check "explains the signature failure" contains "$work/tm.out" "is not signed by"

# 6. Another identity: v0.2.0's genuinely signed manifest, offered as v0.2.1.
reset
fetch v0.2.0 garage_0.2.0_checksums.txt "$serve/garage_0.2.1_checksums.txt"
fetch v0.2.0 garage_0.2.0_checksums.txt.sigstore.json "$serve/garage_0.2.1_checksums.txt.sigstore.json"
dir=$work/other-identity/bin
refuses "refuses a manifest signed for another tag" "$work/oi.out" "$work/other-identity" env GARAGE_INSTALL_BASE_URL="$base" sh "$installer" --version v0.2.1 --install-dir "$dir"
check "explains the identity" contains "$work/oi.out" "is not signed by https://github.com/$repo/.github/workflows/release.yml@refs/tags/v0.2.1"
check "cosign rejects the v0.2.0 identity" contains "$work/oi.out" "got \"https://github.com/$repo/.github/workflows/release.yml@refs/tags/v0.2.0\""

# 7. No cosign on PATH: refuse, never fall back to checksums alone.
tools=$work/tools
mkdir -p "$tools"
for t in sh sed head cut awk grep dirname mktemp cp mv chmod mkdir rm cat ls env curl tar uname sha256sum shasum; do
	p=$(command -v "$t" 2>/dev/null) || continue
	ln -s "$p" "$tools/$t"
done
dir=$work/nocosign/bin
refuses "refuses without cosign" "$work/nc.out" "$work/nocosign" env PATH="$tools" sh "$installer" --version v0.2.1 --install-dir "$dir"
check "explains the missing cosign" contains "$work/nc.out" "cosign v3 or later is required"

# 8. A malformed version, and a release that does not exist.
dir=$work/badversion/bin
refuses "refuses a malformed --version" "$work/bv.out" "$work/badversion" sh "$installer" --version 0.2.1 --install-dir "$dir"
check "explains the version format" contains "$work/bv.out" "vMAJOR.MINOR.PATCH"
dir=$work/noversion/bin
refuses "refuses a release that does not exist" "$work/nv.out" "$work/noversion" sh "$installer" --version v0.0.99 --install-dir "$dir"
check "explains the missing release" contains "$work/nv.out" "cannot download"

# 9. The checkout is unchanged.
check "leaves the checkout unchanged" test "$(git -C "$root" status --porcelain)" = "$status_before"

if [ "$failures" -ne 0 ]; then
	printf '\n%d check(s) failed\n' "$failures"
	exit 1
fi
printf '\nall install.sh release checks passed\n'
