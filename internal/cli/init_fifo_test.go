//go:build unix

package cli

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestInitRejectsFIFOWithoutBlocking(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "force"}[force], func(t *testing.T) {
			projectDir := t.TempDir()
			garageDir := filepath.Join(projectDir, garageDirName)
			if err := syscall.Mkdir(garageDir, 0o755); err != nil {
				t.Fatalf("create %s: %v", garageDirName, err)
			}
			if err := syscall.Mkfifo(configPathIn(projectDir), 0o644); err != nil {
				t.Skipf("FIFOs are not available: %v", err)
			}

			result := make(chan error, 1)
			go func() {
				_, err := initProject(projectDir, force)
				result <- err
			}()

			select {
			case err := <-result:
				if err == nil {
					t.Fatal("expected the FIFO destination to be rejected")
				}
				if !strings.Contains(err.Error(), "not a regular file") {
					t.Errorf("error = %q, want an actionable file-type error", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("initialization blocked while inspecting a FIFO")
			}
		})
	}
}
