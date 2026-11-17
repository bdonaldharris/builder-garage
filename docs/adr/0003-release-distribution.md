# ADR 0003: Release distribution: attested archives with signed checksums, from a tag-dispatched workflow

- **Status:** Accepted
- **Date:** 2026-11-16 (UTC)
- **Decision authority:** Lead maintainer, as Workflow Owner, under [GOVERNANCE.md](../../GOVERNANCE.md). Recorded in:
  - [#120](https://github.com/bdonaldharris/builder-garage/issues/120), its [correction](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6930428330), its [amendment to decision 5](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6935475192), and its [amendment to decision 4](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6938614931) (2026-11-17);
  - [#121's decisions](https://github.com/bdonaldharris/builder-garage/issues/121#issuecomment-6935474831), and those recorded in its readiness reports, [the first](https://github.com/bdonaldharris/builder-garage/issues/121#issuecomment-6935931327) and [the follow-up after the decision 4 amendment](https://github.com/bdonaldharris/builder-garage/issues/121#issuecomment-6938700447);
  - [#123's decisions](https://github.com/bdonaldharris/builder-garage/issues/123#issuecomment-6952041049), and those recorded in [its readiness](https://github.com/bdonaldharris/builder-garage/issues/123#issuecomment-6952505748) (2026-11-17): Linux packages and the release inventory.

## Context

`garage` could only be installed by building it from a checkout, which needs Git and Go. [ADR 0001](0001-go-cobra-cli-foundation.md) chose standalone executables and, in the future, GoReleaser. [ADR 0002](0002-semantic-versioning.md) chose Semantic Versioning and tagged `v0.1.0` as a tag only, leaving release automation, channels, packaging, and signing undecided. The [#69 decision](https://github.com/bdonaldharris/builder-garage/issues/69#issuecomment-6867848834) required signed checksums for release integrity.

[#120](https://github.com/bdonaldharris/builder-garage/issues/120) decides how releases are distributed, as a program of five tracks:
1. the release foundation ([#121](https://github.com/bdonaldharris/builder-garage/issues/121));
2. platform trust (#122);
3. native packages (#123);
4. package-manager channels (#124);
5. release installation and update (#125).

This record covers the program's decisions and the foundation's design.

## Decision

| Area | Decision |
| --- | --- |
| Formats | Archives: `.tar.gz` for macOS and Linux, `.zip` for Windows, each holding the executable, `LICENSE`, and `README.md`. From `v0.2.0`, Linux `.deb` and `.rpm` packages, built with GoReleaser's free nfpm support (#123). macOS `.pkg` (`pkgbuild` and `productsign` on a macOS runner) and Windows `.msi` (WiX, if its license is compatible with MIT distribution) follow their platform-trust prerequisites. GoReleaser Pro is not used. Package-manager channels are a later track. |
| Platforms | macOS, Linux, and Windows, each on amd64 and arm64. |
| Assets | Exactly the release's inventory (see [Release inventory](#6-release-inventory)). Inventory v1 (to `v0.1.x`) is the six archives, a SHA-256 checksum manifest, and its Sigstore bundle. Inventory v2 (from `v0.2.0`) adds the four Linux packages. No skills or manuals bundle. |
| Integrity | The manifest is signed with keyless Sigstore/cosign. Every archive and package also gets a GitHub artifact attestation, as additional provenance. |
| Verification identity | `https://github.com/bdonaldharris/builder-garage/.github/workflows/release.yml@refs/tags/<tag>`, with issuer `https://token.actions.githubusercontent.com`. |
| Platform trust | macOS signing and notarization, and Windows Authenticode signing (#122), are required before platform-native installers, packages, or package-manager channels are introduced. They are not required for the archive release. Until #122 is complete, release notes and verification docs disclose that macOS and Windows executables are not platform-signed. For the direct-download Linux `.deb` and `.rpm` packages, the signed checksum manifest and attestations are their platform trust: there is no PGP package-signing key, and the docs explain verifying a package before installing it. |
| Trigger | Manual dispatch of the release workflow, from an existing `vMAJOR.MINOR.PATCH` tag only. |
| First published release | `v0.1.1`. `v0.1.0` stays at `db2fcac` as a tag-only source snapshot. |
| Permissions | Separate jobs, each with the least privilege it needs. |
| Repository protection | Immutable releases. `v*` tags cannot be updated or deleted. |

### 1. GoReleaser builds; it does not sign or publish

GoReleaser ([`.goreleaser.yaml`](../../.goreleaser.yaml)) builds, archives, and checksums. Signing, attestation, and publication are separate jobs, so that no job holds more permission than its step needs. When GoReleaser's configuration and the permission boundary conflict, GoReleaser is limited to building.

The build matches `make build`: `CGO_ENABLED=0`, `-trimpath`, and the version at link time. Distributed binaries are also stripped with `-s -w`, and source builds remain the debugging path.

GoReleaser's default `main.version` is `{{ .Version }}`, which drops the leading `v`. The configuration sets `{{ .Tag }}`, so a release binary reports exactly its tag, for example `v0.1.1`.

### 2. A release is built only from a clean checkout of its tag

Every executable embeds the canonical skills and manuals from the tagged commit, and `garage skills install` records that commit as their revision. Go records the commit and a clean tree only when the checkout has no untracked, non-ignored files, so nothing is written into the checkout before the build.

### 3. The release workflow

[`.github/workflows/release.yml`](../../.github/workflows/release.yml) is dispatched with a `mode`. It refuses any ref but a version tag, and an unknown mode, before building anything.

| Job | Permissions | Purpose |
| --- | --- | --- |
| `guard` | `contents: read` | Accepts only a `vMAJOR.MINOR.PATCH` tag and a known mode. |
| `build` | `contents: read` | Builds, archives, packages, checksums, and validates the release inventory's files and the executables from the clean tagged commit. Runs the linux/amd64 archive. Uses no environment and no secrets. |
| `run-check` | `contents: read` | Release mode only. Runs every archive on its own platform: `garage version` reports the tag, and `garage skills install` installs skills whose revision is the tagged commit. |
| `package-check` | `contents: read` | Release mode only, from inventory v2. Installs, checks, and removes each Linux package with its own package manager (`apt` on Ubuntu, `dnf` in a digest-pinned Fedora container), on amd64 and arm64. |
| `sign` | `id-token: write` | Signs the manifest with keyless cosign. |
| `attest` | `id-token: write`, `attestations: write` | Attests every archive and package listed in the manifest. |
| `verify` | `contents: read`, `attestations: read` | Verifies the signature (pinned identity, and rejection of another identity), the checksums, and every attestation. |
| `publish` | `contents: write`, in the `release` environment | After the Workflow Owner approves, creates a draft with exactly the verified assets, checks it, and publishes it. |
| `post-verify` | `contents: read`, `attestations: read` | Downloads the published assets and verifies them again. |

- **Dry runs.** `dry-run` mode stops after `build`. It signs, attests, uploads, and publishes nothing, so no public trust material exists for artifacts not meant for distribution. Pull-request CI also builds a GoReleaser snapshot on every pull request, read-only and without secrets.
- **Platform signing.** Its place is a GoReleaser post-build hook ([`platform-sign.sh`](../../.github/scripts/platform-sign.sh)), before archiving, so the checksums, signature, and attestations cover signed executables.
  - Until #122 implements it, the hook leaves every binary unchanged in every mode, so no release mixes signed and unsigned platforms.
  - An unset or unknown mode still fails the build.
  - #122 adds platform signing, and any credentials and environment it needs.
- **Platform-trust disclosure.** Until #122 is complete, the release notes ([`release-notes.sh`](../../.github/scripts/release-notes.sh)) and [Verifying a release](../release-verification.md) state that:
  - macOS executables are not yet Developer ID signed and notarized, and Gatekeeper may restrict them;
  - Windows executables are not yet Authenticode-signed, and SmartScreen may warn;
  - checksum-signature and attestation verification establish integrity and origin, not operating-system publisher trust.

  They recommend downloading, verifying, and extracting from a terminal. They never advise removing quarantine, or bypassing Gatekeeper or SmartScreen.
- **`artifact-metadata: write`** is not granted. `actions/attest` needs it only for storage records, which apply only when pushing to a registry.

### 4. Repository protection

- **Release environments.**
  - The `release` environment requires the Workflow Owner's review, allows only `v*` tags, and does not let administrators bypass its rules.
  - Platform-signing credentials, when #122 introduces them, are held in a protected environment available only to the release workflow.
- **Immutable releases** are enabled before the first release, so a published release's assets and tag cannot change.
- **Two tag rulesets:**
  - one blocks updating or deleting `refs/tags/v*`, with no bypass;
  - one restricts creating `refs/tags/v*`, with Repository admin as its only bypass.
  
  A ruleset's bypass applies to all its rules, which is why there are two. The release workflow never creates tags.

### 5. Releases and buildergarage.app

- **GitHub Releases** distribute the executable archives, with their signed checksum manifest and attestations.
- **[buildergarage.app](https://buildergarage.app)** remains the channel for individual skills and manuals, built from a pinned source revision it displays.
- **Release notes** state the tag and the tagged commit. That commit is also the revision of the embedded skills and manuals, so a user can compare it with the site's "Source revision".

### 6. Release inventory

The files a release publishes are its **release inventory**: a versioned, explicit list. [`release-inventory.sh`](../../.github/scripts/release-inventory.sh) defines every inventory, and every check reads it, so no second list exists.

| Inventory | Releases | Files |
| --- | --- | --- |
| v1 | to `v0.1.x` | Six archives, `garage_<version>_checksums.txt`, and its `.sigstore.json` bundle (8 files). |
| v2 | from `v0.2.0` | v1, plus `garage_<version>_linux_amd64.deb`, `garage_<version>_linux_arm64.deb`, `garage-<version>-1.x86_64.rpm`, and `garage-<version>-1.aarch64.rpm` (12 files). |

- **The inventory is selected by version.** [`verify-release.sh`](../../.github/scripts/verify-release.sh) verifies any published release against its own inventory. A snapshot uses its base version's inventory.
- **Enforcement:**
  - `build` and pull-request CI fail unless `dist` holds exactly the inventory's archives and packages, and the manifest lists exactly them;
  - `publish` creates the release with exactly the verified files;
  - pull-request CI validates the names on every change, before any immutable release.
- **Adding to it.** A later `.pkg` or `.msi` needs an explicit inventory amendment, recorded here, before it is published.
- **Linux packages.**
  - A package installs only `/usr/bin/garage` and its license file (`/usr/share/doc/garage/copyright` for `.deb`, `/usr/share/licenses/garage/LICENSE` for `.rpm`).
  - It has no maintainer scripts, configuration files, or dependencies.
  - `apt`, `dpkg`, `dnf`, and `rpm` own upgrades and removal.
  - Installing, upgrading, or removing it never creates, alters, or removes project configuration, installed skills, manuals, local history, or other user data.
  - [`check-release-package.sh`](../../.github/scripts/check-release-package.sh) checks all of this on both architectures, with sentinel user data, in pull-request CI (with an upgrade) and before every release is signed.

## Consequences

- **A tag that fails cannot be reused.** A defect found after a tag exists ships under the next tag, because protected tags are never moved. A release-mode run that fails after signing leaves a public signature for that tag.
- **The release workflow can run only from a tag that contains it.** GitHub runs a dispatched workflow as it exists at the dispatched ref, so the first release follows the merge of this design.
- **The asset names and the workflow path are a public contract.** Verification commands depend on them.
- **The first releases are not platform-signed.** `v0.1.1` and later releases publish unsigned macOS and Windows executables until #122 is complete, with the platform-trust disclosure in their notes. macOS `.pkg` and Windows `.msi` packages (#123), package-manager channels (#124), and any OS-native installer (#125) wait for #122.
- **The first package release must be `v0.2.0`.** A `v0.1.x` tag selects inventory v1, which has no packages, so a build that produces them fails its checks before anything is signed.
- **Package names are public contract.** The package name `garage` and the path `/usr/bin/garage` carry upgrades between releases.

## Still undecided

- Platform-signing tooling and the Windows signing option (#122).
- macOS `.pkg` and Windows `.msi` packaging readiness, including the WiX license review (#123); package-manager channels (#124); and release installers and update (#125).
- When a release is cut, and what moves the version (as in ADR 0002).
