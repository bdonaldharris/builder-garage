package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// This exercises Cobra -> validated config/origin -> actual glab subprocess ->
// literal JSON stdin -> exact durable note read-back. It never uses an account.
func TestForgeExecutableNoteAndHistory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture; transport/command contracts have platform-neutral unit coverage")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable for offline executable fixture")
	}
	dir := newGitRepo(t, "git@gitlab.example:group/sub/project.git")
	writeFile(t, configPathIn(dir), forgeYAML("gitlab", "group/sub/project")+"    endpoint: https://gitlab.example\n")
	bin := t.TempDir()
	state := filepath.Join(bin, "state.json")
	bodyPath := filepath.Join(bin, "body.md")
	body := "# Discovery Result\nVerified fixture `$(touch unexpected)` and @file stay literal.\n"
	writeFile(t, bodyPath, body)
	script := `import sys,json,os
args=sys.argv[1:]
assert args[0]=='api' and args[args.index('--hostname')+1]=='gitlab.example'
method=args[args.index('--method')+1]
path=args[args.index('--include')+1]
actor={'id':1,'username':'builder'}
resource={'id':60,'iid':6,'project_id':7,'web_url':'https://gitlab.example/group/sub/project/-/issues/6','title':'Issue','description':'Requirements'}
status=200
if path=='projects/group%2Fsub%2Fproject': data={'id':7,'path_with_namespace':'group/sub/project'}
elif path=='user': data=actor
elif '/members/all/1' in path: data={'id':1,'access_level':30}
elif path.endswith('/issues/6'): data=resource
elif path.endswith('/notes') and method=='POST':
 assert args[-2:]==['--input','-']
 request=json.load(sys.stdin)
 data={'id':21,'body':request['body'],'author':actor,'project_id':7,'noteable_id':60,'noteable_iid':6,'noteable_type':'Issue','created_at':'2026-11-24T16:00:00Z','system':False,'resolvable':False}
 with open(os.environ['GARAGE_FIXTURE_STATE'],'w') as f: json.dump(data,f)
 status=201
elif path.endswith('/notes/21'):
 with open(os.environ['GARAGE_FIXTURE_STATE']) as f: data=json.load(f)
else: raise Exception('unexpected fixture request')
print('HTTP/1.1 %d OK\r\nContent-Type: application/json\r\n\r\n%s'%(status,json.dumps(data)),end='')
`
	glab := filepath.Join(bin, "glab")
	if err := os.WriteFile(glab, []byte("#!"+python+"\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GARAGE_FIXTURE_STATE", state)
	args := []string{"forge", "note", "issue", "6", "--record", "discovery", "--body-file", bodyPath, "--actor", "gitlab:gitlab.example/builder", "--authorized"}
	out, stderr, err := runCLI(t, "dev", args...)
	if err != nil || stderr != "" {
		t.Fatalf("executable fixture: %v %s %s", err, stderr, out)
	}
	var result forgeOperationResult
	if err := json.Unmarshal([]byte(out), &result); err != nil || !result.Verified || !strings.HasSuffix(result.URL, "#note_21") {
		t.Fatalf("unverified result %s %v", out, err)
	}
	stored, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	var note operationNote
	if err := json.Unmarshal(stored, &note); err != nil || note.Body != body {
		t.Fatal("stdin body altered")
	}
	isolateUserCache(t)
	out, stderr, err = runCLI(t, "dev", "record", "--mr", "6", "--type", "finalization", "--outcome", "complete", "--url", "https://gitlab.example/group/sub/project/-/merge_requests/6#note_21")
	if err != nil || stderr != "" || !strings.Contains(out, "merge request !6") {
		t.Fatalf("MR record: %v %s %s", err, stderr, out)
	}
	out, stderr, err = runCLI(t, "dev", "history", "list", "--mr", "6")
	if err != nil || stderr != "" || !strings.Contains(out, "MR !6") {
		t.Fatalf("MR history: %v %s %s", err, stderr, out)
	}
	gitIn(t, dir, "remote", "set-url", "origin", "git@gitlab.example:other/project.git")
	_, _, err = runCLI(t, "dev", args...)
	if err == nil {
		t.Fatal("origin mismatch allowed write")
	}
}
