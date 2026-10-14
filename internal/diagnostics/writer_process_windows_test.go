//go:build windows

package diagnostics

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnifiedLogChildWriter(t *testing.T) {
	directory := os.Getenv("WINONE_TEST_LOG_DIRECTORY")
	if directory == "" {
		t.Skip("owned child only")
	}
	for i := 0; i < 30; i++ {
		if err := Record(directory, Event{Source: "capture", Event: "concurrent", Message: fmt.Sprintf("%d-%d", os.Getpid(), i)}); err != nil {
			t.Fatal(err)
		}
	}
}
func TestUnifiedLogMultipleProcessesPreserveEveryRecord(t *testing.T) {
	directory := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	children := make([]*exec.Cmd, 3)
	for i := range children {
		children[i] = exec.Command(exe, "-test.run=^TestUnifiedLogChildWriter$", "-test.timeout=15s")
		children[i].Env = append(os.Environ(), "WINONE_TEST_LOG_DIRECTORY="+directory)
		if err := children[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, child := range children {
		if err := child.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(readFile(t, filepath.Join(directory, "logs", "host.log"))), "\n") {
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if seen[event.Message] {
			t.Fatal("duplicate record")
		}
		seen[event.Message] = true
	}
	if len(seen) != 90 {
		t.Fatalf("lost records: %d", len(seen))
	}
}
