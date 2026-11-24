# GitLab release operations and evidence (adopters)

This is the #201 release tranche of the [#192 capability stack](https://github.com/bdonaldharris/builder-garage/issues/192). It lets a GitLab adopter run an authorized release workflow with durable Garage records and checked provider facts. It is governed by the [#201 Workflow Owner decision](https://github.com/bdonaldharris/builder-garage/issues/201#issuecomment-7142494919).

Builder Garage itself is still built, signed, released, published, updated and distributed through its existing GitHub infrastructure. Nothing here changes that, configures Builder Garage's repository, or grants anyone authority to release. Complete GitLab support remains held for the single feature release and the #202 deployment evidence.

## What is authoritative

A GitLab release has two kinds of evidence, as it does on GitHub:

- **Garage records** on the governing release issue or MR: `[RELEASE — …]` and `[PROMOTION — …]` notes, written with `garage forge note --record release|promotion` and read back ([provider operations](provider-operations.md#record-families)). They carry the same markers, locations and E8 authority rules as GitHub release records. They are the workflow record.
- **Native facts**: the tag, its protection, the GitLab Release, its asset links, package files, and tag pipeline. `garage forge release inspect` reports them. They supplement Garage records and never replace them.

A project's release and evidence conditions (E6) remain free text. `garage status` does not evaluate them, for GitLab or for GitHub. Whoever evaluates a condition uses the records and the inspection output.

## Inspect a release

```sh
garage forge release inspect v1.2.0 --ref <full commit SHA>
```

The command is read-only. It uses the same bounded GET-only client as `garage status` and `garage forge policy`, and prints JSON: the tag, its commit, the release URL, the number of reads, each outcome with its API source, and every asset link with its checked durability and SHA-256 digest. `--ref` is optional; with it, a tag at another commit fails.

| Outcome | Required | Passes when |
| --- | --- | --- |
| `tag` | yes | The tag exists and names a full commit (the `--ref` commit, when given). |
| `protected tag` | yes | GitLab reports that a protected-tag rule covers the tag. A deployment that does not report protection is `unavailable`. |
| `release` | yes | A release exists for the tag, at the tag's commit. |
| `durable assets` | yes, when the release has links | Every link resolves to one file in this project's generic package registry, with a reported SHA-256 digest. A job-artifact link, a non-HTTPS link, a missing file, or two different files under one name fails. A link outside the project's registry is `unavailable`: its durability cannot be checked. A release with no links is `not-required`. |
| `tag pipeline` | no | The latest pipeline for the tag and commit is a tag pipeline that succeeded. |
| `signature evidence` | no | Every subject asset has a durably stored signature asset named `<asset>` plus `.sig`, `.asc`, `.sigstore`, `.sigstore.json` or `.cosign.bundle`. |
| `provenance evidence` | no | At least one provenance asset (`.intoto.jsonl`, `.provenance.json` or `.slsa.json`) exists, and every one is durably stored. |

States are `passed`, `failed`, `unavailable` or `not-required`. An unavailable fact never passes. Inaccessible, truncated, malformed or unsupported responses are `unavailable`, and they do not erase unrelated outcomes.

### Signing and provenance

Signing and provenance are capability-specific. Garage checks only what it can read: that the signature or provenance assets exist, are stored in the package registry, and have registry digests. **It performs no cryptographic verification**, so a `passed` signing outcome never means a signature was verified. Record your own verification on the release record.

`Required: no` means Garage does not impose the check. When an adopter's E6 condition requires signing or provenance, that condition fails while the evidence is `failed` or `unavailable`. When no condition applies, the outcome blocks nothing else. On GitLab Self-Managed or Dedicated, a signing arrangement must be established separately. GitLab.com keyless-signing guidance alone does not establish one.

## Human-executed operations and delegation

Creating a tag or a release is a human operation unless the Workflow Owner delegates it. A human who performs it directly uses glab or the GitLab interface, then reads the result back:

```sh
glab api --hostname gitlab.example.com --method POST 'projects/group%2Fproject/repository/tags' -f tag_name=v1.2.0 -f ref=<full commit SHA>
glab release create v1.2.0 --name 'Release 1.2.0' --notes-file notes.md --repo group/project
garage forge release inspect v1.2.0 --ref <full commit SHA>
```

Garage cannot tell a human invoker from an agent. So every `garage forge release` mutation is the delegated path, and it always requires a checked delegation.

### The delegation note

The Workflow Owner posts one note on the governing release issue for each delegated operation:

````markdown
[WORKFLOW OWNER — RELEASE DELEGATION]

```yaml
garage_delegation: 1
issue: 42
operation: release-create   # tag | release-create | release-update | link
tag: v1.2.0
ref: <full commit SHA>
designation: delegated      # delegated | withdrawn
```

Workflow Owner: <name>. Executing agent: <name, or none>.
````

Before any write, Garage checks that the `--delegation` URL names an exact note on an issue in the configured project, and that:

- its first line is the marker, and its block is exactly the form above, with `issue` naming that issue;
- its author is an E8 Workflow Owner identity qualified as `gitlab:HOST/USER` (a platform role alone never qualifies);
- its operation, tag and ref equal the requested ones;
- it is the latest valid record for that operation, tag and ref, so a later `withdrawn` record, or a later delegation that supersedes it, refuses the write;
- the issue's notes were read completely.

The check never replaces `--authorized` (the invoking workflow's assertion that it obtained authorization), `--actor` (checked against the authenticated glab identity), or Developer-or-higher project access. It does not prove human consent beyond the note's authorship.

### Delegated operations

```sh
garage forge release tag v1.2.0 --ref <SHA> --message 'Release 1.2.0' --actor gitlab:gitlab.example.com/agent --delegation 'https://gitlab.example.com/group/project/-/issues/42#note_101' --authorized
garage forge release create v1.2.0 --ref <SHA> --title 'Release 1.2.0' --body-file notes.md --actor gitlab:gitlab.example.com/agent --delegation 'https://gitlab.example.com/group/project/-/issues/42#note_102' --authorized
garage forge release update v1.2.0 --ref <SHA> --title 'Release 1.2.0' --body-file notes.md --actor gitlab:gitlab.example.com/agent --delegation 'https://gitlab.example.com/group/project/-/issues/42#note_103' --authorized
garage forge release link v1.2.0 --ref <SHA> --name app.tar.gz --url 'https://gitlab.example.com/api/v4/projects/7/packages/generic/release/v1.2.0/app.tar.gz' --link-type package --actor gitlab:gitlab.example.com/agent --delegation 'https://gitlab.example.com/group/project/-/issues/42#note_104' --authorized
```

Replace every example value with the actual approved one; the examples authorize nothing.

- `tag` refuses an existing tag. Tags are never moved or replaced.
- `create` requires the tag to exist already at the delegated commit, and refuses an existing release. It never sends a `ref`, so it cannot create a tag implicitly.
- `update` replaces the complete title and notes of the release at the delegated commit.
- `link` refuses a non-HTTPS URL, a URL with credentials, a job-artifact URL, and a name or URL the release already has. Links are never replaced.

Each mutation is verified by GET read-back and returns `verified: true` with the durable URL. A failed or uncertain write returns an error and is never retried; inspect the provider before deciding what to do. No deletion, package publishing, protected-tag change or protected-environment change is provided.

## Durable release storage

Job artifacts expire and are not a release store. Publish release files to the project's generic package registry, versioned by the tag, and link the release to those package files.

[`docs/examples/gitlab/garage-release.yml`](examples/gitlab/garage-release.yml) and [`publish.sh`](examples/gitlab/publish.sh) are an adopter include for this. Copy both into the adopter's `.gitlab/garage/` directory through a reviewed change, and include the YAML from the adopter's pipeline.

- The `garage:release-publish` job runs only in a pipeline for a protected tag. It uploads every regular file in `GARAGE_RELEASE_DIR` (default `dist`, left there by the adopter's own build) to `GARAGE_RELEASE_PACKAGE` (default `release`) at the tag's version.
- It refuses a file that is already published, because published files are never replaced. It also refuses unsafe names, an unprotected tag, and a non-HTTPS API.
- It writes `garage-release-evidence/manifest.txt` with each file's local SHA-256 digest and package URL. The manifest is a job-environment assertion with no authority. `garage forge release inspect` reads the registry's own digests.
- It creates no tag, release or link. Those stay human or delegated operations.
- Set `GARAGE_RELEASE_IMAGE` to a reviewed, digest-pinned image with `sh` and `curl`, and change `stage` to one that exists in the adopter's pipeline.

Validate the composed YAML with the deployment's CI Lint in an isolated test project. Local tests do not certify a runner, a registry, or GitLab scheduling.

## Credentials and limits

- **glab.** Garage operations use the configured host's authenticated glab identity. Garage stores no credential or TLS exception in configuration or evidence, and never accepts a CI job token for a `garage forge` operation.
- **Release permissions.** Creating releases needs at least Developer access. For a protected tag, GitLab may also require permission to create that tag. A refused write fails; Garage never works around protection.
- **Protected tags.** Configure release-tag creation for approved releasers only, then read the protection back ([CI/policy guidance](gitlab-ci-policy.md)). Restrict protected environments, if used, the same way.
- **CI job token.** The template uses `CI_JOB_TOKEN` only to publish package files in its own project, in a protected-tag pipeline. It is not a general identity: do not use it for workflow records, delegation, or another project's packages. Never run an untrusted fork's configuration with the parent project's secrets.
- **Budget.** Inspection and operations share the bounded client limits: 64 requests, 60 seconds, 20 seconds per request, three 100-item pages, 8 MiB per response.

## Evidence and environment handoff

Validate release operations only in isolated GitLab fixture or test projects. Record the inspection output, the delegation note, and every `unavailable` limitation on the release record. Live use needs installed, authenticated glab, release and protected-tag permissions, a package registry, E8-qualified `gitlab:` Workflow Owner identities, normal certificate trust, and your own signing arrangement. No minimum server or client version, tier, or self-managed trust arrangement is inferred from fixtures; #202 owns that compatibility evidence. No ONEOK access is implied.

Update the [single capability checklist](gitlab-capability-checklist.md) with actual evidence and limits.
