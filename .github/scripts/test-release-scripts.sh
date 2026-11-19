#!/bin/sh
# test-release-scripts.sh: exercise the release workflow's refusals.
#
#   .github/scripts/test-release-scripts.sh
#
# It shows, without running a release, that:
#   - release-guard.sh refuses every ref but a vMAJOR.MINOR.PATCH tag, any
#     unknown mode, and any tag whose commit is not on the released-code
#     branch, in a throwaway repository;
#   - release-inventory.sh selects inventory v1 up to v0.1.x and v2 from
#     v0.2.0, names every file exactly, and refuses a malformed version;
#   - platform-sign.sh never changes a binary, refuses an unset or unknown
#     mode and a missing binary, and refuses a darwin release build unless
#     every macOS signing credential is set (#122);
#   - check-macos-signing-env.sh requires all five macOS signing credentials
#     in release mode only, each certificate and key as one line of base64,
#     and never prints a credential's value;
#   - on macOS only, check-macos-notarization.sh refuses an executable that
#     is not Developer ID signed and notarized.
# CI runs it on every pull request. It runs on macOS and Linux.

set -eu

scripts=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)

work=$(mktemp -d "${TMPDIR:-/tmp}/garage-release-scripts.XXXXXX")
trap 'rm -rf -- "$work"' EXIT

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() {
	printf 'FAIL %s\n' "$1"
	failures=$((failures + 1))
}
accepts() { # accepts NAME COMMAND...
	name=$1
	shift
	if "$@" >"$work/out" 2>&1; then pass "$name"; else
		cat "$work/out"
		fail "$name"
	fi
}
refuses() { # refuses NAME COMMAND...
	name=$1
	shift
	if "$@" >"$work/out" 2>&1; then
		cat "$work/out"
		fail "$name"
	else pass "$name"; fi
}

guard=$scripts/release-guard.sh

# release-guard.sh
# A throwaway repository: main holds v0.0.9 (an earlier commit), v0.1.1,
# v10.20.30, and the annotated v0.4.0; a side branch, standing in for
# staging or a release candidate, holds v0.3.0 and the annotated v0.5.0.
repo=$work/repo
g() {
	git -C "$repo" -c user.name=test -c user.email=test@example.invalid \
		-c commit.gpgsign=false -c tag.gpgsign=false -c core.hooksPath=/dev/null "$@"
}
git init -q "$repo"
g symbolic-ref HEAD refs/heads/main
g commit -q --allow-empty -m first
g tag v0.0.9
g commit -q --allow-empty -m second
g tag v0.1.1
g tag v10.20.30
g tag -a v0.4.0 -m v0.4.0
g switch -q -c side
g commit -q --allow-empty -m "side only"
g tag v0.3.0
g tag -a v0.5.0 -m v0.5.0
g switch -q main
in_repo() { (cd "$repo" && "$@"); }

accepts "guard accepts tag v0.1.1 on main in dry-run mode" in_repo sh "$guard" tag v0.1.1 dry-run main
accepts "guard accepts tag v10.20.30 on main in release mode" in_repo sh "$guard" tag v10.20.30 release main
accepts "guard accepts a tag on an earlier commit of main" in_repo sh "$guard" tag v0.0.9 release main
accepts "guard accepts an annotated tag on main" in_repo sh "$guard" tag v0.4.0 release main
out=$(in_repo sh "$guard" tag v0.1.1 release main)
if [ "$out" = "tag=v0.1.1
version=0.1.1" ]; then pass "guard outputs the tag and version"; else fail "guard outputs the tag and version (got: $out)"; fi
refuses "guard refuses a tag off main in release mode" in_repo sh "$guard" tag v0.3.0 release main
refuses "guard refuses a tag off main in dry-run mode" in_repo sh "$guard" tag v0.3.0 dry-run main
refuses "guard refuses an annotated tag off main" in_repo sh "$guard" tag v0.5.0 release main
if in_repo sh "$guard" tag v0.3.0 release main 2>&1 | grep -F "tag 'v0.3.0' points to commit" | grep -Fq "not on 'main'"; then
	pass "guard names the tag, its commit, and the rule when the tag is off main"
else
	fail "guard names the tag, its commit, and the rule when the tag is off main"
fi
refuses "guard refuses a tag that is not in the repository" in_repo sh "$guard" tag v9.9.9 release main
refuses "guard refuses a released-code ref that is not in the repository" in_repo sh "$guard" tag v0.1.1 release origin/main
refuses "guard refuses an empty released-code ref" in_repo sh "$guard" tag v0.1.1 release ""
outside() { (cd "$work" && GIT_CEILING_DIRECTORIES=$work "$@"); }
refuses "guard refuses outside a Git repository" outside sh "$guard" tag v0.1.1 release main
refuses "guard refuses a missing released-code argument" in_repo sh "$guard" tag v0.1.1 release
refuses "guard refuses the main branch" in_repo sh "$guard" branch main release main
refuses "guard refuses a branch named like a tag" in_repo sh "$guard" branch v0.1.1 dry-run main
refuses "guard refuses a tag without v" in_repo sh "$guard" tag 0.1.1 release main
refuses "guard refuses a pre-release tag" in_repo sh "$guard" tag v0.1.1-rc.1 release main
refuses "guard refuses a tag with leading zeros" in_repo sh "$guard" tag v0.01.1 release main
refuses "guard refuses a partial version" in_repo sh "$guard" tag v0.1 release main
refuses "guard refuses an unknown mode" in_repo sh "$guard" tag v0.1.1 publish main
refuses "guard refuses an empty mode" in_repo sh "$guard" tag v0.1.1 "" main

# release-inventory.sh
inv=$scripts/release-inventory.sh
count() { sh "$inv" "$@" | grep -c .; }
check_eq() { # check_eq NAME GOT WANT
	if [ "$2" = "$3" ]; then pass "$1"; else fail "$1 (got: $2, want: $3)"; fi
}
check_eq "inventory v1 for 0.1.1" "$(sh "$inv" 0.1.1 id)" v1
check_eq "inventory v1 for 0.1.99" "$(sh "$inv" 0.1.99 id)" v1
check_eq "inventory v2 for 0.2.0" "$(sh "$inv" 0.2.0 id)" v2
check_eq "inventory v2 for 1.0.0" "$(sh "$inv" 1.0.0 id)" v2
check_eq "inventory v1 has 8 assets" "$(count 0.1.1)" 8
check_eq "inventory v1 has no packages" "$(sh "$inv" 0.1.1 packages | grep -c . || true)" 0
check_eq "inventory v2 has 12 assets" "$(count 0.2.0)" 12
check_eq "inventory v2 manifest lists 10 files" "$(count 0.2.0 manifest)" 10
check_eq "inventory v2 packages" "$(sh "$inv" 0.2.0 packages | tr '\n' ' ')" \
	"garage-0.2.0-1.aarch64.rpm garage-0.2.0-1.x86_64.rpm garage_0.2.0_linux_amd64.deb garage_0.2.0_linux_arm64.deb "
check_eq "inventory v2 assets include the manifest and bundle" \
	"$(sh "$inv" 0.2.0 all | grep -c '^garage_0.2.0_checksums.txt')" 2
check_eq "a snapshot selects its base version's inventory" "$(sh "$inv" 0.2.0-snapshot+abc1234 id)" v2
check_eq "a snapshot rpm name writes the pre-release with ~" \
	"$(sh "$inv" 0.2.0-snapshot+abc1234 packages | grep x86_64)" "garage-0.2.0~snapshot+abc1234-1.x86_64.rpm"
refuses "inventory refuses a version with v" sh "$inv" v0.2.0
refuses "inventory refuses a partial version" sh "$inv" 0.2
refuses "inventory refuses two pre-release dashes" sh "$inv" 0.2.0-rc-1
refuses "inventory refuses an unknown selection" sh "$inv" 0.2.0 binaries

# install.sh's release mode names the archive with release-inventory.sh's
# rule: the piped installer has no checkout, so it carries its own copy.
installer=$scripts/../../install.sh
if grep -F 'archive=garage_${version}_${os}_${arch}.tar.gz' "$installer" >/dev/null; then
	pass "install.sh names the archive garage_<version>_<os>_<arch>.tar.gz"
else
	fail "install.sh names the archive garage_<version>_<os>_<arch>.tar.gz"
fi
for v in 0.1.1 0.2.1; do
	for os in darwin linux; do
		for arch in amd64 arm64; do
			check_eq "inventory for $v lists install.sh's ${os}/${arch} archive" \
				"$(sh "$inv" "$v" archives | grep -cFx "garage_${v}_${os}_${arch}.tar.gz")" 1
		done
	done
done

# check-macos-signing-env.sh and platform-sign.sh
# Dummy credentials, never real ones: the scripts check presence and shape.
creds=$scripts/check-macos-signing-env.sh
sign=$scripts/platform-sign.sh
p12=$(printf 'not a real p12' | base64 | tr -d '\n')
p8=$(printf 'not a real p8 key' | base64 | tr -d '\n')
with_creds() { # with_creds COMMAND...: run with all five dummy credentials
	MACOS_SIGN_P12=$p12 MACOS_SIGN_PASSWORD=dummy-password MACOS_NOTARY_KEY=$p8 \
		MACOS_NOTARY_KEY_ID=DUMMYKEYID MACOS_NOTARY_ISSUER_ID=dummy-issuer "$@"
}
no_creds() { # no_creds COMMAND...: run with none of them
	env -u MACOS_SIGN_P12 -u MACOS_SIGN_PASSWORD -u MACOS_NOTARY_KEY \
		-u MACOS_NOTARY_KEY_ID -u MACOS_NOTARY_ISSUER_ID "$@"
}
without() { # without NAME COMMAND...: all five but NAME, which is empty
	name=$1
	shift
	with_creds env "$name=" "$@"
}

accepts "credentials not required in snapshot mode" no_creds sh "$creds" snapshot
accepts "credentials not required in dry-run mode" no_creds sh "$creds" dry-run
refuses "credentials required in release mode" no_creds sh "$creds" release
accepts "release mode accepts all five credentials" with_creds sh "$creds" release
for name in MACOS_SIGN_P12 MACOS_SIGN_PASSWORD MACOS_NOTARY_KEY MACOS_NOTARY_KEY_ID MACOS_NOTARY_ISSUER_ID; do
	refuses "release mode refuses an empty $name" without "$name" sh "$creds" release
	if without "$name" sh "$creds" release 2>&1 | grep -Fq "$name"; then
		pass "the refusal names $name"
	else
		fail "the refusal names $name"
	fi
done
refuses "release mode refuses a certificate split over two lines" \
	with_creds env "MACOS_SIGN_P12=$(printf 'not a real p12' | base64 | fold -w 8)" sh "$creds" release
refuses "release mode refuses a key given as a file path" \
	with_creds env "MACOS_NOTARY_KEY=/tmp/AuthKey.p8" sh "$creds" release
refuses "credentials check refuses an unknown mode" with_creds sh "$creds" publish
if with_creds sh "$creds" release 2>&1 | grep -Eq "$p12|dummy-password|$p8|DUMMYKEYID|dummy-issuer"; then
	fail "credentials check prints no credential value"
else
	pass "credentials check prints no credential value"
fi

bin=$work/garage
printf 'binary' >"$bin"
for mode in snapshot dry-run release; do
	for os in linux windows; do
		accepts "hook passes a $os binary in $mode mode" no_creds sh "$sign" "$mode" "$os" "$bin"
	done
done
accepts "hook passes a darwin binary in snapshot mode without credentials" no_creds sh "$sign" snapshot darwin "$bin"
accepts "hook passes a darwin binary in dry-run mode without credentials" no_creds sh "$sign" dry-run darwin "$bin"
refuses "hook refuses a darwin binary in release mode without credentials" no_creds sh "$sign" release darwin "$bin"
refuses "hook refuses a darwin binary in release mode without the notary key" \
	without MACOS_NOTARY_KEY sh "$sign" release darwin "$bin"
accepts "hook passes a darwin binary in release mode with every credential" with_creds sh "$sign" release darwin "$bin"
if [ "$(cat "$bin")" = binary ]; then pass "hook did not modify the binary"; else fail "hook did not modify the binary"; fi
refuses "hook refuses an unset mode" sh "$sign" unset linux "$bin"
refuses "hook refuses an unknown mode" sh "$sign" publish linux "$bin"
refuses "hook refuses a missing binary" sh "$sign" dry-run linux "$work/absent"

# check-macos-notarization.sh, on macOS: an Apple platform binary is signed,
# but neither by a Developer ID Application identity nor notarized.
notary=$scripts/check-macos-notarization.sh
if [ "$(uname -s)" = Darwin ]; then
	mkdir "$work/platform"
	cp /bin/ls "$work/platform/garage"
	tar -czf "$work/platform.tar.gz" -C "$work/platform" garage
	refuses "notarization check refuses an executable that is not Developer ID signed" \
		sh "$notary" "$work/platform.tar.gz" 38397A2FG6
else
	refuses "notarization check refuses to run off macOS" sh "$notary" "$bin" 38397A2FG6
fi

if [ "$failures" -ne 0 ]; then
	echo "$failures check(s) failed"
	exit 1
fi
echo "all release script checks passed"
