# GitLab adopter CI and merge policy

This is the #200 adopter tranche of the single #192 feature. Builder Garage remains hosted, built, signed and released on GitHub. Nothing in this guide configures its production repository or authorizes an adopter change. Complete GitLab support remains held for the full feature release and #202 compatibility evidence.

## Install the validation include

Copy [garage-validation.yml](examples/gitlab/garage-validation.yml) and [validate.sh](examples/gitlab/validate.sh) into the adopter's `.gitlab/garage/` directory through a reviewed repository change. Add this complete include to the adopter's existing `.gitlab-ci.yml`:

```yaml
include:
  - local: .gitlab/garage/garage-validation.yml
```

Retain existing includes, application build/tests and `workflow:rules`. The include adds a `garage:validate` job; it does not replace application validation or impose Go on another application. Use the existing `test` stage or change the job to an adopter stage that actually exists. The adopter's workflow must admit `merge_request_event` pipelines. The job runs for MRs and branches, suppresses duplicate branch jobs when an MR is open, and does not run for tags. It does not configure release jobs or signing.

Set non-secret `GARAGE_CI_IMAGE` to an approved, digest-pinned image containing the reviewed Garage binary and a POSIX shell. This guide does not publish an image, install a mutable latest binary or choose a release version. During the held stack, use only an isolated reviewed test build; adopters receive the completed versioned feature. Check the runner's actual architecture and trust setup. No provider authentication is needed for this offline job. Do not run an untrusted fork's configuration with parent-project secrets; native GitLab runner/variable protection remains the administrator's responsibility.

The script executes `garage version` and `garage config validate`. Missing Garage, invalid config and absent config fail the non-advisory job. It writes version/configuration output and a short job-context file to `garage-ci-evidence/`, retained for one week, including on failure. These artifacts are job-environment **assertions**, not provider-checked evidence or durable Agent records. Environment variables can be overridden; the adapter independently checks pipeline/job/MR ownership. Restrict artifact/project visibility appropriately; never put credentials into Garage config. Job artifacts are not a release distribution store.

Validate the composed YAML with the deployment's [CI Lint API](https://docs.gitlab.com/api/lint/) or pipeline editor in an isolated test project. Local parsing and shell execution do not certify GitLab scheduling, edition compatibility or a remote runner. A rejected include, missing stage/image/binary or no MR pipeline is a setup failure, not a passing validation.

## Inspect checked native evidence

```sh
garage status mr 456
garage forge policy mr 456
```

Use the actual configured target or canonical HTTPS MR URL. The `policy` operation shares status's bounded **GET-only** adapter and returns JSON with the observed head, actual target commit and independent gate outcomes: `passed`, `failed`, `unavailable`, or `not-required`, with `required` and API source links. It does not authorize or perform an approval, merge, project change, protected-ref change or retry. Required unavailable facts cannot pass; advisory absence is not a pass and does not block unrelated gates.

| Control | Required/advisory meaning and checked evidence |
| --- | --- |
| Pipeline enforcement | Required baseline: native project `only_allow_merge_if_pipeline_succeeds: true`, `allow_merge_on_skipped_pipeline: false`. Missing/null settings remain unavailable. A scheduled auto-merge flag is not enforcement or success. |
| Current pipeline | Required: exact source-head pipeline in the expected source/target project, bound by the MR pipeline relationship or by the MR's native head-pipeline pointer plus exact source project/branch/SHA for a branch pipeline. Required jobs must be complete and successful; only explicitly advisory `allow_failure` jobs may fail. Empty/partial/unknown job collections cannot pass. |
| Protected target | Required baseline: actual target branch has complete matching native protections, no direct-push grant, no force pushes and a configured MR merge grant. Every applicable exact/wildcard entry is considered; a permissive overlapping rule cannot be hidden by a restrictive rule. Native administration/unprotection power is not claimed to disappear. |
| Native approvals | The MR's **applicable** `approval_state` rules are separate from its list/count of approvers. Explicit `has_approval_rules: false` means no native rule gate is required; unavailable tier-dependent detail is advisory in that case. Unknown applicability or missing required rule facts never passes. Human acceptance remains required even with zero native approval rules. |
| Human acceptance | Required separately: current-diff native approval by the configured `gitlab:HOST/USER` change acceptor with checked project eligibility. Approval timestamp must cover the latest collected diff bound to current source and target; missing/stale/future times or missing eligibility/policy details remain unavailable. Author/committer restrictions are respected; no sole-maintainer exception is inferred. |
| Discussion resolution | Required only when the checked project setting requires it. Unknown setting or incomplete required resolution evidence is unavailable. If native policy explicitly disables the requirement, resolution is not required; incomplete discussion feeds still prevent workflow review/approval assertions because discussion notes carry workflow records. Unrelated pipeline/policy gates retain their own applicability and evidence. |
| Native merge status | Required current opened, non-draft MR and resolved native `mergeable` status. Checking/approval-sync states, omitted fields or a changed final snapshot are unavailable. A mergeable native state is not Agent #2 review or human authorization. |
| Protected tags | Advisory inventory of native tag patterns for MR inspection; required release-specific tag policy is evaluated in its release workflow. Inaccessible tag detail does not block an unrelated MR pipeline. Do not infer release permission from the inventory. |

The operation rechecks the actual target commit, native project settings, approval/diff data and final MR snapshot within its finite request budget. Head movement, retargeting, a changed target or policy, or approval synchronization invalidates the affected facts. It never polls until a gate turns green.

Current-source-head and native branch/MR pipelines are supported with explicit ownership. **Synthetic merged-result/merge-train SHAs are not silently substituted for the source head**; until their provenance is established by the supported evidence path, the pipeline gate is unavailable. This limits that capability rather than claiming merged-result certification. GitLab version/tier/API differences and clock skew may make approval evidence unavailable. Preserve those exact limits for #202 rather than disabling a gate.

Agent #2 approval, artifact equivalence, supplemental-review triggers and environment/human acceptance requirements remain distinct. The authoritative implementation disposition stays on the governing Issue; supplemental review and equivalence stay on the MR. Neither native policy nor a green pipeline manufactures Garage review or final merge eligibility. Existing GitHub check evaluation is unchanged.

## Configure native settings only when authorized

The following are **complete example operations**, not instructions to run on Builder Garage or proof of permission. Replace the example host/project, ref and account IDs with the approved adopter values. First validate forge/origin and the expected authenticated actor. For human decisions, check the relevant E8-qualified identity. Obtain the workflow's explicit authorization for the exact native fields/action; GitLab must also grant the required administrative or approver permission. Use normally trusted HTTPS and host-scoped glab auth. Do not store tokens in files, command examples or Garage evidence.

Inspect first:

```sh
glab api --hostname gitlab.example.com --method GET 'projects/group%2Fproject'
glab api --hostname gitlab.example.com --method GET 'projects/group%2Fproject/protected_branches?per_page=100&page=1'
glab api --hostname gitlab.example.com --method GET 'projects/group%2Fproject/protected_tags?per_page=100&page=1'
glab api --hostname gitlab.example.com --method GET 'projects/group%2Fproject/approvals'
glab api --hostname gitlab.example.com --method GET 'projects/group%2Fproject/merge_requests/456/approval_state'
```

Read every page within a declared budget before judging complete protection. An error/404 can mean unavailable access/capability, not proven absence. Native rule APIs and UI controls vary by edition; absence is not permission to bypass an equivalent Garage gate.

After approval of the **whole** proposed setting body, a project-settings example is:

```json
{
  "only_allow_merge_if_pipeline_succeeds": true,
  "allow_merge_on_skipped_pipeline": false,
  "only_allow_merge_if_all_discussions_are_resolved": false
}
```

Save that approved body as `/tmp/approved-gitlab-project-settings.json`, then use the explicit operation and read back the affected fields:

```sh
glab api --hostname gitlab.example.com --method PUT 'projects/group%2Fproject' --input /tmp/approved-gitlab-project-settings.json
glab api --hostname gitlab.example.com --method GET 'projects/group%2Fproject'
```

The optional discussion setting is a human/project choice; `false` in the example is not a universal recommendation. Configure successful-pipeline enforcement without skipped-pipeline bypass. Require native MR merging for protected integration/release refs, disable direct/force pushes, and review all overlapping/group-inherited rules. Configure release-tag creation to approved releasers. Use the [protected-branch API](https://docs.gitlab.com/api/protected_branches/) and [protected-tag API](https://docs.gitlab.com/api/protected_tags/) or the native settings UI; compare the complete approved ref/access configuration after writing. Do not silently update every rule, remove protection or create tags to make a check pass.

For native approvals, configure eligible reviewers and applicable project/MR approval rules, and document reset-on-push, author and committer restrictions. Reset-on-push is recommended against stale acceptance, but current-artifact proof remains necessary. Native rule counts do not encode Agent roles. Read the MR's applicable approval state rather than assuming stored MR rules include every project rule. See the [approval API](https://docs.gitlab.com/api/merge_request_approvals/). Do not alter approval rules merely to permit a particular merge.

A configured change acceptor may approve only after reviewing the current artifact and meeting its human gate. An explicit native approval body carries the **actual reviewed head**, for example:

```json
{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
```

```sh
glab api --hostname gitlab.example.com --method POST 'projects/group%2Fproject/merge_requests/456/approve' --input /tmp/approved-gitlab-mr-head.json
garage forge policy mr 456
```

Save the approved actual head body to that path first; the example SHA grants no authority. Wait for a stable diff/approval state through a separately requested observation, not an agent polling loop. Read back the native approval and qualified current-artifact evidence. Native refusal, required reauthentication or unsupported capability remains a failure/unavailable outcome; never put a password in Garage config or evidence.

Only after every independent-review, trigger, CI, human-acceptance and deployment gate and every explicit hold has been satisfied may an authorized maintainer execute the native merge workflow. GitLab's [MR API](https://docs.gitlab.com/api/merge_requests/#merge-a-merge-request) uses a current reviewed `sha` guard and `auto_merge` for merge-on-success; the older `merge_when_pipeline_succeeds` parameter is deprecated. A complete example request, after those gates and explicit merge authorization, is:

```json
{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","auto_merge":true}
```

Save the approved actual head body as `/tmp/approved-gitlab-merge.json`, then:

```sh
glab api --hostname gitlab.example.com --method PUT 'projects/group%2Fproject/merge_requests/456/merge' --input /tmp/approved-gitlab-merge.json
glab api --hostname gitlab.example.com --method GET 'projects/group%2Fproject/merge_requests/456'
```

Scheduling is not evidence of completion. Read back the resulting MR/head/target/merge state, preserve all Garage records, and never retry an uncertain mutation automatically. This tranche does not perform a feature merge, release or native administration.

## Evidence and environment handoff

Use isolated fixtures/test projects for verification. Preserve source URLs, selected host/project, actual head/target, native policy applicability and every unavailable limitation in the governing Issue/MR handoff. Live compatibility requires installed authenticated glab, appropriate project/approval API access, a suitable isolated runner/image and normal certificate trust. No universal CI-job-token scope, paid tier, minimum server/client version or self-managed trust arrangement is inferred from fixtures. No ONEOK access is implied.

Update the [single capability checklist](gitlab-capability-checklist.md) with actual evidence and limits. #201 owns release operations; #202 owns integrated GitLab.com/Self-Managed/Dedicated compatibility evidence. Builder Garage's `.github` CI, releases, signing, rulesets and distribution remain unchanged.
