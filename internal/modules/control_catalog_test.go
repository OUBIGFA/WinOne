package modules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/modulestate"
	"winone/internal/settings"
)

func TestControlUsesRegisteredCatalogForOptionalModuleLifecycle(t *testing.T) {
	directory := t.TempDir()
	statePath := filepath.Join(directory, "modules.json")
	initial, err := modulestate.Open(statePath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := initial.Snapshot()
	snapshot.Modules["example.tool"] = modulestate.Record{}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(statePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	state, err := modulestate.Open(statePath)
	if err != nil {
		t.Fatal(err)
	}
	global, err := settings.Open(filepath.Join(directory, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(global)
	gates := map[string]*Gate{}
	for id := range snapshot.Modules {
		gate, err := NewGate(id, id, state, LifecycleHooks{Start: func() error { return nil }, Stop: func() error { return nil }, IntentChanged: func() error { return nil }, Status: func() host.Module { return host.Module{ID: id, State: "ready"} }, Handle: func(r host.Request) host.Response { return host.Response{ID: r.ID} }})
		if err != nil {
			t.Fatal(err)
		}
		gates[id] = gate
	}
	control := NewControl(state, gates, service.FeatureCatalog)
	if len(control.Snapshot().Modules) != len(catalog.Builtins()) {
		t.Fatal("unregistered module was exposed")
	}
	unregistered, _ := json.Marshal(map[string]any{"id": "example.tool", "enabled": true, "revision": state.Snapshot().Revision})
	if response := control.Handle(host.Request{Version: 1, ID: "unregistered", Method: "setEnabled", Params: unregistered}); response.Error == nil || state.Snapshot().Modules["example.tool"].Enabled {
		t.Fatal("unregistered module was activated")
	}
	manifest := catalog.Manifest{ManifestVersion: 1, ID: "example.tool", Name: catalog.LocalizedName{Chinese: "测试工具", English: "Test Tool"}, Distribution: "optional", Worker: "worker.exe", Activation: "on-demand", SettingsSchemaVersion: 1}
	if err = service.RegisterFeature(manifest, gates[manifest.ID].Status, gates[manifest.ID].Handle); err != nil {
		t.Fatal(err)
	}
	modules := control.Snapshot().Modules
	if len(modules) != len(catalog.Builtins())+1 || modules[len(modules)-1].Manifest.ID != manifest.ID || modules[len(modules)-1].Enabled {
		t.Fatal("registered disabled extension missing", modules)
	}
	response := control.Handle(host.Request{Version: 1, ID: "catalog", Method: "catalog"})
	var listed []catalog.Manifest
	if err = json.Unmarshal(response.Result, &listed); err != nil || len(listed) != len(catalog.Builtins())+1 {
		t.Fatal(response, err)
	}
	params, _ := json.Marshal(map[string]any{"id": manifest.ID, "enabled": true, "revision": state.Snapshot().Revision})
	response = control.Handle(host.Request{Version: 1, ID: "enable", Method: "setEnabled", Params: params})
	var job ControlJob
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	if err = json.Unmarshal(response.Result, &job); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for job.State == "running" {
		if time.Now().After(deadline) {
			t.Fatal("enable timed out")
		}
		time.Sleep(time.Millisecond)
		query, _ := json.Marshal(map[string]string{"id": job.ID})
		response = control.Handle(host.Request{Version: 1, ID: "poll", Method: "jobs.get", Params: query})
		if err = json.Unmarshal(response.Result, &job); err != nil {
			t.Fatal(err)
		}
	}
	if job.State != "completed" || !control.Snapshot().Modules[len(catalog.Builtins())].Enabled {
		t.Fatal("extension enable did not commit", job)
	}
	listed[len(catalog.Builtins())].Name.Chinese = "changed"
	if service.FeatureCatalog()[len(catalog.Builtins())].Name.Chinese != manifest.Name.Chinese {
		t.Fatal("catalog copy changed registration")
	}
}
