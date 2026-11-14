# ADR 0002: Semantic Versioning for garage, starting at v0.1.0

- **Status:** Accepted
- **Date:** 2026-11-14 (UTC)
- **Decision authority:** Lead maintainer, as Workflow Owner, under [GOVERNANCE.md](../../GOVERNANCE.md). Recorded in [#115](https://github.com/bdonaldharris/builder-garage/issues/115).

## Context

Builder Garage is a public, MIT-licensed repository with an open contributor backlog. Every `garage` build nevertheless reports `garage version dev`, and the repository has no tags. Nothing tells a contributor or user which version of the CLI they are running, or what that version promises.

[ADR 0001](0001-go-cobra-cli-foundation.md) left "how releases are versioned, built, signed, and published" undecided. Under the [#98 decision](https://github.com/bdonaldharris/builder-garage/issues/98#issuecomment-6873371683), item 5, a lasting architectural or technology decision is recorded as an ADR. This record decides how `garage` is versioned. It does not decide how releases are built, signed, or published.

When this decision was made:

- `garage version` printed a link-time value, `-X main.version`, which defaults to `dev` in `cmd/garage/main.go`, the `Makefile`, and `install.sh`.
- Installed skills record the build's Git commit as their revision, so skill integrity does not depend on the version string. The skill manifest's `garage_version` is informational.
- The configuration schema, the [responsibility floor](../responsibility-floor.md), the [extension-point registry](../extension-points.md), and each [manual](../../manuals/README.md) carry their own versions.

## Decision

| Area | Decision |
| --- | --- |
| Scheme | [Semantic Versioning](https://semver.org/) |
| First version | `v0.1.0` |
| Tags | `v`-prefixed, for example `v0.1.0` |
| Publication | A tag only. No GitHub Release, not even one with notes only. |
| Default build version | Derived from Git tags. An explicit `VERSION` overrides it. |
| Compatibility before 1.0 | Configuration migration is promised; broad compatibility is not. |

### 1. Semantic Versioning, starting at `v0.1.0`

The CLI is real and usable, but its public configuration and delivery contracts are still taking shape. A pre-1.0 version is the honest signal.

### 2. A tag only

The first tag establishes identity and versioning without implying downloadable release artifacts. Releases are published when release automation and signed assets exist. Release channels, packaging, and signing remain deferred ([#69 decision](https://github.com/bdonaldharris/builder-garage/issues/69#issuecomment-6867848834); [architecture.md](../architecture.md#deliberately-undecided)).

### 3. The default version is derived from Git tags

An explicit `VERSION` remains an override, so release builds stay reproducible. Without one, a build reports what its checkout actually is:

- a tagged checkout reports the tag, for example `v0.1.0`;
- later commits use Git's descriptive form;
- a build with local modifications is visibly marked;
- a build without Git metadata reports `dev`.

This keeps source installation truthful.

### 4. Configuration migration is promised; broad pre-1.0 compatibility is not

Before 1.0, Garage does not promise stable commands, skill contracts, or configuration semantics.

Every configuration-schema change must provide an in-place `garage init` upgrade path. That path:

- preserves existing human choices;
- validates the result;
- never silently discards configuration.

## Consequences

- **The CLI version is independent.** It does not version, and is not tied to, the configuration schema, the responsibility floor, the extension-point registry, the manuals, or the skills. Each keeps its own version or revision.
- **A pre-1.0 minor version may break things.** Under Semantic Versioning, a `0.y.z` version may change commands, skill contracts, or configuration semantics incompatibly, including in a minor version. Configuration upgrades remain migrated in place, as decision 4 requires.
- **Deriving the version is separate work.** Implementing decision 3 in the `Makefile` and `install.sh`, and documenting it, is tracked separately. Until that lands, builds still report `dev` unless `VERSION` is set.
- **The first tag waits for that work.** `v0.1.0` is created only after the derivation is merged, so that a checkout of the tag reports `v0.1.0`.
- **Future release tooling fits.** `main.version` remains the link-time variable, which GoReleaser sets by default ([ADR 0001](0001-go-cobra-cli-foundation.md#what-is-approved-but-not-implemented)).

## Still undecided

This record does not decide:

- release automation, including GoReleaser configuration;
- release channels, packaging, and published installation (deferred under [#69](https://github.com/bdonaldharris/builder-garage/issues/69#issuecomment-6867848834));
- signing;
- when a release is cut, and what moves the version;
- the criteria for 1.0.
