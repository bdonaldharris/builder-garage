package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

// Validate the adopter include's safety/contract, then actually execute its
// script with the built Garage binary on valid, invalid and missing config.
// This is an isolated local fixture, not a claim of GitLab CI Lint/runtime UAT.
func TestGitlabAdopterCITemplateAndScript(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "docs", "examples", "gitlab")
	data, err := os.ReadFile(filepath.Join(source, "garage-validation.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var template map[string]any
	if err := yaml.Unmarshal(data, &template); err != nil {
		t.Fatal(err)
	}
	if len(template) != 1 || template["workflow"] != nil {
		t.Fatal("include overrides the adopter workflow")
	}
	job := template["garage:validate"].(map[string]any)
	if job["allow_failure"] != false || job["image"] != "$GARAGE_CI_IMAGE" || job["artifacts"].(map[string]any)["when"] != "always" {
		t.Fatal("adopter validation/evidence contract lost")
	}
	if _, err := os.Stat(filepath.Join(root, ".gitlab-ci.yml")); !os.IsNotExist(err) {
		t.Fatal("production GitLab CI was introduced")
	}
	if runtime.GOOS == "windows" {
		t.Skip("adopter script requires a POSIX runner; YAML and policy contracts remain tested on Windows")
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell unavailable")
	}
	bin := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(bin, "garage"), "./cmd/garage")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture binary: %v %s", err, out)
	}
	for _, scenario := range []string{"valid", "invalid", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			if scenario != "missing" {
				body := validConfigYAML
				if scenario == "invalid" {
					body = "schema_version: 999\n"
				}
				writeFile(t, configPathIn(dir), body)
			}
			cmd := exec.CommandContext(ctx, shell, filepath.Join(source, "validate.sh"))
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "CI_PROJECT_URL=https://gitlab.example/group/project", "CI_PIPELINE_URL=https://gitlab.example/group/project/-/pipelines/30", "CI_JOB_URL=https://gitlab.example/group/project/-/jobs/301", "CI_COMMIT_SHA="+strings.Repeat("a", 40), "CI_MERGE_REQUEST_IID=5")
			out, err := cmd.CombinedOutput()
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("%s validation: %v %s", scenario, err, out)
			}
			evidence, readErr := os.ReadFile(filepath.Join(dir, "garage-ci-evidence", "context.txt"))
			if readErr != nil || !strings.Contains(string(evidence), "source=ci-job-environment-asserted") || !strings.Contains(string(evidence), "authority=none") || !strings.Contains(string(evidence), "pipelines/30") {
				t.Fatalf("provenance lost: %v %s", readErr, evidence)
			}
			if _, err := os.Stat(filepath.Join(dir, "garage-ci-evidence", "config-validation.txt")); err != nil {
				t.Fatal(err)
			}
		})
	}
}
