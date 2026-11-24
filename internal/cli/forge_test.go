package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func forgeYAML(provider, project string) string {
	return strings.Replace(validConfigYAML, "schema_version: 2", "schema_version: 3", 1) + "  forge:\n    provider: " + provider + "\n    project: " + project + "\n"
}

func TestForgeConfigurationContract(t *testing.T) {
	for name, content := range map[string]string{
		"legacy":             validConfigYAML,
		"v3 implicit GitHub": strings.Replace(validConfigYAML, "schema_version: 2", "schema_version: 3", 1),
		"GitHub explicit":    forgeYAML("github", "o/r"),
		"GitLab subgroups":   forgeYAML("gitlab", "group/subgroup/project"),
		"GitHub dot repo":    forgeYAML("github", "o/.github"),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateConfig(decodeOrFail(t, content)); err != nil {
				t.Fatal(err)
			}
		})
	}
	for name, content := range map[string]string{
		"v2 forge":              validConfigYAML + "  forge:\n    provider: github\n    project: o/r\n",
		"missing provider":      forgeYAML("''", "o/r"),
		"unknown provider":      forgeYAML("other", "o/r"),
		"missing project":       forgeYAML("gitlab", "''"),
		"extra GitHub path":     forgeYAML("github", "g/s/p"),
		"path traversal":        forgeYAML("gitlab", "g/../p"),
		"git suffix":            forgeYAML("gitlab", "g/p.git"),
		"credentials":           forgeYAML("gitlab", "g/p") + "    endpoint: https://user:secret@gitlab.com\n",
		"insecure endpoint":     forgeYAML("gitlab", "g/p") + "    endpoint: http://gitlab.com\n",
		"endpoint path":         forgeYAML("gitlab", "g/p") + "    endpoint: https://gitlab.com/path\n",
		"wrong public provider": forgeYAML("gitlab", "g/p") + "    endpoint: https://github.com\n",
		"endpoint fragment":     forgeYAML("gitlab", "g/p") + "    endpoint: https://gitlab.com/#secret\n",
		"API on GitHub":         forgeYAML("github", "o/r") + "    api_base_url: https://github.com/api/v4\n",
		"API insecure":          forgeYAML("gitlab", "g/p") + "    api_base_url: http://gitlab.com/api/v4\n",
		"API wrong host":        forgeYAML("gitlab", "g/p") + "    api_base_url: https://other.test/api/v4\n",
		"API wrong version":     forgeYAML("gitlab", "g/p") + "    api_base_url: https://gitlab.com/api/v3\n",
		"API query":             forgeYAML("gitlab", "g/p") + "    api_base_url: https://gitlab.com/api/v4?token=secret\n",
	} {
		t.Run(name, func(t *testing.T) {
			err := validateConfig(decodeOrFail(t, content))
			if err == nil {
				t.Fatal("invalid forge accepted")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("secret disclosed")
			}
		})
	}
	f := effectiveForge(forgeConfig{Provider: "gitlab", Project: "g/p"})
	if f.Endpoint != "https://gitlab.com" || f.APIBaseURL != "https://gitlab.com/api/v4" || f.AllowCrossProjectTargets {
		t.Fatalf("wrong defaults: %+v", f)
	}
}

func forgeRemoteGit(remote string) gitRunner {
	return func(context.Context, string, ...string) ([]byte, error) { return []byte(remote), nil }
}

func TestExplicitForgeOriginIdentity(t *testing.T) {
	cfg := decodeOrFail(t, forgeYAML("gitlab", "Group/Sub/Project"))
	cfg.Project.Forge.Endpoint = "https://gitlab.example.test"
	for _, remote := range []string{"https://gitlab.example.test/Group/Sub/Project.git", "https://gitlab.example.test:443/Group/Sub/Project.git", "git@gitlab.example.test:Group/Sub/Project.git", "ssh://git@gitlab.example.test:2222/Group/Sub/Project.git"} {
		if err := checkForgeOrigin(context.Background(), "unused", forgeRemoteGit(remote), cfg); err != nil {
			t.Errorf("%s: %v", remote, err)
		}
	}
	for _, remote := range []string{"https://gitlab.example.test/group/sub/project.git", "git@other.test:Group/Sub/Project.git", "git@gitlab.example.test:Group/Else/Project.git", "https://gitlab.example.test:8443/Group/Sub/Project.git", "file:///Group/Sub/Project.git"} {
		if err := checkForgeOrigin(context.Background(), "unused", forgeRemoteGit(remote), cfg); err == nil {
			t.Errorf("mismatched origin accepted: %s", remote)
		}
	}
	github := decodeOrFail(t, forgeYAML("github", "O/R"))
	if err := checkForgeOrigin(context.Background(), "unused", forgeRemoteGit("git@github.com:o/r.git"), github); err != nil {
		t.Fatal(err)
	}
}

func initGitRemote(t *testing.T, dir, remote string) {
	t.Helper()
	gitIn(t, dir, "init")
	gitIn(t, dir, "remote", "add", "origin", remote)
}

func TestForgeInitValidateAndConfigRoundTrip(t *testing.T) {
	dir := newProjectDir(t)
	initGitRemote(t, dir, "git@gitlab.example.test:group/sub/project.git")
	args := initArgs("--forge-provider", "gitlab", "--forge-project", "group/sub/project", "--forge-endpoint", "https://gitlab.example.test")
	if out, _, err := runCLI(t, "dev", args...); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	path := configPathIn(dir)
	data := readFile(t, path)
	if !strings.Contains(data, "schema_version: 3") || !strings.Contains(data, "provider: gitlab") {
		t.Fatal(data)
	}
	writeFile(t, path, strings.Replace(data, "  forge:", "  # Preserve forge identity.\n  forge:", 1))
	for _, command := range [][]string{
		{"config", "validate"}, {"config", "set", "integration-branch", "integration", "--yes"},
		{"config", "set", "forge-allow-cross-project-targets", "true", "--yes"},
	} {
		if out, _, err := runCLI(t, "dev", command...); err != nil {
			t.Fatalf("%v: %v\n%s", command, err, out)
		}
	}
	before := readFile(t, path)
	if _, _, err := runCLI(t, "dev", "init"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != before || !strings.Contains(got, "# Preserve forge identity.") {
		t.Fatal("repeat init lost data/comments or changed the file")
	}
	cfg := assertValidFile(t, path)
	if cfg.Project.Forge.Project != "group/sub/project" || !cfg.Project.Forge.AllowCrossProjectTargets {
		t.Fatal("configuration edit lost forge")
	}
	gitIn(t, dir, "remote", "set-url", "origin", "git@other.test:group/sub/project.git")
	if _, _, err := runCLI(t, "dev", "config", "validate"); err == nil {
		t.Fatal("validate accepted mismatched origin")
	}
	if _, _, err := runCLI(t, "dev", "init"); err == nil {
		t.Fatal("init accepted mismatched origin")
	}
	if readFile(t, path) != before {
		t.Fatal("failed identity validation changed configuration")
	}
}

func TestForgeInitRefusesMismatchWithoutWriting(t *testing.T) {
	dir := newProjectDir(t)
	initGitRemote(t, dir, "git@github.com:o/r.git")
	if _, _, err := runCLI(t, "dev", initArgs("--forge-provider", "gitlab", "--forge-project", "g/p")...); err == nil {
		t.Fatal("mismatch accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, ".garage")); !os.IsNotExist(err) {
		t.Fatal("failed init wrote files")
	}
}

func TestForgeV2UpgradePreservesChoicesAndComments(t *testing.T) {
	dir := newProjectDir(t)
	initGitRemote(t, dir, "git@github.com:o/r.git")
	path := configPathIn(dir)
	writeFile(t, path, "# Legacy choices.\n"+validConfigYAML)
	if _, _, err := runCLI(t, "dev", "config", "validate"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readFile(t, path), "schema_version: 2") {
		t.Fatal("validation migrated v2")
	}
	if _, _, err := runCLI(t, "dev", "init", "--forge-provider", "github", "--forge-project", "o/r"); err != nil {
		t.Fatal(err)
	}
	after := readFile(t, path)
	if !strings.Contains(after, "# Legacy choices.") || !strings.Contains(after, "schema_version: 3") {
		t.Fatal(after)
	}
	if cfg := assertValidFile(t, path); cfg.Project.Forge == nil || cfg.Project.Discovery.Mode != discoveryJudgment {
		t.Fatal("lost legacy choices")
	}
}

func TestWorkflowTargetGrammar(t *testing.T) {
	for _, c := range []struct {
		args                []string
		kind, host, project string
	}{
		{[]string{"48"}, "", "", ""}, {[]string{"#48"}, "", "", ""},
		{[]string{"issue", "48"}, workflowIssueKind, "", ""}, {[]string{"pr", "48"}, workflowChangeKind, "", ""},
		{[]string{"https://github.com/o/r/issues/48"}, workflowIssueKind, "github.com", "o/r"},
		{[]string{"https://github.example.test/o/r/pull/48"}, workflowChangeKind, "github.example.test", "o/r"},
	} {
		got, err := parseWorkflowTarget(c.args)
		if err != nil || got.Number != 48 || got.Kind != c.kind || got.Host != c.host || got.Project != c.project {
			t.Errorf("%v: %+v, %v", c.args, got, err)
		}
	}
	for _, args := range [][]string{{"mr", "48"}, {"issue", "#48"}, {"pr", "0"}, {"1", "2"}, {"https://github.com/o/r/pull/048"}, {"http://github.com/o/r/pull/48"}, {"https://user:secret@github.com/o/r/pull/48"}, {"https://github.com/o/r/pull/48?token=secret"}, {"https://github.com/o/r/issues/48/extra"}, {"https://github.com/o%2fr/pull/48"}} {
		if _, err := parseWorkflowTarget(args); err == nil {
			t.Errorf("accepted %v", args)
		} else if strings.Contains(err.Error(), "secret") {
			t.Fatal("secret disclosed")
		}
	}
}

func TestProviderDispatchFailsClosedAndPreservesGitHub(t *testing.T) {
	for _, scenario := range []string{"legacy", "explicit", "wrong kind", "wrong host", "cross-project refused", "cross-project enabled", "mismatched origin", "gitlab unavailable", "invalid forge"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			initGitRemote(t, dir, "git@github.com:o/r.git")
			cfg := forgeYAML("github", "o/r")
			target := workflowTarget{Number: 46, Kind: workflowChangeKind}
			wantCalls, wantError := true, false
			switch scenario {
			case "legacy":
				cfg = validConfigYAML
				target.Kind = ""
			case "wrong kind":
				target.Kind = workflowIssueKind
				wantError = true
			case "wrong host":
				target.Host = "other.test"
				wantCalls, wantError = false, true
			case "cross-project refused":
				target.Project = "o/other"
				wantCalls, wantError = false, true
			case "cross-project enabled":
				cfg += "    allow_cross_project_targets: true\n"
				target.Project = "o/other"
			case "mismatched origin":
				cfg = forgeYAML("github", "o/other")
				wantCalls, wantError = false, true
			case "gitlab unavailable":
				cfg = forgeYAML("gitlab", "g/p")
				gitIn(t, dir, "remote", "set-url", "origin", "git@gitlab.com:g/p.git")
				wantCalls, wantError = false, true
			case "invalid forge":
				cfg += "    endpoint: http://github.com\n"
				wantCalls, wantError = false, true
			}
			writeFile(t, configPathIn(dir), cfg)
			calls := 0
			gh := func(ctx context.Context, args ...string) ([]byte, []byte, error) {
				calls++
				return fixtureGH(t, fixtureBytes(t, "pr46"))(ctx, args...)
			}
			env := statusTestEnv(t, dir, gh)
			report, err := statusReportForTarget(context.Background(), target, env)
			if (err != nil) != wantError {
				t.Fatalf("error = %v, want error %t", err, wantError)
			}
			if (calls > 0) != wantCalls {
				t.Fatalf("provider calls = %d, want consulted %t", calls, wantCalls)
			}
			if err == nil {
				var out bytes.Buffer
				if err := report.render(&out); err != nil {
					t.Fatal(err)
				}
				assertContains(t, out.String(), "Status of pull request #46")
			}
		})
	}
}

func TestStatusRequiresSafelyReadableExistingConfiguration(t *testing.T) {
	fixture := fixtureBytes(t, "pr46")
	for _, scenario := range []string{"absent directory", "absent file", "legacy valid", "legacy invalid values", "symlink", "broken symlink", "directory", "non-directory parent", "symlink parent", "broken symlink parent", "unreadable file"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			refused := scenario == "symlink" || scenario == "broken symlink" || scenario == "directory" || scenario == "non-directory parent" || scenario == "symlink parent" || scenario == "broken symlink parent" || scenario == "unreadable file"
			origin := "git@github.com:o/r.git"
			if refused {
				origin = "git@gitlab.com:g/p.git"
			}
			initGitRemote(t, dir, origin)
			path := configPathIn(dir)
			switch scenario {
			case "absent file":
				if err := os.Mkdir(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
			case "legacy valid":
				writeFile(t, path, validConfigYAML)
			case "legacy invalid values":
				writeFile(t, path, "schema_version: 2\n")
			case "symlink", "broken symlink":
				if err := os.Mkdir(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(dir, "forge-config.yaml")
				if scenario == "symlink" {
					writeFile(t, target, forgeYAML("gitlab", "g/p"))
				}
				symlinkOrSkip(t, target, path)
			case "directory":
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatal(err)
				}
			case "non-directory parent":
				writeFile(t, filepath.Dir(path), "not a directory")
			case "symlink parent", "broken symlink parent":
				target := filepath.Join(dir, "external-config-directory")
				if scenario == "symlink parent" {
					writeFile(t, filepath.Join(target, configFileName), forgeYAML("gitlab", "g/p"))
				}
				symlinkOrSkip(t, target, filepath.Dir(path))
			case "unreadable file":
				writeFile(t, path, forgeYAML("gitlab", "g/p"))
				if err := os.Chmod(path, 0); err != nil {
					t.Skip(err)
				}
				t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
				if _, err := os.ReadFile(path); err == nil {
					t.Skip("file permissions do not deny reads on this platform/account")
				}
			}
			calls := 0
			env := statusTestEnv(t, dir, func(ctx context.Context, args ...string) ([]byte, []byte, error) {
				calls++
				return fixtureGH(t, fixture)(ctx, args...)
			})
			cache := t.TempDir()
			env.CacheDir = func() (string, error) {
				if refused {
					t.Error("history read after refusing configuration")
				}
				return cache, nil
			}
			report, err := statusReportFor(context.Background(), 46, env)
			if refused {
				if err == nil || report != nil || calls != 0 {
					t.Fatalf("unsafe config: error=%v, report=%v, provider calls=%d", err, report, calls)
				}
				if !strings.Contains(err.Error(), "cannot select a forge") {
					t.Fatal(err)
				}
			} else if err != nil || report == nil || calls == 0 {
				t.Fatalf("legacy discovery changed: error=%v, report=%v, provider calls=%d", err, report, calls)
			}
		})
	}
}

func TestStatusHistoryRemainsWithOriginProject(t *testing.T) {
	fixture := fixtureBytes(t, "pr46")
	for _, scenario := range []string{"same success", "same unavailable", "foreign success", "foreign unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			initGitRemote(t, dir, "git@github.com:o/r.git")
			writeFile(t, configPathIn(dir), forgeYAML("github", "o/r")+"    allow_cross_project_targets: true\n")
			cache := t.TempDir()
			cacheDir := func() (string, error) { return cache, nil }
			originURL := "https://github.com/o/r/pull/46#issuecomment-local-origin"
			if _, err := recordEvent(context.Background(), recordOptions{PR: 46, Type: "review", Outcome: "approved", URL: originURL}, recordEnv{WorkDir: dir, CacheDir: cacheDir, Git: runGit, Now: time.Now}); err != nil {
				t.Fatal(err)
			}
			foreign := strings.HasPrefix(scenario, "foreign")
			unavailable := strings.HasSuffix(scenario, "unavailable")
			project := "o/r"
			if foreign {
				project = "o/other"
			}
			target, err := parseWorkflowTarget([]string{"https://github.com/" + project + "/pull/46"})
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			env := statusTestEnv(t, dir, func(ctx context.Context, args ...string) ([]byte, []byte, error) {
				calls++
				if foreign && !strings.Contains(strings.Join(args, " "), "name=other") {
					t.Error("foreign target did not select the foreign repository")
				}
				if unavailable {
					return nil, []byte("offline"), errors.New("offline")
				}
				response := bytes.ReplaceAll(fixture, []byte("https://github.com/o/r/"), []byte("https://github.com/"+project+"/"))
				return fixtureGH(t, response)(ctx, args...)
			})
			cacheReads := 0
			env.CacheDir = func() (string, error) { cacheReads++; return cache, nil }
			report, err := statusReportForTarget(context.Background(), target, env)
			if unavailable {
				if !errors.Is(err, errGitHubNotConsulted) {
					t.Fatalf("error=%v, want unavailable", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if report == nil || calls == 0 {
				t.Fatal("expected provider result/report")
			}
			var out bytes.Buffer
			if err := report.render(&out); err != nil {
				t.Fatal(err)
			}
			if foreign {
				if cacheReads != 0 {
					t.Errorf("read origin history %d times for a foreign target", cacheReads)
				}
				assertNotContains(t, out.String(), originURL, "Local history")
			} else {
				if cacheReads != 1 {
					t.Errorf("same-project cache reads=%d, want 1", cacheReads)
				}
				assertContains(t, out.String(), originURL, "Local history", "local, not evidence")
			}
		})
	}
}
