//go:build windows

package directorylinks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"winone/internal/recycle"
)

// This is opt-in because it writes a small fixture on two real volumes and
// recycles the owned migration entries. The ordinary suite keeps using
// same-volume temporary trees.
func TestMigrationCrossVolumeUsesVerifiedCopyAndRestore(t *testing.T) {
	root := os.Getenv("WINONE_CROSS_VOLUME_ROOT")
	source := os.Getenv("WINONE_CROSS_VOLUME_SOURCE")
	if root == "" || source == "" {
		t.Skip("explicit cross-volume migration test")
	}
	if filepath.VolumeName(root) == "" || filepath.VolumeName(source) == "" ||
		filepath.VolumeName(root) == filepath.VolumeName(source) {
		t.Fatal("cross-volume fixture must use different volumes")
	}
	if _, err := os.Stat(root); err == nil {
		t.Fatal("cross-volume root already exists; refusing to touch an unknown directory")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := os.Lstat(source); err == nil {
		if os.Getenv("WINONE_CROSS_VOLUME_RECOVER") == "1" {
			if _, recycleErr := recycle.CheckedLink(context.Background(), source, func() error { return nil }); recycleErr != nil {
				t.Fatal("recovering the owned junction failed", recycleErr)
			}
		} else {
			t.Fatal("cross-volume source already exists; refusing to touch an unknown directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Both roots belong exclusively to this test. Recycle them after all
		// production handles have been released, including on a failed assertion.
		if info, err := os.Lstat(source); err == nil && info.Mode()&os.ModeSymlink != 0 {
			_, _ = recycle.CheckedLink(context.Background(), source, func() error { return nil })
		} else {
			_, _ = recycle.Directory(context.Background(), source)
		}
		_, _ = recycle.Directory(context.Background(), root)
	})
	if err := os.MkdirAll(filepath.Join(source, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "destination"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "sub", "keep.txt"), []byte("cross-volume data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "sub", "keep.txt:metadata"), []byte("cross-volume stream"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(filepath.Join(root, "history"))
	if err != nil {
		t.Fatal(err)
	}
	request := MigrationRequest{Source: source, Directory: filepath.Join(root, "destination"), Name: "cross-volume"}
	plan, err := m.PreviewMigration(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	row, err := m.ExecuteMigration(context.Background(), plan.ID, true, nil)
	if err != nil || row.Status != "active" {
		t.Fatal(row, err)
	}
	if data, err := os.ReadFile(filepath.Join(plan.Destination, "sub", "keep.txt")); err != nil || string(data) != "cross-volume data" {
		t.Fatal("cross-volume copy failed", err)
	}
	restore, err := m.PreviewMigration(context.Background(), MigrationRequest{RecordID: row.ID, CheckedAt: row.CheckedAt}, nil)
	if err != nil {
		t.Fatal(err)
	}
	row, err = m.ExecuteMigration(context.Background(), restore.ID, true, nil)
	if err != nil || row.Status != "restored" {
		t.Fatal(row, err)
	}
	if data, err := os.ReadFile(filepath.Join(source, "sub", "keep.txt:metadata")); err != nil || string(data) != "cross-volume stream" {
		t.Fatal("cross-volume restore lost the stream", err)
	}
}
