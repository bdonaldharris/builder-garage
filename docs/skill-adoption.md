# Builder Garage Skill Adoption Profile

Builder Garage adopts the five canonical engineering [skills](../skills/) it publishes, as its own engineering workflow. This profile makes that adoption explicit, points to Builder Garage's configured choices, and records the other project choices that qualify the skill text. The skills themselves are not repeated here.

The skills are framework artifacts that any project can adopt. This profile is Builder Garage's adoption record as a project. Builder Garage's adoption of the Go Manual is recorded separately, in the [Go Manual profile](go-manual.md).

## Adopted skills

| Skill | Shorthand |
| --- | --- |
| [engineering-discovery-workflow](../skills/engineering-discovery-workflow/SKILL.md) | `eng-discovery` |
| [implementation-readiness-workflow](../skills/implementation-readiness-workflow/SKILL.md) | `readiness-workflow` |
| [application-implementation-workflow](../skills/application-implementation-workflow/SKILL.md) | `implement-workflow` |
| [independent-implementation-review](../skills/independent-implementation-review/SKILL.md) | `indep-review` |
| [integration-pr-review](../skills/integration-pr-review/SKILL.md) | `pr-review` |

**Revision adopted.** The skills carry no version label today. Builder Garage adopts them as they exist on its `main` branch. A specific revision is identified by commit: the skills at the commit that contains a given change are the ones that governed it. Agents should run copies that match `main`. `garage skills install` installs the skills a `garage` binary embeds, each marked with its revision, and `garage skills verify` checks installed copies against them ([Skill installation](skill-installation.md)).

## Configuration

Builder Garage's configured choices are recorded in [`.garage/config.yaml`](../.garage/config.yaml), which is their sole canonical source ([Project configuration](configuration.md)). Run `garage config validate` to see each configured value, and the default that applies where none is configured. This profile explains those choices; it does not restate or redefine them.

The file was generated, not hand-authored, by running this noninteractive command from the repository root ([#35 decision](https://github.com/bdonaldharris/builder-garage/issues/35#issuecomment-6842152164), item 5):

```sh
garage init \
  --integration-branch main \
  --small-corrections enabled \
  --small-corrections-without-issue \
  --small-correction-classifier maintainer \
  --discovery implementer-judgment \
  --suggestions-backlog "#24" \
  --workflow-owner "lead maintainer" \
  --change-acceptor maintainer \
  --product-intent-acceptor "Workflow Owner" \
  --manual go@0.1 \
  --skill-target claude-code@2.1.277@2026-11-12 \
  --skill-target codex@0.137.0@2026-11-12 \
  --skill-target copilot-cli@1.0.93-1@2026-11-12
```

The canonical skills read these values from the configuration, at the extension points they cite ([How the skills read the configuration](configuration.md#how-the-skills-read-the-configuration)). An agent needs a `garage` binary built from this repository on its `PATH` to resolve them. The table explains what each configured choice means for Builder Garage.

| Configured choice | What it means here | Source |
| --- | --- | --- |
| Integration branch ([E1](extension-points.md#e1-integration-branch)) | Implementation pull requests target the configured integration branch, and review records resolve it as their `integration_ref`. This changes the destination only, not any gate. | [GOVERNANCE: Changes to `main`](../GOVERNANCE.md#changes-to-main) |
| Automated reviewer ([E3](extension-points.md#e3-automated-reviewer)) | None is configured, so the canonical default applies: no automated reviewer. No pull-request review is requested from one, and its absence establishes nothing. If any automated reviewer does report findings on a PR head, they still fire minimum PR-review trigger 2. No tool establishes review authority. | [#52](https://github.com/bdonaldharris/builder-garage/issues/52), [#22 decision](https://github.com/bdonaldharris/builder-garage/issues/22#issuecomment-6830592571) (item 3), [#7 decision](https://github.com/bdonaldharris/builder-garage/issues/7#issuecomment-6817635061) (item 5) |
| Small corrections ([E4](extension-points.md#e4-small-correction-entry-path)) | A small correction that changes no meaning, such as a typo, broken link, or formatting, may be proposed as a pull request without an issue, and does not require independent review. Only the configured classifier, a human authority, decides that a change is a small correction; an agent never does ([F5](responsibility-floor.md#rules)). Maintainer approval is still required. | [CONTRIBUTING: Small corrections](../CONTRIBUTING.md#small-corrections), [pull request template](../.github/PULL_REQUEST_TEMPLATE.md), [#16 decision](https://github.com/bdonaldharris/builder-garage/issues/16#issuecomment-6836671788) (item 1) |
| Discovery ([E5](extension-points.md#e5-discovery-applicability)) | The implementer decides whether discovery is needed, and the Workflow Owner may require it. | [#16 decision](https://github.com/bdonaldharris/builder-garage/issues/16#issuecomment-6836671788) (item 5), [CONTRIBUTING: Discover when warranted](../CONTRIBUTING.md#2-discover-when-warranted) |
| Suggestions backlog ([E7](extension-points.md#e7-improvement-suggestions-backlog)) | Non-blocking review observations and dogfooding friction go to the configured issue. A suggestion authorizes nothing until it is promoted into a separately scoped issue. | [#22 decision](https://github.com/bdonaldharris/builder-garage/issues/22#issuecomment-6830592571) (item 5) |
| Human authorities ([E8](extension-points.md#e8-human-authorities)) | The configured roles are explained in [GOVERNANCE: Workflow Owner](../GOVERNANCE.md#workflow-owner) and [GOVERNANCE: Review requirements](../GOVERNANCE.md#review-requirements). | [#8 decision](https://github.com/bdonaldharris/builder-garage/issues/8#issuecomment-6817923896) (item 1), #18 |
| Skill targets ([tooling](tooling-configuration.md)) | The harness targets Builder Garage's maintainer uses, as confirmed by the #37 load test. They grant no workflow authority. | [#37 decision](https://github.com/bdonaldharris/builder-garage/issues/37#issuecomment-6837792244) |

## Other project deltas

These choices qualify the skill text but are not extension-point values.

| Area | Builder Garage choice | Source |
| --- | --- | --- |
| Implementation-review evidence | Agent #2's implementation review is recorded on the originating issue before commit. That record is authoritative, and it governs whether the implementation may advance. | [GOVERNANCE: Review requirements](../GOVERNANCE.md#review-requirements), [CONTRIBUTING: Obtain independent review](../CONTRIBUTING.md#4-obtain-independent-review), [#22 decision](https://github.com/bdonaldharris/builder-garage/issues/22#issuecomment-6830592571) (items 1–2) |
| PR-review condition | An Agent #2 PR review is required when either canonical minimum trigger applies: the PR cannot be shown to be artifact-equivalent to the change Agent #2 approved, or an optional automated reviewer has reported findings on the current PR head. Added triggers are configured at [E2](extension-points.md#e2-additional-pr-review-triggers). The PR review is supplemental to the authoritative issue-recorded implementation review; it does not replace it. When no trigger applies, a Builder Garage PR proceeds on the issue-recorded review, recorded artifact equivalence, CI, and maintainer approval. | [#26 maintainer decision](https://github.com/bdonaldharris/builder-garage/issues/26#issuecomment-6834102663), [#27 maintainer decision](https://github.com/bdonaldharris/builder-garage/issues/27#issuecomment-6833011353), [#22 decision](https://github.com/bdonaldharris/builder-garage/issues/22#issuecomment-6830592571) (item 2), [integration-pr-review: PR-review triggers](../skills/integration-pr-review/SKILL.md#pr-review-triggers) |

## What this adoption does and does not show

Builder Garage's use of its own skills is single-project evidence. It shows that the skills can be carried out end to end in this repository, and it surfaces friction, which is recorded in [#24](https://github.com/bdonaldharris/builder-garage/issues/24). It does not show that the skills suit other projects. Builder Garage has one maintainer and one stack, and no production deployment, user interface, or database, so several skill sections are not exercised here.

## Changing this profile

This profile changes through the normal [contribution process](../CONTRIBUTING.md). A change to a canonical skill is a framework change, made in its own change. Updating this profile to reflect it is a separate adoption step.
