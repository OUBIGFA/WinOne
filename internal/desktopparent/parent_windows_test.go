//go:build windows

package desktopparent

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The child runs only this test binary; Explorer itself is opened read-only
// as a parent and is never signalled or changed.
func TestApplyStartsChildUnderDesktopShell(t *testing.T) {
	if os.Getenv("WINONE_DESKTOP_PARENT_CHILD") == "1" {
		time.Sleep(3 * time.Second)
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
	shellPID, err := windows.GetProcessId(parent.Handle())
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestApplyStartsChildUnderDesktopShell$")
	command.Env = append(os.Environ(), "WINONE_DESKTOP_PARENT_CHILD=1")
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	parent.Apply(command.SysProcAttr)
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	if got := parentProcessID(t, uint32(command.Process.Pid)); got != shellPID {
		t.Fatalf("child parent pid = %d, want desktop shell %d", got, shellPID)
	}
	if elevated(t, uint32(command.Process.Pid)) != parent.Elevated {
		t.Fatal("child did not receive the desktop shell's elevation")
	}
}

func TestStartupInfoNamesParentForCreateProcess(t *testing.T) {
	parent, err := Open()
	if errors.Is(err, ErrDesktopUnavailable) {
		t.Skip("no desktop shell in this session")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	info, release, err := parent.StartupInfo(windows.StartupInfo{Flags: windows.STARTF_USESHOWWINDOW, ShowWindow: windows.SW_HIDE})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if info.Cb != uint32(unsafe.Sizeof(*info)) || info.ProcThreadAttributeList == nil || info.ShowWindow != windows.SW_HIDE {
		t.Fatalf("extended startup block lost its fields: %+v", info)
	}
}

func parentProcessID(t *testing.T, pid uint32) uint32 {
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
	t.Fatalf("child %d not found", pid)
	return 0
}

func elevated(t *testing.T, pid uint32) bool {
	t.Helper()
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(process)
	var token windows.Token
	if err = windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	return token.IsElevated()
}
