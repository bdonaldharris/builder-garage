# Governance

Builder Garage is an open-source project released under the [MIT License](LICENSE). It is **maintainer-led and community-contributed**: anyone may propose and contribute changes, and maintainers decide what the project accepts.

This document describes who holds decision authority, how decisions are made, what review a change requires, and how this document changes. The contribution procedure itself is in [CONTRIBUTING.md](CONTRIBUTING.md).

## Roles

### Lead maintainer

The project founder, B Donald Harris ([@bdonaldharris](https://github.com/bdonaldharris)), is the initial lead maintainer. The lead maintainer:

- holds final decision authority over the project, including its direction, scope, standards, and governance;
- appoints and removes maintainers;
- decides any question on which maintainers do not reach agreement.

### Maintainers

Maintainers are appointed by the lead maintainer. They participate in reviews and project decisions, triage issues, and approve pull requests. The lead maintainer is also a maintainer.

Current maintainers:

- B Donald Harris ([@bdonaldharris](https://github.com/bdonaldharris)) — lead maintainer

### Workflow Owner

The Workflow Owner is the human authority for workflow decisions that belong to a person rather than to an agent. The canonical skills call this role the workflow owner. For Builder Garage, the Workflow Owner is currently the lead maintainer.

The Workflow Owner is the human authority that accepts product intent for Builder Garage. See [Product-intent acceptance](#product-intent-acceptance).

Until Builder Garage's project configuration records its human authorities ([#35](https://github.com/bdonaldharris/builder-garage/issues/35)), this section is the record of that mapping. Once it does, the configured value is authoritative, and this section explains it.

### Contributors

Anyone who opens an issue, takes part in a discussion, performs a review, or submits a change is a contributor. No prior standing is required.

## Decision-making

Decisions are made in the open, on the GitHub issue or pull request they concern, so that the reasoning stays with the record of the change.

1. Maintainers seek agreement through discussion on the governing issue or pull request.
2. Where maintainers do not agree, the lead maintainer decides.
3. The lead maintainer's decision is final.

Architecture is evolutionary. Detailed design decisions are made within the lifecycle of the change that needs them, as described in [docs/architecture.md](docs/architecture.md), rather than settled in advance.

## Review requirements

A substantive change passes two distinct gates. They answer different questions, and neither substitutes for the other.

| Gate | Question it answers | Performed by | When | Recorded |
| --- | --- | --- | --- | --- |
| **Independent engineering review** | Is the change correct for its issue? | A reviewer independent of the implementation; need not be a maintainer | After implementation, before the change is committed | On the originating issue |
| **Maintainer approval** | Does the project accept the change? | A maintainer | On the pull request, before merge | As an approving review on the pull request, except as described under [Pull requests authored by a sole maintainer](#pull-requests-authored-by-a-sole-maintainer) |

- The implementer of a change never performs its independent engineering review. Corrections made in response to review findings are independently re-reviewed before the change proceeds.
- Independent approval permits a change to be committed and proposed as a pull request. It does not authorize merge.
- Maintainer approval does not stand in for a missing independent engineering review.
- The record on the originating issue is the authoritative record of the independent engineering review. A review of the pull request, where one is required, supplements it and never replaces it. Review authority comes from the reviewer's independence and role, not from the tool used, and no GitHub-hosted agent reviewer or vendor integration is required.

The independent review procedure and its review contract are defined by the [independent-implementation-review](skills/independent-implementation-review/SKILL.md) skill. This document does not restate them.

### Product-intent acceptance

Independent engineering review checks a change against its written issue. Whether the result is what was intended for its user is a human judgment, made as part of the project's acceptance of the change. For Builder Garage, the [Workflow Owner](#workflow-owner) is the human authority that accepts product intent.

This adds no gate and changes no skill. The review contract is unchanged. Intent that the issue states is checked by independent review. Intent that the issue does not state is the Workflow Owner's to judge, not the reviewer's.

When a change affects something a user sees or uses, and inspecting it is practical, the Workflow Owner inspects the result in that form before accepting it. Examples are the rendered page rather than its Markdown source, or a command's actual output. This applies only when that condition holds. It is not a step every change must complete.

### Pull requests authored by a sole maintainer

GitHub does not allow the author of a pull request to submit an approving review of it. While the lead maintainer is the only maintainer, maintainer approval of a pull request the lead maintainer authored cannot be recorded as an approving review. Until another authorized maintainer is available, such a pull request proceeds as follows:

- Independent engineering review remains mandatory. This section does not change when it is required, who may perform it, or where it is recorded.
- Maintainer acceptance remains a separate decision. Independent engineering approval does not imply it.
- After independent engineering approval, the lead maintainer may record explicit project acceptance in a comment on the pull request.
- That comment is documented project acceptance. It is not a GitHub approving review, and it does not satisfy a branch protection rule or ruleset that requires an approving review.
- The repository's rules must therefore permit this workflow, and must do so without weakening independent engineering verification. See [docs/repository-configuration.md](docs/repository-configuration.md#sole-maintainer).

This applies only to pull requests authored by the sole maintainer. Pull requests from other contributors receive the maintainer's approving review as usual.

Once additional authorized maintainers are available, a maintainer-authored pull request receives approval from another maintainer under the normal process.

## Changes to `main`

- `main` is Builder Garage's integration destination. Pull requests are opened against `main`.
- `main` changes only through pull requests. No one pushes to `main` directly, including maintainers.
- A pull request merges only after maintainer approval and after all required checks pass.

The canonical [integration-pr-review](skills/integration-pr-review/SKILL.md) skill names `staging` as the branch an integration pull request is opened against. Builder Garage does not have a `staging` branch. It applies that workflow's review, validation, and authorization requirements in full, with one repository-specific destination mapping: where the skill names `staging` as the integration branch, Builder Garage uses `main`. The mapping changes the destination only. It does not change or relax any gate the skill defines.

The human approval that skill requires before merge is the maintainer approval described under [Review requirements](#review-requirements).

How far the repository's settings currently enforce these rules is recorded in [docs/repository-configuration.md](docs/repository-configuration.md).

## Conduct and security

Maintainers are responsible for enforcing the [Code of Conduct](CODE_OF_CONDUCT.md). Vulnerability reports are handled as described in [SECURITY.md](SECURITY.md).

## Licensing

Builder Garage is licensed under the [MIT License](LICENSE), which permits use, modification, and redistribution, including in proprietary derivative works, provided the copyright and permission notices are preserved. Contributions are accepted under the same license.

## Changing this document

Governance changes follow the normal contribution process: an issue describing the proposed change and its reasons, followed by a pull request. The originating issue is the place for community comment. A governance change requires the lead maintainer's approval.
