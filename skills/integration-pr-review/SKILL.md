---
name: integration-pr-review
description: Shorthand: pr-review. Establish, gate, and finalize an implementation pull request using the narrow three-question contract: original acceptance criteria met, no new bug/regression introduced, and changed behavior works. Executes as Agent #1. Treat the issue-recorded Agent #2 implementation review as authoritative, request a supplemental role-based Agent #2 PR review only when its triggers apply, terminate Agent #1 at every Agent #2 handoff, and treat any automated reviewer as optional input, never as review authority.
---

# Integration PR Review

## Executing agent identity

**You are Agent #1 — Implementation Agent performing the integration/PR-finalization responsibilities assigned to Agent #1.**

This skill invocation establishes your agent identity for the entire execution.

**A skill invocation has one agent identity. That identity remains fixed for the entire execution. The executing agent may not change roles during the invocation.**

As Agent #1 in this skill, you own the implementation-side PR responsibilities assigned by this workflow, including:

- establishing and verifying the PR artifact;
- determining whether an Agent #2 PR review is required, and posting the Agent #2 PR Review Request when it is;
- recording Agent #1 implementation/correction state;
- performing validated corrections after a separately invoked Agent #2 requires them;
- committing, pushing, and finalizing approved corrections;
- merge/finalization actions assigned to Agent #1 after the required reviews and gates are satisfied.

You must **not**:

- perform Agent #2's independent review or disposition of any review finding;
- create `[AGENT #2 — INDEPENDENT REVIEW]`;
- treat automated-review findings, severity, or silence as Agent #2 approval;
- approve the current artifact on Agent #2's behalf;
- change identity and continue through an Agent #2 stage in the same invocation;
- wait for, poll, monitor, sleep/retry for, or autonomously continue into a future Agent #2 or automated-review state after a handoff has been posted.

**Reaching another agent's workflow responsibility is a handoff boundary, not an instruction for the current agent to continue by assuming that responsibility.**

A transition from Agent #1 to Agent #2 requires a separate agent invocation.

## Purpose

This skill governs review and merge gating for implementation pull requests targeting the designated integration branch.

Its job is not to perform a general application audit.

Its job is to answer whether the current PR should merge.

## Invocation model

The pull request and its governing issue are the durable system of record for PR-specific requirements and workflow history/state. The current PR head/artifact remains authoritative for what actually exists.

This skill owns the reusable integration-PR procedure. The invocation prompt does not need to restate the review contract, prior Agent #1/Agent #2 execution state, review-disposition history, or workflow rules already recorded on the issue, the PR, and here.

When the agent is already operating in the correct repository workspace, a minimal invocation is sufficient, for example:

```text
review PR #1501 using pr-review
```

Resolve `pr-review` to this canonical skill, retrieve/read the PR and governing issue, and execute this workflow against the current PR artifact.

The governing rule is:

```text
Acceptance criteria met? Yes
New bug/regression introduced? No
Does it work? Yes

= MERGE
```

---

# Two Agent #2 review sequences

Agent #2 performs two distinct review sequences:

- **Implementation review.** Before commit, Agent #2 independently reviews the complete implementation under `independent-implementation-review` and records its disposition on the **originating issue**. That record is authoritative. It determines whether the implementation may be committed and proposed as a PR.
- **PR review.** When a PR-review trigger applies (see [PR-review triggers](#pr-review-triggers)), Agent #2 separately reviews the PR artifact under `independent-implementation-review` and records its disposition on the **PR itself**. This skill governs the PR sequence and is executed by Agent #1. Agent #2 never performs its review under this skill.

A PR review is **supplemental**. It never replaces, substitutes for, or retroactively satisfies the issue-recorded implementation review. A PR whose change has no approving issue-recorded implementation review is not eligible for a PR review. It returns to the implementation workflow.

Agents execute these reviews in their own harnesses. This skill defines the contract, the triggers, and the required durable evidence. It does not orchestrate, schedule, or run agents.

## PR-review triggers

The canonical skill defines **two minimum PR-review triggers**. An Agent #2 PR review is required for the current PR head when either applies:

1. **Artifact equivalence cannot be shown.** The current head cannot be shown to be artifact-equivalent to the latest Agent #2-approved artifact: the implementation approved on the originating issue, or a correction approved in an Agent #2 PR review. See [Artifact equivalence](#artifact-equivalence). Examples: commits added after approval, a rebase or conflict resolution that changed content, files added or dropped, or implementation changes made after approval.
2. **Automated review findings exist.** An optional automated reviewer reported findings on the current head. Findings are review inputs only, so they require Agent #2 disposition before merge.

A project may record **additional** PR-review triggers, in its adoption record or, once one exists, in its configuration. Recorded triggers extend the two minimum triggers. They are not part of the canonical default, and a project may not remove either minimum trigger.

When neither minimum trigger applies and no project-recorded trigger applies, no Agent #2 PR review is required. The PR proceeds on the issue-recorded implementation review, recorded artifact-equivalence evidence, and the remaining merge gates: CI, human approval, and Environment / Deployment Requirements.

## Artifact equivalence

Agent #2 approves an artifact, and usually an uncommitted one: the implementation reviewed on the issue, or a correction re-reviewed on the PR. Committing and pushing that artifact creates a new head SHA that Agent #2 never saw. An approval therefore rests on **artifact equivalence**, not on head-SHA identity.

For every head it pushes, Agent #1 records on the PR:

- the resulting head SHA;
- the Agent #2 approval the head is equivalent to, as a link to that disposition;
- the equivalence evidence: the head's changed files and their content, relative to its base, match the approved artifact exactly. For example, the file list and diff of the pushed commits match the reviewed working-tree diff, and no file was changed after the approving disposition.

A head shown equivalent carries the approval forward. A head that is not shown equivalent fires trigger 1.

---

# Role authority and automated review

Review authority comes from the Agent #2 role, performed in a separate execution context, and from its durable record. It never comes from tool choice.

- An automated reviewer, such as Codex or another tool, is **optional review input** where a project uses one. No GitHub-hosted agent reviewer and no particular vendor integration is required.
- Automated findings are evidence, not disposition. Agent #2 determines each finding's disposition under `independent-implementation-review` using the three-question contract.
- Automated severity, blocker labels, and requested corrections are not inherited. An automated reviewer returning no findings does not authorize Agent #1 to declare approval, and it is not an Agent #2 review.
- Agent #1 receives correction work only after Agent #2 determines that correction is required.

If a project uses an automated reviewer, Agent #1 may request its review with that tool's own trigger and the bounded instructions under [Optional automated review request](#optional-automated-review-request). The tool's trigger is a project choice. This skill does not require it.

---

# Durable PR workflow state

PR workflow state must be recorded on the PR, so subsequent one-line agent invocations can continue without reconstructing context from conversation history.

Use the existing structured markers:

- Agent #1 owns `[AGENT #1 — IMPLEMENTATION]`
- Agent #1 owns `[AGENT #1 — CORRECTION]`
- Agent #2 owns `[AGENT #2 — INDEPENDENT REVIEW]`

An agent must never create a workflow-state comment representing the other agent.

Do not assume the absolute newest PR comment is the relevant workflow state. PRs may contain human comments, automated-review comments, inline review comments, CI activity, and other discussion.

For the current phase:

1. inspect the PR and governing issue;
2. identify the latest relevant workflow-state comment by marker;
3. inspect the current PR head/artifact;
4. use applicable prior workflow-state comments as historical context;
5. perform the action defined by this skill;
6. record the resulting Agent #1 or Agent #2 state back to the PR.

The current PR head/artifact is authoritative. Workflow-state comments record what agents did, found, and decided; they do not override source state.

For a PR review, the `[AGENT #2 — INDEPENDENT REVIEW]` disposition must be written to the PR itself. An Agent #2 response that exists only in the agent conversation does not satisfy the durable PR-state requirement.

The Agent #2 PR review is not complete until the PR comment has been successfully written and verified. If that durable write fails, Agent #2 must report the failure rather than claim that the PR review state was recorded.

## Relating a PR disposition to its artifact and evidence

A PR-side Agent #2 disposition applies to the artifact it reviewed. It must record:

- the PR head commit SHA it reviewed, or, for a correction re-review of an uncommitted artifact, the reviewed working-tree state;
- a link to the approving issue-recorded implementation review it supplements;
- the trigger or triggers that required it;
- each automated finding it dispositioned, if any, and the reason, tied to the three governing questions.

The disposition remains sufficient for any later head that Agent #1 shows to be [artifact-equivalent](#artifact-equivalence) to what Agent #2 reviewed. A later head that is not shown equivalent fires trigger 1 and needs a new Agent #2 PR review.

The latest relevant structured comment should provide enough context for the next agent invocation to continue without a manually constructed history prompt.

---

# Governing review contract

Every integration PR review answers only three questions:

1. **Acceptance criteria met?**
2. **New bug/regression introduced?**
3. **Does it work?**

A current PR requires correction only when review demonstrates:

- an acceptance criterion was not met;
- this PR introduced or materially worsened a bug/regression;
- the changed behavior does not work as intended.

Reviewers may inspect enough surrounding code and execution paths to determine those three things.

Do not turn integration review into a general audit of the application.

Diagnostic scenarios, reviewer-generated matrices, automated examples, and hardening ideas do **not** create new acceptance criteria.

A finding becomes blocking only when it demonstrates:

1. an **original issue acceptance criterion** is unmet;
2. the current artifact introduced or materially worsened a bug/regression; or
3. the implemented behavior does not work.

---

# What does not block this PR

Unless directly caused or materially worsened by the current PR, the following are suggestions:

- preexisting bugs or architectural weaknesses;
- historical-data/deployment cleanup;
- future hardening;
- adjacent improvements;
- broader contract completeness unrelated to the changed behavior;
- unrelated test gaps;
- refactoring or consistency opportunities;
- performance concerns outside the changed execution path;
- speculative concurrency concerns not introduced by the change;
- unrelated security observations;
- findings discovered only because the reviewer happened to trace nearby code.

Do not classify these as Integration Blockers or Production Blockers for the current PR.

Do not hold the PR open for them.

Route worthwhile observations to the repository's persistent `Application Improvement Suggestions` issue.

---

# Consolidated suggestion backlog

Each application repository should maintain one persistent backlog issue named:

`Application Improvement Suggestions`

Do not create a standalone GitHub issue for every review observation.

A suggestion entry should contain only:

- originating issue/PR;
- short description;
- why it may be worth considering;
- review-comment reference when available.

Adding a suggestion does not imply severity, priority, production gating, implementation commitment, or a future standalone issue.

A suggestion becomes planned work only when deliberately promoted later.

---

# Relationship to other skills

The normal flow is:

1. Agent #1 implements under `application-implementation-workflow`.
2. Separately invoked Agent #2 performs the implementation review under `independent-implementation-review`, recorded on the originating issue.
3. If `Yes -> No -> Yes`, Agent #1 commits, pushes, and opens the integration PR.
4. This skill governs the PR sequence: equivalence, PR-review triggers, merge gates, and finalization.

Agent #2 remains independent from Agent #1. Every Agent #2 review, including a PR review, is performed under `independent-implementation-review` by a separately invoked agent.

---

# Invocation state machine

At invocation start, inspect the governing issue and its latest Agent #2 implementation-review disposition, PR existence, the current PR head, any automated-review results on that head, and the latest relevant structured Agent #1/Agent #2 workflow-state comments on the PR.

Do not remain active waiting for a future state. A state that does not exist yet is a terminal boundary for the current invocation.

## No PR exists

When no integration PR exists and a durable approving Agent #2 implementation review exists on the originating issue for the exact approved artifact, Agent #1 may:

1. verify the approved artifact and current issue/implementation state;
2. stage only the approved files;
3. commit;
4. push;
5. open the PR against `staging`;
6. record the applicable `[AGENT #1 — IMPLEMENTATION]` state on the PR, including the [artifact-equivalence](#artifact-equivalence) record for the pushed head (resulting SHA, the approving issue review, and the evidence) and which PR-review triggers apply;
7. if any PR-review trigger applies, post the [Agent #2 PR Review Request](#agent-2-pr-review-request), then STOP.

If no trigger applies, the Agent #1 invocation ends after recording the PR state. Merge proceeds later, when CI, human approval, and the remaining gates are satisfied.

**Immediately after posting an Agent #2 PR Review Request, or an optional automated review request, Agent #1 MUST STOP.**

Agent #1 must **not**:

- wait for Agent #2 or an automated reviewer;
- poll GitHub for results;
- sleep/retry while waiting;
- monitor the PR for a future review state;
- inspect or disposition review findings that appear later;
- initiate or simulate Agent #2;
- create an `[AGENT #2 — INDEPENDENT REVIEW]` record;
- merge the PR while a required review is outstanding.

A request to "finalize" does not authorize Agent #1 to wait for or traverse a future review state.

## PR already exists

At invocation start, determine the durable state that actually exists for the exact current PR head:

- **Agent #2 PR review requested, disposition not yet recorded:** STOP immediately. Do not wait or poll.
- **Automated review requested, result not yet returned:** STOP immediately. Do not wait or poll.
- **Automated findings exist on the current head, no Agent #2 PR disposition for that head:** STOP. Trigger 2 applies. Post the Agent #2 PR Review Request if one has not been posted for this head, then stop.
- **The current head is not shown artifact-equivalent to the latest Agent #2-approved artifact, whether issue or PR:** STOP. Trigger 1 applies. Post the Agent #2 PR Review Request for the current head, then stop.
- **Agent #2 returned `CHANGES REQUIRED`:** STOP. Correction requires a separate Agent #1 invocation using `application-implementation-workflow`.
- **Every applicable trigger has an approving Agent #2 PR disposition that the current head is shown equivalent to, or no trigger applies; the current head's equivalence is recorded; and CI, human approval, Environment / Deployment Requirements, and other existing merge gates are satisfied:** Agent #1 may perform the authorized finalization/merge work.

Each STOP above is terminal for the current invocation. Do not wait for the blocking state to change.

---

# Phase 1 — Establish the PR artifact

Confirm:

- repository;
- PR number;
- governing issue, and its latest approving `[AGENT #2 — INDEPENDENT REVIEW]` implementation disposition;
- latest relevant Agent #1 and Agent #2 workflow-state comments on the PR;
- source and target branches;
- current PR head SHA;
- CI/check state;
- that the current head is shown artifact-equivalent to the latest Agent #2-approved artifact, with the equivalence record on the PR;
- that the PR/handoff contains an explicit `Environment / Deployment Requirements` section.

If equivalence cannot be shown, do not resolve it by argument. Trigger 1 applies.

---

# Phase 2 — Determine PR-review triggers

Evaluate the [PR-review triggers](#pr-review-triggers) for the current head:

1. minimum trigger 1: artifact equivalence, against the latest Agent #2-approved artifact;
2. minimum trigger 2: automated findings, if an optional automated reviewer ran;
3. any additional triggers the project has recorded.

Record the result in the Agent #1 PR state. If any trigger applies, post the Agent #2 PR Review Request below as a new top-level PR comment, then STOP.

The governing model for any PR review is:

> **Exhaustive within the authorized issue boundary; narrow outside it.**

The reviewer completes the bounded review of the authorized issue-scoped behavior before returning a disposition, and does not intentionally stop after the first defect. When one demonstrated defect exposes a shared issue-scoped state, transition, authority, recovery, or other behavior rule, the reviewer inspects enough of the directly affected sibling behavior family to identify additional present current-PR blockers caused by the same rule. Report all presently identifiable current-PR blockers together before correction begins.

This does **not** authorize general repository archaeology, unrelated architecture review, speculative hardening, exhaustive combinatorial testing, acceptance-criteria expansion, or backlog growth.

## Agent #2 PR Review Request

Post this complete top-level PR comment when a PR-review trigger applies. It is a request for a separately invoked Agent #2, run in its own harness under `independent-implementation-review`. It invokes no tool or vendor integration.

```markdown
Agent #2 PR review requested

PR head: <full SHA>
Supplements: <link to the approving issue-recorded implementation review>
Trigger(s): <artifact equivalence not shown | automated findings on this head | project-recorded trigger: name>
Automated findings to disposition: <links, or None>

Review the complete current PR artifact at the head above against the governing issue, under independent-implementation-review.

Use only this governing contract:

1. Are the original issue acceptance criteria met?
2. Did this PR introduce or materially worsen a bug/regression?
3. Does the implemented behavior work?

Inspect only the relevant surrounding execution paths needed to answer those three questions. Do not stop after the first defect. If a demonstrated defect exposes a shared issue-scoped state, transition, authority, recovery, or other behavior rule, perform a bounded sweep of the directly interacting states/transitions governed by that same rule. Report all presently identifiable current-PR blockers together.

Disposition every listed automated finding against the three governing questions. Automated severity or reviewer language is evidence, not authority.

Diagnostic scenarios, matrices, reviewer-generated examples, and test ideas do not create new acceptance criteria.

Record the disposition as [AGENT #2 — INDEPENDENT REVIEW] on this PR, including the reviewed head SHA and the issue review it supplements.
```

## Optional automated review request

A project that uses an automated reviewer may request a review of the stable PR head using that tool's own trigger. When it does, give the tool the same governing contract and bounded-review instructions as the Agent #2 PR Review Request. Ask it to separate findings into blocking, non-blocking, and informational. Requesting automated review is optional. Its findings, if any, trigger an Agent #2 PR review. Its silence establishes nothing.

---

# Phase 3 — Agent #2 PR disposition boundary

This phase is **not executable by the Agent #1 invocation** that posted a review request. That invocation already terminated at the handoff.

A separately invoked Agent #2, under `independent-implementation-review`, reviews the complete current PR artifact and dispositions any automated findings.

For every automated finding, Agent #2 independently asks:

1. Does this show an original acceptance criterion was not met?
2. Did this PR introduce or materially worsen a bug/regression?
3. Does this show the implemented behavior does not work?

Agent #2 determines whether each finding:

- **requires correction** because one or more governing questions fail;
- is **non-blocking or informational** because it does not make a governing question fail;
- is **not valid** against the current artifact or governing contract;
- belongs **outside the current issue boundary** and should be handled separately as follow-up, discovery, or backlog work when warranted.

If none of the three governing questions fails, do not correct the PR for that observation.

Agent #2 finishes reviewing the complete current artifact and dispositioning every finding before returning. When a validated finding exposes a directly related issue-scoped behavior family governed by the same demonstrated cause, Agent #2 applies the bounded sibling-blocker semantics from `independent-implementation-review`.

If Agent #2 determines that a finding is outside the current issue boundary, do not expand the current implementation scope. Record that determination in the `[AGENT #2 — INDEPENDENT REVIEW]` PR comment and return the warranted follow-up/discovery/backlog candidate to the workflow orchestrator for separate handling.

Agent #2 writes the structured `[AGENT #2 — INDEPENDENT REVIEW]` comment to the PR, with the information required under [Relating a PR disposition to its artifact and evidence](#relating-a-pr-disposition-to-its-artifact-and-evidence), the three governing answers, validated blockers, finding dispositions, follow-up/out-of-scope determinations, validation, and verdict. Agent #2 verifies that the PR comment was recorded before terminating.

Do not relitigate a previously adjudicated suggestion unless new evidence shows the current PR actually caused or materially worsened the problem.

---

# Phase 4 — Current-PR correction

This phase does not continue inside a prior PR-review invocation. If Agent #2 returns `CHANGES REQUIRED`, correction begins only through a new/separate Agent #1 invocation using `application-implementation-workflow`, after that durable Agent #2 disposition is present on the PR.

If a genuine current-PR blocker exists:

1. Agent #1 reads the latest applicable `[AGENT #2 — INDEPENDENT REVIEW]` PR disposition and inspects the complete current PR artifact;
2. Agent #1 corrects the validated blocker using the coherent correction-boundary rules defined by `application-implementation-workflow`, not merely the single reported permutation;
3. Agent #1 completes the full executable correction required by that coherent issue-scoped behavior boundary, including correction-scoped wiring, contracts, migrations, adapters, services, tests, configuration, and related work required for the corrected artifact to function as intended;
4. Agent #1 continues resolving build/test failures that are correction-scoped, issue-scoped, or introduced by the current artifact. Unrelated pre-existing failures are evidenced and reported separately and do not require correction-scope expansion merely to achieve a globally green suite;
5. Agent #1 runs relevant validation and applies the universal pre-return completion gate from `application-implementation-workflow`;
6. Agent #1 posts a new `[AGENT #1 — CORRECTION]` workflow-state comment to the PR;
7. Agent #1 may return control only at `READY FOR AGENT #2 RE-REVIEW` or `MATERIAL BLOCKER`;
8. at `READY FOR AGENT #2 RE-REVIEW`, Agent #1 stops; a separately invoked Agent #2 performs the complete-artifact correction re-review under `independent-implementation-review` and posts its resulting `[AGENT #2 — INDEPENDENT REVIEW]` disposition to the PR;
9. if that re-review finds another directly related sibling failure in the same behavioral subsystem, apply the churn-triggered bounded model/state-transition analysis before another correction;
10. if the re-review result is `Yes -> No -> Yes`, Agent #2 returns `APPROVED — MERGE` for the reviewed artifact;
11. in a separate Agent #1 finalization invocation, verify the approved artifact, commit and push exactly that approved correction, and record the [artifact-equivalence](#artifact-equivalence) evidence for the resulting head against the approved correction;
12. a head shown equivalent carries the approval forward. Proceed to merge when CI, human approval, Environment / Deployment Requirements, and other existing merge gates are satisfied. No further review is requested on the normal path;
13. if the pushed head is not shown equivalent to the Agent #2-reviewed correction, or implementation code changes after approval, stop. The approval no longer applies, and trigger 1 requires a new Agent #2 PR review.

Correction work must not expand into suggestions or unrelated cleanup.

Partial corrective progress, unresolved correction-scoped wiring, resolvable compile/build failures, resolvable correction-scoped test failures, or a list of remaining correction tasks are not valid reasons for Agent #1 to return control.

Diagnostic state models, scenario matrices, reviewer-generated examples, and automated examples remain evidence. They do not expand the original acceptance criteria.

---

# Post-approval artifact equivalence

After any Agent #2 approval, on the issue or on the PR:

- Agent #1 may commit/push only the exact reviewed artifact;
- no implementation code changes may be made after approval;
- Agent #1 must verify that the pushed artifact is equivalent to the artifact Agent #2 reviewed, and record the [artifact-equivalence](#artifact-equivalence) evidence for the resulting head;
- no further review is requested on the normal path;
- non-code merge-finalization activity already permitted by the workflow may continue.

If implementation code changes after approval, or the pushed artifact does not match what Agent #2 reviewed, the approval no longer applies. The changed artifact returns to Agent #2 review.

Do not use an automated reviewer to resolve artifact-equivalence uncertainty. Only Agent #2 can approve a changed artifact.

---

# Environment / Deployment Requirements gate

Before declaring the PR merge-ready, verify that the reviewed PR artifact or handoff explicitly reports:

```text
Environment / Deployment Requirements
```

with all known non-code actions required for the reviewed artifact to function in the target environment, or exactly:

```text
Environment / Deployment Requirements: None
```

Do not omit the section.

Verify that the surfaced requirements cover the applicable categories:

- SQL migrations or database schema/data changes;
- manual database operations;
- environment variables;
- secrets;
- third-party/provider configuration;
- webhook configuration;
- external resource creation;
- seed/UAT fixture requirements;
- infrastructure/platform configuration;
- restart or redeploy requirements;
- post-deployment verification requirements.

This gate requires **visibility**, not automatic execution of every environment/deployment action before code review can approve the PR.

An unapplied migration, missing staging secret, unconfigured webhook, absent staging/UAT fixture, or pending restart/redeploy is not automatically a code blocker. Treat it as an environment/deployment requirement unless the implementation itself omitted a required migration, configuration contract, provider integration artifact, or other implementation deliverable needed for the feature to function.

Do not invent deployment work through open-ended archaeology. Verify the known requirements carried forward by readiness/implementation and what is evident from the reviewed artifact.

---

# Phase 5 — Merge decision

The PR is ready to merge when:

- an approving Agent #2 implementation review is recorded on the originating issue;
- the current head's artifact-equivalence record shows it equivalent to the latest Agent #2-approved artifact;
- every applicable PR-review trigger has an approving Agent #2 PR disposition that the current head is shown artifact-equivalent to;
- any genuine current-PR blockers were corrected and re-approved by Agent #2 against the complete artifact;
- `Environment / Deployment Requirements` has been explicitly surfaced, including `None` when no non-code actions are required;
- required CI/checks are acceptable;
- required human approval is present.

Return:

```text
APPROVED — MERGE
```

when the governing result is:

```text
Yes -> No -> Yes
```

Suggestions do not prevent merge.

---

# Merge-ready output contract

When the PR satisfies the three-question contract and all required merge gates, return the merge-ready verdict together with the environment/deployment handoff:

```text
APPROVED — MERGE

Environment / Deployment Requirements
- <requirement>
```

or:

```text
APPROVED — MERGE

Environment / Deployment Requirements: None
```

The Environment / Deployment Requirements section is mandatory even when empty.

This output reports operational requirements; it does not by itself mean those actions have already been executed.

---

# Promotion Protection

Promotion should remain intentionally boring.

That means:

- no planned feature implementation;
- no opportunistic refactoring;
- no acceptance-criteria completion;
- no hidden corrective feature-development cycle inside promotion.

New observations may still surface during promotion, but the same discipline applies: determine whether they are actually caused by the release artifact and whether they prevent the release from working as intended.

Do not convert unrelated application improvements into promotion work.

**Zero gum on the bottom of the shoe** means no known unresolved defect that actually prevents the approved release behavior from working safely as intended, not zero suggestions in the backlog.

---

# Direct-to-production implementation exceptions

A direct-to-production implementation PR uses the same three-question contract and the same review sequences.

Because there is no integration buffer, required CI/human release controls still apply, but review does not become a general system audit.

Correct only blockers demonstrated by the current change.

---

# Reviewer discipline

Automated reviewers surface evidence. They do not own disposition. Only a separately invoked Agent #2 determines whether a finding requires correction, is non-blocking/informational, is invalid, or belongs outside the current issue boundary.

Do not:

- treat every automated comment as mandatory work;
- convert suggestions into blockers because the reviewer uses strong wording;
- convert diagnostic scenarios or automated examples into new acceptance criteria;
- ask any reviewer to search broadly for unrelated production concerns;
- hold the PR open for preexisting or adjacent issues;
- require a particular automated reviewer or vendor integration;
- request an Agent #2 PR review when no trigger applies merely because one is customary;
- use an automated reviewer to resolve artifact-equivalence uncertainty;
- wait, poll, sleep/retry, or monitor for an Agent #2 or automated-review state after posting a terminal handoff.

Do:

- treat the issue-recorded Agent #2 implementation review as authoritative;
- record artifact-equivalence evidence for every pushed head, and evaluate every PR-review trigger for the current head;
- post the complete Agent #2 PR Review Request when a trigger applies;
- route every automated finding to a separately invoked Agent #2 for disposition against the three governing questions;
- correct only validated current-PR defects at their coherent issue-scoped behavior boundary;
- use separate complete-artifact Agent #2 re-review for those corrections;
- merge when every required review applies to an artifact the current head is shown equivalent to, and the required checks, human approval, and other existing gates are satisfied.

---

# Workflow summary

```text
Agent #1 implementation
        ↓
separate Agent #2 implementation review, recorded on the issue (authoritative)
        ↓
Yes -> No -> Yes?
   ├── No → coherent issue-scoped correction → Agent #2 re-review on the issue
   └── Yes
          ↓
      commit / push / PR
          ↓
      verify artifact equivalence; evaluate PR-review triggers
          ↓
      any trigger applies?
         ├── No → record PR state → later Agent #1 merge invocation after CI / human approval
         └── Yes → post Agent #2 PR Review Request → STOP
                    ↓
               separate Agent #2 PR review, recorded on the PR (supplemental)
                    ↓
               validated current-PR blocker?
                  ├── No → APPROVED — MERGE for the reviewed head → later Agent #1 merge invocation
                  └── Yes → STOP → separate Agent #1 correction invocation
                             ↓
                          universal pre-return completion gate
                             ├── MATERIAL BLOCKER
                             └── READY FOR AGENT #2 RE-REVIEW
                                        ↓
                                     Agent #2 complete-artifact re-review
                                        ↓
                                     directly related sibling failure?
                                        ├── Yes → bounded churn analysis → coherent correction boundary
                                        └── No
                                        ↓
                                     Yes -> No -> Yes = APPROVED — MERGE
                                        ↓
                                     commit / push exactly approved correction
                                        ↓
                                     verify artifact equivalence
                                        ↓
                                     later Agent #1 merge invocation after remaining gates
```

If the PR head changes after any Agent #2 approval, or the pushed artifact is not equivalent to what was reviewed, the changed artifact returns to Agent #2 review.

---

# Definition of compliance

An integration PR follows this skill only when:

- the skill immediately establishes the executing agent as Agent #1 for the full invocation;
- Agent #1 never changes identity or assumes Agent #2 responsibilities during the invocation;
- the issue-recorded Agent #2 implementation review is treated as the authoritative review record, and a PR review never replaces it;
- a PR without an approving issue-recorded implementation review returns to the implementation workflow;
- the two minimum PR-review triggers (artifact equivalence, automated findings) and any project-recorded additional triggers are evaluated for the current head;
- approvals rest on recorded artifact equivalence, not head-SHA identity: Agent #1 records the resulting SHA and equivalence evidence for every head it pushes, and a head not shown equivalent fires trigger 1;
- an Agent #2 PR review is requested, through the role-based Agent #2 PR Review Request, whenever a trigger applies, and is not required when none does;
- no automated reviewer, GitHub-hosted agent, or vendor integration is required, and none establishes review authority;
- automated findings are review inputs, and every one is dispositioned by Agent #2 before merge;
- every Agent #2 disposition is made by a separately invoked agent under `independent-implementation-review`, never under this skill;
- reaching an Agent #2 review boundary is a hard stop for Agent #1;
- posting an Agent #2 PR Review Request or an optional automated review request is a terminal Agent #1 handoff;
- Agent #1 never waits, polls, sleeps/retries, monitors, or autonomously continues across a review handoff boundary;
- Agent #1 never creates `[AGENT #2 — INDEPENDENT REVIEW]`;
- PR comments using the structured Agent #1/Agent #2 markers are the durable PR workflow record, while the current PR artifact remains authoritative for source state;
- each Agent #2 PR disposition records the reviewed head SHA or working-tree state, the issue review it supplements, its triggers, and its finding dispositions, and remains sufficient for later heads shown artifact-equivalent to it;
- each Agent #2 PR review is complete only after its comment has been recorded and verified on the PR;
- review is governed by the three questions;
- only original-acceptance failures, PR-introduced regressions, or non-working changed behavior require current correction;
- diagnostic scenarios, matrices, automated examples, and reviewer-generated tests do not create new acceptance criteria;
- suggestions do not hold the PR open, and non-blocking observations route to `Application Improvement Suggestions`;
- Agent #1 applies required corrections under the global Agent #1 execution invariant, coherent correction-boundary rules, and universal pre-return completion gate from `application-implementation-workflow`;
- a separately invoked Agent #2 re-reviews the complete corrected artifact only after `READY FOR AGENT #2 RE-REVIEW`, and a directly related sibling failure triggers bounded churn analysis before another correction;
- Agent #1 commits/pushes only the exact Agent #2-approved artifact and verifies equivalence; if implementation changes after approval or equivalence fails, the changed artifact returns to Agent #2 review;
- the PR is not declared merge-ready until `Environment / Deployment Requirements` has been explicitly surfaced, including `None` when applicable;
- environment/deployment actions are classified as operational requirements rather than code blockers unless the implementation omitted a required artifact/contract;
- required CI and human approval still apply;
- Garage defines the contract and required evidence, and agents run in their own harnesses; nothing in this skill orchestrates agent execution;
- promotion remains intentionally boring.
