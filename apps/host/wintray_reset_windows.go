//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"winone/internal/configuration"
	"winone/internal/modulestate"
	"winone/modules/wintray/preferences"
)

type wintrayResetPreview struct {
	ConfigurationRevision uint64 `json:"configurationRevision"`
	ModuleRevision        uint64 `json:"moduleRevision"`
	ProgramCount          int    `json:"programCount"`
}

func previewWinTrayReset(store *configuration.HostStore, state *modulestate.Store) (wintrayResetPreview, error) {
	if err := store.RecoveryError(); err != nil {
		return wintrayResetPreview{}, err
	}
	document := store.CommittedDocument()
	if _, ok := document.ModuleEnabled["wintray"]; !ok {
		return wintrayResetPreview{}, errors.New("模块生命周期未登记")
	}
	value, err := preferences.Decode(document.Values["wintray"])
	if err != nil {
		return wintrayResetPreview{}, err
	}
	return wintrayResetPreview{document.Revision, state.Snapshot().Revision, len(value.ManagedApps)}, nil
}

// A single configuration transaction restores native resources before applying
// the empty program list. Local recovery journals and launch history stay intact.
func resetWinTray(store *configuration.HostStore, state *modulestate.Store, configurationRevision, moduleRevision uint64, confirmed bool) error {
	if !confirmed {
		return errors.New("请先确认清空托管程序")
	}
	if state.Snapshot().Revision != moduleRevision {
		return configuration.ErrConflict
	}
	document := store.CommittedDocument()
	if document.Revision != configurationRevision {
		return configuration.ErrConflict
	}
	if _, ok := document.ModuleEnabled["wintray"]; !ok {
		return errors.New("模块生命周期未登记")
	}
	current, err := preferences.Decode(document.Values["wintray"])
	if err != nil {
		return err
	}
	// The action clears programs, not the user's language, login choices or
	// general timing preferences. Use the canonical empty-list representation.
	current.ManagedApps = preferences.Defaults().ManagedApps
	document.Values["wintray"], err = json.Marshal(current)
	if err != nil {
		return err
	}
	document.ModuleEnabled["wintray"] = false
	_, err = store.CommitDocument(document)
	return err
}
