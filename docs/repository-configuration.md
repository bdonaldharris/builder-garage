# Repository Configuration: Observed State and Recommendations

**Observed:** 2026-11-12 (UTC), read-only, through the GitHub API with repository admin access.

This is a point-in-time record. It compares the repository's settings with the rules in [GOVERNANCE.md](../GOVERNANCE.md) and recommends changes. **No repository setting was modified in producing it.** Verify the current state before acting on it.

## Observed state

| Area | Observed |
| --- | --- |
| Repository | `bdonaldharris/builder-garage`, public, default branch `main`. Renamed from `agentic-builder-engineering-system`; the old URL redirects. |
| Branch protection on `main` | None. |
| Rulesets | None. |
| GitHub Actions workflows | None. No status checks exist. |
| Collaborators | One: `bdonaldharris` (admin). |
| Merge settings | Merge commits, squash, and rebase all allowed. Auto-merge off. Head branches are not deleted on merge. |
| Private vulnerability reporting | Disabled. |
| Secret scanning and push protection | Disabled. |
| Dependabot security updates | Disabled. |
| Issues / Discussions / Wiki | Enabled / disabled / disabled. |
| Labels | GitHub defaults plus `accessibility`. |
| `CODEOWNERS` | None. |
| Detected community files | README only. License, contributing guide, code of conduct, security policy, and templates are not detected because they are not yet on `main`. |
| Recent history | The five most recent commits on `main` (`1ef761d` through `e970eca`) have no associated pull request. |
| Remote branches | `docs/integration-review-gate` has no commits that are not already on `main`. |

## Gaps against governance

| Governance rule | Enforced today? | Recommendation |
| --- | --- | --- |
| No direct pushes to `main` | No. Direct pushes are possible and have occurred. | Add a ruleset targeting the default branch that requires a pull request before merging, blocks force pushes, and restricts deletion. |
| Maintainer approval before merge | No. | In the same ruleset, require approving reviews only in a form that permits the sole-maintainer workflow. See [Sole maintainer](#sole-maintainer). Once a second maintainer exists, require at least one approving review and dismiss stale approvals when new commits are pushed. |
| Required checks before merge | Not applicable. No checks exist, so none can be required. | Decide which checks are warranted through an issue. Once a check exists, add it to the ruleset as a required status check. |
| Independent engineering review before commit | Not enforceable by repository settings. | No setting applies. The review is recorded on the originating issue, the pull request template asks for a link to it, and maintainers confirm it when approving or recording acceptance. |
| Private vulnerability reporting ([SECURITY.md](../SECURITY.md)) | No. The preferred channel named in the policy is unavailable until this is enabled; only the email fallback works. | Enable private vulnerability reporting. |

## Recommended changes

### Required for the documented governance to hold

1. Create the default-branch ruleset described above, configured as described under [Sole maintainer](#sole-maintainer).
2. Enable private vulnerability reporting.

### Recommended

3. Enable secret scanning and push protection.
4. Enable automatic deletion of head branches after merge.
5. Delete the `docs/integration-review-gate` branch.
6. Update the repository description and topics to match the identity in the [README](../README.md).
7. Update local clones to the current URL: `git remote set-url origin https://github.com/bdonaldharris/builder-garage.git`.

### Optional

8. Create a `discovery` label and add it to the Discovery request template. The template applies no label today because none of the existing labels fits.
9. Add `CODEOWNERS` once there is more than one maintainer.

Dependabot security updates need no action until the repository has dependencies.

## Sole maintainer

GitHub does not allow the author of a pull request to approve it. While the lead maintainer is the only maintainer, a ruleset that requires one approving review, with no exception, would block every pull request the lead maintainer authors.

[GOVERNANCE.md](../GOVERNANCE.md#pull-requests-authored-by-a-sole-maintainer) defines how those pull requests proceed: independent engineering approval remains mandatory, and the lead maintainer may then record explicit project acceptance in a comment on the pull request. That comment is documented project acceptance. It is not a GitHub approving review, and it does not satisfy a branch protection rule or ruleset that requires one. The repository's rules must therefore permit this workflow.

Two configurations permit it:

- **Require pull requests with zero required approvals** until a second maintainer exists. Approving reviews on other contributors' pull requests remain required by governance but are not enforced by settings.
- **Require one approving review and add the lead maintainer to the ruleset's bypass list in pull-request-only mode.** Changes still reach `main` only through pull requests, and the lead maintainer can merge a pull request they authored without an approving review. The bypass is not limited to pull requests the lead maintainer authored, so the approving review on other contributors' pull requests still depends on governance being followed.

Both keep "no direct pushes" enforced. Under either, project acceptance of the lead maintainer's own pull requests is recorded by comment and is not enforced by settings; GitHub cannot require a comment before merge.

Neither weakens independent engineering verification. No repository setting enforces that review today or under either configuration. It is recorded on the originating issue, linked from the pull request, and confirmed before acceptance is recorded.

Once a second authorized maintainer is available, maintainer-authored pull requests receive approval from another maintainer under the normal process. The ruleset can then require at least one approving review without a bypass.

## Unresolved decisions

### Sole-maintainer ruleset configuration

Which of the two configurations under [Sole maintainer](#sole-maintainer) to apply is undecided. The decision belongs to the lead maintainer.

### Required checks

No checks exist, and the project has no technology stack that would imply any. Which checks to add, if any, is undecided.
