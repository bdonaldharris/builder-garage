# Contributing to Builder Garage

Builder Garage is maintainer-led and community-contributed, and contributions are welcome from anyone. This document explains how a change moves from an idea to `main`.

## Before you start

- **Current state.** The repository contains the initial Go CLI foundation and its tests and build validation, alongside engineering skills, standards, [engineering principles](docs/principles.md), [project history](docs/history.md), and governance. See the [project status](README.md#project-status), [CLI development commands](README.md#develop-and-validate), and [architectural direction](docs/architecture.md).
- **Conduct.** Participation is governed by the [Code of Conduct](CODE_OF_CONDUCT.md).
- **Security.** Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md), never in a public issue.
- **License.** Builder Garage is released under the [MIT License](LICENSE). Contributions you submit are licensed under the same terms.
- **Authority.** [GOVERNANCE.md](GOVERNANCE.md) describes who decides what.

## Two separate gates

Every substantive change passes two gates. They answer different questions at different times, and neither replaces the other.

| | Independent engineering review | Maintainer approval |
| --- | --- | --- |
| **Question** | Is this change correct for its issue? | Does the project accept this change? |
| **When** | After implementation, before anything is committed | On the pull request, before merge |
| **Who** | A reviewer independent of the implementation; need not be a maintainer | A project maintainer |
| **Recorded** | On the originating issue | As a GitHub approving review on the pull request, except for [pull requests authored by a sole maintainer](#pull-requests-authored-by-a-sole-maintainer) |
| **Standard** | The review contract in [independent-implementation-review](skills/independent-implementation-review/SKILL.md) | Maintainer judgment under [GOVERNANCE.md](GOVERNANCE.md), plus required checks |

Independent approval allows a change to be committed and proposed. It does not authorize merge. Maintainer approval does not stand in for a missing independent review.

## How a substantive change proceeds

### 1. Start from an issue

Substantive changes are issue-driven. [Open an issue](https://github.com/bdonaldharris/builder-garage/issues/new/choose) using the template that fits, or pick up an existing one.

The issue is the durable record of the change: what is wanted, its acceptance criteria, its scope, and later its discovery findings and review results. Write acceptance criteria that a reviewer can check.

Before investing in implementation, give a maintainer the chance to respond on the issue. This avoids work on a change the project would not accept.

### 2. Discover when warranted

When a change depends on questions that are not yet answered, investigate before implementing. Discovery investigates; it does not implement.

**Document the findings in the originating issue**, so that the reasoning stays with the change. Discovery is not complete until they are recorded there.

- Procedure: [engineering-discovery-workflow](skills/engineering-discovery-workflow/SKILL.md)
- To judge whether a change is understood well enough to implement: [implementation-readiness-workflow](skills/implementation-readiness-workflow/SKILL.md)

Not every change needs a separate discovery pass. Depth should be proportional to the change's risk and uncertainty.

### 3. Implement

Implement the change within the scope of its issue. **Do not commit yet.**

- Procedure: [application-implementation-workflow](skills/application-implementation-workflow/SKILL.md)

### 4. Obtain independent review

Before anything is committed, the complete change is reviewed by someone independent of its implementation. The implementer never reviews their own work.

The review result is recorded on the originating issue. That issue record is the authoritative record of the independent review, and it governs whether the change may be committed. A review of the pull request, where one is required, supplements it and never replaces it.

- Procedure and review contract: [independent-implementation-review](skills/independent-implementation-review/SKILL.md)

### 5. Correct and re-review

If the review requires changes, correct them and return the complete corrected change for independent re-review. Addressing findings does not by itself restore approval; only the re-review does.

### 6. Commit, push, and open a pull request

Only after independent approval:

1. commit the change that was approved, and nothing else;
2. push it to a branch (in your fork, or in this repository if you have access);
3. open a pull request against `main`, completing the [pull request template](.github/PULL_REQUEST_TEMPLATE.md) and linking the issue and its review record.

If the change is modified after approval, it returns to independent review.

- Pull-request-stage procedure: [integration-pr-review](skills/integration-pr-review/SKILL.md)

`main` is Builder Garage's integration destination. The canonical skill names `staging` as the branch a pull request is opened against; Builder Garage has no `staging` branch and maps that destination to `main`. The skill's review, validation, and authorization requirements apply in full. Only the destination differs. See [GOVERNANCE.md](GOVERNANCE.md#changes-to-main).

### 7. Maintainer approval and merge

A maintainer reviews the pull request. It merges only after maintainer approval and after all required checks pass. No one pushes to `main` directly.

Maintainer approval is normally recorded as a GitHub approving review on the pull request.

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
