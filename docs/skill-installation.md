# Skill Installation

**Status:** Canonical. Implemented under [#38](https://github.com/bdonaldharris/builder-garage/issues/38), following the [#38 maintainer decision](https://github.com/bdonaldharris/builder-garage/issues/38#issuecomment-6853389714), the [#15 decision](https://github.com/bdonaldharris/builder-garage/issues/15#issuecomment-6837361506), and the [#37 decision](https://github.com/bdonaldharris/builder-garage/issues/37#issuecomment-6837792244).

`garage skills install` installs or updates the canonical [skills](../skills/) at a project's declared harness targets, together with the [manuals](../manuals/) the project adopts ([#60 decision](https://github.com/bdonaldharris/builder-garage/issues/60#issuecomment-6860594142)). `garage skills verify` checks them, read-only.

## What Garage installs, and where

- **Embedded skills.** The canonical skills are embedded in the `garage` binary when it is built. Installing and verifying fetch nothing at run time.
- **Declared targets only.** Both commands run only from a project whose `.garage/config.yaml` is valid, and act only on the skill targets it declares ([Tooling configuration](tooling-configuration.md)). `--target HARNESS` limits a command to some of them.
- **Paths.** Each target's directory is resolved from its confirmed path rule: the harness's override variable when it is set, otherwise the default under the home directory.

  | Harness | Directory |
  | --- | --- |
  | `claude-code` | `$CLAUDE_CONFIG_DIR/skills`, or `~/.claude/skills` |
  | `codex` | `$CODEX_HOME/skills`, or `~/.codex/skills` |
  | `copilot-cli` | `$COPILOT_HOME/skills`, or `~/.copilot/skills` |

- **What Garage writes.** Garage writes only one directory per canonical skill, `<target>/<skill>/`, and one per delivered manual, `<target>/builder-garage-manual-<name>/`. It never changes or removes anything else in a target, such as `synced/`, `.trash/`, `.system/`, or another tool's skills.
- **Its own state.** Garage keeps its manifest and backups under the user configuration directory (`os.UserConfigDir()`), in `builder-garage/`:
  - macOS: `~/Library/Application Support/builder-garage/`
  - Linux: `$XDG_CONFIG_HOME/builder-garage/`, or `~/.config/builder-garage/`
  - Windows: `%AppData%\builder-garage\`

  The cache directory is not used, so backups survive. This state is never kept inside a project or a skill target.

## Revision

Each installed `SKILL.md` carries `metadata.garage-revision` in its frontmatter. Garage adds it when it installs; the canonical source never contains it.

- **A clean build** carries the full commit SHA it was built from, taken from the version-control metadata Go embeds at build time.
- **Any other build** is refused: one built from a working tree with local modifications, or with no version-control metadata.
- **`--development`** permits a refused build. The skills are then marked as development and unversioned, with `garage-revision: "development+sha256:<skill-set digest>"`.

## Digests

A digest is written `sha256:<64 hex>`. It is the SHA-256 of one line per file, in path order. Each line is the file's slash-separated path, a NUL byte, the lowercase hex SHA-256 of its bytes, and a newline.

- **A skill's digest** covers every file in its directory, by path within that directory. The manifest records the digest of the bytes as installed, including the added revision.
- **The skill-set digest** covers every embedded file of every skill, each by `<skill>/<path>`, and every embedded manual, by `manuals/<file>`. A development revision is bound to it, so it changes whenever a skill or a manual does.

## Status

`verify` reports each canonical skill at each target as one of:

| Status | Meaning |
| --- | --- |
| **Current** | Installed by Garage, unmodified, from this binary's revision |
| **Stale** | Installed by Garage, unmodified, from another revision. Stale makes no claim about whether that revision is older or newer. |
| **Modified** | Installed by Garage, then changed. This is drift. |
| **Missing** | Installed by Garage, then removed |
| **Unmanaged** | Present, but not installed by Garage. A copy is never adopted, even when its bytes match. |
| **Not installed** | Absent |

It also reports:

- **Extra.** A skill that Garage manages at the target but that is no longer canonical. It is left in place.
- **Collision.** A skill of the same name in another path the harness reads. In Claude Code and Copilot it can shadow the managed copy. In Codex both copies are shown. The paths checked are those #37 established:

  | Harness | Other paths checked |
  | --- | --- |
  | `claude-code` | the project's `.claude/skills` |
  | `codex` | `~/.agents/skills`, and the project's `.agents/skills` and `.codex/skills` |
  | `copilot-cli` | `~/.agents/skills` (only when `COPILOT_HOME` is unset or `~/.copilot`), and the project's `.github/skills`, `.agents/skills`, and `.claude/skills` |

- **Harness version.** Garage runs each harness's `--version`. A version that differs from the target's `confirmed_version`, or that cannot be determined, is reported as **Unverified**. That is a warning, separate from skill integrity: the harness's behavior is not confirmed until it is re-tested.

`verify` exits successfully only when every canonical skill and every manual the project adopts is Current at every selected target, with no Extra skill, no leftover modified manual, and no collision. It writes nothing.

## Manuals

Each manual the project adopts (E10) is delivered as its own skill, `builder-garage-manual-<name>`. Its `SKILL.md` carries the manual's text under a frontmatter that names it, says that it applies only in a project whose configuration adopts it, and records `metadata.garage-manual` and `metadata.garage-manual-version`. Being installed never makes a manual apply.

Skill targets are per user, and shared by every project installed from. The manifest records, for each delivered manual, the projects that adopt it. Worktrees and clones of one repository count as one project.

| When `install` runs in a project that… | It… |
| --- | --- |
| adopts a manual | installs or updates it, like a canonical skill, and records the project. A Modified or Unmanaged copy needs `--replace builder-garage-manual-<name>`, after a backup. |
| does not adopt a manual that another project still adopts | leaves it unchanged, and drops only this project's association |
| does not adopt a manual that no other project adopts | removes it, if it is unmodified. A modified copy is kept and reported. |

`verify` reports each adopted manual's status, marks a manual installed only for other projects as informational, and reports a configured version that differs from the delivered one. It also reports a modified manual that no project adopts any longer. That is a problem to resolve by hand.

## Installing and updating

`install` is safe to run again: when every skill is Current, it changes nothing.

It plans every selected target first. It refuses, and writes nothing, when any of these holds:

- **A collision exists.** There is no override; resolve it by hand.
- **A Modified or Unmanaged copy would be replaced without consent.** `--replace SKILL` consents for that skill. The existing copy is then backed up first, under `skill-backups/<UTC time>/<harness>/<skill>/`, and replaced.
- **A copy cannot be backed up safely**, because it contains a symbolic link or another entry that is not a regular file.
- **The build is not clean**, and `--development` is not given.

Otherwise, Not installed and Missing skills are written, and Stale skills are updated without consent, because they are unmodified Garage installs. Each skill directory is checked again immediately before it is changed. If it changed after planning, Garage stops without changing it.

## Manifest and ownership

The manifest, `skills-manifest.json`, records each target directory Garage manages. For each one it records:

- the harness;
- the source project: its repository key and path;
- when the last installation ran, and with which `garage` version;
- the revision, whether it was a development install, and the skill-set digest;
- for each skill, its revision and installed digest, and, for a manual, the projects that adopt it.

The manifest is version 2. A version 1 manifest is read, and written as version 2 by the next installation.

Ownership is per target directory. The last installation owns it, from whichever project it ran in, and `verify` shows which project that was. An invalid manifest is reported, and never silently replaced.

## Boundaries

- Installing or verifying skills writes no GitHub state. It does not assign, invoke, wait for, or poll agents, and it does not advance workflow.
- A Current result shows only that an installed copy matches this binary's embedded skills. It is not evidence that any workflow step happened.
- Only user scope is supported. Project-scope installs, and harnesses other than the three confirmed ones, are not.
- The harness behavior behind the paths and collisions was confirmed on macOS by #37. Behavior on Linux and Windows has not been confirmed.
