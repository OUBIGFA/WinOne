//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/host"
	"winone/internal/settings"
)

func TestDiagnosticsServiceReadsExistingLogsWithoutCallingModules(t *testing.T) {
	directory := t.TempDir()
	store, err := settings.Open(filepath.Join(directory, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(store)
	for _, id := range []string{"capture", "always-on-top", "wintray"} {
		service.RegisterModule(id, func() host.Module { return host.Module{ID: id, State: "disabled"} }, func(r host.Request) host.Response {
			t.Fatal("reading diagnostics must not call a module")
			return host.Response{ID: r.ID}
		})
	}
	registerDiagnostics(service, directory)
	call := func(method, id string) host.Response {
		params, _ := json.Marshal(map[string]string{"module": id})
		return service.Handle(host.Request{Version: 1, ID: "logs", Method: method, Params: params})
	}
	missing := call("diagnostics.read", "capture")
	var logs struct {
		FolderAvailable bool `json:"folderAvailable"`
		Entries         []struct {
			Text string `json:"text"`
		} `json:"entries"`
	}
	if missing.Error != nil || json.Unmarshal(missing.Result, &logs) != nil || logs.FolderAvailable {
		t.Fatal(missing, logs)
	}
	if _, err := os.Stat(filepath.Join(directory, "modules")); !os.IsNotExist(err) {
		t.Fatal("a read created module data", err)
	}
	folder := filepath.Join(directory, "logs")
	if err := os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "host.log"), []byte("fixture process error"), 0600); err != nil {
		t.Fatal(err)
	}
	response := call("diagnostics.read", "capture")
	if response.Error != nil || json.Unmarshal(response.Result, &logs) != nil || !logs.FolderAvailable || len(logs.Entries) != 1 || logs.Entries[0].Text != "fixture process error" {
		t.Fatal(response, logs)
	}
	response = call("diagnostics.folder", "capture")
	var location struct {
		Path string `json:"path"`
	}
	if response.Error != nil || json.Unmarshal(response.Result, &location) != nil || location.Path != folder {
		t.Fatal(response, location)
	}
	for _, id := range []string{"../capture", "unknown", "C:/private"} {
		for _, method := range []string{"diagnostics.read", "diagnostics.folder"} {
			if response := call(method, id); response.Error == nil {
				t.Fatal("accepted unknown diagnostics target", id, method)
			}
		}
	}
	if response := call("diagnostics.delete", "capture"); response.Error == nil {
		t.Fatal("accepted undeclared operation")
	}
}
