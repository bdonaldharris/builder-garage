# Architectural Direction

**Status:** Approved direction. The initial CLI foundation described below is implemented; everything explicitly described as planned or possible remains unimplemented.

This document records the architectural direction approved for Builder Garage. It states direction, not design. It deliberately leaves detailed questions open so that they are answered with evidence, within the lifecycle of the change that needs the answer.

## Identity

Builder Garage is an open-source, project-embedded, extensible agentic engineering framework.

## Direction

- **Project-embedded.** Builder Garage is meant to operate from inside the project it serves, alongside that project's repository, issues, and pull requests, rather than as a separate hosted service. The current operating model already works this way: skills are used in the project's workspace, and the project's GitHub issues and pull requests hold the durable workflow record.
- **CLI-first.** The `garage` command-line interface is the primary interface. Its initial foundation provides help, version reporting, project configuration initialization and validation, and private local event recording; broader capabilities remain future work.
- **Extensible.** Projects that adopt Builder Garage are expected to adapt and extend it. They may adapt it only at the extension points declared in the [extension-point registry](extension-points.md), and never below the [responsibility floor](responsibility-floor.md). A project records those choices in `.garage/config.yaml`, which `garage init` creates and `garage config validate` checks; see [Project configuration](configuration.md).
- **Room to grow.** Libraries, adapters, and potentially independent or distributed execution may be supported in the future. These are possibilities the direction keeps open, not commitments.
- **Single repository.** The project lives in one repository initially.
- **Evolutionary.** The architecture is expected to change as the project learns. See [How the architecture evolves](#how-the-architecture-evolves).

## Conceptual components

Builder Garage is organized around five conceptual components. Each has a directory that documents its responsibility.

| Component | Responsibility | Question it answers |
| --- | --- | --- |
| [Workbench](../workbench/) | Reusable engineering skills, workflows, procedures, and tools | How is the work done? |
| [Crew](../crew/) | Distinct engineering agents with explicit responsibilities and handoff boundaries | Who does the work, and where does each role stop? |
| [Inspection](../inspection/) | Independent reviews, tests, acceptance verification, and quality gates | Is the work correct? |
| [Shipping](../shipping/) | Pull request integration, release readiness, deployment requirements, and promotion | How does accepted work reach its destination? |
| [Manuals](../manuals/) | Maintained technical guidance for designing and building with a particular technology | How should this technology be designed and built? |

These are conceptual boundaries. Whether they later become package, module, or command boundaries is undecided.

Manuals are guidance that a project adopts explicitly. They are not configuration, and adopting one does not make it a requirement; see [Manuals](../manuals/README.md#how-manuals-are-used). A project selects manuals at extension point [E10](extension-points.md#e10-manuals) of the extension-point registry.

## Current repository organization

| Path | Contents |
| --- | --- |
| [`skills/`](../skills/) | The canonical engineering skills. They remain at this path to preserve existing installation paths, references, and invocation contracts. Its `embed.go` embeds them in the `garage` binary ([Skill installation](skill-installation.md)). |
| [`workbench/`](../workbench/), [`crew/`](../crew/), [`inspection/`](../inspection/), [`shipping/`](../shipping/) | Documentation of each conceptual component's responsibility and its governing skills. |
| [`manuals/`](../manuals/) | Documentation of the Manuals component, and the Garage-maintained baseline manuals, beginning with the [Go Manual](../manuals/go.md). Its `embed.go` embeds them in the `garage` binary, which delivers adopted manuals as skills ([Skill installation](skill-installation.md#manuals)). |
| [`docs/`](./) | Architectural direction, the [responsibility floor](responsibility-floor.md) and [extension-point registry](extension-points.md), the [project configuration](configuration.md) and [tooling configuration](tooling-configuration.md) formats, [engineering principles](principles.md), [project history](history.md), Builder Garage's adoption profiles for the [canonical skills](skill-adoption.md) and the [Go Manual](go-manual.md), the [revision-identifiable review evidence](review-evidence.md) design, and repository configuration notes. |
| [`cmd/garage/`](../cmd/garage/) | Entry point for the `garage` executable and its build-time version value. |
| [`internal/cli/`](../internal/cli/) | Cobra command wiring, project configuration initialization and validation, local event recording, and skill installation and verification. It does not implement the conceptual components. |
| [`docs/adr/`](adr/) | Accepted architectural decision records, beginning with the [Go and Cobra CLI foundation](adr/0001-go-cobra-cli-foundation.md). |
| [`.github/`](../.github/) | Issue and pull request templates, and the pull-request validation workflow and its source-installer test. |
| Repository root | README, license, governance, contribution, conduct, and security documents, and the source installer, [`install.sh`](../install.sh). |

The repository now contains the initial Go CLI source, tests, build commands, and pull-request validation alongside its documentation. The Workbench, Crew, Inspection, Shipping, and Manuals directories remain documentation of conceptual boundaries, not implemented packages or runtime components.

## Deliberately undecided

The following remain deliberately undecided:

- commands and behavior beyond `garage --help`, `garage version`, `garage init`, `garage config validate`, `garage record`, `garage skills install` and `verify`, and `garage status`;
- release channels for compiled executables, and installing a published release. Installing from source is decided: on macOS and Linux, [`install.sh`](../install.sh) builds `garage` from a checkout and installs it for the current user. Release installation is deferred until release automation exists, and a Windows installer is a separate follow-up ([#69 decision](https://github.com/bdonaldharris/builder-garage/issues/69#issuecomment-6867848834));
- package and module layout for future conceptual-component implementations;
- adapter and extension interfaces;
- the execution model for independent or distributed execution.

Go, Cobra, Go modules, YAML, standard-library testing, standalone executables, GitHub Actions, and future GoReleaser use are decided in [ADR 0001](adr/0001-go-cobra-cli-foundation.md). `garage status` reads GitHub through the external `gh` command, as the [#19 decision](https://github.com/bdonaldharris/builder-garage/issues/19#issuecomment-6837362562) chose. No additional technology is introduced until an established requirement calls for it.

## How the architecture evolves

Architectural decisions are made through the project's normal [contribution process](../CONTRIBUTING.md):

1. a change that needs a design decision starts from an issue;
2. where the answer is not yet known, discovery is performed and its findings are documented in that issue;
3. the decision is implemented, independently reviewed, and approved like any other substantive change;
4. this document is updated when an approved change alters the direction recorded here.

This follows the discipline recorded in the project's [history](history.md): understand what the evidence supports before turning it into architecture. Approved direction recorded here takes precedence over any historical hypothesis or candidate principle.
