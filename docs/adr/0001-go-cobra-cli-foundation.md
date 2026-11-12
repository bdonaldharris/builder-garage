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
| Release automation | GoReleaser, in the future |

## What the CLI foundation implements

- A Go module, `github.com/bdonaldharris/builder-garage`, requiring Go 1.27.4, the stable release the foundation was built and validated with. `go.mod` is the single source of the Go version; the CI workflow reads it from there.
- Direct dependencies: `github.com/spf13/cobra` v1.10.2 for the CLI, and `go.yaml.in/yaml/v3` v3.0.6 for YAML. The YAML module is the one Cobra itself requires, so it adds no module to the build beyond what Cobra already brings.
- A `garage` executable with `--help`, `version`, and `init`. See the [README](../../README.md#the-garage-cli) for its behavior.
- Project configuration at `.garage/config.yaml`, containing only `schema_version`.
- Tests written with the standard `testing` package only.
- Executables built with CGO disabled, so they need no Go installation at run time, cross-compiled for macOS, Linux, and Windows on amd64 and arm64.
- A GitHub Actions workflow that validates pull requests.

## What is approved but not implemented

- **GoReleaser.** No GoReleaser configuration exists. No release has been built, signed, or published, and the workflow publishes nothing. The version variable is `main.version`, which GoReleaser sets by default, so adopting it later does not require changing the CLI.

## Implementation choices made within this decision

These follow from the decision above and can be revisited through the normal [contribution process](../../CONTRIBUTING.md) without superseding this record.

- **Layout.** `cmd/garage` holds the entry point and `internal/cli` holds command wiring. The conceptual components (Workbench, Crew, Inspection, Shipping) are not represented in code.
- **Version.** `garage version` reports `dev` unless a version is supplied at link time with `-ldflags "-X main.version=<version>"`. No other build metadata is recorded.
- **Configuration protection.** `garage init` never replaces an existing configuration file unless `--force` is given.
- **Command set.** Cobra's generated shell-completion command is disabled so that the CLI exposes only commands the project has deliberately implemented.
- **Build commands.** A `Makefile` wraps the plain Go commands for building, testing, format checking, static analysis, and cross-compilation.

## Consequences

- Building from source requires a Go toolchain. Running a compiled executable does not.
- Go's and Cobra's conventions now shape how commands are added and tested.
- The pull-request workflow produces the repository's first status checks. Whether any becomes a required check is a repository-settings decision for the lead maintainer; see [repository-configuration.md](../repository-configuration.md#required-checks). No repository setting was changed.
- [docs/architecture.md](../architecture.md) no longer lists the technology stack as undecided.

## Still undecided

This record does not decide:

- commands and behavior beyond `--help`, `version`, and `init`;
- configuration content beyond `schema_version`;
- how releases are versioned, built, signed, and published;
- whether the conceptual components become packages, modules, or commands;
- adapter and extension interfaces, and the execution model.
