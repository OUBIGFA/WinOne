//go:build windows

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"
	"winone/internal/ai"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
)

func TestIdleAIConfigurationRequestMergesBackgroundRefresh(t *testing.T) {
	s := queuedAccountService(t, context.Background())
	base := s.store.Configuration()
	base.Providers[0].RefreshModels([]ai.ModelOption{{ID: "a", Name: "A"}})
	if _, err := s.store.SaveConfiguration(base, nil); err != nil {
		t.Fatal(err)
	}
	base = s.store.Configuration()
	draft := s.store.Configuration()
	draft.Providers[0].Name = "Edited account"
	current := s.store.Configuration()
	current.Providers[0].RefreshModels([]ai.ModelOption{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}})
	current.Providers[0].ModelRefreshAt = time.Unix(3600, 0).UTC()
	if _, err := s.store.SaveConfiguration(current, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"configuration": draft, "base": base})
	response := s.handle(host.Request{Version: 1, ID: "idle-save", Method: "configuration.save", Params: raw})
	if response.Error != nil {
		t.Fatal("Host discarded the edit base", response.Error)
	}
	var saved ai.Configuration
	if err := json.Unmarshal(response.Result, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Providers[0].Name != "Edited account" || len(saved.Providers[0].AllModels()) != 2 {
		t.Fatal("Host lost edited/refreshed fields")
	}
}

func TestIdleWinTraySaveAfterGlobalPreferencesChange(t *testing.T) {
	directory := t.TempDir()
	get := func() *modules.Lazy { return nil }
	store, err := configuration.OpenHost(directory, wintrayConfiguration(directory, get))
	if err != nil {
		t.Fatal(err)
	}
	handler := wintrayConfigurationHandler(directory, store, get, func(host.Request) host.Response { t.Fatal("unexpected worker request"); return host.Response{} })
	response := handler(host.Request{Version: 1, ID: "read", Method: "settings.get"})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var draft trayConfigurationSnapshot
	if err = json.Unmarshal(response.Result, &draft); err != nil {
		t.Fatal(err)
	}
	global := store.Snapshot()
	global.Theme = "dark"
	if _, err = store.Save(global); err != nil {
		t.Fatal(err)
	}
	draft.Settings.StartupIntervalSeconds = 17
	raw, _ := json.Marshal(map[string]any{"revision": draft.Revision, "settings": draft.Settings})
	response = handler(host.Request{Version: 1, ID: "save", Method: "settings.save", Params: raw})
	if response.Error != nil {
		t.Fatal("wrapper rejected unrelated global revision", response.Error)
	}
	stored, _, err := store.Module("wintray")
	if err != nil {
		t.Fatal(err)
	}
	var preferences struct {
		StartupIntervalSeconds int `json:"startupIntervalSeconds"`
	}
	if err = json.Unmarshal(stored, &preferences); err != nil || preferences.StartupIntervalSeconds != 17 || store.Snapshot().Theme != "dark" {
		t.Fatal("unrelated preferences lost", err)
	}
}
