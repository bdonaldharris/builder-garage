#!/bin/sh
# check-macos-installer-env.sh: fail closed without the dedicated Installer
# certificate and existing notarization credentials required for a .pkg.
set -eu
[ "$#" -eq 1 ] || { echo "usage: check-macos-installer-env.sh MODE" >&2; exit 2; }
case $1 in snapshot | dry-run) exit 0 ;; release) ;; *) echo "macos-installer: unknown mode '$1'" >&2; exit 2 ;; esac
missing=
for name in MACOS_INSTALLER_P12 MACOS_INSTALLER_PASSWORD MACOS_NOTARY_KEY MACOS_NOTARY_KEY_ID MACOS_NOTARY_ISSUER_ID; do
	eval "value=\${$name:-}"
	[ -n "$value" ] || missing="$missing $name"
done
[ -z "$missing" ] || { echo "macos-installer: release refused; missing:$missing" >&2; exit 1; }
for name in MACOS_INSTALLER_P12 MACOS_NOTARY_KEY; do
	eval "value=\${$name}"
	printf '%s' "$value" | LC_ALL=C grep -Eqx '[A-Za-z0-9+/]+={0,2}' || { echo "macos-installer: $name is not one-line base64" >&2; exit 1; }
done
echo "macos-installer: release credentials are present"
