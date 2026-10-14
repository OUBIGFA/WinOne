//go:build windows

package processlifetime

import (
	"bytes"
	"context"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestExitAfterCleanupChild(t *testing.T) {
	mode := os.Getenv("WINONE_EXIT_CHILD")
	if mode == "" {
		return
	}
	if fixture := os.Getenv("WINONE_EXIT_FIXTURE_DLL"); fixture != "" {
		if _, err := windows.LoadLibrary(fixture); err != nil {
			os.Exit(3)
		}
	}
	// Model run returning: its cleanup must finish before the final exit.
	func() { defer os.WriteFile(os.Getenv("WINONE_EXIT_MARKER"), []byte("saved"), 0600) }()
	if mode == "ordinary" {
		os.Exit(17)
	}
	ExitAfterCleanup(17)
}

func TestExitAfterCleanupPreservesSavedStateAndExitCode(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "saved.txt")
	run := func(mode string) []byte {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestExitAfterCleanupChild$")
		child.Env = append(os.Environ(), "WINONE_EXIT_CHILD="+mode, "WINONE_EXIT_MARKER="+marker)
		output, err := child.CombinedOutput()
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 17 {
			t.Fatalf("%s exit code was not preserved: %v; output=%s", mode, err, output)
		}
		return output
	}
	if os.Getenv("WINONE_EXIT_FIXTURE_DLL") != "" {
		if !bytes.Contains(run("ordinary"), []byte("native-detach")) {
			t.Fatal("the native DLL fixture did not observe ordinary Windows exit")
		}
	}
	if bytes.Contains(run("completed"), []byte("native-detach")) {
		t.Fatal("completed application invoked third-party DLL detach")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "saved" {
		t.Fatal("final exit interrupted application cleanup", err)
	}
}
