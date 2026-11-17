#!/bin/sh
# release-notes.sh: write a garage release's notes to standard output.
#
#   release-notes.sh TAG COMMIT [REPOSITORY]
#
# The notes relate the release tag, the tagged commit (which is also the
# revision of the skills and manuals embedded in every executable), and
# buildergarage.app's pinned source snapshot (docs/adr/0003-release-distribution.md).

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

cat <<EOF
\`garage\` $tag for macOS, Linux, and Windows, on amd64 and arm64.

## Provenance

- **Release tag:** \`$tag\`
- **Tagged commit:** [\`$commit\`]($url/commit/$commit)
- **Embedded skills and manuals:** every executable embeds the canonical skills and manuals from the tagged commit. \`garage skills install\` installs them with \`$commit\` as their revision. No separate skills download is published.
- **buildergarage.app:** the site offers individual skills and manuals from a pinned source revision, shown on each artifact as "Source revision". When that revision is \`$short\`, the site's artifacts are the same bytes this release embeds. Otherwise they come from another revision.

## Verify before you run it

The checksum manifest is signed with keyless Sigstore/cosign by this repository's release workflow at \`$tag\`, and every archive has a GitHub artifact attestation. Verify them as described in [Verifying a release]($url/blob/$tag/docs/release-verification.md):

\`\`\`sh
cosign verify-blob garage_${tag#v}_checksums.txt \\
  --bundle garage_${tag#v}_checksums.txt.sigstore.json \\
  --certificate-identity $url/.github/workflows/release.yml@refs/tags/$tag \\
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
\`\`\`
EOF
