## Governing issue

Closes #

<!--
Substantive changes require an issue. Name it here as #N, Closes #N, Fixes #N, Resolves #N, or
Refs #N; garage status reads this section. Into staging, GitHub does not close the issue on merge:
it stays open until its release is promoted. A promotion pull request names and closes only its
release issue here. Each release-member commit carries `Closes #N` for the member issue it completes;
after promotion, the release issue's member list is checked and any unexpected open member follows the
release runbook's manual fallback.
For a small correction that changes no meaning (typo, broken link, formatting), replace the line
above with "None - small correction". See CONTRIBUTING.md.
-->

## Pull request kind

implementation

<!--
Exactly one of: implementation, promotion, hotfix, clean synchronization, conflicted synchronization.
- implementation: an issue branch into staging, or a registered #192 stack/maintenance destination.
- promotion: release/vX.Y.Z-rc.N into main (docs/release-runbook.md).
- hotfix: a hotfix-designated issue's branch into main.
- clean synchronization: main into staging, without conflicts.
- conflicted synchronization: backmerge/vX.Y.Z into staging, with its own issue and review.
garage status checks the kind against the base and head it permits.
-->

## Stack dependency

<!--
Only for one of the six issues enrolled by the Workflow Owner on #192. Its direct-prerequisite PR must already be bound in the counted registry; a planned entry with an omitted/null prerequisite PR cannot use this declaration to establish eligibility.
Use the registered values; this section alone grants no destination exception.
```yaml
garage_stack: 1
program_issue: 192
sequence: <registered sequence>
direct_prerequisite_issue: <registered issue>
direct_prerequisite_pr: <registered pull request>
target_branch: <prerequisite branch, or its merge target after it lands>
```
-->

## Stack maintenance

<!--
Only for a correction or synchronization to an enrolled branch. Use a temporary
maintenance/stack-192/... head. For a correction, include URLs for correction_record
and agent2_disposition; omit both fields for synchronization. For synchronization
from staging, upstream_pr is 0; for direct-predecessor propagation, identify its registered PR.
```yaml
garage_stack: 1
program_issue: 192
entry_issue: <registered issue>
purpose: correction | synchronize
upstream: <staging or registered direct-predecessor branch>
upstream_pr: <0 or registered direct-predecessor PR>
target_branch: <registered branch>
correction_record: <issue comment URL, correction only>
agent2_disposition: <issue comment URL, correction only>
```
-->

## Summary

<!-- What this pull request changes, in a few sentences. The issue holds the requirements; do not repeat them. -->

## Independent engineering review

<!--
Completed BEFORE this pull request was opened, by a reviewer independent of the implementation.
The reviewed artifact was either the uncommitted change or, under commit-stage review, a pushed commit.
Not required for a small correction; write "Not applicable - small correction".
-->

- Review record (link to the approving comment on the issue, which is the authoritative record of the review):
- Reviewer:

- [ ] The reviewer did not implement this change.
- [ ] Any required corrections were made and independently re-reviewed.
- [ ] This pull request contains the change that was approved, and nothing else.

## Validation

<!-- What was checked, and the result. -->

## Environment / Deployment Requirements

<!--
Non-code actions required for this change to function in its target environment, for example
configuration, secrets, migrations, external resources, or a restart or redeploy.
Do not remove this section. If there are no additional requirements, write "None".
-->

## Maintainer approval

<!-- Nothing to complete here. -->

Independent review approval does not authorize merge. A maintainer approves this pull request separately, and all required checks must pass.

For a promotion pull request, before merging, record on the release issue the [step 4](https://github.com/bdonaldharris/builder-garage/blob/main/docs/release-runbook.md#4-confirm-the-precondition-again) precondition recheck, run immediately before the merge, and the Workflow Owner's [step 5](https://github.com/bdonaldharris/builder-garage/blob/main/docs/release-runbook.md#5-record-acceptance) acceptance.
