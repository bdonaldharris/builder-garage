# Project Specs

**Status:** Canonical. Defined under [#149](https://github.com/bdonaldharris/builder-garage/issues/149), following the [#149 discovery report](https://github.com/bdonaldharris/builder-garage/issues/149#issuecomment-7001367476) and the [#149 maintainer decision](https://github.com/bdonaldharris/builder-garage/issues/149#issuecomment-7010610605).

A project that adopts Builder Garage may keep a project-level specification. It records what the whole project is for and what it must do, so that issues have a durable source of intent to trace back to. [project-spec-workflow](../skills/project-spec-workflow/SKILL.md) (`spec-design`) drafts its content. This document summarizes how a spec relates to the rest of the workflow. The skill governs, and this document does not restate its procedure.

A spec is optional. No project is required to have one, and no gate depends on one.

## Where it lives

A project's spec lives under `docs/specs/`: usually one file, `docs/specs/<project>.md`, optionally split into further files by area. The location is fixed; no project setting changes it.

Its header follows the [ADR](adr/) pattern. It gives the spec's status, its accepting authority, and links to the pull request that accepted it, the decisions it records, and its evidence. The structure is in the skill's [Spec output](../skills/project-spec-workflow/SKILL.md#spec-output), and in its optional [template](../skills/project-spec-workflow/templates/project-spec.md).

## Authority and precedence

A project holds several kinds of truth, each with one authority ([principles: Authority](principles.md#authority)).

| Record | Authority for |
| --- | --- |
| The accepted spec | The project's intended product |
| The governing issue | One change's requirements, acceptance criteria, and scope |
| The current repository | What actually exists |

When an issue and the accepted spec disagree, neither silently wins. The disagreement is reported on the issue, and the product-intent acceptor decides whether the issue or the spec changes.

## How a spec changes

The spec has one way in. It changes only through a pull request whose governing issue names the spec change in its acceptance criteria.

| Workflow point | Spec access |
| --- | --- |
| [project-spec-workflow](../skills/project-spec-workflow/SKILL.md) (planner) | Drafts content. Records the draft on a governing issue or in the conversation, as the Builder chooses. Never writes the file |
| [application-implementation-workflow](../skills/application-implementation-workflow/SKILL.md) (Agent #1) | Writes the file, only when the governing issue's acceptance criteria name the spec change |
| [independent-implementation-review](../skills/independent-implementation-review/SKILL.md) (Agent #2) | Reviews the spec change as part of the artifact, under the three-question contract |
| Discovery, readiness, issue authoring, PR review, and promotion | Read only. Gaps and conflicts are reported to the issue |
| The product-intent acceptor (a human, [E8](extension-points.md#e8-human-authorities)) | Accepts the intent a spec change states, as part of accepting its pull request ([GOVERNANCE: Product-intent acceptance](../GOVERNANCE.md#product-intent-acceptance)) |

A draft kept only in the conversation is not a durable record. The spec becomes durable when the pull request that adds it is merged.

## Specs for existing projects

An existing project's spec is drafted from a verified discovery report and the Builder's stated intent. It keeps three kinds of statement apart:

- **Intended:** what the project is meant to do. Accepted with the spec.
- **Observed:** current behavior, cited from the discovery report, and not yet accepted as intent.
- **Pending:** decisions no one has made yet, each naming its holder.

An Observed statement becomes Intended only through an accepted spec change, so a current defect is never recorded as intent.

## Specs and issue authoring

Once a spec change is merged, its pull request, or the acceptance comment on it, is a durable decision. [issue-authoring-workflow](../skills/issue-authoring-workflow/SKILL.md) may author an issue from it, as from any other durable decision. Each issue still needs the Builder's explicit authorization. A spec never generates issues by itself.

## Builder Garage

Builder Garage adopts `project-spec-workflow` ([Skill adoption](skill-adoption.md)). It keeps no spec of its own yet. That is deferred under the [#149 decision](https://github.com/bdonaldharris/builder-garage/issues/149#issuecomment-7010610605), item 8.
