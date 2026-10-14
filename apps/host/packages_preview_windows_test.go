//go:build windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modulestate"
)

func packagePreviewFixture(t *testing.T) (*installedPackages, *configuration.HostStore, string) {
	t.Helper()
	file, publisher := hostPackageArchiveFixture(t, []byte("inert fixture"))
	return packagePreviewFixtureFromArchive(t, file, publisher)
}
func packagePreviewFixtureFromArchive(t *testing.T, file string, publisher modulepackage.Publisher) (*installedPackages, *configuration.HostStore, string) {
	t.Helper()
	root := t.TempDir()
	state, err := modulestate.Open(filepath.Join(root, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	packages, err := discoverInstalledPackages(context.Background(), root, `\\.\pipe\WinOne-preview-test`, state, packageTestEnvironment, func(string) (modulepackage.Publisher, error) { return publisher, nil }, func() (func(), bool) { return func() {}, true })
	if err != nil {
		t.Fatal(err)
	}
	store, err := configuration.OpenHostWithLifecycle(root, configuration.ModuleLifecycle{Initial: map[string]bool{}, Before: func(map[string]bool) error { return nil }, After: func(map[string]bool) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { packages.Close() })
	return packages, store, file
}

func TestPackagePreviewIsOwnedAndDoesNotInstallOrRun(t *testing.T) {
	packages, store, file := packagePreviewFixture(t)
	params, _ := json.Marshal(map[string]string{"path": file})
	response := packages.handlePreview(host.Request{ID: "start", PeerPID: 11, Method: "packagePreview.start", Params: params}, store)
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var preview packagePreviewView
	if err := json.Unmarshal(response.Result, &preview); err != nil || preview.ID == "" {
		t.Fatal("missing preview", err)
	}
	params, _ = json.Marshal(map[string]string{"id": preview.ID})
	if other := packages.handlePreview(host.Request{ID: "other", PeerPID: 12, Method: "packagePreview.get", Params: params}, store); other.Error == nil {
		t.Fatal("another window accessed preview")
	}
	deadline := time.Now().Add(5 * time.Second)
	for preview.State == "checking" {
		time.Sleep(10 * time.Millisecond)
		response = packages.handlePreview(host.Request{ID: "get", PeerPID: 11, Method: "packagePreview.get", Params: params}, store)
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		if err := json.Unmarshal(response.Result, &preview); err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("preview did not finish")
		}
	}
	if preview.State != "ready" || !preview.CanInstall || preview.Metadata.Module.ID != "example.tool" || preview.Publisher != "example" {
		t.Fatal("verified package was not previewed", preview)
	}
	if packages.Running() {
		t.Fatal("preview ran a module")
	}
	if _, err := os.Stat(filepath.Join(packages.data, "packages")); !os.IsNotExist(err) {
		t.Fatal("preview wrote package storage", err)
	}
	if response = packages.handlePreview(host.Request{ID: "cancel", PeerPID: 11, Method: "packagePreview.cancel", Params: params}, store); response.Error != nil {
		t.Fatal(response.Error)
	}
	response = packages.handlePreview(host.Request{ID: "get", PeerPID: 11, Method: "packagePreview.get", Params: params}, store)
	_ = json.Unmarshal(response.Result, &preview)
	if preview.State != "canceled" || preview.CanInstall {
		t.Fatal("cancelled preview remained installable")
	}
}
