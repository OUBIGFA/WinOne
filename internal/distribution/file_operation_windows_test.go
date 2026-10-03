//go:build windows

package distribution

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func TestUpgradeFileLockedReplacementRetainsRecoveryIntent(t *testing.T) {
	installation, staged, backup, journal := fileOperationFixture(t)
	path := filepath.Join(installation, "WinOne.exe")
	name, _ := windows.UTF16PtrFromString(path)
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
	}()
	if err = ApplyUpgradeFile(installation, staged, journal, 0); err == nil {
		t.Fatal("locked replacement succeeded")
	}
	if journal.Snapshot().FileStates[0] != "writing" {
		t.Fatal("recovery intent lost")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "old" {
		t.Fatal("locked old file changed", err)
	}
	if err = windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	handle = 0
	if err = journal.Transition(journal.Snapshot().Revision, "restoring"); err != nil {
		t.Fatal(err)
	}
	if err = RestoreUpgradeFile(installation, backup, journal, 0); err != nil {
		t.Fatal(err)
	}
	if journal.Snapshot().FileStates[0] != "restored" {
		t.Fatal("failed replacement not recovered")
	}
}
