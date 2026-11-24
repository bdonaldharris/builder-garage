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

	"go.yaml.in/yaml/v3"
)

type glabExchange struct {
	Args   []string        `json:"args"`
	Input  json.RawMessage `json:"input,omitempty"`
	Output string          `json:"output"`
}

// replayGlab writes a fake glab that answers exactly the recorded exchanges in
// order, checks each argument list and literal stdin body, and refuses any
// request beyond the transcript.
func replayGlab(t *testing.T, python string, exchanges []glabExchange) string {
	t.Helper()
	bin := t.TempDir()
	data, err := json.Marshal(exchanges)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(bin, "transcript.json"), string(data))
	script := `import json,os,sys
here=os.path.dirname(os.path.abspath(__file__))
with open(os.path.join(here,'transcript.json')) as f: exchanges=json.load(f)
counter=os.path.join(here,'counter')
n=int(open(counter).read()) if os.path.exists(counter) else 0
assert n<len(exchanges), 'request beyond transcript'
e=exchanges[n]
assert sys.argv[1:]==e['args'], (sys.argv[1:],e['args'])
if e.get('input') is not None:
    assert json.load(sys.stdin)==e['input']
open(counter,'w').write(str(n+1))
sys.stdout.write(e['output'])
`
	if err := os.WriteFile(filepath.Join(bin, "glab"), []byte("#!"+python+"\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func recordingRunner(t *testing.T, g *fakeGitLab, exchanges *[]glabExchange) glabInputRunner {
	return func(ctx context.Context, input []byte, args ...string) ([]byte, []byte, error) {
		out, stderr, err := g.writeRunner(ctx, input, args...)
		e := glabExchange{Args: append([]string{}, args...), Output: string(out)}
		if args[4] != "GET" {
			e.Input = append(json.RawMessage{}, input...)
		}
		*exchanges = append(*exchanges, e)
		return out, stderr, err
	}
}

// This exercises Cobra -> validated config/origin -> actual glab subprocess for
// release inspection and one delegated release creation. It uses no account.
func TestForgeReleaseExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture; release contracts have platform-neutral unit coverage")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable for offline executable fixture")
	}
	dir := newGitRepo(t, "git@gitlab.example:group/sub/project.git")
	cfg := releaseTestConfig(t)
	config, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, configPathIn(dir), string(config))
	before := gitIn(t, dir, "status", "--porcelain")

	t.Run("inspect", func(t *testing.T) {
		g := newFakeGitLab(t)
		var exchanges []glabExchange
		record := recordingRunner(t, g, &exchanges)
		if _, err := inspectGitlabRelease(t.Context(), gitlabTestForge(), releaseTestTag, releaseTestRef, func(ctx context.Context, args ...string) ([]byte, []byte, error) {
			return record(ctx, nil, args...)
		}); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", replayGlab(t, python, exchanges)+string(os.PathListSeparator)+os.Getenv("PATH"))
		out, stderr, err := runCLI(t, "dev", "forge", "release", "inspect", releaseTestTag, "--ref", releaseTestRef)
		if err != nil || stderr != "" {
			t.Fatalf("inspect: %v %s %s", err, stderr, out)
		}
		var result releaseInspection
		if err := json.Unmarshal([]byte(out), &result); err != nil || len(result.Gates) != 7 || result.Commit != releaseTestRef {
			t.Fatalf("inspection output: %v %s", err, out)
		}
		for _, gate := range result.Gates {
			if gate.State != policyPassed {
				t.Fatalf("fixture fact did not pass: %+v", gate)
			}
		}
	})

	t.Run("delegated create", func(t *testing.T) {
		g := newFakeGitLab(t)
		delete(g.releases, releaseTestTag)
		notes := "Release notes with `$(touch unexpected)` and @file kept literal.\n"
		o := releaseOp(releaseOpCreate)
		o.Notes = notes
		var exchanges []glabExchange
		if _, err := executeGitlabRelease(t.Context(), cfg, o, recordingRunner(t, g, &exchanges)); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", replayGlab(t, python, exchanges)+string(os.PathListSeparator)+os.Getenv("PATH"))
		bodyPath := filepath.Join(t.TempDir(), "notes.md")
		writeFile(t, bodyPath, notes)
		args := []string{"forge", "release", "create", releaseTestTag, "--ref", releaseTestRef, "--title", o.Title, "--body-file", bodyPath, "--actor", o.Actor, "--delegation", o.Delegation, "--authorized"}
		out, stderr, err := runCLI(t, "dev", args...)
		if err != nil || stderr != "" {
			t.Fatalf("create: %v %s %s", err, stderr, out)
		}
		var result forgeOperationResult
		if err := json.Unmarshal([]byte(out), &result); err != nil || !result.Verified || !strings.HasSuffix(result.URL, "/-/releases/v1.2.0") {
			t.Fatalf("unverified result: %v %s", err, out)
		}
		// Without --delegation the command refuses before any provider call.
		withoutDelegation := append(append([]string{}, args[:len(args)-3]...), "--authorized")
		if _, _, err := runCLI(t, "dev", withoutDelegation...); err == nil || !strings.Contains(err.Error(), "--delegation") {
			t.Fatalf("release mutation without a delegation: %v", err)
		}
	})
	if after := gitIn(t, dir, "status", "--porcelain"); after != before {
		t.Fatal("release operations changed the repository")
	}
}
