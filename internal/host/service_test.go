package host

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"winone/internal/catalog"
	"winone/internal/settings"
)

func TestFeatureCatalogExcludesInternalServicesAndAcceptsRegisteredExtension(t *testing.T) {
	store, _ := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	service := New(store)
	service.RegisterModule("shortcuts", func() Module { return Module{ID: "shortcuts"} }, func(r Request) Response { return Response{ID: r.ID} })
	manifest := catalog.Manifest{ManifestVersion: 1, ID: "example.tool", Name: catalog.LocalizedName{Chinese: "测试模块", English: "Test Module"}, Distribution: "optional", Worker: "bin/worker.exe", Activation: "on-demand", SettingsSchemaVersion: 1}
	if err := service.RegisterFeature(manifest, func() Module { return Module{ID: manifest.ID, State: "available"} }, func(r Request) Response {
		if r.Method != "ping" {
			t.Errorf("qualified namespace did not strip correctly: %s", r.Method)
		}
		return Response{ID: r.ID, Result: json.RawMessage(`true`)}
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterFeature(manifest, nil, nil); err == nil {
		t.Fatal("duplicate feature registered")
	}
	nested := manifest
	nested.ID = "example.tool.settings"
	if err := service.RegisterFeature(nested, func() Module { return Module{} }, func(r Request) Response { return Response{ID: r.ID} }); err == nil {
		t.Fatal("nested namespace could hijack existing module commands")
	}
	response := service.Handle(Request{Version: 1, ID: "catalog", Method: "modules.catalog"})
	var manifests []catalog.Manifest
	if err := json.Unmarshal(response.Result, &manifests); err != nil || len(manifests) != 4 {
		t.Fatalf("catalog: %v %+v", err, response)
	}
	response = service.Handle(Request{Version: 1, ID: "snapshot", Method: "snapshot"})
	var snapshot Snapshot
	if err := json.Unmarshal(response.Result, &snapshot); err != nil || len(snapshot.Modules) != 4 {
		t.Fatalf("snapshot: %v %+v", err, response)
	}
	for _, module := range snapshot.Modules {
		if module.ID == "shortcuts" {
			t.Fatal("internal service leaked into feature navigation")
		}
	}
	response = service.Handle(Request{Version: 1, ID: "extension-call", Method: "example.tool.ping"})
	if response.Error != nil || response.ID != "extension-call" || string(response.Result) != "true" {
		t.Fatalf("extension route: %+v", response)
	}
}

func TestFeatureCannotOverlapRegisteredServiceNamespaces(t *testing.T) {
	for _, id := range []string{"example.control.internal", "example.control.internal.extension", "example.control"} {
		t.Run(id, func(t *testing.T) {
			store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
			if err != nil {
				t.Fatal(err)
			}
			service := New(store)
			service.RegisterModule("example.control.internal", func() Module { return Module{ID: "example.control.internal"} }, func(r Request) Response { return Response{ID: r.ID, Result: json.RawMessage(`"original"`)} })
			manifest := catalog.Manifest{ManifestVersion: 1, ID: id, Name: catalog.LocalizedName{Chinese: "测试", English: "Test"}, Distribution: "optional", Worker: "worker.exe", Activation: "on-demand", SettingsSchemaVersion: 1}
			if err := service.RegisterFeature(manifest, func() Module { return Module{} }, func(r Request) Response { return Response{ID: r.ID} }); err == nil {
				t.Fatal("extension shadowed service", id)
			}
			response := service.Handle(Request{Version: 1, ID: "original-call", Method: "example.control.internal.ping"})
			if response.Error != nil || string(response.Result) != `"original"` {
				t.Fatal("failed registration changed existing service", response)
			}
		})
	}
}

func TestModuleRoutingKeepsRequestIdentity(t *testing.T) {
	store, _ := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	service := New(store)
	service.RegisterModule("wintray", func() Module { return Module{ID: "wintray", Name: "启动与托盘", State: "ready"} }, func(request Request) Response {
		if request.Method != "launch" {
			t.Errorf("module prefix leaked: %s", request.Method)
		}
		return Response{ID: request.ID, Result: json.RawMessage(`{"job":"1"}`)}
	})
	response := service.Handle(Request{Version: 1, ID: "original", Method: "wintray.launch"})
	if response.ID != "original" || response.Error != nil || string(response.Result) != `{"job":"1"}` {
		t.Fatalf("got %+v", response)
	}
}

func TestProtocolRejectsUnknownVersionAndMethod(t *testing.T) {
	s, _ := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	service := New(s)
	for _, request := range []Request{{Version: 99, ID: "1", Method: "snapshot"}, {Version: 1, ID: "2", Method: "execute.shell"}} {
		response := service.Handle(request)
		if response.Error == nil {
			t.Fatalf("accepted %+v", request)
		}
		if response.ID != request.ID {
			t.Fatal("lost correlation ID")
		}
	}
}

func TestServicePersistsAndReportsConflict(t *testing.T) {
	s, _ := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	service := New(s)
	value := s.Snapshot()
	value.Theme = "dark"
	data, _ := json.Marshal(value)
	r := Request{Version: 1, ID: "1", Method: "settings.save", Params: data}
	if response := service.Handle(r); response.Error != nil {
		t.Fatal(response.Error)
	}
	r.ID = "2"
	if response := service.Handle(r); response.Error == nil || response.Error.Code != "conflict" {
		t.Fatalf("got %+v", response)
	}
}
