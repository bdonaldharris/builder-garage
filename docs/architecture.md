# Architectural Direction

**Status:** Approved direction. The initial CLI foundation described below is implemented; everything explicitly described as planned or possible remains unimplemented.

This document records the architectural direction approved for Builder Garage. It states direction, not design. It deliberately leaves detailed questions open so that they are answered with evidence, within the lifecycle of the change that needs the answer.

## Identity

Builder Garage is an open-source, project-embedded, extensible agentic engineering framework.

## Direction

- **Project-embedded.** Builder Garage is meant to operate from inside the project it serves, alongside that project's repository, issues, and pull requests, rather than as a separate hosted service. The current operating model already works this way: skills are used in the project's workspace, and the project's GitHub issues and pull requests hold the durable workflow record.
- **CLI-first.** The `garage` command-line interface is the primary interface. Its initial foundation provides help, version reporting, and project initialization; broader capabilities remain future work.
- **Extensible.** Projects that adopt Builder Garage are expected to adapt and extend it. The extension mechanisms have not been designed.
- **Room to grow.** Libraries, adapters, and potentially independent or distributed execution may be supported in the future. These are possibilities the direction keeps open, not commitments.
- **Single repository.** The project lives in one repository initially.
- **Evolutionary.** The architecture is expected to change as the project learns. See [How the architecture evolves](#how-the-architecture-evolves).

## Conceptual components

Builder Garage is organized around four conceptual components. Each has a directory that documents its responsibility.

| Component | Responsibility | Question it answers |
| --- | --- | --- |
| [Workbench](../workbench/) | Reusable engineering skills, workflows, procedures, and tools | How is the work done? |
| [Crew](../crew/) | Distinct engineering agents with explicit responsibilities and handoff boundaries | Who does the work, and where does each role stop? |
| [Inspection](../inspection/) | Independent reviews, tests, acceptance verification, and quality gates | Is the work correct? |
| [Shipping](../shipping/) | Pull request integration, release readiness, deployment requirements, and promotion | How does accepted work reach its destination? |

These are conceptual boundaries. Whether they later become package, module, or command boundaries is undecided.

## Current repository organization

| Path | Contents |
| --- | --- |
| [`skills/`](../skills/) | The canonical engineering skills. They remain at this path to preserve existing installation paths, references, and invocation contracts. |
| [`workbench/`](../workbench/), [`crew/`](../crew/), [`inspection/`](../inspection/), [`shipping/`](../shipping/) | Documentation of each conceptual component's responsibility and its governing skills. |
| [`docs/`](./) | Architectural direction, repository configuration notes, and [discovery notes](excavation/). |
| [`cmd/garage/`](../cmd/garage/) | Entry point for the `garage` executable and its build-time version value. |
| [`internal/cli/`](../internal/cli/) | Cobra command wiring and project initialization behavior. It does not implement the conceptual components. |
| [`docs/adr/`](adr/) | Accepted architectural decision records, beginning with the [Go and Cobra CLI foundation](adr/0001-go-cobra-cli-foundation.md). |
| [`.github/`](../.github/) | Issue and pull request templates and pull-request validation workflow. |
| Repository root | README, license, governance, contribution, conduct, and security documents. |

The repository now contains the initial Go CLI source, tests, build commands, and pull-request validation alongside its documentation. The Workbench, Crew, Inspection, and Shipping directories remain documentation of conceptual boundaries, not implemented packages or runtime components.

## Deliberately undecided

The following remain deliberately undecided:

- commands and behavior beyond `garage --help`, `garage version`, and `garage init`;
- configuration beyond the schema version;
- installation and release channels for compiled executables;
- package and module layout for future conceptual-component implementations;
- adapter and extension interfaces;
- the execution model for independent or distributed execution.

Go, Cobra, Go modules, YAML, standard-library testing, standalone executables, GitHub Actions, and future GoReleaser use are decided in [ADR 0001](adr/0001-go-cobra-cli-foundation.md). No additional technology is introduced until an established requirement calls for it.

## How the architecture evolves

Architectural decisions are made through the project's normal [contribution process](../CONTRIBUTING.md):

1. a change that needs a design decision starts from an issue;
2. where the answer is not yet known, discovery is performed and its findings are documented in that issue;
3. the decision is implemented, independently reviewed, and approved like any other substantive change;
4. this document is updated when an approved change alters the direction recorded here.

This follows the discipline the project's [discovery notes](excavation/) were written under: understand what the evidence supports before turning it into architecture. Those notes predate the Builder Garage name and refer to the project by its earlier working name, ABES.
