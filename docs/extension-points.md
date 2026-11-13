# Extension-Point Registry

**Version:** 1
**Status:** Canonical. Defined under [#32](https://github.com/bdonaldharris/builder-garage/issues/32), following the [#16 maintainer decision](https://github.com/bdonaldharris/builder-garage/issues/16#issuecomment-6836671788). That decision established the registry's role and its precedence. The entries below, including their allowed values and defaults, are accepted through this change's independent review and maintainer approval.

This registry lists every point at which a project adopting Builder Garage may adapt the framework. For each point, it gives the values a project may choose, the canonical default, and the constraint the choice must keep. A project may adapt only what this registry declares. The rules no choice may weaken are in the [responsibility floor](responsibility-floor.md).

## How the registry applies

- **Precedence.** The responsibility floor comes first. Valid project configuration supplies values only at the extension points declared here. A canonical default applies only where a project omits a value it is allowed to supply. See [Precedence](responsibility-floor.md#precedence).
- **Declared points only.** A setting that is not declared here is not adaptable. It is an error, not a silent extension.
- **Required choices.** Where an extension point has no default, a project must supply its value explicitly. Leaving it unset is not a valid configuration. `garage config validate` reports each missing value, and `garage init` never writes a configuration without them ([Project configuration](configuration.md)).
- **One authoritative value.** Once a project configures a value, its prose documents, such as GOVERNANCE or an adoption profile, may explain that value or authorize a change to it through the project's normal process. They do not silently redefine it.
- **Configuration format.** A project records these values in the `project` section of `.garage/config.yaml`, as described in [Project configuration](configuration.md). That file is the sole canonical source of a project's configured values. The canonical skills read it only at the extension points they cite, and only when `garage config validate` succeeds ([How the skills read the configuration](configuration.md#how-the-skills-read-the-configuration)). A project's adoption record, such as Builder Garage's [skill-adoption profile](skill-adoption.md), may explain the configured values without redefining them.
- **Validation, not orchestration.** Garage may validate declared values and the structure of recorded evidence. Choosing a value never makes Garage assign, invoke, wait for, or advance any agent or workflow. See [No orchestration](responsibility-floor.md#no-orchestration).

## Extension points

### E1: Integration branch

| | |
| --- | --- |
| **Allowed values** | The name of one branch that implementation pull requests target. |
| **Default** | `staging`, the branch the canonical skills, such as [integration-pr-review](../skills/integration-pr-review/SKILL.md#no-pr-exists), use when E1 is omitted. |
| **Constraint** | Changes the destination only. It changes no gate, review, or evidence requirement. |
| **Source** | [GOVERNANCE: Changes to `main`](../GOVERNANCE.md#changes-to-main), [#6 discovery](https://github.com/bdonaldharris/builder-garage/issues/6) |

### E2: Additional PR-review triggers

| | |
| --- | --- |
| **Allowed values** | A list of named triggers, each with a description of when it applies. |
| **Default** | None. Only the two minimum triggers apply. |
| **Constraint** | Extend-only ([F10](responsibility-floor.md#rules)). A project may add triggers. It may not remove or narrow either minimum trigger. |
| **Source** | [integration-pr-review: PR-review triggers](../skills/integration-pr-review/SKILL.md#pr-review-triggers), [#26 decision](https://github.com/bdonaldharris/builder-garage/issues/26#issuecomment-6834102663) |

### E3: Automated reviewer

| | |
| --- | --- |
| **Allowed values** | None, or a named automated reviewer and the mechanism used to request its review. |
| **Default** | None. No automated reviewer is required. |
| **Constraint** | Review input only ([F9](responsibility-floor.md#rules)). Its findings on the current head fire minimum PR-review trigger 2 and require independent disposition. Its silence or absence establishes nothing. |
| **Source** | [integration-pr-review: Role authority and automated review](../skills/integration-pr-review/SKILL.md#role-authority-and-automated-review), [#7 decision](https://github.com/bdonaldharris/builder-garage/issues/7#issuecomment-6817635061) item 5, [#22 decision](https://github.com/bdonaldharris/builder-garage/issues/22#issuecomment-6830592571) item 3 |

### E4: Small-correction entry path

| | |
| --- | --- |
| **Allowed values** | Disabled, or enabled with:<br>(a) the human authority ([E8](#e8-human-authorities)) who classifies a change as a small correction;<br>(b) whether such a change may be proposed without an issue. |
| **Default** | Disabled. Every change goes through the full lifecycle, including independent review. |
| **Constraint** | Only the configured human authority may classify a change as a small correction, and the classification must be explicit ([F5](responsibility-floor.md#rules)). An agent may not classify a change or exempt itself. Human acceptance of the change still applies ([F11](responsibility-floor.md#rules)). |
| **Source** | [#16 decision](https://github.com/bdonaldharris/builder-garage/issues/16#issuecomment-6836671788) item 1, [#7 decision](https://github.com/bdonaldharris/builder-garage/issues/7#issuecomment-6817635061) items 3–4 |

### E5: Discovery applicability

| | |
| --- | --- |
| **Allowed values** | Implementer judgment with Workflow Owner escalation, or a list of named discovery triggers. |
| **Default** | None. A project must choose one of the allowed values explicitly. Under implementer judgment with Workflow Owner escalation, the implementer decides whether discovery is needed, and the Workflow Owner may require it. |
| **Constraint** | The choice must be explicit, so no value applies by default. Discovery investigates and never implements, whichever value is chosen ([engineering-discovery-workflow](../skills/engineering-discovery-workflow/SKILL.md#core-invariant)). |
| **Source** | [#7 decision](https://github.com/bdonaldharris/builder-garage/issues/7#issuecomment-6817635061) item 6 |

### E6: Release and evidence conditions

| | |
| --- | --- |
| **Allowed values** | None, or a list of named conditions. Each condition says when it applies and what additional evidence it requires. |
| **Default** | None. |
| **Constraint** | Extend-only ([F8](responsibility-floor.md#rules)). A condition adds evidence requirements and never removes one. This point replaces the retired term *production-bound*. |
| **Source** | [#7 decision](https://github.com/bdonaldharris/builder-garage/issues/7#issuecomment-6817635061) item 3 |

### E7: Improvement-suggestions backlog

| | |
| --- | --- |
| **Allowed values** | A reference to one persistent issue, or none. With none, non-blocking observations stay in the follow-up section of the review record that raised them. |
| **Default** | The repository's persistent issue named `Application Improvement Suggestions`, as the canonical skills describe. |
| **Constraint** | A suggestion authorizes nothing until a person deliberately promotes it into separately scoped work. Non-blocking observations never block the current change. |
| **Source** | [application-implementation-workflow: Consolidated suggestion backlog](../skills/application-implementation-workflow/SKILL.md#consolidated-suggestion-backlog), [#22 decision](https://github.com/bdonaldharris/builder-garage/issues/22#issuecomment-6830592571) item 5 |

### E8: Human authorities

| | |
| --- | --- |
| **Allowed values** | For each human responsibility below, the project governance role that holds it, and, where needed, the people or teams who fill that role. |
| **Default** | None. A project must explicitly name the Workflow Owner, the change acceptor, and the product-intent acceptor. It may name the same governance role for more than one of them, for example the Workflow Owner as the product-intent acceptor. The small-correction classifier must be named only if E4 is enabled. |
| **Constraint** | Each responsibility is held by a human, never by an agent, bot, or tool ([F12](responsibility-floor.md#rules), [F9](responsibility-floor.md#rules)). |
| **Source** | [GOVERNANCE: Workflow Owner](../GOVERNANCE.md#workflow-owner), [#8 decision](https://github.com/bdonaldharris/builder-garage/issues/8#issuecomment-6817923896) item 1, [#6 direction](https://github.com/bdonaldharris/builder-garage/issues/6#issuecomment-6817083027) item 7 |

The human responsibilities are:

| Responsibility | What it decides |
| --- | --- |
| **Workflow Owner** | Workflow decisions that belong to a person rather than to an agent, including clarifications and the promotion of suggestions into work. The canonical skills call it the workflow owner. |
| **Change acceptor** | Whether the project accepts a change. This is the human-acceptance gate in [F11](responsibility-floor.md#rules). |
| **Product-intent acceptor** | Whether a result is what was intended for its user ([F12](responsibility-floor.md#rules)). |
| **Small-correction classifier** | Whether a change is a small correction ([E4](#e4-small-correction-entry-path)). |

### E9: User-facing inspection trigger

| | |
| --- | --- |
| **Allowed values** | The canonical trigger, optionally extended with project-specific conditions. |
| **Default** | When a change affects something a user sees or uses, and inspecting it is practical, the product-intent acceptor inspects the result in that form before accepting it. |
| **Constraint** | A trigger evaluated by the human acceptor. It is not a universal step and not an agent gate. Whether a project may turn the trigger off is not decided, so this registry declares only the canonical trigger and project additions to it. |
| **Source** | [#8 decision](https://github.com/bdonaldharris/builder-garage/issues/8#issuecomment-6817923896) item 3, [GOVERNANCE: Product-intent acceptance](../GOVERNANCE.md#product-intent-acceptance) |

### E10: Manuals and conditional stack or provider guidance

| | |
| --- | --- |
| **Allowed values** | A list of adopted [manuals](../manuals/README.md), each with the version adopted, and a list of enabled conditional stack or provider guidance. |
| **Default** | No manual adopted. Conditional guidance carried in the canonical skills applies only when its own stated condition holds. An example is the Supabase guidance, which applies when a change creates a Supabase table. |
| **Constraint** | Guidance only, never a requirement. Adopting guidance never relaxes a floor rule or a canonical skill gate. |
| **Source** | [Manuals: How manuals are used](../manuals/README.md#how-manuals-are-used), [architecture: Conceptual components](architecture.md#conceptual-components), [#6 direction](https://github.com/bdonaldharris/builder-garage/issues/6#issuecomment-6817083027) item 3 |

## Not configurable

These are deliberately not extension points:

- **Every responsibility floor rule.** A project may adopt stricter rules in its own governance. It cannot weaken a floor rule through configuration.
- **Where review evidence is recorded.** The implementation review is recorded on the originating issue, which is authoritative. A pull-request review, when one is triggered, is recorded on the pull request and is supplemental ([F5](responsibility-floor.md#rules)).
- **The three-question review contract** ([F13](responsibility-floor.md#rules)).
- **The two minimum PR-review triggers** ([F10](responsibility-floor.md#rules)).
- **Which vendor, model, or tool fills a role.** Any may fill a role that it can carry out, so there is nothing to configure ([F9](responsibility-floor.md#rules)).

## Not registered in this version

- **Skill installation targets.** These are a Garage tooling setting, not a responsibility extension point. They live in the separate `tooling` section of the configuration, governed by [Tooling configuration](tooling-configuration.md) rather than by this registry ([#35 decision](https://github.com/bdonaldharris/builder-garage/issues/35#issuecomment-6842152164), item 1). They grant no workflow authority.
- **Developer-local history settings.** Developer-local history is private to each developer and is never project configuration ([#19 decision](https://github.com/bdonaldharris/builder-garage/issues/19#issuecomment-6837362562)).

## Changing the registry

The registry changes only through the normal [contribution process](../CONTRIBUTING.md), as a framework change. Adding, removing, or changing an extension point increments the version above. An identifier is never reused. A new extension point must not weaken a floor rule.
