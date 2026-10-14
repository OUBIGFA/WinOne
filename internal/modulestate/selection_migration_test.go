package modulestate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectionIsAddedToLegacyStateWithoutEnablingOldModules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module-state.json")
	data := []byte(`{"schemaVersion":1,"revision":7,"modules":{"capture":{"enabled":false,"pendingStop":true},"always-on-top":{"enabled":true},"wintray":{"enabled":false}}}`)
	os.WriteFile(path, data, 0600)
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := store.Snapshot()
	if got.SchemaVersion != 4 || !got.Modules["stroke"].Enabled || !got.Modules["quick-assistant"].Enabled || !got.Modules["selection"].Enabled || got.Modules["capture"].Enabled || !got.Modules["capture"].PendingStop || got.Modules["wintray"].Enabled {
		t.Fatalf("module migration lost intent: %+v", got)
	}
	backup, err := os.ReadFile(path + ".before-selection.bak")
	if err != nil || string(backup) != string(data) {
		t.Fatal("legacy backup missing", err)
	}
}
