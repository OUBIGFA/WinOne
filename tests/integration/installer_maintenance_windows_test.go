//go:build windows

package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A maintenance executable must never become a normal Host if its arguments
// are lost or replaced. Drive the real binary, not only its flag parser.
func TestDedicatedMaintenanceRejectsRuntimeModesWithoutOpeningAProfile(t *testing.T) {
	executable := os.Getenv("WINONE_INSTALLER_MAINTENANCE_EXE")
	if executable == "" {
		t.Skip("set WINONE_INSTALLER_MAINTENANCE_EXE for the dedicated helper")
	}
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	for _, args := range [][]string{
		nil,
		{"--prepare-uninstall"},
		{"--headless", "--data-dir", filepath.Join(root, "headless")},
		{"--autorun", "--data-dir", filepath.Join(root, "autorun")},
		{"--check-startup", "nonce"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		command := exec.CommandContext(ctx, executable, args...)
		output, err := command.CombinedOutput()
		timedOut := ctx.Err()
		cancel()
		if timedOut != nil {
			t.Fatalf("maintenance started a runtime instead of rejecting %v: %v; %s", args, timedOut, output)
		}
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			t.Fatalf("maintenance did not reject %v with exit 1: %v; %s", args, err, output)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("rejected maintenance command opened a profile: %v; entries=%v", err, entries)
	}
}
