//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/modules/wintray/preferences"
)

func TestDisabledWinTrayImportMergesHistoryWithoutWorker(t *testing.T) {
	directory := t.TempDir()
	get := func() *modules.Lazy { return nil }
	store, err := configuration.OpenHost(directory, wintrayConfiguration(directory, get))
	if err != nil {
		t.Fatal(err)
	}
	handler := wintrayConfigurationHandler(directory, store, get, func(r host.Request) host.Response { t.Fatal("import invoked worker"); return host.Response{} }, func() bool { return false })
	program := filepath.Join(directory, "example.exe")
	history, _ := json.Marshal(map[string]any{"version": 1, "apps": map[string][]time.Time{program: {time.Now().UTC()}}})
	for range 2 {
		raw, revision, err := store.Module("wintray")
		if err != nil {
			t.Fatal(err)
		}
		params, _ := json.Marshal(map[string]any{"revision": fmt.Sprint(revision), "settings": json.RawMessage(raw), "history": json.RawMessage(history)})
		response := handler(host.Request{Version: 1, ID: "import", Method: "settings.save", Params: params})
		if response.Error != nil {
			t.Fatal(response.Error)
		}
	}
	data, err := os.ReadFile(filepath.Join(directory, "modules", "wintray", "startup-history.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Apps map[string][]time.Time `json:"apps"`
	}
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Apps) != 1 || len(saved.Apps[strings.ToLower(program)]) != 1 {
		t.Fatal("history changed on repeated import")
	}
}

func TestWinTrayImportPreviewDoesNotStartWorker(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "settings.json")
	settings := preferences.Defaults()
	settings.StartupIntervalSeconds = 7
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	handler := wintrayConfigurationHandler("", nil, func() *modules.Lazy { t.Fatal("preview requested worker"); return nil }, func(r host.Request) host.Response { t.Fatal("preview forwarded to worker"); return host.Response{} }, func() bool { return false })
	params, _ := json.Marshal(map[string]string{"path": path})
	response := handler(host.Request{Version: 1, ID: "preview", Method: "import.preview", Params: params})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var preview struct {
		Settings preferences.Settings `json:"settings"`
	}
	if err = json.Unmarshal(response.Result, &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Settings.StartupIntervalSeconds != 7 {
		t.Fatal("preview changed settings")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, data) {
		t.Fatal("preview changed source")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatal("preview created output")
	}
}

func TestWinTrayIconPollingDoesNotStartStoppedWorker(t *testing.T) {
	handler := wintrayConfigurationHandler("", nil, func() *modules.Lazy { return nil }, func(request host.Request) host.Response {
		t.Fatal("polling started the worker")
		return host.Response{}
	})
	response := handler(host.Request{Version: 1, ID: "icons", Method: "icons"})
	if response.Error != nil || string(response.Result) != "[]" {
		t.Fatal(response)
	}
}

func TestWinTrayPreferencesPersistWithoutStartingWorker(t *testing.T) {
	directory := t.TempDir()
	get := func() *modules.Lazy { return nil }
	store, err := configuration.OpenHost(directory, wintrayConfiguration(directory, get))
	if err != nil {
		t.Fatal(err)
	}
	handler := wintrayConfigurationHandler(directory, store, get, func(r host.Request) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "module_disabled"}}
	})
	response := handler(host.Request{Version: 1, ID: "read", Method: "settings.get"})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var snapshot struct {
		Revision string               `json:"revision"`
		Settings preferences.Settings `json:"settings"`
	}
	if err = json.Unmarshal(response.Result, &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Settings.StartupIntervalSeconds = 17
	data, _ := json.Marshal(snapshot)
	response = handler(host.Request{Version: 1, ID: "save", Method: "settings.save", Params: data})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	data, err = os.ReadFile(filepath.Join(directory, "modules", "wintray", "managed-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var projected struct {
		Revision uint64               `json:"revision"`
		Settings preferences.Settings `json:"settings"`
	}
	if err = json.Unmarshal(data, &projected); err != nil {
		t.Fatal(err)
	}
	if projected.Revision != 1 || projected.Settings.StartupIntervalSeconds != 17 {
		t.Fatal(projected)
	}
	if _, err = os.Stat(filepath.Join(directory, "modules", "wintray", "settings.json")); !os.IsNotExist(err) {
		t.Fatal("legacy preferences written")
	}
}
