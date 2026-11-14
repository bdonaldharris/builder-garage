# Manuals

Manuals are maintained technical guidance on how software built with a particular technology should be designed and built.

## Current manuals

| Manual | Version | Adopted by Builder Garage in |
| --- | --- | --- |
| [C# / .NET](dotnet.md) | 0.1 (provisional) | Not adopted by Builder Garage |
| [Go](go.md) | 0.1 (provisional) | [Builder Garage Go Manual](../docs/go-manual.md) |
| [Python](python.md) | 0.1 (provisional) | Not adopted by Builder Garage |

## How manuals are used

- **A manual is guidance.** It records practical design and coding conventions for one technology. It is not a requirement, a configuration file, or a workflow.
- **A project adopts a manual explicitly.** The project names the manual and the version it adopts in its own profile. It records there only the choices that are specific to it. Builder Garage's own profile is [docs/go-manual.md](../docs/go-manual.md).
- **Adoption makes a manual the project's default guidance.** Departures from it belong in the project's profile. A departure from a manual is not, by itself, a blocking review finding. An actual defect is still judged under the [review contract](../README.md#review-contract), like any other defect.
- **A manual does not override canonical sources.** What Builder Garage requires is defined by the canonical [skills](../skills/), [GOVERNANCE.md](../GOVERNANCE.md), [CONTRIBUTING.md](../CONTRIBUTING.md), the [architectural direction](../docs/architecture.md), and accepted [ADRs](../docs/adr/). Where a manual and one of those sources differ, the source is authoritative. Manuals link to those sources rather than restating them.

### The catalog and versions

Each manual begins with YAML frontmatter that is its catalog entry ([#60 decision](https://github.com/bdonaldharris/builder-garage/issues/60#issuecomment-6860594142)):

```yaml
---
name: go             # matches the file name, go.md
version: "0.1"
status: provisional  # provisional or stable
description: "One line saying what the manual covers."
---
```

The `garage` binary embeds the manuals and reads the catalog from this frontmatter, so adding a manual needs no code change. The table above must match it, and a test checks that it does. The binary delivers exactly one version of each manual: the one in its frontmatter.

A project adopts manuals at extension point [E10](../docs/extension-points.md#e10-manuals) of the extension-point registry. The choice is recorded in the `project.guidance.manuals` setting of `.garage/config.yaml`:
- **Guided setup.** `garage init` offers the catalog as a pick list.
- **Validation.** `garage config validate` refuses a manual the binary does not carry. It reports a configured version that differs from the delivered one, but does not refuse it. See [Project configuration](../docs/configuration.md).

### Delivery to agents

`garage skills install` delivers each manual the project adopts as its own skill, `builder-garage-manual-<name>`, at the project's skill targets, so that agents in the project can read it. Being installed never makes a manual apply. Only the adopting project's configuration does. The manifest records which projects adopt each delivered manual, and a manual no project adopts any longer is removed. See [Skill installation](../docs/skill-installation.md#manuals).

## Boundary

Manuals own *how a technology should be designed and built*. They do not define:

- how engineering work is carried out ([Workbench](../workbench/));
- who does the work, or where each role stops ([Crew](../crew/));
- whether the work is correct ([Inspection](../inspection/));
- how accepted work reaches its destination ([Shipping](../shipping/));
- how a project selects or configures them ([extension-point registry](../docs/extension-points.md#e10-manuals)).

## Status

Manuals are a conceptual area. Besides the manuals and this README, this directory holds only `embed.go`, which embeds the manuals in the `garage` binary so that it can deliver them. It creates no command and no orchestration mechanism. See the [architectural direction](../docs/architecture.md).
