# Manuals

Manuals are maintained technical guidance on how software built with a particular technology should be designed and built.

## Current manuals

| Manual | Version | Adopted by Builder Garage in |
| --- | --- | --- |
| [Go](go.md) | 0.1 (provisional) | [Builder Garage Go Manual](../docs/go-manual.md) |

## How manuals are used

- **A manual is guidance.** It records practical design and coding conventions for one technology. It is not a requirement, a configuration file, or a workflow.
- **A project adopts a manual explicitly.** The project names the manual and the version it adopts in its own profile. It records there only the choices that are specific to it. Builder Garage's own profile is [docs/go-manual.md](../docs/go-manual.md).
- **Adoption makes a manual the project's default guidance.** Departures from it belong in the project's profile. A departure from a manual is not, by itself, a blocking review finding. An actual defect is still judged under the [review contract](../README.md#review-contract), like any other defect.
- **A manual does not override canonical sources.** What Builder Garage requires is defined by the canonical [skills](../skills/), [GOVERNANCE.md](../GOVERNANCE.md), [CONTRIBUTING.md](../CONTRIBUTING.md), the [architectural direction](../docs/architecture.md), and accepted [ADRs](../docs/adr/). Where a manual and one of those sources differ, the source is authoritative. Manuals link to those sources rather than restating them.

### Versions

Each manual carries a version label in its own text, and a project's profile cites the label it adopts. The label identifies a revision of the guidance. It is not a selection or resolution mechanism.

How projects select manuals, resolve versions, and record extensions through configuration is deferred to [#16](https://github.com/bdonaldharris/builder-garage/issues/16).

## Boundary

Manuals own *how a technology should be designed and built*. They do not define:

- how engineering work is carried out ([Workbench](../workbench/));
- who does the work, or where each role stops ([Crew](../crew/));
- whether the work is correct ([Inspection](../inspection/));
- how accepted work reaches its destination ([Shipping](../shipping/));
- how a project selects, configures, or extends them ([#16](https://github.com/bdonaldharris/builder-garage/issues/16)).

## Status

This directory contains documentation only. Manuals are a conceptual area. This directory creates no runtime component, package boundary, configuration schema, command, or orchestration mechanism. See the [architectural direction](../docs/architecture.md).
