package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// forge is an explicit provider tool, not a workflow orchestrator. Skills own
// permissions, review boundaries and lifecycle decisions before invoking it.
func newForgeCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "forge", Short: "Perform an explicit GitLab workflow provider operation", Long: `Perform one explicitly requested provider operation using configured GitLab
identity and authenticated glab. GitHub workflows retain their existing tools.
Status and the local web interface remain read-only.

Mutation flags assert that the invoking workflow already obtained authorization;
they never grant it. Writes use literal JSON, refuse quick-action lines and
verify durable read-back. Tag and release mutations also require a checked
Workflow Owner release delegation note. No merge, approval, package publishing,
protected-ref change or deletion operation is provided. A failed mutation is
never retried automatically.`, Args: cobra.NoArgs}
	for _, action := range []string{"search", "read", "notes", "create", "update", "note"} {
		cmd.AddCommand(newForgeOperationCommand(action))
	}
	cmd.AddCommand(newForgePolicyCommand(), newForgeReleaseCommand())
	return cmd
}
func newForgeOperationCommand(action string) *cobra.Command {
	var o forgeOperation
	var bodyFile string
	o.Action = action
	cmd := &cobra.Command{Use: action + " <issue|mr> [IID]", Short: action + " a GitLab workflow artifact", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		cfg, err := loadForgeOperationConfig(cmd.Context(), dir, runGit)
		if err != nil {
			return err
		}
		f := effectiveForge(*cfg.Project.Forge)
		if action == "search" || action == "create" {
			if len(args) != 1 {
				return errors.New("search/create accepts a kind, without an IID")
			}
			t, err := parseGitlabTarget([]string{args[0], "1"}, f)
			if err != nil {
				return err
			}
			t.Number = 0
			o.Target = t
		} else {
			t, err := parseGitlabTarget(args, f)
			if err != nil {
				return err
			}
			o.Target = t
		}
		if action == "create" || action == "update" || action == "note" {
			body, err := readOperationBody(bodyFile)
			if err != nil {
				return err
			}
			o.Body = body
		}
		result, err := executeGitlabOperation(cmd.Context(), cfg, o, runGLabInput)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}}
	flags := cmd.Flags()
	switch action {
	case "search":
		flags.StringVar(&o.Search, "search", "", "search text within the selected project")
	case "create", "update", "note":
		flags.StringVar(&bodyFile, "body-file", "", "UTF-8 Markdown file containing the approved complete body")
		flags.StringVar(&o.Actor, "actor", "", "expected authenticated gitlab:HOST/USER")
		flags.BoolVar(&o.Authorized, "authorized", false, "assert the invoking workflow obtained authorization for this exact operation")
		if action == "note" {
			flags.StringVar(&o.Record, "record", "", "decision, discovery, readiness, implementation, correction, review, review-request, pr-review, equivalence, release, promotion or acceptance")
		} else {
			flags.StringVar(&o.Title, "title", "", "approved complete title")
			flags.StringSliceVar(&o.Labels, "label", nil, "approved existing labels (never created)")
		}
		if action == "update" {
			flags.StringVar(&o.State, "state-event", "", "explicitly authorized close or reopen")
		}
		if action == "create" || action == "update" {
			if action == "create" {
				flags.StringVar(&o.Source, "source-branch", "", "MR source branch")
			}
			flags.StringVar(&o.Destination, "target-branch", "", "MR target branch")
		}
	}
	return cmd
}
func readOperationBody(path string) (string, error) {
	if path == "" {
		return "", errors.New("mutation requires --body-file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot read body file: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 1_000_001))
	if err != nil {
		return "", err
	}
	if len(data) > 1_000_000 {
		return "", errors.New("body file exceeds size limit")
	}
	return string(data), nil
}
func loadForgeOperationConfig(ctx context.Context, dir string, git gitRunner) (projectConfig, error) {
	repo, err := identifyRepository(ctx, dir, git)
	if err != nil {
		return projectConfig{}, err
	}
	data, err := readConfigFile(configPathInRepository(repo.TopLevel))
	if err != nil {
		return projectConfig{}, err
	}
	cfg, err := decodeConfig(data)
	if err != nil {
		return cfg, err
	}
	if p := validateConfig(cfg); p != nil {
		return cfg, p
	}
	if cfg.Project.Forge == nil || cfg.Project.Forge.Provider != "gitlab" {
		return cfg, errors.New("forge operations require explicit GitLab configuration; use existing GitHub tools for GitHub")
	}
	return cfg, checkForgeOrigin(ctx, repo.TopLevel, git, cfg)
}

// Policy inspection shares the bounded GET-only provider adapter with status.
// It reports independent native outcomes, never authority to merge or mutate.
func newForgePolicyCommand() *cobra.Command {
	return &cobra.Command{Use: "policy mr IID", Short: "Inspect checked GitLab MR policy evidence without changing it", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		cfg, err := loadForgeOperationConfig(cmd.Context(), dir, runGit)
		if err != nil {
			return err
		}
		target, err := parseGitlabTarget(args, *cfg.Project.Forge)
		if err != nil {
			return err
		}
		if target.Kind != workflowChangeKind {
			return errors.New("policy inspection requires a merge request")
		}
		item, reads, err := (gitlabStatusAdapter{Runner: runGLab, Forge: *cfg.Project.Forge}).Read(cmd.Context(), target, &cfg)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			URL          string               `json:"url"`
			Head         string               `json:"head"`
			Target       string               `json:"target"`
			TargetCommit string               `json:"target_commit"`
			Reads        int                  `json:"reads"`
			Gates        []workflowPolicyGate `json:"gates"`
			Note         string               `json:"note"`
		}{item.URL, item.HeadRefOid, item.BaseRefName, item.BaseRefOid, reads, item.Observation.PolicyGates, "Native evidence only; Agent #2, equivalence, workflow triggers and human authority remain separate. No merge eligibility or authorization is granted."})
	}}
}

// newForgeReleaseCommand groups GitLab release inspection and the delegated
// tag/release mutations (#201).
func newForgeReleaseCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "release", Short: "Inspect or perform a delegated GitLab tag or release operation", Long: `Inspect checked GitLab release facts, or perform one tag or release operation
that a Workflow Owner delegated in a [WORKFLOW OWNER — RELEASE DELEGATION] note
on the release issue. Garage verifies that note before any write. A human who
performs a release operation directly uses glab or the GitLab interface.`, Args: cobra.NoArgs}
	var ref string
	inspect := &cobra.Command{Use: "inspect TAG", Short: "Report checked tag, release, pipeline and asset facts without changing them", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		cfg, err := loadForgeOperationConfig(cmd.Context(), dir, runGit)
		if err != nil {
			return err
		}
		result, err := inspectGitlabRelease(cmd.Context(), *cfg.Project.Forge, args[0], ref, runGLab)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}}
	inspect.Flags().StringVar(&ref, "ref", "", "expected full commit object name of the tag")
	cmd.AddCommand(inspect)
	for _, op := range []struct{ use, operation, short string }{
		{"tag", releaseOpTag, "Create a delegated tag at an exact commit"},
		{"create", releaseOpCreate, "Create a delegated release for an existing tag"},
		{"update", releaseOpUpdate, "Replace a delegated release's title and notes"},
		{"link", releaseOpLink, "Add a delegated asset link to a release"},
	} {
		cmd.AddCommand(newForgeReleaseMutation(op.use, op.operation, op.short))
	}
	return cmd
}

func newForgeReleaseMutation(use, operation, short string) *cobra.Command {
	o := releaseOperation{Operation: operation}
	var notesFile string
	cmd := &cobra.Command{Use: use + " TAG", Short: short, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		cfg, err := loadForgeOperationConfig(cmd.Context(), dir, runGit)
		if err != nil {
			return err
		}
		o.Tag = args[0]
		if operation == releaseOpCreate || operation == releaseOpUpdate {
			if o.Notes, err = readOperationBody(notesFile); err != nil {
				return err
			}
		}
		result, err := executeGitlabRelease(cmd.Context(), cfg, o, runGLabInput)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}}
	flags := cmd.Flags()
	flags.StringVar(&o.Ref, "ref", "", "full commit object name the delegation names")
	flags.StringVar(&o.Actor, "actor", "", "expected authenticated gitlab:HOST/USER")
	flags.StringVar(&o.Delegation, "delegation", "", "URL of the Workflow Owner's release delegation note")
	flags.BoolVar(&o.Authorized, "authorized", false, "assert the invoking workflow obtained authorization for this exact operation")
	switch operation {
	case releaseOpTag:
		flags.StringVar(&o.Message, "message", "", "optional one-line annotation message")
	case releaseOpCreate, releaseOpUpdate:
		flags.StringVar(&o.Title, "title", "", "approved complete release title")
		flags.StringVar(&notesFile, "body-file", "", "UTF-8 Markdown file containing the approved complete release notes")
	case releaseOpLink:
		flags.StringVar(&o.LinkName, "name", "", "asset link name")
		flags.StringVar(&o.LinkURL, "url", "", "HTTPS URL of the durably stored asset")
		flags.StringVar(&o.LinkType, "link-type", "package", "other, package, image or runbook")
	}
	return cmd
}
