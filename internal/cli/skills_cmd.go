package cli

import (
	"github.com/spf13/cobra"
)

func newSkillsCommand(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Install and verify the canonical skills at this project's skill targets",
		Long: `Install, update, and verify the canonical Builder Garage skills at the
harness skill targets declared in this project's .garage/config.yaml.

The skills, and the manuals a project may adopt, are embedded in this garage
binary; nothing is fetched at run time. Each manual the project adopts is
delivered as the skill builder-garage-manual-<name>.
Garage writes only the skill directories it installs, at the declared targets,
and its own state under the user configuration directory. It never changes
harness-owned or third-party skills.

Installing or verifying skills does not authorize or advance any workflow. A
Current result shows only that an installed copy matches this binary's
embedded skills.`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newSkillsInstallCommand(version), newSkillsVerifyCommand(version))
	return cmd
}

func newSkillsInstallCommand(version string) *cobra.Command {
	var opts installOptions
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install or update the canonical skills at the declared skill targets",
		Long: `Install or update the canonical skills at the skill targets declared in
.garage/config.yaml in the current directory. Running it again changes
nothing when every skill is already Current.

Each manual this project adopts is installed as the skill
builder-garage-manual-<name>, and the project is recorded as adopting it. A
manual skill this project does not adopt is left in place while another
project still adopts it; otherwise an unmodified copy is removed, and a
modified one is kept and reported.

Installation is planned first, and refused without writing anything when:
  - a skill of the same name exists in another path the harness reads (a
    collision). There is no override; resolve it by hand;
  - a copy that is Modified, or Unmanaged (not installed by Garage), would be
    replaced without --replace for that skill. With --replace, the existing
    copy is backed up first;
  - this binary was not built from a clean commit, unless --development is
    given. A development install is marked as development and unversioned.

Missing and Stale skills that Garage installed are written without consent.
Skills that Garage manages but that are no longer canonical are left in
place.

A harness whose installed version differs from its confirmed version, or
cannot be determined, is reported as Unverified. Installation proceeds, but
its behavior is not confirmed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := defaultSkillsEnv(version)
			if err != nil {
				return err
			}
			return installSkills(cmd.Context(), env, opts, cmd.OutOrStdout())
		},
	}
	flags := cmd.Flags()
	flags.StringArrayVar(&opts.Targets, "target", nil, "limit to this declared harness target (repeatable): claude-code, codex, or copilot-cli")
	flags.StringArrayVar(&opts.Replace, "replace", nil, "consent to back up and replace the Modified or Unmanaged copy of this skill (repeatable)")
	flags.BoolVar(&opts.Development, "development", false, "allow installing from a build without clean version-control metadata, marked as development")
	return cmd
}

func newSkillsVerifyCommand(version string) *cobra.Command {
	var targets []string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Check installed skills against this binary's embedded skills, read-only",
		Long: `Check the skills at the skill targets declared in .garage/config.yaml in
the current directory, without changing anything.

Each canonical skill at each target is reported as:
  Current        installed by Garage, unmodified, from this binary's revision
  Stale          installed by Garage, unmodified, from another revision
  Modified       installed by Garage, then changed
  Missing        installed by Garage, then removed
  Unmanaged      present, but not installed by Garage
  Not installed  absent

Each manual the project adopts is reported the same way, together with any
difference between the configured version and the version this garage
delivers. A manual skill installed only for other projects is shown for
information. A modified manual skill that no project adopts is a problem.

A skill Garage manages that is no longer canonical is reported as Extra. A
skill of the same name in another path the harness reads is reported as a
collision. Stale makes no claim about whether a revision is older or newer.

Verify exits successfully only when every canonical skill at every selected
target is Current, with no Extra skill and no collision. An Unverified harness
version is a warning and does not affect the result.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := defaultSkillsEnv(version)
			if err != nil {
				return err
			}
			return verifySkills(cmd.Context(), env, targets, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringArrayVar(&targets, "target", nil, "limit to this declared harness target (repeatable)")
	return cmd
}
