#!/bin/sh
# release-notes.sh: write a garage release's notes to standard output.
#
#   release-notes.sh TAG COMMIT [REPOSITORY]
#
# The notes relate the release tag, the tagged commit (which is also the
# revision of the skills and manuals embedded in every executable), and
# buildergarage.app's pinned source snapshot (docs/adr/0003-release-distribution.md).
# When the release's inventory includes Linux packages (release-inventory.sh),
# the notes list them and say how to verify one before installing it.

set -eu

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
	echo "usage: release-notes.sh TAG COMMIT [REPOSITORY]" >&2
	exit 2
fi
tag=$1
commit=$2
repo=${3:-bdonaldharris/builder-garage}
url=https://github.com/$repo
short=$(printf '%s' "$commit" | cut -c1-7)
version=${tag#v}
inventory=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)/release-inventory.sh
packages=$(sh "$inventory" "$version" packages)

cat <<EOF
\`garage\` $tag for macOS, Linux, and Windows, on amd64 and arm64.

## Platform trust

- **macOS:** the executables are signed with the Developer ID Application identity \`Developer ID Application: BRIAN DONALD HARRIS (38397A2FG6)\`, with the hardened runtime and a secure timestamp, and notarized by Apple. A bare executable cannot be stapled, so Gatekeeper confirms the notarization with Apple when it checks a quarantined copy. [Verifying a release]($url/blob/$tag/docs/release-verification.md#macos-signature-and-notarization) shows how to check the signature.
- **Windows:** the executables are not yet Authenticode-signed ([#122]($url/issues/122)). You may encounter SmartScreen warnings.
- **What verification proves:** the signed checksum manifest and the provenance attestations below establish this release's integrity and origin. The macOS signature adds Apple's publisher trust; on Windows, nothing replaces your operating system's publisher trust yet.

Download, verify, and extract from a terminal, as [Verifying a release]($url/blob/$tag/docs/release-verification.md) shows: \`gh release download\` or \`curl\`, verify, then \`tar\` on macOS and Linux, or \`Expand-Archive\` on Windows.
EOF

if [ -n "$packages" ]; then
	cat <<EOF

## Linux packages

\`.deb\` and \`.rpm\` packages install \`/usr/bin/garage\` and its license file, and nothing else. \`apt\`, \`dpkg\`, \`dnf\`, and \`rpm\` own upgrades and removal. No package touches your project configuration, installed skills, or local history.

EOF
	for p in $packages; do
		echo "- \`$p\`"
	done
	cat <<EOF

The packages carry no PGP signature. Verify the one you download against the signed checksum manifest before installing it, as [Verifying a release]($url/blob/$tag/docs/release-verification.md#linux-packages) shows. Then install it from the file, for example \`sudo apt install ./garage_${version}_linux_amd64.deb\` or \`sudo dnf install ./garage-${version}-1.x86_64.rpm\`.
EOF
fi

cat <<EOF

## Provenance

- **Release tag:** \`$tag\`
- **Tagged commit:** [\`$commit\`]($url/commit/$commit)
- **Embedded skills and manuals:** every executable embeds the canonical skills and manuals from the tagged commit. \`garage skills install\` installs them with \`$commit\` as their revision. No separate skills download is published.
- **buildergarage.app:** the site offers individual skills and manuals from a pinned source revision, shown on each artifact as "Source revision". When that revision is \`$short\`, the site's artifacts are the same bytes this release embeds. Otherwise they come from another revision.

## Verify before you run it

The checksum manifest is signed with keyless Sigstore/cosign by this repository's release workflow at \`$tag\`, and every archive and package has a GitHub artifact attestation. Verify them as described in [Verifying a release]($url/blob/$tag/docs/release-verification.md):

\`\`\`sh
cosign verify-blob garage_${version}_checksums.txt \\
  --bundle garage_${version}_checksums.txt.sigstore.json \\
  --certificate-identity $url/.github/workflows/release.yml@refs/tags/$tag \\
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
\`\`\`
EOF
