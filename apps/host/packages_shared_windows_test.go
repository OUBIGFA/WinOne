//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modules"
	"winone/internal/startupprobe"

	"aead.dev/minisign"
)

func TestSharedRuntimeHostInstallAndDependencyAdmission(t *testing.T) {
	public, private, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	file, publisher := hostPackageArchiveFixtureDefined(t, []byte("shared export"), public, private, hostPackageArchiveOptions{Runtime: true})
	p, store, _ := packagePreviewFixtureFromArchive(t, file, publisher)
	service := host.New(store)
	control := modules.NewControl(p.state, nil, service.FeatureCatalog)
	if err = p.bind(service, control, store); err != nil {
		t.Fatal(err)
	}
	configurationRevision, stateRevision := store.CommittedDocument().Revision, p.state.Snapshot().Revision
	preview := readyPackagePreview(t, p, store, file)
	if preview.Metadata.PackageKind() != "runtime" || preview.Operation != "install" {
		t.Fatal("shared runtime preview lost its identity", preview)
	}
	raw, _ := json.Marshal(map[string]string{"id": preview.ID})
	if reply := p.installPreview(host.Request{ID: "install-runtime", PeerPID: 11, Params: raw}, store, service, control); reply.Error != nil {
		t.Fatal(reply.Error)
	}
	if store.CommittedDocument().Revision != configurationRevision || p.state.Snapshot().Revision != stateRevision || len(control.Snapshot().Modules) != 0 || p.Running() {
		t.Fatal("shared runtime acquired module configuration, toggle or a process")
	}
	runtimes, err := modulepackage.OpenRuntimeStore(filepath.Join(p.data, "packages"))
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := runtimes.Snapshot()
	if err != nil || len(inventory.Records) != 1 {
		t.Fatal(inventory, err)
	}
	reference := modulepackage.Dependency{Kind: "runtime", ID: "example.shared", Version: "1.0.0", ManifestSHA256: preview.ManifestSHA256}
	moduleFile, _ := hostPackageArchiveFixtureDefined(t, []byte("inert consumer"), public, private, hostPackageArchiveOptions{Dependencies: []modulepackage.Dependency{reference}})
	consumer := readyPackagePreview(t, p, store, moduleFile)
	raw, _ = json.Marshal(map[string]string{"id": consumer.ID})
	if reply := p.installPreview(host.Request{ID: "install-consumer", PeerPID: 11, Params: raw}, store, service, control); reply.Error != nil {
		t.Fatal(reply.Error)
	}
	if entry := p.byID["example.tool"]; entry == nil || entry.failure != nil || entry.runtime == nil {
		t.Fatal("dependent module was not admitted", entry)
	}
	if p.Running() || len(control.Snapshot().Modules) != 1 {
		t.Fatal("dependency install started code or published a second module")
	}
	opened, err := discoverInstalledPackages(context.Background(), p.data, p.pipe, p.state, p.environment, p.trust, p.guard)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if entry := opened.byID["example.tool"]; entry == nil || entry.failure != nil {
		t.Fatal("dependency did not survive offline discovery", entry)
	}
	raw, _ = json.Marshal(map[string]string{"id": reference.ID, "kind": "runtime", "manifestSha256": reference.ManifestSHA256})
	if reply := p.removalPreview(host.Request{ID: "remove-shared", PeerPID: 11, Params: raw}, store); reply.Error == nil || !strings.Contains(reply.Error.Message, "使用") {
		t.Fatal("referenced runtime became removable")
	}
	reply := moduleConfigurationHandler(store, p.state, control, p.moduleHandler(service, control, store))(host.Request{Version: 1, ID: "inventory", Method: "snapshot"})
	var status struct {
		Packages struct {
			Runtimes []struct {
				ID         string                     `json:"id"`
				RequiredBy []modulepackage.Dependency `json:"requiredBy"`
				CanRemove  bool                       `json:"canRemove"`
			} `json:"runtimes"`
		} `json:"packages"`
	}
	if reply.Error != nil || json.Unmarshal(reply.Result, &status) != nil || len(status.Packages.Runtimes) != 1 || status.Packages.Runtimes[0].ID != "example.shared" || len(status.Packages.Runtimes[0].RequiredBy) != 1 || status.Packages.Runtimes[0].CanRemove {
		t.Fatal("runtime inventory did not identify its consumer", string(reply.Result), reply.Error)
	}
	duplicate := checkedPackagePreview(t, p, store, file)
	if duplicate.CanInstall || duplicate.Error == "" {
		t.Fatal("same shared version was offered as a new install")
	}
	// The regular profile probe must understand these declarations without
	// starting either the module or its shared export.
	var profile struct {
		Committed configuration.Document `json:"committed"`
	}
	if err = json.Unmarshal(probeConfiguration(t), &profile); err != nil {
		t.Fatal(err)
	}
	profile.Committed.Values["example.tool"] = store.CommittedDocument().Values["example.tool"]
	profile.Committed.ModuleEnabled["example.tool"] = false
	raw, err = json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(p.data, "configuration.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	input, err := startupprobe.ReadProfileSnapshot(p.data)
	if err != nil {
		t.Fatal(err)
	}
	if issues, err := checkProfileUpgrade(context.Background(), p.data, input, p.environment, p.trust); err != nil || len(issues) != 0 {
		t.Fatal("shared dependency upgrade check failed", err)
	}
	newer := p.environment
	newer.HostVersion = "2.0.0"
	if _, err = checkProfileUpgrade(context.Background(), p.data, input, newer, p.trust); err == nil || !strings.Contains(err.Error(), "example.shared") {
		t.Fatal("incompatible shared runtime was omitted", err)
	}
	record := inventory.Records[reference.ManifestSHA256]
	if err = os.WriteFile(filepath.Join(p.data, "packages", "shared-runtimes", record.StageID, "worker.exe"), []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	if issues, err := checkProfileUpgrade(context.Background(), p.data, input, p.environment, p.trust); err != nil || len(issues) != 2 {
		t.Fatal("damaged shared export was not isolated with its consumer", issues, err)
	}
}

func TestSharedRuntimeInventoryChecksReferencesInOnePass(t *testing.T) {
	public, private, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	file, publisher := hostPackageArchiveFixtureDefined(t, []byte("shared export"), public, private, hostPackageArchiveOptions{Runtime: true})
	p, store, _ := packagePreviewFixtureFromArchive(t, file, publisher)
	service := host.New(store)
	control := modules.NewControl(p.state, nil, service.FeatureCatalog)
	for _, version := range []string{"1.0.0", "2.0.0", "10.0.0", "3.0.0"} {
		file, _ := hostPackageArchiveFixtureDefined(t, []byte(version), public, private, hostPackageArchiveOptions{Runtime: true, Version: version})
		preview := readyPackagePreview(t, p, store, file)
		raw, _ := json.Marshal(map[string]string{"id": preview.ID})
		if reply := p.installPreview(host.Request{ID: "install", PeerPID: 11, Params: raw}, store, service, control); reply.Error != nil {
			t.Fatal(reply.Error)
		}
	}
	// Count the real trust/verification boundary, not a mocked inventory. Four
	// display manifests plus one four-package reference pass need eight lookups.
	lookups := 0
	p.trust = func(string) (modulepackage.Publisher, error) { lookups++; return publisher, nil }
	view := p.packageInventory()
	if view.Error != "" || len(view.Runtimes) != 4 {
		t.Fatal("runtime inventory failed", view)
	}
	for index, version := range []string{"1.0.0", "2.0.0", "3.0.0", "10.0.0"} {
		if view.Runtimes[index].Version != version {
			t.Fatal("runtime versions are not in semantic order", view.Runtimes)
		}
	}
	for _, row := range view.Runtimes {
		if !row.CanRemove || row.Error != "" || len(row.RequiredBy) != 0 {
			t.Fatal("wrong unused runtime state", row)
		}
	}
	if lookups > 8 {
		t.Fatalf("reference metadata was repeatedly reverified for the same snapshot: %d lookups", lookups)
	}
}
