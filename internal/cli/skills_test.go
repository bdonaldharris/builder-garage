package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bdonaldharris/builder-garage/skills"
)

const (
	testRevisionA = "1111111111111111111111111111111111111111"
	testRevisionB = "2222222222222222222222222222222222222222"
)

func testSkillMD(name string) string {
	return "---\nname: " + name + "\ndescription: \"Test: the " + name + " skill.\"\nmetadata:\n  garage-role: implementer\n---\n\n# " + name + "\n\nBody.\n"
}

func testSkillFS(names ...string) fstest.MapFS {
	m := fstest.MapFS{}
	for _, n := range names {
		m[n+"/SKILL.md"] = &fstest.MapFile{Data: []byte(testSkillMD(n))}
	}
	return m
}

// skillsFixture is an isolated home, user configuration directory, and
// project. Nothing in it reads the real environment.
type skillsFixture struct {
	t         *testing.T
	Home      string
	ConfigDir string
	Project   string
	Vars      map[string]string
	Versions  map[string]string
	Build     buildRevision
	Env       skillsEnv
}

func skillTargetYAML(harness, version string) string {
	rule := supportedSkillTargets[harness]
	return fmt.Sprintf(`    - harness: %s
      scope: user
      path:
        override: %s
        default: %s
      confirmed_version: %s
      confirmed_on: "2026-11-12"
`, harness, rule.Override, rule.Default, version)
}

func newSkillsFixture(t *testing.T, harnesses ...string) *skillsFixture {
	t.Helper()
	root := t.TempDir()
	f := &skillsFixture{
		t:         t,
		Home:      filepath.Join(root, "home"),
		ConfigDir: filepath.Join(root, "userconfig"),
		Project:   filepath.Join(root, "project"),
		Vars:      map[string]string{},
		Versions:  map[string]string{"claude-code": "2.1.277", "codex": "0.137.0", "copilot-cli": "1.0.93-1"},
		Build:     buildRevision{Revision: testRevisionA, Clean: true},
	}
	for _, d := range []string{f.Home, f.ConfigDir, f.Project} {
		mustMkdir(t, d)
	}
	f.writeConfig(harnesses...)
	f.Env = skillsEnv{
		WorkDir:   f.Project,
		Skills:    testSkillFS("alpha", "beta"),
		Build:     func() buildRevision { return f.Build },
		Getenv:    func(k string) string { return f.Vars[k] },
		HomeDir:   func() (string, error) { return f.Home, nil },
		ConfigDir: func() (string, error) { return f.ConfigDir, nil },
		Git: func(_ context.Context, dir string, args ...string) ([]byte, error) {
			if args[0] == "rev-parse" {
				return []byte(f.Env.WorkDir + "\n" + filepath.Join(f.Env.WorkDir, ".git") + "\n"), nil
			}
			return nil, errors.New("no remote")
		},
		Now: func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) },
		ProbeHarness: func(_ context.Context, harness string) (string, error) {
			if v, ok := f.Versions[harness]; ok {
				return v, nil
			}
			return "", errors.New("not found")
		},
		GarageVersion: "test",
	}
	return f
}

func (f *skillsFixture) writeConfig(harnesses ...string) {
	content := validConfigYAML
	if len(harnesses) > 0 {
		content += "tooling:\n  skill_targets:\n"
		for _, h := range harnesses {
			content += skillTargetYAML(h, map[string]string{"claude-code": "2.1.277", "codex": "0.137.0", "copilot-cli": "1.0.93-1"}[h])
		}
	}
	mustMkdir(f.t, filepath.Join(f.Project, garageDirName))
	if err := os.WriteFile(filepath.Join(f.Project, garageDirName, configFileName), []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *skillsFixture) root(harness string) string {
	return map[string]string{
		"claude-code": filepath.Join(f.Home, ".claude", "skills"),
		"codex":       filepath.Join(f.Home, ".codex", "skills"),
		"copilot-cli": filepath.Join(f.Home, ".copilot", "skills"),
	}[harness]
}

func (f *skillsFixture) install(opts installOptions) (string, error) {
	var out bytes.Buffer
	err := installSkills(context.Background(), f.Env, opts, &out)
	return out.String(), err
}

func (f *skillsFixture) verify(targets ...string) (string, error) {
	var out bytes.Buffer
	err := verifySkills(context.Background(), f.Env, targets, &out)
	return out.String(), err
}

func (f *skillsFixture) mustInstall(opts installOptions) string {
	f.t.Helper()
	out, err := f.install(opts)
	if err != nil {
		f.t.Fatalf("install: %v\n%s", err, out)
	}
	return out
}

func (f *skillsFixture) manifestPath() string {
	return filepath.Join(f.ConfigDir, userStateDirName, skillManifestName)
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// snapshotTree records every entry under dir, with its type and content.
func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entry := info.Mode().String()
		if d.Type().IsRegular() {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			entry += " " + string(data) + " " + info.ModTime().String()
		}
		snap[p] = entry
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func sameSnapshot(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestEmbeddedSkillsRenderWithRevision(t *testing.T) {
	set, err := loadSkillSet(skills.FS())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"application-implementation-workflow", "engineering-discovery-workflow",
		"implementation-readiness-workflow", "independent-implementation-review", "integration-pr-review"}
	if got := skillNames(set); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("embedded skills = %v, want %v", got, want)
	}
	for _, s := range set {
		source := findSkillFile(s.Files, skillEntryFile)
		before, err := parseSkillFrontmatter(source.Data)
		if err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
		if before.Metadata["garage-role"] == "" {
			t.Errorf("%s: no metadata.garage-role", s.Name)
		}
		if _, ok := before.Metadata["garage-revision"]; ok {
			t.Errorf("%s: the canonical source sets metadata.garage-revision", s.Name)
		}
		for _, revision := range []string{testRevisionA, developmentRevision(skillSetDigest(set))} {
			files, err := renderSkill(s, revision)
			if err != nil {
				t.Fatalf("%s: %v", s.Name, err)
			}
			after, err := parseSkillFrontmatter(findSkillFile(files, skillEntryFile).Data)
			if err != nil {
				t.Fatalf("%s: %v", s.Name, err)
			}
			if after.Metadata["garage-revision"] != revision || after.Metadata["garage-role"] != before.Metadata["garage-role"] ||
				after.Name != s.Name {
				t.Errorf("%s: rendered frontmatter = %+v", s.Name, after)
			}
			// Only the frontmatter line is added.
			rendered := string(findSkillFile(files, skillEntryFile).Data)
			if strings.Replace(rendered, "  garage-revision: \""+revision+"\"\n", "", 1) != string(source.Data) {
				t.Errorf("%s: rendering changed more than the added revision line", s.Name)
			}
		}
	}
}

func TestInjectSkillRevisionRefusesUnexpectedSources(t *testing.T) {
	cases := map[string]string{
		"no frontmatter":       "# skill\n",
		"unclosed":             "---\nname: x\n",
		"no metadata":          "---\nname: x\n---\nbody\n",
		"revision present":     "---\nname: x\nmetadata:\n  garage-role: reviewer\n  garage-revision: abc\n---\n",
		"flow-style metadata":  "---\nname: x\nmetadata: {garage-role: reviewer}\n---\n",
		"invalid frontmatter":  "---\nname: x: y\nmetadata:\n  garage-role: reviewer\n---\n",
		"metadata not strings": "---\nname: x\nmetadata:\n  garage-role: [a]\n---\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if out, err := injectSkillRevision([]byte(content), testRevisionA); err == nil {
				t.Errorf("expected an error, got:\n%s", out)
			}
		})
	}
	t.Run("all-digit revision stays a string", func(t *testing.T) {
		out, err := injectSkillRevision([]byte(testSkillMD("x")), testRevisionA)
		if err != nil {
			t.Fatal(err)
		}
		fm, err := parseSkillFrontmatter(out)
		if err != nil || fm.Metadata["garage-revision"] != testRevisionA {
			t.Fatalf("frontmatter = %+v, %v", fm, err)
		}
	})
}

func TestSkillDigestsAreStable(t *testing.T) {
	a := digestSkillFiles([]skillFile{{Path: "SKILL.md", Data: []byte("x")}, {Path: "ref/a.md", Data: []byte("y")}})
	b := digestSkillFiles([]skillFile{{Path: "ref/a.md", Data: []byte("y")}, {Path: "SKILL.md", Data: []byte("x")}})
	if a != b || !strings.HasPrefix(a, "sha256:") || len(a) != len("sha256:")+64 {
		t.Fatalf("digests %q and %q, want equal sha256:<64 hex>", a, b)
	}
	if c := digestSkillFiles([]skillFile{{Path: "SKILL.md", Data: []byte("x!")}, {Path: "ref/a.md", Data: []byte("y")}}); c == a {
		t.Error("a content change did not change the digest")
	}
	if c := digestSkillFiles([]skillFile{{Path: "SKILL.md", Data: []byte("x")}}); c == a {
		t.Error("a removed file did not change the digest")
	}
}

func TestResolveSkillRoot(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	rule := supportedSkillTargets["codex"]
	vars := map[string]string{}
	getenv := func(k string) string { return vars[k] }
	if got, err := resolveSkillRoot(rule, getenv, home); err != nil || got != filepath.Join(home, ".codex", "skills") {
		t.Errorf("default = %q, %v", got, err)
	}
	override := filepath.Join(t.TempDir(), "codexhome")
	vars["CODEX_HOME"] = override
	if got, err := resolveSkillRoot(rule, getenv, home); err != nil || got != filepath.Join(override, "skills") {
		t.Errorf("override = %q, %v", got, err)
	}
	vars["CODEX_HOME"] = "relative/dir"
	if _, err := resolveSkillRoot(rule, getenv, home); err == nil {
		t.Error("expected a relative override to be refused")
	}
}

func TestSkillsInstallThenVerifyIsCurrent(t *testing.T) {
	f := newSkillsFixture(t, "claude-code", "codex")
	out := f.mustInstall(installOptions{})
	for _, want := range []string{"alpha", "installed", "Manifest:"} {
		if !strings.Contains(out, want) {
			t.Errorf("install output lacks %q:\n%s", want, out)
		}
	}
	for _, h := range []string{"claude-code", "codex"} {
		data, err := os.ReadFile(filepath.Join(f.root(h), "alpha", skillEntryFile))
		if err != nil {
			t.Fatal(err)
		}
		fm, err := parseSkillFrontmatter(data)
		if err != nil || fm.Metadata["garage-revision"] != testRevisionA {
			t.Errorf("%s: installed frontmatter = %+v, %v", h, fm, err)
		}
	}
	if _, err := os.Stat(f.root("copilot-cli")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("an undeclared target was written")
	}

	vout, err := f.verify()
	if err != nil {
		t.Fatalf("verify: %v\n%s", err, vout)
	}
	if strings.Count(vout, "Current\n") != 4 || !strings.Contains(vout, "All managed skills are Current") {
		t.Errorf("verify output:\n%s", vout)
	}

	m, err := loadSkillManifest(f.manifestPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Targets) != 2 || m.Targets[0].Revision != testRevisionA || m.Targets[0].Repository.Path != f.Project ||
		len(m.Targets[0].Skills) != 2 || !strings.HasPrefix(m.Targets[0].Skills[0].Digest, "sha256:") {
		t.Errorf("manifest = %+v", m)
	}
}

func TestSkillsInstallAgainChangesNothing(t *testing.T) {
	f := newSkillsFixture(t, "claude-code")
	f.mustInstall(installOptions{})
	before := snapshotTree(t, filepath.Dir(f.Home))
	f.Env.Now = func() time.Time { return time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC) }
	out := f.mustInstall(installOptions{})
	if after := snapshotTree(t, filepath.Dir(f.Home)); !sameSnapshot(before, after) {
		t.Errorf("a second install changed files:\n%s", out)
	}
}

func TestSkillsNeverTouchNeighbors(t *testing.T) {
	f := newSkillsFixture(t, "claude-code", "codex")
	mustWrite(t, filepath.Join(f.root("claude-code"), "synced", "docx", "SKILL.md"), "synced")
	mustWrite(t, filepath.Join(f.root("claude-code"), ".trash", "x"), "trash")
	mustWrite(t, filepath.Join(f.root("codex"), ".system", "imagegen", "SKILL.md"), "system")
	mustWrite(t, filepath.Join(f.root("codex"), "third-party", "SKILL.md"), "third party")
	mustWrite(t, filepath.Join(f.root("codex"), "SKILL.md"), "stray")
	neighbors := func() map[string]string {
		all := snapshotTree(t, f.Home)
		for p := range all {
			for _, managed := range []string{"alpha", "beta"} {
				if strings.Contains(p, string(filepath.Separator)+managed) {
					delete(all, p)
				}
			}
			if p == f.root("claude-code") || p == f.root("codex") {
				delete(all, p)
			}
		}
		return all
	}
	before := neighbors()
	f.mustInstall(installOptions{})
	f.Build.Revision = testRevisionB
	f.mustInstall(installOptions{})
	if after := neighbors(); !sameSnapshot(before, after) {
		t.Error("installation changed harness-owned or third-party content")
	}
}

func TestSkillsVerifyReportsEveryState(t *testing.T) {
	f := newSkillsFixture(t, "claude-code")
	f.Env.Skills = testSkillFS("alpha", "beta", "gamma", "delta", "omega")
	mustWrite(t, filepath.Join(f.root("claude-code"), "gamma", "SKILL.md"), testSkillMD("gamma"))
	f.mustInstall(installOptions{Replace: []string{"gamma"}})

	// Modified: alpha. Missing: beta. Unmanaged: delta, never installed by
	// Garage. Stale: every unmodified skill after a revision change. Extra:
	// omega, once it is no longer canonical. Not installed: epsilon.
	mustWrite(t, filepath.Join(f.root("claude-code"), "alpha", "SKILL.md"), "edited")
	if err := os.RemoveAll(filepath.Join(f.root("claude-code"), "beta")); err != nil {
		t.Fatal(err)
	}
	m, _ := loadSkillManifest(f.manifestPath())
	var kept []manifestSkill
	for _, s := range m.Targets[0].Skills {
		if s.Name != "delta" {
			kept = append(kept, s)
		}
	}
	m.Targets[0].Skills = kept
	if err := saveSkillManifest(f.manifestPath(), m); err != nil {
		t.Fatal(err)
	}
	f.Build.Revision = testRevisionB
	f.Env.Skills = testSkillFS("alpha", "beta", "gamma", "delta", "epsilon")

	out, err := f.verify()
	if !errors.Is(err, errSkillsNotCurrent) {
		t.Fatalf("verify error = %v, want errSkillsNotCurrent\n%s", err, out)
	}
	for _, want := range []string{
		"alpha", "Modified",
		"beta", "Missing",
		"gamma", "Stale (installed revision " + testRevisionA,
		"delta", "Unmanaged",
		"epsilon", "Not installed",
		"omega", "Extra",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("verify output lacks %q:\n%s", want, out)
		}
	}
}

func TestSkillsVerifyIsReadOnly(t *testing.T) {
	f := newSkillsFixture(t, "claude-code", "codex", "copilot-cli")
	f.mustInstall(installOptions{Targets: []string{"claude-code"}})
	mustWrite(t, filepath.Join(f.root("claude-code"), "alpha", "SKILL.md"), "edited")
	mustWrite(t, filepath.Join(f.root("codex"), "beta", "SKILL.md"), "unmanaged")
	mustWrite(t, filepath.Join(f.Home, ".agents", "skills", "alpha", "SKILL.md"), "collision")
	before := snapshotTree(t, filepath.Dir(f.Home))
	if _, err := f.verify(); err == nil {
		t.Fatal("expected verification to report problems")
	}
	if after := snapshotTree(t, filepath.Dir(f.Home)); !sameSnapshot(before, after) {
		t.Error("verify changed files")
	}
}

func TestSkillsModifiedCopyNeedsConsentAndIsBackedUp(t *testing.T) {
	f := newSkillsFixture(t, "claude-code")
	f.mustInstall(installOptions{})
	dir := filepath.Join(f.root("claude-code"), "alpha")
	mustWrite(t, filepath.Join(dir, "SKILL.md"), "my local edit")
	mustWrite(t, filepath.Join(dir, "notes.bak"), "stray")
	before := snapshotTree(t, filepath.Dir(f.Home))

	out, err := f.install(installOptions{})
	if !errors.Is(err, errInstallRefused) || !strings.Contains(out, "--replace alpha") {
		t.Fatalf("install = %v, want a refusal naming --replace alpha\n%s", err, out)
	}
	if after := snapshotTree(t, filepath.Dir(f.Home)); !sameSnapshot(before, after) {
		t.Fatal("a refused install changed files")
	}

	out = f.mustInstall(installOptions{Replace: []string{"alpha"}})
	if !strings.Contains(out, "replaced (was Modified; backup at") {
		t.Errorf("install output:\n%s", out)
	}
	backup := filepath.Join(f.ConfigDir, userStateDirName, skillBackupsDirName, "20261113T120000Z", "claude-code", "alpha")
	if data, err := os.ReadFile(filepath.Join(backup, "SKILL.md")); err != nil || string(data) != "my local edit" {
		t.Errorf("backup SKILL.md = %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(backup, "notes.bak")); err != nil || string(data) != "stray" {
		t.Errorf("backup notes.bak = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.bak")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("the replaced skill still holds a file that is not part of it")
	}
	if inside, _ := isWithin(backup, f.root("claude-code")); inside {
		t.Error("the backup is inside a scanned skill folder")
	}
	if _, err := f.verify(); err != nil {
		t.Errorf("verify after replacement: %v", err)
	}
}

func TestSkillsUnmanagedCopyIsNeverAdopted(t *testing.T) {
	f := newSkillsFixture(t, "claude-code")
	set, _ := loadSkillSet(f.Env.Skills)
	rendered, _ := renderSkill(set[0], testRevisionA)
	// Even byte-identical to what Garage would install, it stays unmanaged.
	mustWrite(t, filepath.Join(f.root("claude-code"), "alpha", "SKILL.md"), string(rendered[0].Data))

	out, err := f.verify()
	if !errors.Is(err, errSkillsNotCurrent) || !strings.Contains(out, "Unmanaged") {
		t.Fatalf("verify = %v\n%s", err, out)
	}
	if out, err := f.install(installOptions{}); !errors.Is(err, errInstallRefused) {
		t.Fatalf("install = %v, want a refusal\n%s", err, out)
	}
	if _, err := os.Stat(f.manifestPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Error("a refused install wrote the manifest")
	}
	out = f.mustInstall(installOptions{Replace: []string{"alpha"}})
	if !strings.Contains(out, "replaced (was Unmanaged") {
		t.Errorf("install output:\n%s", out)
	}
}

func TestSkillsStaleAndMissingUpdateWithoutConsent(t *testing.T) {
	f := newSkillsFixture(t, "claude-code")
	f.mustInstall(installOptions{})
	if err := os.RemoveAll(filepath.Join(f.root("claude-code"), "beta")); err != nil {
		t.Fatal(err)
	}
	f.Build.Revision = testRevisionB
	out := f.mustInstall(installOptions{})
	for _, want := range []string{"updated (was Stale)", "reinstalled (was Missing)"} {
		if !strings.Contains(out, want) {
			t.Errorf("install output lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(f.ConfigDir, userStateDirName, skillBackupsDirName)); !errors.Is(err, fs.ErrNotExist) {
		t.Error("an unmodified Garage install was backed up")
	}
	if _, err := f.verify(); err != nil {
		t.Errorf("verify: %v", err)
	}
}

func TestSkillsCollisionsRefuseInstallAndFailVerify(t *testing.T) {
	f := newSkillsFixture(t, "claude-code", "codex")
	mustWrite(t, filepath.Join(f.Home, ".agents", "skills", "alpha", "SKILL.md"), "user copy")
	mustWrite(t, filepath.Join(f.Project, ".claude", "skills", "beta", "SKILL.md"), "project copy")

	out, err := f.install(installOptions{})
	if !errors.Is(err, errInstallRefused) {
		t.Fatalf("install = %v, want a refusal\n%s", err, out)
	}
	for _, want := range []string{"shown to Codex alongside", filepath.Join(".agents", "skills", "alpha"),
		"can shadow", filepath.Join(".claude", "skills", "beta"), "no override"} {
		if !strings.Contains(out, want) {
			t.Errorf("refusal lacks %q:\n%s", want, out)
		}
	}
	for _, h := range []string{"claude-code", "codex"} {
		if _, err := os.Stat(f.root(h)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s was written despite the refusal", h)
		}
	}
	vout, err := f.verify()
	if !errors.Is(err, errSkillsNotCurrent) || strings.Count(vout, "collision:") != 2 {
		t.Errorf("verify = %v\n%s", err, vout)
	}
}

func TestCopilotReadsAgentsHomeOnlyAtItsDefaultHome(t *testing.T) {
	f := newSkillsFixture(t, "copilot-cli")
	mustWrite(t, filepath.Join(f.Home, ".agents", "skills", "alpha", "SKILL.md"), "user copy")
	if out, err := f.verify(); !strings.Contains(out, "collision:") {
		t.Errorf("default COPILOT_HOME: verify = %v\n%s", err, out)
	}
	f.Vars["COPILOT_HOME"] = filepath.Join(f.Home, "elsewhere")
	if out, err := f.verify(); strings.Contains(out, "collision:") {
		t.Errorf("redirected COPILOT_HOME: verify = %v\n%s", err, out)
	}
}

func TestSkillsInstallRequiresACleanBuildOrDevelopment(t *testing.T) {
	f := newSkillsFixture(t, "claude-code")
	for name, build := range map[string]buildRevision{
		"modified":    {Revision: testRevisionA},
		"no metadata": {},
	} {
		f.Build = build
		if out, err := f.install(installOptions{}); !errors.Is(err, errUnversionedBuild) {
			t.Errorf("%s: install = %v\n%s", name, err, out)
		}
	}
	if _, err := os.Stat(f.root("claude-code")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("a refused install wrote skills")
	}

	out := f.mustInstall(installOptions{Development: true})
	if !strings.Contains(out, "development install") {
		t.Errorf("install output:\n%s", out)
	}
	m, _ := loadSkillManifest(f.manifestPath())
	set, _ := loadSkillSet(f.Env.Skills)
	want := developmentRevision(skillSetDigest(set))
	if !m.Targets[0].Development || m.Targets[0].Revision != want || m.Targets[0].SkillSetDigest != skillSetDigest(set) {
		t.Errorf("manifest target = %+v, want development revision %s", m.Targets[0], want)
	}
	data, _ := os.ReadFile(filepath.Join(f.root("claude-code"), "alpha", "SKILL.md"))
	if fm, _ := parseSkillFrontmatter(data); fm.Metadata["garage-revision"] != want {
		t.Errorf("installed revision = %q, want %q", fm.Metadata["garage-revision"], want)
	}
	if _, err := f.verify(); err != nil {
		t.Errorf("verify with the same development build: %v", err)
	}
}

func TestSkillsUnverifiedHarnessIsAWarningOnly(t *testing.T) {
	f := newSkillsFixture(t, "claude-code", "codex")
	f.Versions["claude-code"] = "2.2.0"
	delete(f.Versions, "codex")
	out := f.mustInstall(installOptions{})
	vout, err := f.verify()
	if err != nil {
		t.Fatalf("verify: %v\n%s", err, vout)
	}
	for _, o := range []string{out, vout} {
		if strings.Count(o, "harness Unverified") != 2 || !strings.Contains(o, "installed version 2.2.0") ||
			!strings.Contains(o, "could not be determined") || strings.Contains(o, "as confirmed") {
			t.Errorf("output:\n%s", o)
		}
	}
}

func TestSkillsRequireAValidConfigurationWithTargets(t *testing.T) {
	f := newSkillsFixture(t)
	if _, err := f.verify(); err == nil || !strings.Contains(err.Error(), "declares no skill targets") {
		t.Errorf("no targets: %v", err)
	}
	if err := os.Remove(filepath.Join(f.Project, garageDirName, configFileName)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.install(installOptions{}); !errors.Is(err, errNoConfig) {
		t.Errorf("no configuration: %v", err)
	}
	f.writeConfig("claude-code")
	if _, err := f.install(installOptions{Targets: []string{"codex"}}); err == nil ||
		!strings.Contains(err.Error(), "not a skill target declared") {
		t.Errorf("undeclared --target: %v", err)
	}
	if _, err := f.install(installOptions{Replace: []string{"nope"}}); err == nil ||
		!strings.Contains(err.Error(), "not a canonical skill") {
		t.Errorf("unknown --replace: %v", err)
	}
	if _, err := os.Stat(f.root("claude-code")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("a failed command wrote skills")
	}
}

func TestSkillsInvalidManifestIsNeverReplaced(t *testing.T) {
	f := newSkillsFixture(t, "claude-code")
	mustWrite(t, f.manifestPath(), "{not json")
	if _, err := f.install(installOptions{}); err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Fatalf("install = %v", err)
	}
	if data, _ := os.ReadFile(f.manifestPath()); string(data) != "{not json" {
		t.Error("the invalid manifest was changed")
	}
}

func TestSkillsLastInstallOwnsTheTarget(t *testing.T) {
	f := newSkillsFixture(t, "claude-code")
	f.mustInstall(installOptions{})
	other := filepath.Join(filepath.Dir(f.Project), "other")
	config, err := os.ReadFile(filepath.Join(f.Project, garageDirName, configFileName))
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(other, garageDirName, configFileName), string(config))
	f.Env.WorkDir = other
	out := f.mustInstall(installOptions{})
	if !strings.Contains(out, "last installed from "+f.Project) {
		t.Errorf("install output:\n%s", out)
	}
	m, _ := loadSkillManifest(f.manifestPath())
	if m.Targets[0].Repository.Path != other {
		t.Errorf("owner = %+v, want %s", m.Targets[0].Repository, other)
	}
}

func TestApplySkillActionRefusesAConcurrentChange(t *testing.T) {
	f := newSkillsFixture(t, "claude-code")
	f.mustInstall(installOptions{})
	dir := filepath.Join(f.root("claude-code"), "alpha")
	files, _, _ := readSkillDir(dir)
	inspection := skillInspection{Name: "alpha", Dir: dir, State: stateStale, Digest: digestSkillFiles(files)}
	mustWrite(t, filepath.Join(dir, "SKILL.md"), "changed after planning")
	action := skillAction{Inspection: inspection, Files: []skillFile{{Path: "SKILL.md", Data: []byte("new")}}}
	_, err := applySkillAction(action, resolvedTarget{Config: skillTarget{Harness: "claude-code"}, Root: f.root("claude-code")}, t.TempDir())
	if !errors.Is(err, errSkillChanged) {
		t.Fatalf("apply = %v, want errSkillChanged", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "SKILL.md")); string(data) != "changed after planning" {
		t.Error("the concurrent change was overwritten")
	}
}

func TestSkillsRefuseSymlinkedContent(t *testing.T) {
	f := newSkillsFixture(t, "claude-code")
	target := filepath.Join(t.TempDir(), "elsewhere")
	mustMkdir(t, target)
	mustMkdir(t, f.root("claude-code"))
	if err := os.Symlink(target, filepath.Join(f.root("claude-code"), "alpha")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	out, err := f.install(installOptions{Replace: []string{"alpha"}})
	if !errors.Is(err, errInstallRefused) || !strings.Contains(out, "not a regular file") {
		t.Fatalf("install = %v\n%s", err, out)
	}
}
