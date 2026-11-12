# Builder Garage Go Manual

Builder Garage adopts the Garage-maintained [Go Manual](../manuals/go.md), **version 0.1 (provisional)**, as its default Go guidance.

This profile records only the choices specific to Builder Garage. Everything else follows the baseline, which this profile does not repeat. What adoption means, including that a departure is not by itself a blocking review finding, is described in [Manuals](../manuals/README.md#how-manuals-are-used).

## Project deltas

Each row links to the source that owns the choice. Where a row and its source differ, the source is authoritative.

| Area | Builder Garage choice | Source |
| --- | --- | --- |
| Technology | Go, Cobra, Go modules, YAML, the standard `testing` package, standalone executables, and GitHub Actions. The Go version is declared once, in `go.mod`. | [ADR 0001](adr/0001-go-cobra-cli-foundation.md#decision), [`go.mod`](../go.mod) |
| CLI foundation | A single `garage` executable. `cmd/garage` is the entry point and holds the link-time `main.version` value. `internal/cli` holds the Cobra command wiring. | [ADR 0001: Implementation choices](adr/0001-go-cobra-cli-foundation.md#implementation-choices-made-within-this-decision), [README: The `garage` CLI](../README.md#the-garage-cli) |
| Command wiring and behavior | Initialization behavior shares the `internal/cli` package with the command wiring. It is callable without argument parsing, through `initProject`. A separate package is deferred until a capability needs one, as the baseline's package guidance advises. | [`internal/cli/init.go`](../internal/cli/init.go) |
| Project layout | The conceptual components, including Manuals, are not represented in code. Package and module layout for any future implementation is deliberately undecided. | [Architectural direction: Deliberately undecided](architecture.md#deliberately-undecided) |
| Project configuration | `garage init` creates `.garage/config.yaml`, containing only `schema_version`. It never replaces an existing file unless `--force` is given, and a forced write replaces the file through a temporary file in the same directory. Configuration beyond the schema version is undecided. | [README: Capabilities](../README.md#capabilities), [`internal/cli/init.go`](../internal/cli/init.go), [#16](https://github.com/bdonaldharris/builder-garage/issues/16) |
| Build and validation | Executables are built with CGO disabled and cross-compiled for macOS, Linux, and Windows on amd64 and arm64. `make check` runs the local checks. Pull requests run formatting, static analysis, build, and cross-compilation checks, with tests on Linux, macOS, and Windows. | [`Makefile`](../Makefile), [CI workflow](../.github/workflows/ci.yml) |
| Durable evidence | GitHub Issue and pull request records are the durable collaborative evidence of workflow state. Go code should not treat local files or local history as authoritative workflow or review evidence. | [README: Operating model](../README.md#operating-model), [GOVERNANCE: Review requirements](../GOVERNANCE.md#review-requirements), [#6 decision comment](https://github.com/bdonaldharris/builder-garage/issues/6#issuecomment-6817083027) (item 9) |
| No orchestration | Builder Garage does not build an orchestration engine. Agents run in their own harnesses. Garage's role is to define and distribute contracts and to validate project configuration and available evidence. Go code should not assign, schedule, or run agents. | [#6 decision comment](https://github.com/bdonaldharris/builder-garage/issues/6#issuecomment-6817083027) (item 8), a direction record, not an implemented specification |

## Baseline guidance not yet exercised

The current code has no operations that block on an external system, and it defines no interfaces. The baseline's guidance on [contexts](../manuals/go.md#contexts), [interfaces](../manuals/go.md#interfaces), and network-facing [external boundaries](../manuals/go.md#external-boundaries) applies when the code gains such operations. Until then, this profile records no delta for them.

## Changing this profile

This profile changes through the normal [contribution process](../CONTRIBUTING.md). Adopting a later baseline version means updating the version above and reviewing the deltas against it.
