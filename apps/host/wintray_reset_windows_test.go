//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/configuration"
	"winone/internal/modulestate"
	"winone/modules/wintray/preferences"
)

func TestWinTrayResetRestoresBeforeClearingAndPreservesGlobalChoices(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "restore failure"}[fail], func(t *testing.T) {
			directory := t.TempDir()
			initial := preferences.Defaults()
			initial.RunAtLogon = true
			initial.StartMinimizedToTray = true
			initial.ExitAfterManagedAppsCompleted = true
			initial.StartupIntervalSeconds = 17
			initial.CloseWindowRetrySeconds = 19
			if err := json.Unmarshal([]byte(`[{"id":"one","name":"test","exePath":"C:\\Apps\\test.exe"}]`), &initial.ManagedApps); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(initial)
			restored := false
			adapter := configuration.ModuleAdapter{ID: "wintray", Load: func() (json.RawMessage, error) { return raw, nil }, Validate: func(data json.RawMessage) error { _, err := preferences.Decode(data); return err }, Apply: func(data json.RawMessage, _ uint64) error {
				value, _ := preferences.Decode(data)
				if len(value.ManagedApps) == 0 && !restored {
					t.Fatal("cleared before restoration")
				}
				return nil
			}}
			other := configuration.ModuleAdapter{ID: "capture", Load: func() (json.RawMessage, error) { return json.RawMessage(`{"retained":true}`), nil }, Validate: func(json.RawMessage) error { return nil }, Apply: func(json.RawMessage, uint64) error { return nil }}
			store, err := configuration.OpenHostWithLifecycle(directory, configuration.ModuleLifecycle{Initial: map[string]bool{"wintray": true, "capture": true}, Before: func(target map[string]bool) error {
				if !target["wintray"] {
					if fail {
						return errors.New("restore failed")
					}
					restored = true
				}
				return nil
			}, After: func(map[string]bool) error { return nil }}, adapter, other)
			if err != nil {
				t.Fatal(err)
			}
			state, err := modulestate.Open(filepath.Join(directory, "module-state.json"))
			if err != nil {
				t.Fatal(err)
			}
			preview, err := previewWinTrayReset(store, state)
			if err != nil {
				t.Fatal(err)
			}
			if preview.ProgramCount != 1 {
				t.Fatal("incorrect preview")
			}
			preserved := filepath.Join(directory, "runtime-record")
			if err = os.WriteFile(preserved, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			if err = resetWinTray(store, state, preview.ConfigurationRevision, preview.ModuleRevision+1, true); !errors.Is(err, configuration.ErrConflict) {
				t.Fatal("stale module state accepted")
			}
			err = resetWinTray(store, state, preview.ConfigurationRevision, preview.ModuleRevision, true)
			document := store.CommittedDocument()
			saved, _ := preferences.Decode(document.Values["wintray"])
			if string(document.Values["capture"]) != `{"retained":true}` || !document.ModuleEnabled["capture"] {
				t.Fatal("other module changed")
			}
			retained, readErr := os.ReadFile(preserved)
			if readErr != nil || string(retained) != "keep" {
				t.Fatal("runtime data removed")
			}
			if fail {
				if err == nil || len(saved.ManagedApps) != 1 || !document.ModuleEnabled["wintray"] {
					t.Fatal("failed restore committed reset")
				}
				return
			}
			if err != nil || len(saved.ManagedApps) != 0 || document.ModuleEnabled["wintray"] {
				t.Fatal("reset incomplete", err)
			}
			if !saved.RunAtLogon || !saved.StartMinimizedToTray || !saved.ExitAfterManagedAppsCompleted {
				t.Fatal("global choices changed")
			}
			if saved.StartupIntervalSeconds != 17 || saved.CloseWindowRetrySeconds != 19 {
				t.Fatal("general timing preferences changed")
			}
			if err = resetWinTray(store, state, preview.ConfigurationRevision, preview.ModuleRevision, true); err == nil {
				t.Fatal("stale preview accepted")
			}
		})
	}
}

func TestWinTrayResetRequiresConfirmation(t *testing.T) {
	if err := resetWinTray(nil, nil, 0, 0, false); err == nil {
		t.Fatal("missing confirmation accepted")
	}
}
