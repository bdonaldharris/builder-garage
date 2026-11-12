---
name: integration-pr-review
description: Shorthand: pr-review. Review and gate an implementation pull request using the narrow three-question contract: original acceptance criteria met, no new bug/regression introduced, and changed behavior works. Run one substantive Codex review on the stable initial production-bound head, terminate Agent #1 at Codex/Agent #2 handoffs, require separately invoked Agent #2 disposition, and make Agent #2 correction re-review the final substantive approval gate for corrected artifacts.
---

# Integration PR Review

## Executing agent identity

**You are Agent #1 — Implementation Agent performing the integration/PR-finalization responsibilities assigned to Agent #1.**

This skill invocation establishes your agent identity for the entire execution.

**A skill invocation has one agent identity. That identity remains fixed for the entire execution. The executing agent may not change roles during the invocation.**

As Agent #1 in this skill, you own the implementation-side PR responsibilities assigned by this workflow, including:

- establishing and verifying the PR artifact;
- posting the canonical Codex review request when required;
- recording Agent #1 implementation/correction state;
- performing validated corrections after a separately invoked Agent #2 requires them;
- committing, pushing, and finalizing approved corrections;
- merge/finalization actions assigned to Agent #1 after required independent disposition is present.

You must **not**:

- perform Agent #2's independent review or Codex-finding disposition;
- create `[AGENT #2 — INDEPENDENT REVIEW]`;
- treat Codex findings, Codex severity, or Codex silence as Agent #2 approval;
- approve the current artifact on Agent #2's behalf;
- change identity and continue through an Agent #2 stage in the same invocation;
- wait for, poll, monitor, sleep/retry for, or autonomously continue into a future Codex/Agent #2 state after a handoff has been posted.

**Reaching another agent's workflow responsibility is a handoff boundary, not an instruction for the current agent to continue by assuming that responsibility.**

A transition from Agent #1 to Agent #2 requires a separate agent invocation.

## Purpose

This skill governs review and merge gating for implementation pull requests targeting the designated integration branch.

Its job is not to perform a general application audit.

Its job is to answer whether the current PR should merge.

## Invocation model

The pull request and its governing issue are the durable system of record for PR-specific requirements and workflow history/state. The current PR head/artifact remains authoritative for what actually exists.

This skill owns the reusable integration-PR review procedure. The invocation prompt does not need to restate the review contract, prior Agent #1/Agent #2 execution state, Codex disposition history, or workflow rules already recorded on the PR and defined here.

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

# Durable PR workflow state

Production-bound workflow state must be recorded on the PR so subsequent one-line agent invocations can continue without reconstructing context from ChatGPT history.

Use the existing structured markers:

- Agent #1 owns `[AGENT #1 — IMPLEMENTATION]`
- Agent #1 owns `[AGENT #1 — CORRECTION]`
- Agent #2 owns `[AGENT #2 — INDEPENDENT REVIEW]`

An agent must never create a workflow-state comment representing the other agent.

Do not assume the absolute newest PR comment is the relevant workflow state. PRs may contain human comments, Codex comments, inline review comments, CI activity, and other discussion.

For the current phase:

1. inspect the PR and governing issue;
2. identify the latest relevant workflow-state comment by marker;
3. inspect the current PR head/artifact;
4. use applicable prior workflow-state comments as historical context;
5. perform the action defined by this skill;
6. record the resulting Agent #1 or Agent #2 state back to the PR.

The current PR head/artifact is authoritative. Workflow-state comments record what agents did, found, and decided; they do not override source state.

For production-bound review, the `[AGENT #2 — INDEPENDENT REVIEW]` disposition must be written to the PR itself. An Agent #2 response that exists only in the agent conversation does not satisfy the durable PR-state requirement.

The Agent #2 review invocation is not complete until the PR comment has been successfully written and verified. If that durable write fails, Agent #2 must report the failure rather than claim that the PR review state was recorded.

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
2. Separately invoked Agent #2 performs `independent-implementation-review`.
3. If `Yes -> No -> Yes`, Agent #1 commits, pushes, and opens the integration PR.
4. This skill governs the PR review and merge decision.

Agent #2 remains independent from Agent #1.

The authoritative record of Agent #2's implementation review is its disposition on the originating issue, made before commit under `independent-implementation-review`. That record determines whether the implementation may advance. This skill governs a separate PR-review sequence, executed by Agent #1. When an Agent #2 PR review is part of that sequence, Agent #2 performs it under `independent-implementation-review`, never under this skill. Its PR record is supplemental and never replaces, or substitutes for, the issue-recorded implementation review.

Review authority comes from the Agent #2 role and its durable record, not from tool choice.

Automated-review findings are review inputs, not authoritative dispositions. **Codex is a review input, not Agent #2.** Codex review does not constitute Agent #2 review.

Codex identifies findings; separately invoked Agent #2 determines their disposition under `independent-implementation-review` using the same three-question contract.

Do not automatically inherit Codex's blocker classification, severity, or requested correction. Codex returning no findings does not authorize Agent #1 to declare Agent #2 approval. Agent #1 receives correction work only after Agent #2 determines that correction is required.

---

# Invocation state machine

At invocation start, inspect the governing issue, PR existence, current PR head, Codex review state, and the latest relevant structured Agent #1/Agent #2 workflow-state comments.

Do not remain active waiting for a future state. A state that does not exist yet is a terminal boundary for the current invocation.

## No PR exists

When no integration PR exists and a durable Agent #2 approval exists for the exact approved implementation artifact, Agent #1 may:

1. verify the approved artifact and current issue/implementation state;
2. stage only the approved files;
3. commit;
4. push;
5. open the PR against `staging`;
6. record the applicable Agent #1 implementation state on the PR;
7. post the complete canonical top-level Codex Integration Review Request.

**Immediately after posting the canonical Codex review request, Agent #1 MUST STOP.**

The Codex request is a terminal handoff. The current Agent #1 invocation ends after posting the request. A separate invocation is required after Codex returns.

Agent #1 must **not**:

- wait for Codex;
- poll GitHub for Codex results;
- sleep/retry while waiting;
- monitor the PR for a future review state;
- independently inspect or disposition Codex findings that appear later;
- initiate or simulate Agent #2;
- create an `[AGENT #2 — INDEPENDENT REVIEW]` record;
- merge the PR.

A request to "finalize" does not authorize Agent #1 to wait for or traverse a future review state.

## PR already exists

At invocation start, determine the durable state that actually exists for the exact current PR head:

- **Codex requested, result not yet returned:** STOP immediately. Do not wait or poll.
- **Codex findings returned, no Agent #2 disposition for the exact current stable head:** STOP immediately. A separately invoked Agent #2 must disposition the complete artifact and Codex findings.
- **Agent #2 returned `CHANGES REQUIRED`:** STOP. Correction requires a separate Agent #1 invocation using `application-implementation-workflow`.
- **Agent #2 approved a corrected artifact, but the pushed/current artifact is not equivalent to what Agent #2 reviewed or implementation code changed after approval:** STOP. The approval is no longer sufficient; the changed artifact must return to Agent #2 review.
- **Agent #2 approved the corrected artifact, the pushed artifact is verified equivalent to that reviewed artifact, and CI, human approval, Environment / Deployment Requirements, and other existing merge gates are satisfied:** Agent #1 may perform the authorized finalization/merge work without another Codex review.

Each STOP above is terminal for the current invocation. Do not wait for the blocking state to change.

---

# Phase 1 — Establish the PR artifact

Confirm:

- repository;
- PR number;
- latest relevant Agent #1 and Agent #2 workflow-state comments for the current phase;
- governing issue;
- source and target branches;
- current PR head SHA;
- CI/check state;
- that the PR represents the implementation Agent #2 approved;
- that the PR/handoff contains an explicit `Environment / Deployment Requirements` section.

If the pushed artifact differs materially from the artifact Agent #2 approved, stop and resolve that mismatch before merge.

---

# Phase 2 — One normal automated review

Run the repository's normal automated/Codex PR review on the stable implementation head.

For Codex, post a **new top-level PR comment using the Canonical Codex Integration Review Request below**.

`@codex review` is the technical invocation trigger only. A bare `@codex review` by itself is **not** the complete review request required by this workflow and does **not** satisfy this phase.

The governing model is:

> **Exhaustive within the authorized issue boundary; narrow outside it.**

The automated reviewer must complete its bounded review of the authorized issue-scoped behavior before returning findings.

It must not intentionally stop after the first defect.

When one demonstrated defect exposes a shared issue-scoped state, transition, authority, recovery, or other behavior rule, the automated reviewer must inspect enough of the directly affected sibling behavior family to identify additional **present current-PR blockers** caused by the same rule.

Report all presently identifiable current-PR blockers together before correction begins.

This does **not** authorize general repository archaeology, unrelated architecture review, speculative hardening, exhaustive combinatorial testing, acceptance-criteria expansion, or backlog growth.

After Agent #1 posts the complete canonical Codex request, **the current invocation ends immediately**.

Do not wait for Codex, poll GitHub, sleep/retry, monitor the PR, or continue when Codex later returns. A future Codex result requires a separate invocation.

When Codex findings exist, they are durable PR review inputs for a separately invoked Agent #2 under `independent-implementation-review`.

## Canonical Codex Integration Review Request

Use this complete top-level PR comment for the **one substantive Codex review on the stable initial production-bound PR head**.

Do not reuse it after an Agent #2-approved correction. Agent #2 correction re-review is the final substantive review gate for the corrected artifact unless implementation code changes after approval.

```markdown
@codex review

Review the complete current PR artifact against the governing issue and the current stable PR head.

Use only this governing contract:

1. Are the original issue acceptance criteria met?
2. Did this PR introduce or materially worsen a bug/regression?
3. Does the implemented behavior work?

Inspect only the relevant surrounding execution paths needed to answer those three questions.

Do not stop after the first defect.

If a demonstrated defect exposes a shared issue-scoped state, transition, authority, recovery, or other behavior rule, perform a bounded sweep of the directly interacting states/transitions governed by that same rule.

Report all presently identifiable current-PR blockers together before returning the review.

Separate findings into:
- Blocking: demonstrates at least one failure of the three governing questions above.
- Non-blocking: valid observation that does not fail any of the three governing questions.
- Informational: useful context that does not affect merge eligibility.

Diagnostic scenarios, matrices, reviewer-generated examples, and test ideas do not create new acceptance criteria.

Do not expand this review into general repository archaeology, unrelated architecture review, speculative hardening, exhaustive combinatorial testing, backlog growth, or unrelated product behavior.

Automated severity or reviewer language is evidence, not authority. Every finding must be independently dispositioned against the three governing questions.

Review the complete current PR artifact before returning.
```

---
# Phase 3 — Agent #2 disposition boundary

This phase is **not executable by the Agent #1 invocation that posted the Codex request**; that invocation already terminated at the handoff.

After Codex results exist, a separately invoked Agent #2 under `independent-implementation-review` reviews the complete current artifact and dispositions the automated findings.

Codex identifies findings. Agent #2 determines their disposition.

For every automated finding, Agent #2 independently asks:

1. Does this show an original acceptance criterion was not met?
2. Did this PR introduce or materially worsen a bug/regression?
3. Does this show the implemented behavior does not work?

Codex severity, blocker language, or requested correction is not authoritative and must not be inherited automatically.

Agent #2 determines whether each finding:

- **requires correction** because one or more governing questions fail;
- is **non-blocking or informational** because it does not make a governing question fail;
- is **not valid** against the current artifact or governing contract;
- belongs **outside the current issue boundary** and should be handled separately as follow-up, discovery, or backlog work when warranted.

If none of the three governing questions fails, do not correct the PR for that observation.

Do not begin correction after the first automated finding merely because Codex labeled it blocking or severe. Agent #2 must finish reviewing the complete current artifact and dispositioning the complete automated-review result for the current stable head.

When a validated finding exposes a directly related issue-scoped behavior family governed by the same demonstrated cause, Agent #2 applies the bounded sibling-blocker semantics from `independent-implementation-review` before finalizing disposition.

Report all presently identifiable validated current-PR blockers together before correction begins.

If Agent #2 determines that a finding is outside the current issue boundary, do not expand the current implementation scope. Record that determination in the `[AGENT #2 — INDEPENDENT REVIEW]` PR comment and return the warranted follow-up/discovery/backlog candidate to the workflow orchestrator for separate handling.

After dispositioning the complete automated-review result, Agent #2 writes the structured `[AGENT #2 — INDEPENDENT REVIEW]` comment to the PR, including the three governing answers, validated blockers, Codex dispositions, follow-up/out-of-scope determinations, validation, and verdict. Agent #2 must verify that the PR comment was successfully recorded before terminating. Returning the review text only in the agent response does not satisfy this phase.

Do not relitigate a previously adjudicated suggestion unless new evidence shows the current PR actually caused or materially worsened the problem.

---

# Phase 4 — Current-PR correction

This phase does not continue inside the prior PR-review invocation. If Agent #2 returns `CHANGES REQUIRED`, the PR-review invocation is terminal. Correction begins only through a new/separate Agent #1 invocation using `application-implementation-workflow` after that durable Agent #2 disposition is present on the PR.

If a genuine current-PR blocker exists:

1. Agent #1 reads the latest applicable `[AGENT #2 — INDEPENDENT REVIEW]` PR disposition and inspects the complete current PR artifact;
2. Agent #1 corrects the validated blocker using the coherent correction-boundary rules defined by `application-implementation-workflow`, not merely the single reported permutation.
3. Agent #1 completes the full executable correction required by that coherent issue-scoped behavior boundary, including correction-scoped wiring, contracts, migrations, adapters, services, tests, configuration, and related work required for the corrected artifact to function as intended.
4. Agent #1 continues resolving build/test failures that are correction-scoped, issue-scoped, or introduced by the current artifact. Unrelated pre-existing failures must be evidenced and reported separately and do not require correction-scope expansion merely to achieve a globally green suite.
5. Agent #1 runs relevant validation and applies the universal pre-return completion gate from `application-implementation-workflow`.
6. Agent #1 posts a new `[AGENT #1 — CORRECTION]` workflow-state comment to the PR.
7. Agent #1 may return control only at `READY FOR AGENT #2 RE-REVIEW` or `MATERIAL BLOCKER`.
8. at `READY FOR AGENT #2 RE-REVIEW`, Agent #1 stops; a separately invoked Agent #2 performs the complete-artifact correction re-review under `independent-implementation-review` and posts its resulting `[AGENT #2 — INDEPENDENT REVIEW]` disposition to the PR.
9. If that re-review finds another directly related sibling failure in the same behavioral subsystem, apply the churn-triggered bounded model/state-transition analysis before another correction.
10. Agent #2 answers:
   - Original acceptance criteria met?
   - New bug/regression introduced?
   - Does it work?
11. If the result is `Yes -> No -> Yes`, Agent #2 returns:
   `APPROVED — MERGE`
12. In a separate Agent #1 finalization invocation after Agent #2 returns `APPROVED — MERGE`, verify the approved artifact, commit and push exactly that approved correction, and verify that the pushed artifact is equivalent to what Agent #2 reviewed.
13. Do **not** post another Codex review request. Proceed to merge when CI, human approval, Environment / Deployment Requirements, and other existing merge gates are satisfied.
14. If the pushed artifact is not equivalent to the Agent #2-reviewed correction, or implementation code changes after approval, stop. The approval is no longer sufficient and the changed artifact must return to Agent #2 review.

Correction work must not expand into suggestions or unrelated cleanup.

Partial corrective progress, unresolved correction-scoped wiring, resolvable compile/build failures, resolvable correction-scoped test failures, or a list of remaining correction tasks are not valid reasons for Agent #1 to return control.

Diagnostic state models, scenario matrices, reviewer-generated examples, and automated examples remain evidence. They do not expand the original acceptance criteria.

---

# Post-correction artifact equivalence

After Agent #2 returns `APPROVED — MERGE` for a corrected production-bound artifact:

- Agent #2 owns final substantive correction approval;
- Agent #1 may commit/push only the exact reviewed correction;
- no implementation code changes may be made after Agent #2 approval;
- Agent #1 must verify that the pushed artifact is equivalent to the artifact Agent #2 reviewed;
- no additional Codex review is performed on the normal path;
- non-code merge-finalization activity already permitted by the workflow may continue.

If implementation code changes after Agent #2 approval, or the pushed artifact does not match what Agent #2 reviewed, the approval is no longer sufficient. The changed artifact must return to Agent #2 review.

Do not trigger Codex again to resolve artifact-equivalence uncertainty.

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

- acceptance criteria are met;
- the current PR has not introduced an unresolved bug/regression;
- the changed behavior works;
- any genuine current-PR blockers found by automated review were corrected and re-approved by Agent #2 against the complete artifact;
- a durable Agent #2 approval exists for the exact current PR head;
- the pushed artifact matches the artifact/head Agent #2 approved;
- after any validated correction, the pushed artifact has been verified equivalent to the Agent #2-approved correction and no implementation code changed after approval;
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

A direct-to-production implementation PR uses the same three-question contract.

Because there is no integration buffer, required CI/human release controls still apply, but review does not become a general system audit.

Correct only blockers demonstrated by the current change.

---

# Automated reviewer discipline

Automated reviewers surface evidence. They do not own disposition. Codex is not Agent #2. Codex identifies findings; only a separately invoked Agent #2 determines whether those findings require correction, are non-blocking/informational, are invalid, or belong outside the current issue boundary.

Do not:

- treat every automated comment as mandatory work;
- convert suggestions into blockers because the reviewer uses strong wording;
- convert diagnostic scenarios or automated examples into new acceptance criteria;
- ask automated review to search broadly for unrelated production concerns;
- hold the PR open for preexisting or adjacent issues;
- run another Codex review after Agent #2 approves a corrected artifact on the normal path;
- use Codex to resolve artifact-equivalence uncertainty after Agent #2 approval;
- wait, poll, sleep/retry, or monitor for a Codex/Agent #2 state after posting a terminal handoff.

Do:

- run one normal automated review on the stable initial implementation head using the complete Canonical Codex Integration Review Request;
- never treat a bare `@codex review` as the complete workflow request;
- require the automated reviewer to complete the authorized issue-scoped review and consolidate presently identifiable blockers before correction;
- route the complete automated-review result to separately invoked Agent #2 for independent disposition against the three governing questions;
- correct only validated current-PR defects at their coherent issue-scoped behavior boundary;
- use separate complete-artifact Agent #2 re-review for those corrections;
- after Agent #2 returns `APPROVED — MERGE` on a corrected artifact, commit/push only the exact reviewed correction and verify artifact equivalence;
- if implementation code changes after Agent #2 approval or equivalence fails, return the changed artifact to Agent #2 review rather than triggering Codex again;
- merge when the Agent #2-approved corrected artifact remains unchanged and required checks, human approval, and other existing gates are satisfied.

---

# Workflow summary

```text
Agent #1 implementation
        ↓
separate Agent #2 complete-artifact review
        ↓
Yes -> No -> Yes?
   ├── No → coherent issue-scoped correction → Agent #2 re-review
   └── Yes
          ↓
      commit / push / PR
          ↓
      post canonical Codex review request
          ↓
        STOP
          ↓
   separate Agent #2 invocation
          ↓
      Agent #2 disposition recorded on PR
          ↓
      validated current-PR blocker?
         ├── No → later Agent #1 merge invocation after CI / human approval
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
                            no further Codex review
                               ↓
                            later Agent #1 merge invocation after remaining gates
```

If implementation code changes after Agent #2 approval, or the pushed artifact is not equivalent to the reviewed correction, the changed artifact returns to Agent #2 review.

---

# Definition of compliance

An integration PR follows this skill only when:

- the skill immediately establishes the executing agent as Agent #1 for the full invocation;
- Agent #1 never changes identity or assumes Agent #2 responsibilities during the invocation;
- Codex is a review input, not Agent #2, and Codex silence/finding labels never substitute for Agent #2 disposition;
- every Agent #2 disposition requires a separate `independent-implementation-review` invocation;
- reaching an Agent #2 review/disposition boundary is a hard stop for Agent #1;
- posting the initial canonical Codex review request is a terminal Agent #1 handoff; the invocation ends immediately afterward;
- Agent #1 never waits, polls, sleeps/retries, monitors, or autonomously continues across a Codex/Agent #2 handoff boundary;
- when an existing PR is not yet in a state authorized for Agent #1 action, Agent #1 stops rather than waiting for that state to appear;
- Agent #1 never creates `[AGENT #2 — INDEPENDENT REVIEW]`;
- PR comments using the structured Agent #1/Agent #2 markers are the durable workflow-history record while the current PR artifact remains authoritative for source state;
- the latest relevant workflow-state comment is selected by marker/current phase rather than by assuming the newest PR comment is relevant;
- Agent #1 implementation/correction state and Agent #2 review/disposition state are written back to the PR;
- each Agent #2 PR review/disposition is complete only after the `[AGENT #2 — INDEPENDENT REVIEW]` comment has been successfully recorded and verified on the PR itself;
- an Agent #2 response that exists only outside the PR does not satisfy the durable production-bound review state;
- review is governed by the three questions;
- one normal automated review runs on the stable implementation head using the complete Canonical Codex Integration Review Request;
- a bare `@codex review` by itself does not satisfy the workflow;
- the automated reviewer is instructed to review the complete current PR artifact, avoid first-finding termination, and report all presently identifiable current-PR blockers together;
- automated findings are treated as review inputs rather than authoritative dispositions;
- separately invoked Agent #2 determines each Codex finding's disposition against the complete current artifact and the three governing questions;
- Codex severity, blocker labels, and requested corrections do not control Agent #1 correction work;
- out-of-scope findings do not expand the current implementation and are returned to the workflow orchestrator for separate follow-up/discovery/backlog handling when warranted;
- only original-acceptance failures, PR-introduced regressions, or non-working changed behavior require current correction;
- diagnostic scenarios, matrices, automated examples, and reviewer-generated tests do not create new acceptance criteria;
- suggestions do not hold the PR open;
- non-blocking observations route to `Application Improvement Suggestions` instead of automatically becoming standalone issues;
- Agent #1 applies required corrections under the global Agent #1 execution invariant and coherent correction-boundary rules from `application-implementation-workflow`;
- Agent #1 applies the universal pre-return completion gate and returns correction control only at `READY FOR AGENT #2 RE-REVIEW` or `MATERIAL BLOCKER`;
- unrelated pre-existing build/test failures do not force correction-scope expansion when they are evidenced separately;
- separately invoked Agent #2 re-reviews the complete corrected artifact only after `READY FOR AGENT #2 RE-REVIEW`;
- a directly related sibling failure after correction triggers bounded churn analysis before another correction;
- `Yes -> No -> Yes` after correction produces `APPROVED — MERGE`;
- after a validated Codex blocker correction, Agent #2 correction re-review is the final substantive review gate;
- `Yes -> No -> Yes` on the corrected artifact produces `APPROVED — MERGE` without another Codex review;
- Agent #1 may commit/push only the exact Agent #2-approved correction and must verify artifact equivalence;
- if implementation code changes after approval or equivalence fails, the changed artifact returns to Agent #2 review rather than Codex;
- automated findings remain evidence, not authority;
- the PR is not declared merge-ready until `Environment / Deployment Requirements` has been explicitly surfaced, including `None` when applicable;
- environment/deployment actions are classified as operational requirements rather than code blockers unless the implementation omitted a required artifact/contract;
- required CI and human approval still apply;
- promotion remains intentionally boring.
