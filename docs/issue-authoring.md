# Issue Authoring

**Status:** Canonical. Defined under [#65](https://github.com/bdonaldharris/builder-garage/issues/65), following the [#57 discovery report](https://github.com/bdonaldharris/builder-garage/issues/57#issuecomment-6864804509), the [#57 maintainer decision](https://github.com/bdonaldharris/builder-garage/issues/57#issuecomment-6865008014), and the [#65 decision](https://github.com/bdonaldharris/builder-garage/issues/65#issuecomment-6865606502).

The canonical skills can turn durable evidence into a well-formed GitHub issue. This document summarizes that contract and defines the `garage-authoring` record. The procedure itself is the "Issue authoring" section, which appears identically in each skill that may author. The skills govern; this document does not restate their procedure in full.

## Who authors what

By default, an issue is authored by the role that produced its evidence, from that role's own durable record ([#57 decision](https://github.com/bdonaldharris/builder-garage/issues/57#issuecomment-6865008014), item 1).

| Skill | Technician, position | May author |
| --- | --- | --- |
| [engineering-discovery-workflow](../skills/engineering-discovery-workflow/SKILL.md) | `discovery`, none | follow-ups to its own report, and further discovery requests |
| [application-implementation-workflow](../skills/application-implementation-workflow/SKILL.md) | `implementer`, Agent #1 | scoped follow-ups for work found outside the current issue's scope |
| [independent-implementation-review](../skills/independent-implementation-review/SKILL.md) | `reviewer`, Agent #2 | verified defects outside the current change, and out-of-scope findings dispositioned as follow-up |
| [issue-authoring-workflow](../skills/issue-authoring-workflow/SKILL.md) | `planner`, none | consolidated issues across several records, new proposals shaped from a durable decision, and promoted suggestions |

An unmet acceptance criterion, or a defect, in the current change stays a review blocker. A non-blocking improvement goes to the suggestions backlog (E7), unless the Workflow Owner promotes it.

## The contract

- **Explicit authorization.** An issue is created only when the Builder explicitly authorizes creating it in the current task. A recommendation is not authorization, and neither is approval of a plan, report, or review.
- **Durable evidence.** Every issue links durable GitHub records. An evidence-producing role authors only after its own record is written and verified.
- **Duplicate check.** Open and closed issues are searched, read-only, before drafting. Candidates are shown to the Builder, and never commented on, closed, or linked.
- **Template resolution.**
  1. The repository's `.github/ISSUE_TEMPLATE/` files, read directly, because GitHub's GraphQL `issueTemplates` does not list YAML issue forms. `config.yml` is honored.
  2. Otherwise, the skill's fallback template, `templates/issue.md`. It is shipped with the skill, so `garage skills install` delivers it and `garage skills verify` checks it.

  A configured project override is not supported yet; it would need a new extension point.
- **Confirmation.** The Builder sees the complete draft, including the title, body, labels, template, and duplicate candidates, and answers with an explicit yes.
- **Limited GitHub state.** Only the approved title, body, and labels are set. Labels are never created. Project fields, assignees, milestones, and other issues are left alone, unless the Builder names the action.
- **Verification.** The created issue is read back and checked.
- **No authority.** An authored issue never authorizes implementing it, and never assigns, invokes, or schedules anyone.

## The `garage-authoring` record

Every authored issue's body ends with one fenced YAML block:

```yaml
garage_authoring: 1
checkable:
  source_evidence:
    - "https://github.com/OWNER/REPO/issues/57#issuecomment-6864804509"
  decision: "https://github.com/OWNER/REPO/issues/57#issuecomment-6865008014"
asserted:
  technician: planner          # discovery | readiness | planner | implementer | reviewer
  position: none               # Agent #1 | Agent #2 | none
  authorization: "the Builder's words authorizing creation, or a link to them"
  decision_status: decided     # decided | pending
```

| Element | Classification | Notes |
| --- | --- | --- |
| `source_evidence` | **Checkable** | Required. Each entry is a durable GitHub issue, pull-request, or comment URL, and its existence can be confirmed. |
| `decision` | **Checkable** | Present when `decision_status` is `decided`. A durable URL. |
| The issue's GitHub author and time | **Checkable** from GitHub | The author is an account. Agent-authored issues are posted under the Builder's account, so the author does not identify the role. |
| `technician`, `position` | **Asserted** | Self-declared; nothing certifies them. |
| `authorization` | **Asserted** | Authorization is usually given in a session, so the record quotes it. |
| `decision_status` | **Asserted** | A judgment. `pending` names what remains undecided. |

The record identifies how an issue came to be. It authorizes nothing. `garage status` does not read it ([#57 decision](https://github.com/bdonaldharris/builder-garage/issues/57#issuecomment-6865008014), item 3).

## The fallback template

The fallback asks for:

- **Required:** the problem or outcome, the intended result, the acceptance criteria, the boundaries, the decision status with a link, evidence links, and dependencies.
- **Optional:** suggested labels and type.

The authoring record follows those fields. The template is byte-identical in each authoring skill.
