//go:build windows

package distribution

import (
	"golang.org/x/sys/windows"
	"path/filepath"
	"testing"
)

func TestUpgradeJournalWriteFailureDoesNotAdvanceState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.json")
	store, err := CreateUpgradeJournal(path, journalPlan())
	if err != nil {
		t.Fatal(err)
	}
	name, _ := windows.UTF16PtrFromString(path)
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
	}()
	if err = store.Transition(0, "applying"); err == nil {
		t.Fatal("locked journal accepted write")
	}
	if state := store.Snapshot(); state.Revision != 0 || state.Phase != "prepared" {
		t.Fatal("memory state advanced", state)
	}
	reopened, err := OpenUpgradeJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Snapshot().Revision != 0 {
		t.Fatal("disk state advanced")
	}
	if err = windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	handle = 0
	if err = store.Transition(0, "applying"); err != nil {
		t.Fatal("retry failed", err)
	}
}
