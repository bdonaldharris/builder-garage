package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// statusEnv holds the external dependencies of "garage status", so tests can
// substitute them.
type statusEnv struct {
	WorkDir  string
	CacheDir func() (string, error)
	Git      gitRunner
	GH       ghRunner
	GLab     glabRunner
	Now      func() time.Time
}

func defaultStatusEnv() (statusEnv, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return statusEnv{}, fmt.Errorf("cannot determine the current directory: %w", err)
	}
	return statusEnv{
		WorkDir:  workDir,
		CacheDir: os.UserCacheDir,
		Git:      runGitNoLazyFetch,
		GH:       runGH,
		GLab:     runGLab,
		Now:      time.Now,
	}, nil
}

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status [NUMBER | issue NUMBER | pr NUMBER | mr IID | HTTPS-URL]",
		Short: "Show authoritative issue or change workflow facts from the configured forge",
		Long: `Show recorded workflow facts for an issue or change in this project's forge.

GitHub retains NUMBER and #NUMBER, issue NUMBER, pr NUMBER, and canonical
HTTPS /OWNER/REPO/issues/NUMBER or /OWNER/REPO/pull/NUMBER URLs. GitLab requires
issue IID, mr IID, or a canonical HTTPS /GROUP/PROJECT/-/issues/IID or
/GROUP/PROJECT/-/merge_requests/IID URL. Bare GitLab numbers and #IID are
ambiguous and refused. Markdown uses #IID for issues and !IID for MRs;
qualified subgroup/project references require cross-project targets enabled.

Schema-v3 project.forge selects provider, host and project; origin must match.
Legacy schema-v2 GitHub projects need no migration. URLs stay on the configured
host; another project requires allow_cross_project_targets. GitLab never falls
back to GitHub, and foreign targets never use origin-project history.

GitHub uses authenticated gh with one read-only GraphQL query plus at most one
follow-up query. GitLab uses the selected host's authenticated glab REST-v4
identity and explicit GET/resource allowlists: at most 64 calls, three pages of
100 items per collection, 20 seconds per call and 60 seconds per observation.
Incomplete, inaccessible, confidential, unsupported and unavailable facts are
reported explicitly. Unavailable required evidence never passes a gate.

Every fact names its provider and checked/asserted basis. GitHub record
eligibility remains OWNER/MEMBER/COLLABORATOR; GitLab workflow records require
checked Developer-or-higher project membership. Native MR approvals are not
Agent #2 reviews or proof of a qualified human change acceptor. Qualified
GitLab authority/write support and complete compatibility certification belong
to later feature-stack issues. This observation slice is not a public rollout
of complete GitLab support.

Status writes nothing to either forge, stores no credentials or TLS exceptions,
and never retries, polls, assigns or advances an agent. Local history is never
authoritative evidence. Missing authentication or inaccessible resources never
select another provider. The existing pull-request kind, issue review, artifact
equivalence and separate human acceptance gates remain in force.`,
		Example: `  garage status 48
  garage status #33
  garage status issue 33
  garage status pr 48
  garage status https://github.com/owner/repo/pull/48
  garage status mr 48`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := workflowTarget{Args: args}
			env, err := defaultStatusEnv()
			if err != nil {
				return err
			}
			report, err := statusReportForTarget(cmd.Context(), target, env)
			if report != nil {
				if renderErr := report.render(cmd.OutOrStdout()); renderErr != nil {
					return renderErr
				}
			}
			return err
		},
	}
}

func parseStatusTarget(arg string) (int, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(arg, "#"))
	if err != nil || n < 1 || n > maxTargetNumber {
		return 0, fmt.Errorf("%q is not an issue or pull request number", arg)
	}
	return n, nil
}

// runStatus builds and prints the status of one issue or pull request. When
// GitHub cannot be read, it prints what it can, reports no workflow state, and
// returns errGitHubNotConsulted.
func runStatus(ctx context.Context, number int, env statusEnv, w io.Writer) error {
	report, err := statusReportFor(ctx, number, env)
	if report != nil {
		if renderErr := report.render(w); renderErr != nil {
			return renderErr
		}
	}
	return err
}

func mergeStatusReferenceIssues(item *workflowItem, issues []workflowIssue, governing map[int]bool) {
	for _, issue := range issues {
		if governing[issue.Number] {
			item.BodyIssues = append(item.BodyIssues, issue)
		}
	}
}

// statusReportFor is the shared typed status operation. Command and web
// adapters render its report independently; it never parses rendered output.
func statusReportFor(ctx context.Context, number int, env statusEnv) (*statusReport, error) {
	return statusReportForTarget(ctx, workflowTarget{Number: number}, env)
}

func statusReportForTarget(ctx context.Context, target workflowTarget, env statusEnv) (*statusReport, error) {
	number := target.Number
	if ctx == nil {
		ctx = context.Background()
	}
	repo, err := identifyRepository(ctx, env.WorkDir, env.Git)
	if err != nil {
		return nil, err
	}

	var cfg *projectConfig
	configProblem := ""
	// Read once so provider selection uses the exact configuration inspected.
	// Only genuine absence permits legacy discovery. A refused/unreadable
	// existing file cannot establish that explicit forge identity is absent.
	data, readErr := readConfigFile(configPathInRepository(repo.TopLevel))
	if readErr != nil {
		configProblem = oneLine(readErr.Error())
		if !errors.Is(readErr, errNoConfig) {
			return nil, fmt.Errorf("cannot select a forge: %w", readErr)
		}
	} else {
		loaded, decodeErr := decodeConfig(data)
		if decodeErr != nil {
			return nil, fmt.Errorf("cannot select a forge: %w", decodeErr)
		}
		if problems := validateConfig(loaded); problems != nil {
			configProblem = oneLine(problems.Error())
			if proj(&loaded).Forge != nil {
				return nil, fmt.Errorf("cannot select a forge: %s", configProblem)
			}
		} else {
			cfg = &loaded
		}
	}
	if len(target.Args) > 0 {
		var parseErr error
		if cfg != nil && proj(cfg).Forge != nil && proj(cfg).Forge.Provider == "gitlab" {
			target, parseErr = parseGitlabTarget(target.Args, *proj(cfg).Forge)
		} else {
			target, parseErr = parseWorkflowTarget(target.Args)
		}
		if parseErr != nil {
			return nil, parseErr
		}
		number = target.Number
	}
	if cfg != nil {
		if err := checkForgeOrigin(ctx, repo.TopLevel, env.Git, *cfg); err != nil {
			return nil, err
		}
		if f := proj(cfg).Forge; f != nil && f.Provider == "gitlab" {
			return gitlabStatusReportFor(ctx, target, cfg, env, repo)
		}
	}
	ghRepo, err := githubRepository(ctx, repo.TopLevel, env.Git)
	var item *workflowItem
	reads := 1
	includeOriginHistory := target.Project == ""
	if err == nil {
		if cfg != nil && cfg.Project.Forge != nil {
			f := effectiveForge(*cfg.Project.Forge)
			u, _ := url.Parse(f.Endpoint)
			parts := strings.Split(f.Project, "/")
			ghRepo = ghRepository{Host: u.Host, Owner: parts[0], Name: parts[1]}
		}
		if target.Host != "" && !strings.EqualFold(canonicalHTTPSHost(target.Host), canonicalHTTPSHost(ghRepo.Host)) {
			return nil, errors.New("target URL does not match the configured forge host")
		}
		includeOriginHistory = target.Project == "" || strings.EqualFold(target.Project, ghRepo.Owner+"/"+ghRepo.Name)
		if !includeOriginHistory {
			if cfg == nil || proj(cfg).Forge == nil || !cfg.Project.Forge.AllowCrossProjectTargets {
				return nil, errors.New("cross-project targets are not enabled")
			}
			parts := strings.Split(target.Project, "/")
			ghRepo.Owner, ghRepo.Name = parts[0], parts[1]
		}
		var provider statusProvider = githubStatusAdapter{Runner: env.GH, Repository: ghRepo}
		item, reads, err = provider.Read(ctx, target, cfg)
		if err == nil && target.Kind != "" && target.Kind != item.Kind {
			return nil, errors.New("the target kind does not match the authoritative GitHub resource")
		}
	}
	if errors.Is(err, errStatusNotFound) {
		return nil, fmt.Errorf("%s/%s #%d: %w", ghRepo.Owner, ghRepo.Name, number, errStatusNotFound)
	}

	var report *statusReport
	if err != nil {
		report = &statusReport{header: []string{
			fmt.Sprintf("Status of #%d", number),
			"",
			"GITHUB WAS NOT CONSULTED: " + err.Error() + ".",
			"No workflow state and no eligible action are reported. GitHub records are the only",
			"authority for them, and local history never stands in for GitHub.",
		}}
		addConfigSection(report, statusInputs{Item: &workflowItem{}, Config: cfg, ConfigProblem: configProblem})
	} else {
		report = buildStatusReport(statusInputs{
			Item:          item,
			Config:        cfg,
			ConfigProblem: configProblem,
			Recompute: func(ref, commit string) (string, string) {
				return recomputeCommitChangeset(ctx, env.Git, repo.TopLevel, ref, commit)
			},
			ReadAt: env.Now(),
			Reads:  reads,
		})
	}

	kind := "issue"
	if item != nil && item.isPullRequest() {
		kind = "pr"
	}
	// The cache belongs to origin, not every project with the same number.
	// Omit it for a foreign target even if that provider read was unavailable.
	if includeOriginHistory {
		addLocalHistory(report, env, repo, kind, number, item == nil)
	}

	if item == nil {
		return report, errGitHubNotConsulted
	}
	return report, nil
}

// addLocalHistory appends local events for the target. When GitHub was not
// read, the target's kind is unknown, so events of both kinds are shown.
func addLocalHistory(r *statusReport, env statusEnv, repo repoIdentity, kind string, number int, bothKinds bool) {
	cacheDir, err := env.CacheDir()
	if err != nil {
		addLocalSection(r, kind, number, nil, 0, "the user cache directory cannot be located")
		return
	}
	events, discarded, err := readHistory(historyPath(cacheDir, repo.Key))
	if err != nil {
		addLocalSection(r, kind, number, nil, 0, oneLine(err.Error()))
		return
	}
	if bothKinds {
		addLocalSection(r, "issue", number, events, discarded, "")
		addLocalSection(r, "pr", number, events, 0, "")
		return
	}
	addLocalSection(r, kind, number, events, discarded, "")
}

// runGitNoLazyFetch runs git like runGit, with lazy fetching from a promisor
// remote disabled. Git 2.44 and later honor GIT_NO_LAZY_FETCH; earlier
// versions ignore it, so recomputeCommitChangeset also declines to run in a
// partial clone.
func runGitNoLazyFetch(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_NO_LAZY_FETCH=1")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.Bytes(), err
}

// partialCloneReason reports why objects cannot be guaranteed local in dir:
// in a partial clone, git fetches a missing object from the promisor remote
// on demand, so even a presence probe could fetch. It reads configuration
// only, never objects, and returns "" for an ordinary clone.
func partialCloneReason(ctx context.Context, git gitRunner, dir string) string {
	const cannot = "this is a partial clone, where git fetches missing objects on demand, and status never fetches"
	out, err := git(ctx, dir, "config", "--get-regexp", `^remote\..+\.promisor$|^extensions\.partialclone$`)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "" // no such keys: not a partial clone
		}
		return "whether this is a partial clone cannot be determined, so local recomputation cannot be guaranteed not to fetch"
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		key, value, _ := strings.Cut(strings.TrimSpace(line), " ")
		value = strings.ToLower(strings.TrimSpace(value))
		if strings.HasSuffix(key, ".promisor") && value != "false" && value != "no" && value != "off" && value != "0" {
			return cannot
		}
		if key == "extensions.partialclone" && value != "" {
			return cannot
		}
	}
	return ""
}

// recomputeCommitChangeset recomputes the garage-artifact-v1 changeset of a
// committed artifact from local objects only: the SHA-256 of the raw,
// NUL-separated change listing between the merge base and the commit's tree.
// It never fetches: in a partial clone it does not run at all, because any
// object read there could fetch. When it does not recompute, it returns ""
// and the reason.
func recomputeCommitChangeset(ctx context.Context, git gitRunner, dir, integrationRef, commit string) (string, string) {
	if !objectNamePattern.MatchString(integrationRef) || !objectNamePattern.MatchString(commit) {
		return "", "the recorded object names are not full object names"
	}
	ctx, cancel := context.WithTimeout(ctx, 2*gitTimeout)
	defer cancel()
	if reason := partialCloneReason(ctx, git, dir); reason != "" {
		return "", reason
	}
	for _, object := range []string{commit, integrationRef} {
		if _, err := git(ctx, dir, "cat-file", "-e", object+"^{commit}"); err != nil {
			return "", "commit " + shortSHA(object) + " is not present locally"
		}
	}
	base, err := git(ctx, dir, "merge-base", integrationRef, commit)
	if err != nil {
		return "", "the merge base cannot be determined locally"
	}
	tree, err := git(ctx, dir, "rev-parse", "--verify", commit+"^{tree}")
	if err != nil {
		return "", "the commit's tree cannot be read locally"
	}
	raw, err := git(ctx, dir, "diff-tree", "-r", "-z", "--raw", "--no-abbrev", "--no-renames",
		strings.TrimSpace(string(base)), strings.TrimSpace(string(tree)))
	if err != nil {
		return "", "the change listing cannot be computed locally"
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), ""
}

func configPathInRepository(dir string) string {
	return filepath.Join(dir, garageDirName, configFileName)
}
