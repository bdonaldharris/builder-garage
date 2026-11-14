# Responsibility Floor

**Version:** 2. Version 1 stated F7 with interim wording that pointed to the evidence format's design in [#34](https://github.com/bdonaldharris/builder-garage/issues/34); F7's wording was updated for the adopted evidence format under [#76](https://github.com/bdonaldharris/builder-garage/issues/76).
**Status:** Canonical. Adopted by the [#16 maintainer decision](https://github.com/bdonaldharris/builder-garage/issues/16#issuecomment-6836671788).

The responsibility floor is the set of rules that every project adopting Builder Garage keeps, whatever it configures. No project configuration, adoption profile, governance document, or in-session instruction may weaken a floor rule.

The floor and the [extension-point registry](extension-points.md) are Builder Garage's canonical normative sources for what a project may and may not adapt. The registry lists the only things a project may configure. This document lists what configuration can never weaken.

## Precedence

When sources disagree, they apply in this order:

1. **The responsibility floor** (this document).
2. **Valid project configuration**, which supplies values only at the extension points declared in the [registry](extension-points.md).
3. **Canonical defaults**, which apply only where a project omits a value it is allowed to supply.

Further rules:

- A configuration value that would weaken a floor rule is invalid. It is rejected, not applied.
- A project may configure only what the registry declares. Anything the registry does not declare is not adaptable.
- GOVERNANCE, adoption profiles, and decision records may explain a configured value, or authorize a change to it through the project's normal process. They may not silently redefine it.
- An instruction given during a session cannot authorize a floor violation. Changing project configuration through its review path is the only way to change a configurable value.
- If a canonical skill or governance text appears to permit something a floor rule forbids, the floor rule governs, and the conflict is a defect to correct through the [contribution process](../CONTRIBUTING.md).

## Rule shapes

Each rule has one of two shapes:

| Shape | Meaning |
| --- | --- |
| **Fixed** | Configuration cannot change the rule. A project may still adopt stricter requirements in its own governance, as long as they do not conflict with it. |
| **Extend-only** | The rule names a registry extension point through which a project may add to it. A project can never use that extension point to remove or relax anything the rule requires. |

## Rules

Each rule states the invariant. The linked sources define how it is carried out. Identifiers are stable and are never reused.

| ID | Rule | Shape | Defined in |
| --- | --- | --- | --- |
| **F1** | The implementer of a change never performs its independent review. | Fixed | [GOVERNANCE: Review requirements](../GOVERNANCE.md#review-requirements), [application-implementation-workflow](../skills/application-implementation-workflow/SKILL.md#executing-agent-identity) |
| **F2** | Implementation and independent review run in separate execution contexts, such as separate sessions or threads. They may use the same tool or model. Separation is a recorded assertion. Garage never claims to certify it. | Fixed | [independent-implementation-review: Invocation boundary](../skills/independent-implementation-review/SKILL.md#invocation-boundary), [README: Agent identity and role boundaries](../README.md#agent-identity-and-role-boundaries) |
| **F3** | A skill invocation has one role for its whole execution. Reaching another role's responsibility is a terminal handoff. The agent stops, and does not wait, poll, or continue in the other role. | Fixed | [README: Agent identity and role boundaries](../README.md#agent-identity-and-role-boundaries), [application-implementation-workflow: Role separation](../skills/application-implementation-workflow/SKILL.md#role-separation) |
| **F4** | A role writes only its own workflow-state markers, and never a record that represents another role. | Fixed | [integration-pr-review: Durable PR workflow state](../skills/integration-pr-review/SKILL.md#durable-pr-workflow-state), [independent-implementation-review: Durable review-state comments](../skills/independent-implementation-review/SKILL.md#durable-review-state-comments) |
| **F5** | Every change receives independent review before it is committed or advanced, and the record on the originating issue is the authoritative review record. The only exception is a small correction that a configured human authority explicitly classifies as one ([E4](extension-points.md#e4-small-correction-entry-path)). An agent may not classify a change as a small correction, and may not exempt itself from review. | Fixed | [GOVERNANCE: Review requirements](../GOVERNANCE.md#review-requirements), [#16 decision](https://github.com/bdonaldharris/builder-garage/issues/16#issuecomment-6836671788) item 1 |
| **F6** | A correction does not restore approval. Only an independent re-review of the complete corrected artifact does. | Fixed | [GOVERNANCE: Review requirements](../GOVERNANCE.md#review-requirements), [independent-implementation-review: Correction re-review](../skills/independent-implementation-review/SKILL.md#correction-re-review) |
| **F7** | An approval applies only to the artifact it reviewed, and to later artifacts shown to be equivalent to it. Implementation-completion and review records identify the artifact they cover precisely enough for a later comparison. | Fixed | [integration-pr-review: Artifact equivalence](../skills/integration-pr-review/SKILL.md#artifact-equivalence), [Review evidence](review-evidence.md), [#16 decision](https://github.com/bdonaldharris/builder-garage/issues/16#issuecomment-6836671788) item 6 |
| **F8** | Workflow evidence is durable on the governing GitHub Issue or pull request, and is written and then verified. A conversation, a local file, or developer-local history is never the record. A project may require additional evidence ([E6](extension-points.md#e6-release-and-evidence-conditions)), but never less. | Extend-only | [independent-implementation-review: Durable review-state comments](../skills/independent-implementation-review/SKILL.md#durable-review-state-comments), [README: Operating model](../README.md#operating-model), [#19 decision](https://github.com/bdonaldharris/builder-garage/issues/19#issuecomment-6837362562) |
| **F9** | Role, not vendor, model, or tool, determines responsibility and authority. Any vendor, model, or tool may fill a role if it meets that role's contract, including producing its durable record. An automated reviewer's output is review input only, and its findings require independent disposition before merge. | Fixed | [integration-pr-review: Role authority and automated review](../skills/integration-pr-review/SKILL.md#role-authority-and-automated-review), [#16 decision](https://github.com/bdonaldharris/builder-garage/issues/16#issuecomment-6836671788) item 3 |
| **F10** | An independent pull-request review is required when either minimum trigger applies: the pull request cannot be shown to be artifact-equivalent to the approved change, or an automated reviewer reports findings on the current head. A project may add triggers ([E2](extension-points.md#e2-additional-pr-review-triggers)), but may not remove either minimum trigger. | Extend-only | [integration-pr-review: PR-review triggers](../skills/integration-pr-review/SKILL.md#pr-review-triggers), [#26 decision](https://github.com/bdonaldharris/builder-garage/issues/26#issuecomment-6834102663) |
| **F11** | Independent review and human acceptance of a change are distinct gates. Neither substitutes for the other. | Fixed | [GOVERNANCE: Review requirements](../GOVERNANCE.md#review-requirements) |
| **F12** | Product intent is accepted by an explicit human authority ([E8](extension-points.md#e8-human-authorities)), not by an agent or an agent gate. | Fixed | [GOVERNANCE: Product-intent acceptance](../GOVERNANCE.md#product-intent-acceptance), [#8 decision](https://github.com/bdonaldharris/builder-garage/issues/8#issuecomment-6817923896), [#16 decision](https://github.com/bdonaldharris/builder-garage/issues/16#issuecomment-6836671788) item 2 |
| **F13** | The three-question review contract is the same for every change. Analysis and verification depth vary through triggers, not through risk classes. | Fixed | [README: Review contract](../README.md#review-contract), [#7 decision](https://github.com/bdonaldharris/builder-garage/issues/7#issuecomment-6817635061) item 1 |
| **F14** | No configuration may falsely claim independent review, collapse independent responsibilities into one execution context, or disregard durable evidence. | Fixed | [#7 decision](https://github.com/bdonaldharris/builder-garage/issues/7#issuecomment-6817635061) item 2, [#16 decision](https://github.com/bdonaldharris/builder-garage/issues/16#issuecomment-6836671788) |

## No orchestration

The floor constrains what agents and people do. It does not make Garage an orchestrator. Garage may validate declared configuration and the structure of recorded evidence. It does not assign agents, invoke them, wait or poll for them, advance workflow state, or certify that agents worked independently. Agents run in their own harnesses, and GitHub Issue and PR records remain the authoritative evidence.

## Current state

The artifacts that carry out the floor:

- **Project configuration** declares values at the registry's extension points ([Project configuration](configuration.md)). Builder Garage's own values are in its `.garage/config.yaml`.
- **The canonical skills** read that configuration only at the extension points they cite ([How the skills read the configuration](configuration.md#how-the-skills-read-the-configuration)). Each skill states the floor rules it must itself enforce. No skill restates the complete floor ([#33](https://github.com/bdonaldharris/builder-garage/issues/33)).
- **Revision-identifiable evidence** under F7 uses the format in [Review evidence](review-evidence.md), which the skills' record templates adopt. Records written before that format was adopted remain valid historical evidence ([Review evidence: Compatibility](review-evidence.md#compatibility)).
- **Reporting.** `garage status` ([#40](https://github.com/bdonaldharris/builder-garage/issues/40)) reports the workflow records of one issue or pull request ([README: Workflow status](../README.md#workflow-status)). It reads GitHub once and writes nothing. It keeps checked facts and asserted claims separate, compares recorded changesets, and recomputes a committed artifact only from objects already local. It reports a record without an evidence block as legacy, and does not evaluate it. It does not certify that agents worked independently, approve a change, or advance workflow state.

## Changing the floor

The floor changes only through the normal [contribution process](../CONTRIBUTING.md), as a framework change. Any change to a rule's wording, shape, or set increments the version above. A rule may be retired, but its identifier is never reused.
