# Engineering History

This document is a concise account of where Builder Garage's engineering practices came from. It is historical context, not a source of requirements. Current requirements are listed in [principles.md](principles.md#established). Approved architecture is in [architecture.md](architecture.md), and it takes precedence over any hypothesis recorded here.

## Name

The project began as **ABES**, the Agentic Builder Engineering System, in a repository named `agentic-builder-engineering-system`. It was renamed **Builder Garage**. Records written before the rename use the earlier name.

## Project lineage

The practices were not designed from a blank page. They developed through building, reviewing, failing, correcting, and releasing software across a sequence of projects:

> **BitVoices → HindSite → FlashNotes → Stewart**

Knowledge moved forward through this sequence, so later projects are not independent confirmation of earlier practices. The lineage answers two different questions: whether a concern keeps mattering across different kinds of software (recurrence), and how its treatment changed with experience (evolution).

- **BitVoices** is the earliest major source. It contains substantial explicit machinery: specifications, plans, dependency modeling, standards, CI gates, staging and promotion mechanics, and agent instructions. Part of its structure came from GitHub Spec Kit. A mechanism found there may therefore be inherited tooling rather than a proven practice, and Builder Garage does not adopt Spec Kit's structure.
- **HindSite** added stronger governance of product truth versus engineering truth, deterministic versus inferred behavior, documentation authority, issue boundaries, readiness and completion, and bounded AI participation. It showed that a specification alone is not sufficient.
- **FlashNotes** applied the accumulated discipline to a small personal application. Repeated independent review exposed defect classes in authority, lifecycle timing, stale work, state transitions, and regression sensitivity. These became the stateful-system candidates in [principles.md](principles.md#stateful-and-asynchronous-systems). FlashNotes also showed that rigor does not require maximal ceremony.
- **Stewart** introduced an explicitly agentic runtime with a supervising agent and bounded specialists. Its significance was still being assessed, and it is not a template for Builder Garage's agent model.

When a practice changes between projects, the useful question is whether the change is an *evolution* in understanding or an *adaptation* to that project's context. A mechanism that disappears may have been inferior, or merely unnecessary for that project. To trace a practice across the lineage, the checkpoint used the lens *Origin → Application → Failure / Pressure → Adaptation → Later Expression → Current Hypothesis*. It is an analytical aid, not a required workflow or documentation format. For inherited mechanisms, the excavation distinguished *inherited mechanism → adapted practice → discovered principle*. The existence of a mechanism is not evidence that it should become canonical.

## Excavation

From August to October 2026, the project recovered its practices from that history before designing any architecture, under the discipline *excavate before we architect*. Three records were produced:

- an **initial inventory** of 33 findings, each classified as established practice, evidence-backed candidate standard, emerging concept, open question, or historical hypothesis;
- a **checkpoint** (2026-08-31) on the body of evidence as a whole, covering the lineage above, strengthened hypotheses, and twelve open questions;
- a **finding on visible UI metadata** (2026-10-06), which produced two candidate standards.

The checkpoint concluded that the implementation lifecycle (implementation, independent review, correction, re-review, integration validation, promotion protection, promotion) expresses a deeper set of recurring concerns: authority, truth and precedence, context, boundaries, evidence, uncertainty, accountability, comprehension, proportionality, semantic preservation, progressive verification, and learning from failure. It did not declare these to be principles. It described the emerging system as larger than an AI-assisted coding workflow and narrower than a universal software architecture.

The excavation was followed by the project's approved direction: the Builder Garage identity, the [architectural direction](architecture.md), open-source [governance](../GOVERNANCE.md), and the [Go CLI foundation](adr/0001-go-cobra-cli-foundation.md).

### Earlier five-agent model

An early operating model separated five roles: Solutions Architect, Engineering Coordinator, Primary Developer, Senior Developer Peer Reviewer, and QA / Adversarial Reviewer. The two-agent model now defined by the canonical skills (Agent #1 implementation, Agent #2 independent review) superseded it. Its lasting insight was the separation of implementation authority from review authority. The five roles, their names, and their tool assignments are not part of Builder Garage.

### What the excavation declined to fix in advance

The checkpoint listed things it did not consider justified as universal architecture. Some have since been decided through the normal process:

| Item | Current state |
| --- | --- |
| A CLI and an implementation technology | Decided: Go and Cobra, [ADR 0001](adr/0001-go-cobra-cli-foundation.md). |
| A literal `staging` branch | Builder Garage maps the skills' `staging` destination to `main` ([GOVERNANCE.md](../GOVERNANCE.md#changes-to-main)). |
| A fixed agent count | The skills define two distinct agents. No orchestrator exists. |
| An orchestrator design, Stewart's topology, the five-agent model, Spec Kit's structure, a universal document hierarchy, a universal risk classification, identical ceremony for every project | Still undecided or not adopted. Risk and rigor are tracked in [#7](https://github.com/bdonaldharris/builder-garage/issues/7). |

## Parked: containerization as a Build System mental model

**Status: deferred.** This is not approved Builder Garage architecture, an implementation approach, or a product direction.

The checkpoint preserved an analogy for a possible future executable "Build System" that would package *engineering behavior* rather than only AI capability into a reproducible, versioned, isolated, and portable environment. In the analogy, core doctrine is a base layer, capabilities are layered on top, persistent project context is a volume, project configuration is environment variables, engineering gates are health checks, and the builder remains authoritative at explicit human-judgment checkpoints. The checkpoint also noted possible uses outside this repository and an undecided distribution model.

It was recorded only so it would not be lost. It does not invite selecting container technology, defining architecture, or creating a roadmap. Builder Garage's own MIT license decides nothing about the concept. Any revival of the concept starts from an issue under the [contribution process](../CONTRIBUTING.md).

## Retired excavation records

The three excavation records were removed from the active documentation under [#5](https://github.com/bdonaldharris/builder-garage/issues/5) after their contents were accounted for. Their full text remains in Git history:

- [Initial inventory](https://github.com/bdonaldharris/builder-garage/blob/0b6749aa2786f7bf2bc0807ab9d1648b91cc4743/docs/excavation/initial-inventory.md)
- [2026-08-31 checkpoint](https://github.com/bdonaldharris/builder-garage/blob/0b6749aa2786f7bf2bc0807ab9d1648b91cc4743/docs/excavation/2026-08-31-excavation-checkpoint.md)
- [2026-10-06 visible UI metadata and evidence](https://github.com/bdonaldharris/builder-garage/blob/0b6749aa2786f7bf2bc0807ab9d1648b91cc4743/docs/excavation/2026-10-06-visible-ui-metadata-and-evidence.md)

### Disposition

Destinations: **Established** means already required (see [principles.md](principles.md#established)). **Candidate** and **Hypothesis** refer to the corresponding sections of [principles.md](principles.md). **Issue** means tracked on GitHub. **History** means this document. **Elsewhere** means it belongs to another project and is not carried here.

#### Initial inventory

| # | Finding | Destination |
| --- | --- | --- |
| 1 | Explicit standards over tribal knowledge | Established |
| 2 | Architecture before implementation | Established. Proportional depth: Issue [#7](https://github.com/bdonaldharris/builder-garage/issues/7) |
| 3 | PRs are not the first serious review | Established |
| 4 | Independent implementation and review authority | Established |
| 5 | Correction followed by independent re-review | Established. Abbreviated re-review: resolved, not permitted. Current canon requires complete, bounded re-review of the corrected artifact ([#7 decision](https://github.com/bdonaldharris/builder-garage/issues/7#issuecomment-6817635061), item 8) |
| 6 | Staging as the deep implementation gate | Established. No literal `staging` branch required |
| 7 | Promotion should be intentionally boring | Established. The skills define "boring" |
| 8 | Progressive uncertainty reduction | Candidate |
| 9–10 | Single source of truth; explicit authority modeling | Candidate (merged, with the checkpoint's broader framing) |
| 11–15 | Lifecycle timing; revoke before cleanup; stale work powerless; separate state domains; preserve accepted values | Candidate (conditional) |
| 16 | Review state machines, not only happy paths | Established (bounded form) + Candidate (broader, conditional; triggering → [#7](https://github.com/bdonaldharris/builder-garage/issues/7)) |
| 17 | Invariant-strengthening fixes | Established as the coherent correction boundary, bounded to issue scope |
| 18 | Regression sensitivity verification | Candidate. Threshold: Issue [#7](https://github.com/bdonaldharris/builder-garage/issues/7) |
| 19–21 | Evidence-bounded claims; layer-appropriate verification; unknown is acceptable | Candidate. "Unknown" in discovery and readiness is Established |
| 22 | Promotion Protection | Established. Owned by the integration workflow |
| 23 | Product intent alignment | Issue [#8](https://github.com/bdonaldharris/builder-garage/issues/8) |
| 24 | Risk-proportional rigor | Issue [#7](https://github.com/bdonaldharris/builder-garage/issues/7) |
| 25 | Earlier five-agent model | History |
| 26 | Same discipline for people and agents | Established. Human versus agent authority: Issue [#6](https://github.com/bdonaldharris/builder-garage/issues/6) |
| 27 | Discover before automating | Established in the evolutionary architecture direction. The CLI has since been approved |
| 28 | Canonical, versioned engineering system | Realized by this repository. History |
| 29 | A governing system rather than a workflow | Superseded by the approved framework identity. The authority question: Issue [#6](https://github.com/bdonaldharris/builder-garage/issues/6) |
| 30 | Governing-system authority must be explicit | Issue [#6](https://github.com/bdonaldharris/builder-garage/issues/6) |
| 31 | Progressive formalization | Candidate |
| 32 | Separate agent context from customer UI | Candidate |
| 33 | Mechanism-specific source verification | Candidate |

#### Checkpoint (2026-08-31)

| Section | Destination |
| --- | --- |
| Purpose (a non-normative benchmark; "excavate before we architect") | History (Excavation) |
| What We Are Excavating | History (Project lineage) |
| Project lineage and its interpretation | History. Detail specific to a single project is Elsewhere |
| Current Excavation Lens | History (Project lineage), as an analytical aid, not a required workflow or format |
| Recurring concerns beneath the lifecycle | History. These were never declared principles, and their candidate forms are in principles.md |
| Authority broader than a single source of truth; capability does not imply authority; truth has location and precedence | Candidate (authority) |
| Context is engineering infrastructure | Established (operating model) |
| Uncertainty stays visible; verification authority is bounded | Candidate (evidence) |
| Boundaries should not weaken semantics | Candidate |
| Refusing unnecessary engineering | Established |
| Human accountability persists through delegation | Issue [#6](https://github.com/bdonaldharris/builder-garage/issues/6) |
| Comprehensibility; intentional compromise | Hypothesis |
| Standards emerge from engineering evidence | Candidate (progressive formalization) |
| Rigor should be proportional; invariants and context-sensitive expressions | Issue [#7](https://github.com/bdonaldharris/builder-garage/issues/7) |
| Governs truth without supplying project truth | Issue [#6](https://github.com/bdonaldharris/builder-garage/issues/6) |
| What must not be prematurely canonized | History, with current state |
| Relationship To The Initial Inventory | Superseded: both records are retired together, and this disposition record replaces their cross-reference |
| Benchmark | History (Excavation) |
| Open questions 1, 2, 7 | Issue [#7](https://github.com/bdonaldharris/builder-garage/issues/7) |
| Open questions 4, 5, 6, 10, 12 | Issue [#6](https://github.com/bdonaldharris/builder-garage/issues/6). The execution model also remains undecided in [architecture.md](architecture.md#deliberately-undecided) |
| Open question 3 (which BitVoices mechanisms came from Spec Kit) | Elsewhere: a question about BitVoices' own history |
| Open questions 8, 9 (when a practice becomes normative; preserving the evidence behind a standard) | The contribution process decides adoption. This document, Git history, and issues preserve evidence |
| Open question 11 (preserving comprehension) | Hypothesis |
| Parked containerization concept | History (deferred) |

#### Visible UI metadata and evidence (2026-10-06)

| Section | Destination |
| --- | --- |
| Finding: no evidence that agents render UI metadata as memory | Candidate (customer UI), as its rationale |
| Separate collaboration context from customer UI | Candidate |
| Verify the mechanism, not the topic | Candidate (evidence) |
| Verification implications | Merged into the two candidates above: rendered-state review and automated scanning followed by human judgment go to customer UI; direct evidence before codifying a generated explanation goes to mechanism-specific source verification; "not documented" as a valid result goes to evidence-bounded claims |
