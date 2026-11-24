# GitLab workflow observation

This is the #198 observation tranche of the [#192 capability stack](https://github.com/bdonaldharris/builder-garage/issues/192). It does not establish a separate public rollout or complete GitLab parity. The #199 [workflow operations](provider-operations.md) tranche adds explicit writes and qualified authority configuration. The #200 [CI/policy tranche](gitlab-ci-policy.md) adds source-linked checked native gate outcomes and an adopter CI include. The #201 [release tranche](gitlab-release.md) adds GET-only release inspection and delegated tag/release operations. Deployment-matrix certification remains a subsequent tranche.

Configure schema-v3 [`project.forge`](configuration.md#forge-identity-schema-3), install `glab`, and authenticate to the configured host using that CLI. Garage stores no credentials or TLS exceptions. HTTPS with normal certificate trust is required; origin must match the configured project. An inaccessible project never selects GitHub instead.

```sh
garage status issue 123
garage status mr 123
garage status https://gitlab.example.com/group/subgroup/project/-/issues/123
garage status https://gitlab.example.com/group/subgroup/project/-/merge_requests/123
```

Issue and MR IIDs are separate namespaces. Bare GitLab numbers and bare `#123` CLI arguments are refused. Within Markdown, `#123` identifies an issue and `!123` an MR; `group/subgroup/project#123` and `group/subgroup/project!123` require `allow_cross_project_targets: true`. Cross-project targets stay on the configured host. Legacy GitHub commands remain unchanged.

The read-only local web view has an artifact-kind selector: select Issue or GitLab MR before looking up an IID. It uses the same provider-aware operation and retains the loopback/capability/origin controls. No editing is provided. GitLab targets do not reuse GitHub private-history entries; `record --mr` and typed history now belong to the explicit [workflow operations](provider-operations.md) path; they cannot mutate provider state.

## Observation boundary

The adapter explicitly selects the authenticated host and GET method for REST-v4 project identity, typed issues/MRs, notes, discussions, MR closing relationships, issue closing MRs, inherited project membership, actual target branch commits, head pipelines, and native approval counts. Only those resources and pagination parameters are allowed. Project IDs learned from an MR's source-project relationship may resolve a fork; arbitrary project IDs/URLs cannot become API requests.

Limits are 64 calls and 60 seconds per observation, 20 seconds per call, three pages of 100 items per collection, and 8 MiB per response. There is no polling or retry. Server continuations cannot change host, endpoint, project or query policy. A full page without totals prompts another bounded page; exhausted budgets and partial failures are explicitly truncated/unavailable. A read that does not establish access is not proof of resource absence or public visibility. Private API payloads and authentication stderr are not persisted or echoed in failures.

Facts include issue/MR identity, state and confidentiality where returned, workflow notes/discussions, typed references and governing relationships, membership, actual source/head/target commit, pipeline facts, native merge/policy facts and approval counts where available. A pipeline is eligible as a current-source-head check only when its SHA matches the observed MR head. A changed/unreadable final MR snapshot invalidates head-bound evidence. Unavailable capabilities, inaccessible notes and omitted fields remain explicit limitations. Every consumed notes/discussions feed must be complete before workflow review or approval equivalence can be established, even when native discussion resolution is not required.

Each report line names GitLab and its checked/asserted basis. Shared Garage evidence parsing, artifact comparisons and review evaluation are reused. Developer-or-higher checked membership is required for GitLab Agent #1/#2 records; missing membership grants no eligibility. Evidence is bound to the observed issue identity before evaluation. Every declared/native governing issue remains required even when a read fails; an incomplete set cannot establish review equivalence. Current completion/disposition links, artifacts and chronology control approval, including superseding rejections and supplemental MR reviews. Historical approvals cannot override the current authoritative state; unchanged applicable artifacts retain approval equivalence. Counts/native approval users never become an Agent #2 review. Human acceptance now requires checked qualified identity, current-diff native approval timestamp/eligibility and stable source/target evidence; unavailable native details cannot satisfy it. No merge eligibility is inferred from that partial evidence.

## Validation and compatibility

Deterministic injected-runner fixtures cover behavior without a live account. Runtime use requires authenticated `glab`; a minimum glab/Self-Managed version and a general GitLab.com/Self-Managed/Dedicated compatibility claim require the later deployment matrix. Missing tier/approval/visibility capability is reported rather than assumed present. Compilation and fixtures do not establish browser UAT or a live GitLab deployment pass.

Provider contracts: [glab API invocation](https://docs.gitlab.com/cli/api/), [REST identity/pagination](https://docs.gitlab.com/api/rest/), [issues](https://docs.gitlab.com/api/issues/), [MRs and closing relationships](https://docs.gitlab.com/api/merge_requests/), [discussions](https://docs.gitlab.com/api/discussions/), [inherited membership](https://docs.gitlab.com/api/project_members/), and [native approvals](https://docs.gitlab.com/api/merge_request_approvals/).
