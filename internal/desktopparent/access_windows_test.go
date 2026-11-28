//go:build windows

package desktopparent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// The child exercises the actual permissions Chromium/Crashpad needs. The
// explicit elevated run is necessary: a normal creator cannot reproduce the
// administrator-default security descriptor that caused the original failure.
func TestDesktopChildOwnProcessAccess(t *testing.T) {
	if os.Getenv("WINONE_PARENT_ACCESS_HELPER") == "child" {
		handle, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ|windows.PROCESS_DUP_HANDLE, false, windows.GetCurrentProcessId())
		if err != nil {
			t.Fatalf("child cannot open its own process for crash handler/IPC: %v", err)
		}
		defer windows.CloseHandle(handle)
		var token windows.Token
		if err := windows.OpenProcessToken(handle, windows.TOKEN_QUERY, &token); err != nil {
			t.Fatalf("child cannot query its own token: %v", err)
		}
		defer token.Close()
		if token.IsElevated() {
			t.Fatal("desktop child unexpectedly elevated")
		}
		thread, err := windows.OpenThread(windows.THREAD_QUERY_INFORMATION|windows.THREAD_GET_CONTEXT, false, windows.GetCurrentThreadId())
		if err != nil {
			t.Fatalf("child cannot open its own thread: %v", err)
		}
		windows.CloseHandle(thread)
		return
	}
	parent, err := Open()
	if errors.Is(err, ErrDesktopUnavailable) {
		t.Skip("no desktop shell in this session")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if parent.Elevated {
		t.Skip("requires a normal desktop shell")
	}
	elevated := windows.GetCurrentProcessToken().IsElevated()
	if os.Getenv("WINONE_PARENT_ACCESS_REQUIRE_ELEVATED") == "1" && !elevated {
		t.Fatal("cross-integrity check was not elevated")
	}
	t.Logf("creator elevated=%v, desktop elevated=%v", elevated, parent.Elevated)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDesktopChildOwnProcessAccess$", "-test.v")
	command.Env = append(os.Environ(), "WINONE_PARENT_ACCESS_HELPER=child")
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	parent.Apply(command.SysProcAttr)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("child: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}
