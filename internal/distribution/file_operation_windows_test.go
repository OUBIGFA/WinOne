//go:build windows

package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
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

// An image section can block replacement after the process that loaded it has
// gone. Preserve the old bytes and the write intent until the section is gone.
func TestUpgradeFilePreservesMappedImageAndCanResume(t *testing.T) {
	installation, staged, backup, initial := fileOperationFixture(t)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{installation, backup} {
		if err := os.WriteFile(filepath.Join(directory, "WinOne.exe"), old, 0600); err != nil {
			t.Fatal(err)
		}
	}
	plan := initial.Snapshot().Plan
	sum := sha256.Sum256(old)
	plan.Files[0].PreviousSHA256 = hex.EncodeToString(sum[:])
	journal, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "mapped.json"), plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Transition(0, "applying"); err != nil {
		t.Fatal(err)
	}
	path, err := windows.UTF16PtrFromString(filepath.Join(installation, "WinOne.exe"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	const secImage = 0x01000000
	section, mapErr := windows.CreateFileMapping(file, nil, windows.PAGE_READONLY|secImage, 0, 0, nil)
	closeErr := windows.CloseHandle(file)
	if err := errors.Join(mapErr, closeErr); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if section != 0 {
			if err := windows.CloseHandle(section); err != nil {
				t.Error(err)
			}
		}
	}()
	view, err := windows.MapViewOfFile(section, windows.FILE_MAP_READ, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if view != 0 {
			if err := windows.UnmapViewOfFile(view); err != nil {
				t.Error(err)
			}
		}
	}()
	if err := ApplyUpgradeFile(installation, staged, journal, 0); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("mapped image replacement: %v", err)
	}
	actual, err := os.ReadFile(filepath.Join(installation, "WinOne.exe"))
	if err != nil || sha256.Sum256(actual) != sum || journal.Snapshot().FileStates[0] != "writing" {
		t.Fatal("failed replacement changed the original or lost its intent", err)
	}
	if err := windows.UnmapViewOfFile(view); err != nil {
		t.Fatal(err)
	}
	view = 0
	if err := windows.CloseHandle(section); err != nil {
		t.Fatal(err)
	}
	section = 0
	if err := ApplyUpgradeFile(installation, staged, journal, 0); err != nil {
		t.Fatal(err)
	}
	actual, err = os.ReadFile(filepath.Join(installation, "WinOne.exe"))
	if err != nil || string(actual) != "new" || journal.Snapshot().FileStates[0] != "applied" {
		t.Fatal("replacement did not resume after releasing the image", err)
	}
}
