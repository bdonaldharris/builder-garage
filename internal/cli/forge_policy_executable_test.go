package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func TestForgePolicyExecutableIsReadOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture; native policy contracts have platform-neutral tests")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python unavailable for offline executable fixture")
	}
	fixtureCtx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	base := policyFixture(t, nil)
	responses := map[string]string{}
	capture := &gitlabFixtureRunner{t: t, override: func(endpoint string, _ int) ([]byte, bool) {
		data, _, err := base.run(fixtureCtx, "api", "--hostname", "gitlab.example", "--method", "GET", "--include", endpoint)
		if err != nil {
			t.Fatal(err)
		}
		responses[strings.SplitN(endpoint, "?", 2)[0]] = string(data)
		return data, true
	}}
	readPolicyFixture(t, capture)
	// The real command uses the wall clock. Keep native fixture approvals after
	// their diff and before now, instead of depending on the test machine's date.
	for key, value := range responses {
		value = strings.ReplaceAll(value, "2026-11-24T12:00:00Z", time.Now().UTC().Add(-2*time.Hour).Format(time.RFC3339))
		responses[key] = strings.ReplaceAll(value, "2026-11-24T12:30:00Z", time.Now().UTC().Add(-time.Hour).Format(time.RFC3339))
	}
	dir := newGitRepo(t, "git@gitlab.example:group/sub/project.git")
	cfg := mustDecodeGitlabConfig(t)
	cfg.Project.Authorities.ChangeAcceptor.Members = []string{"gitlab:gitlab.example/maintainer"}
	config, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, configPathIn(dir), string(config))
	bin := t.TempDir()
	payload, err := json.Marshal(responses)
	if err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(bin, "responses.json")
	writeFile(t, dataPath, string(payload))
	t.Setenv("GARAGE_POLICY_RESPONSES", dataPath)
	script := `import json,os,sys
args=sys.argv[1:]
assert args[:6]==['api','--hostname','gitlab.example','--method','GET','--include']
assert len(args)==7
path=args[6].split('?',1)[0]
with open(os.environ['GARAGE_POLICY_RESPONSES']) as f: data=json.load(f)
sys.stdout.write(data[path])
`
	if err := os.WriteFile(filepath.Join(bin, "glab"), []byte("#!"+python+"\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	before := gitIn(t, dir, "status", "--porcelain")
	out, stderr, err := runCLI(t, "dev", "forge", "policy", "mr", "5")
	if err != nil || stderr != "" {
		t.Fatalf("policy command: %v %s %s", err, stderr, out)
	}
	var result struct {
		Head  string               `json:"head"`
		Gates []workflowPolicyGate `json:"gates"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result.Head != strings.Repeat("a", 40) || len(result.Gates) < 7 {
		t.Fatalf("missing normalized policy evidence: %s", out)
	}
	for _, g := range result.Gates {
		if g.Required && (g.State != policyPassed || len(g.Sources) == 0) {
			t.Fatalf("required fixture fact did not pass: %+v", g)
		}
	}
	if after := gitIn(t, dir, "status", "--porcelain"); after != before {
		t.Fatal("policy observation changed repository")
	}
	_, _, err = runCLI(t, "dev", "forge", "policy", "issue", "5")
	if err == nil {
		t.Fatal("policy inspection accepted an Issue")
	}
}

// Refuse unknown access-level semantics instead of inventing a merge grant.
func TestNativeProtectionAccessUnknown(t *testing.T) {
	level := 99
	if allowed, known := nativeAccess(glAccess{AccessLevel: &level}); allowed || known {
		t.Fatal("unknown level became an effective grant")
	}
	if !nativeRefMatches("release/*", "release/a/b") || nativeRefMatches("main", "main2") {
		t.Fatal("native wildcard semantics lost")
	}
	// Observation cancellation remains bounded; no native write runner is used.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	c := newGitlabClient(func(context.Context, ...string) ([]byte, []byte, error) {
		t.Fatal("canceled read invoked runner")
		return nil, nil, nil
	}, gitlabTestForge())
	if _, err := c.get(ctx, "GET", "projects/group%2Fsub%2Fproject/protected_branches", &[]glProtectedRef{}); err == nil {
		t.Fatal("cancellation ignored")
	}
}
