package cli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

// testManual returns a manual file with a catalog entry.
func testManual(name, version, status, description, body string) string {
	return "---\nname: " + name + "\nversion: \"" + version + "\"\nstatus: " + status +
		"\ndescription: \"" + description + "\"\n---\n\n" + body
}

// useManualCatalog makes fsys the manual catalog seen by validation, guided
// setup, and skill installation for the rest of the test.
func useManualCatalog(t *testing.T, fsys fs.FS) {
	t.Helper()
	saved := manualCatalogFS
	manualCatalogFS = func() fs.FS { return fsys }
	t.Cleanup(func() { manualCatalogFS = saved })
}

func testCatalog() fstest.MapFS {
	return fstest.MapFS{
		"README.md": &fstest.MapFile{Data: []byte("# Manuals\n")},
		"go.md":     &fstest.MapFile{Data: []byte(testManual("go", "0.1", "provisional", "Go guidance.", "# Go Manual\n\nUse small packages.\n"))},
		"rust.md":   &fstest.MapFile{Data: []byte(testManual("rust", "1.0", "stable", "Rust guidance.", "# Rust Manual\n\nPrefer ownership.\n"))},
	}
}

// --- The catalog ---

func TestEmbeddedManualCatalogMatchesTheManualsReadme(t *testing.T) {
	catalog, err := embeddedManualCatalog()
	if err != nil {
		t.Fatalf("embedded catalog: %v", err)
	}
	if findManual(catalog, "go") == nil {
		t.Fatalf("the embedded catalog %v has no go manual", manualNames(catalog))
	}
	readme := readFile(t, filepath.Join("..", "..", "manuals", "README.md"))
	table := readme[strings.Index(readme, "## Current manuals"):]
	table = table[:strings.Index(table[3:], "\n## ")+3]
	rows := regexp.MustCompile(`(?m)^\| \[[^\]]+\]\(([^)]+)\) \| ([^|]+?) \|`).FindAllStringSubmatch(table, -1)
	if len(rows) != len(catalog) {
		t.Fatalf("manuals/README.md lists %d manuals; the catalog has %d (%v)", len(rows), len(catalog), manualNames(catalog))
	}
	for _, m := range catalog {
		found := false
		for _, row := range rows {
			if row[1] == m.File {
				found = true
				if want := m.Version + " (" + m.Status + ")"; row[2] != want {
					t.Errorf("manuals/README.md lists %s as %q; its frontmatter says %q", m.File, row[2], want)
				}
			}
		}
		if !found {
			t.Errorf("manuals/README.md does not list %s", m.File)
		}
	}
}

func TestLoadManualCatalogReadsFrontmatterAndSkipsTheReadme(t *testing.T) {
	catalog, err := loadManualCatalog(testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if got := manualNames(catalog); len(got) != 2 || got[0] != "go" || got[1] != "rust" {
		t.Fatalf("catalog = %v, want [go rust]", got)
	}
	g := catalog[0]
	if g.Version != "0.1" || g.Status != "provisional" || g.Description != "Go guidance." || string(g.Body) != "# Go Manual\n\nUse small packages.\n" {
		t.Errorf("go entry = %+v", g)
	}
}

func TestLoadManualCatalogRefusesInvalidEntries(t *testing.T) {
	cases := map[string]string{
		"no frontmatter":       "# Go Manual\n",
		"unknown field":        "---\nname: go\nversion: \"0.1\"\nstatus: stable\ndescription: \"x\"\nowner: me\n---\n\nbody\n",
		"name not file name":   testManual("golang", "0.1", "stable", "x", "body\n"),
		"unknown status":       testManual("go", "0.1", "draft", "x", "body\n"),
		"empty description":    testManual("go", "0.1", "stable", "", "body\n"),
		"version with a space": testManual("go", "zero one", "stable", "x", "body\n"),
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := loadManualCatalog(fstest.MapFS{"go.md": &fstest.MapFile{Data: []byte(content)}})
			if err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestManualSkillDeliversTheManualAndSaysWhenItApplies(t *testing.T) {
	catalog, _ := loadManualCatalog(testCatalog())
	skill, err := manualSkill(catalog[0])
	if err != nil {
		t.Fatal(err)
	}
	if skill.Name != "builder-garage-manual-go" || len(skill.Files) != 1 {
		t.Fatalf("skill = %s with %d files", skill.Name, len(skill.Files))
	}
	data := skill.Files[0].Data
	fm, err := parseSkillFrontmatter(data)
	if err != nil {
		t.Fatal(err)
	}
	if fm.Name != skill.Name || fm.Metadata["garage-manual"] != "go" || fm.Metadata["garage-manual-version"] != "0.1" {
		t.Errorf("frontmatter = %+v", fm)
	}
	text := string(data)
	for _, want := range []string{"Apply it only in a project whose .garage/config.yaml adopts the go manual",
		"being installed does not make it apply", "Use small packages."} {
		if !strings.Contains(text, want) {
			t.Errorf("manual skill does not contain %q:\n%s", want, text)
		}
	}
	rendered, err := renderSkill(skill, testRevisionA)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if fm, _ := parseSkillFrontmatter(rendered[0].Data); fm.Metadata["garage-revision"] != testRevisionA {
		t.Errorf("rendered revision = %q", fm.Metadata["garage-revision"])
	}
}

// --- Validation ---

func TestValidationChecksManualsAgainstTheCatalog(t *testing.T) {
	useManualCatalog(t, testCatalog())
	withManual := func(name, version string) projectConfig {
		return decodeOrFail(t, validConfigYAML+"  guidance:\n    manuals:\n      - {name: "+name+", version: \""+version+"\"}\n")
	}
	problems := validateConfig(withManual("python", "1.0"))
	if len(problems) != 1 || !strings.Contains(problems[0], `"python" is not a manual this garage carries; available: go, rust`) {
		t.Errorf("unknown manual: problems = %v", problems)
	}
	if problems := validateConfig(withManual("go", "0.2")); problems != nil {
		t.Errorf("a known manual at another version must stay valid: %v", problems)
	}

	var out strings.Builder
	if err := writeConfigSummary(&out, withManual("go", "0.2")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "go 0.2 (this garage delivers 0.1)") {
		t.Errorf("summary does not report the version difference:\n%s", out.String())
	}
}

func TestGuidedSetupOffersTheCatalogAsAPickList(t *testing.T) {
	useManualCatalog(t, testCatalog())
	noDefaultBranch(t)
	dir := t.TempDir()
	answers := append(append([]string(nil), defaultAnswers[:len(defaultAnswers)-2]...),
		"2", // manuals: rust only
		"",  // agent tools: none
		"y")
	ask := &scriptedPrompter{answers: answers}
	if _, err := initProject(dir, projectConfig{}, ask); err != nil {
		t.Fatal(err)
	}
	all := ask.transcript()
	for _, want := range []string{"1) go 0.1 (provisional): Go guidance.", "2) rust 1.0 (stable): Rust guidance."} {
		if !strings.Contains(all, want) {
			t.Errorf("the pick list does not show %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "Manual name") || strings.Contains(all, "Version adopted") {
		t.Error("guided setup still asks for a manual name or version as free text")
	}
	g := mustLoadValid(t, dir).Project.Guidance
	if g == nil || len(g.Manuals) != 1 || g.Manuals[0] != (manualChoice{Name: "rust", Version: "1.0"}) {
		t.Errorf("manuals = %+v, want rust 1.0, the delivered version", g)
	}
}

// --- Delivery ---

// writeConfigWithManuals writes the fixture configuration with the given
// adopted manuals ("name@version") and skill targets.
func (f *skillsFixture) writeConfigWithManuals(manuals []string, harnesses ...string) {
	content := validConfigYAML
	if len(manuals) > 0 {
		content += "  guidance:\n    manuals:\n"
		for _, m := range manuals {
			name, version, _ := strings.Cut(m, "@")
			content += "      - {name: " + name + ", version: \"" + version + "\"}\n"
		}
	}
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

// otherProject returns a second project that shares f's home, user
// configuration directory, and skill targets.
func (f *skillsFixture) otherProject() *skillsFixture {
	g := *f
	g.Project = filepath.Join(filepath.Dir(f.Project), "project-b")
	mustMkdir(f.t, g.Project)
	g.Env.WorkDir = g.Project
	g.Env.Git = func(_ context.Context, dir string, args ...string) ([]byte, error) {
		if args[0] == "rev-parse" {
			return []byte(g.Project + "\n" + filepath.Join(g.Project, ".git") + "\n"), nil
		}
		return nil, errors.New("no remote")
	}
	return &g
}

func manualRecord(t *testing.T, f *skillsFixture, harness, skill string) *manifestSkill {
	t.Helper()
	m, err := loadSkillManifest(f.manifestPath())
	if err != nil {
		t.Fatal(err)
	}
	return m.target(f.root(harness)).skill(skill)
}

const goSkill = "builder-garage-manual-go"

func TestAdoptedManualsAreDeliveredAsSkillsForTheirProjects(t *testing.T) {
	useManualCatalog(t, testCatalog())
	a := newSkillsFixture(t)
	a.writeConfigWithManuals([]string{"go@0.1"}, "claude-code")
	b := a.otherProject()
	b.writeConfigWithManuals(nil, "claude-code")
	goDir := filepath.Join(a.root("claude-code"), goSkill)
	rustDir := filepath.Join(a.root("claude-code"), "builder-garage-manual-rust")

	// Project A adopts go: only go is installed, for A.
	out := a.mustInstall(installOptions{})
	if !strings.Contains(out, goSkill) || !strings.Contains(out, "(manual go 0.1)") {
		t.Errorf("install output:\n%s", out)
	}
	if data, err := os.ReadFile(filepath.Join(goDir, "SKILL.md")); err != nil || !strings.Contains(string(data), "Use small packages.") {
		t.Fatalf("the go manual skill was not installed: %v", err)
	}
	if _, err := os.Stat(rustDir); !errors.Is(err, fs.ErrNotExist) {
		t.Error("a manual no project adopts was installed")
	}
	if r := manualRecord(t, a, "claude-code", goSkill); r == nil || len(r.Projects) != 1 || r.Projects[0].Path != a.Project {
		t.Fatalf("manifest record = %+v, want project A only", r)
	}
	if vout, err := a.verify(); err != nil || !strings.Contains(vout, "Current (manual go 0.1, adopted here)") {
		t.Fatalf("verify A = %v\n%s", err, vout)
	}

	// Project B adopts nothing: go stays, still for A, and is not B's problem.
	b.mustInstall(installOptions{})
	if _, err := os.Stat(goDir); err != nil {
		t.Fatal("installing from another project removed a manual still adopted by A")
	}
	vout, err := b.verify()
	if err != nil || !strings.Contains(vout, "installed for 1 other project(s); not adopted here") {
		t.Fatalf("verify B = %v\n%s", err, vout)
	}

	// B adopts go too: both projects are associated.
	b.writeConfigWithManuals([]string{"go@0.1"}, "claude-code")
	b.mustInstall(installOptions{})
	if r := manualRecord(t, a, "claude-code", goSkill); r == nil || len(r.Projects) != 2 {
		t.Fatalf("manifest record = %+v, want projects A and B", r)
	}

	// A drops go: B still adopts it, so it stays.
	a.writeConfigWithManuals(nil, "claude-code")
	out = a.mustInstall(installOptions{})
	if _, err := os.Stat(goDir); err != nil {
		t.Fatalf("a manual still adopted by B was removed:\n%s", out)
	}
	if r := manualRecord(t, a, "claude-code", goSkill); r == nil || len(r.Projects) != 1 || r.Projects[0].Path != b.Project {
		t.Fatalf("manifest record = %+v, want project B only", r)
	}

	// B drops go: no project adopts it, so the unmodified copy is removed.
	b.writeConfigWithManuals(nil, "claude-code")
	out = b.mustInstall(installOptions{})
	if !strings.Contains(out, "removed: no project adopts it any longer") {
		t.Errorf("install output:\n%s", out)
	}
	if _, err := os.Stat(goDir); !errors.Is(err, fs.ErrNotExist) {
		t.Error("a manual no project adopts was left in place")
	}
	if r := manualRecord(t, a, "claude-code", goSkill); r != nil {
		t.Errorf("the removed manual is still recorded: %+v", r)
	}
	if _, err := b.verify(); err != nil {
		t.Errorf("verify after removal: %v", err)
	}
}

func TestAModifiedManualNoProjectAdoptsIsKeptAndReported(t *testing.T) {
	useManualCatalog(t, testCatalog())
	f := newSkillsFixture(t)
	f.writeConfigWithManuals([]string{"go@0.1"}, "claude-code")
	f.mustInstall(installOptions{})
	goFile := filepath.Join(f.root("claude-code"), goSkill, "SKILL.md")
	mustWrite(t, goFile, "my notes")

	f.writeConfigWithManuals(nil, "claude-code")
	out := f.mustInstall(installOptions{})
	if !strings.Contains(out, "kept: no project adopts it, but it was modified") {
		t.Errorf("install output:\n%s", out)
	}
	if data, _ := os.ReadFile(goFile); string(data) != "my notes" {
		t.Error("a modified manual copy was changed or removed")
	}
	vout, err := f.verify()
	if !errors.Is(err, errSkillsNotCurrent) || !strings.Contains(vout, "no project adopts it; left in place because it was modified") {
		t.Errorf("verify = %v\n%s", err, vout)
	}
}

func TestAnUnmanagedManualCopyNeedsConsent(t *testing.T) {
	useManualCatalog(t, testCatalog())
	f := newSkillsFixture(t)
	f.writeConfigWithManuals([]string{"go@0.1"}, "claude-code")
	mustWrite(t, filepath.Join(f.root("claude-code"), goSkill, "SKILL.md"), "hand copy")

	out, err := f.install(installOptions{})
	if !errors.Is(err, errInstallRefused) || !strings.Contains(out, "--replace "+goSkill) {
		t.Fatalf("install = %v, want a refusal naming --replace %s\n%s", err, goSkill, out)
	}
	out = f.mustInstall(installOptions{Replace: []string{goSkill}})
	if !strings.Contains(out, "replaced (was Unmanaged; backup at") {
		t.Errorf("install output:\n%s", out)
	}
}

func TestAManualVersionDifferenceIsReportedNotRefused(t *testing.T) {
	useManualCatalog(t, testCatalog())
	f := newSkillsFixture(t)
	f.writeConfigWithManuals([]string{"go@0.2"}, "claude-code")
	f.mustInstall(installOptions{})
	vout, err := f.verify()
	if err != nil {
		t.Fatalf("verify: %v\n%s", err, vout)
	}
	if !strings.Contains(vout, "the configuration names version 0.2, but this garage delivers 0.1") {
		t.Errorf("verify output:\n%s", vout)
	}
}

func TestAManualSkillCollisionRefusesInstall(t *testing.T) {
	useManualCatalog(t, testCatalog())
	f := newSkillsFixture(t)
	f.writeConfigWithManuals([]string{"go@0.1"}, "claude-code")
	mustWrite(t, filepath.Join(f.Project, ".claude", "skills", goSkill, "SKILL.md"), "project copy")
	out, err := f.install(installOptions{})
	if !errors.Is(err, errInstallRefused) || !strings.Contains(out, goSkill+" also exists at") {
		t.Fatalf("install = %v\n%s", err, out)
	}
}

func TestAVersion1ManifestIsReadAndUpgraded(t *testing.T) {
	useManualCatalog(t, testCatalog())
	f := newSkillsFixture(t)
	f.writeConfigWithManuals(nil, "claude-code")
	mustMkdir(t, filepath.Dir(f.manifestPath()))
	mustWrite(t, f.manifestPath(), "{\"version\": 1, \"targets\": []}\n")
	f.mustInstall(installOptions{})
	m, err := loadSkillManifest(f.manifestPath())
	if err != nil || m.Version != skillManifestVersion {
		t.Fatalf("manifest = %+v, %v; want version %d", m, err, skillManifestVersion)
	}
	if !strings.Contains(readFile(t, f.manifestPath()), "\"version\": 2") {
		t.Error("the upgraded manifest was not written as version 2")
	}
}
