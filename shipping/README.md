# Shipping

Shipping covers integration, pull requests, production-bound review, release readiness, deployment requirements, and promotion.

Changes must satisfy the governing implementation and integration skills, including any environment, secrets, webhooks, migrations, seeds, and deployment prerequisites. Human approval and CI requirements remain applicable where required.

See [Integration PR Review](../skills/integration-pr-review/SKILL.md) and [Application Implementation Workflow](../skills/application-implementation-workflow/SKILL.md). This directory documents the responsibility area; it does not replace the canonical workflow.

## Boundary

Shipping owns *how accepted work reaches its destination*: integration, release readiness, and promotion. It begins once a change has passed independent review and is proposed as a pull request. Review performed at the pull-request stage remains governed by the [Inspection](../inspection/) review contract.

## Status

This directory contains documentation only. No release or deployment tooling has been implemented. See the [architectural direction](../docs/architecture.md).
