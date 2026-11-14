# Repository Configuration: Observed State and Recommendations

**Observed:** 2026-11-14 (UTC), read-only, through the GitHub API with repository admin access.

This is a point-in-time record. It compares the repository's settings with the rules in [GOVERNANCE.md](../GOVERNANCE.md) and recommends changes. **No repository setting was modified in producing it.** Verify the current state before acting on it. The previous observation, from 2026-11-12, remains in Git history.

## Observed state

| Area | Observed |
| --- | --- |
| Repository | `bdonaldharris/builder-garage`, public, default branch `main`. Renamed from `agentic-builder-engineering-system`; the old URL redirects. The description is set. No topics are set. |
| Collaborators | One: `bdonaldharris` (admin). |
| Rulesets | One: **Protect main**. Active, defined on the repository, targeting the default branch. No bypass actors. |
| **Protect main** rules | **Restrict deletions.**<br>**Block force pushes.**<br>**Require a pull request before merging:** 0 required approving reviews; stale approvals are not dismissed when new commits are pushed; code-owner review is not required; approval of the most recent push is not required; review threads must be resolved; extra approval is required for unattributed changes; merge commits, squash, and rebase are allowed.<br>**Require status checks to pass:** `Format, vet, build, cross-compile`, `Test (ubuntu-latest)`, `Test (macos-latest)`, and `Test (windows-latest)`. Branches are not required to be up to date before merging. The checks are also enforced when a branch is created. |
| Branch protection on `main` | None. The ruleset above is the only protection. |
| GitHub Actions workflows | One repository workflow, CI ([`ci.yml`](../.github/workflows/ci.yml)), on pull requests. Besides the four required checks, it runs `Source installer (ubuntu-latest)` and `Source installer (macos-latest)`, which are not required. GitHub also lists two dynamic workflows of its own, Copilot cloud agent and Dependency Graph. |
| GitHub Actions policy | Actions enabled. All actions and reusable workflows are allowed. SHA pinning is not required. The default workflow token is read-only, and workflows cannot approve pull requests. |
| Merge settings | Merge commits, squash, and rebase all allowed. Auto-merge off. Head branches are deleted on merge. |
| Private vulnerability reporting | Enabled. |
| Secret scanning and push protection | Both enabled. Non-provider patterns and validity checks are disabled. |
| Dependabot | Alerts enabled. Security updates disabled. The repository has Go module dependencies (`go.mod`): `github.com/spf13/cobra` and `go.yaml.in/yaml/v3`, and indirectly `github.com/spf13/pflag` and `github.com/inconshreveable/mousetrap`. |
| Issues / Discussions / Wiki / Projects | Enabled / disabled / enabled / enabled. The wiki is the community wiki ([#70](https://github.com/bdonaldharris/builder-garage/issues/70)). |
| Labels | GitHub defaults, plus `accessibility`, `chore`, `discovery`, `extension`, `hotfix`, `plugin`, and the area labels `area:cli`, `area:configuration`, `area:crew`, `area:inspection`, `area:manuals`, `area:shipping`, and `area:workbench`. |
| `CODEOWNERS` | None. |
| Community profile | Health 100%. Detected: README, license, contributing guide, code of conduct, and pull request template. The profile reports no issue template, although `.github/ISSUE_TEMPLATE/` holds issue forms. |
| Recent history | The ten most recent commits on `main` (`70aab17` back to `2cff999`) all belong to merged pull requests, #71 through #79. |
| Remote branches | `main` only. |

## Gaps against governance

The repository's settings now enforce these [GOVERNANCE.md](../GOVERNANCE.md) rules:

- **No direct pushes to `main`.** The **Protect main** ruleset requires a pull request, blocks force pushes and deletion, and has no bypass actors ([Changes to `main`](../GOVERNANCE.md#changes-to-main)).
- **All required checks pass before merge.** The ruleset requires four checks. Which checks should be required is still open; see [Required checks](#required-checks).
- **Private vulnerability reporting**, the preferred channel in [SECURITY.md](../SECURITY.md), is available.

These rules are not enforced by settings:

| Governance rule | Enforced today? | Recommendation |
| --- | --- | --- |
| Maintainer approval before merge | No. The ruleset requires 0 approving reviews, so that the sole maintainer's own pull requests can merge. Approval of other contributors' pull requests is required by governance, not by settings. See [Sole maintainer](#sole-maintainer). | Once a second maintainer exists, require at least one approving review and dismiss stale approvals when new commits are pushed. |
| Independent engineering review before commit | Not enforceable by repository settings. | No setting applies. The review is recorded on the originating issue, the pull request template asks for a link to it, and maintainers confirm it when approving or recording acceptance. |

## Recommended changes

### Required for the documented governance to hold

None. The ruleset and private vulnerability reporting that the previous observation required are in place.

### Recommended

1. Set repository topics that match the identity in the [README](../README.md). The description is already set.
2. Update local clones to the current URL: `git remote set-url origin https://github.com/bdonaldharris/builder-garage.git`. This is a per-clone action, not a repository setting.

Whether to enable Dependabot security updates is an open decision; see [Dependabot security updates](#dependabot-security-updates).

### Optional

3. Apply the existing `discovery` label in the Discovery request template ([`discovery.yml`](../.github/ISSUE_TEMPLATE/discovery.yml)). The template applies no label today.
4. Add `CODEOWNERS` once there is more than one maintainer.

## Sole maintainer

GitHub does not allow the author of a pull request to approve it. While the lead maintainer is the only maintainer, a ruleset that requires one approving review, with no exception, would block every pull request the lead maintainer authors.

[GOVERNANCE.md](../GOVERNANCE.md#pull-requests-authored-by-a-sole-maintainer) defines how those pull requests proceed: independent engineering approval remains mandatory, and the lead maintainer may then record explicit project acceptance in a comment on the pull request. That comment is documented project acceptance. It is not a GitHub approving review, and it does not satisfy a branch protection rule or ruleset that requires one. The repository's rules must therefore permit this workflow.

Two configurations permit it:

- **Require pull requests with zero required approvals** until a second maintainer exists. Approving reviews on other contributors' pull requests remain required by governance but are not enforced by settings.
- **Require one approving review and add the lead maintainer to the ruleset's bypass list in pull-request-only mode.** Changes still reach `main` only through pull requests, and the lead maintainer can merge a pull request they authored without an approving review. The bypass is not limited to pull requests the lead maintainer authored, so the approving review on other contributors' pull requests still depends on governance being followed.

Both keep "no direct pushes" enforced. Under either, project acceptance of the lead maintainer's own pull requests is recorded by comment and is not enforced by settings; GitHub cannot require a comment before merge.

Neither weakens independent engineering verification. No repository setting enforces that review under either configuration. It is recorded on the originating issue, linked from the pull request, and confirmed before acceptance is recorded.

**In effect, as observed:** the first configuration. **Protect main** requires a pull request with zero approving reviews and has no bypass actors. No durable record of the decision to use it has been found, so the choice is listed under [Unresolved decisions](#sole-maintainer-ruleset-configuration).

Once a second authorized maintainer is available, maintainer-authored pull requests receive approval from another maintainer under the normal process. The ruleset can then require at least one approving review without a bypass.

## Unresolved decisions

Each decision below belongs to the lead maintainer. Each one stays here, with its observed setting, until the decision is recorded on [#77](https://github.com/bdonaldharris/builder-garage/issues/77) or linked from it.

### Sole-maintainer ruleset configuration

Which of the two configurations under [Sole maintainer](#sole-maintainer) to use is not recorded. **Observed:** the first, a pull request with zero required approving reviews and no bypass actors.

### Required checks

Which checks are required is not recorded. **Observed:** four required checks, `Format, vet, build, cross-compile`, `Test (ubuntu-latest)`, `Test (macos-latest)`, and `Test (windows-latest)`, with no up-to-date requirement. Also open: whether `Source installer (ubuntu-latest)` and `Source installer (macos-latest)`, which run on every pull request, become required.

### Dependabot security updates

Whether to enable Dependabot security updates, now that the repository has Go module dependencies, is not recorded. **Observed:** security updates disabled, alerts enabled.

### Actions policy

Whether to keep the current GitHub Actions policy or restrict it is not recorded. **Observed:** all actions and reusable workflows allowed, and SHA pinning not required.
