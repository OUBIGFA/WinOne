package settings

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Reading a full-version profile must keep the other preferences and the
// original file while selecting a page that still exists in this edition.
func TestRemovedDefaultPagesReadAsOverviewWithoutChangingFile(t *testing.T) {
	for _, page := range []string{"chat", "onebot", "flash-chat"} {
		t.Run(page, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			original := Settings{SchemaVersion: 1, Theme: "dark", Language: "en-US", DefaultPage: page, CaptureShortcut: "Shift+A", PinShortcut: "Alt+Q", Revision: 7}
			data, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			store, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			got := store.Snapshot()
			if got.DefaultPage != "overview" || got.Theme != original.Theme || got.Language != original.Language || got.Revision != original.Revision || got.CaptureShortcut != original.CaptureShortcut {
				t.Fatalf("old profile was not projected safely: %+v", got)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(data, after) {
				t.Fatalf("reading rewrote the old profile: %v", err)
			}
		})
	}
}

func TestRemovedDefaultPagesCannotBeSelected(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{"chat", "onebot", "flash-chat"} {
		next := store.Snapshot()
		next.DefaultPage = page
		if _, err = store.Save(next); err == nil {
			t.Errorf("removed page accepted: %s", page)
		}
	}
}
