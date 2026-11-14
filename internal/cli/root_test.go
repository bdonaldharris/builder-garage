package cli

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

// runCLI executes the garage command tree with args and returns what it wrote
// to stdout and stderr.
func runCLI(t *testing.T, version string, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	root := NewRootCommand(version)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	// A nil slice would make Cobra fall back to the test binary's own arguments.
	root.SetArgs(append([]string{}, args...))

	err = root.Execute()
	return out.String(), errOut.String(), err
}

func TestHelpDescribesPurposeAndCommands(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}, {}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, _, err := runCLI(t, "dev", args...)
			if err != nil {
				t.Fatalf("garage %v: unexpected error: %v", args, err)
			}

			for _, want := range []string{
				"Builder Garage is an agentic engineering system for builders",
				"Usage:",
				"garage [command]",
				"Available Commands:",
				"config ",
				"init ",
				"record ",
				"status ",
				"version ",
				"Flags:",
				"-h, --help",
			} {
				if !strings.Contains(stdout, want) {
					t.Errorf("help output does not contain %q:\n%s", want, stdout)
				}
			}

			if strings.Contains(stdout, "completion") {
				t.Errorf("help output lists an unimplemented completion command:\n%s", stdout)
			}
		})
	}
}

// TestHelpSummaryNamesCapabilities checks that the summary garage --help prints
// names what the CLI does today, consistent with the README's Project status.
func TestHelpSummaryNamesCapabilities(t *testing.T) {
	stdout, _, err := runCLI(t, "dev", "--help")
	if err != nil {
		t.Fatalf("garage --help: unexpected error: %v", err)
	}
	// Compare with whitespace collapsed, so line wrapping cannot split a phrase.
	summary := strings.Join(strings.Fields(stdout), " ")

	for _, want := range []string{
		"initializes, upgrades, validates, shows, and changes a project's Builder Garage configuration",
		"records private developer-local workflow events",
		"installs and verifies the canonical skills and the manuals a project adopts at a project's harness targets",
		"shows the read-only workflow status of an issue or pull request from GitHub",
		"reports its own version",
	} {
		if !strings.Contains(summary, want) {
			t.Errorf("help summary does not contain %q:\n%s", want, stdout)
		}
	}
}

func TestRegisteredCommands(t *testing.T) {
	var got []string
	for _, cmd := range NewRootCommand("dev").Commands() {
		got = append(got, cmd.Name())
	}
	slices.Sort(got)

	want := []string{"config", "init", "record", "skills", "status", "version"}
	if !slices.Equal(got, want) {
		t.Fatalf("registered commands = %v, want %v", got, want)
	}
}

func TestSubcommandHelp(t *testing.T) {
	stdout, _, err := runCLI(t, "dev", "init", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"garage init [flags]", ".garage", "--discovery", "--workflow-owner", "--skill-target"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("init help does not contain %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "--force") {
		t.Errorf("init help still offers --force:\n%s", stdout)
	}
}

func TestUnknownCommandFails(t *testing.T) {
	stdout, stderr, err := runCLI(t, "dev", "bogus")
	if err == nil {
		t.Fatal("expected an error for an unknown command")
	}
	if !strings.Contains(err.Error(), `unknown command "bogus"`) {
		t.Errorf("error = %q, want it to name the unknown command", err)
	}
	if !strings.Contains(stderr, "Error:") {
		t.Errorf("stderr = %q, want the error to be reported there", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want it empty on failure", stdout)
	}
}

func TestVersionCommand(t *testing.T) {
	for _, version := range []string{"dev", "1.2.3"} {
		t.Run(version, func(t *testing.T) {
			stdout, stderr, err := runCLI(t, version, "version")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if want := "garage version " + version + "\n"; stdout != want {
				t.Errorf("stdout = %q, want %q", stdout, want)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want it empty", stderr)
			}
		})
	}
}

func TestVersionRejectsArguments(t *testing.T) {
	stdout, _, err := runCLI(t, "dev", "version", "extra")
	if err == nil {
		t.Fatal("expected an error for an unexpected argument")
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want it empty on failure", stdout)
	}
}
