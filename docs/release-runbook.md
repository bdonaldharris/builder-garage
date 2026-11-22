# Release Runbook

This runbook is the procedure for releasing `garage` under [ADR 0004](adr/0004-release-promotion.md): from `staging`, through a short-lived release candidate, to `main`, a version tag, and a published release. The release workflow itself, and how to verify what it publishes, are in [ADR 0003](adr/0003-release-distribution.md) and [Verifying a release](release-verification.md).

Every step that needs a human is the maintainer's. An agent never tags or approves the `release` environment. An agent may execute a release-candidate cut only as [step 2](#2-cut-the-release-candidate) defines, after the Workflow Owner explicitly authorizes that exact cut. The authorization and the accountability stay with the human.

## Before you start

- **Use `garage` `v0.3.0` or later, with the skills reinstalled.** Builder Garage's configuration names a release branch (`release_branch`), which earlier releases cannot read: they fail `garage config validate`, and every skill stops ([CONTRIBUTING: Minimum garage version](../CONTRIBUTING.md#minimum-garage-version)). After upgrading, run `garage skills install`.
- **Open a release issue** from the "Release" issue form, labeled `release`. List every member issue and pull request merged into `staging` since the last release, and every hotfix and synchronization issue since then.

## 1. Confirm the release-candidate precondition

No candidate may be cut unless `staging` incorporates every hotfix currently on `main` ([ADR 0004, decision 5](adr/0004-release-promotion.md#5-the-release-candidate-precondition)).

```sh
git fetch origin
git rev-list --no-merges origin/staging..origin/main   # must print nothing
```

- **The output must be empty.** That is the provenance check: promotion merge commits are excluded, because they add no change of their own.
- **Record the command and its output on the release issue,** under "Release-candidate precondition".
- **Behavior check:** each hotfix on `main` must have its synchronization recorded on the release issue, either as a clean synchronization pull request, or as a conflicted-synchronization issue with its review.

Ancestry is provenance only; the synchronization records are the behavior evidence.

## 2. Cut the release candidate

Only Repository admin can create a `release/**` branch, and nobody can push to one afterwards.

```sh
git push origin origin/staging:refs/heads/release/vX.Y.Z-rc.1
```

- **The branch must be a commit on `staging`.** Cutting it from `origin/staging` guarantees that.
- **`X.Y.Z` is the version** the release issue targets, written as a release tag is, without leading zeros. `N` starts at 1.
- **A defect found in the candidate** is fixed by an ordinary issue into `staging`. Then cut `rc.(N+1)` from `staging` again. The candidate itself never receives commits.

### An agent-executed cut

The maintainer may have an agent execute the cut instead. Authorization, execution, and credentials stay separate:

- the Workflow Owner authorizes the exact cut;
- the agent executes it;
- the push succeeds only because it is made with the credentials of an account that holds Repository admin.

The agent holds no repository role of its own, and the "Restrict release-candidate creation" ruleset still applies.

1. **Authorization.** The agent may proceed only after the Workflow Owner has explicitly authorized the exact branch name, `release/vX.Y.Z-rc.N`, and the full commit SHA, in a durable record on the release issue. A request in a conversation is not that authorization. An authorization covers only the branch name and SHA it names.
2. **Recheck, immediately before the push.**

   ```sh
   git fetch origin
   git rev-list --no-merges origin/staging..origin/main          # must print nothing
   git rev-parse origin/staging                                  # must equal the authorized full SHA
   git ls-remote --exit-code origin refs/heads/release/vX.Y.Z-rc.N  # must find nothing (exit status 2)
   ```

   - **If the provenance check prints anything,** or the branch already exists, the agent stops without pushing and reports the failure on the release issue.
   - **If any recheck command itself fails,** the agent also stops without pushing and reports it. That covers a failed `git fetch`, a failed provenance command, or a `git ls-remote` exit status other than 2, such as 128 on a network or authentication error. Only exit status 2 shows that the branch does not exist.
   - **The authorized SHA must still be the `staging` tip.** If `staging` has advanced, the agent stops without pushing. It reports the new tip's full SHA on the release issue and requests fresh authorization for that exact commit. An agent never silently substitutes a different commit for the authorized one.
3. **Push the authorized SHA explicitly,** never a branch reference such as `origin/staging`, which may have moved since the authorization:

   ```sh
   git push origin <authorized full SHA>:refs/heads/release/vX.Y.Z-rc.N
   ```

4. **Record the cut on the release issue.** The record cites the Workflow Owner's authorization, names the agent as the executor, and includes the recheck output and the pushed SHA. GitHub attributes the push to the account whose credentials made it, so this record is what identifies the executor. An agent-executed cut is never described as manually performed.

An agent never overwrites, moves, deletes, or force-pushes an existing candidate. If the authorized branch name already exists, the agent stops.

## 3. Open the promotion pull request

Open a pull request from `release/vX.Y.Z-rc.N` to `main`, using the pull request template:

- **Each release-member commit:** its substantive commit message says `Closes #N` for the one member issue that commit completes. The issue stays open when the commit merges into `staging`; GitHub closes it when that retained commit reaches default branch `main` through the promotion.
- **`## Governing issue`:** `Closes #N` for the release issue only. Do not add member issues as closing lines in the promotion PR body.
- **`## Pull request kind`:** `promotion`.

Then:

- **Required checks:** all eight required checks must pass on the candidate's head. They include the release-configuration snapshot builds and the source-installer jobs. They are the candidate's validation.
- **`garage status <PR>`** should report "pull-request kind: promotion, declared", and that implementation-review gates do not apply.

## 4. Confirm the precondition again

Repeat step 1 just before merging, and record the result on the release issue. If a hotfix landed on `main` since the candidate was cut:

- **A different version from the candidate's:** synchronize the hotfix into `staging`, abandon the candidate by closing its pull request, and cut `rc.(N+1)`.
- **The same version** (a version collision): follow [Version collision](#version-collision).

## 5. Record acceptance

The Workflow Owner records acceptance of the promotion on the release issue.

## 6. Merge

Merge the promotion pull request with a merge commit, the only method `main` allows. After the merge:

- confirm that the release issue closes;
- the candidate branch is deleted automatically;
- confirm that the merge commit's tree equals the candidate head's tree:

  ```sh
  git fetch origin
  test "$(git rev-parse 'origin/main^{tree}')" = "$(git rev-parse '<candidate head commit>^{tree}')"
  ```
- **Verify every member issue listed on the release issue is closed.** For a member that remains open, first check that its accepted change is present in the release. Only then close it manually, and add a durable note on the release issue naming the member, the completion check, and the manual-closure fallback. Do not treat a promotion-PR body closing line as this verification.

No synchronization into `staging` is needed after a promotion. The promotion's merge commit adds nothing `staging` lacks ([ADR 0004, decision 4](adr/0004-release-promotion.md#4-the-promotion-invariant)).

## 7. Tag, dry-run, release

1. **Tag the promotion's merge commit,** as Repository admin:

   ```sh
   git tag vX.Y.Z <merge commit>
   git push origin vX.Y.Z
   ```

2. **Dry-run:** `gh workflow run release.yml --ref vX.Y.Z -f mode=dry-run`. The guard refuses a tag whose commit is not on `main`.
3. **Release:** `gh workflow run release.yml --ref vX.Y.Z -f mode=release`, then approve the protected `release` environment as Workflow Owner.
4. **Record** the tag and the release URL on the release issue.

A tag that fails cannot be reused. A defect found after tagging ships under the next version ([ADR 0003](adr/0003-release-distribution.md#consequences)).

## Hotfixes

A hotfix and its synchronization follow [GOVERNANCE: Hotfixes and staging synchronization](../GOVERNANCE.md#hotfixes-and-staging-synchronization):

1. **Designation:** the Workflow Owner designates the issue with a hotfix-designation record ([E11](extension-points.md#e11-release-branch)).
2. **The hotfix pull request:** it starts from `main`, targets `main`, and declares `## Pull request kind` `hotfix`. Every gate applies.
3. **Release:** after it merges, release the next patch version of `main` through [step 7](#7-tag-dry-run-release).
4. **Synchronization:** synchronize the hotfix into `staging` immediately.
   - **Clean:** a merge-commit pull request from `main` to `staging`, declared `clean synchronization`.
   - **Conflicted:** a linked synchronization issue, and a `backmerge/vX.Y.Z` branch cut from `staging`, with `main` merged into it and the conflict resolved. Its pull request is declared `conflicted synchronization`, and it is independently reviewed and validated before it merges.
5. **Records:** the next release issue lists the hotfix and its synchronization.

## Version collision

When a hotfix is released under the version an open candidate targets (D-E):

1. Abandon the candidate: close its pull request.
2. Close its release issue as superseded, linking the hotfix release in its "Supersession" field.
3. Open a new release issue for the next valid Semantic Versioning release, and carry forward the unreleased accepted work.
4. Cut its first candidate as `release/vNEXT-rc.1`.

A released version is never reused as another candidate.
