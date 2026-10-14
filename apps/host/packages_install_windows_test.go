//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modules"
)

func readyPackagePreview(t *testing.T, p *installedPackages, store *configuration.HostStore, file string) packagePreviewView {
	t.Helper()
	preview := checkedPackagePreview(t, p, store, file)
	if !preview.CanInstall {
		t.Fatal(preview.Error)
	}
	return preview
}
func checkedPackagePreview(t *testing.T, p *installedPackages, store *configuration.HostStore, file string) packagePreviewView {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"path": file})
	reply := p.handlePreview(host.Request{ID: "start", PeerPID: 11, Method: "packagePreview.start", Params: raw}, store)
	if reply.Error != nil {
		t.Fatal(reply.Error)
	}
	var preview packagePreviewView
	_ = json.Unmarshal(reply.Result, &preview)
	raw, _ = json.Marshal(map[string]string{"id": preview.ID})
	deadline := time.Now().Add(5 * time.Second)
	for preview.State == "checking" {
		time.Sleep(10 * time.Millisecond)
		reply = p.handlePreview(host.Request{ID: "get", PeerPID: 11, Method: "packagePreview.get", Params: raw}, store)
		if reply.Error != nil {
			t.Fatal(reply.Error)
		}
		_ = json.Unmarshal(reply.Result, &preview)
		if time.Now().After(deadline) {
			t.Fatal("preview timed out")
		}
	}
	return preview
}
func TestConfirmedPackageInstallUsesPreviewAndStaysDisabled(t *testing.T) {
	p, store, file := packagePreviewFixture(t)
	preview := readyPackagePreview(t, p, store, file)
	service := host.New(store)
	control := modules.NewControl(p.state, nil, service.FeatureCatalog)
	if err := p.bind(service, control, store); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"id": preview.ID})
	if wrong := p.installPreview(host.Request{ID: "wrong", PeerPID: 12, Params: raw}, store, service, control); wrong.Error == nil {
		t.Fatal("another window consumed approval")
	}
	reply := p.installPreview(host.Request{ID: "install", PeerPID: 11, Params: raw}, store, service, control)
	if reply.Error != nil {
		t.Fatal(reply.Error)
	}
	if p.Running() || store.CommittedDocument().ModuleEnabled["example.tool"] || p.state.Snapshot().Modules["example.tool"].Enabled {
		t.Fatal("install activated module code")
	}
	settings, _, err := store.Module("example.tool")
	if err != nil || string(settings) != `{"size":3}` {
		t.Fatal("default settings missing", err)
	}
	registry, err := modulepackage.OpenRegistry(filepath.Join(p.data, "packages"))
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := registry.Snapshot()
	if err != nil || inventory.Records["example.tool"].ManifestSHA256 != preview.ManifestSHA256 {
		t.Fatal("wrong installed package", err)
	}
	if status := p.packageCleanupStatus(); status.Pending || status.Error != "" {
		t.Fatal("successful installation left preparation cleanup", status)
	}
	if replay := p.installPreview(host.Request{ID: "again", PeerPID: 11, Params: raw}, store, service, control); replay.Error == nil {
		t.Fatal("consumed preview replayed")
	}
}
func TestPackageInstallRejectsChangedSourceOrSettings(t *testing.T) {
	for _, change := range []string{"source", "settings"} {
		t.Run(change, func(t *testing.T) {
			p, store, file := packagePreviewFixture(t)
			preview := readyPackagePreview(t, p, store, file)
			service := host.New(store)
			control := modules.NewControl(p.state, nil, service.FeatureCatalog)
			if change == "source" {
				if err := os.WriteFile(file, []byte("changed archive"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				settings := store.Snapshot()
				settings.Theme = "dark"
				if _, err := store.Save(settings); err != nil {
					t.Fatal(err)
				}
			}
			raw, _ := json.Marshal(map[string]string{"id": preview.ID})
			if reply := p.installPreview(host.Request{ID: "install", PeerPID: 11, Params: raw}, store, service, control); reply.Error == nil {
				t.Fatal("changed approval accepted")
			}
			if _, exists := store.CommittedDocument().Values["example.tool"]; exists {
				t.Fatal("failed install published settings")
			}
			if p.Running() {
				t.Fatal("failed installation executed code")
			}
		})
	}
}
