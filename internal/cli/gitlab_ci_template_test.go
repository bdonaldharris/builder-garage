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

// The adopter release include publishes durable package files only. Execute
// its script against a fake curl to check refusals, immutability and the
// manifest. This is a local fixture, not GitLab runner or registry evidence.
func TestGitlabAdopterReleaseTemplateAndScript(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "docs", "examples", "gitlab")
	data, err := os.ReadFile(filepath.Join(source, "garage-release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var template map[string]any
	if err := yaml.Unmarshal(data, &template); err != nil {
		t.Fatal(err)
	}
	if len(template) != 1 || template["workflow"] != nil {
		t.Fatal("release include overrides the adopter workflow")
	}
	job := template["garage:release-publish"].(map[string]any)
	rules := job["rules"].([]any)
	if job["allow_failure"] != false || job["image"] != "$GARAGE_RELEASE_IMAGE" || job["release"] != nil || len(rules) != 2 || rules[0].(map[string]any)["if"] != `$CI_COMMIT_TAG && $CI_COMMIT_REF_PROTECTED == "true"` || rules[1].(map[string]any)["when"] != "never" {
		t.Fatalf("release include contract lost: %v", job)
	}
	if script := job["script"].([]any); len(script) != 1 || script[0] != "sh .gitlab/garage/publish.sh" {
		t.Fatal("release include runs more than the publish script")
	}
	if runtime.GOOS == "windows" {
		t.Skip("adopter script requires a POSIX runner; the include contract remains tested on Windows")
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell unavailable")
	}
	bin := t.TempDir()
	fakeCurl := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_CURL_LOG"
for arg in "$@"; do
  if [ "$arg" = --upload-file ]; then exit "${FAKE_UPLOAD_EXIT:-0}"; fi
done
printf '%s' "${FAKE_PROBE:-404}"
`
	if err := os.WriteFile(filepath.Join(bin, "curl"), []byte(fakeCurl), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		env     []string
		files   map[string]string
		ok      bool
		uploads int
		needle  string
	}{
		{"publishes every file", nil, map[string]string{"app.tar.gz": "app", "app.tar.gz.sig": "sig"}, true, 2, "file=app.tar.gz sha256="},
		{"unprotected tag", []string{"CI_COMMIT_REF_PROTECTED=false"}, map[string]string{"app": "x"}, false, 0, "unprotected"},
		{"branch pipeline", []string{"CI_COMMIT_TAG="}, map[string]string{"app": "x"}, false, 0, "tag pipeline"},
		{"plain HTTP API", []string{"CI_API_V4_URL=http://gitlab.example/api/v4"}, map[string]string{"app": "x"}, false, 0, "not HTTPS"},
		{"already published", []string{"FAKE_PROBE=200"}, map[string]string{"app": "x"}, false, 0, "never replaced"},
		{"existence unknown", []string{"FAKE_PROBE=403"}, map[string]string{"app": "x"}, false, 0, "cannot establish"},
		{"upload uncertain", []string{"FAKE_UPLOAD_EXIT=22"}, map[string]string{"app": "x"}, false, 1, "inspect the package registry"},
		{"unsafe file name", nil, map[string]string{"app 1.tar": "x"}, false, 0, "not a valid generic package file name"},
		{"no files", nil, map[string]string{}, false, 0, "no release files"},
		{"invalid package", []string{"GARAGE_RELEASE_PACKAGE=../x"}, map[string]string{"app": "x"}, false, 0, "package name"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "dist"), 0o755); err != nil {
				t.Fatal(err)
			}
			for name, body := range test.files {
				writeFile(t, filepath.Join(dir, "dist", name), body)
			}
			log := filepath.Join(t.TempDir(), "curl.log")
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, shell, filepath.Join(source, "publish.sh"))
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "FAKE_CURL_LOG="+log,
				"CI_COMMIT_TAG=v1.2.0", "CI_COMMIT_REF_PROTECTED=true", "CI_API_V4_URL=https://gitlab.example/api/v4", "CI_PROJECT_ID=7",
				"CI_JOB_TOKEN=job-token", "CI_COMMIT_SHA="+strings.Repeat("a", 40), "CI_PIPELINE_URL=https://gitlab.example/group/project/-/pipelines/30")
			cmd.Env = append(cmd.Env, test.env...)
			out, err := cmd.CombinedOutput()
			if (err == nil) != test.ok || !strings.Contains(string(out), test.needle) {
				t.Fatalf("publish: %v %s", err, out)
			}
			calls, _ := os.ReadFile(log)
			if got := strings.Count(string(calls), "--upload-file"); got != test.uploads {
				t.Fatalf("uploads %d, want %d: %s", got, test.uploads, calls)
			}
			if test.ok {
				if !strings.Contains(string(calls), "https://gitlab.example/api/v4/projects/7/packages/generic/release/v1.2.0/app.tar.gz") {
					t.Fatalf("not published to the versioned generic package: %s", calls)
				}
				manifest, err := os.ReadFile(filepath.Join(dir, "garage-release-evidence", "manifest.txt"))
				if err != nil || !strings.Contains(string(manifest), "authority=none") || !strings.Contains(string(manifest), "source=ci-job-environment-asserted") {
					t.Fatalf("manifest provenance lost: %v %s", err, manifest)
				}
			}
			if strings.Contains(string(out), "job-token") {
				t.Fatal("the job token was echoed")
			}
		})
	}
}
