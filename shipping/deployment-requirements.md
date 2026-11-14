# Deployment Requirements and Promotion

> **Non-normative guide.** This guide gives worked examples for applying existing rules. It adds no gate, ceremony, or authority, and it does not change any rule. The canonical skills govern: [Application Implementation Workflow](../skills/application-implementation-workflow/SKILL.md) and [Integration PR Review](../skills/integration-pr-review/SKILL.md). Where this guide and a skill differ, the skill is authoritative.

This guide is for implementers who write an Environment / Deployment Requirements section, and for reviewers who check one. It covers:

1. [what belongs in the section](#what-belongs-in-the-section);
2. [code defect or environment requirement](#code-defect-or-environment-requirement);
3. [visibility, not execution](#visibility-not-execution);
4. [release evidence through E6](#release-evidence-through-e6);
5. [boring promotion and Promotion Protection](#boring-promotion-and-promotion-protection).

The examples describe a generic application that adopts Builder Garage. Names such as `ORDERS_API_URL` are placeholders. No deployment platform, provider, or vendor is required, and any provider mentioned is only an illustration. Builder Garage itself has no release or promotion process, and nothing here describes one.

## What belongs in the section

The section lists the non-code actions needed for the reviewed artifact to work in its target environment. Who owns the list:

- **Readiness** records the requirements it can anticipate, in the `Known Environment / Deployment Requirements` part of its [readiness output](../skills/implementation-readiness-workflow/SKILL.md#readiness-output).
- **Agent #1** owns the authoritative list, and keeps it current as the artifact changes ([Environment / Deployment Requirements](../skills/application-implementation-workflow/SKILL.md#environment--deployment-requirements)).
- **The integration PR review** checks that the list is surfaced before the PR is merge-ready ([Environment / Deployment Requirements gate](../skills/integration-pr-review/SKILL.md#environment--deployment-requirements-gate)).

### Categories

Both skills name the same eleven categories, to be evaluated *when applicable*. They are prompts for thinking about the change, not a list to tick off. The wording below follows the gate; the implementation skill words a few of them at more length.

| Category | Example entry |
| --- | --- |
| SQL migrations or database schema/data changes | Apply migration `0042_add_order_status.sql` to staging, then production, before the new release starts serving traffic. |
| Manual database operations | Run the one-off backfill in `scripts/backfill_order_status.sql` once in each environment, after the migration. |
| Environment variables | Set `ORDERS_API_URL` in staging and production. The value is the base URL of the orders service in that environment. |
| Secrets | Add `PAYMENTS_WEBHOOK_SECRET` to the secret store in each environment. The value comes from the payment provider's dashboard. |
| Third-party/provider configuration | In the email provider's settings, verify the sending domain used for order receipts. |
| Webhook configuration | Register `https://<environment host>/webhooks/payments` with the payment provider for the `payment.succeeded` event. |
| External resource creation | Create the `order-exports` storage bucket in each environment. |
| Seed/UAT fixture requirements | Load `fixtures/uat_orders.json` in staging, so reviewers can open an order in each status. |
| Infrastructure/platform configuration | Raise the export worker's memory limit to 1 GB in each environment. |
| Restart or redeploy requirements | Restart the background worker after deploy, so it picks up the new queue name. |
| Post-deployment verification requirements | After deploy, place a test order in staging and confirm it reaches `confirmed` within a minute. |

A single change rarely needs entries in this many categories. Each category is evaluated, and an entry is written only when an action is actually needed. Placeholder entries such as "Secrets: none" add nothing.

A good entry says what to do, where, and, when it matters, in what order. It names the setting or file, never the secret's value.

### A complete section

For a change that adds payment confirmation by webhook:

```text
Environment / Deployment Requirements
- Apply migration 0042_add_order_status.sql in each environment, before the release serves traffic.
- Add PAYMENTS_WEBHOOK_SECRET to the secret store in each environment (value from the payment provider's dashboard).
- Register https://<environment host>/webhooks/payments with the payment provider for payment.succeeded.
- After deploy, place a test order in staging and confirm it reaches confirmed.
```

### When nothing is required

When the change needs no non-code action, the skills require exactly this line:

```text
Environment / Deployment Requirements: None
```

The section is never omitted, even when it is empty. A documentation change, or a code change that needs no new configuration, migration, resource, or restart, uses this line.

A project's own templates may lay the section out differently. Builder Garage's [pull request template](../.github/PULL_REQUEST_TEMPLATE.md), for example, has an `Environment / Deployment Requirements` heading and asks for `None` beneath it when nothing is required. The line above is the form the skills state.

## Code defect or environment requirement

Both skills draw this line ([Environment / Deployment Requirements](../skills/application-implementation-workflow/SKILL.md#environment--deployment-requirements), [Environment / Deployment Requirements gate](../skills/integration-pr-review/SKILL.md#environment--deployment-requirements-gate)). An action that is still pending in an environment, such as an unapplied migration, a missing staging secret, an unconfigured webhook, an absent fixture, or a pending restart, is an environment requirement. It is a code defect only when the implementation itself left out a deliverable the feature needs, such as the migration, configuration contract, or provider integration artifact.

The question to ask is: *does the artifact contain everything the feature needs, so that only an environment action remains?*

| The change… | Classification |
| --- | --- |
| includes the migration that adds `orders.status`, but it has not been applied in staging yet | Environment requirement. List it in the section. |
| reads `orders.status`, but no migration in the change creates that column | Code defect. The artifact is missing the migration, and listing an environment action would not supply it. |
| verifies webhook signatures using `PAYMENTS_WEBHOOK_SECRET`, which is not yet set in staging | Environment requirement. |
| needs a webhook signing secret, but the code never reads one, and accepts unsigned events | Code defect. |

An environment requirement is not, by itself, a code blocker. A code defect is judged under the [review contract](../skills/integration-pr-review/SKILL.md#governing-review-contract), like any other defect.

Reviewers verify the requirements carried forward by readiness and implementation, and what is evident from the reviewed artifact. The gate does not ask them to dig for deployment work beyond that.

## Visibility, not execution

The gate requires the requirements to be *visible*. It does not require every action to have been carried out before the PR can be approved ([Environment / Deployment Requirements gate](../skills/integration-pr-review/SKILL.md#environment--deployment-requirements-gate)).

So the merge-ready output, `APPROVED — MERGE` with its requirements block ([Merge-ready output contract](../skills/integration-pr-review/SKILL.md#merge-ready-output-contract)):

- **does claim** that the change passed the review contract, and that the known non-code actions are listed;
- **does not claim** that those actions have been carried out, in any environment;
- **does not say** who carries them out, or when.

For example, a PR approved with "Apply migration 0042 in each environment" in its block may merge before the migration is applied anywhere. The block tells whoever deploys the change what must happen for it to work.

## Release evidence through E6

A project that wants extra evidence before a change merges, such as a recorded staging verification for changes that touch payments, adds it through [E6: Release and evidence conditions](../docs/extension-points.md#e6-release-and-evidence-conditions). E6 is the only way a project adds release evidence. Prose, templates, and checklists outside it add none.

- Conditions are configured as `release_conditions` in the project's `.garage/config.yaml`; [Project configuration](../docs/configuration.md#the-project-section) gives the shape.
- A condition adds evidence. It never removes a gate ([Phase 5 — Merge decision](../skills/integration-pr-review/SKILL.md#phase-5--merge-decision)).
- The default is none. With no conditions configured, no extra release evidence is required.

This guide defines no release condition. Which conditions a project needs, if any, is the project's choice, made through its normal review of its configuration.

## Boring promotion and Promotion Protection

Promotion moves already-approved work toward release. It should stay intentionally boring: no planned feature work, no opportunistic refactoring, no acceptance-criteria completion, and no hidden correction cycle inside the promotion ([Promotion Protection](../skills/integration-pr-review/SKILL.md#promotion-protection), [Promotion](../skills/application-implementation-workflow/SKILL.md#promotion)).

Observations can still surface during promotion. Ask the same two questions of each: *is it caused by the release artifact,* and *does it stop the approved behavior from working safely as intended?*

**A late observation, routed to the backlog.** While promoting a release that adds order exports, someone notices that the export page's date picker does not remember the last range chosen. That is how it has always worked, and the exports behave as approved. It is not caused by the release, and it does not stop the release from working. It goes to the project's suggestions backlog ([E7](../docs/extension-points.md#e7-improvement-suggestions-backlog), see [Consolidated suggestion backlog](../skills/integration-pr-review/SKILL.md#consolidated-suggestion-backlog)). It stays out of the promotion. It becomes planned work only if the Workflow Owner ([E8](../docs/extension-points.md#e8-human-authorities)) later promotes it.

**A demonstrated defect in the release artifact.** During the same promotion, exports of more than 1,000 orders fail, because the new export query has a limit that the approved change introduced. That is caused by the release artifact and stops approved behavior from working. It is not fixed inside the promotion. The correction goes through the normal implementation and review workflow, and the promotion carries the corrected, approved result.

"Zero gum on the bottom of the shoe" means no known defect that stops the approved release behavior from working safely. It does not mean an empty backlog ([Promotion Protection](../skills/integration-pr-review/SKILL.md#promotion-protection), [Zero gum on the bottom of the shoe](../skills/application-implementation-workflow/SKILL.md#zero-gum-on-the-bottom-of-the-shoe)). See also [What does not block this PR](../skills/integration-pr-review/SKILL.md#what-does-not-block-this-pr).

## Boundary

This guide explains existing rules with examples. It defines no checklist, release condition, release playbook, or release automation, and it requires no deployment platform or vendor. The canonical skills, the [responsibility floor](../docs/responsibility-floor.md), and the [extension-point registry](../docs/extension-points.md) remain the sources of truth.
