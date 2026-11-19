#!/bin/sh
# check-macos-signing-env.sh: refuse a release build without every macOS
# signing and notarization credential (#122, D3).
#
#   check-macos-signing-env.sh MODE
#
# MODE is GARAGE_RELEASE_MODE. In release mode, GoReleaser's notarize.macos
# signs and notarizes the darwin executables (.goreleaser.yaml), and every one
# of these must be set:
#
#   MACOS_SIGN_P12          the Developer ID Application certificate and its
#                           full chain, as a .p12, base64-encoded on one line
#   MACOS_SIGN_PASSWORD     the .p12's password
#   MACOS_NOTARY_KEY        the App Store Connect API key (.p8), base64-encoded
#                           on one line
#   MACOS_NOTARY_KEY_ID     the API key's ID
#   MACOS_NOTARY_ISSUER_ID  the API key's issuer ID
#
# GoReleaser signs but does not notarize when a notary value is empty, and
# only logs it, so this refusal is what keeps a release from shipping
# un-notarized macOS executables. The release workflow runs it before
# GoReleaser; platform-sign.sh runs it again inside GoReleaser, for each darwin
# binary. In snapshot and dry-run modes, notarization is disabled and nothing
# is required.
#
# It names missing or malformed variables, and never prints their values.

set -eu

if [ "$#" -ne 1 ]; then
	echo "usage: check-macos-signing-env.sh MODE" >&2
	exit 2
fi

case $1 in
snapshot | dry-run)
	echo "macos-signing: $1 build; macOS notarization is disabled, so no credentials are required"
	exit 0
	;;
release) ;;
*)
	echo "macos-signing: GARAGE_RELEASE_MODE must be snapshot, dry-run, or release; got '$1'" >&2
	exit 1
	;;
esac

missing=
for name in MACOS_SIGN_P12 MACOS_SIGN_PASSWORD MACOS_NOTARY_KEY MACOS_NOTARY_KEY_ID MACOS_NOTARY_ISSUER_ID; do
	eval "value=\${$name:-}"
	if [ -z "$value" ]; then
		missing="$missing $name"
	fi
done
if [ -n "$missing" ]; then
	echo "macos-signing: release build refused; these macOS signing credentials are not set:$missing" >&2
	echo "macos-signing: they come from the macos-signing environment (#122)" >&2
	exit 1
fi

# The certificate and the key must be one line of standard base64, as quill
# decodes them; a line break or a file path would fail only at signing time.
malformed=
for name in MACOS_SIGN_P12 MACOS_NOTARY_KEY; do
	eval "value=\$$name"
	if ! printf '%s' "$value" | LC_ALL=C grep -Eqx '[A-Za-z0-9+/]+={0,2}' ||
		[ "$(printf '%s' "$value" | wc -l | tr -d ' ')" -ne 0 ]; then
		malformed="$malformed $name"
	fi
done
if [ -n "$malformed" ]; then
	echo "macos-signing: release build refused; these are not one line of base64:$malformed" >&2
	exit 1
fi

echo "macos-signing: release build; all five macOS signing and notarization credentials are set"
