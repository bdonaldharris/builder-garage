# Gate Map

This page traces, in lifecycle order, every gate a change passes between its issue and its merge. For each gate it gives the question the gate answers, who acts, the record it needs and where that record lives, what kind of rule sets it, and the section that defines it.

**This map is non-normative.** It creates no rule. Each gate is defined by the source it links, and where this page and a source differ, the source governs. It is not a checklist: the [review contract](../README.md#review-contract) is the same three questions for every change ([F13](../docs/responsibility-floor.md#rules)), and nothing here adds a requirement, gate, or authority.

## How to read an entry

Each entry names its source by kind, one or more of:

- **Floor rule** (F1–F14): fixed or extend-only, whatever a project configures. See the [responsibility floor](../docs/responsibility-floor.md#rules).
- **Extension point** (E1–E10): a value a project may configure, within the constraint the registry states. See the [extension-point registry](../docs/extension-points.md).
- **Skill or governance rule**: set by a canonical skill or by governance, and not adaptable through configuration, because "Anything the registry does not declare is not adaptable" ([Precedence](../docs/responsibility-floor.md#precedence)).

Every record named below is durable on the governing GitHub issue or pull request, written and then verified. A conversation, a local file, or local history is never the record ([F8](../docs/responsibility-floor.md#rules)).

Agent #1 and Agent #2 are workflow positions, filled today by the implementer and the reviewer ([Crew: Workflow positions](../crew/README.md#workflow-positions)). When people do the work without agents, the same sequence applies ([CONTRIBUTING: Working with agents](../CONTRIBUTING.md#working-with-agents)). Any vendor, model, or tool may fill a role that it can carry out ([F9](../docs/responsibility-floor.md#rules)).

## Order and conditions

Gates 1 to 6 run in sequence. Two apply only under a condition: readiness (gate 1), and the supplemental PR review (gate 6). Correction (gate 4) repeats while a review requires changes.

Gates 7 to 11 are conditions of the merge decision. They hold together on the current pull request, not one after another ([integration-pr-review: Phase 5](../skills/integration-pr-review/SKILL.md#phase-5--merge-decision)). Gate 11's inspection applies only when its trigger holds.

## The gates

### 1. Readiness, where it applies

| | |
| --- | --- |
| **Question** | Is the change understood well enough to implement responsibly? |
| **Who acts** | The `readiness` Technician. It applies to non-trivial or uncertain changes. A small, already-understood change may skip a separate pass, which does not waive the readiness concerns. |
| **Record** | When a readiness report is recorded, it is a comment on the governing issue. |
| **Source kind** | Skill rule. Not configurable. Discovery, a separate investigation, has its applicability set at [E5](../docs/extension-points.md#e5-discovery-applicability). |
| **Defined in** | [implementation-readiness-workflow: Applicability](../skills/implementation-readiness-workflow/SKILL.md#applicability), [CONTRIBUTING: Discover when warranted](../CONTRIBUTING.md#2-discover-when-warranted) |

### 2. Agent #1 hold point

| | |
| --- | --- |
| **Question** | Is the implementation complete and ready for independent review, and not proposed? By default it is uncommitted; under commit-stage review it is pushed to a branch with no pull request. |
| **Who acts** | Agent #1, the implementer. Reaching review is a terminal handoff: Agent #1 stops ([F3](../docs/responsibility-floor.md#rules)). |
| **Record** | `[AGENT #1 — IMPLEMENTATION]`, with its evidence block identifying the artifact, on the governing issue. |
| **Source kind** | Floor rules [F3, F4, F5, F7](../docs/responsibility-floor.md#rules). |
| **Defined in** | [application-implementation-workflow: Mandatory hold point](../skills/application-implementation-workflow/SKILL.md#mandatory-hold-point), [Review evidence](../docs/review-evidence.md), [CONTRIBUTING: Implement](../CONTRIBUTING.md#3-implement) |

### 3. Independent implementation review

| | |
| --- | --- |
| **Question** | The three questions: acceptance criteria met? New bug or regression introduced? Does it work? `Yes -> No -> Yes` is approval. |
| **Who acts** | Agent #2, the reviewer, in an execution context separate from the implementation. It is never the implementer. |
| **Record** | `[AGENT #2 — INDEPENDENT REVIEW]` on the originating issue. This record is authoritative, and it decides whether the change may be proposed as a pull request. Where review is recorded is not configurable. |
| **Source kind** | Floor rules [F1, F2, F5, F7, F13](../docs/responsibility-floor.md#rules). The one exception to F5 is a small correction that the configured human authority explicitly classifies as one ([E4](../docs/extension-points.md#e4-small-correction-entry-path)). An agent may never classify a change, or exempt itself. |
| **Defined in** | [independent-implementation-review: Review record](../skills/independent-implementation-review/SKILL.md#review-record), [GOVERNANCE: Review requirements](../GOVERNANCE.md#review-requirements), [CONTRIBUTING: Obtain independent review](../CONTRIBUTING.md#4-obtain-independent-review) |

### 4. Correction and complete re-review

| | |
| --- | --- |
| **Question** | Does the complete corrected artifact answer the three questions `Yes -> No -> Yes`? |
| **Who acts** | Agent #1 corrects, in a new invocation. Agent #2 re-reviews the complete corrected artifact, in its own. |
| **Record** | `[AGENT #1 — CORRECTION]`, then a new `[AGENT #2 — INDEPENDENT REVIEW]`. Each correction record goes where the disposition it addresses was posted: the originating issue, or the pull request after a PR review. |
| **Source kind** | Floor rule [F6](../docs/responsibility-floor.md#rules): a correction does not restore approval; only the re-review does. |
| **Defined in** | [application-implementation-workflow: Phase 3](../skills/application-implementation-workflow/SKILL.md#phase-3--correction-loop), [independent-implementation-review: Correction re-review](../skills/independent-implementation-review/SKILL.md#correction-re-review), [CONTRIBUTING: Correct and re-review](../CONTRIBUTING.md#5-correct-and-re-review) |

### 5. Commit and pull request

| | |
| --- | --- |
| **Question** | Is exactly the approved artifact what is proposed, whether it is committed now or was already committed under commit-stage review? |
| **Who acts** | Agent #1, in a new invocation after the approval is recorded. |
| **Record** | The pull request, against the configured integration branch, linking the issue and its review record. For each pushed head, an Agent #1 equivalence record on the pull request. |
| **Source kind** | Floor rules [F5, F7](../docs/responsibility-floor.md#rules). [E1](../docs/extension-points.md#e1-integration-branch) sets the destination only. |
| **Defined in** | [application-implementation-workflow: Phase 4](../skills/application-implementation-workflow/SKILL.md#phase-4--commit-push-and-integration-pr), [integration-pr-review: Artifact equivalence](../skills/integration-pr-review/SKILL.md#artifact-equivalence), [CONTRIBUTING: Commit, push, and open a pull request](../CONTRIBUTING.md#6-commit-push-and-open-a-pull-request) |

### 6. PR-review triggers and the supplemental PR review

| | |
| --- | --- |
| **Question** | Does the current head need an Agent #2 PR review? It does when equivalence to the approved artifact cannot be shown, when an automated reviewer reports findings on the head, or when a configured trigger applies. |
| **Who acts** | Agent #1 evaluates the triggers. When one applies, a separately invoked Agent #2 reviews the PR artifact and disposes of every automated finding. |
| **Record** | On the pull request: the equivalence record, and, when a trigger applies, the Agent #2 PR Review Request and Agent #2's PR review. The PR review is supplemental. It never replaces the issue-recorded review. |
| **Source kind** | Floor rule [F10](../docs/responsibility-floor.md#rules), extend-only through [E2](../docs/extension-points.md#e2-additional-pr-review-triggers). An automated reviewer, if one is configured at [E3](../docs/extension-points.md#e3-automated-reviewer), is review input only ([F9](../docs/responsibility-floor.md#rules)). |
| **Defined in** | [integration-pr-review: Two Agent #2 review sequences](../skills/integration-pr-review/SKILL.md#two-agent-2-review-sequences), [PR-review triggers](../skills/integration-pr-review/SKILL.md#pr-review-triggers) |

### 7. Required checks

| | |
| --- | --- |
| **Question** | Do the required checks pass on the pull request? |
| **Who acts** | The project's CI. Agent #1 confirms the result at the merge decision. |
| **Record** | The check results on the pull request. |
| **Source kind** | Governance rule. Which checks are required is a repository setting, not an extension point. |
| **Defined in** | [GOVERNANCE: Changes to `main`](../GOVERNANCE.md#changes-to-main), [integration-pr-review: Phase 5](../skills/integration-pr-review/SKILL.md#phase-5--merge-decision) |

### 8. Environment / Deployment Requirements visibility

| | |
| --- | --- |
| **Question** | Are the non-code actions the artifact needs in its target environment stated, or stated as `None`? The gate requires visibility, not that the actions are already done. |
| **Who acts** | Agent #1 surfaces them during implementation and verifies them before declaring the pull request merge-ready. |
| **Record** | The `Environment / Deployment Requirements` section of the Agent #1 records and of the pull request description. |
| **Source kind** | Skill rule. Not configurable. |
| **Defined in** | [application-implementation-workflow: Environment / Deployment Requirements](../skills/application-implementation-workflow/SKILL.md#environment--deployment-requirements), [integration-pr-review: Environment / Deployment Requirements gate](../skills/integration-pr-review/SKILL.md#environment--deployment-requirements-gate) |

### 9. Release conditions

| | |
| --- | --- |
| **Question** | Does each configured release condition that applies have its required evidence? |
| **Who acts** | Each condition states when it applies and what evidence it requires. Agent #1 confirms the evidence is present at the merge decision. |
| **Record** | The condition's evidence, on the pull request or the governing issue. |
| **Source kind** | Extension point [E6](../docs/extension-points.md#e6-release-and-evidence-conditions), extend-only under [F8](../docs/responsibility-floor.md#rules). A condition adds evidence and never removes a gate. The default is none. |
| **Defined in** | [integration-pr-review: Phase 5](../skills/integration-pr-review/SKILL.md#phase-5--merge-decision) |

### 10. Change-acceptor approval

| | |
| --- | --- |
| **Question** | Does the project accept this change? |
| **Who acts** | The configured change acceptor, a human. |
| **Record** | A GitHub approving review on the pull request. For a pull request authored by a sole maintainer, the lead maintainer may instead record explicit acceptance in a comment on the pull request, after independent approval. That comment is not an approving review. |
| **Source kind** | Floor rule [F11](../docs/responsibility-floor.md#rules). The role is named at [E8](../docs/extension-points.md#e8-human-authorities). |
| **Defined in** | [GOVERNANCE: Review requirements](../GOVERNANCE.md#review-requirements), [GOVERNANCE: Pull requests authored by a sole maintainer](../GOVERNANCE.md#pull-requests-authored-by-a-sole-maintainer), [CONTRIBUTING: Maintainer approval and merge](../CONTRIBUTING.md#7-maintainer-approval-and-merge) |

### 11. Product-intent acceptance and the inspection trigger

| | |
| --- | --- |
| **Question** | Is the result what was intended for its user? When the change affects something a user sees or uses, and inspecting it is practical, the acceptor inspects it in that form first. |
| **Who acts** | The configured product-intent acceptor, a human, who also evaluates the inspection trigger. No agent stands in for it. |
| **Record** | Made as part of the project's acceptance of the change. The sources define no separate record. |
| **Source kind** | Floor rule [F12](../docs/responsibility-floor.md#rules). The acceptor is named at [E8](../docs/extension-points.md#e8-human-authorities), and the inspection trigger is [E9](../docs/extension-points.md#e9-user-facing-inspection-trigger), which a project may extend. |
| **Defined in** | [GOVERNANCE: Product-intent acceptance](../GOVERNANCE.md#product-intent-acceptance), [integration-pr-review: Phase 5](../skills/integration-pr-review/SKILL.md#phase-5--merge-decision) |

## Three distinct decisions

Independent engineering review (gate 3), change-acceptor approval (gate 10), and product-intent acceptance (gate 11) answer different questions, and none substitutes for another.

- Independent review asks whether the change is correct for its written issue. Its approval permits a pull request. It does not authorize merge.
- Change acceptance asks whether the project accepts the change. It does not stand in for a missing independent review ([F11](../docs/responsibility-floor.md#rules)).
- Product-intent acceptance asks whether the result is what was intended for its user. Intent that the issue states is checked by independent review. Intent that it does not state is the human acceptor's to judge, never an agent's ([F12](../docs/responsibility-floor.md#rules)).

A project may name the same governance role for more than one of these human responsibilities ([E8](../docs/extension-points.md#e8-human-authorities)). The decisions stay distinct.

## External contributors

An external contributor follows the review path decided in the [#98 D1 decision](https://github.com/bdonaldharris/builder-garage/issues/98#issuecomment-6873371683), option A: contributor-held separation. The decision defines that path. This map does not.

Option B, commit-stage review of a pushed branch, is available under the [#99 decision](https://github.com/bdonaldharris/builder-garage/issues/99#issuecomment-6883827812). When the reviewer cannot reach the contributor's workspace, the contributor pushes the change to a branch without opening a pull request, and the reviewer reviews that commit. It is used only when the invocation or the issue asks for it ([application-implementation-workflow: Commit-stage review](../skills/application-implementation-workflow/SKILL.md#commit-stage-review)).

## Builder Garage example

*Project-specific. This section describes Builder Garage's own configuration, not the canonical map.*

- **Configured values.** Builder Garage's extension-point values are in [`.garage/config.yaml`](../.garage/config.yaml), and [GOVERNANCE](../GOVERNANCE.md#workflow-owner) explains them. Its integration branch is `main`, and its Workflow Owner, currently the lead maintainer, is the product-intent acceptor.
- **Required checks.** The checks in force are recorded as observed, not decided, in [Repository configuration: Required checks](../docs/repository-configuration.md#required-checks).
- **Sole maintainer.** While the lead maintainer is the only maintainer, gate 10 for the lead maintainer's own pull requests may be recorded as an explicit acceptance comment. The repository's ruleset that permits this is recorded, also as observed and not decided, in [Repository configuration: Sole maintainer](../docs/repository-configuration.md#sole-maintainer).
- **Small corrections.** Builder Garage enables [E4](../docs/extension-points.md#e4-small-correction-entry-path). See [CONTRIBUTING: Small corrections](../CONTRIBUTING.md#small-corrections).
