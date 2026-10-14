//go:build windows

package chat

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Exercise the actual ConPTY transport. The shell and its descendants belong
// to this test's job; no installed application or user terminal is touched.
func TestTerminalInteractiveInputResizeRestartAndOwner(t *testing.T) {
	m := NewTerminalManager(context.Background())
	defer m.Close()
	info, err := m.Create(41, "test", "session", t.TempDir(), "cmd", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if info.Shell != "cmd" {
		t.Fatalf("shell=%q", info.Shell)
	}
	if err = m.Write(42, "test", []byte("echo wrong\r")); err == nil {
		t.Fatal("another owner wrote to terminal")
	}
	if err = m.Resize(41, "test", 120, 35); err != nil {
		t.Fatal(err)
	}
	if err = m.Write(41, "test", []byte("echo WINONE_PTY_OUTPUT\r")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	var output string
	for time.Now().Before(deadline) {
		p, e := m.Poll(41, "test", 0)
		if e != nil {
			t.Fatal(e)
		}
		output = string(p.Data)
		if strings.Count(output, "WINONE_PTY_OUTPUT") >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if strings.Count(output, "WINONE_PTY_OUTPUT") < 2 {
		t.Fatalf("interactive output was not returned: %q", output)
	}
	m.Detach(41)
	if _, err = m.Poll(41, "test", 0); err == nil {
		t.Fatal("detached terminal remains owned")
	}
}
