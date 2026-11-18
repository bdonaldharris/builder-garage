# Verifying a release

Each `garage` release on the [Releases page](https://github.com/bdonaldharris/builder-garage/releases) carries:

- **Six archives:** `garage_<version>_<os>_<arch>.tar.gz` for macOS and Linux, and `.zip` for Windows, on amd64 and arm64;
- **From `v0.2.0`, four Linux packages:** `garage_<version>_linux_amd64.deb`, `garage_<version>_linux_arm64.deb`, `garage-<version>-1.x86_64.rpm`, and `garage-<version>-1.aarch64.rpm`;
- **`garage_<version>_checksums.txt`:** the SHA-256 checksum manifest of the archives and packages;
- **`garage_<version>_checksums.txt.sigstore.json`:** the manifest's [Sigstore](https://www.sigstore.dev/) bundle.

Every archive and package also has a [GitHub artifact attestation](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations).

`<version>` is the tag without its `v`. For tag `v0.1.1`, the manifest is `garage_0.1.1_checksums.txt`. Which files each release carries is its release inventory, recorded in [ADR 0003](adr/0003-release-distribution.md#release-inventory).

There are no published releases until `v0.1.1`. The design is recorded in [ADR 0003](adr/0003-release-distribution.md).

## Platform trust

Until platform signing ([#122](https://github.com/bdonaldharris/builder-garage/issues/122)) is complete:

- **macOS:** the executables are not yet Developer ID signed and notarized. You may encounter Gatekeeper restrictions.
- **Windows:** the executables are not yet Authenticode-signed. You may encounter SmartScreen warnings.
- **What verification proves:** the signed checksum manifest and the provenance attestations establish a release's integrity and origin. They do not replace your operating system's publisher trust.

Download, verify, and extract from a terminal, as the steps below show:
- download with `gh release download` or `curl`;
- verify the signature and checksum;
- extract with `tar` on macOS and Linux, or `Expand-Archive` on Windows.

## What to verify

1. **The manifest's signature.** It was signed by this repository's release workflow, run at that tag. This is the check that matters most: it ties the manifest to the release workflow, and the manifest ties every archive to it.
2. **Your archive's checksum,** against the signed manifest.
3. **Optionally, your archive's attestation,** as additional provenance.

You need [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v3 or later for step 1, and the [GitHub CLI](https://cli.github.com/) (`gh`) for step 3.

## Installing with install.sh

On macOS and Linux, [`install.sh`](../install.sh) performs the signature and checksum steps below for you, then installs `garage` for the current user. It installs nothing if a check fails or cannot run. See [Install a release](../README.md#install-a-release). It needs cosign v3 or later. It does not verify the attestation, which is optional provenance (step 3) and needs an authenticated `gh`.

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

## Linux packages

From `v0.2.0`, each release also carries `.deb` and `.rpm` packages for amd64 and arm64. A package installs `/usr/bin/garage` and its license file, and nothing else. `apt`, `dpkg`, `dnf`, and `rpm` own upgrades and removal. Installing, upgrading, or removing a package never touches project configuration, installed skills, manuals, or local history.

The packages carry no PGP signature. Their integrity and origin come from the same signed checksum manifest and attestations as the archives, so **verify the package before installing it**:

```sh
TAG=v0.2.0
VERSION=${TAG#v}
PACKAGE="garage_${VERSION}_linux_amd64.deb"   # or garage_${VERSION}_linux_arm64.deb,
                                                #    garage-${VERSION}-1.x86_64.rpm, garage-${VERSION}-1.aarch64.rpm
gh release download "$TAG" --repo bdonaldharris/builder-garage \
  --pattern "$PACKAGE" \
  --pattern "garage_${VERSION}_checksums.txt" \
  --pattern "garage_${VERSION}_checksums.txt.sigstore.json"

cosign verify-blob "garage_${VERSION}_checksums.txt" \
  --bundle "garage_${VERSION}_checksums.txt.sigstore.json" \
  --certificate-identity "https://github.com/bdonaldharris/builder-garage/.github/workflows/release.yml@refs/tags/${TAG}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --check --ignore-missing "garage_${VERSION}_checksums.txt"

# Optionally, the package's attestation:
gh attestation verify "$PACKAGE" --repo bdonaldharris/builder-garage \
  --signer-workflow bdonaldharris/builder-garage/.github/workflows/release.yml
```

`cosign` prints `Verified OK`, and `sha256sum` prints the package's name followed by `OK`. Only then install it from the file:

```sh
sudo apt install "./$PACKAGE"   # .deb (Debian, Ubuntu)
sudo dnf install "./$PACKAGE"   # .rpm (Fedora, RHEL)
garage version
```

Remove it with `sudo apt remove garage` or `sudo dnf remove garage`. The package installs `garage` system-wide in `/usr/bin`, while [`install.sh`](../install.sh) installs it for one user in `~/.local/bin`. Use one or the other, so that only one `garage` is on your `PATH`.

## Homebrew (Linux)

On Linux, `garage` is also available as a Homebrew cask from Builder Garage's tap, [`bdonaldharris/homebrew-tap`](https://github.com/bdonaldharris/homebrew-tap):

```sh
brew tap bdonaldharris/tap
brew trust --tap bdonaldharris/tap
brew install --cask bdonaldharris/tap/garage
garage version
```

**Always use the full name, `bdonaldharris/tap/garage`.** homebrew/core has an unrelated formula also called `garage` (the Garage object store), which installs a different `garage` command. `brew install garage` would install that one instead, and it conflicts with this cask on Homebrew's `bin`.

Homebrew requires you to trust a third-party tap before it uses its casks. `brew trust` is that explicit step: review the tap's [cask](https://github.com/bdonaldharris/homebrew-tap/blob/main/Casks/garage.rb) first if you want to see what it installs.

- **What it installs.** The cask downloads the same Linux archive as the release, pinned to the SHA-256 in the release's signed checksum manifest, and installs `garage` on Homebrew's `bin`.
- **How the tap is updated.** After each release passes its final verification, the release workflow commits the cask to the tap. Before committing, it checks the cask's checksums against the published, cosign-verified manifest.
- **Upgrade and removal.** Upgrade with `brew upgrade --cask bdonaldharris/tap/garage`, and remove it with `brew uninstall --cask bdonaldharris/tap/garage`.
- **macOS isn't offered yet.** The cask serves Linux only until macOS executables are notarized ([#122](https://github.com/bdonaldharris/builder-garage/issues/122)). On macOS, Homebrew refuses it with "This cask requires Linux."

To check the archive the cask uses yourself, verify the release's manifest as above. Then compare the cask's `sha256` for your architecture with the manifest's entry for `garage_<version>_linux_<arch>.tar.gz`.

Use one installation method at a time: the Homebrew cask, a Linux package (`/usr/bin`), or [`install.sh`](../install.sh) (`~/.local/bin`), so that only one `garage` is on your `PATH`.

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

## For maintainers

[`.github/scripts/verify-release.sh`](../.github/scripts/verify-release.sh) runs every check above against a directory of a release's assets. It also confirms that the assets are exactly the release's inventory (selected by its tag, so every published release stays verifiable), and that a signature check pinned to another identity fails. The release workflow runs it before publishing and again on the published release.
