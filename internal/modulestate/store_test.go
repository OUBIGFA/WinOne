package modulestate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPendingStopSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modules.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	first := store.Snapshot()
	if _, err = store.SetIntent(first.Revision, "capture", false); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	pending := restarted.Snapshot()
	if pending.Modules["capture"].Enabled || !pending.Modules["capture"].PendingStop {
		t.Fatal("interrupted stop was forgotten")
	}
	if _, err = restarted.SetIntent(pending.Revision, "capture", true); err == nil {
		t.Fatal("enabled before recovery")
	}
	if _, err = restarted.FinishStop("capture"); err != nil {
		t.Fatal(err)
	}
	if restarted.Snapshot().Modules["capture"] != (Record{}) {
		t.Fatal("completed stop did not settle")
	}
	first.Modules["wintray"] = Record{}
	if !store.Snapshot().Modules["wintray"].Enabled {
		t.Fatal("snapshot mutation altered stored state")
	}
}

func TestFailedWriteKeepsPreviousIntent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modules.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetIntent(0, "capture", false); err == nil {
		t.Fatal("failed persistence was accepted")
	}
	if !store.Snapshot().Modules["capture"].Enabled || store.Snapshot().Revision != 0 {
		t.Fatal("failed write changed in-memory intent")
	}
}

func TestIncompleteStateNeverSilentlyEnablesModules(t *testing.T) {
	for _, data := range []string{`{}`, `null`, `{"schemaVersion":1,"modules":{}}`, `{"schemaVersion":1,"modules":{"capture":{"enabled":false}}}`} {
		path := filepath.Join(t.TempDir(), "modules.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path); err == nil {
			t.Fatalf("incomplete state accepted: %s", data)
		}
		preserved, _ := os.ReadFile(path)
		if string(preserved) != data {
			t.Fatal("incomplete state was overwritten")
		}
	}
}
