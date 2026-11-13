package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/bdonaldharris/builder-garage/skills"
)

// Garage's per-user skill state lives under the user configuration
// directory, never the cache, so backups survive (#38 decision 6).
const (
	userStateDirName     = "builder-garage"
	skillManifestName    = "skills-manifest.json"
	skillBackupsDirName  = "skill-backups"
	skillManifestVersion = 1
	harnessProbeTimeout  = 10 * time.Second
)

// skillsEnv holds the external dependencies of skill installation and
// verification, so tests can substitute them. Tests never use the real home
// or configuration directories.
type skillsEnv struct {
	WorkDir       string
	Skills        fs.FS
	Build         func() buildRevision
	Getenv        func(string) string
	HomeDir       func() (string, error)
	ConfigDir     func() (string, error)
	Git           gitRunner
	Now           func() time.Time
	ProbeHarness  func(ctx context.Context, harness string) (string, error)
	GarageVersion string
}

func defaultSkillsEnv(garageVersion string) (skillsEnv, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return skillsEnv{}, fmt.Errorf("cannot determine the current directory: %w", err)
	}
	return skillsEnv{
		WorkDir:       workDir,
		Skills:        skills.FS(),
		Build:         readBuildRevision,
		Getenv:        os.Getenv,
		HomeDir:       os.UserHomeDir,
		ConfigDir:     os.UserConfigDir,
		Git:           runGit,
		Now:           time.Now,
		ProbeHarness:  probeHarnessVersion,
		GarageVersion: garageVersion,
	}, nil
}

// harnessCommands are the executables whose version identifies each
// supported harness. They are run directly, without a shell.
var harnessCommands = map[string]string{
	"claude-code": "claude",
	"codex":       "codex",
	"copilot-cli": "copilot",
}

var harnessVersionPattern = regexp.MustCompile(`\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?`)

// probeHarnessVersion runs "<harness command> --version" and returns the
// first version number in its output.
func probeHarnessVersion(ctx context.Context, harness string) (string, error) {
	name, ok := harnessCommands[harness]
	if !ok {
		return "", fmt.Errorf("unknown harness %q", harness)
	}
	ctx, cancel := context.WithTimeout(ctx, harnessProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s --version: %w", name, err)
	}
	version := harnessVersionPattern.FindString(out.String())
	if version == "" {
		return "", fmt.Errorf("%s --version printed no version number", name)
	}
	return version, nil
}

// harnessCheck is what Garage observed about a target's harness version.
type harnessCheck struct {
	Confirmed bool
	Observed  string
	Reason    string
}

func checkHarness(ctx context.Context, env skillsEnv, target skillTarget) harnessCheck {
	observed, err := env.ProbeHarness(ctx, target.Harness)
	switch {
	case err != nil:
		return harnessCheck{Reason: fmt.Sprintf("the installed version could not be determined (%v); confirmed with %s",
			err, target.ConfirmedVersion)}
	case observed != target.ConfirmedVersion:
		return harnessCheck{Observed: observed, Reason: fmt.Sprintf("installed version %s; behavior confirmed only with %s on %s",
			observed, target.ConfirmedVersion, target.ConfirmedOn)}
	default:
		return harnessCheck{Confirmed: true, Observed: observed}
	}
}

// resolvedTarget is a declared skill target with its directory resolved.
type resolvedTarget struct {
	Config skillTarget
	Root   string
}

// resolveSkillRoot applies a confirmed path rule: the override, when its
// environment variable is set, before the default under the home directory.
func resolveSkillRoot(rule skillPathRule, getenv func(string) string, home string) (string, error) {
	name, rest, ok := strings.Cut(strings.TrimPrefix(rule.Override, "$"), "/")
	if !ok || !strings.HasPrefix(rule.Override, "$") {
		return "", fmt.Errorf("unsupported override rule %q", rule.Override)
	}
	if value := getenv(name); value != "" {
		if !filepath.IsAbs(value) {
			return "", fmt.Errorf("%s is set to %q, which is not an absolute path", name, value)
		}
		return filepath.Join(value, filepath.FromSlash(rest)), nil
	}
	defaultRest, ok := strings.CutPrefix(rule.Default, "~/")
	if !ok {
		return "", fmt.Errorf("unsupported default rule %q", rule.Default)
	}
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("the home directory %q is not an absolute path", home)
	}
	return filepath.Join(home, filepath.FromSlash(defaultRest)), nil
}

// otherReadPaths returns the directories, besides the managed root, in which
// the harness also loads user or project skills, as established by #37. A
// same-named skill in one of them shadows or duplicates the managed copy.
func otherReadPaths(target resolvedTarget, projectDir, home string, getenv func(string) string) []string {
	project := func(p string) string { return filepath.Join(projectDir, filepath.FromSlash(p)) }
	agentsHome := filepath.Join(home, ".agents", "skills")
	var paths []string
	switch target.Config.Harness {
	case "claude-code":
		paths = []string{project(".claude/skills")}
	case "codex":
		paths = []string{agentsHome, project(".agents/skills"), project(".codex/skills")}
	case "copilot-cli":
		// Copilot reads ~/.agents/skills only when COPILOT_HOME is unset or
		// is its default location.
		if copilotHome := getenv("COPILOT_HOME"); copilotHome == "" ||
			filepath.Clean(copilotHome) == filepath.Join(home, ".copilot") {
			paths = append(paths, agentsHome)
		}
		paths = append(paths, project(".github/skills"), project(".agents/skills"), project(".claude/skills"))
	}
	var out []string
	for _, p := range paths {
		if filepath.Clean(p) != filepath.Clean(target.Root) {
			out = append(out, p)
		}
	}
	return out
}

// skillCollision is a same-named skill in another path the harness reads.
type skillCollision struct {
	Skill string
	Path  string
}

func (c skillCollision) describe(harness string) string {
	effect := "can shadow the managed copy"
	if harness == "codex" {
		effect = "is shown to Codex alongside the managed copy"
	}
	return fmt.Sprintf("%s also exists at %s, which %s", c.Skill, c.Path, effect)
}

func findCollisions(target resolvedTarget, names []string, projectDir, home string, getenv func(string) string) ([]skillCollision, error) {
	var found []skillCollision
	for _, dir := range otherReadPaths(target, projectDir, home, getenv) {
		for _, name := range names {
			p := filepath.Join(dir, name, skillEntryFile)
			_, err := os.Lstat(p)
			switch {
			case err == nil:
				found = append(found, skillCollision{Skill: name, Path: filepath.Join(dir, name)})
			case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
				// Absent, or a path component is a file: not a skill.
			default:
				return nil, fmt.Errorf("cannot inspect %s: %w", p, err)
			}
		}
	}
	return found, nil
}

// skillState classifies one canonical skill at one target.
type skillState int

const (
	stateNotInstalled skillState = iota // absent, and not in the manifest
	stateUnmanaged                      // present, but not installed by Garage
	stateMissing                        // in the manifest, but absent
	stateModified                       // installed by Garage, then changed
	stateStale                          // unmodified, from another revision
	stateCurrent                        // unmodified, from this binary's revision
)

func (s skillState) String() string {
	return [...]string{"Not installed", "Unmanaged", "Missing", "Modified", "Stale", "Current"}[s]
}

// skillInspection is the observed state of one skill directory.
type skillInspection struct {
	Name  string
	Dir   string
	State skillState
	// Digest is the observed digest of the directory, when it holds only
	// regular files and directories.
	Digest string
	// Unsupported names an entry Garage cannot back up or replace, such as a
	// symbolic link; replacement is refused while it exists.
	Unsupported string
	// Revision is the revision the manifest records for the skill.
	Revision string
}

// readSkillDir returns the regular files under dir, by slash-separated
// relative path. unsupported names the first entry that is neither a regular
// file nor a directory.
func readSkillDir(dir string) (files []skillFile, unsupported string, err error) {
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return nil
		case d.Type().IsRegular():
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			files = append(files, skillFile{Path: filepath.ToSlash(rel), Data: data})
			return nil
		default:
			if unsupported == "" {
				unsupported = p
			}
			return nil
		}
	})
	sortSkillFiles(files)
	return files, unsupported, err
}

// inspectSkill classifies the skill directory root/name against the
// manifest entry for it, if any, and the current revision.
func inspectSkill(root, name string, owned *manifestSkill, revision string) (skillInspection, error) {
	dir := filepath.Join(root, name)
	result := skillInspection{Name: name, Dir: dir}
	if owned != nil {
		result.Revision = owned.Revision
	}
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if owned != nil {
			result.State = stateMissing
		} else {
			result.State = stateNotInstalled
		}
		return result, nil
	case err != nil:
		return result, fmt.Errorf("cannot inspect %s: %w", dir, err)
	}
	if !info.IsDir() {
		result.Unsupported = dir
	} else {
		files, unsupported, err := readSkillDir(dir)
		if err != nil {
			return result, fmt.Errorf("cannot read %s: %w", dir, err)
		}
		result.Unsupported = unsupported
		if unsupported == "" {
			result.Digest = digestSkillFiles(files)
		}
	}
	switch {
	case owned == nil:
		result.State = stateUnmanaged
	case result.Digest == "" || result.Digest != owned.Digest:
		result.State = stateModified
	case owned.Revision != revision:
		result.State = stateStale
	default:
		result.State = stateCurrent
	}
	return result, nil
}

// skillManifest is Garage's per-user record of the skill targets it manages.
// It binds each installed skill to its revision and the digest of its
// installed bytes, and records which installation last owned each target.
type skillManifest struct {
	Version int              `json:"version"`
	Targets []manifestTarget `json:"targets"`
}

type manifestTarget struct {
	Harness string `json:"harness"`
	// Path is the resolved target directory; ownership is per directory.
	Path           string             `json:"path"`
	Repository     manifestRepository `json:"repository"`
	InstalledAt    string             `json:"installed_at"`
	GarageVersion  string             `json:"garage_version"`
	Revision       string             `json:"revision"`
	Development    bool               `json:"development"`
	SkillSetDigest string             `json:"skill_set_digest"`
	Skills         []manifestSkill    `json:"skills"`
}

// manifestRepository identifies the project the last installation ran from.
type manifestRepository struct {
	Key  string `json:"key,omitempty"`
	Path string `json:"path"`
}

type manifestSkill struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
	Digest   string `json:"digest"`
}

func (m *skillManifest) target(root string) *manifestTarget {
	for i := range m.Targets {
		if filepath.Clean(m.Targets[i].Path) == filepath.Clean(root) {
			return &m.Targets[i]
		}
	}
	return nil
}

func (t *manifestTarget) skill(name string) *manifestSkill {
	if t == nil {
		return nil
	}
	for i := range t.Skills {
		if t.Skills[i].Name == name {
			return &t.Skills[i]
		}
	}
	return nil
}

func userStateDir(env skillsEnv) (string, error) {
	dir, err := env.ConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate the user configuration directory: %w", err)
	}
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("the user configuration directory %q is not an absolute path", dir)
	}
	return filepath.Join(dir, userStateDirName), nil
}

// loadSkillManifest reads the manifest. A missing manifest is empty; an
// unreadable or invalid one is an error, never silently replaced.
func loadSkillManifest(path string) (skillManifest, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return skillManifest{Version: skillManifestVersion}, nil
	}
	if err != nil {
		return skillManifest{}, fmt.Errorf("cannot inspect the skill manifest %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return skillManifest{}, fmt.Errorf("refusing to read the skill manifest %s: it is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return skillManifest{}, fmt.Errorf("cannot read the skill manifest %s: %w", path, err)
	}
	var m skillManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return skillManifest{}, fmt.Errorf("the skill manifest %s is not valid: %w; correct or remove it by hand", path, err)
	}
	if m.Version != skillManifestVersion {
		return skillManifest{}, fmt.Errorf("the skill manifest %s has unsupported version %d", path, m.Version)
	}
	return m, nil
}

func saveSkillManifest(path string, m skillManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("cannot create %s: %w", filepath.Dir(path), err)
	}
	return writeFileAtomically(path, append(data, '\n'))
}

// writeFileAtomically writes content to a temporary file in path's directory
// and then replaces path with it, so path never holds partial content.
func writeFileAtomically(path string, content []byte) error {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to replace %s: it is not a regular file", path)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".garage-*.tmp")
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	tempPath := file.Name()
	if err := writeTemporaryConfig(file, content); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	if err := commitConfigReplacement(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("cannot replace %s: %w", path, err)
	}
	return nil
}

// skillsContext is everything a skills command derives before it inspects a
// target.
type skillsContext struct {
	ProjectDir string
	Repository manifestRepository
	Home       string
	Skills     []canonicalSkill
	SetDigest  string
	Targets    []resolvedTarget
	StateDir   string
	Manifest   skillManifest
}

// prepareSkills loads the valid project configuration, the embedded skills,
// the selected targets, and the manifest. It writes nothing.
func prepareSkills(ctx context.Context, env skillsEnv, selected []string) (skillsContext, error) {
	cfg, err := loadValidConfig(env.WorkDir)
	if err != nil {
		return skillsContext{}, err
	}
	var declared []skillTarget
	if cfg.Tooling != nil && cfg.Tooling.SkillTargets != nil {
		declared = *cfg.Tooling.SkillTargets
	}
	if len(declared) == 0 {
		return skillsContext{}, errors.New("the project configuration declares no skill targets; " +
			"add them with \"garage init --skill-target HARNESS@VERSION@YYYY-MM-DD\"")
	}

	home, err := env.HomeDir()
	if err != nil {
		return skillsContext{}, fmt.Errorf("cannot locate the home directory: %w", err)
	}
	var targets []resolvedTarget
	for _, t := range declared {
		if len(selected) > 0 && !containsString(selected, t.Harness) {
			continue
		}
		root, err := resolveSkillRoot(t.Path, env.Getenv, home)
		if err != nil {
			return skillsContext{}, fmt.Errorf("cannot resolve the %s skill target: %w", t.Harness, err)
		}
		targets = append(targets, resolvedTarget{Config: t, Root: root})
	}
	for _, name := range selected {
		if !containsTarget(declared, name) {
			return skillsContext{}, fmt.Errorf("--target %s is not a skill target declared in the project configuration", name)
		}
	}

	set, err := loadSkillSet(env.Skills)
	if err != nil {
		return skillsContext{}, err
	}

	repo := manifestRepository{Path: env.WorkDir}
	if identity, err := identifyRepository(ctx, env.WorkDir, env.Git); err == nil {
		repo = manifestRepository{Key: identity.Key, Path: identity.TopLevel}
	} else if !errors.Is(err, errNotRepository) {
		return skillsContext{}, err
	}

	stateDir, err := userStateDir(env)
	if err != nil {
		return skillsContext{}, err
	}
	manifest, err := loadSkillManifest(filepath.Join(stateDir, skillManifestName))
	if err != nil {
		return skillsContext{}, err
	}
	return skillsContext{
		ProjectDir: env.WorkDir,
		Repository: repo,
		Home:       home,
		Skills:     set,
		SetDigest:  skillSetDigest(set),
		Targets:    targets,
		StateDir:   stateDir,
		Manifest:   manifest,
	}, nil
}

func containsString(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}

func containsTarget(targets []skillTarget, harness string) bool {
	for _, t := range targets {
		if t.Harness == harness {
			return true
		}
	}
	return false
}

// targetReport is the verification of one target.
type targetReport struct {
	Target     resolvedTarget
	Harness    harnessCheck
	Owner      *manifestTarget
	Skills     []skillInspection
	Extras     []manifestSkill
	Collisions []skillCollision
}

// inspectTarget inspects every canonical skill, every managed skill that is
// no longer canonical, and every collision at one target. It writes nothing.
func inspectTarget(ctx context.Context, env skillsEnv, sc skillsContext, target resolvedTarget, revision string) (targetReport, error) {
	report := targetReport{Target: target, Harness: checkHarness(ctx, env, target.Config)}
	owner := sc.Manifest.target(target.Root)
	report.Owner = owner
	for _, s := range sc.Skills {
		inspection, err := inspectSkill(target.Root, s.Name, owner.skill(s.Name), revision)
		if err != nil {
			return report, err
		}
		report.Skills = append(report.Skills, inspection)
	}
	if owner != nil {
		for _, ms := range owner.Skills {
			if !containsString(skillNames(sc.Skills), ms.Name) {
				report.Extras = append(report.Extras, ms)
			}
		}
	}
	collisions, err := findCollisions(target, skillNames(sc.Skills), sc.ProjectDir, sc.Home, env.Getenv)
	if err != nil {
		return report, err
	}
	report.Collisions = collisions
	return report, nil
}

// errSkillsNotCurrent reports that verification found anything other than
// current skills without collisions.
var errSkillsNotCurrent = errors.New("installed skills are not all current")

// verifySkills inspects the selected targets and writes a report. It never
// writes to any skill target, the manifest, or the backups.
func verifySkills(ctx context.Context, env skillsEnv, selected []string, w io.Writer) error {
	sc, err := prepareSkills(ctx, env, selected)
	if err != nil {
		return err
	}
	revision := currentRevision(env.Build(), sc.SetDigest)
	fmt.Fprintf(w, "Embedded skills: revision %s\n", revision)

	problems := 0
	for _, target := range sc.Targets {
		report, err := inspectTarget(ctx, env, sc, target, revision)
		if err != nil {
			return err
		}
		writeTargetReport(w, report, sc.Repository)
		for _, s := range report.Skills {
			if s.State != stateCurrent {
				problems++
			}
		}
		problems += len(report.Extras) + len(report.Collisions)
	}
	if problems > 0 {
		return fmt.Errorf("%w: %d problem(s) found", errSkillsNotCurrent, problems)
	}
	fmt.Fprintln(w, "\nAll managed skills are Current, with no collisions.")
	return nil
}

func writeTargetReport(w io.Writer, r targetReport, current manifestRepository) {
	fmt.Fprintf(w, "\n%s  %s\n", r.Target.Config.Harness, r.Target.Root)
	if r.Harness.Confirmed {
		fmt.Fprintf(w, "  harness: %s, as confirmed on %s\n", r.Harness.Observed, r.Target.Config.ConfirmedOn)
	} else {
		fmt.Fprintf(w, "  warning: harness Unverified: %s. Its behavior is not confirmed.\n", r.Harness.Reason)
	}
	if r.Owner != nil {
		from := r.Owner.Repository.Path
		if r.Owner.Repository.Key != current.Key || r.Owner.Repository.Path != current.Path {
			from += " (another project; the last install owns this target)"
		}
		fmt.Fprintf(w, "  owner: installed from %s at %s, revision %s\n", from, r.Owner.InstalledAt, r.Owner.Revision)
	}
	for _, s := range r.Skills {
		line := fmt.Sprintf("  %-40s %s", s.Name, s.State)
		switch {
		case s.State == stateStale:
			line += fmt.Sprintf(" (installed revision %s)", s.Revision)
		case s.Unsupported != "":
			line += fmt.Sprintf(" (contains %s, which is not a regular file)", s.Unsupported)
		}
		fmt.Fprintln(w, line)
	}
	for _, e := range r.Extras {
		fmt.Fprintf(w, "  %-40s Extra (managed by Garage, but no longer a canonical skill)\n", e.Name)
	}
	for _, c := range r.Collisions {
		fmt.Fprintf(w, "  collision: %s\n", c.describe(r.Target.Config.Harness))
	}
}

// installOptions are the caller's choices for an installation.
type installOptions struct {
	Targets     []string
	Replace     []string
	Development bool
}

// skillAction is the planned change to one skill at one target.
type skillAction struct {
	Inspection skillInspection
	Files      []skillFile
	Digest     string
	Backup     bool
}

// errInstallRefused reports that the plan required something the user has
// not consented to, or that cannot be done safely. Nothing was written.
var errInstallRefused = errors.New("installation refused; nothing was changed")

// installSkills installs or updates the canonical skills at the selected
// targets. It first plans every target and refuses, writing nothing, when
// any collision exists, when a modified or unmanaged copy would be replaced
// without consent, or when a copy cannot be replaced safely.
func installSkills(ctx context.Context, env skillsEnv, opts installOptions, w io.Writer) error {
	sc, err := prepareSkills(ctx, env, opts.Targets)
	if err != nil {
		return err
	}
	names := skillNames(sc.Skills)
	for _, r := range opts.Replace {
		if !containsString(names, r) {
			return fmt.Errorf("--replace %s is not a canonical skill (canonical skills: %s)", r, strings.Join(names, ", "))
		}
	}
	revision, development, err := installRevision(env.Build(), sc.SetDigest, opts.Development)
	if err != nil {
		return err
	}
	if inside, err := isWithin(sc.StateDir, sc.ProjectDir); err != nil {
		return err
	} else if inside {
		return fmt.Errorf("refusing to keep skill state inside the project: %s is within %s", sc.StateDir, sc.ProjectDir)
	}

	type targetPlan struct {
		Report  targetReport
		Actions []skillAction
	}
	var plans []targetPlan
	var refusals []string
	for _, target := range sc.Targets {
		for _, other := range sc.Targets {
			if other.Config.Harness != target.Config.Harness && filepath.Clean(other.Root) == filepath.Clean(target.Root) {
				return fmt.Errorf("the %s and %s targets resolve to the same directory %s",
					target.Config.Harness, other.Config.Harness, target.Root)
			}
		}
		if inside, err := isWithin(sc.StateDir, target.Root); err != nil {
			return err
		} else if inside {
			return fmt.Errorf("refusing to keep skill state inside the %s target %s", target.Config.Harness, target.Root)
		}
		if info, err := os.Lstat(target.Root); err == nil && (!info.IsDir() || info.Mode()&fs.ModeSymlink != 0) {
			refusals = append(refusals, fmt.Sprintf("%s: %s is not a plain directory; Garage does not write through it",
				target.Config.Harness, target.Root))
			continue
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("cannot inspect %s: %w", target.Root, err)
		}

		report, err := inspectTarget(ctx, env, sc, target, revision)
		if err != nil {
			return err
		}
		for _, c := range report.Collisions {
			refusals = append(refusals, fmt.Sprintf("%s: %s. Resolve it by hand; there is no override.",
				target.Config.Harness, c.describe(target.Config.Harness)))
		}
		plan := targetPlan{Report: report}
		for i, s := range report.Skills {
			if s.State == stateCurrent {
				continue
			}
			action := skillAction{Inspection: s}
			switch s.State {
			case stateModified, stateUnmanaged:
				if s.Unsupported != "" {
					refusals = append(refusals, fmt.Sprintf("%s: %s is %s and contains %s, which is not a regular file; "+
						"Garage cannot back it up or replace it. Resolve it by hand.",
						target.Config.Harness, s.Dir, strings.ToLower(s.State.String()), s.Unsupported))
					continue
				}
				if !containsString(opts.Replace, s.Name) {
					refusals = append(refusals, fmt.Sprintf("%s: %s is %s. To replace it, after a backup, pass --replace %s.",
						target.Config.Harness, s.Dir, strings.ToLower(s.State.String()), s.Name))
					continue
				}
				action.Backup = true
			}
			files, err := renderSkill(sc.Skills[i], revision)
			if err != nil {
				return err
			}
			action.Files = files
			action.Digest = digestSkillFiles(files)
			plan.Actions = append(plan.Actions, action)
		}
		plans = append(plans, plan)
	}
	if len(refusals) > 0 {
		for _, r := range refusals {
			fmt.Fprintf(w, "refused: %s\n", r)
		}
		return errInstallRefused
	}

	now := env.Now().UTC()
	manifestPath := filepath.Join(sc.StateDir, skillManifestName)
	backupRoot := uniquePath(filepath.Join(sc.StateDir, skillBackupsDirName, now.Format("20060102T150405Z")))
	fmt.Fprintf(w, "Installing skills at revision %s\n", revision)
	if development {
		fmt.Fprintln(w, "warning: this is a development install. Its skills are marked unversioned, bound only to their skill-set digest.")
	}

	for _, plan := range plans {
		target := plan.Report.Target
		fmt.Fprintf(w, "\n%s  %s\n", target.Config.Harness, target.Root)
		if !plan.Report.Harness.Confirmed {
			fmt.Fprintf(w, "  warning: harness Unverified: %s. Its behavior is not confirmed.\n", plan.Report.Harness.Reason)
		}
		if owner := plan.Report.Owner; owner != nil &&
			(owner.Repository.Key != sc.Repository.Key || owner.Repository.Path != sc.Repository.Path) {
			fmt.Fprintf(w, "  note: this target was last installed from %s; this install now owns it\n", owner.Repository.Path)
		}
		if err := os.MkdirAll(target.Root, 0o755); err != nil {
			return fmt.Errorf("cannot create %s: %w", target.Root, err)
		}

		// The manifest starts from what this target holds now: Current
		// skills, and every other skill Garage already owns there. It is
		// saved after each skill is written, so an interrupted install
		// never leaves a written skill unrecorded.
		entry := manifestTarget{
			Harness:        target.Config.Harness,
			Path:           target.Root,
			Repository:     sc.Repository,
			InstalledAt:    now.Format(time.RFC3339),
			GarageVersion:  env.GarageVersion,
			Revision:       revision,
			Development:    development,
			SkillSetDigest: sc.SetDigest,
		}
		for _, s := range plan.Report.Skills {
			switch {
			case s.State == stateCurrent:
				entry.Skills = append(entry.Skills, manifestSkill{Name: s.Name, Revision: s.Revision, Digest: s.Digest})
			case plan.Report.Owner.skill(s.Name) != nil:
				entry.Skills = append(entry.Skills, *plan.Report.Owner.skill(s.Name))
			}
		}
		entry.Skills = append(entry.Skills, plan.Report.Extras...)
		save := func() error {
			sort.Slice(entry.Skills, func(i, j int) bool { return entry.Skills[i].Name < entry.Skills[j].Name })
			setManifestTarget(&sc.Manifest, entry)
			return saveSkillManifest(manifestPath, sc.Manifest)
		}

		if len(plan.Actions) == 0 && !sameOwnership(plan.Report.Owner, entry) {
			if err := save(); err != nil {
				return err
			}
		}
		for _, action := range plan.Actions {
			result, err := applySkillAction(action, target, backupRoot)
			if err != nil {
				return err
			}
			recorded := manifestSkill{Name: action.Inspection.Name, Revision: revision, Digest: action.Digest}
			replaced := false
			for i := range entry.Skills {
				if entry.Skills[i].Name == recorded.Name {
					entry.Skills[i], replaced = recorded, true
				}
			}
			if !replaced {
				entry.Skills = append(entry.Skills, recorded)
			}
			if err := save(); err != nil {
				return fmt.Errorf("%s was written, but the manifest could not be saved: %w", action.Inspection.Dir, err)
			}
			fmt.Fprintf(w, "  %-40s %s\n", action.Inspection.Name, result)
		}
		for _, s := range plan.Report.Skills {
			if s.State == stateCurrent {
				fmt.Fprintf(w, "  %-40s Current\n", s.Name)
			}
		}
		for _, e := range plan.Report.Extras {
			fmt.Fprintf(w, "  %-40s Extra (managed by Garage, no longer canonical; left in place)\n", e.Name)
		}
	}
	fmt.Fprintf(w, "\nManifest: %s\n", manifestPath)
	return nil
}

// sameOwnership reports whether entry records exactly what existing already
// records, apart from when and by which garage version it was written.
func sameOwnership(existing *manifestTarget, entry manifestTarget) bool {
	if existing == nil {
		return false
	}
	a, b := *existing, entry
	a.InstalledAt, b.InstalledAt = "", ""
	a.GarageVersion, b.GarageVersion = "", ""
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(ja, jb)
}

// uniquePath returns base, or base with the smallest "-N" suffix that does
// not exist yet.
func uniquePath(base string) string {
	candidate := base
	for n := 1; ; n++ {
		if _, err := os.Lstat(candidate); errors.Is(err, fs.ErrNotExist) {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, n)
	}
}

func setManifestTarget(m *skillManifest, entry manifestTarget) {
	if existing := m.target(entry.Path); existing != nil {
		*existing = entry
		return
	}
	m.Targets = append(m.Targets, entry)
	sort.Slice(m.Targets, func(i, j int) bool { return m.Targets[i].Path < m.Targets[j].Path })
}

// errSkillChanged reports that a skill directory changed after it was
// inspected; it is left as it is.
var errSkillChanged = errors.New("the skill changed while garage was running")

// applySkillAction writes one skill. An existing directory is first checked
// again against the digest observed when it was planned, and is backed up
// when the action requires it. Files in the directory that are not part of
// the skill are removed only after that check, and after the backup.
func applySkillAction(action skillAction, target resolvedTarget, backupRoot string) (string, error) {
	s := action.Inspection
	exists := s.State != stateNotInstalled && s.State != stateMissing
	if exists {
		files, unsupported, err := readSkillDir(s.Dir)
		if err != nil {
			return "", fmt.Errorf("cannot read %s: %w", s.Dir, err)
		}
		if unsupported != "" || digestSkillFiles(files) != s.Digest {
			return "", fmt.Errorf("%w: %s was not changed; run the install again", errSkillChanged, s.Dir)
		}
		if action.Backup {
			backupDir := filepath.Join(backupRoot, target.Config.Harness, s.Name)
			if err := backupSkill(files, backupDir); err != nil {
				return "", err
			}
		}
	} else if err := os.Mkdir(s.Dir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create %s: %w", s.Dir, err)
	}

	wanted := map[string]bool{}
	for _, f := range action.Files {
		wanted[f.Path] = true
		dest := filepath.Join(s.Dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return "", fmt.Errorf("cannot create %s: %w", filepath.Dir(dest), err)
		}
		if err := writeFileAtomically(dest, f.Data); err != nil {
			return "", err
		}
	}
	if exists {
		files, _, err := readSkillDir(s.Dir)
		if err != nil {
			return "", fmt.Errorf("cannot read %s: %w", s.Dir, err)
		}
		for _, f := range files {
			if !wanted[f.Path] {
				if err := os.Remove(filepath.Join(s.Dir, filepath.FromSlash(f.Path))); err != nil {
					return "", fmt.Errorf("cannot remove %s from %s: %w", f.Path, s.Dir, err)
				}
			}
		}
	}

	files, unsupported, err := readSkillDir(s.Dir)
	if err != nil || unsupported != "" || digestSkillFiles(files) != action.Digest {
		return "", fmt.Errorf("%s does not hold the expected skill after writing it", s.Dir)
	}

	switch s.State {
	case stateNotInstalled:
		return "installed", nil
	case stateMissing:
		return "reinstalled (was Missing)", nil
	case stateStale:
		return "updated (was Stale)", nil
	default:
		return fmt.Sprintf("replaced (was %s; backup at %s)", s.State,
			filepath.Join(backupRoot, target.Config.Harness, s.Name)), nil
	}
}

// backupSkill copies a skill's files to dir, which must not exist yet.
func backupSkill(files []skillFile, dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return fmt.Errorf("cannot create %s: %w", filepath.Dir(dir), err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create the backup %s: %w", dir, err)
	}
	for _, f := range files {
		dest := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
			return fmt.Errorf("cannot back up %s: %w", f.Path, err)
		}
		if err := os.WriteFile(dest, f.Data, 0o600); err != nil {
			return fmt.Errorf("cannot back up %s: %w", f.Path, err)
		}
	}
	return nil
}
