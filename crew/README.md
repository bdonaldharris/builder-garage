# Crew

The Crew is who does the work. It has two kinds of members ([#57 decision](https://github.com/bdonaldharris/builder-garage/issues/57#issuecomment-6865008014), items 2 and 4):

- **Human authorities** make accountable decisions.
- **Technicians** perform bounded work and produce evidence.

Workflow positions are separate from both: they place a Technician in the implementation and review sequence.

## Human authorities

Human authorities are configured at extension point [E8](../docs/extension-points.md#e8-human-authorities), and are always people, never agents, bots, or tools:

| Authority | Decides |
| --- | --- |
| **Workflow Owner** | workflow decisions that belong to a person |
| **Change acceptor** | whether the project accepts a change |
| **Product-intent acceptor** | whether a result is what was intended |
| **Small-correction classifier** | whether a change is a small correction. This authority exists only when small corrections are enabled. |

## Technicians

A Technician is an agent or a human specialist that performs bounded work and produces evidence. Each canonical skill holds one Technician role for its whole invocation, recorded as its `metadata.garage-role`.

| Technician | Answers | Skill |
| --- | --- | --- |
| `discovery` | What does the existing system actually do? | [engineering-discovery-workflow](../skills/engineering-discovery-workflow/SKILL.md) |
| `readiness` | Is this change understood well enough to implement? | [implementation-readiness-workflow](../skills/implementation-readiness-workflow/SKILL.md) |
| `planner` | How should approved work be framed and entered into the backlog? | [issue-authoring-workflow](../skills/issue-authoring-workflow/SKILL.md) |
| `implementer` | Implementation, correction, and integration-PR finalization | [application-implementation-workflow](../skills/application-implementation-workflow/SKILL.md), [integration-pr-review](../skills/integration-pr-review/SKILL.md) |
| `reviewer` | Was the artifact independently shown to be correct? | [independent-implementation-review](../skills/independent-implementation-review/SKILL.md) |

A planner may prepare evidence or an issue. It never makes the product, workflow, or acceptance decisions that belong to the Builder or to a configured human authority.

Later needs may add Technicians, such as `tester`, `release`, or `documentation`. A new one is added through a framework change, in the same way.

## Workflow positions

**Agent #1** and **Agent #2** are workflow positions, not role names. Today, the implementer acts as Agent #1, and the reviewer as Agent #2. A position is not permanent: a future Technician could fill one.

The separation rule holds whichever Technicians fill the positions: **the same execution context never both implements and independently reviews the same artifact** ([F1, F2](../docs/responsibility-floor.md#rules)). A skill invocation holds one role for its whole execution, and reaching another role's responsibility is a terminal handoff ([F3](../docs/responsibility-floor.md#rules)).

These are separate invocations, not interchangeable personas or phases of one invocation. The [root operating model](../README.md#agent-identity-and-role-boundaries) and the canonical skills govern behavior.

## Boundary

The Crew owns *who* does the work and where each role's responsibility ends. The procedures a role follows belong to the [Workbench](../workbench/); the standard its output is held to belongs to [Inspection](../inspection/).

## Status

This directory contains documentation only. Roles are defined by the skills linked above. No agent orchestration has been implemented, and nothing here assigns agents. See the [architectural direction](../docs/architecture.md).
