#!/bin/sh
# check-signing-credentials.sh: refuse a release while any platform-signing
# credential is missing (#120 decision 4).
#
#   check-signing-credentials.sh
#
# The release workflow maps each secret of the protected release-signing
# environment to the environment variable of the same name. This list is the
# single place the required names are defined; the workflow's mapping must
# match it. Platform signing is #122, which may change these names; it changes
# them here and in .github/workflows/release.yml together.

set -eu

required="
MACOS_SIGN_P12
MACOS_SIGN_PASSWORD
MACOS_NOTARY_KEY
MACOS_NOTARY_KEY_ID
MACOS_NOTARY_ISSUER_ID
WINDOWS_SIGN_CERTIFICATE
WINDOWS_SIGN_PASSWORD
"

missing=
for name in $required; do
	eval "value=\${$name:-}"
	if [ -z "$value" ]; then
		missing="$missing $name"
	fi
done

if [ -n "$missing" ]; then
	echo "check-signing-credentials: refusing to release: these platform-signing credentials are not configured:$missing" >&2
	echo "check-signing-credentials: macOS and Windows executables must be platform-signed before any release is published (#120 decision 4, #122)" >&2
	exit 1
fi
echo "check-signing-credentials: every platform-signing credential is configured"
