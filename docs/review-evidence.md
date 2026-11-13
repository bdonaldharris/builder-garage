# Revision-Identifiable Review Evidence

**Status:** Adopted. Designed under [#34](https://github.com/bdonaldharris/builder-garage/issues/34), and adopted into the canonical skills' record templates under [#33](https://github.com/bdonaldharris/builder-garage/issues/33). It implements item 6 of the [#16 maintainer decision](https://github.com/bdonaldharris/builder-garage/issues/16#issuecomment-6836671788).

This document defines how a workflow record identifies the exact artifact it covers, and how it separates facts that can be checked from claims that cannot. The record templates in the canonical [skills](../skills/) carry this format, and Agent #2 stops when valid Agent #1 completion evidence in this format is absent. Records use the resolved object name of the integration branch tip as `integration_ref`, never a bare branch name. Automated validation of recorded evidence is [#40](https://github.com/bdonaldharris/builder-garage/issues/40).

## What does not change

- **The issue review stays authoritative.** Agent #2's implementation review on the originating issue governs whether an implementation may advance. A PR review is supplemental. See [GOVERNANCE: Review requirements](../GOVERNANCE.md#review-requirements). A digest identifies what a review covered. It does not create review authority, and it does not replace a review.
- **Garage cannot certify separate execution contexts.** GitHub shows who posted a comment, not which session produced it. On #17 and #25–#27, every Agent #1 and Agent #2 record was posted by the same account. Context separation is always recorded as an assertion. Nothing in this design verifies it, and no tool built on it may claim to.
- **No orchestration.** The design defines values that agents record and that a verifier can recompute. It does not assign, invoke, schedule, or wait for any agent.

## Artifact identity: `garage-artifact-v1`

### What is identified

An artifact is a change: the difference between a **base commit** `B` and a **result tree** `T`.

| Input | Working-tree artifact: an uncommitted change | Committed artifact: a pushed head `H` |
| --- | --- | --- |
| Integration reference `I` | The project's configured integration branch tip, as fetched. Builder Garage: `origin/main`. | Same |
| Base `B` | `git merge-base I HEAD` | `git merge-base I H` while the PR is open. After merge, `git merge-base M^1 M^2`, where `M` is the merge commit. |
| Result tree `T` | The working tree, staged into a **temporary** index (procedure below) | `H^{tree}` |

An artifact's **stage** is either `working-tree` or `commit`. The stage is set by what the reviewer inspects, not by where the record is posted:

- **Working tree.** An implementation reviewed on the issue before commit. Also a correction that Agent #2 re-reviews on the PR before Agent #1 commits it ([integration-pr-review: Artifact equivalence](../skills/integration-pr-review/SKILL.md#artifact-equivalence)).
- **Commit.** A pushed PR head, such as one reviewed in a triggered PR review.

For a working-tree artifact, the working tree is the artifact, because it is what the reviewer inspects. The developer's real index is never read or changed.

### Recorded values

| Value | Definition |
| --- | --- |
| `method` | `garage-artifact-v1` |
| `object_format` | The repository's object format: `sha1` or `sha256` (`git rev-parse --show-object-format`) |
| `base` | The full object name of `B` |
| `tree` | The full object name of `T` |
| `changeset` | `sha256:` followed by the lowercase hex SHA-256 of the exact bytes written by `git diff-tree -r -z --raw --no-abbrev --no-renames B T` |
| `paths` | The number of paths in that output (informational) |

The `changeset` digest hashes Git's raw change listing. For every changed path, that listing gives the old mode, new mode, old object name, new object name, status, and path, with NUL separators. Because each object name is a content hash, the digest binds the full content of every changed file without hashing patch text.

### Equivalence

A PR head `H` is **artifact-equivalent** to an approved artifact `A` when `changeset(H)` equals `A.changeset`. This is the mechanical form of the rule in [integration-pr-review: Artifact equivalence](../skills/integration-pr-review/SKILL.md#artifact-equivalence): the head's changed files and their content match the approved artifact exactly.

- When `B` is unchanged, `tree` equality is a stronger check and also holds.
- After a rebase onto, or a merge of, integration commits that touch **none** of the artifact's paths, `tree` and `base` change, but `changeset` stays equal. The head is still equivalent.
- If the integration branch changed a path the artifact also changes, `changeset` differs. Equivalence is not shown, and minimum trigger 1 applies. This is intended: the approved change was reviewed against different content.

### Edge cases

| Case | Behavior | Shown by |
| --- | --- | --- |
| New untracked file | Included. `git add -A` in the temporary index stages it. | Synthetic test, `g.txt` |
| Ignored file | Excluded, exactly as a commit would exclude it | Synthetic test, `h.log` |
| Binary file | Identified by its object name. Patch text plays no part. | Synthetic test, `b.bin` |
| Rename | Listed as a delete plus an add. `--no-renames` turns detection off, so the result never depends on a similarity threshold. | Synthetic test, `c.txt` → `c2.txt` |
| Deleted file | New mode `000000` and the null object name | Synthetic test, `d.txt` |
| Mode-only change | Captured by the mode columns | Synthetic test, `e.sh`, and a negative case |
| Line endings | The identity is of normalized repository content, as Git would commit it under the repository's `.gitattributes` and `core.autocrlf`. A working-tree change from LF to CRLF in a `text=auto` file is not a change. | Synthetic test, `f.txt` |
| Index differs from working tree | The working tree wins, and the index is ignored | Synthetic test: `a.txt` staged, then edited again |
| Symbolic links and submodules | Covered by Git's own modes (`120000`, `160000`) and object names | Follows from the raw format; not separately exercised |
| Diff configuration | Has no effect. `diff-tree` is a plumbing command, and its raw output does not use diff algorithm, context, prefix, color, or rename settings. | Synthetic test, seven configuration values |
| SHA-1 or SHA-256 repository | Supported. `object_format` is recorded so a verifier compares like with like. | Only `sha1` exercised |
| Sparse checkout | **Not supported in v1.** A temporary index built from `HEAD` would treat files outside the sparse set as deleted. The procedure refuses with an error. | Refusal coded; not exercised |
| Squash or rebase merge, audited after merge | There is no merge commit whose parents give `B`. A post-merge audit must use the `base` in the approving record. | Not exercised |

### Why not hash the patch

Builder Garage's first informational digest, used on #18, was `sha256(git diff --binary <base>)`. It reproduces at the PR stage only when every machine uses the same diff configuration. It also omits untracked files.

- The #18 change hashes to `7794ea3e…` by that method. With `diff.noprefix=true`, the same change hashes to `14d94019…`.
- `git diff <base>` omits a new untracked file, so an approved change that adds a file would not be identified at the issue stage.

`garage-artifact-v1` has neither weakness.

### Reference procedure

This POSIX shell procedure is the reference definition. A Garage implementation must produce identical values. It runs from inside the repository.

```sh
#!/bin/sh
# garage-artifact-v1
#   artifact-v1.sh <integration-ref>            working-tree artifact (stage: working-tree)
#   artifact-v1.sh <integration-ref> <commit>   committed artifact (stage: commit)
set -eu
ref=$1
rev=${2:-}
if [ -n "$rev" ]; then
  base=$(git merge-base "$ref" "$rev")
  tree=$(git rev-parse --verify "$rev^{tree}")
else
  if [ "$(git config --bool core.sparseCheckout || echo false)" = true ]; then
    echo "garage-artifact-v1: sparse checkouts are not supported" >&2
    exit 2
  fi
  base=$(git merge-base "$ref" HEAD)
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  GIT_INDEX_FILE="$tmp/index" git read-tree HEAD
  GIT_INDEX_FILE="$tmp/index" git add -A
  tree=$(GIT_INDEX_FILE="$tmp/index" git write-tree)
fi
changeset=$(git diff-tree -r -z --raw --no-abbrev --no-renames "$base" "$tree" | shasum -a 256 | cut -d' ' -f1)
paths=$(git diff-tree -r -z --name-only --no-renames "$base" "$tree" | tr -cd '\000' | wc -c | tr -d ' ')
echo "base=$(git rev-parse "$base") tree=$tree changeset=sha256:$changeset paths=$paths"
```

For a post-merge audit, pass the merge commit's first parent as `<integration-ref>` and its second parent as `<commit>`.

### Demonstrated results

Against the real #18 change, reconstructed in a throwaway clone:

| Stage | `base` | `tree` | `changeset` |
| --- | --- | --- | --- |
| Issue stage: base `b871324`, approved files uncommitted | `b8713248…` | `b731c035f7ac5bec41d44a95622c593fad4796d2` | `sha256:45532f9d8c5763fda80a94867d7688680d48c5406325a51b89f8468927ddead0` |
| PR stage: head `ae2adf6` | `b8713248…` | `b731c035…` | `sha256:45532f9d…` (identical) |
| After merge: base from the parents of merge commit `9837e3e` | `b8713248…` | `b731c035…` | `sha256:45532f9d…` (identical) |

A synthetic throwaway repository exercised every case in [Edge cases](#edge-cases) that is marked as shown. The working-tree and committed values matched. The changeset survived an unrelated rebase and a merge of the integration branch. A one-byte edit, a mode-only edit, an added file, and an integration change to a path the artifact also changes were each detected.

## Evidence block: `garage-evidence` v1

A workflow record carries one fenced YAML block. The block separates values a verifier can recompute or confirm (`checkable`) from claims no verifier can confirm (`asserted`). The surrounding Markdown comment keeps its existing marker and human-readable sections.

### Where each record lives

Location and links depend on the record type, as the canonical skills place each record. The location says nothing about the artifact's stage.

| `record` | Posted on | `responds_to` | Other required links |
| --- | --- | --- | --- |
| `implementation` | The governing issue | — | — |
| `correction` | The governing issue, after an implementation review; or the PR, after a PR review ([integration-pr-review: Current-PR correction](../skills/integration-pr-review/SKILL.md#phase-4--current-pr-correction)) | — | — |
| `review` | The governing issue | An earlier Agent #1 `implementation` or `correction` record on the governing issue | — |
| `pr-review` | The PR | An earlier record on the same PR for the reviewed artifact: the Agent #1 PR state record or Agent #2 PR Review Request for a triggered review, or the Agent #1 `correction` record for a correction re-review | `supplements`: an earlier approving `review` record on the governing issue. A `pr-review` without one is invalid, because the issue review stays authoritative. |
| `equivalence` | The PR | — | `equivalent_to`: the approving `review` on the governing issue, or the approving `pr-review` on the PR, that the pushed head is equivalent to; `none` when equivalence is not shown, so minimum trigger 1 applies |

### Agent #1 completion record

````markdown
[AGENT #1 — IMPLEMENTATION]

```yaml
garage_evidence: 1
record: implementation        # implementation | correction
issue: 34
checkable:
  artifact:
    method: garage-artifact-v1
    object_format: sha1
    stage: working-tree      # working-tree | commit
    integration_ref: <full object name of the integration branch tip>
    base: <full object name>
    tree: <full object name>
    changeset: sha256:<64 hex>
    paths: 1
asserted:
  status: ready-for-review    # ready-for-review | ready-for-re-review | material-blocker
  validation_run: true        # the listed validation was run; its output is not reproduced here
```

<existing human-readable sections>
````

### Agent #2 review record

````markdown
[AGENT #2 — INDEPENDENT REVIEW]

```yaml
garage_evidence: 1
record: review               # review | pr-review
issue: 34
responds_to: "<URL of the record reviewed; see Where each record lives>"
supplements: "<pr-review only: URL of the approving review on the governing issue>"
checkable:
  artifact:                  # recomputed by the reviewer from the artifact it inspected
    method: garage-artifact-v1
    object_format: sha1
    stage: working-tree      # working-tree | commit, by what was inspected, not by record type
    integration_ref: <full object name of the integration branch tip>
    base: <full object name>
    tree: <full object name>
    changeset: sha256:<64 hex>
    paths: 1
    commit: "<present only when stage is commit: the head reviewed>"
  matches_responds_to: true  # this artifact equals the one in responds_to
asserted:
  execution_context: separate-from-implementation
  inspected: complete-artifact
  answers: {acceptance_criteria_met: true, new_regression: false, works: true}
  verdict: approved          # approved | changes-required
```

<existing human-readable sections>
````

### Equivalence record on a PR

The [artifact-equivalence record](../skills/integration-pr-review/SKILL.md#artifact-equivalence) that Agent #1 posts for each pushed head carries the same block, with `record: equivalence`. Its `checkable.artifact` is computed at `stage: commit` for the pushed head. `equivalent_to` links the approving disposition: the issue review, or a `pr-review` that approved a PR-side correction.

## What can be checked, and what remains asserted

| Element | Classification | How a verifier treats it |
| --- | --- | --- |
| A record exists, its location (issue or PR), its author account, and its time | **Checked** from GitHub | Read from the API |
| Record location and order, by record type ([Where each record lives](#where-each-record-lives)): the record is where its type belongs; `responds_to` is earlier and in the location its type requires; a `pr-review`'s `supplements` is an earlier approving `review` on the governing issue | **Checked** from GitHub | Compare locations, timestamps, and URLs |
| Block present and well formed | **Checked** | Parse the YAML |
| Committed artifact (`stage: commit`), on any record | **Checked** | Recompute from the repository at `commit` |
| Working-tree artifact (`stage: working-tree`), whether recorded on the issue or on the PR | **Checkable only by a holder of the working tree** at the time, such as Agent #2 recomputing it during review. Later it is **checked transitively**: the changeset of the pushed head whose equivalence record links the approval must equal it. | Compare with the equivalence record and the head |
| `matches_responds_to` | **Checked** by comparing the two records' changesets | Compare values; do not trust the boolean |
| CI results, maintainer approving review | **Checked** from GitHub | Read from the API |
| Role (Agent #1 or Agent #2) | **Asserted.** The marker is self-declared, and the author account may be shared. | Report as asserted |
| Separate execution context | **Asserted. Never certified.** | Report as asserted; never as verified |
| The three answers, verdict, and depth of inspection | **Asserted** judgment. A verifier confirms only that they are present. | Report as recorded, not as verified |
| Validation was run | **Asserted** unless reproduced, for example by CI | Report as asserted |

A verifier's output must keep these two columns visible. A "valid" result means only that every checkable element checked. It never means that independence was proven.

## Compatibility

- **Integration PR review.** Changeset equality is the equivalence evidence `integration-pr-review` asks Agent #1 to record. A head whose changeset differs from the latest approved artifact fires minimum trigger 1. Nothing about the two minimum triggers changes.
- **Developer-local history ([#19](https://github.com/bdonaldharris/builder-garage/issues/19#issuecomment-6837362562)).** A local event's "artifact identity" field holds `method`, `base`, and `changeset`. These are object names and a digest, with no file content, so the #19 privacy rules are kept. Local events stay non-authoritative.
- **Project configuration ([#35](https://github.com/bdonaldharris/builder-garage/issues/35)).** `integration_ref` comes from the configured integration branch (extension point E1 in the [#16 report](https://github.com/bdonaldharris/builder-garage/issues/16#issuecomment-6836405183)).
- **Records written before adoption.** Free-text and patch-hash records, such as those on #18, remain valid historical evidence. A verifier reports them as `legacy` and does not try to recompute them.

## Not decided here

- The verifier, its command, and its GitHub access. See [#35](https://github.com/bdonaldharris/builder-garage/issues/35) and [#40](https://github.com/bdonaldharris/builder-garage/issues/40).
- Support for sparse checkouts, and post-merge audit of squash or rebase merges without a recorded `base`.
