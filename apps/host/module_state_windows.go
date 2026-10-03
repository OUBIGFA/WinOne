//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"winone/internal/modulestate"
)

// Existing product-level intent wins. Only profiles without that state migrate
// the old engine switch; unreadable legacy data quarantines the one affected module.
func openModuleState(directory string) (*modulestate.Store, []string, error) {
	path := filepath.Join(directory, "module-state.json")
	_, statErr := os.Stat(path)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, nil, statErr
	}
	store, err := modulestate.Open(path)
	if err != nil {
		return nil, nil, err
	}
	if statErr == nil {
		return store, nil, nil
	}
	data, err := os.ReadFile(filepath.Join(directory, "modules", "always-on-top", "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return store, nil, nil
	}
	var legacy struct {
		SchemaVersion int   `json:"schemaVersion"`
		Enabled       *bool `json:"enabled"`
	}
	quarantine := err != nil
	if !quarantine {
		quarantine = json.Unmarshal(data, &legacy) != nil || legacy.SchemaVersion > 1 || legacy.SchemaVersion < 0
	}
	if quarantine || legacy.Enabled != nil && !*legacy.Enabled {
		if _, err = store.SetIntent(store.Snapshot().Revision, "always-on-top", false); err != nil {
			return nil, nil, err
		}
		return store, []string{"always-on-top"}, nil
	}
	return store, nil, nil
}
