# ADR 0003: Release distribution: signed, attested archives from a tag-dispatched workflow

- **Status:** Accepted
- **Date:** 2026-11-16 (UTC)
- **Decision authority:** Lead maintainer, as Workflow Owner, under [GOVERNANCE.md](../../GOVERNANCE.md). Recorded in:
  - [#120](https://github.com/bdonaldharris/builder-garage/issues/120), its [correction](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6930428330), and its [amendment to decision 5](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6935475192);
  - [#121's decisions](https://github.com/bdonaldharris/builder-garage/issues/121#issuecomment-6935474831), and those recorded in [#121's readiness](https://github.com/bdonaldharris/builder-garage/issues/121#issuecomment-6935931327).

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
| Formats | Archives first: `.tar.gz` for macOS and Linux, `.zip` for Windows, each holding the executable, `LICENSE`, and `README.md`. Native packages and package-manager channels are later tracks. |
| Platforms | macOS, Linux, and Windows, each on amd64 and arm64. |
| Assets | Exactly the six archives, a SHA-256 checksum manifest, and its Sigstore bundle. No skills or manuals bundle. |
| Integrity | The manifest is signed with keyless Sigstore/cosign. Every archive also gets a GitHub artifact attestation, as additional provenance. |
| Verification identity | `https://github.com/bdonaldharris/builder-garage/.github/workflows/release.yml@refs/tags/<tag>`, with issuer `https://token.actions.githubusercontent.com`. |
| Platform trust | macOS signing and notarization, and Windows Authenticode signing, are required before any release is published (#122). |
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
| `build` | `contents: read`, in the `release-signing` environment | Refuses release mode without platform-signing credentials. Builds, archives, checksums, and validates the archives and executables. Runs the linux/amd64 archive. |
| `run-check` | `contents: read` | Release mode only. Runs every archive on its own platform: `garage version` reports the tag, and `garage skills install` installs skills whose revision is the tagged commit. |
| `sign` | `id-token: write` | Signs the manifest with keyless cosign. |
| `attest` | `id-token: write`, `attestations: write` | Attests every archive listed in the manifest. |
| `verify` | `contents: read`, `attestations: read` | Verifies the signature (pinned identity, and rejection of another identity), the checksums, and every attestation. |
| `publish` | `contents: write`, in the `release` environment | After the Workflow Owner approves, creates a draft with exactly the verified assets, checks it, and publishes it. |
| `post-verify` | `contents: read`, `attestations: read` | Downloads the published assets and verifies them again. |

- **Dry runs.** `dry-run` mode stops after `build`. It signs, attests, uploads, and publishes nothing, so no public trust material exists for artifacts not meant for distribution. Pull-request CI also builds a GoReleaser snapshot on every pull request, read-only and without secrets.
- **Platform signing.** It happens in a GoReleaser post-build hook ([`platform-sign.sh`](../../.github/scripts/platform-sign.sh)), before archiving, so the checksums, signature, and attestations cover signed executables. Until #122 implements it, release mode fails closed twice: in the credentials check, and in the hook.
- **`artifact-metadata: write`** is not granted. `actions/attest` needs it only for storage records, which apply only when pushing to a registry.

### 4. Repository protection

- **Release environments.**
  - The `release` environment requires the Workflow Owner's review, allows only `v*` tags, and does not let administrators bypass its rules.
  - The `release-signing` environment holds the platform-signing credentials, and allows only `v*` tags.
- **Immutable releases** are enabled before the first release, so a published release's assets and tag cannot change.
- **Two tag rulesets:**
  - one blocks updating or deleting `refs/tags/v*`, with no bypass;
  - one restricts creating `refs/tags/v*`, with Repository admin as its only bypass.
  
  A ruleset's bypass applies to all its rules, which is why there are two. The release workflow never creates tags.

### 5. Releases and buildergarage.app

- **GitHub Releases** distribute the signed and attested executables.
- **[buildergarage.app](https://buildergarage.app)** remains the channel for individual skills and manuals, built from a pinned source revision it displays.
- **Release notes** state the tag and the tagged commit. That commit is also the revision of the embedded skills and manuals, so a user can compare it with the site's "Source revision".

## Consequences

- **A tag that fails cannot be reused.** A defect found after a tag exists ships under the next tag, because protected tags are never moved. A release-mode run that fails after signing leaves a public signature for that tag.
- **The release workflow can run only from a tag that contains it.** GitHub runs a dispatched workflow as it exists at the dispatched ref, so the first release follows the merge of this design.
- **The asset names and the workflow path are a public contract.** Verification commands depend on them.
- **`v0.1.1` waits for #122.** Until platform signing is implemented and its credentials are configured, release mode refuses to run past the guard.

## Still undecided

- Platform-signing tooling and the Windows signing option (#122).
- Native packages (#123), package-manager channels (#124), and release installers and update (#125).
- When a release is cut, and what moves the version (as in ADR 0002).
