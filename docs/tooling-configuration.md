# Tooling Configuration

**Status:** Canonical. Defined under [#35](https://github.com/bdonaldharris/builder-garage/issues/35), following decision 1 of the [#35 maintainer decision](https://github.com/bdonaldharris/builder-garage/issues/35#issuecomment-6842152164).

The `tooling` section of [`.garage/config.yaml`](configuration.md) holds settings for Garage's own tools. Tooling settings are **outside** the [extension-point registry](extension-points.md):
- They are not responsibility choices.
- They grant no workflow authority.
- They cannot satisfy or relax any [floor](responsibility-floor.md) rule.

This document is the schema for the section. A tooling key not defined here is an error.

## `skill_targets`

The harness locations where Garage may manage installed copies of the canonical skills. `garage skills install` and `garage skills verify` act on these targets; see [Skill installation](skill-installation.md). This section only records which targets a project uses and what has been confirmed about them.

Each entry has:

| Field | Value |
| --- | --- |
| `harness` | `claude-code`, `codex`, or `copilot-cli`. Each harness may appear once. |
| `scope` | `user`. Only user scope is supported ([#15 decision](https://github.com/bdonaldharris/builder-garage/issues/15#issuecomment-6837361506), item 2). |
| `path.override`, `path.default` | The confirmed path rule for the harness, exactly as in the table below. The override is used when its environment variable is set; otherwise the default is used. |
| `confirmed_version` | The harness version with which the target's behavior was last confirmed |
| `confirmed_on` | The date of that confirmation, written `YYYY-MM-DD` |

The supported targets and their path rules come from the [#37 decision](https://github.com/bdonaldharris/builder-garage/issues/37#issuecomment-6837792244), item 1:

| `harness` | `path.override` | `path.default` |
| --- | --- | --- |
| `claude-code` | `$CLAUDE_CONFIG_DIR/skills` | `~/.claude/skills` |
| `codex` | `$CODEX_HOME/skills` | `~/.codex/skills` |
| `copilot-cli` | `$COPILOT_HOME/skills` | `~/.copilot/skills` |

Garage does not manage `~/.agents/skills` or any other path. A project cannot add a target or change a path rule through configuration: the path fields must equal the confirmed rule, so a configuration cannot direct Garage to write anywhere else. `garage init --skill-target HARNESS@VERSION@YYYY-MM-DD` fills the scope and path rule for the harness.

### Confirmation

`confirmed_version` and `confirmed_on` record evidence about a harness's behavior. They are not a version requirement. When the installed harness version differs from the confirmed version, or cannot be determined, `garage skills install` and `garage skills verify` report the target as **Unverified** ([Skill installation](skill-installation.md#status)):
- This is not an integrity failure.
- The target's behavior must not be represented as confirmed until it is re-tested.
- Re-testing records a new version and date through the project's normal review process.

`garage config validate` checks only the form of these fields. It never runs a harness.
