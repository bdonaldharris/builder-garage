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

Builder Garage's project configuration records these human authorities at extension point [E8](docs/extension-points.md#e8-human-authorities), in [`.garage/config.yaml`](.garage/config.yaml). The configured value is authoritative, and this section explains it. Whether a configured authority is human is a governance responsibility; Garage's validation does not certify it.

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
| **Independent engineering review** | Is the change correct for its issue? | A reviewer independent of the implementation; need not be a maintainer | After implementation, before the change is proposed for integration | On the originating issue |
| **Maintainer approval** | Does the project accept the change? | A maintainer | On the pull request, before merge | As an approving review on the pull request, except as described under [Pull requests authored by a sole maintainer](#pull-requests-authored-by-a-sole-maintainer) |

- The implementer of a change never performs its independent engineering review. Corrections made in response to review findings are independently re-reviewed before the change proceeds.
- Independent approval permits a change to be proposed as a pull request. It does not authorize merge. A change may be committed and pushed to a branch before review only to make it available for review, and it is not proposed until it is approved. Opening a pull request, including a draft, proposes it.
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

The canonical [integration-pr-review](skills/integration-pr-review/SKILL.md) skill opens an integration pull request against the integration branch configured at extension point [E1](docs/extension-points.md#e1-integration-branch) in [`.garage/config.yaml`](.garage/config.yaml). Its default, `staging`, does not apply, because Builder Garage configures its own. Builder Garage applies that workflow's review, validation, and authorization requirements in full. The configured branch changes the destination only. It does not change or relax any gate the skill defines.

The human approval that skill requires before merge is the maintainer approval described under [Review requirements](#review-requirements).

How far the repository's settings currently enforce these rules is recorded in [docs/repository-configuration.md](docs/repository-configuration.md).

## Hotfixes and staging synchronization

This section takes effect when Builder Garage configures `staging` as its integration branch and `main` as its release branch ([E11](docs/extension-points.md#e11-release-branch)), in the cutover tracked by [#140](https://github.com/bdonaldharris/builder-garage/issues/140). Until then, every change targets `main`, and there is no separate hotfix path.

It carries out decision 6 of the [release-promotion decision](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6962490422), as clarified by the [hotfix-synchronization amendment](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6984649202) and the [D-A and D-E decisions](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6984929399).

A hotfix and its staging synchronization are separate artifacts, with separate records.

### The hotfix

- **Entry.** A defect in the latest release that cannot wait for the next promotion. Security fixes are the canonical case.
- **Designation.** The Workflow Owner designates the issue with a [hotfix-designation record](https://github.com/bdonaldharris/builder-garage/issues/139#issuecomment-6989280538) on it, as the [registry](docs/extension-points.md#e11-release-branch) defines. A label is never a designation, and an agent never designates an issue.
- **Branch and pull request.** The hotfix branch starts from `main`, and its pull request targets `main` with `Closes #N`. It merges with a merge commit.
- **Review.** Every gate applies unchanged, including issue-recorded independent review and maintainer approval.
- **Release.** The hotfix is released as `main`'s next patch version, through the normal tag, dry-run, and release steps. It may be released without waiting for conflicts in `staging`.

### The staging synchronization

Immediately after the hotfix merges, it is synchronized from `main` into `staging`:

- **Clean.** A `main` → `staging` pull request that merges without conflict, by merge commit. It is linked from the hotfix issue as that hotfix's synchronization.
- **Conflicted.** A distinct change, not administrative cleanup within the already-reviewed hotfix:
  - **The synchronization issue.** A new issue, linked to the hotfix issue, records:
    - the conflict;
    - the resolution decision;
    - the resulting behavior in `staging`;
    - the original hotfix it preserves;
    - validation evidence;
    - the independent-review disposition.
  - **The branch.** A short-lived `backmerge/vX.Y.Z` branch, cut from `staging`, into which `main` is merged and the conflict resolved. Its pull request targets `staging`, so the ordinary workflow applies.
  - **Behavior.** The hotfix's required behavior is preserved in the upcoming release, even if the `staging` implementation differs or needs an additional correction.
  - **Review.** The synchronization is independently reviewed and validated before it merges. It is never folded into the hotfix's review.

**Provenance is not behavior.** After the synchronization, `main` is an ancestor of `staging`. That ancestry is provenance only. The synchronization's review and tests establish that the hotfix's behavior is preserved.

### Release gating

- The next release issue lists every relevant hotfix and synchronization issue.
- No release candidate is cut or promoted unless `staging` incorporates every hotfix currently on `main`.
- **A hotfix after a release candidate is cut,** under a different version: synchronize it, abandon that candidate, and cut the next candidate sequence number.
- **Version collision.** If a hotfix is released under the version an open release candidate targets:
  1. Abandon that candidate.
  2. Close its release issue as superseded by the hotfix release.
  3. Open a new release issue for the next valid Semantic Versioning release, carrying forward the unreleased accepted work.
  4. Cut its first candidate as `release/vNEXT-rc.1`.

  A released version is never reused as another release candidate.

## Conduct and security

Maintainers are responsible for enforcing the [Code of Conduct](CODE_OF_CONDUCT.md). Vulnerability reports are handled as described in [SECURITY.md](SECURITY.md).

## Licensing

Builder Garage is licensed under the [MIT License](LICENSE), which permits use, modification, and redistribution, including in proprietary derivative works, provided the copyright and permission notices are preserved. Contributions are accepted under the same license.

## Changing this document

Governance changes follow the normal contribution process: an issue describing the proposed change and its reasons, followed by a pull request. The originating issue is the place for community comment. A governance change requires the lead maintainer's approval.
