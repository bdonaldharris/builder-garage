# Repository Configuration: Observed State and Recommendations

**Observed:** 2026-11-14 (UTC), read-only, through the GitHub API with repository admin access. **Re-observed on 2026-11-19 (UTC)**, after the cutover settings of [#140](https://github.com/bdonaldharris/builder-garage/issues/140) were applied: the rulesets, the merge methods they allow, the required checks, the remote branches, and the labels. Rows marked *(2026-11-19)* come from that re-observation. The others are from 2026-11-14.

This is a point-in-time record. It compares the repository's settings with the rules in [GOVERNANCE.md](../GOVERNANCE.md) and recommends changes. **No repository setting was modified in producing it.** Verify the current state before acting on it. The previous observation, from 2026-11-12, remains in Git history.

## Observed state

| Area | Observed |
| --- | --- |
| Repository | `bdonaldharris/builder-garage`, public, default branch `main`. Renamed from `agentic-builder-engineering-system`; the old URL redirects. The description is set. No topics are set. |
| Collaborators | One: `bdonaldharris` (admin). |
| Rulesets *(2026-11-19)* | Six, all active, defined on the repository:<br>**Protect main** (27087247), targeting the default branch;<br>**Protect staging** (27611942), targeting `refs/heads/staging`;<br>**Protect release candidates** (27611943) and **Restrict release-candidate creation** (27611945), targeting `refs/heads/release/**`;<br>**Protect version tags** and **Restrict version tag creation**, targeting `refs/tags/v*` ([ADR 0003](adr/0003-release-distribution.md#4-repository-protection)).<br>Only the two creation rulesets have a bypass actor: Repository admin. A ruleset's bypass applies to all its rules, so creation is restricted in its own ruleset. |
| **Protect main** and **Protect staging** rules *(2026-11-19)* | The same rules on both:<br>**Restrict deletions.**<br>**Block force pushes.**<br>**Require a pull request before merging:** 0 required approving reviews; stale approvals are not dismissed when new commits are pushed; code-owner review is not required; approval of the most recent push is not required; review threads must be resolved; extra approval is required for unattributed changes; **merge commits only** ([ADR 0004](adr/0004-release-promotion.md#2-merge-commits-only)).<br>**Require status checks to pass:** eight checks, `Format, vet, build, cross-compile`, `Test (ubuntu-latest)`, `Test (macos-latest)`, `Test (windows-latest)`, `Release configuration (snapshot, amd64)`, `Release configuration (snapshot, arm64)`, `Source installer (ubuntu-latest)`, and `Source installer (macos-latest)`, each from GitHub Actions. Branches are not required to be up to date before merging. The checks are also enforced when a branch is created. |
| **Protect release candidates** rules *(2026-11-19)* | **Restrict updates:** no one can push to a `release/**` branch, which therefore receives no commits after it is cut.<br>**Block force pushes.**<br>Deletion is not restricted, so a merged candidate's branch is deleted automatically. |
| **Restrict release-candidate creation** rules *(2026-11-19)* | **Restrict creations:** only Repository admin can create a `release/**` branch ([D-D](https://github.com/bdonaldharris/builder-garage/issues/140#issuecomment-6999760359)). |
| Branch protection on `main` and `staging` | None. The rulesets above are the only protection. |
| GitHub Actions workflows | One repository workflow, CI ([`ci.yml`](../.github/workflows/ci.yml)), on pull requests. Besides the four required checks, it runs `Source installer (ubuntu-latest)` and `Source installer (macos-latest)`, which are not required. GitHub also lists two dynamic workflows of its own, Copilot cloud agent and Dependency Graph. |
| GitHub Actions policy | Actions enabled. All actions and reusable workflows are allowed. SHA pinning is not required. The default workflow token is read-only, and workflows cannot approve pull requests. |
| Merge settings | At the repository level, merge commits, squash, and rebase are all allowed. Auto-merge is off. Head branches are deleted on merge. *(2026-11-19)* The rulesets on `main` and `staging` allow merge commits only. |
| Private vulnerability reporting | Enabled. |
| Secret scanning and push protection | Both enabled. Non-provider patterns and validity checks are disabled. |
| Dependabot | Alerts enabled. Security updates disabled. The repository has Go module dependencies (`go.mod`): `github.com/spf13/cobra` and `go.yaml.in/yaml/v3`, and indirectly `github.com/spf13/pflag` and `github.com/inconshreveable/mousetrap`. |
| Issues / Discussions / Wiki / Projects | Enabled / disabled / enabled / enabled. The wiki is the community wiki ([#70](https://github.com/bdonaldharris/builder-garage/issues/70)). |
| Labels *(2026-11-19)* | GitHub defaults, plus `accessibility`, `chore`, `discovery`, `extension`, `hotfix`, `plugin`, `release`, and the area labels `area:cli`, `area:configuration`, `area:crew`, `area:inspection`, `area:manuals`, `area:shipping`, and `area:workbench`. |
| `CODEOWNERS` | None. |
| Community profile | Health 100%. Detected: README, license, contributing guide, code of conduct, and pull request template. The profile reports no issue template, although `.github/ISSUE_TEMPLATE/` holds issue forms. |
| Recent history | The ten most recent commits on `main` (`70aab17` back to `2cff999`) all belong to merged pull requests, #71 through #79. |
| Remote branches *(2026-11-19)* | `main`, the default branch and release branch, and `staging`, the integration branch, created from `main` at `ce2e300`. |

## Gaps against governance

The repository's settings now enforce these [GOVERNANCE.md](../GOVERNANCE.md) rules:

- **No direct pushes to `staging` or `main`.** The **Protect staging** and **Protect main** rulesets require a pull request, block force pushes and deletion, and have no bypass actors ([Changes to `staging` and `main`](../GOVERNANCE.md#changes-to-staging-and-main)).
- **Merge commits only on `staging` and `main`** ([ADR 0004](adr/0004-release-promotion.md#2-merge-commits-only)).
- **All required checks pass before merge.** Both rulesets require the eight checks [decided under D-C](#required-checks).
- **A release candidate receives no commits,** and only Repository admin can cut one ([ADR 0004](adr/0004-release-promotion.md#6-validating-a-release-candidate)).
- **Private vulnerability reporting**, the preferred channel in [SECURITY.md](../SECURITY.md), is available.

These rules are not enforced by settings:

| Governance rule | Enforced today? | Recommendation |
| --- | --- | --- |
| Maintainer approval before merge | No. The ruleset requires 0 approving reviews, so that the sole maintainer's own pull requests can merge. Approval of other contributors' pull requests is required by governance, not by settings. See [Sole maintainer](#sole-maintainer). | Once a second maintainer exists, require at least one approving review and dismiss stale approvals when new commits are pushed. |
| Independent engineering review before the pull request | Not enforceable by repository settings. | No setting applies. The review is recorded on the originating issue, the pull request template asks for a link to it, and maintainers confirm it when approving or recording acceptance. |

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

**In effect, as observed:** the first configuration. **Protect main** and **Protect staging** require a pull request with zero approving reviews, and have no bypass actors. This is now decided: while the lead maintainer is the only maintainer, required approving reviews may remain at zero on both protected branches ([#120 decision](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6962490422), item 7).

Once a second authorized maintainer is available, maintainer-authored pull requests receive approval from another maintainer under the normal process. The ruleset can then require at least one approving review without a bypass.

## Unresolved decisions

Each decision below belongs to the lead maintainer. Each one stays here, with its observed setting, until the decision is recorded on [#77](https://github.com/bdonaldharris/builder-garage/issues/77) or linked from it.

Since decided, and no longer open:

- **Sole-maintainer ruleset configuration:** the first configuration under [Sole maintainer](#sole-maintainer), zero required approvals on both protected branches ([#120 decision](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6962490422), item 7).
- <a id="required-checks"></a>**Required checks:** the eight checks listed under [Observed state](#observed-state), on both `main` and `staging` ([D-C decision on #140](https://github.com/bdonaldharris/builder-garage/issues/140#issuecomment-6999760359)).

### Dependabot security updates

Whether to enable Dependabot security updates, now that the repository has Go module dependencies, is not recorded. **Observed:** security updates disabled, alerts enabled.

### Actions policy

Whether to keep the current GitHub Actions policy or restrict it is not recorded. **Observed:** all actions and reusable workflows allowed, and SHA pinning not required.

## Temporary GitLab stack ruleset

The six-entry #192 stack exception requires an active temporary ruleset over the exact registered branch refs. The 2026-11-22 readiness check found no stack-specific ruleset or enrolled registry; verify GitHub's live state before relying on this procedure. The ruleset is an operational prerequisite, not something `garage status` can establish.

### Configure before a branch becomes a prerequisite

1. Develop each issue branch from its registered direct prerequisite. Branch creation is allowed from that already-checked predecessor without requiring workflow results at creation.
2. The #196 prerequisite change must merge before the live stack exception can be enrolled. After that merge, the Workflow Owner publishes the counted #192 registry and the maintainer creates and verifies this ruleset from its exact entries before any enrolled PR targets a registered branch.
3. Before another enrolled PR targets that branch, add its exact `refs/heads/<registered-branch>` to the temporary ruleset. Maintain the rule's include list as the Workflow Owner updates the registry; do not use a broad wildcard that protects unrelated branches.
4. Configure the ruleset with no bypass actors; require pull requests, merge commits only, resolved review threads, deletion restriction, and blocked force pushes/non-fast-forward updates. Keep required approving reviews at zero, consistent with **Protect staging** and the sole-maintainer decision; governance's independent-review and maintainer-acceptance gates remain unchanged. Require the same eight GitHub Actions checks as **Protect staging** above. Status checks must gate merges but must not prevent creation of the branch from a predecessor whose checks have already passed.
5. Verify the active ruleset's exact refs, bypass list, and required rules on GitHub before opening a PR against any enrolled target. Record that verification on #192. A ruleset change or missing protection is a material blocker.

### Maintain and remove protection

Keep the ruleset active for all six refs through implementation, maintenance/correction PRs, sequential upstream propagation, and integration into `staging`. Do not push directly or bypass the PR/check gates. Once all six changes and required propagation have reached `staging`, a maintainer with ruleset-administration authority verifies the completed integration, removes the temporary ruleset, and deletes the enrolled stack branches. Update this observed-state document only after confirming the resulting live GitHub state.
