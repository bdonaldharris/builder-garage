# Crew

The Crew consists of specialized engineering agents with distinct responsibilities, identity boundaries, and terminal handoffs.

- **Agent #1 — Implementation Agent:** implementation, correction, validation, and implementation-side integration work.
- **Agent #2 — Independent Review Agent:** independent complete-artifact review and approval or changes-required disposition.

These are separate agent invocations—not interchangeable personas or phases of a single invocation. The [root operating model](../README.md#agent-identity-and-role-boundaries) and canonical [implementation](../skills/application-implementation-workflow/SKILL.md) and [independent review](../skills/independent-implementation-review/SKILL.md) skills govern behavior.

## Boundary

The Crew owns *who* does the work and where each role's responsibility ends. The procedures a role follows belong to the [Workbench](../workbench/); the standard its output is held to belongs to [Inspection](../inspection/).

## Status

This directory contains documentation only. Roles are defined by the skills linked above; no agent orchestration has been implemented. See the [architectural direction](../docs/architecture.md).
