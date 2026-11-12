# Engineering Principles

This document records the reusable engineering principles behind Builder Garage and how mature each one is. **It adds no requirements.**

What Builder Garage requires is defined by the canonical [skills](../skills/), [GOVERNANCE.md](../GOVERNANCE.md), [CONTRIBUTING.md](../CONTRIBUTING.md), the [architectural direction](architecture.md), and accepted [ADRs](adr/). Where this document and one of those sources differ, the source is authoritative.

The principles come from engineering practice described in [history.md](history.md).

## Status

| Status | Meaning |
| --- | --- |
| **Established** | Already required. Listed here with a link to the source that requires it, and not restated. |
| **Candidate** | Supported by concrete engineering evidence. Not required. Reviewers and contributors may use it as a judgment aid. Not following an unapproved candidate is not, by itself, a blocking finding. An actual defect found by applying a candidate is still judged under the established [review contract](../README.md#review-contract), like any other defect. |
| **Hypothesis** | Supported by experience, not yet resolved. |
| **Open question** | Tracked as a GitHub issue. |

A candidate becomes a requirement only through the normal [contribution process](../CONTRIBUTING.md): an issue, independent review, and maintainer approval, with the requirement written into the canonical skill or governance document that owns it. Appearing here does not make anything a requirement.

## Established

| Principle | Required by |
| --- | --- |
| Make engineering expectations explicit instead of relying on memory or convention. Durable artifacts carry context: the Issue/PR holds *what*, the skill holds *how*, and the prompt holds *action and location*. | [README: Operating model](../README.md#operating-model), [CONTRIBUTING: Changing skills and standards](../CONTRIBUTING.md#changing-skills-and-standards) |
| Understand a non-trivial change before implementing it. | [implementation-readiness-workflow](../skills/implementation-readiness-workflow/SKILL.md), [engineering-discovery-workflow](../skills/engineering-discovery-workflow/SKILL.md) |
| The implementer never approves their own work. Independent review happens before commit, so the pull request is not the first serious review. | [GOVERNANCE: Review requirements](../GOVERNANCE.md#review-requirements), [independent-implementation-review](../skills/independent-implementation-review/SKILL.md) |
| Corrections do not restore approval. Only independent re-review does. | [GOVERNANCE: Review requirements](../GOVERNANCE.md#review-requirements), [application-implementation-workflow](../skills/application-implementation-workflow/SKILL.md) |
| Correct the demonstrated cause, not only the reported permutation, within the issue's scope. | [application-implementation-workflow: Coherent correction boundary](../skills/application-implementation-workflow/SKILL.md) |
| When a defect exposes a shared state or transition rule, review the directly related sibling behavior. Do not audit beyond it. | [independent-implementation-review](../skills/independent-implementation-review/SKILL.md), [application-implementation-workflow: Correction-churn trigger](../skills/application-implementation-workflow/SKILL.md) |
| An integration gate validates work before promotion. Promotion stays intentionally boring, and Promotion Protection keeps it so. | [integration-pr-review](../skills/integration-pr-review/SKILL.md), [application-implementation-workflow](../skills/application-implementation-workflow/SKILL.md), [GOVERNANCE: Changes to main](../GOVERNANCE.md#changes-to-main) |
| In discovery and readiness, "Unknown" is a legitimate result. Do not convert it into an assumption. | [engineering-discovery-workflow](../skills/engineering-discovery-workflow/SKILL.md), [implementation-readiness-workflow](../skills/implementation-readiness-workflow/SKILL.md) |
| Refuse unnecessary engineering: no speculative abstraction, and no technology without an established requirement. | [implementation-readiness-workflow](../skills/implementation-readiness-workflow/SKILL.md), [architecture: Deliberately undecided](architecture.md#deliberately-undecided) |
| People and agents follow the same engineering discipline. | [CONTRIBUTING: Working with agents](../CONTRIBUTING.md#working-with-agents) |

## Candidates

### Evidence and verification

- **Evidence-bounded claims.** A validation artifact supports only claims within what it can observe. Compilation, source inspection, tests, generated artifacts, independent review, integration behavior, and physical-device testing establish different kinds of evidence. "Verified", "not verified", and "not verifiable at this layer" are all legitimate results. Absence of evidence is not proof.
- **Layer-appropriate verification.** Verify a behavior at the boundary that can observe it. Repository logic, generated artifacts, browser integration, external services, and physical devices may each need a different method.
- **Mechanism-specific source verification.** A source supports a claim only when it supports the exact mechanism claimed, not an adjacent topic. For a material claim about agent or tool behavior: state the precise causal claim, identify the system boundary, obtain the exact supporting quotation or primary documentation, compare it with the claim, and record remaining uncertainty. In particular, a generated explanation that would shape an engineering workflow or policy needs direct supporting evidence before it is codified.
- **Regression sensitivity.** For an important invariant or a previously found defect class, confirm when practical that violating the invariant makes the expected tests fail. A passing test alone does not show that it would catch the regression. When this applies is open; see [#7](https://github.com/bdonaldharris/builder-garage/issues/7).

### Authority

- **Explicit authority and precedence.** When two values must always agree, first ask whether both should exist. A concern should have one authoritative representation at a given level, with other representations derived from it. A system may still hold several kinds of truth, such as product intent, configuration, runtime state, observations, and inferences. Each should have an identifiable authority and an explicit precedence over competing representations.
- **Capability does not imply authority.** An actor, component, or model able to perform an operation is not thereby authorized to decide what that operation represents.
- **Boundaries should not silently weaken semantics.** Crossing an API, persistence, transport, lifecycle, or asynchronous boundary should preserve the behavior the system depends on, even where a weaker implementation appears to work.

### Stateful and asynchronous systems

These apply only where a change owns state, asynchronous work, a UI lifecycle, or an external resource. They do not widen the review boundaries the canonical skills define. How to recognize when they apply is open; see [#7](https://github.com/bdonaldharris/builder-garage/issues/7).

- **Review state machines, not only happy paths.** Review a stateful or asynchronous feature across its meaningful transitions, terminal paths, failure paths, and stale-event behavior, as well as its nominal journey. Derive the relevant scenarios from the feature itself, not from a universal checklist. The canonical skills already require a bounded form of this, but only when a demonstrated defect exposes a shared rule; see [Established](#established). The broader up-front form is a candidate only.
- **Timing is part of correctness.** An invariant has a time by which it must hold, whether event-synchronous, commit-synchronous, before observation or paint, or eventual. Eventually holding is not always enough.
- **Revoke authority before unreliable cleanup.** Withdraw logical authority before calling cleanup that may throw, re-enter, call back later, or fail to release the resource.
- **Stale work is powerless.** Results from terminated, superseded, canceled, or unauthorized work must not change current authoritative state.
- **Separate state domains.** Application state, external lifecycle state such as browser or SDK state, and physical resource state are distinct. Evidence about one does not establish another.
- **Do not regress an authoritatively accepted value.** Once the application has accepted or displayed a value as current, a later external reconciliation should not silently replace it merely because the external system defines finality more narrowly.

### Product surface

- **Keep agent collaboration context out of customer UI.** Agent-readable context belongs in non-customer channels: repository structure, component names, comments, structured metadata, project instructions, test identifiers, and authorized tool context. Rendering a string in the UI gives an agent no memory it lacks from source access. Before release, review visible strings, labels, badges, tooltips, empty and error states, controls, and diagnostics in customer-representative states. Use automated scans to surface candidates, then make a human product judgment. Preserve accessibility labels, customer-relevant status, useful diagnostics, and intended error handling. Product-intent verification more broadly is open; see [#8](https://github.com/bdonaldharris/builder-garage/issues/8).

### Lifecycle and method

- **Progressive uncertainty reduction.** Each stage should narrow the kinds of surprise left for the next, so that promotion confirms rather than discovers.
- **Progressive formalization.** Practices move from observed experience to an articulated principle, to a standard, and to enforcement only where that is warranted. A standard should be justified by engineering need, and not everything needs to become executable.

## Hypotheses

- **Comprehension debt.** Agent-assisted construction can outpace the understanding of the people accountable for the software. Preserving reasoning, decisions, and trade-offs may be an engineering property, not only documentation.
- **Intentional compromise.** Technical debt and temporary limitations can be legitimate when chosen and bounded deliberately. The distinction that matters is deliberate compromise versus degradation discovered later.
- **Invariants and context-sensitive expressions.** A small set of obligations might hold everywhere while projects satisfy them through different mechanisms. Tracked in [#7](https://github.com/bdonaldharris/builder-garage/issues/7).
- **The framework governs how project truth is established, not the truth itself.** Tracked in [#6](https://github.com/bdonaldharris/builder-garage/issues/6).

## Open questions

- [#6 Clarify framework authority and project-specific authority](https://github.com/bdonaldharris/builder-garage/issues/6)
- [#7 Define context-sensitive engineering rigor](https://github.com/bdonaldharris/builder-garage/issues/7)
- [#8 Evaluate product intent and completion verification](https://github.com/bdonaldharris/builder-garage/issues/8)
