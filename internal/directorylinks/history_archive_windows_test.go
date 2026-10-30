//go:build windows

package directorylinks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func archiveFixture(t *testing.T, kind, object string) (*Manager, Record, string, []byte) {
	t.Helper()
	source, row, path := linkFixture(t, kind, object)
	archive, err := source.ExportHistory([]string{row.ID})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(archive)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(filepath.Join(t.TempDir(), "imported"))
	if err != nil {
		t.Fatal(err)
	}
	return m, row, path, raw
}
func TestHistoryArchiveRoundTripRebuildsLocalIdentityWithoutChangingData(t *testing.T) {
	for _, kind := range []string{"junction", "hardlink", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			object := "file"
			if kind != "hardlink" {
				object = "directory"
			}
			m, original, source, raw := archiveFixture(t, kind, object)
			var exported struct {
				Entries []map[string]json.RawMessage `json:"entries"`
			}
			if err := json.Unmarshal(raw, &exported); err != nil {
				t.Fatal(err)
			}
			if len(exported.Entries) != 1 || len(exported.Entries[0]) != 4 || exported.Entries[0]["entryIdentity"] != nil || exported.Entries[0]["id"] != nil {
				t.Fatal("archive retained local authority", string(raw))
			}
			preview, err := m.PreviewHistoryImport(context.Background(), raw)
			if err != nil || len(preview.Records) != 1 || len(m.List()) != 0 {
				t.Fatal(preview, err)
			}
			// A caller changing the returned preview cannot change the stored intent.
			preview.Records[0].TargetPath = "C:\\wrong"
			rows, err := m.ImportHistory(context.Background(), preview.ID)
			if err != nil || len(rows) != 1 || rows[0].Status != "active" || rows[0].TargetPath != source {
				t.Fatal(rows, err)
			}
			if rows[0].ID == original.ID || !sameIdentity(rows[0].EntryIdentity, original.EntryIdentity) {
				t.Fatal("imported foreign id or failed local inspection")
			}
			if _, err = m.ImportHistory(context.Background(), preview.ID); err == nil {
				t.Fatal("import token reused")
			}
			for _, path := range []string{source, original.LinkPath} {
				if object == "directory" {
					path = filepath.Join(path, "keep.txt")
				}
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "target data" {
					t.Fatal("data changed", path, err)
				}
			}
			reopened, err := New(m.directory)
			if err != nil || len(reopened.List()) != 1 {
				t.Fatal("history did not persist", err)
			}
		})
	}
}
func TestHistoryArchiveRejectsForgedLinksAndReportsConflicts(t *testing.T) {
	m, row, source, raw := archiveFixture(t, "junction", "directory")
	var archive HistoryArchive
	if err := json.Unmarshal(raw, &archive); err != nil {
		t.Fatal(err)
	}
	normal := archive.Entries[0]
	normal.LinkPath = source
	changed := archive.Entries[0]
	changed.TargetPath = filepath.Dir(source)
	archive.Entries = append(archive.Entries, archive.Entries[0], normal, changed)
	data, _ := json.Marshal(archive)
	p, err := m.PreviewHistoryImport(context.Background(), data)
	if err != nil || len(p.Records) != 1 || len(p.Issues) != 3 {
		t.Fatal(p, err)
	}
	if _, err = m.ImportHistory(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	p, err = m.PreviewHistoryImport(context.Background(), raw)
	if err != nil || p.ID != "" || len(p.Issues) != 1 {
		t.Fatal("existing record could be overwritten", p, err)
	}
	f, err := openState(row.LinkPath, false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()
	if !sameIdentity(f.identity, row.EntryIdentity) {
		t.Fatal("preview modified real entry")
	}
}
func TestHistoryArchiveImportRejectsChangedObjectAndIsAtomic(t *testing.T) {
	m, row, _, raw := archiveFixture(t, "junction", "directory")
	p, err := m.PreviewHistoryImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate an external replacement. Move the original fixture, do not delete it.
	if err = os.Rename(row.LinkPath, row.LinkPath+".original"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(row.LinkPath, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ImportHistory(context.Background(), p.ID); err == nil || len(m.List()) != 0 {
		t.Fatal("replacement accepted", err)
	}
	if info, err := os.Stat(row.LinkPath); err != nil || !info.IsDir() {
		t.Fatal("replacement changed", err)
	}
}
func TestHistoryArchiveImportRechecksTargetIdentity(t *testing.T) {
	m, _, source, raw := archiveFixture(t, "junction", "directory")
	p, err := m.PreviewHistoryImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(source, source+".old"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ImportHistory(context.Background(), p.ID); err == nil || len(m.List()) != 0 {
		t.Fatal("target replacement accepted", err)
	}
}
func TestHistoryArchiveCancellationExpirationAndSaveFailure(t *testing.T) {
	m, _, _, raw := archiveFixture(t, "hardlink", "file")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.PreviewHistoryImport(ctx, raw); err == nil {
		t.Fatal("canceled preview succeeded")
	}
	p, err := m.PreviewHistoryImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.ImportHistory(ctx, p.ID); err == nil || len(m.List()) != 0 {
		t.Fatal("canceled import changed history", err)
	}
	p, err = m.PreviewHistoryImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	saved := m.historyImports[p.ID]
	saved.ExpiresAt = "2000-01-01T00:00:00Z"
	m.historyImports[p.ID] = saved
	if _, err = m.ImportHistory(context.Background(), p.ID); err == nil {
		t.Fatal("expired preview succeeded")
	}
	p, err = m.PreviewHistoryImport(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(m.directory, "history.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = m.ImportHistory(context.Background(), p.ID); err == nil || len(m.List()) != 0 {
		t.Fatal("failed persistence left imported rows", err)
	}
}
func TestHistoryArchiveBrokenTargetAndMalformedInput(t *testing.T) {
	m, _, source, raw := archiveFixture(t, "junction", "directory")
	if err := os.Rename(source, source+".kept"); err != nil {
		t.Fatal(err)
	}
	p, err := m.PreviewHistoryImport(context.Background(), raw)
	if err != nil || len(p.Records) != 1 || p.Records[0].Reason != "target-missing" {
		t.Fatal(p, err)
	}
	rows, err := m.ImportHistory(context.Background(), p.ID)
	if err != nil || rows[0].Status != "broken" {
		t.Fatal(rows, err)
	}
	for _, raw := range []string{`null`, `{"version":2,"entries":[]}`, `{"version":1,"entries":[]}`, `{"version":1,"entries":[{"linkPath":"C:\\x","entryIdentity":{"file":"forged"}}]}`, strings.Repeat("x", (4<<20)+1)} {
		if _, err = m.PreviewHistoryImport(context.Background(), []byte(raw)); err == nil {
			t.Fatal("invalid archive accepted")
		}
	}
}
func TestHistoryArchiveFileExportDoesNotOverwriteAndSelectedOnly(t *testing.T) {
	m, row, _ := linkFixture(t, "hardlink", "file")
	directory := t.TempDir()
	first, err := m.ExportHistoryFile([]string{row.ID}, directory)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.ExportHistoryFile([]string{row.ID}, directory)
	if err != nil || first == second {
		t.Fatal(first, second, err)
	}
	if _, err = m.ExportHistory([]string{"missing"}); err == nil {
		t.Fatal("missing selection silently exported")
	}
	if _, err = m.ExportHistory(nil); err == nil {
		t.Fatal("empty selection exported all")
	}
	importer, err := New(filepath.Join(t.TempDir(), "history"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := importer.PreviewHistoryFile(context.Background(), first)
	if err != nil || len(p.Records) != 1 {
		t.Fatal(p, err)
	}
}
