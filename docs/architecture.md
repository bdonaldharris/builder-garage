# Architectural Direction

**Status:** Approved direction. Nothing described here as planned or possible has been implemented.

This document records the architectural direction approved for Builder Garage. It states direction, not design. It deliberately leaves detailed questions open so that they are answered with evidence, within the lifecycle of the change that needs the answer.

## Identity

Builder Garage is an open-source, project-embedded, extensible agentic engineering framework.

## Direction

- **Project-embedded.** Builder Garage is meant to operate from inside the project it serves, alongside that project's repository, issues, and pull requests, rather than as a separate hosted service. The current operating model already works this way: skills are used in the project's workspace, and the project's GitHub issues and pull requests hold the durable workflow record.
- **CLI-first.** A command-line interface is the planned primary interface. It does not exist yet.
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
| [`.github/`](../.github/) | Issue and pull request templates. |
| Repository root | README, license, governance, contribution, conduct, and security documents. |

The repository contains documentation only. There is no source code, build, or runtime.

## Deliberately undecided

The following have not been decided, and this document does not decide them:

- the technology stack, including implementation language and distribution format;
- the CLI's commands and behavior;
- how Builder Garage is installed into, or embedded within, a project;
- package and module layout;
- adapter and extension interfaces;
- the execution model for independent or distributed execution.

No technology is introduced until an established requirement calls for it.

## How the architecture evolves

Architectural decisions are made through the project's normal [contribution process](../CONTRIBUTING.md):

1. a change that needs a design decision starts from an issue;
2. where the answer is not yet known, discovery is performed and its findings are documented in that issue;
3. the decision is implemented, independently reviewed, and approved like any other substantive change;
4. this document is updated when an approved change alters the direction recorded here.

This follows the discipline the project's [discovery notes](excavation/) were written under: understand what the evidence supports before turning it into architecture. Those notes predate the Builder Garage name and refer to the project by its earlier working name, ABES.
