//go:build windows

package logintask

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAdministratorLauncherRejectsOtherExecutablesAndLaunchArguments(t *testing.T) {
	for _, path := range []string{"WinOne.exe", `C:\Other.exe`, `C:\WinOne.Host.exe`} {
		if _, err := NewAdministratorLauncher(path); err == nil {
			t.Fatal(path)
		}
	}
	executable := filepath.Join(t.TempDir(), "WinOne.exe")
	if err := os.WriteFile(executable, []byte("non-executable test fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	launcher, err := NewAdministratorLauncher(executable)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	launcher.invoke = func(name string, mode LaunchMode, sid, path string) (string, error) {
		calls++
		if path != executable || sid == "" || name != "WinOne-Administrator-"+sid {
			t.Fatal(name, sid, path)
		}
		return "accepted", nil
	}
	for _, mode := range []LaunchMode{"", "interactive --other", "login\" --data-dir=other"} {
		if _, err = launcher.Run(mode); err == nil {
			t.Fatal("untrusted mode accepted", mode)
		}
	}
	if calls != 0 {
		t.Fatal("invalid mode reached the scheduler")
	}
	for _, mode := range []LaunchMode{InteractiveLaunch, LoginLaunch} {
		if _, err = launcher.Run(mode); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}
func TestAdministratorLauncherRejectsMissingEntryBeforeScheduling(t *testing.T) {
	launcher, err := NewAdministratorLauncher(filepath.Join(t.TempDir(), "WinOne.exe"))
	if err != nil {
		t.Fatal(err)
	}
	launcher.invoke = func(string, LaunchMode, string, string) (string, error) {
		t.Fatal("missing executable was scheduled")
		return "", nil
	}
	if _, err = launcher.Run(InteractiveLaunch); err == nil {
		t.Fatal("missing entry accepted")
	}
}
