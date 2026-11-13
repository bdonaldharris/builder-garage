package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect the project configuration",
		Long: `Inspect the project configuration in .garage/config.yaml.

The configuration records a project's choices at the extension points the
Builder Garage extension-point registry declares, and Garage tooling settings.`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newConfigValidateCommand())
	return cmd
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
		for _, m := range pc.Guidance.Manuals {
			manuals = append(manuals, m.Name+" "+m.Version)
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
