package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and change the project configuration",
		Long: `Inspect and change the project configuration in .garage/config.yaml.

The configuration records a project's choices at the extension points the
Builder Garage extension-point registry declares, and Garage tooling settings.

Settings are named as "garage init" names its flags, and take values in the
same formats. "garage config show" lists every setting with its name.

Every change is checked against the complete configuration before it is
written, shown for confirmation, and written atomically. A change that would
leave the configuration invalid is refused and nothing is written. Outside an
interactive terminal, a change is written only with --yes. Editing the file
by hand remains supported.

These commands change only .garage/config.yaml. They never commit, push, or
open pull requests, and they authorize nothing: change configured values
through the project's normal review process.`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(
		newConfigValidateCommand(),
		newConfigShowCommand(),
		newConfigGetCommand(),
		newConfigSetCommand(),
		newConfigUnsetCommand(),
		newConfigAddCommand(),
		newConfigRemoveCommand(),
	)
	return cmd
}

// settingsHelp lists the settings of the given kinds, with their value
// formats and plain-language descriptions.
func settingsHelp(kinds ...optionKind) string {
	var b strings.Builder
	b.WriteString("Settings:\n")
	for _, o := range allOptions() {
		if !slices.Contains(kinds, o.kind) {
			continue
		}
		about := o.about
		if o.required {
			about += " (required)"
		}
		fmt.Fprintf(&b, "  %s %s\n      %s\n", o.name, o.format, about)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func newConfigShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show every setting in plain language",
		Long: `Show every setting of the project configuration in plain language, with
the name to use with "garage config get" and "garage config set". A setting
the project has not set shows the default that applies, marked "(default)".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := currentDir()
			if err != nil {
				return err
			}
			cfg, err := loadConfigForCommand(dir)
			if err != nil {
				return err
			}
			_, err = io.WriteString(cmd.OutOrStdout(), formatShow(describeConfiguration(cfg)))
			return err
		},
	}
}

// formatShow writes each summary line with the setting's name.
func formatShow(lines []summaryLine) string {
	width := 0
	label := func(l summaryLine) string { return l.label + " (" + l.name + ")" }
	for _, l := range lines {
		width = max(width, len(label(l)))
	}
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "%-*s  %s\n", width, label(l), l.value)
	}
	return b.String()
}

func newConfigGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get SETTING",
		Short: "Print one setting's value",
		Long: `Print one setting's value, in the format its "garage init" flag takes. A
list prints one item per line. A setting the project has not set prints the
default that applies, marked "(default)".

` + settingsHelp(optionValue, optionList),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, err := lookupOption(args[0])
			if err != nil {
				return err
			}
			dir, err := currentDir()
			if err != nil {
				return err
			}
			cfg, err := loadConfigForCommand(dir)
			if err != nil {
				return err
			}
			values := o.get(&cfg)
			if values == nil {
				values = []string{o.def + " (default)"}
			}
			_, err = io.WriteString(cmd.OutOrStdout(), strings.Join(values, "\n")+"\n")
			return err
		},
	}
}

// setCompanions are the options "garage config set" takes with a setting,
// so that settings that depend on each other change together. Each is
// itself a setting, named as init's flag.
var setCompanions = map[string]string{
	"automated-reviewer-request":      "automated-reviewer",
	"small-corrections-without-issue": "small-corrections",
	"small-correction-classifier":     "small-corrections",
	"discovery-trigger":               "discovery",
}

func newConfigSetCommand() *cobra.Command {
	var yes bool
	var request, classifier string
	var withoutIssue bool
	var triggers []string
	cmd := &cobra.Command{
		Use:   "set SETTING VALUE",
		Short: "Set one setting",
		Long: `Set one setting to a value, in the format its "garage init" flag takes.
A list setting is changed with "garage config add" and "garage config remove".

Settings that depend on each other are set together:

  set automated-reviewer NAME --automated-reviewer-request TEXT
  set small-corrections enabled --small-correction-classifier ROLE
      [--small-corrections-without-issue]
  set discovery triggers --discovery-trigger "NAME|WHEN" ...

Setting discovery to implementer-judgment removes its discovery triggers,
and disabling small corrections removes small-corrections-without-issue,
because those apply only to the other choice; the change shown before
writing lists them. Any other change that would leave the configuration
invalid is refused with the reason.

` + settingsHelp(optionValue),
		Example: `  garage config set integration-branch main
  garage config set workflow-owner "lead maintainer:alice"`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, err := lookupOption(args[0])
			if err != nil {
				return err
			}
			if o.kind != optionValue {
				return wrongVerb(o, "set")
			}
			var companions []func(*projectConfig) error
			for flag, owner := range setCompanions {
				if !cmd.Flags().Changed(flag) {
					continue
				}
				if owner != o.name {
					return fmt.Errorf("--%s is set together with %s, not %s", flag, owner, o.name)
				}
				companion, _ := optionByName(flag)
				switch flag {
				case "automated-reviewer-request":
					companions = append(companions, func(c *projectConfig) error { return companion.set(c, request) })
				case "small-correction-classifier":
					companions = append(companions, func(c *projectConfig) error { return companion.set(c, classifier) })
				case "small-corrections-without-issue":
					companions = append(companions, func(c *projectConfig) error {
						return companion.set(c, strconv.FormatBool(withoutIssue))
					})
				case "discovery-trigger":
					companions = append(companions, func(c *projectConfig) error {
						companion.unset(c)
						for _, t := range triggers {
							if err := companion.add(c, t); err != nil {
								return err
							}
						}
						return nil
					})
				}
			}
			value := args[1]
			return runConfigChange(cmd, yes, configChange{apply: func(c *projectConfig) error {
				if err := o.set(c, value); err != nil {
					return err
				}
				for _, apply := range companions {
					if err := apply(c); err != nil {
						return err
					}
				}
				return nil
			}})
		},
	}
	f := cmd.Flags()
	f.StringVar(&request, "automated-reviewer-request", "", "how the automated reviewer's review is requested, with automated-reviewer")
	f.StringVar(&classifier, "small-correction-classifier", "", "who decides that a change is a small correction, ROLE[:MEMBERS], with small-corrections")
	f.BoolVar(&withoutIssue, "small-corrections-without-issue", false, "a small correction may be proposed without an issue, with small-corrections")
	f.StringArrayVar(&triggers, "discovery-trigger", nil, "a discovery trigger, NAME|WHEN, with discovery triggers (repeatable; replaces the triggers)")
	addYesFlag(cmd, &yes)
	return cmd
}

func newConfigUnsetCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "unset SETTING...",
		Short: "Remove settings so that their defaults apply",
		Long: `Remove one or more settings, so that their defaults apply. Unsetting a
list removes all of its items. Several settings are removed together, which
is how settings that depend on each other are removed:

  garage config unset small-corrections small-correction-classifier

A required setting cannot be unset; set a new value instead. The retired
setting conditional-guidance may be unset even though it makes the
configuration invalid, provided that the result is valid.

` + settingsHelp(optionValue, optionList, optionRetired),
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var options []configOption
			for _, name := range args {
				o, err := lookupOption(name)
				if err != nil {
					return err
				}
				if o.required {
					return fmt.Errorf("%s is required and cannot be unset; give it a new value with \"garage config set %s\" instead",
						o.name, o.name)
				}
				options = append(options, o)
			}
			retired := len(options) == 1 && options[0].kind == optionRetired
			return runConfigChange(cmd, yes, configChange{retiredUnset: retired, apply: func(c *projectConfig) error {
				for _, o := range options {
					o.unset(c)
				}
				return nil
			}})
		},
	}
	addYesFlag(cmd, &yes)
	return cmd
}

func newConfigAddCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "add SETTING VALUE",
		Short: "Add one item to a list setting",
		Long: `Add one item to a list setting, in the format its "garage init" flag takes.
"add manual NAME" adopts the version of the manual this garage delivers, and
"add skill-target HARNESS" uses the version and date its skill path was
confirmed with.

` + settingsHelp(optionList),
		Example: `  garage config add manual go
  garage config add skill-target codex
  garage config add release-condition "migration|a change alters the schema|a tested rollback"`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, err := lookupOption(args[0])
			if err != nil {
				return err
			}
			if o.kind != optionList {
				return wrongVerb(o, "add")
			}
			value := args[1]
			return runConfigChange(cmd, yes, configChange{apply: func(c *projectConfig) error { return o.add(c, value) }})
		},
	}
	addYesFlag(cmd, &yes)
	return cmd
}

func newConfigRemoveCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove SETTING NAME",
		Short: "Remove one item from a list setting",
		Long: `Remove one item from a list setting, identified by its name: a trigger's
or condition's NAME, a manual's NAME, a skill target's HARNESS, or an
inspection condition's exact text. Removing the last item removes the setting.

` + settingsHelp(optionList),
		Example: `  garage config remove manual go`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, err := lookupOption(args[0])
			if err != nil {
				return err
			}
			if o.kind != optionList {
				return wrongVerb(o, "remove")
			}
			key := args[1]
			return runConfigChange(cmd, yes, configChange{apply: func(c *projectConfig) error { return o.remove(c, key) }})
		},
	}
	addYesFlag(cmd, &yes)
	return cmd
}

func addYesFlag(cmd *cobra.Command, yes *bool) {
	cmd.Flags().BoolVarP(yes, "yes", "y", false, "write the change without asking for confirmation")
}

func runConfigChange(cmd *cobra.Command, yes bool, change configChange) error {
	dir, err := currentDir()
	if err != nil {
		return err
	}
	var ask prompter
	if isInteractive(cmd.InOrStdin(), cmd.OutOrStdout()) {
		ask = linePrompter{in: bufio.NewReader(cmd.InOrStdin()), out: cmd.OutOrStdout()}
	}
	return changeConfiguration(dir, change, ask, yes, cmd.OutOrStdout())
}

func lookupOption(name string) (configOption, error) {
	o, ok := optionByName(name)
	if !ok {
		var names []string
		for _, o := range allOptions() {
			names = append(names, o.name)
		}
		return configOption{}, fmt.Errorf("unknown setting %q; settings: %s", name, strings.Join(names, ", "))
	}
	return o, nil
}

func wrongVerb(o configOption, verb string) error {
	switch o.kind {
	case optionList:
		return fmt.Errorf("%s is a list: change it with \"garage config add\" or \"garage config remove\", not %s", o.name, verb)
	case optionRetired:
		return fmt.Errorf("%s is retired and cannot be changed; remove it with \"garage config unset %s\"", o.name, o.name)
	default:
		return fmt.Errorf("%s holds one value: change it with \"garage config set\", not %s", o.name, verb)
	}
}

func newConfigValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Check .garage/config.yaml against the configuration schema",
		Long: `Check the project configuration in .garage/config.yaml in the current
directory.

Validation is read-only. It checks the schema and every declared, mechanically
checkable value, and reports all problems it finds. It exits with an error when
the configuration is missing, cannot be read, or is invalid.

Validation does not inspect agent sessions, authorize work, or advance
workflow state. It does not certify that a configured authority is human, or
that agents worked independently.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("cannot determine the current directory: %w", err)
			}
			cfg, err := loadValidConfig(projectDir)
			if err != nil {
				return err
			}
			return writeConfigSummary(cmd.OutOrStdout(), cfg)
		},
	}
}

// loadValidConfig reads, strictly decodes, and validates the configuration in
// projectDir.
func loadValidConfig(projectDir string) (projectConfig, error) {
	configPath := filepath.Join(projectDir, garageDirName, configFileName)
	data, err := readConfigFile(configPath)
	if err != nil {
		return projectConfig{}, err
	}
	cfg, err := decodeConfig(data)
	if err != nil {
		return projectConfig{}, fmt.Errorf("cannot parse %s: %w", configPath, err)
	}
	if problems := validateConfig(cfg); problems != nil {
		return projectConfig{}, fmt.Errorf("%s: %w", configPath, problems)
	}
	return cfg, nil
}

func writeConfigSummary(w io.Writer, cfg projectConfig) error {
	pc := cfg.Project
	var b strings.Builder
	fmt.Fprintf(&b, "%s is valid (schema version %d).\n\n",
		filepath.Join(garageDirName, configFileName), cfg.SchemaVersion)
	b.WriteString("Extension points:\n")
	row := func(id, name, value string) { fmt.Fprintf(&b, "  %-4s %-28s %s\n", id, name, value) }

	if pc.IntegrationBranch != nil {
		row("E1", "integration branch", *pc.IntegrationBranch)
	} else {
		row("E1", "integration branch", defaultIntegrationBranch+" (default)")
	}
	row("E2", "added PR-review triggers", namesOrDefault(namedConditionNames(pc.PRReviewTriggers), "none (default)"))
	if pc.AutomatedReviewer != nil {
		row("E3", "automated reviewer", pc.AutomatedReviewer.Name+" (optional input only)")
	} else {
		row("E3", "automated reviewer", "none (default)")
	}
	switch {
	case pc.SmallCorrections == nil:
		row("E4", "small corrections", "disabled (default)")
	case *pc.SmallCorrections.Enabled:
		without := "an issue is required"
		if pc.SmallCorrections.WithoutIssue != nil && *pc.SmallCorrections.WithoutIssue {
			without = "may be proposed without an issue"
		}
		row("E4", "small corrections", "enabled; "+without)
	default:
		row("E4", "small corrections", "disabled")
	}
	if pc.Discovery.Mode == discoveryTriggers {
		row("E5", "discovery", "triggers: "+strings.Join(namedConditionNames(pc.Discovery.Triggers), ", "))
	} else {
		row("E5", "discovery", "implementer judgment, with Workflow Owner escalation")
	}
	var conditions []string
	if pc.ReleaseConditions != nil {
		for _, c := range *pc.ReleaseConditions {
			conditions = append(conditions, c.Name)
		}
	}
	row("E6", "release conditions", namesOrDefault(conditions, "none (default)"))
	if pc.SuggestionsBacklog != nil {
		row("E7", "suggestions backlog", *pc.SuggestionsBacklog)
	} else {
		row("E7", "suggestions backlog", "the "+defaultSuggestionsIssue+" issue (default)")
	}
	a := pc.Authorities
	row("E8", "Workflow Owner", a.WorkflowOwner.Role)
	row("", "change acceptor", a.ChangeAcceptor.Role)
	row("", "product-intent acceptor", a.ProductIntentAcceptor.Role)
	if a.SmallCorrectionClassifier != nil {
		row("", "small-correction classifier", a.SmallCorrectionClassifier.Role)
	}
	var added []string
	if pc.UserFacingInspection != nil {
		added = pc.UserFacingInspection.AdditionalConditions
	}
	if len(added) > 0 {
		row("E9", "user-facing inspection", fmt.Sprintf("canonical trigger plus %d project condition(s)", len(added)))
	} else {
		row("E9", "user-facing inspection", "canonical trigger")
	}
	var manuals []string
	if pc.Guidance != nil {
		catalog, _ := embeddedManualCatalog()
		for _, m := range pc.Guidance.Manuals {
			entry := m.Name + " " + m.Version
			if c := findManual(catalog, m.Name); c != nil && c.Version != m.Version {
				entry += " (this garage delivers " + c.Version + ")"
			}
			manuals = append(manuals, entry)
		}
	}
	row("E10", "manuals", namesOrDefault(manuals, "none (default)"))

	b.WriteString("\nTooling:\n")
	if cfg.Tooling == nil || cfg.Tooling.SkillTargets == nil || len(*cfg.Tooling.SkillTargets) == 0 {
		b.WriteString("  skill targets: none\n")
	} else {
		for _, t := range *cfg.Tooling.SkillTargets {
			fmt.Fprintf(&b, "  skill target %-12s %s scope, confirmed with %s on %s\n",
				t.Harness, t.Scope, t.ConfirmedVersion, t.ConfirmedOn)
		}
	}

	b.WriteString("\nChecked: the schema and every declared, mechanically checkable value.\n")
	b.WriteString("Not certified: that configured authorities are human, that agents worked\n")
	b.WriteString("independently, or any workflow state. GitHub Issue and pull request records\n")
	b.WriteString("remain the authoritative evidence.\n")

	_, err := io.WriteString(w, b.String())
	return err
}

func namedConditionNames(list *[]namedCondition) []string {
	if list == nil {
		return nil
	}
	names := make([]string, 0, len(*list))
	for _, c := range *list {
		names = append(names, c.Name)
	}
	return names
}

func namesOrDefault(names []string, fallback string) string {
	if len(names) == 0 {
		return fallback
	}
	return strings.Join(names, ", ")
}
