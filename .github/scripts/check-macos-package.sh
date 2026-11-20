#!/bin/sh
# check-macos-package.sh: inspect and exercise a disposable macOS installer.
#
# GARAGE_MACOS_PACKAGE_CHECK=1 check-macos-package.sh MODE PACKAGE VERSION TAG REVISION [OLDER_PACKAGE]
set -eu

[ "${GARAGE_MACOS_PACKAGE_CHECK:-}" = 1 ] || { echo "check-macos-package: set GARAGE_MACOS_PACKAGE_CHECK=1 on a disposable macOS runner" >&2; exit 2; }
[ "$#" -ge 5 ] && [ "$#" -le 6 ] || { echo "usage: check-macos-package.sh MODE PACKAGE VERSION TAG REVISION [OLDER_PACKAGE]" >&2; exit 2; }
mode=$1
pkg=$2
version=$3
tag=$4
rev=$5
older=${6:-}
case $mode in snapshot | dry-run | release) ;; *) echo "check-macos-package: unknown mode '$mode'" >&2; exit 2 ;; esac
[ "$(uname -s)" = Darwin ] || { echo "check-macos-package: needs macOS" >&2; exit 2; }
[ -f "$pkg" ] || { echo "check-macos-package: package not found: $pkg" >&2; exit 1; }

name=$(basename "$pkg")
case $name in garage_*_darwin_amd64.pkg) arch=amd64 ;; garage_*_darwin_arm64.pkg) arch=arm64 ;; *) echo "check-macos-package: unexpected package name: $name" >&2; exit 1 ;; esac
receipt=com.buildergarage.garage.pkg.$arch
root=/usr/local/lib/buildergarage
work=$(mktemp -d "${TMPDIR:-/tmp}/garage-macos-package-check.XXXXXX")
sentinel=garage-macos-package-check-$$
cleanup() {
	if [ -x "$root"/*/"$arch"/uninstall ]; then sudo "$root"/*/"$arch"/uninstall >/dev/null 2>&1 || true; fi
	sudo pkgutil --forget "$receipt" >/dev/null 2>&1 || true
	rm -rf -- "$work" "$HOME/.claude/skills/$sentinel" "$HOME/.config/builder-garage/$sentinel"
}
trap cleanup EXIT HUP INT TERM
failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() { printf 'FAIL %s\n' "$1"; failures=$((failures + 1)); }
mkdir -p "$work/project/.garage" "$HOME/.claude/skills/$sentinel" "$HOME/.config/builder-garage/$sentinel"
printf 'schema_version: 2\n# sentinel\n' >"$work/project/.garage/config.yaml"
printf 'sentinel skill\n' >"$HOME/.claude/skills/$sentinel/SKILL.md"
printf 'sentinel history\n' >"$HOME/.config/builder-garage/$sentinel/history.jsonl"
user_data() { find "$work/project/.garage" "$HOME/.claude/skills/$sentinel" "$HOME/.config/builder-garage/$sentinel" -type f -exec cksum {} \; | LC_ALL=C sort; }
before=$(user_data)
unchanged() { if [ "$(user_data)" = "$before" ]; then pass "user data unchanged after $1"; else fail "user data unchanged after $1"; fi; }

pkgutil --expand-full "$pkg" "$work/expanded"
payload=$work/expanded/component.pkg/Payload
expected="usr/local/lib/buildergarage"
if [ -x "$payload/$expected/$version/$arch/garage" ] && [ -f "$payload/$expected/$version/$arch/LICENSE" ] && [ -f "$payload/$expected/$version/$arch/PACKAGE-OWNERSHIP.json" ] && [ -x "$payload/$expected/$version/$arch/uninstall" ]; then
	pass "$name has the versioned executable, license, ownership marker, and uninstall helper"
else
	fail "$name has the exact package payload"
fi
if grep -Fq "\"package\":\"$receipt\"" "$payload/$expected/$version/$arch/PACKAGE-OWNERSHIP.json"; then pass "ownership marker names $receipt"; else fail "ownership marker names $receipt"; fi
if grep -Fq -- "--identifier $receipt" "$work/expanded/component.pkg/PackageInfo" || grep -Fq "identifier=\"$receipt\"" "$work/expanded/component.pkg/PackageInfo"; then pass "component receipt is $receipt"; else fail "component receipt is $receipt"; fi
if [ "$mode" = release ]; then
	if pkgutil --check-signature "$pkg" >"$work/signature" 2>&1 && grep -q 'Developer ID Installer:' "$work/signature"; then pass "Developer ID Installer signature"; else cat "$work/signature"; fail "Developer ID Installer signature"; fi
	if xcrun stapler validate "$pkg" >"$work/stapler" 2>&1; then pass "stapled notarization ticket"; else cat "$work/stapler"; fail "stapled notarization ticket"; fi
fi

# A hosted macOS runner is disposable. Prove that neither a foreign command
# nor a textually plausible command paired with a foreign current link is
# adopted or replaced before testing the successful path.
if [ -e /usr/local/bin/garage ] || [ -L /usr/local/bin/garage ] || [ -e "$root" ] || [ -L "$root" ]; then
	echo "check-macos-package: disposable runner already has a garage destination" >&2
	exit 1
fi
sudo mkdir -p "$root"
sudo ln -s /private/garage-foreign-current "$root/current"
sudo ln -s "$root/current/garage" /usr/local/bin/garage
if sudo installer -pkg "$pkg" -target / >"$work/foreign-current" 2>&1; then
	fail "refuses a foreign current link"
else
	pass "refuses a foreign current link"
fi
if [ -L "$root/current" ] && [ "$(readlink "$root/current")" = /private/garage-foreign-current ] && [ -L /usr/local/bin/garage ] && [ "$(readlink /usr/local/bin/garage)" = "$root/current/garage" ]; then
	pass "foreign current link and command are preserved"
else
	fail "foreign current link and command are preserved"
fi
sudo rm -f /usr/local/bin/garage
sudo rm -rf "$root"
printf 'foreign command\n' | sudo tee /usr/local/bin/garage >/dev/null
if sudo installer -pkg "$pkg" -target / >"$work/foreign-command" 2>&1; then
	fail "refuses a foreign command"
else
	pass "refuses a foreign command"
fi
if [ "$(sudo cat /usr/local/bin/garage)" = "foreign command" ]; then pass "foreign command is preserved"; else fail "foreign command is preserved"; fi
sudo rm -f /usr/local/bin/garage

if [ -n "$older" ]; then
	if sudo installer -pkg "$older" -target / >"$work/older-install" 2>&1; then pass "installs older package $(basename "$older")"; else cat "$work/older-install"; fail "installs older package $(basename "$older")"; fi
	unchanged "installing the older package"
fi
if sudo installer -pkg "$pkg" -target / >"$work/install" 2>&1; then pass "installs $name"; else cat "$work/install"; fail "installs $name"; fi
unchanged "installing $name"
if [ -L /usr/local/bin/garage ] && [ "$(readlink /usr/local/bin/garage)" = "$root/current/garage" ] && [ "$(readlink "$root/current")" = "$version/$arch" ]; then pass "controlled symlinks select $version/$arch"; else fail "controlled symlinks select $version/$arch"; fi
if [ -x /usr/local/bin/garage ] && [ "$(/usr/local/bin/garage version)" = "garage version $tag" ]; then pass "installed garage reports $tag"; else fail "installed garage reports $tag"; fi
unchanged "running installed garage"
if [ -x "$root/$version/$arch/uninstall" ]; then
	if sudo "$root/$version/$arch/uninstall" >"$work/uninstall" 2>&1; then pass "package-owned uninstall helper runs"; else cat "$work/uninstall"; fail "package-owned uninstall helper runs"; fi
else fail "package-owned uninstall helper exists"; fi
if [ ! -e /usr/local/bin/garage ] && [ ! -e "$root/current" ] && ! pkgutil --pkg-info "$receipt" >/dev/null 2>&1; then pass "uninstall removes only package links and receipt"; else fail "uninstall removes only package links and receipt"; fi
unchanged "explicit uninstall"

[ "$failures" -eq 0 ] || { echo "$failures check(s) failed"; exit 1; }
echo "all macOS package checks passed for $name"
