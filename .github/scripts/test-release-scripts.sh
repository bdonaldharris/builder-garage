#!/bin/sh
# test-release-scripts.sh: exercise the release workflow's refusals.
#
#   .github/scripts/test-release-scripts.sh
#
# It shows, without running a release, that:
#   - release-guard.sh refuses every ref but a vMAJOR.MINOR.PATCH tag, any
#     unknown mode, and any tag whose commit is not on the released-code
#     branch, in a throwaway repository;
#   - release-inventory.sh selects inventory v1 up to v0.1.x, v2 from v0.2.0,
#     and v3 from v0.5.0, names every file exactly, leaves the macOS
#     installer packages out of its goreleaser selection, and refuses a
#     malformed version;
#   - check-release-dist.sh requires the macOS installer packages by default,
#     and only the files GoReleaser builds with its goreleaser set, and
#     refuses an unknown set (#179);
#   - check-channel-cask.sh requires both Linux and macOS cask stanzas, and
#     pins exactly their release archives to the checksum manifest (#124);
#   - macos-installer-identity.sh selects only a valid Developer ID Installer
#     identity for Team ID 38397A2FG6, refuses any other, and names a matching
#     identity that is present but not valid; build-macos-package.sh looks it
#     up with a find-identity policy the tool accepts; on macOS only, against
#     a throwaway keychain, find-identity accepts that invocation (#182);
#   - platform-sign.sh never changes a binary, refuses an unset or unknown
#     mode and a missing binary, and refuses a darwin release build unless
#     every macOS signing credential is set (#122);
#   - check-macos-signing-env.sh requires all five macOS signing credentials
#     in release mode only, each certificate and key as one line of base64,
#     and never prints a credential's value;
#   - on macOS only, check-macos-notarization.sh refuses an executable that
#     is not Developer ID signed and notarized;
#   - npm-packages.sh writes the launcher and the macOS and Linux platform
#     packages, pinned to one version, with no install scripts and no
#     Windows package, and refuses a missing or mismatched archive or a
#     malformed version; npm-dist-tag.sh never moves "latest" backward and
#     refuses prereleases; npm-publish.sh refuses bad arguments; and the npm
#     launcher runs its platform's executable, passing arguments and exit
#     status through, or says npm does not provide garage (#158). The bounded
#     registry wait succeeds only after every named package is served and
#     fails without publishing when one never appears. npm-publish.sh and
#     check-npm-install.sh --registry give the wait step each of the
#     launcher's platform packages as a separate, valid name, and
#     npm-publish.sh publishes the launcher after them (#186). An already-
#     published platform package is skipped when it matches, and publication
#     continues to the launcher; one that differs fails the run, and no
#     launcher is published (#189). The launcher and package-name checks need
#     node, and are skipped without it.
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
check_eq "inventory v3 for 1.0.0" "$(sh "$inv" 1.0.0 id)" v3
check_eq "inventory v2 for 0.4.9" "$(sh "$inv" 0.4.9 id)" v2
check_eq "inventory v3 for 0.5.0" "$(sh "$inv" 0.5.0 id)" v3
check_eq "inventory v1 has 8 assets" "$(count 0.1.1)" 8
check_eq "inventory v1 has no packages" "$(sh "$inv" 0.1.1 packages | grep -c . || true)" 0
check_eq "inventory v2 has 12 assets" "$(count 0.2.0)" 12
check_eq "inventory v2 manifest lists 10 files" "$(count 0.2.0 manifest)" 10
check_eq "inventory v2 packages" "$(sh "$inv" 0.2.0 packages | tr '\n' ' ')" \
	"garage-0.2.0-1.aarch64.rpm garage-0.2.0-1.x86_64.rpm garage_0.2.0_linux_amd64.deb garage_0.2.0_linux_arm64.deb "
check_eq "inventory v2 assets include the manifest and bundle" \
	"$(sh "$inv" 0.2.0 all | grep -c '^garage_0.2.0_checksums.txt')" 2
check_eq "inventory v3 has 14 assets" "$(count 0.5.0)" 14
check_eq "inventory v3 manifest lists 12 files" "$(count 0.5.0 manifest)" 12
check_eq "inventory v3 packages include macOS installers" \
	"$(sh "$inv" 0.5.0 packages | grep -c '^garage_0.5.0_darwin_.*\.pkg$')" 2
check_eq "inventory v3 goreleaser selection lists 10 files" "$(count 0.5.0 goreleaser)" 10
check_eq "inventory v3 goreleaser selection has no macOS installer" \
	"$(sh "$inv" 0.5.0 goreleaser | grep -c '\.pkg$' || true)" 0
check_eq "inventory v3 goreleaser selection is the manifest without the installers" \
	"$(sh "$inv" 0.5.0 goreleaser | tr '\n' ' ')" "$(sh "$inv" 0.5.0 manifest | grep -v '\.pkg$' | tr '\n' ' ')"
check_eq "inventory v2 goreleaser selection equals its manifest" \
	"$(sh "$inv" 0.2.0 goreleaser | tr '\n' ' ')" "$(sh "$inv" 0.2.0 manifest | tr '\n' ' ')"
check_eq "inventory v1 goreleaser selection equals its manifest" \
	"$(sh "$inv" 0.1.1 goreleaser | tr '\n' ' ')" "$(sh "$inv" 0.1.1 manifest | tr '\n' ' ')"
check_eq "a snapshot selects its base version's inventory" "$(sh "$inv" 0.2.0-snapshot+abc1234 id)" v2
check_eq "a snapshot rpm name writes the pre-release with ~" \
	"$(sh "$inv" 0.2.0-snapshot+abc1234 packages | grep x86_64)" "garage-0.2.0~snapshot+abc1234-1.x86_64.rpm"
refuses "inventory refuses a version with v" sh "$inv" v0.2.0
refuses "inventory refuses a partial version" sh "$inv" 0.2
refuses "inventory refuses two pre-release dashes" sh "$inv" 0.2.0-rc-1
refuses "inventory refuses an unknown selection" sh "$inv" 0.2.0 binaries

# check-channel-cask.sh
check_cask=$scripts/check-channel-cask.sh
cask_version=0.5.0
cask_repo=example/garage
cask_sha=$(printf '%064d' 0)
cask_manifest=$work/cask-checksums.txt
for name in $(sh "$inv" "$cask_version" archives | grep -E '_(linux|darwin)_'); do
	printf '%s *%s\n' "$cask_sha" "$name" >>"$cask_manifest"
done
cask=$work/garage.rb
cat >"$cask" <<EOF
cask "garage" do
  version "$cask_version"
  on_macos do
    on_arm do
      sha256 "$cask_sha"
      url "https://github.com/$cask_repo/releases/download/v#{version}/garage_#{version}_darwin_arm64.tar.gz"
    end
    on_intel do
      sha256 "$cask_sha"
      url "https://github.com/$cask_repo/releases/download/v#{version}/garage_#{version}_darwin_amd64.tar.gz"
    end
  end
  on_linux do
    on_arm do
      sha256 "$cask_sha"
      url "https://github.com/$cask_repo/releases/download/v#{version}/garage_#{version}_linux_arm64.tar.gz"
    end
    on_intel do
      sha256 "$cask_sha"
      url "https://github.com/$cask_repo/releases/download/v#{version}/garage_#{version}_linux_amd64.tar.gz"
    end
  end
  binary "garage"
end
EOF
accepts "cask pins Linux and macOS archives to the manifest" \
	sh "$check_cask" "$cask" "$cask_manifest" "$cask_version" "$cask_repo"
no_macos=$work/no-macos.rb
sed '/^  on_macos do$/d' "$cask" >"$no_macos"
refuses "cask refuses a missing macOS stanza" \
	sh "$check_cask" "$no_macos" "$cask_manifest" "$cask_version" "$cask_repo"
bad_cask_manifest=$work/cask-checksums-bad.txt
sed "s/^$cask_sha \\*garage_${cask_version}_darwin_arm64.tar.gz/$(printf '%064d' 1) *garage_${cask_version}_darwin_arm64.tar.gz/" \
	"$cask_manifest" >"$bad_cask_manifest"
refuses "cask refuses a checksum that differs from the manifest" \
	sh "$check_cask" "$cask" "$bad_cask_manifest" "$cask_version" "$cask_repo"
extra_cask=$work/extra-cask.rb
cp "$cask" "$extra_cask"
printf '  sha256 "%s"\n  url "https://github.com/%s/releases/download/v%s/garage_%s_windows_amd64.zip"\n' \
	"$cask_sha" "$cask_repo" "$cask_version" "$cask_version" >>"$extra_cask"
refuses "cask refuses an unexpected Windows download" \
	sh "$check_cask" "$extra_cask" "$cask_manifest" "$cask_version" "$cask_repo"

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

# The Installer certificate is deliberately separate from the Application
# certificate. Its preflight shares only the existing notarization values.
installer_creds=$scripts/check-macos-installer-env.sh
with_installer_creds() {
	MACOS_INSTALLER_P12=$p12 MACOS_INSTALLER_PASSWORD=dummy-installer-password MACOS_NOTARY_KEY=$p8 \
		MACOS_NOTARY_KEY_ID=DUMMYKEYID MACOS_NOTARY_ISSUER_ID=dummy-issuer "$@"
}
accepts "Installer credentials are not required in snapshot mode" sh "$installer_creds" snapshot
refuses "Installer credentials are required in release mode" sh "$installer_creds" release
accepts "Installer preflight accepts dedicated credentials" with_installer_creds sh "$installer_creds" release
refuses "Installer preflight refuses an Application certificate in place of an Installer certificate" \
	env MACOS_SIGN_P12=$p12 MACOS_SIGN_PASSWORD=dummy-password MACOS_NOTARY_KEY=$p8 MACOS_NOTARY_KEY_ID=DUMMYKEYID MACOS_NOTARY_ISSUER_ID=dummy-issuer sh "$installer_creds" release

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

# check-release-dist.sh: which inventory files a dist must hold (#179).
# A fake GoReleaser dist for inventory v3: the six archives, each holding a
# real (tiny) Go executable, the four Linux packages, and their manifest, but
# no macOS installer packages, which GoReleaser never builds. Only the
# inventory checks are asserted; the executables' build settings fail by
# design, so the script's overall status is not.
distchk=$scripts/check-release-dist.sh
refuses "dist check refuses an unknown set" sh "$distchk" "$work" 0.5.0 0.5.0 HEAD archives
refuses "dist check refuses a sixth argument" sh "$distchk" "$work" 0.5.0 0.5.0 HEAD goreleaser extra
if command -v sha256sum >/dev/null 2>&1; then
	dsha() { sha256sum "$1" | cut -d' ' -f1; }
else
	dsha() { shasum -a 256 "$1" | cut -d' ' -f1; }
fi
mkzip() { # mkzip ZIP DIR FILE...: a zip of FILEs in DIR, with zip or python3
	z=$1
	d=$2
	shift 2
	if command -v zip >/dev/null 2>&1; then
		(cd "$d" && zip -q "$z" "$@")
	else
		(cd "$d" && python3 -I -m zipfile -c "$z" "$@")
	fi
}
if command -v go >/dev/null 2>&1 && { command -v zip >/dev/null 2>&1 || command -v python3 >/dev/null 2>&1; }; then
	gd=$work/distchk
	v=0.5.0-snapshot+abc1234
	mkdir -p "$gd/mod" "$gd/src" "$gd/dist"
	printf 'module example.invalid/garage\n\ngo 1.21\n' >"$gd/mod/go.mod"
	printf 'package main\n\nfunc main() {}\n' >"$gd/mod/main.go"
	(cd "$gd/mod" && CGO_ENABLED=0 GOFLAGS=-buildvcs=false go build -trimpath -o "$gd/src/garage" .)
	cp "$gd/src/garage" "$gd/src/garage.exe"
	printf 'MIT\n' >"$gd/src/LICENSE"
	printf 'readme\n' >"$gd/src/README.md"
	for name in $(sh "$inv" "$v" archives); do
		case $name in
		*.zip) mkzip "$gd/dist/$name" "$gd/src" garage.exe LICENSE README.md ;;
		*) tar -czf "$gd/dist/$name" -C "$gd/src" garage LICENSE README.md ;;
		esac
	done
	for name in $(sh "$inv" "$v" packages | grep -v '\.pkg$'); do printf 'pkg\n' >"$gd/dist/$name"; done
	manifest() { # manifest: GoReleaser's checksum manifest for the files in dist
		: >"$gd/dist/garage_${v}_checksums.txt"
		for f in $(cd "$gd/dist" && ls -1 | grep -E '\.(tar\.gz|zip|deb|rpm|pkg)$' | LC_ALL=C sort); do
			printf '%s  %s\n' "$(dsha "$gd/dist/$f")" "$f" >>"$gd/dist/garage_${v}_checksums.txt"
		done
	}
	manifest
	distrun() { sh "$distchk" "$gd/dist" "$v" "$v" HEAD "$@" 2>&1 || true; }
	has() { # has NAME LINE OUTPUT: OUTPUT contains LINE exactly
		if printf '%s\n' "$3" | grep -Fxq -- "$2"; then pass "$1"; else
			printf '%s\n' "$3"
			fail "$1"
		fi
	}
	out=$(distrun goreleaser)
	has "v3 GoReleaser dist holds exactly the goreleaser set" "ok   dist holds exactly the inventory v3 archives and packages" "$out"
	has "v3 GoReleaser manifest lists exactly the goreleaser set" "ok   manifest lists exactly the inventory v3 files" "$out"
	has "v3 GoReleaser dist matches its manifest" "ok   manifest matches the files" "$out"
	has "v3 goreleaser set still requires each Linux package" "ok   package garage_${v}_linux_amd64.deb exists" "$out"
	if printf '%s\n' "$out" | grep -q '\.pkg'; then
		printf '%s\n' "$out"
		fail "v3 goreleaser set does not require the macOS installer packages"
	else pass "v3 goreleaser set does not require the macOS installer packages"; fi
	out=$(distrun)
	has "the default set still requires the macOS installer packages" "FAIL package garage_${v}_darwin_arm64.pkg exists" "$out"
	has "the default set refuses a dist without them" "FAIL dist holds exactly the inventory v3 archives and packages (got: $(cd "$gd/dist" && ls -1 | grep -E '\.(tar\.gz|zip|deb|rpm|pkg)$' | LC_ALL=C sort | tr '\n' ' '))" "$out"
	out=$(distrun manifest)
	has "set manifest is the default" "FAIL package garage_${v}_darwin_amd64.pkg exists" "$out"
	rpm=$(sh "$inv" "$v" packages | grep x86_64)
	mv "$gd/dist/$rpm" "$gd/rpm"
	manifest
	out=$(distrun goreleaser)
	has "the goreleaser set fails without a Linux package" "FAIL package $rpm exists" "$out"
	mv "$gd/rpm" "$gd/dist/$rpm"
	printf 'pkg\n' >"$gd/dist/garage_${v}_darwin_amd64.pkg"
	manifest
	out=$(distrun goreleaser)
	if printf '%s\n' "$out" | grep -Fq "FAIL dist holds exactly the inventory v3 archives and packages"; then
		pass "the goreleaser set fails with an extra package"
	else
		printf '%s\n' "$out"
		fail "the goreleaser set fails with an extra package"
	fi
	rm "$gd/dist/garage_${v}_darwin_amd64.pkg"
	v=0.2.0-snapshot+abc1234
	rm -rf "$gd/dist"
	mkdir "$gd/dist"
	for name in $(sh "$inv" "$v" archives); do
		case $name in
		*.zip) mkzip "$gd/dist/$name" "$gd/src" garage.exe LICENSE README.md ;;
		*) tar -czf "$gd/dist/$name" -C "$gd/src" garage LICENSE README.md ;;
		esac
	done
	for name in $(sh "$inv" "$v" packages); do printf 'pkg\n' >"$gd/dist/$name"; done
	manifest
	has "v2: the default set accepts a GoReleaser dist" "ok   dist holds exactly the inventory v2 archives and packages" "$(distrun)"
	has "v2: the goreleaser set accepts the same dist" "ok   dist holds exactly the inventory v2 archives and packages" "$(distrun goreleaser)"
else
	echo "skip the dist inventory checks: go, and zip or python3, are required"
fi

# macos-installer-identity.sh: the Developer ID Installer identity that
# build-macos-package.sh signs with, from "security find-identity" listings
# (#182). The listings are fixtures in find-identity's own format.
ident=$scripts/macos-installer-identity.sh
id_dir=$work/identity
mkdir -p "$id_dir"
printf '  1) 1111111111111111111111111111111111111111 "Developer ID Application: Example (38397A2FG6)"\n  2) 2222222222222222222222222222222222222222 "Developer ID Installer: Example (38397A2FG6)"\n     2 valid identities found\n' >"$id_dir/valid"
printf '     0 valid identities found\n' >"$id_dir/none"
printf '  1) 3333333333333333333333333333333333333333 "Developer ID Installer: Other (ABCDE12345)"\n     1 valid identities found\n' >"$id_dir/other-team"
printf '  1) 1111111111111111111111111111111111111111 "Developer ID Application: Example (38397A2FG6)"\n     1 valid identities found\n' >"$id_dir/application-only"
printf '\nPolicy: X.509 Basic\n  Matching identities\n  1) 4444444444444444444444444444444444444444 "Developer ID Installer: Example (38397A2FG6)" (CSSMERR_TP_NOT_TRUSTED)\n     1 identities found\n\n  Valid identities only\n     0 valid identities found\n' >"$id_dir/untrusted-all"
check_eq "identity: selects the valid Developer ID Installer identity" \
	"$(sh "$ident" "$id_dir/valid")" "Developer ID Installer: Example (38397A2FG6)"
refuses "identity: refuses a keychain with no valid identity" sh "$ident" "$id_dir/none"
refuses "identity: refuses another team's installer identity" sh "$ident" "$id_dir/other-team"
refuses "identity: refuses a Developer ID Application identity" sh "$ident" "$id_dir/application-only"
refuses "identity: refuses an untrusted identity" sh "$ident" "$id_dir/none" "$id_dir/untrusted-all"
refuses "identity: refuses a missing listing" sh "$ident" "$id_dir/absent"
refuses "identity: refuses no arguments" sh "$ident"
if sh "$ident" "$id_dir/none" 2>&1 | grep -Fxq "macos-package: no Developer ID Installer identity for Team ID 38397A2FG6"; then
	pass "identity: the refusal says no Developer ID Installer identity exists"
else
	fail "identity: the refusal says no Developer ID Installer identity exists"
fi
if sh "$ident" "$id_dir/none" "$id_dir/untrusted-all" 2>&1 | grep -Fq "'Developer ID Installer: Example (38397A2FG6)' is in the keychain but is not a valid identity"; then
	pass "identity: the refusal names a matching identity that is present but not valid"
else
	fail "identity: the refusal names a matching identity that is present but not valid"
fi
if sh "$ident" "$id_dir/none" "$id_dir/none" 2>&1 | grep -Fq "is in the keychain"; then
	fail "identity: no present-but-not-valid note without a matching identity"
else
	pass "identity: no present-but-not-valid note without a matching identity"
fi
# The release-mode lookup uses a policy find-identity accepts, and reports
# the lookup's own failure apart from finding no identity.
pkgbuilder=$scripts/build-macos-package.sh
if grep -Fq 'find-identity -v -p installer' "$pkgbuilder"; then
	fail "build-macos-package does not use the unsupported installer policy"
else
	pass "build-macos-package does not use the unsupported installer policy"
fi
if grep -Fq 'security find-identity -v -p basic "$keychain"' "$pkgbuilder" &&
	grep -Fq 'macos-package: security find-identity failed (status $st)' "$pkgbuilder" &&
	grep -Fq 'sh "$scripts/macos-installer-identity.sh"' "$pkgbuilder"; then
	pass "build-macos-package looks the identity up with -v -p basic and the selection helper"
else
	fail "build-macos-package looks the identity up with -v -p basic and the selection helper"
fi
# On macOS: the invocation against a throwaway keychain, never a real one.
if [ "$(uname -s)" = Darwin ] && command -v security >/dev/null 2>&1; then
	kc=$id_dir/test.keychain-db
	security create-keychain -p test-password "$kc" >/dev/null 2>&1
	accepts "identity: find-identity accepts -v -p basic" security find-identity -v -p basic "$kc"
	refuses "identity: find-identity rejects the installer policy" security find-identity -v -p installer "$kc"
	security find-identity -v -p basic "$kc" >"$id_dir/kc-valid"
	security find-identity -p basic "$kc" >"$id_dir/kc-all"
	refuses "identity: an empty keychain has no installer identity" sh "$ident" "$id_dir/kc-valid" "$id_dir/kc-all"
	if command -v openssl >/dev/null 2>&1; then
		# A self-signed identity named like the real one is present, but not
		# valid: it is refused, and named.
		openssl req -x509 -newkey rsa:2048 -nodes -keyout "$id_dir/key.pem" -out "$id_dir/cert.pem" -days 2 \
			-subj "/CN=Developer ID Installer: Test (38397A2FG6)/OU=38397A2FG6" >/dev/null 2>&1
		openssl pkcs12 -export -legacy -inkey "$id_dir/key.pem" -in "$id_dir/cert.pem" -out "$id_dir/test.p12" -passout pass:test >/dev/null 2>&1 ||
			openssl pkcs12 -export -inkey "$id_dir/key.pem" -in "$id_dir/cert.pem" -out "$id_dir/test.p12" -passout pass:test >/dev/null 2>&1
		security unlock-keychain -p test-password "$kc" >/dev/null 2>&1
		security import "$id_dir/test.p12" -k "$kc" -P test >/dev/null 2>&1
		security find-identity -v -p basic "$kc" >"$id_dir/kc-valid"
		security find-identity -p basic "$kc" >"$id_dir/kc-all"
		refuses "identity: a self-signed installer identity is not valid" sh "$ident" "$id_dir/kc-valid" "$id_dir/kc-all"
		if sh "$ident" "$id_dir/kc-valid" "$id_dir/kc-all" 2>&1 | grep -Fq "'Developer ID Installer: Test (38397A2FG6)' is in the keychain but is not a valid identity"; then
			pass "identity: the self-signed identity is named as present but not valid"
		else
			fail "identity: the self-signed identity is named as present but not valid"
		fi
	else
		echo "skip the self-signed identity check: openssl is not installed"
	fi
	security delete-keychain "$kc" >/dev/null 2>&1 || true
else
	echo "skip the find-identity invocation checks: not macOS"
fi

# npm-packages.sh, npm-dist-tag.sh, npm-publish.sh, and the npm launcher
npmpk=$scripts/npm-packages.sh
disttag=$scripts/npm-dist-tag.sh
npmpub=$scripts/npm-publish.sh
npmwait=$scripts/npm-wait-for-packages.sh
npmcheck=$scripts/check-npm-install.sh
if command -v sha256sum >/dev/null 2>&1; then
	sha() { sha256sum "$1" | cut -d' ' -f1; }
else
	sha() { shasum -a 256 "$1" | cut -d' ' -f1; }
fi
# Fake archives for every target, and their manifest.
arch=$work/npm-archives
mkdir -p "$arch/src"
printf 'MIT\n' >"$arch/src/LICENSE"
printf 'readme\n' >"$arch/src/README.md"
for t in darwin_arm64 darwin_amd64 linux_arm64 linux_amd64 windows_arm64 windows_amd64; do
	printf '#!/bin/sh\necho %s\n' "$t" >"$arch/src/garage"
	tar -czf "$arch/garage_9.8.7_$t.tar.gz" -C "$arch/src" garage LICENSE README.md
	printf '%s  %s\n' "$(sha "$arch/garage_9.8.7_$t.tar.gz")" "garage_9.8.7_$t.tar.gz" >>"$arch/garage_9.8.7_checksums.txt"
done
accepts "npm-packages writes the packages" sh "$npmpk" 9.8.7 "$arch" "$work/npm"
check_eq "npm-packages writes the launcher and four platform packages" "$(ls "$work/npm" | tr '\n' ' ')" \
	"garage garage-darwin-arm64 garage-darwin-x64 garage-linux-arm64 garage-linux-x64 "
check_eq "the linux-x64 package holds the linux_amd64 executable" "$(sh "$work/npm/garage-linux-x64/bin/garage")" linux_amd64
check_eq "the darwin-arm64 package holds the darwin_arm64 executable" "$(sh "$work/npm/garage-darwin-arm64/bin/garage")" darwin_arm64
if [ -x "$work/npm/garage-darwin-x64/bin/garage" ]; then pass "platform executables are executable"; else fail "platform executables are executable"; fi
if grep -q '"os": \["linux"\]' "$work/npm/garage-linux-arm64/package.json" && grep -q '"cpu": \["arm64"\]' "$work/npm/garage-linux-arm64/package.json"; then
	pass "platform packages declare npm's os and cpu"
else
	fail "platform packages declare npm's os and cpu"
fi
check_eq "the launcher pins every platform package to its version" \
	"$(grep -c '"@buildergarage/garage-[a-z0-9-]*": "9.8.7"' "$work/npm/garage/package.json")" 4
if grep -rq '"scripts"\|install"' "$work/npm"/*/package.json; then fail "no package has an install script"; else pass "no package has an install script"; fi
if grep -q win32 "$work/npm/garage/package.json"; then fail "no Windows package is named"; else pass "no Windows package is named"; fi
if grep -q '"url": "git+https://github.com/bdonaldharris/builder-garage.git"' "$work/npm/garage/package.json"; then
	pass "packages name this repository, as provenance requires"
else
	fail "packages name this repository, as provenance requires"
fi
refuses "npm-packages refuses an existing output directory" sh "$npmpk" 9.8.7 "$arch" "$work/npm"
refuses "npm-packages refuses a malformed version" sh "$npmpk" 9.8 "$arch" "$work/npm-bad-version"
mkdir -p "$work/npm-missing"
cp "$arch"/garage_9.8.7_checksums.txt "$arch"/garage_9.8.7_darwin_*.tar.gz "$work/npm-missing/"
refuses "npm-packages refuses a missing archive" sh "$npmpk" 9.8.7 "$work/npm-missing" "$work/npm-out-missing"
mkdir -p "$work/npm-tampered"
cp "$arch"/garage_9.8.7_* "$work/npm-tampered/"
printf 'x' >>"$work/npm-tampered/garage_9.8.7_linux_amd64.tar.gz"
refuses "npm-packages refuses an archive that does not match the manifest" sh "$npmpk" 9.8.7 "$work/npm-tampered" "$work/npm-out-tampered"
if [ ! -e "$work/npm-out-tampered" ]; then pass "npm-packages writes nothing when an archive is refused"; else fail "npm-packages writes nothing when an archive is refused"; fi

check_eq "dist-tag for a first publish is latest" "$(sh "$disttag" 0.4.0)" latest
check_eq "dist-tag for a newer version is latest" "$(sh "$disttag" 0.10.0 0.9.0)" latest
check_eq "dist-tag for the same version is latest" "$(sh "$disttag" 0.4.0 0.4.0)" latest
check_eq "dist-tag for an older version keeps latest" "$(sh "$disttag" 0.2.4 0.3.0)" release-0.2
check_eq "dist-tag compares numerically" "$(sh "$disttag" 0.9.0 0.10.0)" release-0.9
refuses "dist-tag refuses a prerelease" sh "$disttag" 0.4.0-rc.1
refuses "dist-tag refuses a malformed current latest" sh "$disttag" 0.4.0 latest

refuses "npm-publish refuses a missing packages directory" sh "$npmpub" "$work/absent" latest
refuses "npm-publish refuses an empty dist-tag" sh "$npmpub" "$work/npm" ""
refuses "npm-publish refuses an unknown option" sh "$npmpub" "$work/npm" latest --force

# The wait helper is exercised against a fake registry. It has no publish
# command, proving both paths run without publishing anything.
mkdir "$work/npm-stub"
cat >"$work/npm-stub/npm" <<'EOF'
#!/bin/sh
set -eu
[ "$1" = view ] || exit 97
spec=$2
case ":${NPM_STUB_PRESENT:-}:" in
*":$spec:"*) printf '%s\n' "${spec##*@}" ;;
*) echo E404 >&2; exit 1 ;;
esac
EOF
chmod 0755 "$work/npm-stub/npm"
accepts "npm wait accepts every platform package once served" \
	env NPM_STUB_PRESENT='@buildergarage/garage-linux-x64@9.8.7:@buildergarage/garage-darwin-arm64@9.8.7' \
	GARAGE_NPM_WAIT_ATTEMPTS=2 GARAGE_NPM_WAIT_SECONDS=0 PATH="$work/npm-stub:$PATH" \
	sh "$npmwait" 9.8.7 @buildergarage/garage-linux-x64 @buildergarage/garage-darwin-arm64
refuses "npm wait fails before installation when a platform package never appears" \
	env NPM_STUB_PRESENT='@buildergarage/garage-linux-x64@9.8.7' \
	GARAGE_NPM_WAIT_ATTEMPTS=2 GARAGE_NPM_WAIT_SECONDS=0 PATH="$work/npm-stub:$PATH" \
	sh "$npmwait" 9.8.7 @buildergarage/garage-linux-x64 @buildergarage/garage-darwin-arm64
refuses "npm wait refuses zero attempts" \
	env GARAGE_NPM_WAIT_ATTEMPTS=0 sh "$npmwait" 9.8.7 @buildergarage/garage-linux-x64

mkdir "$work/npm-install-stub"
cat >"$work/npm-install-stub/npm" <<'EOF'
#!/bin/sh
set -eu
case $1 in
view)
	case "$2 $3" in
	'@buildergarage/garage@9.8.7 version') printf '9.8.7\n' ;;
	'@buildergarage/garage@9.8.7 optionalDependencies')
		printf '{"@buildergarage/garage-linux-x64":"9.8.7","@buildergarage/garage-darwin-arm64":"9.8.7"}\n' ;;
	'@buildergarage/garage-linux-x64@9.8.7 version') printf '9.8.7\n' ;;
	*) echo E404 >&2; exit 1 ;;
	esac
	;;
install) echo install >>"$NPM_STUB_LOG"; exit 97 ;;
*) exit 97 ;;
esac
EOF
chmod 0755 "$work/npm-install-stub/npm"
install_log=$work/npm-install.log
refuses "registry install waits for every platform package before installing" \
	env NPM_STUB_LOG="$install_log" GARAGE_NPM_WAIT_ATTEMPTS=1 GARAGE_NPM_WAIT_SECONDS=0 \
	PATH="$work/npm-install-stub:$PATH" sh "$npmcheck" --registry 9.8.7 v9.8.7 revision
if [ -e "$install_log" ]; then
	fail "registry install does not run npm install before every platform package is served"
else
	pass "registry install does not run npm install before every platform package is served"
fi

mkdir "$work/npm-publish-stub"
cat >"$work/npm-publish-stub/npm" <<'EOF'
#!/bin/sh
set -eu
case $1 in
view) echo E404 >&2; exit 1 ;;
publish) basename "$2" >>"$NPM_STUB_LOG" ;;
*) exit 97 ;;
esac
EOF
chmod 0755 "$work/npm-publish-stub/npm"
publish_log=$work/npm-publish.log
refuses "npm-publish fails before publishing the launcher when platform packages are not served" \
	env NPM_STUB_LOG="$publish_log" GARAGE_NPM_WAIT_ATTEMPTS=1 GARAGE_NPM_WAIT_SECONDS=0 \
	PATH="$work/npm-publish-stub:$PATH" sh "$npmpub" "$work/npm" latest
if grep -q '^garage$' "$publish_log" 2>/dev/null; then
	fail "npm-publish does not publish the launcher before every platform package is served"
else
	pass "npm-publish does not publish the launcher before every platform package is served"
fi

# Which names reach the wait step (#186). The launcher's platform packages
# must be queried as four separate, valid names: the v0.5.1 publication
# passed them as one argument joined by a literal "\n". This registry stub
# records every "npm view" spec, serves a package once it is published, and
# never serves anything else.
platform_specs='@buildergarage/garage-darwin-arm64@9.8.7
@buildergarage/garage-darwin-x64@9.8.7
@buildergarage/garage-linux-arm64@9.8.7
@buildergarage/garage-linux-x64@9.8.7'
mkdir "$work/npm-names-stub"
cat >"$work/npm-names-stub/npm" <<'EOF'
#!/bin/sh
set -eu
case $1 in
view)
	printf '%s\n' "$2" >>"$NPM_STUB_VIEWS"
	if [ "${3:-}" = optionalDependencies ]; then
		printf '%s\n' "$NPM_STUB_DEPS_JSON"
	elif grep -Fxq -- "$2" "$NPM_STUB_SERVED" 2>/dev/null; then
		printf '%s\n' "${2##*@}"
	else
		echo E404 >&2
		exit 1
	fi
	;;
publish)
	spec=$(node -p 'const p = require(process.argv[1] + "/package.json"); p.name + "@" + p.version' "$2")
	printf '%s\n' "$spec" >>"$NPM_STUB_SERVED"
	printf '%s\n' "$spec" >>"$NPM_STUB_PUBLISHED"
	;;
install) echo install >>"$NPM_STUB_INSTALL"; exit 97 ;;
pack)
	# The published copy of NAME@VERSION, as a .tgz in the current directory,
	# made from NPM_STUB_PACK_FROM's generated package of the same name. The
	# package named by NPM_STUB_MISMATCH gets a different executable.
	name=${2%@*}
	base=${name#@buildergarage/}
	tmp=$(mktemp -d)
	cp -R "$NPM_STUB_PACK_FROM/$base" "$tmp/package"
	if [ "$2" = "${NPM_STUB_MISMATCH:-}" ]; then
		printf 'not the generated executable\n' >"$tmp/package/bin/garage"
	fi
	tar -czf "buildergarage-$base-${2##*@}.tgz" -C "$tmp" package
	rm -rf "$tmp"
	;;
*) exit 97 ;;
esac
EOF
chmod 0755 "$work/npm-names-stub/npm"
# valid_specs FILE: every line is one package spec, with no backslash or
# whitespace inside it.
valid_specs() { ! grep -q '[\\[:space:]]' "$1"; }

if command -v node >/dev/null 2>&1; then
	nd=$work/npm-names
	mkdir -p "$nd"
	: >"$nd/served"
	accepts "npm-publish publishes the launcher once its platform packages are served" \
		env NPM_STUB_VIEWS="$nd/views" NPM_STUB_SERVED="$nd/served" NPM_STUB_PUBLISHED="$nd/published" \
		NPM_STUB_INSTALL="$nd/install" NPM_STUB_DEPS_JSON='{}' \
		GARAGE_NPM_WAIT_ATTEMPTS=1 GARAGE_NPM_WAIT_SECONDS=0 \
		PATH="$work/npm-names-stub:$PATH" sh "$npmpub" "$work/npm" latest
	if valid_specs "$nd/views"; then
		pass "npm-publish queries only valid, separate package names"
	else
		cat "$nd/views"
		fail "npm-publish queries only valid, separate package names"
	fi
	# Each platform package is queried twice: before publishing it, and by
	# the wait step before the launcher.
	check_eq "npm-publish's wait step queries each platform package separately" \
		"$(grep -v '^@buildergarage/garage@' "$nd/views" | LC_ALL=C sort | uniq -c | awk '{ print $1, $2 }' | tr '\n' ' ')" \
		"$(printf '%s\n' "$platform_specs" | sed 's/^/2 /' | tr '\n' ' ')"
	check_eq "npm-publish publishes the four platform packages, then the launcher" \
		"$(head -n 4 "$nd/published" | LC_ALL=C sort | tr '\n' ' ')|$(sed -n '5p' "$nd/published")|$(wc -l <"$nd/published" | tr -d ' ')" \
		"$(printf '%s\n' "$platform_specs" | tr '\n' ' ')|@buildergarage/garage@9.8.7|5"

	rd=$work/npm-registry-names
	mkdir -p "$rd"
	printf '%s\n' '@buildergarage/garage@9.8.7' "$platform_specs" >"$rd/served"
	refuses "registry install stops at the stubbed npm install" \
		env NPM_STUB_VIEWS="$rd/views" NPM_STUB_SERVED="$rd/served" NPM_STUB_PUBLISHED="$rd/published" \
		NPM_STUB_INSTALL="$rd/install" \
		NPM_STUB_DEPS_JSON='{"@buildergarage/garage-darwin-arm64":"9.8.7","@buildergarage/garage-darwin-x64":"9.8.7","@buildergarage/garage-linux-arm64":"9.8.7","@buildergarage/garage-linux-x64":"9.8.7"}' \
		GARAGE_NPM_WAIT_ATTEMPTS=1 GARAGE_NPM_WAIT_SECONDS=0 \
		PATH="$work/npm-names-stub:$PATH" sh "$npmcheck" --registry 9.8.7 v9.8.7 revision
	if valid_specs "$rd/views"; then
		pass "registry install queries only valid, separate package names"
	else
		cat "$rd/views"
		fail "registry install queries only valid, separate package names"
	fi
	check_eq "registry install waits for each declared platform package separately" \
		"$(grep -v '^@buildergarage/garage@' "$rd/views" | LC_ALL=C sort | tr '\n' ' ')" \
		"$(printf '%s\n' "$platform_specs" | tr '\n' ' ')"
	if [ -s "$rd/install" ]; then
		pass "registry install proceeds to npm install once every platform package is served"
	else
		fail "registry install proceeds to npm install once every platform package is served"
	fi

	# The already-published path, which recovers a partial publication (#189).
	# The four platform packages are already served at 9.8.7. When each
	# published copy matches the generated package, none is published again,
	# and processing continues: the launcher is published, once.
	ad=$work/npm-already
	mkdir -p "$ad"
	printf '%s\n' "$platform_specs" >"$ad/served"
	accepts "npm-publish recovers when the platform packages are already published and match" \
		env NPM_STUB_VIEWS="$ad/views" NPM_STUB_SERVED="$ad/served" NPM_STUB_PUBLISHED="$ad/published" \
		NPM_STUB_INSTALL="$ad/install" NPM_STUB_DEPS_JSON='{}' NPM_STUB_PACK_FROM="$work/npm" \
		GARAGE_NPM_WAIT_ATTEMPTS=1 GARAGE_NPM_WAIT_SECONDS=0 \
		PATH="$work/npm-names-stub:$PATH" sh "$npmpub" "$work/npm" latest
	check_eq "npm-publish skips each matching already-published platform package" \
		"$(grep -c 'is already published, and matches; not publishing it again' "$work/out" || true)" 4
	check_eq "npm-publish then continues and publishes only the launcher" \
		"$(tr '\n' ' ' <"$ad/published" 2>/dev/null)" "@buildergarage/garage@9.8.7 "

	# One published copy differs from the generated package: the run fails,
	# and the launcher is never published.
	md=$work/npm-mismatch
	mkdir -p "$md"
	printf '%s\n' "$platform_specs" >"$md/served"
	refuses "npm-publish fails when an already-published platform package differs" \
		env NPM_STUB_VIEWS="$md/views" NPM_STUB_SERVED="$md/served" NPM_STUB_PUBLISHED="$md/published" \
		NPM_STUB_INSTALL="$md/install" NPM_STUB_DEPS_JSON='{}' NPM_STUB_PACK_FROM="$work/npm" \
		NPM_STUB_MISMATCH=@buildergarage/garage-linux-x64@9.8.7 \
		GARAGE_NPM_WAIT_ATTEMPTS=1 GARAGE_NPM_WAIT_SECONDS=0 \
		PATH="$work/npm-names-stub:$PATH" sh "$npmpub" "$work/npm" latest
	if grep -q 'differs from the package generated here' "$work/out" 2>/dev/null; then
		pass "npm-publish names the differing package"
	else
		cat "$work/out"
		fail "npm-publish names the differing package"
	fi
	if [ -s "$md/published" ]; then
		cat "$md/published"
		fail "npm-publish publishes nothing, and no launcher, when a platform package differs"
	else
		pass "npm-publish publishes nothing, and no launcher, when a platform package differs"
	fi
else
	echo "skip the npm package-name checks: node is not installed"
fi

if command -v node >/dev/null 2>&1; then
	host=$(node -p 'process.platform + "-" + process.arch')
	l=$work/launcher
	mkdir -p "$l/bin" "$l/node_modules/@buildergarage/garage-$host/bin"
	cp "$scripts/../../npm/garage.js" "$l/bin/garage.js"
	printf '#!/bin/sh\necho "args: $*"\nexit 3\n' >"$l/node_modules/@buildergarage/garage-$host/bin/garage"
	chmod 0755 "$l/node_modules/@buildergarage/garage-$host/bin/garage"
	out=$(node "$l/bin/garage.js" one "two words" 2>&1) && st=0 || st=$?
	check_eq "the launcher passes arguments through" "$out" "args: one two words"
	check_eq "the launcher exits with the executable's status" "$st" 3
	rm -rf "$l/node_modules"
	out=$(node "$l/bin/garage.js" version 2>&1) && st=0 || st=$?
	check_eq "the launcher fails without its platform package" "$st" 1
	if printf '%s\n' "$out" | grep -q "npm does not provide garage for $host"; then
		pass "the launcher says npm does not provide garage, and points to the docs"
	else
		fail "the launcher says npm does not provide garage, and points to the docs (got: $out)"
	fi
else
	echo "skip the npm launcher checks: node is not installed"
fi

if [ "$failures" -ne 0 ]; then
	echo "$failures check(s) failed"
	exit 1
fi
echo "all release script checks passed"
