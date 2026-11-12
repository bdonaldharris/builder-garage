package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// recordOptions are the caller-supplied fields of one event.
type recordOptions struct {
	Issue    int
	PR       int
	Type     string
	Outcome  string
	URL      string
	Artifact string
}

// recordEnv holds the external dependencies of recording, so tests can
// substitute them.
type recordEnv struct {
	WorkDir  string
	CacheDir func() (string, error)
	Git      gitRunner
	Now      func() time.Time
}

func defaultRecordEnv() (recordEnv, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return recordEnv{}, fmt.Errorf("cannot determine the current directory: %w", err)
	}
	return recordEnv{
		WorkDir:  workDir,
		CacheDir: os.UserCacheDir,
		Git:      runGit,
		Now:      time.Now,
	}, nil
}

func newRecordCommand() *cobra.Command {
	var opts recordOptions

	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record a private, local workflow event for this repository",
		Long: `Record a private workflow event in this developer's local history.

The event is stored in the user cache directory, keyed to the current git
repository, and is never written inside the repository or sent anywhere.
At most 500 events are kept per repository; older events are discarded.

Local history is an annotation only. It is not workflow evidence: GitHub
Issue and pull request records are authoritative, and local history never
authorizes, blocks, or advances work.

Only these fields are stored: a timestamp, the repository key, the issue or
pull request number, the event type, the outcome, an optional link to the
durable GitHub record, and an optional artifact identity.

Event types: ` + strings.Join(historyEventTypes, ", ") + `
Outcomes:    ` + strings.Join(historyOutcomes, ", ") + `

Recording is best effort. If the event cannot be recorded, a warning is
printed and the command still exits successfully.`,
		Example: `  garage record --issue 39 --type implementation --outcome ready \
    --url https://github.com/OWNER/REPO/issues/39#issuecomment-123`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := defaultRecordEnv()
			if err == nil {
				var result historyAppendResult
				result, err = recordEvent(cmd.Context(), opts, env)
				if err == nil {
					return reportRecorded(cmd, opts, result)
				}
			}
			// Best effort: a failed recording warns and never fails the command.
			_, werr := fmt.Fprintf(cmd.ErrOrStderr(), "garage: warning: event not recorded: %v\n", err)
			return werr
		},
	}

	flags := cmd.Flags()
	flags.IntVar(&opts.Issue, "issue", 0, "issue number the event concerns")
	flags.IntVar(&opts.PR, "pr", 0, "pull request number the event concerns")
	flags.StringVar(&opts.Type, "type", "", "event type (required)")
	flags.StringVar(&opts.Outcome, "outcome", "", "event outcome (required)")
	flags.StringVar(&opts.URL, "url", "", "https link to the durable GitHub record")
	flags.StringVar(&opts.Artifact, "artifact", "", "artifact identity (format pending #34)")

	return cmd
}

func reportRecorded(cmd *cobra.Command, opts recordOptions, result historyAppendResult) error {
	target := fmt.Sprintf("issue #%d", opts.Issue)
	if opts.PR != 0 {
		target = fmt.Sprintf("pull request #%d", opts.PR)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Recorded %s (%s) for %s\n",
		opts.Type, opts.Outcome, target); err != nil {
		return err
	}
	if result.Discarded > 0 {
		_, err := fmt.Fprintf(cmd.ErrOrStderr(),
			"garage: warning: discarded %d unreadable history entries\n", result.Discarded)
		return err
	}
	return nil
}

// recordEvent builds an event from opts, identifies the repository, and
// appends the event to its history in the user cache. It never writes inside
// the repository.
func recordEvent(ctx context.Context, opts recordOptions, env recordEnv) (historyAppendResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	kind, number, err := recordTarget(opts)
	if err != nil {
		return historyAppendResult{}, err
	}

	repo, err := identifyRepository(ctx, env.WorkDir, env.Git)
	if err != nil {
		return historyAppendResult{}, err
	}

	cacheDir, err := env.CacheDir()
	if err != nil {
		return historyAppendResult{}, fmt.Errorf("cannot locate the user cache directory: %w", err)
	}
	if !filepath.IsAbs(cacheDir) {
		return historyAppendResult{}, fmt.Errorf("the user cache directory %q is not an absolute path", cacheDir)
	}
	inside, err := isWithin(historyPath(cacheDir, repo.Key), repo.TopLevel)
	if err != nil {
		return historyAppendResult{}, err
	}
	if inside {
		return historyAppendResult{}, fmt.Errorf(
			"refusing to store history inside the repository: the user cache directory %s is within %s",
			cacheDir, repo.TopLevel)
	}

	event := historyEvent{
		Version:  historySchemaVersion,
		Time:     env.Now().UTC().Format(time.RFC3339),
		Repo:     repo.Key,
		Kind:     kind,
		Number:   number,
		Type:     opts.Type,
		Outcome:  opts.Outcome,
		URL:      opts.URL,
		Artifact: opts.Artifact,
	}
	return appendHistoryEvent(cacheDir, event)
}

// recordTarget returns the record kind and number named by exactly one of
// --issue and --pr.
func recordTarget(opts recordOptions) (string, int, error) {
	switch {
	case opts.Issue != 0 && opts.PR != 0:
		return "", 0, fmt.Errorf("%w: give either --issue or --pr, not both", errHistoryInput)
	case opts.Issue != 0:
		return "issue", opts.Issue, validateTarget("issue", opts.Issue)
	case opts.PR != 0:
		return "pr", opts.PR, validateTarget("pr", opts.PR)
	default:
		return "", 0, fmt.Errorf("%w: give --issue or --pr", errHistoryInput)
	}
}

// isWithin reports whether path is root or lies below it, after resolving
// symbolic links in the parts of both paths that exist.
func isWithin(path, root string) (bool, error) {
	resolvedPath, err := resolveExisting(path)
	if err != nil {
		return false, err
	}
	resolvedRoot, err := resolveExisting(root)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil {
		// Different volumes: path cannot be inside root.
		return false, nil
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}

// resolveExisting resolves symbolic links in the longest resolvable prefix of
// path and rejoins the remainder. A component that does not exist, or that is
// not a directory, ends the resolvable prefix; creating the remainder then
// fails later with its own error.
func resolveExisting(path string) (string, error) {
	path = filepath.Clean(path)
	var remainder []string
	current := path
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(remainder) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, remainder[i])
			}
			return resolved, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path, nil
		}
		remainder = append(remainder, filepath.Base(current))
		current = parent
	}
}
