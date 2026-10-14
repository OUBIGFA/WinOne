//go:build windows

package main

import (
	"aead.dev/minisign"
	"context"
	"crypto/rand"
	"encoding/json"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modules"
)

func TestFailedUpdateRetainsMigrationBeforePackageTransaction(t *testing.T) {
	public, private, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	old, publisher := hostPackageArchiveFixtureSigned(t, []byte("old inert worker"), public, private, "1.0.0")
	next, _ := hostPackageArchiveFixtureDefined(t, []byte("new inert worker"), public, private, hostMigrationArchiveOptions("1.1.0"))
	p, store, _ := packagePreviewFixtureFromArchive(t, old, publisher)
	service := host.New(store)
	control := modules.NewControl(p.state, nil, service.FeatureCatalog)
	if err = p.bind(service, control, store); err != nil {
		t.Fatal(err)
	}
	first := readyPackagePreview(t, p, store, old)
	raw, _ := json.Marshal(map[string]string{"id": first.ID})
	if reply := p.installPreview(host.Request{PeerPID: 11, Params: raw}, store, service, control); reply.Error != nil {
		t.Fatal(reply.Error)
	}
	approved := readyPackagePreview(t, p, store, next)
	path := filepath.Join(p.data, "packages", ".winone-module-registry.json")
	wide, _ := windows.UTF16PtrFromString(path)
	lock, err := windows.CreateFile(wide, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if lock != 0 {
			_ = windows.CloseHandle(lock)
		}
	}()
	before := store.CommittedDocument()
	raw, _ = json.Marshal(map[string]string{"id": approved.ID})
	if reply := p.updatePreview(host.Request{PeerPID: 11, Params: raw}, store, service, control); reply.Error == nil {
		t.Fatal("blocked package transaction reported success")
	}
	if store.CommittedDocument().Revision != before.Revision || p.Running() {
		t.Fatal("preparation failure changed settings or launched code")
	}
	registry, err := modulepackage.OpenRegistry(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	registered, err := registry.Snapshot()
	if err != nil || registered.Pending != nil || registered.Records["example.tool"].Version != "1.0.0" {
		t.Fatal("failure did not precede the transaction", registered, err)
	}
	ledger, err := registry.Reclamation()
	if err != nil || len(ledger.Items) != 1 {
		t.Fatal("orphaned migration lost cleanup ownership", ledger, err)
	}
	var prepared modulepackage.ReclamationItem
	for _, item := range ledger.Items {
		prepared = item
		if item.Kind != "preparation" || item.MigrationSHA256 == "" {
			t.Fatal(item)
		}
		migration, err := store.LoadPackageMigration(item.ChangeID, item.MigrationSHA256)
		if err != nil || migration.AfterPackageSHA256 != item.Package.ManifestSHA256 || string(migration.Before) != string(before.Values["example.tool"]) {
			t.Fatal("preparation did not bind the saved recovery data", err)
		}
	}
	if !p.packageCleanupStatus().Pending {
		t.Fatal("failed update cleanup is not visible")
	}
	if err = windows.CloseHandle(lock); err != nil {
		t.Fatal(err)
	}
	lock = 0
	approval := modulepackage.StageOptions{Directory: filepath.Dir(path), Publisher: publisher, Environment: p.environment, ExpectedManifestSHA256: prepared.Package.ManifestSHA256}
	change := modulepackage.ChangeRequest{ID: prepared.ChangeID, RegistryRevision: registered.Revision, StateRevision: p.state.Snapshot().Revision, Current: registered.Records["example.tool"], Replacement: &modulepackage.ReplacementStage{ID: prepared.Package.StageID, Options: approval}, MigrationSHA256: prepared.MigrationSHA256}
	if _, err = registry.BeginChange(context.Background(), change, p.state); err != nil {
		t.Fatal(err)
	}
	if reply := p.cleanupPackages(host.Request{Method: "cleanupPackages"}, store); reply.Error != nil {
		t.Fatal(reply.Error)
	}
	if _, err = store.LoadPackageMigration(prepared.ChangeID, prepared.MigrationSHA256); err != nil {
		t.Fatal("cleanup removed a pending transaction's backup", err)
	}
}

func TestFailedRegistrationRetainsVisiblePreparation(t *testing.T) {
	p, store, source := packagePreviewFixture(t)
	preview := readyPackagePreview(t, p, store, source)
	service := host.New(store)
	control := modules.NewControl(p.state, nil, service.FeatureCatalog)
	if err := p.bind(service, control, store); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(p.data, "packages")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, ".winone-module-registry.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":2,"revision":0,"records":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	wide, _ := windows.UTF16PtrFromString(path)
	lock, err := windows.CreateFile(wide, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(lock)
	before := store.CommittedDocument().Revision
	raw, _ := json.Marshal(map[string]string{"id": preview.ID})
	reply := p.installPreview(host.Request{ID: "install", PeerPID: 11, Params: raw}, store, service, control)
	if reply.Error == nil {
		t.Fatal("blocked registration reported success")
	}
	if p.Running() || store.CommittedDocument().Revision != before {
		t.Fatal("failed preparation ran code or committed configuration")
	}
	registry, err := modulepackage.OpenRegistry(directory)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := registry.Reclamation()
	if err != nil || len(ledger.Items) != 1 {
		t.Fatal("failed installation lost staged files", ledger, err)
	}
	for _, item := range ledger.Items {
		if item.Kind != "preparation" || item.Package.ModuleID != "example.tool" {
			t.Fatal(item)
		}
		if _, err := os.Stat(filepath.Join(directory, item.Package.StageID, modulepackage.StageReceiptName)); err != nil {
			t.Fatal("failure was not after staging", err)
		}
	}
	if status := p.packageCleanupStatus(); !status.Pending || status.Error != "" {
		t.Fatal("cleanup entry did not expose failed preparation", status)
	}
}
