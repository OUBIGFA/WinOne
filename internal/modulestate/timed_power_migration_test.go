package modulestate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTimedPowerUpgradePreservesIntentAndOriginalBackup(t *testing.T) {
	for _, existing := range []bool{false, true} {
		root := t.TempDir()
		path := filepath.Join(root, "module-state.json")
		old := Snapshot{SchemaVersion: 4, Revision: 22, Modules: map[string]Record{"capture": {PendingStop: true}, "always-on-top": {Enabled: true}, "wintray": {}, "selection": {}, "quick-assistant": {}, "stroke": {}}}
		if existing {
			old.Modules["timed-power"] = Record{}
		}
		before, _ := json.Marshal(old)
		if err := os.WriteFile(path, before, 0600); err != nil {
			t.Fatal(err)
		}
		store, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		got := store.Snapshot()
		if got.SchemaVersion != 5 || got.Revision != 23 || got.Modules["timed-power"].Enabled == existing {
			t.Fatal(got)
		}
		for id, want := range old.Modules {
			if got.Modules[id] != want {
				t.Fatalf("changed %s", id)
			}
		}
		backup, err := os.ReadFile(path + ".before-timed-power.bak")
		if err != nil || !bytes.Equal(backup, before) {
			t.Fatal("original backup", err)
		}
		reopened, err := Open(path)
		if err != nil || reopened.Snapshot().Revision != 23 {
			t.Fatal("upgrade repeated", err)
		}
		// Read-only maintenance input accepts both generations without rewriting.
		for _, state := range []Snapshot{old, got} {
			raw, _ := json.Marshal(state)
			if _, err := DecodeSnapshot(raw); err != nil {
				t.Fatal(err)
			}
		}
		delete(got.Modules, "timed-power")
		invalid, _ := json.Marshal(got)
		if _, err := DecodeSnapshot(invalid); err == nil {
			t.Fatal("incomplete current state accepted by maintenance")
		}
		if err := os.WriteFile(path, invalid, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path); err == nil {
			t.Fatal("incomplete current state silently repaired")
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(after, invalid) {
			t.Fatal("damaged state overwritten")
		}
	}
}
