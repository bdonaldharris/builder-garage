// Package cli implements the garage command-line interface.
//
// It contains command wiring only. Builder Garage's conceptual components
// (Workbench, Crew, Inspection, Shipping) are not implemented here.
package cli

import "github.com/spf13/cobra"

const rootLong = `Builder Garage is an agentic engineering system for builders: engineering
practices, standards, roles, contracts, and workflows for responsible software
construction with AI agents.

The garage CLI is at its foundation stage. It initializes, upgrades,
validates, shows, and changes a project's Builder Garage configuration,
records private developer-local workflow events, installs and verifies the
canonical skills and the manuals a project adopts at a project's harness
targets, shows the read-only workflow status of an issue or pull request
from GitHub, and reports its own version.`

// NewRootCommand builds the garage command tree. The version argument is the
// value reported by "garage version".
func NewRootCommand(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "garage",
		Short: "Builder Garage: an agentic engineering system for builders",
		Long:  rootLong,
		// A failed command reports its error without repeating the usage text.
		SilenceUsage: true,
	}

	// Only deliberately implemented commands are exposed; Cobra's generated
	// shell-completion command is not part of the current command set.
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		newConfigCommand(),
		newInitCommand(),
		newRecordCommand(),
		newSkillsCommand(version),
		newStatusCommand(),
		newVersionCommand(version),
	)

	return root
}
