//go:build windows

package main

import (
	"aead.dev/minisign"
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/catalog"
	"winone/internal/configuration"
	"winone/internal/distribution"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modules"
	"winone/internal/modulestate"
)

var packageTestEnvironment = modulepackage.Environment{HostVersion: "0.0.1", ProtocolVersion: 1, DesignSystemVersion: 1, Platform: "windows-x64", WindowsBuild: 22631}

func hostPackageArchiveFixture(t *testing.T, worker []byte, version ...string) (string, modulepackage.Publisher) {
	t.Helper()
	public, private, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return hostPackageArchiveFixtureSigned(t, worker, public, private, version...)
}

// Reuse one test publisher across versions, as a real module update does.
func hostPackageArchiveFixtureSigned(t *testing.T, worker []byte, public minisign.PublicKey, private minisign.PrivateKey, version ...string) (string, modulepackage.Publisher) {
	options := hostPackageArchiveOptions{}
	if len(version) > 0 {
		options.Version = version[0]
	}
	return hostPackageArchiveFixtureDefined(t, worker, public, private, options)
}

type hostPackageArchiveOptions struct {
	Version, Schema, Defaults string
	SchemaVersion             int
	Migrations                []modulepackage.SettingsMigration
	Patches                   map[string][]byte
	Dependencies              []modulepackage.Dependency
	Runtime                   bool
	ID                        string
}

func hostPackageArchiveFixtureDefined(t *testing.T, worker []byte, public minisign.PublicKey, private minisign.PrivateKey, options hostPackageArchiveOptions) (string, modulepackage.Publisher) {
	t.Helper()
	publisher := modulepackage.Publisher{Namespace: "example", PublicKey: public.String()}
	files := map[string][]byte{"worker.exe": worker, "LICENSE.txt": []byte("fixture license"), "settings.schema.json": []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"size":{"type":"integer","minimum":1,"maximum":20}},"required":["size"],"additionalProperties":false}`), "settings.defaults.json": []byte(`{"size":3}`)}
	manifest := modulepackage.Manifest{Format: "winone.module.v1", Metadata: modulepackage.Metadata{SchemaVersion: 2, Version: "1.0.0", Module: catalog.Manifest{ManifestVersion: 1, ID: "example.tool", Name: catalog.LocalizedName{Chinese: "测试工具", English: "Test tool"}, Distribution: "optional", Worker: "worker.exe", Activation: "on-demand", SettingsSchemaVersion: 1}, Compatibility: modulepackage.Compatibility{MinimumHostVersion: "0.0.1", MaximumHostVersionExclusive: "2.0.0", ProtocolVersion: 1, DesignSystemVersion: 1, Platform: "windows-x64", MinimumWindowsBuild: 22000}, Settings: &modulepackage.SettingsFiles{Schema: "settings.schema.json", Defaults: "settings.defaults.json"}}}
	if options.Version != "" {
		manifest.Metadata.Version = options.Version
	}
	if options.SchemaVersion > 0 {
		manifest.Metadata.Module.SettingsSchemaVersion = options.SchemaVersion
	}
	if options.Schema != "" {
		files["settings.schema.json"] = []byte(options.Schema)
	}
	if options.Defaults != "" {
		files["settings.defaults.json"] = []byte(options.Defaults)
	}
	if len(options.Migrations) > 0 {
		manifest.Metadata.SchemaVersion = 3
		manifest.Metadata.Settings.Migrations = options.Migrations
	}
	if len(options.Dependencies) > 0 {
		manifest.Metadata.SchemaVersion = 4
		manifest.Metadata.Kind = "module"
		manifest.Metadata.Dependencies = options.Dependencies
	}
	if options.ID != "" {
		manifest.Metadata.Module.ID = options.ID
	}
	if options.Runtime {
		manifest.Format = "winone.runtime.v1"
		manifest.Metadata.SchemaVersion = 4
		manifest.Metadata.Kind = "runtime"
		manifest.Metadata.Runtime = &modulepackage.RuntimeManifest{ID: "example.shared", Name: catalog.LocalizedName{Chinese: "共享依赖", English: "Shared dependency"}, Exports: []modulepackage.RuntimeExport{{Name: "library", Path: "worker.exe", Kind: "file"}}}
		if options.ID != "" {
			manifest.Metadata.Runtime.ID = options.ID
		}
		manifest.Metadata.Module = catalog.Manifest{}
		manifest.Metadata.Settings = nil
		delete(files, "settings.schema.json")
		delete(files, "settings.defaults.json")
	}
	for name, body := range options.Patches {
		files[name] = body
	}
	for name, raw := range files {
		sum := sha256.Sum256(raw)
		manifest.Files = append(manifest.Files, distribution.File{Path: name, Size: int64(len(raw)), SHA256: hex.EncodeToString(sum[:])})
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	files["module.json"] = raw
	files["module.json.minisig"] = minisign.Sign(private, raw)
	var buffer bytes.Buffer
	zipper := zip.NewWriter(&buffer)
	for name, body := range files {
		entry, err := zipper.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err = zipper.Close(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "fixture.winone-module")
	if err = os.WriteFile(archive, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return archive, publisher
}

func installHostPackageFixture(t *testing.T, root string, state *modulestate.Store, worker []byte, version ...string) modulepackage.Publisher {
	t.Helper()
	archive, publisher := hostPackageArchiveFixture(t, worker, version...)
	directory := filepath.Join(root, "packages")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	checked, err := modulepackage.VerifyArchive(context.Background(), archive, publisher)
	if err != nil {
		t.Fatal(err)
	}
	options := modulepackage.StageOptions{Directory: directory, Publisher: publisher, Environment: packageTestEnvironment, ExpectedManifestSHA256: checked.Manifest().Digest()}
	stage, err := modulepackage.StageArchive(context.Background(), archive, options)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := modulepackage.OpenRegistry(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = registry.PrepareModule(context.Background(), filepath.Base(stage.Directory()), options, 0, state); err != nil {
		t.Fatal(err)
	}
	return publisher
}

func TestInstalledPackageDiscoveryPreparesConfigurationWithoutLaunchingCode(t *testing.T) {
	root := t.TempDir()
	state, err := modulestate.Open(filepath.Join(root, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	publisher := installHostPackageFixture(t, root, state, []byte("inert worker"))
	packages, err := discoverInstalledPackages(context.Background(), root, `\\.\pipe\WinOne-package-discovery`, state, packageTestEnvironment, func(string) (modulepackage.Publisher, error) { return publisher, nil }, func() (func(), bool) { return func() {}, true })
	if err != nil {
		t.Fatal(err)
	}
	defer packages.Close()
	gates := map[string]*modules.Gate{}
	hooks := modules.LifecycleHooks{Start: func() error { return nil }, Stop: func() error { return nil }, IntentChanged: func() error { return nil }, Status: func() host.Module { return host.Module{State: "ready"} }, Handle: func(r host.Request) host.Response { return host.Response{ID: r.ID} }}
	for _, manifest := range catalog.Builtins() {
		gate, err := modules.NewGate(manifest.ID, manifest.Name.Chinese, state, hooks)
		if err != nil {
			t.Fatal(err)
		}
		gates[manifest.ID] = gate
	}
	packages.addGates(gates)
	var service *host.Service
	control := modules.NewControl(state, gates, func() []catalog.Manifest {
		if service == nil {
			return catalog.Builtins()
		}
		return service.FeatureCatalog()
	})
	store, err := configuration.OpenHostWithLifecycle(root, nativeModuleLifecycle(state, control.LifecycleGates), packages.adapters()...)
	if err != nil {
		t.Fatal(err)
	}
	service = host.New(store)
	if err = packages.bind(service, control, store); err != nil {
		t.Fatal(err)
	}
	if packages.Running() {
		t.Fatal("discovery launched code")
	}
	reply := service.Handle(host.Request{Version: 1, ID: "read", Method: "packages.settings.get", Params: json.RawMessage(`{"id":"example.tool"}`)})
	if reply.Error != nil || !bytes.Contains(reply.Result, []byte(`"size":3`)) {
		t.Fatal("disabled module settings unavailable", reply.Error)
	}
	if packages.Running() {
		t.Fatal("reading settings launched code")
	}
	projection, err := os.ReadFile(filepath.Join(root, "modules", "example.tool", "settings.json"))
	if err != nil || !bytes.Contains(projection, []byte(`"size":3`)) {
		t.Fatal("canonical settings were not projected", err)
	}
}

func TestModuleTrustComesOnlyFromBuildConfiguration(t *testing.T) {
	lookup, err := moduleTrust("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lookup("example"); err == nil {
		t.Fatal("default build trusted an external publisher")
	}
	public, _, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	valid := modulepackage.Publisher{Namespace: "example", PublicKey: public.String()}
	encode := func(publishers []modulepackage.Publisher) string {
		raw, _ := json.Marshal(map[string]any{"publishers": publishers})
		return base64.StdEncoding.EncodeToString(raw)
	}
	lookup, err = moduleTrust(encode([]modulepackage.Publisher{valid}))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := lookup("example"); err != nil || got != valid {
		t.Fatal("configured publisher missing", err)
	}
	if _, err = lookup("another"); err == nil {
		t.Fatal("publisher namespace escaped trust boundary")
	}
	if _, err = moduleTrust(encode([]modulepackage.Publisher{valid, valid})); err == nil {
		t.Fatal("duplicate publishers accepted")
	}
	valid.PublicKey = "invalid"
	if _, err = moduleTrust(encode([]modulepackage.Publisher{valid})); err == nil {
		t.Fatal("invalid trust key accepted")
	}
}

func TestInstalledPackageDiscoveryRejectsUntrustedPublisher(t *testing.T) {
	root := t.TempDir()
	state, err := modulestate.Open(filepath.Join(root, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	installHostPackageFixture(t, root, state, []byte("inert worker"))
	packages, err := discoverInstalledPackages(context.Background(), root, `\\.\pipe\WinOne-package-discovery`, state, packageTestEnvironment, func(string) (modulepackage.Publisher, error) {
		return modulepackage.Publisher{}, modulepackage.ErrUntrustedPublisher
	}, func() (func(), bool) { return func() {}, true })
	if err != nil || len(packages.entries) != 1 || packages.entries[0].failure == nil || packages.entries[0].runtime != nil {
		t.Fatal("untrusted package was executable or stopped discovery", err)
	}
	if _, err = os.Stat(filepath.Join(root, "modules", "example.tool", "settings.json")); !os.IsNotExist(err) {
		t.Fatal("untrusted package changed configuration")
	}
}

func openFaultPackageHost(t *testing.T, root string, state *modulestate.Store, publisher modulepackage.Publisher) (*installedPackages, *configuration.HostStore, *host.Service, *modules.Control) {
	t.Helper()
	packages, err := discoverInstalledPackages(context.Background(), root, `\\.\pipe\WinOne-quarantine`, state, packageTestEnvironment, func(string) (modulepackage.Publisher, error) { return publisher, nil }, func() (func(), bool) { return func() {}, true })
	if err != nil {
		t.Fatal(err)
	}
	hooks := modules.LifecycleHooks{Start: func() error { return nil }, Stop: func() error { return nil }, IntentChanged: func() error { return nil }, Status: func() host.Module { return host.Module{State: "ready"} }, Handle: func(r host.Request) host.Response { return host.Response{ID: r.ID} }}
	gates := map[string]*modules.Gate{}
	for _, manifest := range catalog.Builtins() {
		gate, err := modules.NewGate(manifest.ID, manifest.Name.Chinese, state, hooks)
		if err != nil {
			t.Fatal(err)
		}
		gates[manifest.ID] = gate
	}
	packages.addGates(gates)
	var service *host.Service
	control := modules.NewControl(state, gates, func() []catalog.Manifest {
		if service == nil {
			return catalog.Builtins()
		}
		return service.FeatureCatalog()
	})
	store, err := configuration.OpenHostWithLifecycle(root, nativeModuleLifecycle(state, control.LifecycleGates, packages.isolated), packages.adapters()...)
	if err != nil {
		t.Fatal("fault prevented configuration loading", err)
	}
	service = host.New(store)
	for _, manifest := range catalog.Builtins() {
		gate := gates[manifest.ID]
		service.RegisterModule(manifest.ID, gate.Status, gate.Handle)
	}
	if err = packages.bind(service, control, store); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := packages.Close(); err != nil {
			t.Error(err)
		}
	})
	return packages, store, service, control
}

func TestFaultedPackagesKeepHostUsableAndPreservePendingState(t *testing.T) {
	for _, failure := range []string{"payload", "registry", "settings", "pending-stop", "valid-pending-stop"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			state, err := modulestate.Open(filepath.Join(root, "module-state.json"))
			if err != nil {
				t.Fatal(err)
			}
			publisher := installHostPackageFixture(t, root, state, []byte("inert worker"))
			_, healthy, _, _ := openFaultPackageHost(t, root, state, publisher)
			next := healthy.CommittedDocument()
			next.ModuleEnabled["example.tool"] = true
			if _, err = healthy.CommitDocument(next); err != nil {
				t.Fatal(err)
			}
			registry, err := modulepackage.OpenRegistry(filepath.Join(root, "packages"))
			if err != nil {
				t.Fatal(err)
			}
			inventory, err := registry.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "payload", "pending-stop":
				if err = os.WriteFile(filepath.Join(root, "packages", inventory.Records["example.tool"].StageID, "worker.exe"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "registry":
				if err = os.WriteFile(filepath.Join(root, "packages", ".winone-module-registry.json"), []byte(`{"broken":true}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "settings":
				raw, err := os.ReadFile(filepath.Join(root, "configuration.json"))
				if err != nil {
					t.Fatal(err)
				}
				var doc map[string]json.RawMessage
				_ = json.Unmarshal(raw, &doc)
				var committed configuration.Document
				_ = json.Unmarshal(doc["committed"], &committed)
				committed.Values["example.tool"] = json.RawMessage(`{"size":99}`)
				doc["committed"], _ = json.Marshal(committed)
				raw, _ = json.Marshal(doc)
				if err = os.WriteFile(filepath.Join(root, "configuration.json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if failure == "pending-stop" || failure == "valid-pending-stop" {
				if _, err = state.SetIntent(state.Snapshot().Revision, "example.tool", false); err != nil {
					t.Fatal(err)
				}
			}
			before := state.Snapshot().Modules["example.tool"]
			packages, store, service, control := openFaultPackageHost(t, root, state, publisher)
			if packages.Running() {
				t.Fatal("fault started worker")
			}
			original := append([]byte(nil), store.CommittedDocument().Values["example.tool"]...)
			settings := store.Snapshot()
			settings.Theme = "dark"
			if _, err = store.Save(settings); err != nil {
				t.Fatal("fault blocked unrelated settings", err)
			}
			if !bytes.Equal(original, store.CommittedDocument().Values["example.tool"]) || state.Snapshot().Modules["example.tool"] != before {
				t.Fatal("fault cleared data or recovery marker")
			}
			rows := control.Snapshot().Modules
			found := false
			for _, row := range rows {
				if row.Manifest.ID == "example.tool" {
					found = row.Blocked && row.Runtime.State == "unavailable" && row.Runtime.Detail != ""
				}
			}
			if !found {
				t.Fatal("fault missing from tool management")
			}
			if response := service.Handle(host.Request{Version: 1, ID: "blocked", Method: "example.tool.echo"}); response.Error == nil {
				t.Fatal("quarantined route executable")
			}
		})
	}
}
