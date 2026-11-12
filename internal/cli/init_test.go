package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// newProjectDir returns an empty temporary directory and makes it the working
// directory for the duration of the test, so "garage init" never touches a
// real project.
func newProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

func configPathIn(projectDir string) string {
	return filepath.Join(projectDir, garageDirName, configFileName)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent of %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links are not available: %v", err)
	}
}

func entryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read directory %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestInitCreatesConfiguration(t *testing.T) {
	dir := newProjectDir(t)

	stdout, stderr, err := runCLI(t, "dev", "init")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, garageDirName))
	if err != nil {
		t.Fatalf("stat %s: %v", garageDirName, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", garageDirName)
	}

	if got := entryNames(t, filepath.Join(dir, garageDirName)); len(got) != 1 || got[0] != configFileName {
		t.Errorf("%s contains %v, want only %s", garageDirName, got, configFileName)
	}

	for _, want := range []string{
		"Initialized Builder Garage",
		"created " + filepath.Join(garageDirName, configFileName),
		"schema version 1",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, stdout)
		}
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want it empty", stderr)
	}
}

func TestInitGeneratesValidYAML(t *testing.T) {
	dir := newProjectDir(t)

	if _, _, err := runCLI(t, "dev", "init"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content := readFile(t, configPathIn(dir))

	// The file must be a YAML mapping holding exactly the schema version.
	var generic map[string]any
	if err := yaml.Unmarshal([]byte(content), &generic); err != nil {
		t.Fatalf("generated configuration is not valid YAML: %v\n%s", err, content)
	}
	if len(generic) != 1 || generic["schema_version"] != configSchemaVersion {
		t.Errorf("configuration = %v, want only schema_version: %d", generic, configSchemaVersion)
	}

	// It must also decode into the configuration type without unknown fields.
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(content)))
	decoder.KnownFields(true)
	var cfg projectConfig
	if err := decoder.Decode(&cfg); err != nil {
		t.Fatalf("decode into projectConfig: %v", err)
	}
	if cfg.SchemaVersion != configSchemaVersion {
		t.Errorf("schema version = %d, want %d", cfg.SchemaVersion, configSchemaVersion)
	}
}

func TestInitInExistingProjectLeavesOtherFilesUntouched(t *testing.T) {
	dir := newProjectDir(t)
	writeFile(t, filepath.Join(dir, "README.md"), "# existing project\n")
	writeFile(t, filepath.Join(dir, "src", "main.txt"), "source\n")

	if _, _, err := runCLI(t, "dev", "init"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := readFile(t, filepath.Join(dir, "README.md")); got != "# existing project\n" {
		t.Errorf("README.md was modified: %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "src", "main.txt")); got != "source\n" {
		t.Errorf("src/main.txt was modified: %q", got)
	}
	if got := entryNames(t, dir); len(got) != 3 {
		t.Errorf("project directory contains %v, want .garage, README.md, and src", got)
	}
}

func TestInitDoesNotOverwriteExistingConfiguration(t *testing.T) {
	dir := newProjectDir(t)
	const existing = "schema_version: 1\ncustom: keep-me\n"
	writeFile(t, configPathIn(dir), existing)

	stdout, stderr, err := runCLI(t, "dev", "init")
	if err == nil {
		t.Fatal("expected an error when configuration already exists")
	}
	if !errors.Is(err, errConfigExists) {
		t.Errorf("error = %v, want it to wrap errConfigExists", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error = %q, want it to explain --force", err)
	}
	if !strings.Contains(stderr, "Error:") {
		t.Errorf("stderr = %q, want the error to be reported there", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want no completion message on failure", stdout)
	}
	if got := readFile(t, configPathIn(dir)); got != existing {
		t.Errorf("existing configuration was modified: %q", got)
	}
}

func TestInitRepeatedIsSafe(t *testing.T) {
	dir := newProjectDir(t)

	if _, _, err := runCLI(t, "dev", "init"); err != nil {
		t.Fatalf("first init: unexpected error: %v", err)
	}
	first := readFile(t, configPathIn(dir))

	for attempt := 2; attempt <= 3; attempt++ {
		if _, _, err := runCLI(t, "dev", "init"); !errors.Is(err, errConfigExists) {
			t.Fatalf("init attempt %d: error = %v, want errConfigExists", attempt, err)
		}
		if got := readFile(t, configPathIn(dir)); got != first {
			t.Fatalf("init attempt %d changed the configuration: %q", attempt, got)
		}
	}

	if got := entryNames(t, filepath.Join(dir, garageDirName)); len(got) != 1 {
		t.Errorf("%s contains %v after repeated init, want only %s", garageDirName, got, configFileName)
	}
}

func TestInitForceOverwritesExistingConfiguration(t *testing.T) {
	dir := newProjectDir(t)
	writeFile(t, configPathIn(dir), "custom: replace-me\n")
	writeFile(t, filepath.Join(dir, garageDirName, "notes.txt"), "unrelated\n")

	stdout, _, err := runCLI(t, "dev", "init", "--force")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want, err := renderConfig()
	if err != nil {
		t.Fatalf("renderConfig: %v", err)
	}
	if got := readFile(t, configPathIn(dir)); got != string(want) {
		t.Errorf("configuration = %q, want %q", got, want)
	}
	if !strings.Contains(stdout, "overwrote "+filepath.Join(garageDirName, configFileName)) {
		t.Errorf("stdout does not report the overwrite:\n%s", stdout)
	}
	if got := readFile(t, filepath.Join(dir, garageDirName, "notes.txt")); got != "unrelated\n" {
		t.Errorf("unrelated file in %s was modified: %q", garageDirName, got)
	}
}

func TestInitForceWithoutExistingConfiguration(t *testing.T) {
	dir := newProjectDir(t)

	stdout, _, err := runCLI(t, "dev", "init", "--force")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "created ") {
		t.Errorf("stdout should report creation, not an overwrite:\n%s", stdout)
	}
	if _, err := os.Stat(configPathIn(dir)); err != nil {
		t.Errorf("configuration was not created: %v", err)
	}
}

func TestInitRejectsSymlinkedGarageDirectory(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "force"}[force], func(t *testing.T) {
			projectDir := t.TempDir()
			externalDir := t.TempDir()
			externalConfig := filepath.Join(externalDir, configFileName)
			const original = "external: unchanged\n"
			writeFile(t, externalConfig, original)
			symlinkOrSkip(t, externalDir, filepath.Join(projectDir, garageDirName))

			if _, err := initProject(projectDir, force); err == nil {
				t.Fatal("expected a symlinked .garage directory to be rejected")
			} else if !strings.Contains(err.Error(), "symbolic link") {
				t.Errorf("error = %q, want it to identify the symbolic link", err)
			}
			if got := readFile(t, externalConfig); got != original {
				t.Errorf("external configuration was modified: %q", got)
			}
			if got := entryNames(t, externalDir); len(got) != 1 || got[0] != configFileName {
				t.Errorf("external directory contains %v, want only %s", got, configFileName)
			}
		})
	}
}

func TestInitRejectsSymlinkedConfiguration(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "force"}[force], func(t *testing.T) {
			projectDir := t.TempDir()
			externalDir := t.TempDir()
			externalConfig := filepath.Join(externalDir, "external.yaml")
			const original = "external: unchanged\n"
			writeFile(t, externalConfig, original)
			if err := os.Mkdir(filepath.Join(projectDir, garageDirName), 0o755); err != nil {
				t.Fatalf("create %s: %v", garageDirName, err)
			}
			symlinkOrSkip(t, externalConfig, configPathIn(projectDir))

			if _, err := initProject(projectDir, force); err == nil {
				t.Fatal("expected a symlinked configuration to be rejected")
			} else if !strings.Contains(err.Error(), "symbolic link") {
				t.Errorf("error = %q, want it to identify the symbolic link", err)
			}
			if got := readFile(t, externalConfig); got != original {
				t.Errorf("external configuration was modified: %q", got)
			}
			info, err := os.Lstat(configPathIn(projectDir))
			if err != nil {
				t.Fatalf("lstat configuration symlink: %v", err)
			}
			if info.Mode()&os.ModeSymlink == 0 {
				t.Error("configuration symlink was replaced")
			}
		})
	}
}

func TestInitForceReplacesHardLinkWithoutModifyingPeer(t *testing.T) {
	projectDir := t.TempDir()
	externalConfig := filepath.Join(projectDir, "shared-config.yaml")
	const original = "shared: unchanged\n"
	writeFile(t, externalConfig, original)
	if err := os.Mkdir(filepath.Join(projectDir, garageDirName), 0o755); err != nil {
		t.Fatalf("create %s: %v", garageDirName, err)
	}
	if err := os.Link(externalConfig, configPathIn(projectDir)); err != nil {
		t.Skipf("hard links are not available: %v", err)
	}

	overwritten, err := initProject(projectDir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !overwritten {
		t.Error("overwritten = false, want true")
	}
	if got := readFile(t, externalConfig); got != original {
		t.Errorf("hard-linked peer was modified: %q", got)
	}
	want, err := renderConfig()
	if err != nil {
		t.Fatalf("renderConfig: %v", err)
	}
	if got := readFile(t, configPathIn(projectDir)); got != string(want) {
		t.Errorf("configuration = %q, want %q", got, want)
	}
	externalInfo, err := os.Stat(externalConfig)
	if err != nil {
		t.Fatalf("stat hard-linked peer: %v", err)
	}
	configInfo, err := os.Stat(configPathIn(projectDir))
	if err != nil {
		t.Fatalf("stat replacement configuration: %v", err)
	}
	if os.SameFile(externalInfo, configInfo) {
		t.Error("replacement configuration still shares the original inode")
	}
}

func TestReplacementPreparationFailurePreservesOriginal(t *testing.T) {
	projectDir := t.TempDir()
	configPath := configPathIn(projectDir)
	const original = "original: keep\n"
	writeFile(t, configPath, original)
	garageDir := filepath.Dir(configPath)
	garageInfo, err := os.Lstat(garageDir)
	if err != nil {
		t.Fatalf("lstat %s: %v", garageDirName, err)
	}
	injectedErr := errors.New("injected temporary write failure")
	prepare := func(dir string, content []byte) (string, error) {
		return prepareTemporaryConfigUsing(
			dir,
			content,
			func(*os.File, []byte) error { return injectedErr },
		)
	}

	err = replaceConfiguration(
		configPath,
		[]byte("replacement\n"),
		garageInfo,
		prepare,
		commitConfigReplacement,
	)
	if !errors.Is(err, injectedErr) {
		t.Fatalf("error = %v, want it to wrap the injected write failure", err)
	}
	if got := readFile(t, configPath); got != original {
		t.Errorf("original configuration was modified: %q", got)
	}
	if got := entryNames(t, garageDir); len(got) != 1 || got[0] != configFileName {
		t.Errorf("%s contains %v after failed preparation, want only %s",
			garageDirName, got, configFileName)
	}
}

func TestReplacementCommitFailurePreservesOriginal(t *testing.T) {
	projectDir := t.TempDir()
	configPath := configPathIn(projectDir)
	const original = "original: keep\n"
	writeFile(t, configPath, original)
	garageDir := filepath.Dir(configPath)
	garageInfo, err := os.Lstat(garageDir)
	if err != nil {
		t.Fatalf("lstat %s: %v", garageDirName, err)
	}
	injectedErr := errors.New("injected replacement failure")

	err = replaceConfiguration(
		configPath,
		[]byte("replacement\n"),
		garageInfo,
		prepareTemporaryConfig,
		func(string, string) error { return injectedErr },
	)
	if !errors.Is(err, injectedErr) {
		t.Fatalf("error = %v, want it to wrap the injected replacement failure", err)
	}
	if got := readFile(t, configPath); got != original {
		t.Errorf("original configuration was modified: %q", got)
	}
	if got := entryNames(t, garageDir); len(got) != 1 || got[0] != configFileName {
		t.Errorf("%s contains %v after failed replacement, want only %s",
			garageDirName, got, configFileName)
	}
}

// A regular file named .garage makes directory creation fail on every
// platform, without depending on permission bits or the current user.
func TestInitFailsWhenGarageDirCannotBeCreated(t *testing.T) {
	dir := newProjectDir(t)
	blocker := filepath.Join(dir, garageDirName)
	writeFile(t, blocker, "not a directory\n")

	for _, args := range [][]string{{"init"}, {"init", "--force"}} {
		stdout, stderr, err := runCLI(t, "dev", args...)
		if err == nil {
			t.Fatalf("garage %v: expected an error", args)
		}
		if !strings.Contains(err.Error(), garageDirName) {
			t.Errorf("garage %v: error = %q, want it to name %s", args, err, garageDirName)
		}
		if !strings.Contains(stderr, "Error:") {
			t.Errorf("garage %v: stderr = %q, want the error to be reported there", args, stderr)
		}
		if stdout != "" {
			t.Errorf("garage %v: stdout = %q, want no completion message on failure", args, stdout)
		}
		if got := readFile(t, blocker); got != "not a directory\n" {
			t.Errorf("garage %v: the existing %s file was modified: %q", args, garageDirName, got)
		}
	}
}

// A directory at the configuration path cannot be written as a file, with or
// without --force, on every platform.
func TestInitFailsWhenConfigurationPathIsADirectory(t *testing.T) {
	dir := newProjectDir(t)
	if err := os.MkdirAll(configPathIn(dir), 0o755); err != nil {
		t.Fatalf("create directory at the configuration path: %v", err)
	}

	for _, args := range [][]string{{"init"}, {"init", "--force"}} {
		stdout, _, err := runCLI(t, "dev", args...)
		if err == nil {
			t.Fatalf("garage %v: expected an error", args)
		}
		if stdout != "" {
			t.Errorf("garage %v: stdout = %q, want no completion message on failure", args, stdout)
		}
		info, statErr := os.Stat(configPathIn(dir))
		if statErr != nil || !info.IsDir() {
			t.Errorf("garage %v: the directory at the configuration path was replaced", args)
		}
	}
}

func TestInitRejectsArguments(t *testing.T) {
	dir := newProjectDir(t)

	if _, _, err := runCLI(t, "dev", "init", "extra"); err == nil {
		t.Fatal("expected an error for an unexpected argument")
	}
	if got := entryNames(t, dir); len(got) != 0 {
		t.Errorf("project directory contains %v, want nothing created", got)
	}
}

func TestInitProjectUsesTheGivenDirectory(t *testing.T) {
	dir := t.TempDir()

	overwritten, err := initProject(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if overwritten {
		t.Error("overwritten = true for a new configuration")
	}
	if _, err := os.Stat(configPathIn(dir)); err != nil {
		t.Errorf("configuration was not created in the given directory: %v", err)
	}
}
