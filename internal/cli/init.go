package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

const (
	// garageDirName is the directory "garage init" creates in a project.
	garageDirName = ".garage"
	// configFileName is the configuration file inside garageDirName.
	configFileName = "config.yaml"
	// configSchemaVersion identifies the layout of the configuration file.
	// Increment it when the layout changes incompatibly.
	configSchemaVersion = 1

	configHeader = "# Builder Garage project configuration.\n" +
		"# Created by \"garage init\". schema_version identifies the layout of this file.\n"
)

// errConfigExists reports that a configuration file is already present and
// was left untouched.
var errConfigExists = errors.New("configuration already exists")

// projectConfig is the content of the project configuration file. It records
// only values that have an established purpose.
type projectConfig struct {
	SchemaVersion int `yaml:"schema_version"`
}

func newInitCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Builder Garage in the current directory",
		Long: `Initialize Builder Garage in the current project directory.

Creates a .garage directory containing config.yaml, a minimal configuration
file that records the configuration schema version. Other files in the project
are not modified.

An existing configuration file is never overwritten unless --force is given.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("cannot determine the current directory: %w", err)
			}

			overwritten, err := initProject(projectDir, force)
			if err != nil {
				return err
			}

			action := "created"
			if overwritten {
				action = "overwrote"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(),
				"Initialized Builder Garage in %s\n  %s %s (schema version %d)\n",
				projectDir, action, filepath.Join(garageDirName, configFileName), configSchemaVersion)
			return err
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing configuration file")

	return cmd
}

// initProject creates the .garage directory and its configuration file inside
// projectDir. An existing configuration file is replaced only when force is
// true, in which case overwritten is reported as true; otherwise it is left
// untouched and an error wrapping errConfigExists is returned.
//
// The implementation repeats path and directory-identity checks around
// sensitive operations, but Go's portable path-based filesystem APIs cannot
// eliminate every race with a hostile process concurrently replacing parent
// directory entries. The final destination is never opened for forced writes:
// a completed same-directory temporary file replaces its directory entry.
func initProject(projectDir string, force bool) (overwritten bool, err error) {
	garageDir := filepath.Join(projectDir, garageDirName)
	configPath := filepath.Join(garageDir, configFileName)

	content, err := renderConfig()
	if err != nil {
		return false, err
	}

	garageInfo, err := ensureGarageDirectory(garageDir)
	if err != nil {
		return false, err
	}

	exists, err := inspectConfigDestination(configPath)
	if err != nil {
		return false, err
	}
	if exists && !force {
		return false, configExistsError(configPath)
	}
	overwritten = exists

	if force {
		if err := replaceConfiguration(
			configPath,
			content,
			garageInfo,
			prepareTemporaryConfig,
			commitConfigReplacement,
		); err != nil {
			return false, err
		}
		return overwritten, nil
	}

	if err := createConfigurationExclusive(configPath, content, garageDir, garageInfo); err != nil {
		return false, err
	}
	return false, nil
}

func ensureGarageDirectory(garageDir string) (fs.FileInfo, error) {
	info, err := os.Lstat(garageDir)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.Mkdir(garageDir, 0o755); err != nil {
			if !errors.Is(err, fs.ErrExist) {
				return nil, fmt.Errorf("cannot create the %s directory: %w", garageDirName, err)
			}
		}
		info, err = os.Lstat(garageDir)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot inspect the %s directory: %w", garageDirName, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to use %s: the directory is a symbolic link", garageDir)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("cannot create the %s directory: %s already exists and is not a directory",
			garageDirName, garageDir)
	}
	return info, nil
}

func verifyGarageDirectory(garageDir string, expected fs.FileInfo) error {
	current, err := os.Lstat(garageDir)
	if err != nil {
		return fmt.Errorf("cannot verify the %s directory: %w", garageDirName, err)
	}
	if current.Mode()&fs.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(expected, current) {
		return fmt.Errorf("refusing to write configuration: the %s directory changed during initialization",
			garageDirName)
	}
	return nil
}

func inspectConfigDestination(configPath string) (bool, error) {
	info, err := os.Lstat(configPath)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cannot inspect the configuration destination: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return false, fmt.Errorf(
			"refusing to write configuration: %s is a symbolic link; remove it and retry",
			configPath,
		)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf(
			"refusing to write configuration: %s is not a regular file (%s); remove it and retry",
			configPath,
			info.Mode().Type(),
		)
	}
	return true, nil
}

func configExistsError(configPath string) error {
	return fmt.Errorf("%w: %s was left unchanged (use --force to overwrite it)",
		errConfigExists, configPath)
}

func createConfigurationExclusive(
	configPath string,
	content []byte,
	garageDir string,
	garageInfo fs.FileInfo,
) error {
	if err := verifyGarageDirectory(garageDir, garageInfo); err != nil {
		return err
	}

	// O_EXCL preserves ordinary initialization's create-only contract and, in
	// combination with O_CREATE, does not follow a final-component symlink.
	file, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			exists, inspectErr := inspectConfigDestination(configPath)
			if inspectErr != nil {
				return inspectErr
			}
			if exists {
				return configExistsError(configPath)
			}
		}
		return fmt.Errorf("cannot create the configuration file: %w", err)
	}

	createdInfo, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return fmt.Errorf("cannot inspect the new configuration file: %w", statErr)
	}

	if err := verifyGarageDirectory(garageDir, garageInfo); err != nil {
		_ = file.Close()
		removeIfSameFile(configPath, createdInfo)
		return err
	}

	_, writeErr := file.Write(content)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		removeIfSameFile(configPath, createdInfo)
		return fmt.Errorf("cannot write the configuration file: %w", err)
	}
	return nil
}

type temporaryConfigPreparer func(string, []byte) (string, error)
type configReplacementCommitter func(string, string) error

func replaceConfiguration(
	configPath string,
	content []byte,
	garageInfo fs.FileInfo,
	prepare temporaryConfigPreparer,
	commit configReplacementCommitter,
) error {
	garageDir := filepath.Dir(configPath)
	if err := verifyGarageDirectory(garageDir, garageInfo); err != nil {
		return err
	}

	tempPath, err := prepare(garageDir, content)
	if err != nil {
		return fmt.Errorf("cannot prepare the replacement configuration: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	if err := verifyGarageDirectory(garageDir, garageInfo); err != nil {
		return err
	}
	if _, err := inspectConfigDestination(configPath); err != nil {
		return err
	}
	if err := commit(tempPath, configPath); err != nil {
		return fmt.Errorf("cannot replace the configuration file: %w", err)
	}
	committed = true
	return nil
}

func prepareTemporaryConfig(garageDir string, content []byte) (string, error) {
	return prepareTemporaryConfigUsing(garageDir, content, writeTemporaryConfig)
}

func prepareTemporaryConfigUsing(
	garageDir string,
	content []byte,
	write func(*os.File, []byte) error,
) (string, error) {
	file, err := os.CreateTemp(garageDir, "."+configFileName+".tmp-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	keep := false
	defer func() {
		if !keep {
			_ = file.Close()
			_ = os.Remove(path)
		}
	}()

	if err := write(file, content); err != nil {
		return "", err
	}
	keep = true
	return path, nil
}

func writeTemporaryConfig(file *os.File, content []byte) error {
	_, writeErr := file.Write(content)
	chmodErr := file.Chmod(0o644)
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, chmodErr, syncErr, closeErr)
}

func removeIfSameFile(path string, expected fs.FileInfo) {
	if expected == nil {
		return
	}
	current, err := os.Lstat(path)
	if err != nil || current.Mode()&fs.ModeSymlink != 0 {
		return
	}
	if os.SameFile(expected, current) {
		_ = os.Remove(path)
	}
}

// renderConfig returns the YAML content of a new configuration file.
func renderConfig() ([]byte, error) {
	body, err := yaml.Marshal(projectConfig{SchemaVersion: configSchemaVersion})
	if err != nil {
		return nil, fmt.Errorf("cannot encode the configuration: %w", err)
	}
	return append([]byte(configHeader), body...), nil
}
