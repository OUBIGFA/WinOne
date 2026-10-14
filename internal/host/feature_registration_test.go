package host

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"winone/internal/catalog"
	"winone/internal/settings"
)

func registrationFixture(t *testing.T) (*Service, catalog.Manifest) {
	t.Helper()
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := catalog.Manifest{ManifestVersion: 1, ID: "example.tool", Name: catalog.LocalizedName{Chinese: "示例", English: "Example"}, Distribution: "optional", Worker: "worker.exe", Activation: "on-demand", SettingsSchemaVersion: 1}
	return New(store), manifest
}

func TestFeatureRegistrationRemovalIsScopedAndRetryable(t *testing.T) {
	service, manifest := registrationFixture(t)
	status := func() Module { return Module{ID: manifest.ID, State: "disabled"} }
	handler := func(r Request) Response { return Response{ID: r.ID, Result: json.RawMessage(`"old"`)} }
	registration, err := service.BindFeature(manifest, status, handler)
	if err != nil {
		t.Fatal(err)
	}
	if err = registration.Remove(); err != nil {
		t.Fatal(err)
	}
	if len(service.FeatureCatalog()) != len(catalog.Builtins()) {
		t.Fatal("removed feature remains in catalog")
	}
	if reply := service.Handle(Request{Version: 1, ID: "removed", Method: "example.tool.ping"}); reply.Error == nil {
		t.Fatal("removed route was callable")
	}
	_, err = service.BindFeature(manifest, status, func(r Request) Response { return Response{ID: r.ID, Result: json.RawMessage(`"new"`)} })
	if err != nil {
		t.Fatal(err)
	}
	if err = registration.Remove(); err != nil {
		t.Fatal("exact removal replay failed", err)
	}
	if reply := service.Handle(Request{Version: 1, ID: "new", Method: "example.tool.ping"}); reply.Error != nil || string(reply.Result) != `"new"` {
		t.Fatal("old removal revoked a later registration", reply)
	}
}

func TestFeatureRegistrationRejectsChangedBindingAndBuiltinRemoval(t *testing.T) {
	service, manifest := registrationFixture(t)
	status := func() Module { return Module{ID: manifest.ID} }
	handler := func(r Request) Response { return Response{ID: r.ID, Result: json.RawMessage(`"replacement"`)} }
	registration, err := service.BindFeature(manifest, status, handler)
	if err != nil {
		t.Fatal(err)
	}
	service.RegisterModule(manifest.ID, status, handler)
	if err = registration.Remove(); err == nil {
		t.Fatal("stale registration removed a replacement service")
	}
	if reply := service.Handle(Request{Version: 1, ID: "keep", Method: "example.tool.ping"}); reply.Error != nil || string(reply.Result) != `"replacement"` {
		t.Fatal(reply)
	}
	service, manifest = registrationFixture(t)
	manifest.Distribution = "bundled"
	registration, err = service.BindFeature(manifest, status, handler)
	if err != nil {
		t.Fatal(err)
	}
	if err = registration.Remove(); err == nil {
		t.Fatal("runtime removal accepted a bundled feature")
	}
}
