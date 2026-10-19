//go:build windows

package directorylinks

import (
	"context"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func TestForgetInvalidRechecksAndNeverRemovesFilesystemEntries(t *testing.T) {
	m, active, source := linkFixture(t, "hardlink", "file")
	// A stale invalid status must not cause a now-valid relationship to disappear.
	m.records[0].Status = "broken"
	missing := active
	missing.ID, missing.LinkPath, missing.Status = "missing", filepath.Join(filepath.Dir(active.LinkPath), "missing.txt"), "active"
	removed := active
	removed.ID, removed.Status = "removed", "removed"
	m.records = append(m.records, missing, removed)
	rows, count, err := m.ForgetInvalid(context.Background())
	if err != nil || count != 2 || len(rows) != 1 || rows[0].Status != "active" {
		t.Fatal(rows, count, err)
	}
	for _, path := range []string{source, active.LinkPath} {
		if raw, err := os.ReadFile(path); err != nil || string(raw) != "target data" {
			t.Fatal("history cleanup touched data", path, err)
		}
	}
	reopened, err := New(m.directory)
	if err != nil || len(reopened.List()) != 1 {
		t.Fatal("cleanup did not persist", err)
	}
}

func TestForgetInvalidRetainsUnverifiedEntries(t *testing.T) {
	m, row, _ := linkFixture(t, "hardlink", "file")
	path, err := windows.UTF16PtrFromString(row.LinkPath)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	m.records[0].Status = "broken"
	rows, count, err := m.ForgetInvalid(context.Background())
	if err != nil || count != 0 || len(rows) != 1 || rows[0].Status != "unknown" {
		t.Fatal("unverified entry lost", rows, count, err)
	}
}

func TestForgetInvalidRollsBackOnCancellationAndSaveFailure(t *testing.T) {
	m, row, _ := linkFixture(t, "hardlink", "file")
	m.records[0].LinkPath = filepath.Join(filepath.Dir(row.LinkPath), "missing.txt")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := m.ForgetInvalid(ctx); err == nil || len(m.List()) != 1 {
		t.Fatal("cancelled cleanup changed history", err)
	}
	history := filepath.Join(m.directory, "history.json")
	if err := os.Rename(history, history+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(history, 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.ForgetInvalid(context.Background()); err == nil || len(m.List()) != 1 {
		t.Fatal("failed save changed in-memory history", err)
	}
}
