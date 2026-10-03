//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
)

func TestCapturePreferencesPersistWhileDisabled(t *testing.T) {
	directory := t.TempDir()
	store, err := configuration.OpenHost(directory, captureConfiguration(directory, func() *modules.Lazy { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	handler := captureConfigurationHandler(store, func(r host.Request) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "module_disabled"}}
	})
	response := handler(host.Request{Version: 1, ID: "read", Method: "settings.get"})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var value capturePreferences
	if err = json.Unmarshal(response.Result, &value); err != nil {
		t.Fatal(err)
	}
	value.OutputDirectory = filepath.Join(directory, "images")
	if !value.OpenEditor {
		t.Fatal("new capture settings must open the editor")
	}
	value.ScrollDelay = 700
	value.OpenEditor = true
	value.FileNameTemplate = "work-{date}-{sequence}"
	data, _ := json.Marshal(value)
	response = handler(host.Request{Version: 1, ID: "save", Method: "settings.save", Params: data})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	data, err = os.ReadFile(filepath.Join(directory, "modules", "capture", "managed-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if value.Revision != 1 || value.ScrollDelay != 700 || !value.OpenEditor || value.FileNameTemplate != "work-{date}-{sequence}" {
		t.Fatal(value)
	}
	preview := handler(host.Request{Version: 1, ID: "preview", Method: "filename.preview", Params: json.RawMessage(`{}`)})
	if preview.Error != nil {
		t.Fatal(preview.Error)
	}
	var suggested struct {
		Name string `json:"name"`
	}
	if err = json.Unmarshal(preview.Result, &suggested); err != nil || len(suggested.Name) == 0 {
		t.Fatal("missing filename preview", err)
	}
	invalid := handler(host.Request{Version: 1, ID: "preview-bad", Method: "filename.preview", Params: json.RawMessage(`{"template":"../outside"}`)})
	if invalid.Error == nil {
		t.Fatal("path template was accepted")
	}
	response = handler(host.Request{Version: 1, ID: "capture", Method: "capture.start"})
	if response.Error == nil || response.Error.Code != "module_disabled" {
		t.Fatal("disabled capture activated")
	}
}

func TestGlobalThemeOverridesLegacyCaptureTheme(t *testing.T) {
	directory := t.TempDir()
	get := func() *modules.Lazy { return nil }
	store, err := configuration.OpenHost(directory, captureConfiguration(directory, get))
	if err != nil {
		t.Fatal(err)
	}
	next := store.Snapshot()
	next.Theme = "dark"
	if _, err = store.Save(next); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "modules", "capture", "managed-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var projected capturePreferences
	if err = json.Unmarshal(data, &projected); err != nil {
		t.Fatal(err)
	}
	if projected.Theme != "dark" {
		t.Fatal("capture did not follow global theme", projected.Theme)
	}
	projected.Theme = "light"
	params, _ := json.Marshal(projected)
	handler := captureConfigurationHandler(store, func(r host.Request) host.Response { return host.Response{ID: r.ID} })
	response := handler(host.Request{Version: 1, ID: "save", Method: "settings.save", Params: params})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	if err = json.Unmarshal(response.Result, &projected); err != nil {
		t.Fatal(err)
	}
	if projected.Theme != "dark" {
		t.Fatal("legacy module theme remained authoritative")
	}
	revision := store.Snapshot().Revision
	reopened, err := configuration.OpenHost(directory, captureConfiguration(directory, get))
	if err != nil || reopened.Snapshot().Revision != revision {
		t.Fatal("restart repeatedly normalized unchanged preferences", err)
	}
}
