#!/bin/sh
# build-macos-package.sh: make one architecture-specific garage installer.
#
#   build-macos-package.sh MODE VERSION ARCH ARCHIVE OUTPUT
#
# MODE is snapshot, dry-run, or release. Release mode imports the dedicated
# Developer ID Installer certificate into a temporary keychain, signs the flat
# package, submits it to Apple's notary service, and staples the accepted
# ticket. Other modes deliberately make an unsigned package for structural
# and lifecycle checks only.

set -eu

if [ "$#" -ne 5 ]; then
	echo "usage: build-macos-package.sh MODE VERSION ARCH ARCHIVE OUTPUT" >&2
	exit 2
fi
mode=$1
version=$2
arch=$3
archive=$4
output=$5

case $mode in snapshot | dry-run | release) ;; *) echo "build-macos-package: unknown mode '$mode'" >&2; exit 2 ;; esac
case $arch in amd64 | arm64) ;; *) echo "build-macos-package: ARCH must be amd64 or arm64" >&2; exit 2 ;; esac
case $version in *[!0-9A-Za-z.+-]* | '') echo "build-macos-package: invalid version" >&2; exit 2 ;; esac
[ -f "$archive" ] || { echo "build-macos-package: archive not found: $archive" >&2; exit 1; }
[ ! -e "$output" ] || { echo "build-macos-package: output already exists: $output" >&2; exit 1; }
for tool in pkgbuild productbuild tar; do command -v "$tool" >/dev/null 2>&1 || { echo "build-macos-package: missing $tool" >&2; exit 1; }; done

scripts=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
root=/usr/local/lib/buildergarage
receipt=com.buildergarage.garage.pkg.$arch
work=$(mktemp -d "${TMPDIR:-/tmp}/garage-macos-pkg.XXXXXX")
keychain=
cleanup() {
	[ -z "$keychain" ] || security delete-keychain "$keychain" >/dev/null 2>&1 || true
	rm -rf -- "$work"
}
trap cleanup EXIT HUP INT TERM

payload=$work/payload$root/$version/$arch
mkdir -p "$payload"
tar -xzf "$archive" -C "$payload" garage LICENSE
chmod 0755 "$payload/garage"
chmod 0644 "$payload/LICENSE"
cat >"$payload/PACKAGE-OWNERSHIP.json" <<EOF
{"package":"$receipt","version":"$version","architecture":"$arch"}
EOF
chmod 0644 "$payload/PACKAGE-OWNERSHIP.json"
cat >"$payload/uninstall" <<EOF
#!/bin/sh
set -eu
root=$root
receipt=$receipt
owned() { [ -f "\$1/PACKAGE-OWNERSHIP.json" ] && grep -Fq "\\\"package\\\":\\\"\$receipt\\\"" "\$1/PACKAGE-OWNERSHIP.json"; }
for payload in "\$root"/*/*; do
	[ -d "\$payload" ] || continue
	owned "\$payload" || continue
	rm -rf -- "\$payload"
	parent=\$(dirname "\$payload")
	rmdir "\$parent" 2>/dev/null || true
done
if [ -L "\$root/current" ] && [ "\$(readlink "\$root/current")" = "$version/$arch" ]; then rm -f -- "\$root/current"; fi
if [ -L /usr/local/bin/garage ] && [ "\$(readlink /usr/local/bin/garage)" = "$root/current/garage" ]; then rm -f -- /usr/local/bin/garage; fi
pkgutil --forget "\$receipt" >/dev/null 2>&1 || true
rmdir "\$root" 2>/dev/null || true
EOF
chmod 0755 "$payload/uninstall"

mkdir "$work/scripts"
cat >"$work/scripts/preinstall" <<EOF
#!/bin/sh
set -eu
root=$root
expected=$root/current/garage
valid_current=false
if [ -e "\$root/current" ] || [ -L "\$root/current" ]; then
	if [ -L "\$root/current" ]; then
		target=\$(readlink "\$root/current")
		case "\$target" in
		/* | *..*) ;;
		*)
			payload="\$root/\$target"
			if [ -d "\$payload" ] && [ -f "\$payload/PACKAGE-OWNERSHIP.json" ] && \
				grep -Eq '"package":"com\\.buildergarage\\.garage\\.pkg\\.(arm64|amd64)".*"architecture":"(arm64|amd64)"' "\$payload/PACKAGE-OWNERSHIP.json"; then
				valid_current=true
			fi
			;;
		esac
	fi
fi
if [ -e /usr/local/bin/garage ] || [ -L /usr/local/bin/garage ]; then
	[ "\$valid_current" = true ] && [ -L /usr/local/bin/garage ] && [ "\$(readlink /usr/local/bin/garage)" = "\$expected" ] || { echo "Builder Garage package refuses foreign /usr/local/bin/garage" >&2; exit 1; }
fi
if [ -e "\$root" ] && [ ! -d "\$root" ]; then echo "Builder Garage package refuses non-directory \$root" >&2; exit 1; fi
if [ -d "\$root" ]; then
	for entry in "\$root"/*; do
		[ -e "\$entry" ] || [ -L "\$entry" ] || continue
		if [ "\$(basename "\$entry")" = current ]; then
			[ "\$valid_current" = true ] || { echo "Builder Garage package refuses foreign current link in \$root" >&2; exit 1; }
			continue
		fi
		valid=false
		if [ -d "\$entry" ]; then
			for marker in "\$entry"/*/PACKAGE-OWNERSHIP.json; do
				[ -f "\$marker" ] || continue
				grep -Eq '"package":"com\\.buildergarage\\.garage\\.pkg\\.(arm64|amd64)".*"architecture":"(arm64|amd64)"' "\$marker" && valid=true
			done
		fi
		\$valid || { echo "Builder Garage package refuses unmarked content in \$root" >&2; exit 1; }
	done
fi
EOF
cat >"$work/scripts/postinstall" <<EOF
#!/bin/sh
set -eu
root=$root
version=$version
arch=$arch
receipt=$receipt
mkdir -p "\$root"
chmod 0755 "\$root"
tmp="\$root/.current-\$\$"
ln -s "\$version/\$arch" "\$tmp"
mv -f "\$tmp" "\$root/current"
ln -s "\$root/current/garage" /usr/local/bin/.garage-\$\$
mv -f /usr/local/bin/.garage-\$\$ /usr/local/bin/garage
for marker in "\$root"/*/\$arch/PACKAGE-OWNERSHIP.json; do
	[ -f "\$marker" ] || continue
	payload=\$(dirname "\$marker")
	[ "\$payload" = "\$root/\$version/\$arch" ] && continue
	grep -Fq "\\\"package\\\":\\\"\$receipt\\\"" "\$marker" || continue
	rm -rf -- "\$payload"
	rmdir "\$(dirname "\$payload")" 2>/dev/null || true
done
EOF
chmod 0755 "$work/scripts/preinstall" "$work/scripts/postinstall"

pkgbuild --root "$work/payload" --scripts "$work/scripts" --ownership recommended --identifier "$receipt" --version "$version" --install-location / "$work/component.pkg"
productbuild --package "$work/component.pkg" "$work/unsigned.pkg"

if [ "$mode" != release ]; then
	mv "$work/unsigned.pkg" "$output"
	echo "macos-package: unsigned $mode package written to $output"
	exit 0
fi

sh "$scripts/check-macos-installer-env.sh" release
for tool in security xcrun; do command -v "$tool" >/dev/null 2>&1 || { echo "build-macos-package: missing $tool" >&2; exit 1; }; done
keychain=$work/installer.keychain-db
keypass=$(openssl rand -hex 24)
security create-keychain -p "$keypass" "$keychain"
security set-keychain-settings -lut 21600 "$keychain"
security unlock-keychain -p "$keypass" "$keychain"
printf '%s' "$MACOS_INSTALLER_P12" | base64 -D >"$work/installer.p12"
security import "$work/installer.p12" -k "$keychain" -P "$MACOS_INSTALLER_PASSWORD" -T /usr/bin/productsign >/dev/null
security set-key-partition-list -S apple-tool:,apple: -s -k "$keypass" "$keychain" >/dev/null
# find-identity has no installer policy (#182); "basic" evaluates plain X.509
# validity, and -v keeps only valid identities. Its own failure is reported
# apart from finding no identity: in a pipeline, it would be hidden.
security find-identity -v -p basic "$keychain" >"$work/identities-valid" 2>"$work/find-identity.err" && st=0 || st=$?
if [ "$st" -ne 0 ]; then
	cat "$work/find-identity.err" >&2
	echo "macos-package: security find-identity failed (status $st)" >&2
	exit 1
fi
# The listing without -v only explains a refusal; its failure is not fatal.
security find-identity -p basic "$keychain" >"$work/identities-all" 2>/dev/null || : >"$work/identities-all"
identity=$(sh "$scripts/macos-installer-identity.sh" "$work/identities-valid" "$work/identities-all") || exit 1
productsign --keychain "$keychain" --sign "$identity" "$work/unsigned.pkg" "$output"
printf '%s' "$MACOS_NOTARY_KEY" | base64 -D >"$work/AuthKey.p8"
xcrun notarytool submit "$output" --key "$work/AuthKey.p8" --key-id "$MACOS_NOTARY_KEY_ID" --issuer "$MACOS_NOTARY_ISSUER_ID" --wait
xcrun stapler staple "$output"
xcrun stapler validate "$output"
echo "macos-package: signed, notarized, and stapled $output with $identity"
