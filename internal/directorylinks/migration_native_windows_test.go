//go:build windows

package directorylinks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Explicit system integration: only this fixture's own trees are recycled.
// The test never empties the Recycle Bin, closes applications or changes settings.
func TestMigrationNativeRecycleAndRestore(t *testing.T) {
	if os.Getenv("WINONE_RECYCLE_INTEGRATION") != "1" {
		t.Skip("explicit owned migration/recycle test")
	}
	m, req, _ := migrationFixture(t)
	p, err := m.PreviewMigration(context.Background(), req, nil)
	if err != nil {
		t.Fatal(err)
	}
	row, err := m.ExecuteMigration(context.Background(), p.ID, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "active" || len(row.Migration.Retained) != 0 {
		t.Fatal("native backup recycle not verified", row.Migration)
	}
	if _, err = os.Lstat(row.Migration.Backup); !os.IsNotExist(err) {
		t.Fatal("backup did not leave its old location", err)
	}
	p, err = m.PreviewMigration(context.Background(), MigrationRequest{RecordID: row.ID, CheckedAt: row.CheckedAt}, nil)
	if err != nil {
		t.Fatal(err)
	}
	row, err = m.ExecuteMigration(context.Background(), p.ID, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "restored" || len(row.Migration.Retained) != 0 {
		t.Fatal("native restore cleanup not verified", row.Migration)
	}
	if data, err := os.ReadFile(filepath.Join(req.Source, "sub", "keep.txt:metadata")); err != nil || string(data) != "named stream" {
		t.Fatal("restored data missing", err)
	}
	if _, err = os.Lstat(p.Source); !os.IsNotExist(err) {
		t.Fatal("old data location was not recycled", err)
	}
}
