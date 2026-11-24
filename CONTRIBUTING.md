# Contributing to Builder Garage

Builder Garage is maintainer-led and community-contributed, and contributions are welcome from anyone. This document explains how a change moves from an idea to `staging`, and from there to a release on `main`.

## Provider binding for adopters

Builder Garage itself continues to use GitHub. Adopter repositories may select GitLab using validated forge configuration; [provider operations](docs/provider-operations.md) maps the same contribution workflow to GitLab Issues/MRs and verified top-level notes. Authoritative implementation review remains on the Issue; supplemental review and equivalence belong on the MR. Human authorization, role separation and all promotion gates remain unchanged. GitLab Markdown templates live under `.gitlab/issue_templates` and `.gitlab/merge_request_templates`; GitHub retains its existing forms/templates and tools. `garage forge` performs explicit authorized GitLab operations, never autonomous workflow execution; local web remains read-only.

## Before you start

- **Current state.** The repository contains the initial Go CLI foundation and its tests and build validation, alongside engineering skills, standards, [engineering principles](docs/principles.md), [project history](docs/history.md), and governance. See the [project status](README.md#project-status), [CLI development commands](README.md#develop-and-validate), and [architectural direction](docs/architecture.md).
- **Conduct.** Participation is governed by the [Code of Conduct](CODE_OF_CONDUCT.md).
- **Security.** Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md), never in a public issue.
- **License.** Builder Garage is released under the [MIT License](LICENSE). Contributions you submit are licensed under the same terms.
- **Authority.** [GOVERNANCE.md](GOVERNANCE.md) describes who decides what.
- **First contribution.** [Your first contribution](docs/first-contribution.md) walks a first-time contributor through one change, from finding an issue to an open pull request.

## Minimum garage version

Work on this repository with **`garage` `v0.3.0` or later**, and reinstall the skills from it:

```sh
garage version          # v0.3.0 or later
garage skills install
```

- **Why the minimum.** Builder Garage's configuration names a release branch (`release_branch`). Releases before `v0.3.0` cannot read it: `garage config validate` fails with "field release_branch not found", and every canonical skill stops when validation fails.
- **Why the reinstall.** It gives agents the skill versions that carry the hotfix rules (E11).
- **Every environment needs it,** including remote or hosted agent setups, which install their own copy.

## Two separate gates

Every substantive change passes two gates. They answer different questions at different times, and neither replaces the other.

| | Independent engineering review | Maintainer approval |
| --- | --- | --- |
| **Question** | Is this change correct for its issue? | Does the project accept this change? |
| **When** | After implementation, before the change is proposed as a pull request | On the pull request, before merge |
| **Who** | A reviewer independent of the implementation; need not be a maintainer | A project maintainer |
| **Recorded** | On the originating issue | As a GitHub approving review on the pull request, except for [pull requests authored by a sole maintainer](#pull-requests-authored-by-a-sole-maintainer) |
| **Standard** | The review contract in [independent-implementation-review](skills/independent-implementation-review/SKILL.md) | Maintainer judgment under [GOVERNANCE.md](GOVERNANCE.md), plus required checks |

Independent approval allows a change to be committed and proposed. It does not authorize merge. Maintainer approval does not stand in for a missing independent review.

## How a substantive change proceeds

### 1. Start from an issue

Substantive changes are issue-driven. [Open an issue](https://github.com/bdonaldharris/builder-garage/issues/new/choose) using the template that fits, or pick up an existing one.

The issue is the durable record of the change: what is wanted, its acceptance criteria, its scope, and later its discovery findings and review results. Write acceptance criteria that a reviewer can check.

The change-proposal template also has an optional **Intended outcome** field. Use it to describe what the result should be like for its user and, where useful, how it will be observed. It supplements the acceptance criteria and does not replace them.

Before investing in implementation, give a maintainer the chance to respond on the issue. This avoids work on a change the project would not accept.

#### Working on an existing issue

An issue's assignee shows who is working on it, so that the same change is not implemented twice in parallel.

- **Requesting assignment.** If you are not a maintainer, ask to work on an existing issue in a comment on that issue, rather than assigning yourself. A maintainer assigns one primary owner.
- **Inactive assignments.** A maintainer may release an assignment that has become inactive, so that others can take up the issue.
- **What assignment means.** Assignment coordinates work. It does not approve a change or authorize its implementation, and it does not replace independent review or maintainer approval. Every substantive change still passes the [two separate gates](#two-separate-gates).
- **When the work ends.** A merged pull request that closes the issue ends the assignment. If a pull request is rejected or withdrawn while work remains, the work may be revised, and the issue unassigned, reassigned, or reopened, as appropriate.

### 2. Discover when warranted

When a change depends on questions that are not yet answered, investigate before implementing. Discovery investigates; it does not implement.

**Document the findings in the originating issue**, so that the reasoning stays with the change. Discovery is not complete until they are recorded there.

- Procedure: [engineering-discovery-workflow](skills/engineering-discovery-workflow/SKILL.md)
- To judge whether a change is understood well enough to implement: [implementation-readiness-workflow](skills/implementation-readiness-workflow/SKILL.md)

Not every change needs a separate discovery pass. Depth should be proportional to the change's risk and uncertainty.

### 3. Implement

Implement the change within the scope of its issue. **Do not commit yet**, by default. Under [commit-stage review](skills/application-implementation-workflow/SKILL.md#commit-stage-review), when the issue or the person running the implementation asks for it, commit the change and push it to a branch without opening a pull request.

- Procedure: [application-implementation-workflow](skills/application-implementation-workflow/SKILL.md)

### 4. Obtain independent review

Before the change is proposed as a pull request, including a draft, the complete change is reviewed by someone independent of its implementation. The implementer never reviews their own work.

The change is reviewed in one of two forms:

- **Uncommitted, the default.** The reviewer inspects the implementer's working tree, so the reviewer must be able to reach it.
- **Committed, when asked for.** Under [commit-stage review](skills/application-implementation-workflow/SKILL.md#commit-stage-review), the implementer commits the change and pushes it to a branch without opening a pull request, for example in their fork. A reviewer who cannot reach the implementer's workspace reviews that commit. Use this path only when the issue or the person running the implementation asks for it.

The review result is recorded on the originating issue. That issue record is the authoritative record of the independent review, and it governs whether the change may be proposed as a pull request. A review of the pull request, where one is required, supplements it and never replaces it.

- Procedure and review contract: [independent-implementation-review](skills/independent-implementation-review/SKILL.md)

**Contributors outside the project.** You may hold this separation yourself. Run the implementation and the independent review as separate invocations, in separate execution contexts, in your own workspace, and post both records on the originating issue ([F1, F2](docs/responsibility-floor.md#rules)). This path is set by the [#98 D1 decision](https://github.com/bdonaldharris/builder-garage/issues/98#issuecomment-6873371683). [Contributor-held separation](crew/filling-roles.md#contributor-held-separation) explains it, and [Your first contribution](docs/first-contribution.md#4-implement-and-review-in-separate-execution-contexts) walks through it. If you cannot run separate execution contexts, for example because you work entirely by hand, ask on the issue before implementing. Someone outside your workspace may then review a pushed commit under commit-stage review, as described above.

### 5. Correct and re-review

If the review requires changes, correct them and return the complete corrected change for independent re-review. Addressing findings does not by itself restore approval; only the re-review does.

### 6. Commit, push, and open a pull request

Only after independent approval:

1. commit the change that was approved, and nothing else;
2. push it to a branch (in your fork, or in this repository if you have access);
3. open a pull request against `staging`, completing the [pull request template](.github/PULL_REQUEST_TEMPLATE.md), linking the issue and its review record, and declaring its kind, `implementation`.

Under commit-stage review, the approved change is already committed and pushed: skip steps 1 and 2, and open the pull request from the reviewed commit, or from one whose change is identical.

If the change is modified after approval, it returns to independent review.

- Pull-request-stage procedure: [integration-pr-review](skills/integration-pr-review/SKILL.md)

`staging` is Builder Garage's integration branch, configured in [`.garage/config.yaml`](.garage/config.yaml), and the canonical skill opens pull requests against it. Only a hotfix-designated issue targets `main`, the release branch. The skill's review, validation, and authorization requirements apply in full; the configured branch changes only the destination. See [GOVERNANCE: Changes to `staging` and `main`](GOVERNANCE.md#changes-to-staging-and-main).

### 7. Maintainer approval and merge

A maintainer reviews the pull request. It merges only after maintainer approval and after all required checks pass, with a merge commit. No one pushes to `staging` or `main` directly.

Merging into `staging` does not close the issue: it stays open until its release is promoted to `main`, and the promotion pull request closes it ([Release runbook](docs/release-runbook.md)).

Maintainer approval is normally recorded as a GitHub approving review on the pull request.

Product intent is accepted at this stage. The Workflow Owner, currently the lead maintainer, is the human authority that accepts whether the result is what was intended for its user. When a change affects something a user sees or uses, and inspecting it is practical, the Workflow Owner inspects the result in that form before accepting it. Examples are the rendered page or a command's actual output. This is not a step every change must complete, and it does not change the independent review contract. [GOVERNANCE.md](GOVERNANCE.md#product-intent-acceptance) is the authority for this policy.

#### Pull requests authored by a sole maintainer

GitHub does not allow a pull request's author to approve it. While the lead maintainer is the only maintainer, a pull request the lead maintainer authored proceeds as follows:

1. Independent engineering review remains mandatory and is completed first, as in steps 4 and 5.
2. Maintainer acceptance remains a separate decision. After independent engineering approval, the lead maintainer may record explicit project acceptance in a comment on the pull request.
3. That comment is documented project acceptance. It is not a GitHub approving review, and it does not satisfy a repository rule that requires one. The repository's rules must permit this workflow without weakening independent engineering verification; see [docs/repository-configuration.md](docs/repository-configuration.md#sole-maintainer).

Once additional authorized maintainers are available, a maintainer-authored pull request receives approval from another maintainer under the normal process. [GOVERNANCE.md](GOVERNANCE.md#pull-requests-authored-by-a-sole-maintainer) is the authority for this policy.

## Small corrections

The lifecycle above applies to substantive changes. A small correction that changes no meaning, such as a typo, a broken link, or formatting, may be proposed directly as a pull request without an issue. It still requires maintainer approval.

A maintainer decides whether a change is substantive and may ask for an issue first.

## Working with agents

The linked skills define how AI agents carry out this process, including the separation between the implementing agent and the independently invoked review agent. See [Agent identity and role boundaries](README.md#agent-identity-and-role-boundaries) and the skill shorthand in the [README](README.md#skill-shorthand).

- Implementation and independent review are separate agent invocations, in separate execution contexts such as separate sessions. An agent never reviews work it implemented.
- Output from automated review tools is optional input to independent review, not a substitute for it. Review authority comes from the reviewer's role and the recorded review, not from the tool used. Contributors are not required to enable a GitHub-hosted agent reviewer or any particular vendor integration.

The same sequence applies when people do the work without agents: someone other than the implementer performs the independent review and records the result on the issue.

## Changing skills and standards

The skills under [`skills/`](skills/) are the project's canonical engineering standards. Changing one is a substantive change and follows the full process above.

Link to the canonical skill rather than restating its contents elsewhere. Summaries drift; the skill is the source of truth.
