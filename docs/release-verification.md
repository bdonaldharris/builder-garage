# Verifying a release

Each `garage` release on the [Releases page](https://github.com/bdonaldharris/builder-garage/releases) carries:

- **Six archives:** `garage_<version>_<os>_<arch>.tar.gz` for macOS and Linux, and `.zip` for Windows, on amd64 and arm64;
- **`garage_<version>_checksums.txt`:** the SHA-256 checksum manifest of the six archives;
- **`garage_<version>_checksums.txt.sigstore.json`:** the manifest's [Sigstore](https://www.sigstore.dev/) bundle.

Every archive also has a [GitHub artifact attestation](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations).

`<version>` is the tag without its `v`. For tag `v0.1.1`, the manifest is `garage_0.1.1_checksums.txt`.

There are no published releases until `v0.1.1`. The design is recorded in [ADR 0003](adr/0003-release-distribution.md).

## What to verify

1. **The manifest's signature.** It was signed by this repository's release workflow, run at that tag. This is the check that matters most: it ties the manifest to the release workflow, and the manifest ties every archive to it.
2. **Your archive's checksum,** against the signed manifest.
3. **Optionally, your archive's attestation,** as additional provenance.

You need [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v3 or later for step 1, and the [GitHub CLI](https://cli.github.com/) (`gh`) for step 3.

## macOS and Linux

Download the archive for your platform, the manifest, and its bundle into one directory, for example:

```sh
TAG=v0.1.1
VERSION=${TAG#v}
gh release download "$TAG" --repo bdonaldharris/builder-garage \
  --pattern "garage_${VERSION}_linux_amd64.tar.gz" \
  --pattern "garage_${VERSION}_checksums.txt" \
  --pattern "garage_${VERSION}_checksums.txt.sigstore.json"
```

1. Verify the manifest's signature, pinned to the release workflow at that tag:

   ```sh
   cosign verify-blob "garage_${VERSION}_checksums.txt" \
     --bundle "garage_${VERSION}_checksums.txt.sigstore.json" \
     --certificate-identity "https://github.com/bdonaldharris/builder-garage/.github/workflows/release.yml@refs/tags/${TAG}" \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com
   ```

   It prints `Verified OK`. Any other result means the manifest is not the one the release workflow signed for that tag: do not use the archive.

2. Verify your archive against the manifest:

   ```sh
   # Linux
   sha256sum --check --ignore-missing "garage_${VERSION}_checksums.txt"
   # macOS
   shasum -a 256 --check --ignore-missing "garage_${VERSION}_checksums.txt"
   ```

   It prints your archive's name followed by `OK`.

3. Optionally, verify the archive's attestation:

   ```sh
   gh attestation verify "garage_${VERSION}_linux_amd64.tar.gz" \
     --repo bdonaldharris/builder-garage \
     --signer-workflow bdonaldharris/builder-garage/.github/workflows/release.yml
   ```

Then extract the archive and run it:

```sh
tar -xzf "garage_${VERSION}_linux_amd64.tar.gz" garage
./garage version
```

It prints `garage version v0.1.1`. Move `garage` to a directory on your `PATH`.

## Windows (PowerShell)

```powershell
$Tag = "v0.1.1"
$Version = $Tag.TrimStart("v")
gh release download $Tag --repo bdonaldharris/builder-garage `
  --pattern "garage_${Version}_windows_amd64.zip" `
  --pattern "garage_${Version}_checksums.txt" `
  --pattern "garage_${Version}_checksums.txt.sigstore.json"
```

1. Verify the manifest's signature:

   ```powershell
   cosign verify-blob "garage_${Version}_checksums.txt" `
     --bundle "garage_${Version}_checksums.txt.sigstore.json" `
     --certificate-identity "https://github.com/bdonaldharris/builder-garage/.github/workflows/release.yml@refs/tags/$Tag" `
     --certificate-oidc-issuer https://token.actions.githubusercontent.com
   ```

2. Verify your archive against the manifest:

   ```powershell
   $Archive = "garage_${Version}_windows_amd64.zip"
   $Expected = (Select-String -Path "garage_${Version}_checksums.txt" -Pattern "  $Archive$").Line.Split(" ")[0]
   $Actual = (Get-FileHash -Algorithm SHA256 $Archive).Hash.ToLower()
   if ($Actual -eq $Expected) { "OK" } else { throw "checksum mismatch for $Archive" }
   ```

3. Optionally, verify the attestation:

   ```powershell
   gh attestation verify $Archive --repo bdonaldharris/builder-garage `
     --signer-workflow bdonaldharris/builder-garage/.github/workflows/release.yml
   ```

Then extract the archive and run it:

```powershell
Expand-Archive $Archive -DestinationPath garage
.\garage\garage.exe version
```

## Matching any release

To accept the release workflow at any version tag, rather than one tag, replace `--certificate-identity` with an anchored pattern:

```sh
--certificate-identity-regexp '^https://github\.com/bdonaldharris/builder-garage/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$'
```

Pinning the exact tag is stronger: it also proves that the manifest belongs to the release you meant to download.

## How a release relates to the skills and manuals

- **Embedded skills and manuals.** Every `garage` executable embeds the canonical skills and manuals from its tagged commit. `garage skills install` installs them with that commit as their revision. No separate skills download is published.
- **Release notes** name the tag and the tagged commit.
- **[buildergarage.app](https://buildergarage.app)** offers individual skills and manuals from a pinned source revision, shown on each artifact as "Source revision". When that revision is the release's commit, the site's artifacts are the same bytes the release embeds.

## Platform signatures

macOS executables are signed and notarized, and Windows executables are Authenticode-signed, before any release is published. Platform signing is tracked in [#122](https://github.com/bdonaldharris/builder-garage/issues/122). Its verification steps are added here when it is implemented.

## For maintainers

[`.github/scripts/verify-release.sh`](../.github/scripts/verify-release.sh) runs every check above against a directory of a release's assets. It also confirms that the assets are exactly the expected eight files, and that a signature check pinned to another identity fails. The release workflow runs it before publishing and again on the published release.
