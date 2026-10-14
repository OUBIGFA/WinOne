//go:build windows

package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modulestate"
	"winone/internal/quickassistant"
	"winone/internal/resultwindow"
	"winone/internal/settings"
)

func TestQuickAssistantPreferencesAreIndependentAndRevisionProtected(t *testing.T) {
	directory := t.TempDir()
	store, err := configuration.OpenHost(directory, selectionConfiguration(directory, nil), quickAssistantConfiguration(directory, nil))
	if err != nil {
		t.Fatal(err)
	}
	selectionBefore, _, _ := store.Module("selection")
	handle := quickAssistantConfigurationHandler(store, func(r host.Request) host.Response { return host.Response{ID: r.ID} })
	got := handle(host.Request{Method: "settings.get"})
	var prefs quickassistant.Preferences
	if err = json.Unmarshal(got.Result, &prefs); err != nil {
		t.Fatal(err)
	}
	prefs.Actions[0], prefs.Actions[1] = prefs.Actions[1], prefs.Actions[0]
	prefs.Actions = append(prefs.Actions, resultwindow.SelectionAction{ID: "custom-mail", Name: "邮件", Kind: "ai", Icon: "conversation", Prompt: "Write a reply.", Enabled: true})
	payload, _ := json.Marshal(prefs)
	saved := handle(host.Request{Method: "settings.save", Params: payload})
	if saved.Error != nil {
		t.Fatal(saved.Error)
	}
	var loaded quickassistant.Preferences
	json.Unmarshal(saved.Result, &loaded)
	if loaded.Actions[0].ID != "explain" || len(loaded.Actions) != 6 || loaded.Revision <= prefs.Revision {
		t.Fatal(loaded)
	}
	if stale := handle(host.Request{Method: "settings.save", Params: payload}); stale.Error == nil {
		t.Fatal("stale action editor overwrote settings")
	}
	selectionAfter, _, _ := store.Module("selection")
	if !reflect.DeepEqual(selectionBefore, selectionAfter) {
		t.Fatal("quick edits changed selection preferences")
	}
}
func TestQuickAssistantStopKeepsOtherResults(t *testing.T) {
	windows := &resultWindows{ctx: context.Background(), quickAssistantAllowed: true, sessions: map[string]*resultSession{"quick": {id: "quick", source: "quick-assistant", cancelled: make(chan struct{})}, "selection": {id: "selection", source: "selection", cancelled: make(chan struct{})}}}
	runtime := &quickAssistantRuntime{results: windows}
	if err := runtime.stop(); err != nil {
		t.Fatal(err)
	}
	if windows.quickAssistantAllowed || windows.sessions["quick"] != nil || windows.sessions["selection"] == nil {
		t.Fatal("module stop affected unrelated windows")
	}
	if err := windows.openQuickAssistant(resultwindow.DefaultSelectionActions()); err == nil {
		t.Fatal("disabled assistant opened")
	}
}
func TestDisabledCaptureDoesNotChangeResolvedQuickShortcut(t *testing.T) {
	states, err := modulestate.Open(filepath.Join(t.TempDir(), "modules.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = states.SetIntent(states.Snapshot().Revision, "capture", false); err != nil {
		t.Fatal(err)
	}
	old := "Ctrl+Shift+Q"
	runtime := &shortcutRuntime{registrar: &recordingRegistrar{}, moduleState: states}
	if err = runtime.applyConfiguration(settings.Settings{TranslateShortcut: &old}); err != nil {
		t.Fatal(err)
	}
	for _, binding := range runtime.bindings {
		if binding.ID == "quick-assistant" && binding.Shortcut != "" {
			t.Fatal("disabling capture silently changed assistant shortcut")
		}
	}
}
