# Filling Crew Roles

**Status:** Canonical guide, non-normative. It explains how people, agents, or a mix of the two fill the [Crew](README.md) roles in any project that adopts Builder Garage. It adds nothing. The canonical [skills](../skills/), the [responsibility floor](../docs/responsibility-floor.md), and the project's governance govern (Builder Garage: [GOVERNANCE.md](../GOVERNANCE.md)). Where this guide and one of them differ, the source governs, and the difference is a defect in this guide.

## Who can fill a role

Any person, agent, or tool can fill a Technician role or a workflow position, as long as it meets that role's contract. The contract includes the role's durable record ([F9](../docs/responsibility-floor.md#rules)). A person who fills a role writes the same record an agent would, with the same marker and the same `garage-evidence` block ([Review evidence](../docs/review-evidence.md#evidence-block-garage-evidence-v1)). No separate format exists for people.

**Agent #1** and **Agent #2** are workflow positions, not kinds of agent. The skills address "the agent", but a person can fill either position.

Filling a Technician role is not exercising a human authority. Accepting a change or its product intent stays with the configured human authorities ([E8](../docs/extension-points.md#e8-human-authorities)). Independent review and human acceptance are distinct gates ([F11, F12](../docs/responsibility-floor.md#rules)).

## Roles and where they stop

Each entry gives what the role answers, where its responsibility ends, and the record it leaves. Follow the links for the procedure.

### Technicians

- **`discovery`** answers: what does the existing system actually do?
  - **Ends** when the discovery report is durably recorded and verified. It changes nothing, and it does not authorize implementation.
  - **Record:** the discovery report on the originating issue, or on a durable artifact linked from it. See [engineering-discovery-workflow: Durable discovery documentation](../skills/engineering-discovery-workflow/SKILL.md#durable-discovery-documentation).
- **`readiness`** answers: is this change understood well enough to implement?
  - **Ends** at one decision, `READY` or `NOT READY`. A `READY` report becomes context for the implementer, and it waives no later gate. See [implementation-readiness-workflow: Readiness decision](../skills/implementation-readiness-workflow/SKILL.md#readiness-decision) and [Handoff to implementation](../skills/implementation-readiness-workflow/SKILL.md#handoff-to-implementation).
  - **Record:** when recorded, the readiness report on the governing issue. See [Readiness output](../skills/implementation-readiness-workflow/SKILL.md#readiness-output).
- **`planner`** answers: how should approved work be framed and entered into the backlog, how should proposed project intent be drafted for acceptance, and how should a Builder's intent be expressed as a prompt?
  - **Ends,** for an issue, when the authorized issue is created and read back. The issue starts nothing, and the planner makes no product, workflow, or acceptance decision. See [issue-authoring-workflow: Executing role](../skills/issue-authoring-workflow/SKILL.md#executing-role).
  - **Record:** the issue itself, ending with its authoring record. See [Authoring record](../skills/issue-authoring-workflow/SKILL.md#authoring-record). The other Technicians author their own follow-ups in the same way, under each skill's "Issue authoring" section.
  - **Ends,** for a spec, at the draft and its hand-off. It writes no file, and the spec lands only through a reviewed change. See [project-spec-workflow: Executing role](../skills/project-spec-workflow/SKILL.md#executing-role).
  - **Record:** the draft, either as a verified comment on a governing issue or in the conversation only, which is not durable. The Builder chooses. See [Project specs](../docs/project-specs.md).
  - **Ends,** for a prompt, when the prompt, any gaps, or a refusal is handed to the Builder. It runs nothing, and a prompt never declares that work is ready. See [prompt-authoring-workflow: Executing role](../skills/prompt-authoring-workflow/SKILL.md#executing-role).
  - **Record:** none. The prompt is displayed in the conversation, and saved to a file only when the Builder asks. A saved prompt is not a workflow record, and it is committed only through the project's normal review. See [Saving](../skills/prompt-authoring-workflow/SKILL.md#saving).
- **`implementer`** answers for implementation, correction, and integration-PR finalization. It fills **Agent #1** today.
  - **Ends** at every handoff to Agent #2, or at a material blocker. The work stays uncommitted until Agent #2 approves it, or, under [commit-stage review](../skills/application-implementation-workflow/SKILL.md#commit-stage-review), is pushed to a branch that is not proposed. See [application-implementation-workflow: Role separation](../skills/application-implementation-workflow/SKILL.md#role-separation) and [Phase 4](../skills/application-implementation-workflow/SKILL.md#phase-4--commit-push-and-integration-pr).
  - **Record:** `[AGENT #1 — IMPLEMENTATION]` and `[AGENT #1 — CORRECTION]`, each with an evidence block. See [Durable workflow-state comments](../skills/application-implementation-workflow/SKILL.md#durable-workflow-state-comments). On the pull request, it also records an equivalence record for each pushed head. See [integration-pr-review: Artifact equivalence](../skills/integration-pr-review/SKILL.md#artifact-equivalence) and [Durable PR workflow state](../skills/integration-pr-review/SKILL.md#durable-pr-workflow-state).
- **`reviewer`** answers: was the artifact independently shown to be correct? It fills **Agent #2** today.
  - **Ends** at its verified disposition, `APPROVED` or `CHANGES REQUIRED`. It never corrects the artifact. It also stops, without a verdict, when valid Agent #1 completion evidence is missing. See [independent-implementation-review: Prerequisite](../skills/independent-implementation-review/SKILL.md#prerequisite-valid-agent-1-completion-evidence) and [Invocation boundary](../skills/independent-implementation-review/SKILL.md#invocation-boundary).
  - **Record:** `[AGENT #2 — INDEPENDENT REVIEW]` with an evidence block. It is posted on the originating issue, where it is authoritative, or on the pull request for a triggered PR review, where it is supplemental. See [Review record](../skills/independent-implementation-review/SKILL.md#review-record).

### Workflow positions

- **Agent #1** implements and corrects, then hands the artifact to Agent #2 and stops. It writes only Agent #1 markers ([F4](../docs/responsibility-floor.md#rules)). See [application-implementation-workflow: Executing agent identity](../skills/application-implementation-workflow/SKILL.md#executing-agent-identity).
- **Agent #2** reviews the complete artifact independently, records its disposition, and stops. It writes only the Agent #2 marker. See [independent-implementation-review: Executing agent identity](../skills/independent-implementation-review/SKILL.md#executing-agent-identity).

`discovery`, `readiness`, and `planner` hold no workflow position.

[Review evidence: Where each record lives](../docs/review-evidence.md#where-each-record-lives) lists every record type, where it is posted, and what it links to.

## Separate execution contexts

The same execution context never both implements and independently reviews the same artifact ([F1, F2](../docs/responsibility-floor.md#rules)). Each invocation holds one role and stops at its handoff ([F3](../docs/responsibility-floor.md#rules)). In practice, separate execution contexts can be:

- two sessions of one tool, where the review session does not carry the implementation session's conversation;
- two different tools;
- two people.

Separation is a recorded assertion. No tool certifies it, and Garage never claims to ([F2](../docs/responsibility-floor.md#rules), [Review evidence: What can be checked](../docs/review-evidence.md#what-can-be-checked-and-what-remains-asserted)). `garage status` reports it as asserted ([README: Workflow status](../README.md#workflow-status)).

## Contributor-held separation

One operator, a person, may run the implementer and the reviewer as two separate invocations in separate execution contexts, and post both records.

It stays consistent with the floor when:

- **F1.** The implementing context never reviews. The operator starts the reviewer in a new context that has not seen the implementation conversation, and the reviewer inspects the artifact itself, not Agent #1's summary.
- **F2.** The two contexts are separate. They may use the same tool or model. The review record asserts the separation, and no tool checks it.
- **F3.** Each invocation stops at its handoff. The implementer stops at `READY FOR AGENT #2 REVIEW`. The operator then starts the reviewer. The implementer does not continue into review.

Both records may be posted from the same GitHub account. That is expected, and it is why role and separation are recorded as assertions ([Review evidence: What does not change](../docs/review-evidence.md#what-does-not-change)).

## Collaboration patterns

These are illustrations. Each follows the same skills and leaves the same records.

### One person with two agent sessions

The person runs Agent #1 in one session and Agent #2 in another, as in [Contributor-held separation](#contributor-held-separation). Each session is invoked with its own skill, and each posts its own record. The person decides when to start the next session. No session waits for, or starts, another one.

### A person implementing, an agent reviewing

The person fills Agent #1 by hand. They implement, run the validation, compute the artifact identity with the [reference procedure](../docs/review-evidence.md#reference-procedure), and post `[AGENT #1 — IMPLEMENTATION]` with its evidence block. They then start an agent as Agent #2, under `independent-implementation-review`, in the same workspace, so it can recompute the identity from the uncommitted working tree. The agent's record is the review. The person does not edit or replace it.

### One person implementing, another person reviewing

On the default working-tree path, this works only when the reviewer can reach the implementer's uncommitted working tree, for example in the same workspace or environment. The implementation record identifies a working-tree artifact. Agent #2 confirms it by recomputing the identity at the same stage before reviewing ([Prerequisite, check 5](../skills/independent-implementation-review/SKILL.md#prerequisite-valid-agent-1-completion-evidence)). A working-tree artifact is checkable only by someone who holds that working tree ([Review evidence: What can be checked](../docs/review-evidence.md#what-can-be-checked-and-what-remains-asserted)).

When the reviewer cannot reach that working tree, the implementer uses [commit-stage review](../skills/application-implementation-workflow/SKILL.md#commit-stage-review), when the issue or the person running the implementation asks for it. The implementer commits and pushes the change to a branch without opening a pull request, and records it at `stage: commit`. The reviewer fetches that commit, recomputes its identity, and reviews it ([#99 decision](https://github.com/bdonaldharris/builder-garage/issues/99#issuecomment-6883827812)).

`garage status` counts a workflow record only when its author is an owner, member, or collaborator of the repository, and reports the others as ignored ([README: Workflow status](../README.md#workflow-status)).

## Worked example

An illustration of one issue, #N, from implementation through review to the pull request, on the default working-tree path. One operator runs every Technician step in a new session of the same coding agent. The tool is an example only. Any person, agent, or tool could fill each step.

| Step | Role and position | Invocation (example) | Record left | Where |
| --- | --- | --- | --- | --- |
| 1 | `implementer`, Agent #1 | `Implement #N using application-implementation-workflow` | `[AGENT #1 — IMPLEMENTATION]`, status `ready-for-review`, `stage: working-tree` | Issue #N |
| 2 | `reviewer`, Agent #2 | `Review #N using indep-review` | `[AGENT #2 — INDEPENDENT REVIEW]`, verdict `changes-required` | Issue #N |
| 3 | `implementer`, Agent #1 | `Correct #N using application-implementation-workflow` | `[AGENT #1 — CORRECTION]`, status `ready-for-re-review` | Issue #N |
| 4 | `reviewer`, Agent #2 | `Re-review #N using indep-review` | `[AGENT #2 — INDEPENDENT REVIEW]`, verdict `approved` | Issue #N |
| 5 | `implementer`, Agent #1 | `Open the PR for #N using pr-review` | `[AGENT #1 — IMPLEMENTATION]` equivalence record for the pushed head, `equivalent_to` the step 4 approval | The pull request |
| 6 | Change acceptor and product-intent acceptor (people) | — | Acceptance, as the project's governance records it | The pull request |

1. The implementer works in the issue branch, leaves the change uncommitted, records its identity, and stops ([Mandatory hold point](../skills/application-implementation-workflow/SKILL.md#mandatory-hold-point)).
2. A new session reviews. It first recomputes the `changeset` and confirms that it matches step 1. It finds a blocker, records `CHANGES REQUIRED`, and stops.
3. A new implementer session corrects the change, records the new identity, and stops.
4. A new reviewer session reviews the complete corrected artifact, not only the correction ([F6](../docs/responsibility-floor.md#rules)). It approves and stops.
5. A new implementer session confirms that the working tree still has the approved `changeset`. It commits exactly that change, pushes it, opens the pull request against the integration branch ([E1](../docs/extension-points.md#e1-integration-branch)), and records equivalence. No PR-review trigger applies, so no PR review follows ([integration-pr-review: PR-review triggers](../skills/integration-pr-review/SKILL.md#pr-review-triggers)).
6. The configured human authorities accept the change and its product intent. The pull request merges after its remaining gates pass.

Builder Garage example: the integration branch is `staging`, a maintainer approves the pull request, and the Workflow Owner accepts product intent ([CONTRIBUTING: How a substantive change proceeds](../CONTRIBUTING.md#how-a-substantive-change-proceeds)).

## What this guide does not change

This guide adds no role, gate, ceremony, record, or authority. It does not assign, invoke, or schedule anyone. New Technicians are added only through a framework change ([Crew](README.md#technicians)).
