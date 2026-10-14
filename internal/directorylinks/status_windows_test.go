//go:build windows

package directorylinks

import (
	"context"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
)

func TestJunctionRejectsAFileAtItsDirectoryTarget(t *testing.T) {
	m, r, source := linkFixture(t, "junction", "directory")
	if err := os.Rename(source, source+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("wrong type"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := m.Check(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "broken" || got.Reason != "target-type-changed" {
		t.Fatalf("wrong target type was not distinguished: %+v", got)
	}
}

func TestLockedEntryIsUnknownRatherThanMissingOrActive(t *testing.T) {
	m, r, _ := linkFixture(t, "hardlink", "file")
	name, _ := windows.UTF16PtrFromString(r.LinkPath)
	h, err := windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	got, err := m.Check(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "unknown" || got.Reason != "entry-unverified" {
		t.Fatalf("sharing failure misreported: %+v", got)
	}
}

func TestScanOfRenamedSourceFindsVerifiedHardLinkAlias(t *testing.T) {
	m, r, source := linkFixture(t, "hardlink", "file")
	newSource := source + ".renamed"
	if err := os.Rename(source, newSource); err != nil {
		t.Fatal(err)
	}
	got, err := m.Check(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "other-links" || !got.Accessible {
		t.Fatal(got)
	}
	if err := m.Forget(r.ID); err != nil {
		t.Fatal(err)
	}
	result, err := m.Scan(context.Background(), filepath.Dir(r.LinkPath), false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range result.Records {
		if row.LinkPath == r.LinkPath {
			found = true
			if row.Status != "active" || row.TargetPath != newSource {
				t.Fatalf("actual alias not resolved: %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("link not found")
	}
}

func TestRetargetedJunctionRequiresFreshInspectionBeforeRemoval(t *testing.T) {
	m, r, source := linkFixture(t, "junction", "directory")
	second := source + ".second"
	os.Mkdir(second, 0700)
	os.WriteFile(filepath.Join(second, "keep.txt"), []byte("second target"), 0600)
	name, _ := windows.UTF16PtrFromString(nativePath(r.LinkPath))
	h, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := junctionData(second)
	if err != nil {
		windows.CloseHandle(h)
		t.Fatal(err)
	}
	var count uint32
	err = windows.DeviceIoControl(h, windows.FSCTL_SET_REPARSE_POINT, &raw[0], uint32(len(raw)), nil, 0, &count, nil)
	windows.CloseHandle(h)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(r.LinkPath, "keep.txt")); err != nil || string(data) != "second target" {
		t.Fatal("fixture did not retarget", err)
	}
	if _, err := m.Remove(context.Background(), r.ID); err == nil {
		t.Fatal("uninspected target change allowed deletion")
	}
	got, err := m.Check(context.Background(), r.ID)
	if err != nil || got.Status != "target-changed" || got.ActualTarget != second {
		t.Fatal(got, err)
	}
	t.Run("recycle-reviewed-entry", func(t *testing.T) {
		if os.Getenv("WINONE_RECYCLE_INTEGRATION") != "1" {
			t.Skip("explicit recycle integration")
		}
		if _, err := m.Remove(context.Background(), r.ID); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{source, second} {
			if _, err := os.Stat(filepath.Join(path, "keep.txt")); err != nil {
				t.Fatal("target was removed", path, err)
			}
		}
	})
}

func TestLegacyNTFSIdentityRemainsCompatibleWithFileIdInfo(t *testing.T) {
	m, r, _ := linkFixture(t, "hardlink", "file")
	f, err := openState(r.LinkPath, false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(f.handle, &info); err != nil {
		t.Fatal(err)
	}
	legacy := Identity{Volume: info.VolumeSerialNumber, File: fmt.Sprintf("%08x%08x", info.FileIndexHigh, info.FileIndexLow), Created: uint64(info.CreationTime.HighDateTime)<<32 | uint64(info.CreationTime.LowDateTime)}
	m.records[0].EntryIdentity = legacy
	m.records[0].TargetIdentity = legacy
	got, err := m.Check(context.Background(), r.ID)
	if err != nil || got.Status != "active" {
		t.Fatal("legacy history changed identity", got, err)
	}
}
