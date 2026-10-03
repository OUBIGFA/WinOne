package host

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"winone/internal/settings"
)

func TestSnapshotReportsRuntimePrivilegeWithoutChangingPreferences(t *testing.T) {
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := New(store)
	before := store.Snapshot()
	for _, administrator := range []bool{true, false} {
		service.SetAdministrator(administrator)
		response := service.Handle(Request{Version: 1, ID: "read", Method: "snapshot"})
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		var snapshot Snapshot
		if err = json.Unmarshal(response.Result, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Administrator != administrator {
			t.Fatal(snapshot.Administrator)
		}
	}
	if store.Snapshot() != before {
		t.Fatal("runtime privilege changed saved preferences")
	}
}
