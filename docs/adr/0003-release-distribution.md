# ADR 0003: Release distribution: attested archives with signed checksums, from a tag-dispatched workflow

- **Status:** Accepted
- **Date:** 2026-11-16 (UTC)
- **Decision authority:** Lead maintainer, as Workflow Owner, under [GOVERNANCE.md](../../GOVERNANCE.md). Recorded in:
  - [#120](https://github.com/bdonaldharris/builder-garage/issues/120), its [correction](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6930428330), its [amendment to decision 5](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6935475192), and its [amendment to decision 4](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6938614931) (2026-11-17);
  - [#121's decisions](https://github.com/bdonaldharris/builder-garage/issues/121#issuecomment-6935474831), and those recorded in its readiness reports, [the first](https://github.com/bdonaldharris/builder-garage/issues/121#issuecomment-6935931327) and [the follow-up after the decision 4 amendment](https://github.com/bdonaldharris/builder-garage/issues/121#issuecomment-6938700447);
  - [#123's decisions](https://github.com/bdonaldharris/builder-garage/issues/123#issuecomment-6952041049), and those recorded in [its readiness](https://github.com/bdonaldharris/builder-garage/issues/123#issuecomment-6952505748) (2026-11-17): Linux packages and the release inventory;
  - [#124's decisions](https://github.com/bdonaldharris/builder-garage/issues/124#issuecomment-6959529918), and those recorded in [its readiness](https://github.com/bdonaldharris/builder-garage/issues/124#issuecomment-6959615104) (2026-11-17): package-manager channels, starting with the Homebrew cask on Linux;
  - #120's [amendment to decision 4 for npm](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-7027245596), and the decisions recorded in [#158's readiness](https://github.com/bdonaldharris/builder-garage/issues/158#issuecomment-7027540094) (2026-11-19): the npm channel.

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
| Formats | Archives: `.tar.gz` for macOS and Linux, `.zip` for Windows, each holding the executable, `LICENSE`, and `README.md`. From `v0.2.0`, Linux `.deb` and `.rpm` packages, built with GoReleaser's free nfpm support (#123). macOS `.pkg` (`pkgbuild` and `productsign` on a macOS runner) and Windows `.msi` (WiX, if its license is compatible with MIT distribution) follow their platform-trust prerequisites. GoReleaser Pro is not used. Package-manager channels: see [Package-manager channels](#7-package-manager-channels). |
| Platforms | macOS, Linux, and Windows, each on amd64 and arm64. |
| Assets | Exactly the release's inventory (see [Release inventory](#6-release-inventory)). Inventory v1 (to `v0.1.x`) is the six archives, a SHA-256 checksum manifest, and its Sigstore bundle. Inventory v2 (from `v0.2.0`) adds the four Linux packages. No skills or manuals bundle. |
| Integrity | The manifest is signed with keyless Sigstore/cosign. Every archive and package also gets a GitHub artifact attestation, as additional provenance. |
| Verification identity | `https://github.com/bdonaldharris/builder-garage/.github/workflows/release.yml@refs/tags/<tag>`, with issuer `https://token.actions.githubusercontent.com`. |
| Platform trust | macOS signing and notarization, and Windows Authenticode signing (#122), are required before platform-native installers, packages, or package-manager channels are introduced. They are not required for the archive release. #122 ships one platform at a time: macOS first, then Windows. Until a platform is signed, release notes and verification docs disclose that its executables are not platform-signed. For the direct-download Linux `.deb` and `.rpm` packages, the signed checksum manifest and attestations are their platform trust: there is no PGP package-signing key, and the docs explain verifying a package before installing it. |
| Trigger | Manual dispatch of the release workflow, from an existing `vMAJOR.MINOR.PATCH` tag whose commit is on `main` only. |
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

[`.github/workflows/release.yml`](../../.github/workflows/release.yml) is dispatched with a `mode`. It refuses any ref but a version tag, a tag whose commit is not reachable from `main`, and an unknown mode, before building anything. `main` is the public released-code branch ([#120 decision](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6962490422), item 8), so a release is built only from a commit on it (#138). The check runs in both modes, and refuses when it cannot be decided.

| Job | Permissions | Purpose |
| --- | --- | --- |
| `guard` | `contents: read` | Accepts only a `vMAJOR.MINOR.PATCH` tag whose commit is on `main`, and a known mode. |
| `build` | `contents: read`, in the `macos-signing` environment | Builds, archives, packages, checksums, and validates the release inventory's files and the executables from the clean tagged commit. In release mode, signs and notarizes the macOS executables before archiving, with the environment's five secrets (#122); in dry-run mode, it passes them to no step. Runs the linux/amd64 archive. |
| `run-check` | `contents: read` | Release mode only. Runs every archive on its own platform: `garage version` reports the tag, and `garage skills install` installs skills whose revision is the tagged commit. On macOS, it first checks the executable's Developer ID signature, Team ID, notarization, and a quarantined run (#122). |
| `package-check` | `contents: read` | Release mode only, from inventory v2. Installs, checks, and removes each Linux package with its own package manager (`apt` on Ubuntu, `dnf` in a digest-pinned Fedora container), on amd64 and arm64. |
| `sign` | `id-token: write` | Signs the manifest with keyless cosign. |
| `attest` | `id-token: write`, `attestations: write` | Attests every archive and package listed in the manifest. |
| `verify` | `contents: read`, `attestations: read` | Verifies the signature (pinned identity, and rejection of another identity), the checksums, and every attestation. |
| `publish` | `contents: write`, in the `release` environment | After the Workflow Owner approves, creates a draft with exactly the verified assets, checks it, and publishes it. |
| `post-verify` | `contents: read`, `attestations: read` | Downloads the published assets and verifies them again. |
| `channel-publish` | `contents: read`, `attestations: read`, in the `channel-publish` environment | After `post-verify`, re-verifies the published release, checks the GoReleaser-generated Homebrew cask against its manifest, and commits the cask to `bdonaldharris/homebrew-tap` with the tap's own token (see [Package-manager channels](#7-package-manager-channels)). |
| `channel-verify` | `contents: read` | Installs the cask from the tap on amd64 and arm64 Linux, checks `garage`, and uninstalls it. |

- **Dry runs.** `dry-run` mode stops after `build`. It signs, attests, uploads, and publishes nothing, so no public trust material exists for artifacts not meant for distribution. Pull-request CI also builds a GoReleaser snapshot on every pull request, read-only and without secrets.
- **Platform signing** happens after the build and before archiving, so the checksums, signature, and attestations cover signed executables.
  - **macOS (#122).** In release mode, GoReleaser's `notarize.macos` signs the darwin executables with the Developer ID Application identity, with the hardened runtime and a secure timestamp, and notarizes them, in the `build` job, which stays on Linux with `contents: read`.
    - The five credentials are in the `macos-signing` environment: `v*` tags only, no reviewers. They reach only the two steps that need them, in release mode.
    - A release fails closed: [`check-macos-signing-env.sh`](../../.github/scripts/check-macos-signing-env.sh) refuses missing credentials, before GoReleaser and again in the post-build hook ([`platform-sign.sh`](../../.github/scripts/platform-sign.sh)). `run-check` requires Gatekeeper's "Notarized Developer ID" verdict, the signing Team ID, and a quarantined run ([`check-macos-notarization.sh`](../../.github/scripts/check-macos-notarization.sh)) before the manifest is signed.
    - Snapshot and dry-run builds are unsigned.
  - **Windows** executables are not yet Authenticode-signed (#122, phase 2). A release may therefore mix signed macOS and unsigned Windows executables, and its notes say so.
  - An unset or unknown mode still fails the build.
- **Platform-trust disclosure.** The release notes ([`release-notes.sh`](../../.github/scripts/release-notes.sh)) and [Verifying a release](../release-verification.md) state, per platform, that:
  - macOS executables are signed by `Developer ID Application: BRIAN DONALD HARRIS (38397A2FG6)` and notarized, and how to check that;
  - until #122's Windows phase is complete, Windows executables are not yet Authenticode-signed, and SmartScreen may warn;
  - checksum-signature and attestation verification establish integrity and origin, not operating-system publisher trust.

  They recommend downloading, verifying, and extracting from a terminal. They never advise removing quarantine, or bypassing Gatekeeper or SmartScreen.
- **`artifact-metadata: write`** is not granted. `actions/attest` needs it only for storage records, which apply only when pushing to a registry.

### 4. Repository protection

- **Release environments.**
  - The `release` environment requires the Workflow Owner's review, allows only `v*` tags, and does not let administrators bypass its rules.
  - The macOS signing credentials are held in the `macos-signing` environment, available only to the release workflow's `build` job, from `v*` tags (#122).
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

### 7. Package-manager channels

- **Phasing.**
  - The Homebrew cask on **Linux** ships first. The release's signed checksum manifest and attestations are its trust, as for the Linux packages.
  - Homebrew on **macOS** needs notarized macOS executables (#122), because Homebrew quarantines every cask download and macOS kills a quarantined unsigned executable. It is added by its own change (#124).
  - **Scoop and winget** wait for Windows platform trust (#122).
  - **npm** (#158) ships macOS and Linux. Windows is excepted from the platform-trust requirement for npm only ([#120 amendment](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-7027245596)): unsigned Windows executables may be published with a disclosure that never advises disabling a Windows protection. Whether Windows is published at all follows a clean Windows 11 test, recorded on #158; until then, it is not.
- **The cask.**
  - GoReleaser generates it, as `homebrew_casks`, from the release's own Linux archives and checksums. There is no hand-maintained formula.
  - It selects only the Linux archives: the build is defined once and split into Linux and other platforms with a YAML anchor, so asset names do not change. Homebrew on macOS therefore refuses it ("This cask requires Linux.").
  - It is not a release asset, and the release inventory is unchanged.
- **The publication boundary.**
  - Channels are published only after the immutable release has passed `post-verify`, which follows the Workflow Owner's release approval.
  - `channel-publish` first re-verifies the published release, then checks that the cask's downloads are exactly the inventory's Linux archives, with the manifest's checksums ([`check-channel-cask.sh`](../../.github/scripts/check-channel-cask.sh)).
  - **A channel failure does not affect the release.** It is retried by re-running the failed job, within 30 days, against the same release; the generated cask is kept 30 days. After that, the next release updates the channel.
- **npm publishing** is its own workflow, [`npm-publish.yml`](../../.github/workflows/npm-publish.yml), never called from `release.yml`.
  - **Why separate:** npm trusted publishing identifies the publisher by its workflow filename, and needs `id-token: write`. In `release.yml`, that permission would let a job mint a Sigstore certificate as the release workflow, the identity users pin for the checksum manifest. Running from `main`, this workflow's identity is `npm-publish.yml@refs/heads/main`.
  - **When:** it runs after every Release run, whatever its result, and decides from the release itself. The tag must pass `release-guard.sh`, its GitHub release must be published (not a draft or prerelease), and the release must pass `verify-release.sh`. A dry run has no release, so nothing is published. A manual dispatch, from `main`, recovers a failed publication or backfills a release, with the same checks.
  - **What:** [`npm-packages.sh`](../../.github/scripts/npm-packages.sh) checks each archive against the signed manifest, then writes a launcher, `@buildergarage/garage`, and one package per platform, `@buildergarage/garage-<os>-<cpu>`, holding the archive's executable byte for byte. No package has an install script.
  - **How:** [`npm-publish.sh`](../../.github/scripts/npm-publish.sh) publishes the platform packages, then the launcher, with provenance. A version already on npm is compared, not republished, so a partial failure is recovered by running again. [`npm-dist-tag.sh`](../../.github/scripts/npm-dist-tag.sh) never moves `latest` backward: a backfilled older release gets `release-MAJOR.MINOR`.
  - **Credentials:** the `npm-publish` environment allows only `main`, has no reviewers (the Workflow Owner approved the release itself), and holds no secret: trusted publishing uses OIDC. A token is used once, only if npm requires one for the first publish, then deleted.
  - **Verification:** the `verify` job installs from the registry on Linux and macOS, amd64 and arm64, runs `npm audit signatures`, and runs the archive checks through the npm-installed `garage` ([`check-npm-install.sh`](../../.github/scripts/check-npm-install.sh)). Pull-request CI runs the same checks on locally packed packages, including an upgrade.
- **Repositories and credentials.**
  - Each channel has a dedicated, Builder Garage-owned repository: `bdonaldharris/homebrew-tap`, then `bdonaldharris/scoop-bucket` and a fork of `microsoft/winget-pkgs` when those phases begin.
  - Each has its own fine-grained credential, held in the protected `channel-publish` environment. It is never the release workflow's token, and never has access to this repository.
  - The tap and bucket receive direct commits. winget receives a pull request from the fork, and Microsoft's review is its own gate.
- **Names.**
  - Every channel uses the package name `garage`, except npm. There, `garage` belongs to an unrelated package, so npm uses the scoped `@buildergarage/garage` ([#120 amendment](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-7027245596)). The installed command is `garage` everywhere.
  - winget's publisher metadata is "B Donald Harris", describing project ownership. It does not claim an Authenticode publisher identity for unsigned executables.
- **Users trust the tap explicitly** (`brew trust --tap bdonaldharris/tap`), as Homebrew requires. Nothing bypasses tap trust.

## Consequences

- **A tag that fails cannot be reused.** A defect found after a tag exists ships under the next tag, because protected tags are never moved. A release-mode run that fails after signing leaves a public signature for that tag.
- **The release workflow can run only from a tag that contains it.** GitHub runs a dispatched workflow as it exists at the dispatched ref, so the first release follows the merge of this design.
- **The asset names and the workflow path are a public contract.** Verification commands depend on them.
- **The first releases are not platform-signed.** `v0.1.1` and later releases publish unsigned macOS and Windows executables until #122 signs each platform, with the platform-trust disclosure in their notes. macOS is signed and notarized from the first release built after #122's macOS phase. macOS `.pkg` and Windows `.msi` packages (#123), Homebrew on macOS, Scoop, and winget (#124), and any OS-native installer (#125) wait for #122.
- **The first package release must be `v0.2.0`.** A `v0.1.x` tag selects inventory v1, which has no packages, so a build that produces them fails its checks before anything is signed.
- **Package names are public contract.** The package name `garage` and the path `/usr/bin/garage` carry upgrades between releases. So do the tap `bdonaldharris/tap` and its cask `garage`.
- **A failed channel publication leaves a release without its channel update.** Re-run the job within 30 days, or the next release updates the channel.

## Still undecided

- Windows signing (#122, phase 2).
- macOS `.pkg` and Windows `.msi` packaging readiness, including the WiX license review (#123); Homebrew on macOS, Scoop, and winget, including whether a fine-grained credential can open the winget pull request (#124); and release installers and update (#125).
- When a release is cut, and what moves the version (as in ADR 0002).
