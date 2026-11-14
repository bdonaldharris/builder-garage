# Your First Contribution

**Builder Garage-specific.** This guide walks a first-time contributor through one change to Builder Garage, from finding an issue to an open pull request. It adds no requirement. [CONTRIBUTING.md](../CONTRIBUTING.md), [GOVERNANCE.md](../GOVERNANCE.md), and the canonical [skills](../skills/) govern, and each step below links to the source that defines it. Where this guide and a source differ, the source governs.

## 1. Find an issue and ask to work on it

Look for open issues labelled [`good first issue`](https://github.com/bdonaldharris/builder-garage/issues?q=is%3Aissue%20is%3Aopen%20label%3A%22good%20first%20issue%22), which are good for newcomers, or [`help wanted`](https://github.com/bdonaldharris/builder-garage/issues?q=is%3Aissue%20is%3Aopen%20label%3A%22help%20wanted%22), where extra attention is needed.

Ask to work on the issue in a comment on it, and wait for a maintainer to assign it. Assignment coordinates work. It does not approve the change. See [Working on an existing issue](../CONTRIBUTING.md#working-on-an-existing-issue).

Read the issue's acceptance criteria and scope before starting. The issue is the durable record of the change ([Start from an issue](../CONTRIBUTING.md#1-start-from-an-issue)).

## 2. Set up

- **Build and validate.** [Build from source](../README.md#build-from-source) lists the Go version, and [Develop and validate](../README.md#develop-and-validate) lists the checks, including `make check`.
- **Install `garage`.** The skills resolve this project's configuration through `garage config validate`, so your agent needs a `garage` binary on its `PATH`. [Install from source](../README.md#install-from-source) puts one there. Check the platforms it supports in [Current limitations](../README.md#current-limitations).
- **Install the skills.** If you work with an agent, install the canonical skills for it as [Skill installation](skill-installation.md#installing-and-updating) describes.

## 3. Follow the path from issue to merge

A change goes from issue to implementation, independent review, any correction and re-review, commit and pull request, and maintainer approval and merge. [How a substantive change proceeds](../CONTRIBUTING.md#how-a-substantive-change-proceeds) defines each step, and the [gate map](../inspection/gate-map.md) shows the gates in order.

Two gates are separate: independent engineering review before the change is proposed as a pull request, and maintainer approval of the pull request before merge. Neither replaces the other ([Two separate gates](../CONTRIBUTING.md#two-separate-gates)).

## 4. Implement and review in separate execution contexts

You hold the separation between implementation and independent review yourself. Run the implementation and the independent review as separate invocations, in separate execution contexts, in your own workspace, and post both records on the originating issue. This is the path the [#98 D1 decision](https://github.com/bdonaldharris/builder-garage/issues/98#issuecomment-6873371683) set for external contributors, and [Obtain independent review](../CONTRIBUTING.md#4-obtain-independent-review) states it.

- **What separation means.** [Separate execution contexts](../crew/filling-roles.md#separate-execution-contexts) and [Contributor-held separation](../crew/filling-roles.md#contributor-held-separation) explain it, including why the reviewer starts in a new context that has not seen the implementation conversation.
- **Ways to do it.** [Collaboration patterns](../crew/filling-roles.md#collaboration-patterns) describes them.
- **If you cannot run separate contexts,** for example because you are working entirely by hand, ask on the issue before implementing. Someone outside your workspace may then review a pushed commit under [commit-stage review](../skills/application-implementation-workflow/SKILL.md#commit-stage-review).

The procedures are [application-implementation-workflow](../skills/application-implementation-workflow/SKILL.md) for implementation and [independent-implementation-review](../skills/independent-implementation-review/SKILL.md) for the review. README's [Skill shorthand](../README.md#skill-shorthand) lists their short names. No particular AI model, vendor, or tool is required.

## 5. Post the two records on the issue

Two records go on the originating issue:

1. The **implementation record**, `[AGENT #1 — IMPLEMENTATION]`, posted when the change is ready for review. By default the change is still uncommitted. Under commit-stage review, it is pushed to a branch with no pull request. See [Agent #1 completion record](review-evidence.md#agent-1-completion-record).
2. The **review record**, `[AGENT #2 — INDEPENDENT REVIEW]`, posted by the separate review context. See [Agent #2 review record](review-evidence.md#agent-2-review-record).

Each carries a `garage-evidence` block that identifies the exact change with its `garage-artifact-v1` identity. Compute it with the [reference procedure](review-evidence.md#reference-procedure). [Where each record lives](review-evidence.md#where-each-record-lives) lists every record, including the one that goes on the pull request.

## 6. Know what `garage status` shows

`garage status <issue>` reads the issue's records from GitHub. It counts a workflow record only when its author is an owner, member, or collaborator of the repository, and reports the others as ignored ([Workflow status](../README.md#workflow-status), [#40 decision](https://github.com/bdonaldharris/builder-garage/issues/40#issuecomment-6853390215)). If you are not a collaborator, your records appear as ignored there. They are still the durable record on the issue.

## 7. Open the pull request

After the review record approves the change, commit exactly the approved change, push it to a branch in your fork, and open a pull request against `main`. If the change was already pushed for commit-stage review, open the pull request from that reviewed commit instead ([Commit, push, and open a pull request](../CONTRIBUTING.md#6-commit-push-and-open-a-pull-request)).

- **The template.** Complete the [pull request template](../.github/PULL_REQUEST_TEMPLATE.md), and link the issue and its approving review record.
- **The equivalence record.** The pull-request stage also asks for an [equivalence record on the pull request](review-evidence.md#equivalence-record-on-a-pr), as [integration-pr-review](../skills/integration-pr-review/SKILL.md) defines.

A maintainer then reviews and approves the pull request, and it merges after all required checks pass ([Maintainer approval and merge](../CONTRIBUTING.md#7-maintainer-approval-and-merge)). Independent review approval does not authorize merge.
