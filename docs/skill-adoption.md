# Builder Garage Skill Adoption Profile

Builder Garage adopts the five canonical engineering [skills](../skills/) it publishes, as its own engineering workflow. This profile makes that adoption explicit and records the project choices that differ from, or qualify, the skill text. The skills themselves are not repeated here.

The skills are framework artifacts that any project can adopt. This profile is Builder Garage's adoption record as a project. Builder Garage's adoption of the Go Manual is recorded separately, in the [Go Manual profile](go-manual.md).

## Adopted skills

| Skill | Shorthand |
| --- | --- |
| [engineering-discovery-workflow](../skills/engineering-discovery-workflow/SKILL.md) | `eng-discovery` |
| [implementation-readiness-workflow](../skills/implementation-readiness-workflow/SKILL.md) | `readiness-workflow` |
| [application-implementation-workflow](../skills/application-implementation-workflow/SKILL.md) | `implement-workflow` |
| [independent-implementation-review](../skills/independent-implementation-review/SKILL.md) | `indep-review` |
| [integration-pr-review](../skills/integration-pr-review/SKILL.md) | `pr-review` |

**Revision adopted.** The skills carry no version label today. Builder Garage adopts them as they exist on its `main` branch. A specific revision is identified by commit: the skills at the commit that contains a given change are the ones that governed it. Agents should run copies that match `main`. Distributing skills and verifying installed copies is owned by [#15](https://github.com/bdonaldharris/builder-garage/issues/15).

## Project deltas

Each row records a choice that differs from, or qualifies, the skill text. Where a row and its source differ, the source is authoritative.

| Area | Builder Garage choice | Source |
| --- | --- | --- |
| Integration branch | `main`. Where a skill names `staging` as the integration branch, Builder Garage uses `main`. This changes the destination only, not any gate. | [GOVERNANCE: Changes to `main`](../GOVERNANCE.md#changes-to-main) |
| Implementation-review evidence | Agent #2's implementation review is recorded on the originating issue before commit. That record is authoritative, and it governs whether the implementation may advance. | [GOVERNANCE: Review requirements](../GOVERNANCE.md#review-requirements), [CONTRIBUTING: Obtain independent review](../CONTRIBUTING.md#4-obtain-independent-review), [#22 decision](https://github.com/bdonaldharris/builder-garage/issues/22#issuecomment-6830592571) (items 1–2) |
| PR-review condition | An Agent #2 PR review is required when either canonical minimum trigger applies: the PR cannot be shown to be artifact-equivalent to the change Agent #2 approved, or an optional automated reviewer has reported findings on the current PR head. Builder Garage records no additional trigger today. The PR review is supplemental to the authoritative issue-recorded implementation review; it does not replace it. When no trigger applies, a Builder Garage PR proceeds on the issue-recorded review, recorded artifact equivalence, CI, and maintainer approval. | [#26 maintainer decision](https://github.com/bdonaldharris/builder-garage/issues/26#issuecomment-6834102663), [#27 maintainer decision](https://github.com/bdonaldharris/builder-garage/issues/27#issuecomment-6833011353), [#22 decision](https://github.com/bdonaldharris/builder-garage/issues/22#issuecomment-6830592571) (item 2), [integration-pr-review: PR-review triggers](../skills/integration-pr-review/SKILL.md#pr-review-triggers) |
| Automated review input | Codex, or any other automated reviewer, is optional review input where available. Builder Garage does not require a GitHub-hosted agent reviewer, and no tool establishes review authority. | [#22 decision](https://github.com/bdonaldharris/builder-garage/issues/22#issuecomment-6830592571) (item 3), [#7 decision](https://github.com/bdonaldharris/builder-garage/issues/7#issuecomment-6817635061) (item 5) |
| Small corrections | A small correction that changes no meaning, such as a typo, broken link, or formatting, may be proposed as a pull request without an issue, and does not require independent review. It still requires maintainer approval, and a maintainer decides whether a change is substantive. This is interim treatment. Builder Garage's configured small-correction policy is decided under #16. | [CONTRIBUTING: Small corrections](../CONTRIBUTING.md#small-corrections), [pull request template](../.github/PULL_REQUEST_TEMPLATE.md), [#7 decision](https://github.com/bdonaldharris/builder-garage/issues/7#issuecomment-6817635061) (item 4) |
| Suggestions inbox | Non-blocking review observations and dogfooding friction go to the persistent [Application Improvement Suggestions](https://github.com/bdonaldharris/builder-garage/issues/24) issue, which the skills name as the backlog. A suggestion authorizes nothing until it is promoted into a separately scoped issue. | [#22 decision](https://github.com/bdonaldharris/builder-garage/issues/22#issuecomment-6830592571) (item 5) |

## Interim documentation

This profile is interim, human-readable documentation. It is not configuration, and nothing reads or validates it. How projects select skills, record adaptations, and validate them through configuration is owned by [#16](https://github.com/bdonaldharris/builder-garage/issues/16). When that model exists, these choices move into Builder Garage's configuration, and this profile is replaced or reduced to a pointer.

## What this adoption does and does not show

Builder Garage's use of its own skills is single-project evidence. It shows that the skills can be carried out end to end in this repository, and it surfaces friction, which is recorded in [#24](https://github.com/bdonaldharris/builder-garage/issues/24). It does not show that the skills suit other projects. Builder Garage has one maintainer and one stack, and no production deployment, user interface, or database, so several skill sections are not exercised here.

## Changing this profile

This profile changes through the normal [contribution process](../CONTRIBUTING.md). A change to a canonical skill is a framework change, made in its own change. Updating this profile to reflect it is a separate adoption step.
