//go:build windows

package directorylinks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRecyclingPreservesSourceAndTarget(t *testing.T) {
	if os.Getenv("WINONE_RECYCLE_INTEGRATION") != "1" {
		t.Skip("explicit owned-link recycle test")
	}
	for _, tc := range []struct{ kind, object string }{{"hardlink", "file"}, {"junction", "directory"}, {"symlink", "file"}, {"symlink", "directory"}} {
		t.Run(tc.kind+tc.object, func(t *testing.T) {
			m, r, source := linkFixture(t, tc.kind, tc.object)
			result, err := m.Remove(context.Background(), r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != "removed" {
				t.Fatalf("not recycled: %+v", result)
			}
			if _, err := os.Lstat(r.LinkPath); !os.IsNotExist(err) {
				t.Fatal("link entry remains", err)
			}
			path := source
			if tc.object == "directory" {
				path = filepath.Join(source, "keep.txt")
			}
			if raw, err := os.ReadFile(path); err != nil || string(raw) != "target data" {
				t.Fatalf("source/target damaged: %s %v", raw, err)
			}
		})
	}
}

func TestRemoveRefusesReplacedEntryAndForgetKeepsFiles(t *testing.T) {
	m, r, _ := linkFixture(t, "hardlink", "file")
	if err := os.Rename(r.LinkPath, r.LinkPath+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.LinkPath, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Remove(context.Background(), r.ID); err == nil {
		t.Fatal("replacement was eligible for recycling")
	}
	if err := m.Forget(r.ID); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(r.LinkPath); err != nil || string(raw) != "replacement" {
		t.Fatal("forget affected a file", err)
	}
	if len(m.List()) != 0 {
		t.Fatal("record not forgotten")
	}
}

func TestHistoryRetainsInspectionsAcrossRestartAndBatch(t *testing.T) {
	m, r, source := linkFixture(t, "hardlink", "file")
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	rows, err := m.CheckMany(context.Background(), []string{r.ID, r.ID})
	if err != nil || len(rows) != 1 || rows[0].Status != "unlinked" {
		t.Fatal(rows, err)
	}
	reopened, err := New(m.directory)
	if err != nil {
		t.Fatal(err)
	}
	saved := reopened.List()
	if len(saved) != 1 || saved[0].CheckedAt != rows[0].CheckedAt || saved[0].Status != "unlinked" {
		t.Fatal("inspection history not retained", saved)
	}
}

func TestBrokenJunctionCanBeRecycledWithoutItsTarget(t *testing.T) {
	if os.Getenv("WINONE_RECYCLE_INTEGRATION") != "1" {
		t.Skip("explicit owned-link recycle test")
	}
	m, r, source := linkFixture(t, "junction", "directory")
	saved := source + ".saved"
	if err := os.Rename(source, saved); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Remove(context.Background(), r.ID); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(filepath.Join(saved, "keep.txt")); err != nil || string(raw) != "target data" {
		t.Fatal("removed target data", err)
	}
}
