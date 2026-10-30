package modulestate

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestStrokeMigrationPreservesExistingIntentAndAddsExplorerGestures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module-state.json")
	before := []byte(`{"schemaVersion":3,"revision":12,"modules":{"capture":{"enabled":false,"pendingStop":true},"always-on-top":{"enabled":false},"wintray":{"enabled":false},"selection":{"enabled":true},"quick-assistant":{"enabled":false}}}`)
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	state := store.Snapshot()
	if state.SchemaVersion != 5 || state.Revision != 14 || !state.Modules["timed-power"].Enabled || !state.Modules["stroke"].Enabled || !state.Modules["capture"].PendingStop || state.Modules["wintray"].Enabled || state.Modules["quick-assistant"].Enabled {
		t.Fatalf("lost migration intent: %+v", state)
	}
	backup, err := os.ReadFile(path + ".before-stroke.bak")
	if err != nil || !bytes.Equal(backup, before) {
		t.Fatalf("migration backup: %v", err)
	}
	reopened, err := Open(path)
	if err != nil || reopened.Snapshot().Revision != state.Revision {
		t.Fatalf("repeat open changed migration: %v", err)
	}
}
