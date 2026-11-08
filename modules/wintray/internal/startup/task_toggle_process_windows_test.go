//go:build windows

package startup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTaskInventoryReaderProcess(t *testing.T) {
	if len(os.Args) != 5 || os.Args[2] != "--" || os.Args[3] != "inventory-probe" {
		return
	}
	dir := os.Args[4]
	if err := os.WriteFile(filepath.Join(dir, "ready"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	// The child has its own watchdog: a panicking parent cannot run cleanup.
	deadline := time.Now().Add(75 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "stop")); err == nil {
			return
		}
		if _, err := readTaskInventory(runSchtasks); err != nil {
			// The full query can include unrelated machine tasks. Keep only
			// the command failure, not its complete XML, in test output.
			t.Fatal(strings.SplitN(err.Error(), ": <Tasks>", 2)[0])
		}
	}
	t.Fatal("inventory probe was not released by its parent")
}

func TestTaskInventoryDuringCrossProcessLogonToggle(t *testing.T) {
	if os.Getenv("WINTRAY_STARTUP_LIVE_TEST") != "1" {
		t.Skip("explicit isolated Task Scheduler check")
	}
	task, err := NewLogonTask()
	if err != nil {
		t.Fatal(err)
	}
	task.name = fmt.Sprintf("WinOneCrossProcessToggleTest-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() {
		if err := task.Remove(); err != nil {
			t.Error(err)
		}
	})
	dir := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, self, "-test.run=^TestTaskInventoryReaderProcess$", "--", "inventory-probe", dir)
	var output strings.Builder
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.WriteFile(filepath.Join(dir, "stop"), nil, 0600); err != nil {
			t.Error(err)
		}
		if err := cmd.Wait(); err != nil {
			t.Errorf("separate inventory process failed: %v\n%s", err, output.String())
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("inventory process not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for i := 0; i < 20; i++ {
		if err := task.Sync(filepath.Join(systemDir(), "cmd.exe"), "/c exit"); err != nil {
			t.Fatal(err)
		}
		if err := task.Remove(); err != nil {
			t.Fatal(err)
		}
	}
}
