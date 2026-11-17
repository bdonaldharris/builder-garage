#!/bin/sh
# verify-release.sh: verify a garage release's assets, as a user would
# (docs/release-verification.md).
#
#   verify-release.sh DIR TAG [REPOSITORY]
#
#   DIR         a directory holding the release's assets, and nothing else
#   TAG         the release tag, for example v0.1.1
#   REPOSITORY  owner/name; defaults to bdonaldharris/builder-garage
#
# It checks that:
#   - DIR holds exactly TAG's release inventory (release-inventory.sh): v1
#     up to v0.1.x is the six archives, the checksum manifest, and its
#     Sigstore bundle; v2 from v0.2.0 adds the Linux .deb and .rpm packages;
#   - the manifest's cosign signature verifies, pinned to the release
#     workflow at TAG and GitHub Actions' OIDC issuer;
#   - the same signature does NOT verify for another identity (the release
#     workflow on the main branch), so the pin is effective;
#   - every archive and package matches the manifest;
#   - every archive and package has a GitHub artifact attestation from the
#     release workflow.
#
# It needs cosign, gh (authenticated), and sha256sum or shasum.

set -eu

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
	echo "usage: verify-release.sh DIR TAG [REPOSITORY]" >&2
	exit 2
fi
dir=$1
tag=$2
repo=${3:-bdonaldharris/builder-garage}

case $tag in
v[0-9]*.[0-9]*.[0-9]*) ;;
*)
	echo "verify-release: $tag is not a v-prefixed version tag" >&2
	exit 2
	;;
esac
version=${tag#v}

workflow=.github/workflows/release.yml
identity=https://github.com/$repo/$workflow@refs/tags/$tag
issuer=https://token.actions.githubusercontent.com
manifest=garage_${version}_checksums.txt
bundle=$manifest.sigstore.json

failures=0
pass() { printf 'ok   %s\n' "$1"; }
fail() {
	printf 'FAIL %s\n' "$1"
	failures=$((failures + 1))
}

# The release's inventory, selected by version (release-inventory.sh), so
# every published release stays verifiable with this script.
inventory=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)/release-inventory.sh
inv_id=$(sh "$inventory" "$version" id)
files=$(sh "$inventory" "$version" manifest)

# 1. Exactly the expected assets.
want=$(sh "$inventory" "$version" all | tr '\n' ' ')
got=$(ls -1 "$dir" | LC_ALL=C sort | tr '\n' ' ')
if [ "$got" = "$want" ]; then pass "the assets are exactly release inventory $inv_id"; else
	fail "the assets are exactly release inventory $inv_id (got: $got)"
fi

# 2. The manifest's signature, pinned to the release workflow at this tag.
if cosign verify-blob "$dir/$manifest" --bundle "$dir/$bundle" \
	--certificate-identity "$identity" --certificate-oidc-issuer "$issuer"; then
	pass "the manifest is signed by $identity"
else
	fail "the manifest is signed by $identity"
fi

# 3. The pin is effective: another identity is rejected.
other=https://github.com/$repo/$workflow@refs/heads/main
if cosign verify-blob "$dir/$manifest" --bundle "$dir/$bundle" \
	--certificate-identity "$other" --certificate-oidc-issuer "$issuer" >/dev/null 2>&1; then
	fail "a signature check pinned to another identity ($other) fails"
else
	pass "a signature check pinned to another identity ($other) fails"
fi

# 4. Every archive and package matches the signed manifest.
if command -v sha256sum >/dev/null 2>&1; then
	sum() { sha256sum -c "$1"; }
else
	sum() { shasum -a 256 -c "$1"; }
fi
if (cd "$dir" && sum "$manifest"); then pass "every listed file matches the manifest"; else fail "every listed file matches the manifest"; fi

# 5. Every archive's and package's attestation, from the release workflow.
for a in $files; do
	if gh attestation verify "$dir/$a" --repo "$repo" --signer-workflow "$repo/$workflow" >/dev/null; then
		pass "$a has an attestation from $workflow"
	else
		fail "$a has an attestation from $workflow"
	fi
done

if [ "$failures" -ne 0 ]; then
	echo "$failures check(s) failed"
	exit 1
fi
echo "release $tag verified"
