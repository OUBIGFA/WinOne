package modules

import (
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/modulestate"
)

func TestPackageGateRequiresExactPersistedBinding(t *testing.T) {
	store, err := modulestate.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := catalog.Manifest{ManifestVersion: 1, ID: "example.tool", Name: catalog.LocalizedName{Chinese: "示例", English: "Example"}, Distribution: "optional", Worker: "worker.exe", Activation: "on-demand", SettingsSchemaVersion: 1}
	first, second := strings.Repeat("a", 64), strings.Repeat("b", 64)
	if _, err = store.BindPackage(manifest.ID, first); err != nil {
		t.Fatal(err)
	}
	hooks := LifecycleHooks{Start: func() error { t.Fatal("construction must not activate module"); return nil }, Stop: func() error { return nil }, IntentChanged: func() error { return nil }, Status: func() host.Module { return host.Module{ID: manifest.ID, State: "ready"} }, Handle: func(r host.Request) host.Response { return host.Response{ID: r.ID} }}
	if _, err = NewPackageGate(manifest, second, store, hooks); err == nil {
		t.Fatal("gate accepted another package's binding")
	}
	gate, err := NewPackageGate(manifest, first, store, hooks)
	if err != nil || gate.Status().State != "disabled" {
		t.Fatal(err)
	}
}

func TestOldPackageGateCannotActivateOrHandleAfterRebinding(t *testing.T) {
	store, err := modulestate.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := catalog.Manifest{ManifestVersion: 1, ID: "example.tool", Name: catalog.LocalizedName{Chinese: "示例", English: "Example"}, Distribution: "optional", Worker: "worker.exe", Activation: "on-demand", SettingsSchemaVersion: 1}
	first, second := strings.Repeat("a", 64), strings.Repeat("b", 64)
	if _, err = store.BindPackage(manifest.ID, first); err != nil {
		t.Fatal(err)
	}
	starts, calls := 0, 0
	gate, err := NewPackageGate(manifest, first, store, LifecycleHooks{Start: func() error { starts++; return nil }, Stop: func() error { return nil }, IntentChanged: func() error { return nil }, Status: func() host.Module { return host.Module{ID: manifest.ID, State: "ready"} }, Handle: func(r host.Request) host.Response { calls++; return host.Response{ID: r.ID} }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.BindPackage(manifest.ID, second); err != nil {
		t.Fatal(err)
	}
	if err = gate.SetEnabled(store.Snapshot().Revision, true); err == nil || starts != 0 {
		t.Fatal("old package was activated", err)
	}
	if _, err = store.SetIntent(store.Snapshot().Revision, manifest.ID, true); err != nil {
		t.Fatal(err)
	}
	if reply := gate.Handle(host.Request{Version: 1, ID: "stale", Method: "run"}); reply.Error == nil || calls != 0 {
		t.Fatal("old package handled new binding")
	}
	if gate.Enabled() || gate.Status().State != "faulted" {
		t.Fatal("old package still reported usable")
	}
}

func TestPackageGateDoesNotExemptWorkerReadsFromMaintenance(t *testing.T) {
	store, err := modulestate.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := catalog.Manifest{ManifestVersion: 1, ID: "example.tool", Name: catalog.LocalizedName{Chinese: "测试", English: "Test"}, Distribution: "optional", Worker: "worker.exe", Activation: "on-demand", SettingsSchemaVersion: 1}
	digest := strings.Repeat("a", 64)
	if _, err = store.BindPackage(manifest.ID, digest); err != nil {
		t.Fatal(err)
	}
	calls := 0
	gate, err := NewPackageGate(manifest, digest, store, LifecycleHooks{Start: func() error { return nil }, Stop: func() error { return nil }, IntentChanged: func() error { return nil }, Status: func() host.Module { return host.Module{} }, Handle: func(r host.Request) host.Response { calls++; return host.Response{ID: r.ID} }})
	if err != nil {
		t.Fatal(err)
	}
	if err = gate.SetEnabled(store.Snapshot().Revision, true); err != nil {
		t.Fatal(err)
	}
	queue := host.NewConfigurationQueue(nil)
	gate.SetActionGuard(queue.AcquireAction)
	release, ok := queue.AcquireMaintenance()
	if !ok {
		t.Fatal("could not acquire maintenance")
	}
	defer release()
	for _, method := range []string{"snapshot", "settings.get", "jobs.get", "echo"} {
		reply := gate.Handle(host.Request{Version: 1, ID: "blocked", Method: method})
		if reply.Error == nil || reply.Error.Code != "configuration_busy" {
			t.Errorf("worker method bypassed maintenance: %s: %+v", method, reply)
		}
	}
	if calls != 0 {
		t.Fatal("maintenance permitted worker calls", calls)
	}
}
