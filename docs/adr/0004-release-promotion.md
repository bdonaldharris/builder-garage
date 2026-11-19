# ADR 0004: Release promotion through `staging` and short-lived release candidates

- **Status:** Accepted
- **Date:** 2026-11-19 (UTC)
- **Decision authority:** Lead maintainer, as Workflow Owner, under [GOVERNANCE.md](../../GOVERNANCE.md). Recorded in:
  - the [release-promotion decision on #120](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6962490422), items 1–8;
  - the [hotfix-synchronization amendment](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6984649202);
  - the [D-A and D-E decisions](https://github.com/bdonaldharris/builder-garage/issues/120#issuecomment-6984929399);
  - the [D-C and D-D decisions on #140](https://github.com/bdonaldharris/builder-garage/issues/140#issuecomment-6999760359).
- **Evidence:** the [#131 discovery report](https://github.com/bdonaldharris/builder-garage/issues/131#issuecomment-6980246909) and its [ancestry correction](https://github.com/bdonaldharris/builder-garage/issues/131#issuecomment-6984930273); the [#140 discovery report](https://github.com/bdonaldharris/builder-garage/issues/140#issuecomment-6999684592).

## Context

Until this decision, every change merged straight to `main`, and a release was tagged on `main` whenever the maintainer chose. Nothing separated accepted-but-unreleased work from released work, and no record listed which issues a release contained. [ADR 0002](0002-semantic-versioning.md) left "when a release is cut" undecided.

## Decision

### 1. The model

```text
issue branch → PR → staging
staging → short-lived release/vX.Y.Z-rc.N branch
release candidate → PR → main → version tag and published release
hotfix: expedited PR → main → patch tag, then synchronization main → staging
```

- **`main`** remains the repository's default branch and the public released-code branch.
- **`staging`** is the configured integration branch ([E1](../extension-points.md#e1-integration-branch)). Ordinary issue branches start from it, and their pull requests target it.
- **`main` is the release branch** ([E11](../extension-points.md#e11-release-branch)). Only a promotion from a release candidate, or a hotfix-designated issue, reaches it.
- **A release candidate** is a short-lived branch, `release/vX.Y.Z-rc.N`, cut from `staging`. There is no permanent `rc` branch.

### 2. Merge commits only

`main` and `staging` accept merge commits only. Squash and rebase merges create new commits. On a promotion or a synchronization, that would break the provenance relationship between the two branches, and the tagged tree would no longer be the tree the release candidate's pull request tested.

### 3. Graph ancestry versus behavioral equivalence

- **Ancestry is provenance only.** Which commits `staging` contains says where changes came from. It never says that a change's required behavior is present.
- **Behavior is established by review and tests.** That a hotfix's required behavior survives in `staging` is shown by:
  - its clean synchronization, which introduces no new change;
  - or, for a conflicted synchronization, its own issue, independent review, and validation.
- **This record never treats ancestry as evidence of behavior.** After a conflicted synchronization, `main` is an ancestor of `staging` even where the `staging` implementation differs from `main`'s.

### 4. The promotion invariant

- **Immediately after a promotion,** the merge-base of `main` and `staging` has `main`'s tree. The promotion's merge commit exists only on `main`, so `main` is not then an ancestor of `staging`. Its tree, however, adds nothing `staging` lacks.
- **While the invariant holds,** the next promotion pull request is conflict-free, and its merge commit's tree equals the release candidate's tree.
- **A hotfix breaks the invariant.** Its synchronization into `staging` restores the graph relationship.

### 5. The release-candidate precondition

No release candidate may be cut or promoted unless `staging` incorporates every hotfix currently on `main`.

- **Provenance check:** `git rev-list --no-merges origin/staging..origin/main` is empty. Promotion merge commits are excluded, because they add no change of their own.
- **Behavior check:** each hotfix on `main` has its synchronization recorded, clean or reviewed, on the release issue.
- **A hotfix that lands after a candidate is cut, under a different version:** synchronize it, abandon the candidate, and cut `rc.(N+1)`.
- **A version collision** (D-E): when a hotfix is released under the version an open candidate targets, that candidate is abandoned. Its release issue is closed as superseded by the hotfix release. A new release issue opens for the next valid Semantic Versioning release and carries the unreleased accepted work forward. Its first candidate is `release/vNEXT-rc.1`. A released version is never reused as another candidate.

### 6. Validating a release candidate

- **A candidate is validated by its promotion pull request's CI,** then by the tag's `dry-run` ([ADR 0003](0003-release-distribution.md)). The required checks on `main` and `staging` include the release-configuration snapshot builds and the source-installer jobs (D-C).
- **No candidate is published as a prerelease.** The release guard accepts only `vMAJOR.MINOR.PATCH` tags whose commit is on `main`.
- **A candidate branch receives no commits.** A fix goes to `staging` through an ordinary issue, and the next `rc.N` is cut. The "Protect release candidates" ruleset blocks pushes to `release/**`, and only Repository admin can create one (D-D).

### 7. Promotion

- **A promotion pull request's gates are:**
  - the release issue's member list, each member with its approval record;
  - the candidate's head being a commit on `staging`;
  - the release-candidate precondition;
  - the required checks;
  - the Workflow Owner's acceptance on the release issue.

  Implementation-review gates do not apply to a declared, matching promotion, because each member was reviewed when it merged into `staging`.
- **Member issues stay open until the promotion pull request closes them.** GitHub's closing keywords act only on a pull request into the default branch. A pull request into `staging` therefore leaves its issue open, and the promotion pull request's description lists `Closes #N` for the release issue and every member.
- **The release** is then tagged on the promotion's merge commit, dry-run, and released, under [ADR 0003](0003-release-distribution.md).

### 8. Hotfixes and synchronization

A hotfix and its synchronization into `staging` are separate artifacts, with separate records, as [GOVERNANCE: Hotfixes and staging synchronization](../../GOVERNANCE.md#hotfixes-and-staging-synchronization) defines.

## Consequences

- **Releases batch accepted work.** A version needs a release issue, a candidate, and a promotion pull request. It is no longer one tag per merged feature.
- **The procedure is in the [release runbook](../release-runbook.md).**
- **A minimum `garage` version applies.** The configuration names `release_branch`, which `garage` releases before `v0.3.0` cannot read. Every environment working on this repository needs `v0.3.0` or later, with the skills reinstalled ([CONTRIBUTING: Minimum garage version](../../CONTRIBUTING.md#minimum-garage-version)).
- **Amends [ADR 0002](0002-semantic-versioning.md):** "when a release is cut" is decided here. A release is cut by a release issue and its promotion.

## Still undecided

- The criteria for 1.0.
