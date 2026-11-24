//go:build windows

package orchestrator

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"winone/internal/desktopparent"
	"winone/modules/wintray/internal/config"
)

func stubDesktopParent(t *testing.T, open func() (*desktopparent.Parent, error), elevated bool) {
	t.Helper()
	originalOpen, originalElevated := openDesktopParent, processElevated
	openDesktopParent = open
	processElevated = func() bool { return elevated }
	t.Cleanup(func() { openDesktopParent, processElevated = originalOpen, originalElevated })
}

func TestLaunchUnderDesktopFallsBackOnlyWhereTheDesktopCannotBeUsed(t *testing.T) {
	started := &startedProcess{}
	for _, tc := range []struct {
		name      string
		open      func() (*desktopparent.Parent, error)
		elevated  bool
		firstErr  error
		wantCalls []bool // whether each attempt received a parent
		wantErr   error
	}{
		{name: "desktop unavailable", open: func() (*desktopparent.Parent, error) { return nil, desktopparent.ErrDesktopUnavailable }, wantCalls: []bool{false}},
		{name: "other account", open: func() (*desktopparent.Parent, error) { return nil, desktopparent.ErrDifferentAccount }, wantCalls: []bool{false}},
		{name: "desktop parent", open: func() (*desktopparent.Parent, error) { return &desktopparent.Parent{}, nil }, wantCalls: []bool{true}},
		{name: "requires administrator from elevated WinOne", open: func() (*desktopparent.Parent, error) { return &desktopparent.Parent{}, nil }, elevated: true, firstErr: windows.ERROR_ELEVATION_REQUIRED, wantCalls: []bool{true, false}},
		{name: "requires administrator from standard WinOne", open: func() (*desktopparent.Parent, error) { return &desktopparent.Parent{}, nil }, firstErr: windows.ERROR_ELEVATION_REQUIRED, wantCalls: []bool{true}, wantErr: windows.ERROR_ELEVATION_REQUIRED},
		{name: "ordinary failure is not retried", open: func() (*desktopparent.Parent, error) { return &desktopparent.Parent{}, nil }, elevated: true, firstErr: os.ErrNotExist, wantCalls: []bool{true}, wantErr: os.ErrNotExist},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubDesktopParent(t, tc.open, tc.elevated)
			var calls []bool
			process, err := launchUnderDesktop(func(parent *desktopparent.Parent) (*startedProcess, error) {
				calls = append(calls, parent != nil)
				if len(calls) == 1 && tc.firstErr != nil {
					return nil, tc.firstErr
				}
				return started, nil
			})
			if len(calls) != len(tc.wantCalls) {
				t.Fatalf("attempts = %v, want %v", calls, tc.wantCalls)
			}
			for i := range calls {
				if calls[i] != tc.wantCalls[i] {
					t.Fatalf("attempts = %v, want %v", calls, tc.wantCalls)
				}
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || process != nil {
					t.Fatalf("got process=%v err=%v, want %v", process, err, tc.wantErr)
				}
				return
			}
			if err != nil || process != started {
				t.Fatalf("got process=%v err=%v", process, err)
			}
		})
	}
}

// Real launches of this test binary: GUI-style and native console routes must
// both leave WinTray's process tree and run under the desktop shell.
func TestManagedProgramsStartUnderDesktopShell(t *testing.T) {
	parent, err := desktopparent.Open()
	if errors.Is(err, desktopparent.ErrDesktopUnavailable) {
		t.Skip("no desktop shell in this session")
	}
	if err != nil {
		t.Fatal(err)
	}
	shellPID, err := windows.GetProcessId(parent.Handle())
	parent.Close()
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The helper waits until this file exists; it is killed before that.
	release := filepath.Join(t.TempDir(), "release")
	for _, mode := range []launchMode{launchNoWindow, launchHiddenConsole} {
		process, err := startConfiguredProcess(config.ManagedAppEntry{ExePath: self, Args: runnerHelperArgs("wait", release)}, mode)
		if err != nil {
			t.Fatalf("mode %d: %v", mode, err)
		}
		got := desktopTestParentPID(t, uint32(process.Process.Pid))
		_ = process.Process.Kill()
		_ = process.Wait()
		if got != shellPID {
			t.Fatalf("mode %d: parent pid = %d, want desktop shell %d (WinTray pid %d)", mode, got, shellPID, os.Getpid())
		}
	}
}

func desktopTestParentPID(t *testing.T, pid uint32) uint32 {
	t.Helper()
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(snapshot)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ProcessID == pid {
			return entry.ParentProcessID
		}
	}
	t.Fatalf("process %d not found", pid)
	return 0
}
