# Provider operations

This is the provider binding of the canonical Garage workflows. Read the project's valid `.garage/config.yaml` before selecting tools. With no forge block, existing GitHub projects retain their behavior. Explicit `project.forge` is authoritative and origin must match it. A mirror or an unavailable API never selects another provider. GitLab support ships only with the complete #192 feature; this tranche does not establish general deployment compatibility.

## Names and durable locations

A **change request** is a GitHub pull request (PR) or GitLab merge request (MR). Canonical procedure names `integration-pr-review`, `pr-review`, `--pr` examples and the existing `Agent #2 PR review requested` marker retain their compatibility names. In a GitLab workflow, PR responsibilities apply to the MR, with typed MR targets and `--mr` private history. Do not rename evidence keys, Agent markers, verdicts, role/context assertions or `garage-artifact-v1`.

The selected provider's **governing Issue** is the authoritative record for requirements, discovery, readiness, implementation/correction and the independent implementation-review disposition. Current-change equivalence and supplemental review belong on its **PR/MR**. An MR note cannot replace the issue implementation review. Inline discussions, native approvals, CI and local history never substitute for these records.

On GitHub use top-level Issue/PR comments and existing `gh` or connected GitHub operations. On GitLab use top-level Issue/MR notes, not diff discussions, with `#note_ID` durable links. Fetch exact written content back and verify its body, author and destination. Link the verified record in the handoff before any best-effort local history annotation. Preserve response/supplement chronology and exact artifact identity. Unavailable or truncated evidence never passes a gate.

## GitLab operation tools

`garage forge` performs one explicit operation through configured-host authenticated `glab`; it does not run a workflow or confer authorization. The workflow must first obtain every authorization it requires (including complete issue-draft approval), complete all implementation/review hold points, and identify the expected acting account. Mutations require `--authorized` as an assertion of that already-obtained authorization plus `--actor gitlab:HOST/USER`; neither is proof of human consent. Never use an agent record to impersonate the human decision-maker. When publishing a human decision on their behalf, name the human authority and the executing agent in its body.

GitHub workflows retain existing `gh`/connector tools. Do not route GitHub through this GitLab operation command or rewrite provider-specific API identifiers.

```sh
# Read/search; the project comes from validated configuration.
garage forge search issue --search 'search terms'
garage forge search mr --search 'search terms'
garage forge read issue 123
garage forge read mr 456
garage forge notes issue 123
garage forge notes mr 456

# Only after the canonical workflow authorized these exact fields.
garage forge create issue --title 'Approved title' --body-file /tmp/approved-issue.md --actor gitlab:gitlab.example.com/builder --authorized
garage forge create mr --title 'Approved title' --body-file /tmp/approved-mr.md --source-branch feature/123 --target-branch staging --actor gitlab:gitlab.example.com/builder --authorized
garage forge update issue 123 --title 'Approved complete title' --body-file /tmp/approved-issue.md --actor gitlab:gitlab.example.com/builder --authorized
garage forge update mr 456 --title 'Approved complete title' --body-file /tmp/approved-mr.md --actor gitlab:gitlab.example.com/builder --authorized

garage forge note issue 123 --record readiness --body-file /tmp/readiness.md --actor gitlab:gitlab.example.com/builder --authorized
garage forge note mr 456 --record equivalence --body-file /tmp/equivalence.md --actor gitlab:gitlab.example.com/builder --authorized
```

Replace the example host, account, IIDs, branches and file paths with the actual approved values; never infer that these examples authorize execution. `read`, `update` and `note` also accept a canonical HTTPS target URL; cross-project targeting stays on the configured host and requires explicit enablement. `notes` retrieves a complete bounded top-level note collection with bodies and identities, without granting the authors workflow authority. Search is project-scoped, bounded and incomplete on pagination failure; an incomplete duplicate search cannot authorize issue creation. Create/update requires the complete approved title and description, not an opaque patch. Use `--label NAME` only for approved existing labels; absence/inaccessibility refuses the write rather than creating a label. An authorized update may also explicitly retarget an MR with `--target-branch NAME` or close/reopen with `--state-event close|reopen`; the resulting state is read back. These flags do not authorize a lifecycle change. No merge, native approval, release, tag, deletion, protected-policy or web mutation operation is exposed here.

Successful mutations return `verified: true`, `complete: true` and the exact durable URL only after GET read-back. Read/search output has `verified: false` because no mutation was verified. Failed/uncertain writes return an error; inspect the provider before deciding whether to retry. Never automatically repeat a POST after a timeout or failed read-back. Search/read may contain private provider content: use it only in the authorized context. The tool does not persist it.

Bodies use literal JSON stdin. Lines beginning with an optional space then `/` and a letter are refused, including inside code examples, because GitLab quick actions could mutate lifecycle state. Rephrase or indent such examples with a non-slash prefix; do not use workflow-record transport to execute quick actions. Missing auth, permissions, capabilities or response identity fails closed; errors do not echo authentication stderr/private response bodies.

### Record families

| `--record` | Required destination and content |
| --- | --- |
| `decision` | Governing Issue or relevant MR, first line `[WORKFLOW OWNER — …]`, explicit human decision and executing-agent attribution, E8 Workflow Owner identity |
| `discovery` | Governing Issue, `# Discovery…`, canonical discovery report |
| `readiness` | Governing Issue, `# Implementation Readiness`, canonical readiness report |
| `implementation` | Governing Issue, unchanged Agent #1 implementation marker and `record: implementation` block |
| `correction` | Where the disposition arose (Issue or MR), unchanged Agent #1 correction marker and block |
| `review` | Governing Issue, unchanged Agent #2 marker and `record: review`; current completion in `responds_to` |
| `review-request` | MR, existing `Agent #2 PR review requested` marker and `PR head:` with its actual current head; this is a terminal Agent #1 handoff, never permission to invoke Agent #2 internally |
| `pr-review` | MR, unchanged Agent #2 marker and `record: pr-review`; MR response plus authoritative Issue `supplements` |
| `equivalence` | MR, unchanged Agent #1 marker and commit-stage equivalence block; applicable approving Issue/MR disposition in `equivalent_to`, or `none` |
| `release` / `promotion` | Governing release Issue or relevant MR, `[RELEASE — …]` / `[PROMOTION — …]` heading; canonical release/promotion evidence, no release execution |
| `acceptance` | Relevant Issue/MR, `[WORKFLOW OWNER — CHANGE ACCEPTANCE]`, explicit human project-acceptance statement, E8 change-acceptor identity |

Agent/discovery/readiness/release-record writers need checked Developer-or-higher access. Decision and acceptance writers must match their E8-qualified identity; a platform role alone never establishes it. Role and independence remain assertions made by the separately invoked agent; successful transport cannot certify them. Decision/release headings classify transport only, not approval or release readiness. The existing review/equivalence parser validates the artifact block. The caller still owns whole-artifact inspection and all canonical evidence requirements. For MR Agent records, `issue` must appear in that MR's Governing issue section. Response links are checked against complete bounded top-level notes and current eligible records; malformed, stale or misplaced references cannot establish successful transport.

## Authority configuration

```yaml
project:
  authorities:
    workflow_owner:
      role: lead maintainer
      members:
        - github:builder
        - gitlab:gitlab.example.com/builder
```

`github:USER` and legacy unqualified members identify GitHub accounts. `gitlab:HOST/USER` identifies one GitLab account on one HTTPS host (including a non-default port when used). Roles describe responsibility and never fill in a missing GitLab identity. The actual membership API must establish Developer-or-higher access for Agent notes. Configured identity, project access, human consent, Agent #2 approval and native merge policy remain distinct facts. Native MR approval never becomes an Agent #2 note. Qualified identity support alone cannot certify current-artifact approval policy; that policy remains the CI/MR-policy tranche.

## Templates and local history

Adopters may copy the chosen `.gitlab` Markdown templates from Garage into their own repository through a reviewed change; skill installation does not install repository templates automatically. For GitHub resolve `.github/ISSUE_TEMPLATE/`, honor `config.yml`, retain YAML forms and `.github/pull_request_template.md`. For GitLab inspect `.gitlab/issue_templates/*.md` and `.gitlab/merge_request_templates/*.md`; these are Markdown, not GitHub forms. Choose the matching purpose; ask if ambiguous. If no matching project template exists, use the invoking skill's fallback, when one exists. Preserve required fields and show each complete draft for approval exactly as the authoring workflow requires. The shared authoring procedure's `gh` examples are GitHub-specific examples, not a reason to skip GitLab authorization/template resolution. Use the provider operations above for GitLab and verify exact created title/body and any separately authorized metadata.

After durable read-back, make one best-effort private record attempt: `garage record --issue IID …` or, for GitLab MR notes, `garage record --mr IID …`; GitHub keeps `--pr`. `garage history list --mr IID` filters MR events. GitLab history uses a provider/host/project key distinct from legacy remote-based GitHub history; Issue and MR IIDs remain separate kinds. It stays private, bounded, offline and never workflow evidence. Lost/unavailable local history cannot block or authorize work.

## Limits

`status` and local web routes remain observation-only and cannot invoke provider mutations. The #192 registry/ruleset exception is specific to Builder Garage's GitHub repository; it grants no GitLab adopter destination exception. Builder Garage's own GitHub CI, releases, publishing, distribution and update backend are unchanged. Adopter CI/MR policy, release operations, version/tier/deployment certification and general compatibility claims belong to #200–#202. Operations are bounded to 64 calls and 60 seconds, 20 seconds per call, three 100-item pages and 8 MiB per response; they never retry. Runtime operations require installed authenticated glab, normally trusted HTTPS and resource permissions. Never store credentials/TLS exceptions in Garage config or evidence. No ONEOK access is implied.

The repository's single `docs/gitlab-capability-checklist.md` capability checklist maps every adopter workflow to its owning tranche and evidence; it must be completed before integration/release.

## Adopter CI and native policy inspection

`garage forge policy mr IID` (or a canonical MR URL) reports checked native CI/policy outcomes and API source links through the same bounded GET-only adapter as status. Required unavailable facts never pass. Optional/tier-inapplicable details do not block unrelated capabilities. Agent #2, artifact equivalence, human authority and native policy remain distinct; this read operation grants no merge permission. It handles current source-head and properly bound branch/MR pipelines; synthetic merged-result SHAs are reported unavailable rather than equated to that head. See the repository's `docs/gitlab-ci-policy.md` and `docs/examples/gitlab/` for adopter templates, explicit native configuration/approval/merge-operation guidance, runtime setup and verification limits. Those native examples require their own workflow/human authorization and read-back; the Garage command does not execute them. Builder Garage gains no active GitLab pipeline or production setting change.
