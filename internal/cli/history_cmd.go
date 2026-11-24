package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Reading developer-local workflow history (#136).
//
// "garage history" reads the private history "garage record" writes. It is
// offline and read-only: it makes no network or GitHub request, never
// follows a stored link, and never writes, locks, or creates anything in the
// history directory (#19 decision, #136 decisions). Histories are never
// merged: --all-local shows each file, with its events grouped under it.

// historyLabel is the same qualification garage status gives local history.
const historyLabel = "private annotation; may be incomplete; never evidence"

// historyFilePattern matches the history files "garage record" writes. Lock,
// temporary, and other files in the directory are not history.
var historyFilePattern = regexp.MustCompile(`^[0-9a-f]{32}\.jsonl$`)

// Output formats.
const (
	historyFormatText  = "text"
	historyFormatJSONL = "jsonl"
)

// historyEnv holds the external dependencies of reading history, so tests
// can substitute them.
type historyEnv struct {
	WorkDir  string
	CacheDir func() (string, error)
	Git      gitRunner
}

func defaultHistoryEnv() (historyEnv, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return historyEnv{}, fmt.Errorf("cannot determine the current directory: %w", err)
	}
	return historyEnv{WorkDir: workDir, CacheDir: os.UserCacheDir, Git: runGit}, nil
}

type historyListOptions struct {
	Issue    int
	PR       int
	MR       int
	AllLocal bool
	Format   string
}

func newHistoryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Read this developer's private, local workflow history",
		Long: `Read the private workflow history that "garage record" keeps in the user
cache directory, one file per repository identity.

Local history is an annotation only: it may be incomplete, and it is never
workflow evidence. Provider Issue and PR/MR records are authoritative.
These commands are offline and read-only. They never contact GitHub or the
network, never open a stored link, and never change the history.`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newHistoryListCommand(), newHistoryPathCommand())
	return cmd
}

func newHistoryListCommand() *cobra.Command {
	var opts historyListOptions
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List this repository's local workflow events",
		Long: `List the local workflow events recorded for this repository, oldest first:
the time, the issue, PR or MR, the event type, the outcome, and the
link to the provider record.

With --all-local, list every local history file instead, whichever
repository it belongs to: its path and key, its event count and date range,
the provider repositories its events link to, and its events, grouped under
it. Histories are never merged. A file is marked "events link to this
repository" when every link in it points to this checkout's origin
repository; that is a label read from the stored links, not proof.

--format jsonl prints one JSON object per line: each event as it is stored
and, with --all-local, one {"file": {...}} summary before each file's events.`,
		Example: `  garage history list
  garage history list --issue 125
  garage history list --all-local
  garage history list --format jsonl`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := defaultHistoryEnv()
			if err != nil {
				return err
			}
			return runHistoryList(cmd.Context(), env, opts, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	flags := cmd.Flags()
	flags.IntVar(&opts.Issue, "issue", 0, "show only events for this issue")
	flags.IntVar(&opts.PR, "pr", 0, "show only events for this pull request")
	flags.IntVar(&opts.MR, "mr", 0, "show only events for this GitLab merge request")
	flags.BoolVar(&opts.AllLocal, "all-local", false, "list every local history file, each with its own events")
	flags.StringVar(&opts.Format, "format", historyFormatText, "output format: text or jsonl")
	return cmd
}

func newHistoryPathCommand() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "path",
		Short: "Show where this repository's local history is stored",
		Long: `Show the path and key of this repository's local history file, and how many
events it holds. The path is reported even when no event has been recorded
yet.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := defaultHistoryEnv()
			if err != nil {
				return err
			}
			return runHistoryPath(cmd.Context(), env, format, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&format, "format", historyFormatText, "output format: text or jsonl")
	return cmd
}

// historyFilter selects the events of one issue or pull request; a zero
// filter selects every event.
type historyFilter struct {
	Kind   string
	Number int
}

func (f historyFilter) matches(e historyEvent) bool {
	return f.Kind == "" || (e.Kind == f.Kind && e.Number == f.Number)
}

func (f historyFilter) describe() string {
	if f.Kind == "" {
		return "this repository"
	}
	return targetLabel(f.Kind, f.Number)
}

func parseHistoryFilter(issue, pr int, mr ...int) (historyFilter, error) {
	if len(mr) > 0 && mr[0] != 0 {
		if issue != 0 || pr != 0 {
			return historyFilter{}, errors.New("give exactly one of --issue, --pr or --mr")
		}
		return historyFilter{"mr", mr[0]}, filterError(validateTarget("mr", mr[0]))
	}
	switch {
	case issue != 0 && pr != 0:
		return historyFilter{}, errors.New("give either --issue or --pr, not both")
	case issue != 0:
		return historyFilter{"issue", issue}, filterError(validateTarget("issue", issue))
	case pr != 0:
		return historyFilter{"pr", pr}, filterError(validateTarget("pr", pr))
	}
	return historyFilter{}, nil
}

func filterError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(strings.TrimPrefix(err.Error(), errHistoryInput.Error()+": "))
}

func checkHistoryFormat(format string) error {
	if format != historyFormatText && format != historyFormatJSONL {
		return fmt.Errorf("unknown --format %q: use text or jsonl", format)
	}
	return nil
}

func targetLabel(kind string, number int) string {
	if kind == "mr" {
		return fmt.Sprintf("MR !%d", number)
	}
	if kind == "pr" {
		return fmt.Sprintf("PR #%d", number)
	}
	return fmt.Sprintf("issue #%d", number)
}

// historyCacheDir returns the absolute user cache directory.
func historyCacheDir(env historyEnv) (string, error) {
	cacheDir, err := env.CacheDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate the user cache directory: %w", err)
	}
	if !filepath.IsAbs(cacheDir) {
		return "", fmt.Errorf("the user cache directory %q is not an absolute path", cacheDir)
	}
	return cacheDir, nil
}

func runHistoryList(ctx context.Context, env historyEnv, opts historyListOptions, out, errOut io.Writer) error {
	if err := checkHistoryFormat(opts.Format); err != nil {
		return err
	}
	filter, err := parseHistoryFilter(opts.Issue, opts.PR, opts.MR)
	if err != nil {
		return err
	}
	if opts.AllLocal {
		return listAllLocalHistory(ctx, env, filter, opts.Format, out)
	}

	repo, err := historyRepository(ctx, env.WorkDir, env.Git)
	if err != nil {
		return err
	}
	cacheDir, err := historyCacheDir(env)
	if err != nil {
		return err
	}
	path := historyPath(cacheDir, repo.Key)
	events, discarded, err := readHistory(path)
	if err != nil {
		return err
	}
	var shown []historyEvent
	for _, e := range events {
		if filter.matches(e) {
			shown = append(shown, e)
		}
	}

	if opts.Format == historyFormatJSONL {
		if err := writeEventLines(out, shown); err != nil {
			return err
		}
		if discarded > 0 {
			_, err = fmt.Fprintf(errOut, "garage: warning: skipped %d unreadable history entries in %s\n", discarded, path)
		}
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Local history for this repository (%s)\n", historyLabel)
	fmt.Fprintf(&b, "File: %s\n\n", path)
	if len(shown) == 0 {
		if filter.Kind == "" {
			b.WriteString("No local events for this repository yet.\n")
		} else {
			fmt.Fprintf(&b, "No local events for %s.\n", filter.describe())
		}
	} else {
		for _, e := range shown {
			b.WriteString(formatEventLine(e, ""))
		}
		fmt.Fprintf(&b, "\n%s.\n", countNoun(len(shown), "event"))
	}
	if discarded > 0 {
		fmt.Fprintf(&b, "%s skipped.\n", countNoun(discarded, "unreadable entry"))
	}
	_, err = io.WriteString(out, b.String())
	return err
}

// historyFileSummary describes one local history file for --all-local.
type historyFileSummary struct {
	Path           string         `json:"path"`
	Key            string         `json:"key"`
	Current        bool           `json:"current"`
	ThisRepository bool           `json:"this_repository"`
	Events         int            `json:"events"`
	Discarded      int            `json:"discarded"`
	First          string         `json:"first,omitempty"`
	Last           string         `json:"last,omitempty"`
	Links          map[string]int `json:"links"`
	Unlinked       int            `json:"unlinked"`
	Error          string         `json:"error,omitempty"`
}

func listAllLocalHistory(ctx context.Context, env historyEnv, filter historyFilter, format string, out io.Writer) error {
	// Outside a repository there is no current file and no mark; every file
	// is still listed.
	var repo repoIdentity
	if r, err := historyRepository(ctx, env.WorkDir, env.Git); err == nil {
		repo = r
	}
	cacheDir, err := historyCacheDir(env)
	if err != nil {
		return err
	}
	dir := filepath.Join(cacheDir, historyAppDir, historySubDir)
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("cannot read the history directory: %w", err)
	}

	var b strings.Builder
	if format == historyFormatText {
		fmt.Fprintf(&b, "Local history files (%s)\n", historyLabel)
		fmt.Fprintf(&b, "Directory: %s\n", dir)
	}
	files := 0
	for _, entry := range entries {
		if !historyFilePattern.MatchString(entry.Name()) {
			continue
		}
		files++
		path := filepath.Join(dir, entry.Name())
		key := strings.TrimSuffix(entry.Name(), historyFileExt)
		events, discarded, readErr := readHistory(path)
		summary := summarizeHistoryFile(path, key, events, discarded, repo)
		if readErr != nil {
			summary.Error = oneLine(readErr.Error())
		}
		var shown []historyEvent
		for _, e := range events {
			if filter.matches(e) {
				shown = append(shown, e)
			}
		}
		if format == historyFormatJSONL {
			if err := writeJSONLine(&b, struct {
				File historyFileSummary `json:"file"`
			}{summary}); err != nil {
				return err
			}
			if err := writeEventLines(&b, shown); err != nil {
				return err
			}
			continue
		}
		writeFileSummaryText(&b, summary)
		if summary.Error == "" {
			if len(shown) == 0 && len(events) > 0 {
				fmt.Fprintf(&b, "  No local events for %s in this file.\n", filter.describe())
			}
			for _, e := range shown {
				b.WriteString(formatEventLine(e, "  "))
			}
		}
	}
	if format == historyFormatText && files == 0 {
		b.WriteString("\nNo local history files.\n")
	}
	_, err = io.WriteString(out, b.String())
	return err
}

// summarizeHistoryFile counts a file's events, their date range, and the
// repositories their links point to. The "this repository" mark holds only
// when the file has a linked event and every link points to repo's origin.
func summarizeHistoryFile(path, key string, events []historyEvent, discarded int, repo repoIdentity) historyFileSummary {
	s := historyFileSummary{
		Path:      path,
		Key:       key,
		Current:   repo.Key != "" && key == repo.Key,
		Events:    len(events),
		Discarded: discarded,
		Links:     map[string]int{},
	}
	var first, last time.Time
	for _, e := range events {
		if t, err := time.Parse(time.RFC3339, e.Time); err == nil {
			if first.IsZero() || t.Before(first) {
				first, s.First = t, e.Time
			}
			if last.IsZero() || t.After(last) {
				last, s.Last = t, e.Time
			}
		}
		if repository, ok := linkedRepository(e.URL); ok {
			s.Links[repository]++
		} else {
			s.Unlinked++
		}
	}
	if repo.Remote != "" && len(s.Links) > 0 {
		s.ThisRepository = true
		for repository := range s.Links {
			if repository != repo.Remote {
				s.ThisRepository = false
			}
		}
	}
	return s
}

// linkedRepository returns "host/owner/repo", in lower case, for a stored
// link of the form https://host/owner/repo/...; the link is only parsed,
// never fetched.
func linkedRepository(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", false
	}
	if split := strings.Index(u.Path, "/-/"); split > 0 {
		project := strings.TrimPrefix(u.Path[:split], "/")
		if validForgeProject(project, "gitlab") {
			return strings.ToLower(canonicalHTTPSHost(u.Host) + "/" + project), true
		}
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) < 2 || segments[0] == "" || segments[1] == "" {
		return "", false
	}
	return strings.ToLower(u.Hostname() + "/" + segments[0] + "/" + segments[1]), true
}

func writeFileSummaryText(b *strings.Builder, s historyFileSummary) {
	var marks []string
	if s.Current {
		marks = append(marks, "current")
	}
	if s.ThisRepository {
		marks = append(marks, "events link to this repository")
	}
	b.WriteString("\n" + s.Key)
	if len(marks) > 0 {
		b.WriteString("  (" + strings.Join(marks, "; ") + ")")
	}
	b.WriteString("\n")
	fmt.Fprintf(b, "  File: %s\n", s.Path)
	if s.Error != "" {
		fmt.Fprintf(b, "  Unreadable: %s\n", s.Error)
		return
	}
	if s.Events == 0 {
		b.WriteString("  Events: 0\n")
	} else {
		fmt.Fprintf(b, "  Events: %d, %s to %s\n", s.Events, s.First, s.Last)
	}
	if len(s.Links) > 0 {
		repositories := make([]string, 0, len(s.Links))
		for r := range s.Links {
			repositories = append(repositories, r)
		}
		sort.Strings(repositories)
		parts := make([]string, len(repositories))
		for i, r := range repositories {
			parts[i] = fmt.Sprintf("%s (%d)", r, s.Links[r])
		}
		fmt.Fprintf(b, "  Links: %s\n", strings.Join(parts, ", "))
	}
	if s.Unlinked > 0 {
		fmt.Fprintf(b, "  Without a link: %d\n", s.Unlinked)
	}
	if s.Discarded > 0 {
		fmt.Fprintf(b, "  Unreadable entries skipped: %d\n", s.Discarded)
	}
}

func formatEventLine(e historyEvent, indent string) string {
	line := fmt.Sprintf("%s%s  %s  %s  %s", indent, e.Time, targetLabel(e.Kind, e.Number), e.Type, e.Outcome)
	if e.URL != "" {
		line += "  " + e.URL
	}
	return line + "\n"
}

func writeEventLines(w io.Writer, events []historyEvent) error {
	for _, e := range events {
		if err := writeJSONLine(w, e); err != nil {
			return err
		}
	}
	return nil
}

func writeJSONLine(w io.Writer, v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(append(line, '\n'))
	return err
}

func countNoun(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	if strings.HasSuffix(noun, "entry") {
		return fmt.Sprintf("%d %sentries", n, strings.TrimSuffix(noun, "entry"))
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func runHistoryPath(ctx context.Context, env historyEnv, format string, out io.Writer) error {
	if err := checkHistoryFormat(format); err != nil {
		return err
	}
	repo, err := historyRepository(ctx, env.WorkDir, env.Git)
	if err != nil {
		return err
	}
	cacheDir, err := historyCacheDir(env)
	if err != nil {
		return err
	}
	path := historyPath(cacheDir, repo.Key)
	exists := false
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
		exists = true
	}
	events, _, readErr := readHistory(path)

	if format == historyFormatJSONL {
		if err := writeJSONLine(out, struct {
			Path   string `json:"path"`
			Key    string `json:"key"`
			Exists bool   `json:"exists"`
			Events int    `json:"events"`
		}{path, repo.Key, exists, len(events)}); err != nil {
			return err
		}
		return readErr
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", path)
	fmt.Fprintf(&b, "Key: %s\n", repo.Key)
	switch {
	case readErr != nil:
	case !exists:
		b.WriteString("No events yet: the file does not exist.\n")
	default:
		fmt.Fprintf(&b, "Events: %d\n", len(events))
	}
	if _, err := io.WriteString(out, b.String()); err != nil {
		return err
	}
	return readErr
}
