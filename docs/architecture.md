# Architectural Direction

**Status:** Approved direction. The initial CLI foundation described below is implemented; everything explicitly described as planned or possible remains unimplemented.

This document records the architectural direction approved for Builder Garage. It states direction, not design. It deliberately leaves detailed questions open so that they are answered with evidence, within the lifecycle of the change that needs the answer.

## Identity

Builder Garage is an open-source, project-embedded, extensible agentic engineering framework.

## Direction

- **Project-embedded.** Builder Garage is meant to operate from inside the project it serves, alongside that project's repository, issues, and pull requests, rather than as a separate hosted service. The current operating model already works this way: skills are used in the project's workspace, and the project's GitHub issues and pull requests hold the durable workflow record.
- **CLI-first.** The `garage` command-line interface is the primary interface. Its initial foundation provides help, version reporting, project configuration initialization and validation, private local event recording, and an explicitly started read-only loopback local project view; broader capabilities remain future work.
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
| [`internal/cli/`](../internal/cli/) | Cobra command wiring; initializing, upgrading, validating, showing, and changing project configuration; local event recording and reading; installing and verifying the canonical skills and the manuals a project adopts; the read-only workflow status of an issue or pull request, read from GitHub; an explicitly started read-only loopback local project view; and the release-update check. It does not implement the conceptual components. |
| [`docs/adr/`](adr/) | Accepted architectural decision records, beginning with the [Go and Cobra CLI foundation](adr/0001-go-cobra-cli-foundation.md). |
| [`.github/`](../.github/) | Issue and pull request templates, and the pull-request validation workflow and its source-installer test. |
| Repository root | README, license, governance, contribution, conduct, and security documents, and the source installer, [`install.sh`](../install.sh). |

The repository now contains the initial Go CLI source, tests, build commands, and pull-request validation alongside its documentation. The Workbench, Crew, Inspection, Shipping, and Manuals directories remain documentation of conceptual boundaries, not implemented packages or runtime components.

## Provider boundary

The #197 provider-core slice keeps configuration and status ownership in `internal/cli`. `status_model.go` defines transport-independent workflow targets and evidence facts. The shared CLI/web operation consumes the small `statusProvider` read interface; `status_github_adapter.go` owns the GitHub query budget and explicit normalization of the GraphQL DTOs in `status_github.go`. Report, evidence, kind, and stack evaluation consume normalized facts, without GitHub transport structs or JSON decoding. The existing GitHub evidence rules and read-only query guard remain intact.

Optional schema-v3 `project.forge` selects authoritative provider identity and requires matching origin. Schema-v2 GitHub projects keep their existing behavior without migration. The GitLab adapter uses the configured host's authenticated `glab` REST-v4 boundary for bounded read-only observation. Its typed issue/MR parser, normalized availability/membership facts, and provider-specific report presentation reuse the shared status/evidence operation. Observation refuses write methods and unsupported resources before invoking glab; configuration never sends a GitLab project through GitHub. GitLab writes and qualified authority configuration remain subsequent stack work. This boundary adds no workflow authority, GitLab production infrastructure, or claim of complete GitLab support.

## Deliberately undecided

The following remain deliberately undecided:

- commands and behavior beyond `garage --help`, `garage version`, `garage init`, `garage config validate`, `show`, `get`, `set`, `unset`, `add`, and `remove`, `garage record`, `garage history list` and `path`, `garage skills install` and `verify`, `garage status`, `garage web`, and `garage update check`;
- the remaining release channels and installers. Decided so far:
  - **Installing from source:** on macOS and Linux, [`install.sh`](../install.sh) builds `garage` from a checkout and installs it for the current user.
  - **Installing a published release:** on macOS and Linux, `install.sh` also installs a release, after verifying its cosign-signed checksum manifest, pinned to the release workflow at that tag ([#125](https://github.com/bdonaldharris/builder-garage/issues/125)).
  - **Checking for a newer release:** `garage update check` reports the latest release, read from GitHub only when it is run, and lists the documented update paths. It replaces no binary ([#125](https://github.com/bdonaldharris/builder-garage/issues/125)).
  - **Release archives, signed checksums, attestations, Linux packages, and the Linux Homebrew cask** are decided in [ADR 0003](adr/0003-release-distribution.md).

  The Windows installer, replacing the binary from `garage` itself or updating it in the background, and the platform-native channels that wait for platform signing are later work of [#120](https://github.com/bdonaldharris/builder-garage/issues/120);
- package and module layout for future conceptual-component implementations;
- adapter and extension interfaces;
- the execution model for independent or distributed execution.

Go, Cobra, Go modules, YAML, standard-library testing, standalone executables, GitHub Actions, and GoReleaser are decided in [ADR 0001](adr/0001-go-cobra-cli-foundation.md). Release distribution, including Sigstore/cosign signing and GitHub artifact attestations, is decided in [ADR 0003](adr/0003-release-distribution.md). `garage status` reads GitHub through the external `gh` command, as the [#19 decision](https://github.com/bdonaldharris/builder-garage/issues/19#issuecomment-6837362562) chose. No additional technology is introduced until an established requirement calls for it.

## How the architecture evolves

Architectural decisions are made through the project's normal [contribution process](../CONTRIBUTING.md):

1. a change that needs a design decision starts from an issue;
2. where the answer is not yet known, discovery is performed and its findings are documented in that issue;
3. the decision is implemented, independently reviewed, and approved like any other substantive change;
4. this document is updated when an approved change alters the direction recorded here.

This follows the discipline recorded in the project's [history](history.md): understand what the evidence supports before turning it into architecture. Approved direction recorded here takes precedence over any historical hypothesis or candidate principle.

## Explicit GitLab provider operations

`garage forge` adds bounded, explicit Issue/MR search/read/create/update and workflow-note operations through glab. Command wiring is separate from the injected operation boundary. It validates configured forge/origin, typed target and authenticated acting identity; canonical workflows retain authorization/review responsibilities. Mutations use literal JSON stdin, disallow quick-action lines, and require exact durable GET read-back. Uncertain writes are not retried. The observation client's GET allowlist and read-only local web remain separate. Qualified E8 matching does not become independent review or native approval policy. GitLab private history uses a distinct provider/host/project namespace and MR kind, preserving legacy GitHub v1 files/keys. [Provider operations](provider-operations.md) defines installed skill bindings and limits.

## GitLab CI and native policy evidence

The #200 adapter extension keeps native gate outcomes (`passed`, `failed`, `unavailable`, `not-required`) and applicability/source URLs in the normalized observation model. Pipeline/job ownership, effective matching branch protections, native rule state, qualified current-diff human approval and discussion policy remain separate facts. Bounded consistency re-reads invalidate stale source/target/policy evidence; optional capability failures do not erase unrelated gate outcomes. `garage forge policy` is explicit GET-only inspection, sharing status's adapter; web remains read-only. Adopter CI examples live under `docs/examples/gitlab/`, with no active root GitLab pipeline or change to GitHub production CI. [CI/policy guidance](gitlab-ci-policy.md) records the limits and authorized native operations.

## GitLab release evidence and delegated operations

The #201 release support keeps the same split. `garage forge release inspect` reads tag, protection, release, asset-link, generic-package and tag-pipeline facts through the observation client's GET allowlist, extended with exact tag, release, link, package and filtered pipeline resources, and reports them as source-linked policy outcomes. Signing and provenance outcomes check asset presence and durable storage only. Delegated tag/release/link mutations live in the explicit forge operation path. Before any write they verify a Workflow Owner release delegation note against E8-qualified identity, exact operation, tag and ref, and the latest record. They then refuse implicit tag creation and replacement, and verify by read-back. Status, web and E6 evaluation are unchanged. Adopter package publishing is an example include under `docs/examples/gitlab/`; Builder Garage's GitHub release system is untouched. [Release guidance](gitlab-release.md) records the limits.
