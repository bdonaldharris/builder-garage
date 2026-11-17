# ADR 0001: Go and Cobra for the CLI foundation

- **Status:** Accepted
- **Date:** 2026-11-12 (UTC)
- **Decision authority:** Lead maintainer, under [GOVERNANCE.md](../../GOVERNANCE.md)

## Context

The [architectural direction](../architecture.md) establishes Builder Garage as project-embedded and CLI-first, and until this decision it left the technology stack, including implementation language and distribution format, deliberately undecided.

Building the first executable required those choices to be made. The lead maintainer approved the technical decisions below as inputs to the change that introduced the CLI foundation. This record documents them; it does not add to them.

## Decision

| Area | Decision |
| --- | --- |
| Implementation language | Go |
| CLI framework | [Cobra](https://github.com/spf13/cobra) |
| Dependency management | Go modules |
| Configuration format | YAML |
| Testing | Go's standard `testing` package |
| Distribution | Standalone compiled executables |
| Continuous integration | GitHub Actions |
| Release automation | GoReleaser (configured under [ADR 0003](0003-release-distribution.md)) |

## What the CLI foundation implements

- A Go module, `github.com/bdonaldharris/builder-garage`, requiring Go 1.27.4, the stable release the foundation was built and validated with. `go.mod` is the single source of the Go version; the CI workflow reads it from there.
- Direct dependencies: `github.com/spf13/cobra` v1.10.2 for the CLI, and `go.yaml.in/yaml/v3` v3.0.6 for YAML. The YAML module is the one Cobra itself requires, so it adds no module to the build beyond what Cobra already brings.
- A `garage` executable with `--help`, `version`, and `init`. See the [README](../../README.md#the-garage-cli) for its behavior.
- Project configuration at `.garage/config.yaml`, containing only `schema_version`.
- Tests written with the standard `testing` package only.
- Executables built with CGO disabled, so they need no Go installation at run time, cross-compiled for macOS, Linux, and Windows on amd64 and arm64.
- A GitHub Actions workflow that validates pull requests.

## What is approved but not implemented

- **GoReleaser.** When this record was accepted, no GoReleaser configuration existed, and no release had been built, signed, or published. It was expected that GoReleaser would set the version variable, `main.version`, by default. That holds for the variable, but not for the version's form: GoReleaser's default sets `{{ .Version }}`, which drops the leading `v`. The release configuration therefore sets `-X main.version={{ .Tag }}` explicitly, so a release reports exactly its tag, for example `v0.1.1`. GoReleaser is now configured, and releases are designed, under [ADR 0003](0003-release-distribution.md). The CLI did not change.

## Implementation choices made within this decision

These follow from the decision above and can be revisited through the normal [contribution process](../../CONTRIBUTING.md) without superseding this record.

- **Layout.** `cmd/garage` holds the entry point and `internal/cli` holds command wiring. The conceptual components (Workbench, Crew, Inspection, Shipping) are not represented in code.
- **Version.** `garage version` originally reported `dev` unless a version was supplied at link time with `-ldflags "-X main.version=<version>"`. Revised under [#117](https://github.com/bdonaldharris/builder-garage/issues/117), following [ADR 0002](0002-semantic-versioning.md): `make build` and `install.sh` supply a version derived from the checkout's Git tags unless `VERSION` overrides it, and a plain `go build` still reports `dev`. See the [README](../../README.md#versions). The link-time variable is unchanged, and no other build metadata is recorded.
- **Configuration protection.** `garage init` originally never replaced an existing configuration file unless `--force` was given. Revised under [#35](https://github.com/bdonaldharris/builder-garage/issues/35): `--force` is removed, and `init` upgrades an existing configuration in place without discarding any configured value. See [Project configuration](../configuration.md).
- **Command set.** Cobra's generated shell-completion command is disabled so that the CLI exposes only commands the project has deliberately implemented.
- **GitHub access.** Added under [#40](https://github.com/bdonaldharris/builder-garage/issues/40): `garage status` runs the external `gh` command, as the [#19 decision](https://github.com/bdonaldharris/builder-garage/issues/19#issuecomment-6837362562) chose. Garage adds no Go dependency for it and stores no token.
- **Build commands.** A `Makefile` wraps the plain Go commands for building, testing, format checking, static analysis, and cross-compilation.

## Consequences

- Building from source requires a Go toolchain. Running a compiled executable does not.
- Go's and Cobra's conventions now shape how commands are added and tested.
- The pull-request workflow produces the repository's first status checks. Whether any becomes a required check is a repository-settings decision for the lead maintainer; see [repository-configuration.md](../repository-configuration.md#required-checks). No repository setting was changed.
- [docs/architecture.md](../architecture.md) no longer lists the technology stack as undecided.

## Still undecided

This record does not decide:

- commands and behavior beyond `--help`, `version`, and `init`;
- configuration content beyond `schema_version` (since decided under #35; see [Project configuration](../configuration.md));
- how releases are versioned (since decided under [ADR 0002](0002-semantic-versioning.md)), built, signed, and published;
- whether the conceptual components become packages, modules, or commands;
- adapter and extension interfaces, and the execution model.
