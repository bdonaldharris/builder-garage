## Governing issue

Closes #

<!--
Substantive changes require an issue. Name it here as #N, Closes #N, Fixes #N, Resolves #N, or
Refs #N; garage status reads this section. Into staging, GitHub does not close the issue on merge:
it stays open until its release is promoted. A promotion pull request lists the release issue and
"Closes #N" for every member issue.
For a small correction that changes no meaning (typo, broken link, formatting), replace the line
above with "None - small correction". See CONTRIBUTING.md.
-->

## Pull request kind

implementation

<!--
Exactly one of: implementation, promotion, hotfix, clean synchronization, conflicted synchronization.
- implementation: an issue branch into staging.
- promotion: release/vX.Y.Z-rc.N into main (docs/release-runbook.md).
- hotfix: a hotfix-designated issue's branch into main.
- clean synchronization: main into staging, without conflicts.
- conflicted synchronization: backmerge/vX.Y.Z into staging, with its own issue and review.
garage status checks the kind against the base and head it permits.
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
