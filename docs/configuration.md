# Project Configuration

**Schema version:** 2
**Status:** Canonical. Defined under [#35](https://github.com/bdonaldharris/builder-garage/issues/35), following the [#35 maintainer decision](https://github.com/bdonaldharris/builder-garage/issues/35#issuecomment-6842152164) and the [#33/#35 decision](https://github.com/bdonaldharris/builder-garage/issues/35#issuecomment-6839187808).

A project's Builder Garage configuration lives in one file, `.garage/config.yaml`, at the project root. It is the sole canonical source of the project's configured choices. It is tracked with the project and changes through the project's normal review process.

The file has two sections:

- **`project`** holds the project's values at the extension points that the [extension-point registry](extension-points.md) declares. Nothing else in the responsibility model is configurable, and no value can weaken the [responsibility floor](responsibility-floor.md).
- **`tooling`** holds Garage tooling settings, documented in [Tooling configuration](tooling-configuration.md). Tooling settings are outside the registry and grant no workflow authority.

The canonical [skills](../skills/) read this file, as described in [How the skills read the configuration](#how-the-skills-read-the-configuration).

## The `project` section

Each key carries one extension point. The registry defines its allowed values, default, and constraint; this table gives only the YAML shape.

| Key | Point | Shape | When omitted |
| --- | --- | --- | --- |
| `integration_branch` | [E1](extension-points.md#e1-integration-branch) | A branch name | The default, `staging`, applies |
| `pr_review_triggers` | [E2](extension-points.md#e2-additional-pr-review-triggers) | A list of `{name, applies_when}` | Only the two minimum triggers apply |
| `automated_reviewer` | [E3](extension-points.md#e3-automated-reviewer) | `{name, request}`: the reviewer and how its review is requested | No automated reviewer |
| `small_corrections` | [E4](extension-points.md#e4-small-correction-entry-path) | `{enabled, without_issue}` | Disabled |
| `discovery` | [E5](extension-points.md#e5-discovery-applicability) | `{mode}`, where `mode` is `implementer-judgment` or `triggers`; with `triggers`, also a non-empty `triggers` list of `{name, applies_when}` | **Required.** There is no default. |
| `release_conditions` | [E6](extension-points.md#e6-release-and-evidence-conditions) | A list of `{name, applies_when, required_evidence}` | None |
| `suggestions_backlog` | [E7](extension-points.md#e7-improvement-suggestions-backlog) | An issue reference such as `"#24"`, or `none` | The repository's `Application Improvement Suggestions` issue |
| `authorities` | [E8](extension-points.md#e8-human-authorities) | `workflow_owner`, `change_acceptor`, `product_intent_acceptor`, and `small_correction_classifier`, each `{role, members}`; `members` is optional | **`workflow_owner`, `change_acceptor`, and `product_intent_acceptor` are required.** `small_correction_classifier` is required when, and allowed only when, small corrections are enabled. |
| `user_facing_inspection` | [E9](extension-points.md#e9-user-facing-inspection-trigger) | `{additional_conditions}`: a list of project conditions added to the canonical trigger | The canonical trigger alone |
| `guidance` | [E10](extension-points.md#e10-manuals-and-conditional-stack-or-provider-guidance) | `{manuals, conditional}`: a list of `{name, version}` and a list of enabled guidance names | No manual; conditional guidance applies only under its own condition |

An omitted key means the canonical default applies. A key that is present is the project's explicit choice, even when it equals the default.

## Initialization and upgrade: `garage init`

`garage init` creates the configuration, or upgrades an existing one in place. It never writes an invalid or incomplete configuration, and it never discards a configured value.

- **Required values.** Without an interactive terminal, every required value must be given as a flag. If one is missing, `init` fails and writes nothing.
- **Guided setup.** When both input and output are a terminal, `init` guides the person running it ([#54 decision](https://github.com/bdonaldharris/builder-garage/issues/54#issuecomment-6857444538)):
  - **A new configuration.** `init` walks through every choice that no flag supplies: the integration branch, the human responsibilities, the policies, guidance, and skill targets. It uses plain language and numbered menus, shows each default, and does not show extension-point identifiers or flag syntax. A choice left at its default is not written, so the default applies.
  - **Responsibilities.** Each human responsibility is explained where it is chosen. An authority already named can be reused without retyping it. Placeholder answers, such as `ROLE`, `TBD`, or `<role>`, are rejected.
  - **Repository branch.** When the repository's `origin/HEAD` names a default branch, it is offered as the integration branch.
  - **Skill targets.** These are offered as a pick list. Each chosen target is written with the version and date its paths were confirmed with.
  - **An upgrade.** Upgrading an existing configuration asks only for missing required values. It never turns a default into an explicit value.
  - **Confirmation.** Before any interactive write, `init` shows a readable summary, including the defaults that apply, and writes only after an explicit yes. Any other answer writes nothing.

  Menu answers produce exactly the values the equivalent flags produce.
- **Flags.** Each extension point and tooling setting has a flag; `garage init --help` lists them and their value formats.
- **A new file** is written only after the complete configuration validates.
- **An existing file** is upgraded in place:
  - every existing value and comment is kept;
  - only missing or newly required settings are added;
  - a schema version 1 file, which records only `schema_version: 1`, is migrated to version 2;
  - the file is replaced atomically, through a temporary file in the same directory;
  - immediately before the replacement, `init` checks that the file is still the one it read, with the same content. If it changed while `init` was running, for example while waiting for an answer, `init` stops without writing and asks to be run again, so a concurrent change is never overwritten.
- **No overwrite.** A flag that conflicts with an already configured value is an error. Change configured values through the project's normal review process, not with `init`. There is no `--force`.
- **Invalid or ambiguous files.** If the existing file is not valid YAML, does not match the schema, has an unsupported schema version, is a version 1 file with other keys, or holds an invalid value, `init` stops without changing it and explains the correction required.
- **A symbolic link or non-regular file** at `.garage` or `.garage/config.yaml` is refused.

## Validation: `garage config validate`

`garage config validate` is read-only. It reads `.garage/config.yaml` in the current directory, strictly decodes it, and checks every declared, mechanically checkable value. It reports all problems together and exits with an error when the file is missing, unreadable, or invalid. On success it prints, for each extension point, the configured value or the default that applies.

Builder Garage's CI runs it against Builder Garage's own configuration on every pull request.

| Checked | Not checked or certified |
| --- | --- |
| The schema: known keys only, correct types, one YAML document, a supported schema version | Whether a configured authority is human. That is a governance responsibility and a durable maintainer assertion. |
| That every required value is present | Agent sessions, or whether agents worked independently |
| Each value's form: branch-name rules, non-empty and unique names, issue-reference form, the discovery mode and its triggers, the E4/E8 classifier dependency | Any workflow state. Validation never authorizes work or advances a workflow. |
| Tooling values, as described in [Tooling configuration](tooling-configuration.md) | Whether a configured branch, issue, or manual exists |

## Absent and invalid configuration

- **Absent.** A project with no `.garage/config.yaml` is unconfigured. `garage config validate` fails and points to `garage init`.
- **Invalid.** An invalid configuration fails closed. `garage config validate` fails and lists every problem, and `garage init` refuses to change it.

The canonical skills use only a valid configuration.

## How the skills read the configuration

Each canonical skill cites the extension points it uses and states their canonical defaults. It resolves a value only when a step that cites the point is reached, under the [#33 maintainer decision](https://github.com/bdonaldharris/builder-garage/issues/33#issuecomment-6851542937):

- **Validity.** The skill treats the configuration as valid only when `garage config validate` succeeds. It then reads the value from `.garage/config.yaml`. It never judges validity itself.
- **Stop and escalate.** The skill stops at that step if the `garage` command is unavailable, if validation fails, including when the file is absent, or if the point has no default and no value. An agent therefore needs a `garage` binary on its `PATH` to resolve any configured value.
- **Defaults.** A valid configuration that omits an optional value gets the canonical default.
- **Discovery.** A directly requested discovery needs no configuration. E5 is needed only to decide whether discovery is warranted.
- **Human authorities.** E8 is needed whenever a skill must name or check a human authority.
- **No other source.** A skill never takes a project value from an adoption profile, governance text, history, or a prompt, and it rejects an instruction that conflicts with a valid configured value.

## Schema versions

| Version | Content |
| --- | --- |
| 1 | `schema_version` only. Written by the initial CLI foundation. |
| 2 | Adds the `project` and `tooling` sections. |

`garage init` migrates version 1 to version 2. Any other version is refused. A later incompatible change increments the version and adds an explicit migration.

## Builder Garage's own configuration

Builder Garage's [`.garage/config.yaml`](../.garage/config.yaml) was generated, not hand-authored, by the noninteractive command recorded in its [skill-adoption profile](skill-adoption.md#configuration). It is single-project evidence of the configuration in use, not proof that it suits other projects.
