//go:build windows

package modulepackage

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/modulestate"

	"golang.org/x/sys/windows"
)

// Signed fixture workers read the real context and exported file on every RPC.
// Removing either lease, passing a wrong path or launching without the context
// breaks the consumer-visible assertions below.
func TestDependencyRuntimeTwoWorkersShareAndRelease(t *testing.T) {
	if os.Getenv("WINONE_MODULE_RUNTIME_INTEGRATION") != "1" {
		t.Skip("explicit isolated worker integration")
	}
	ctx := context.Background()
	registry, err := OpenRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := OpenRuntimeStore(registry.directory)
	if err != nil {
		t.Fatal(err)
	}
	base, publisher, key := signedTestManifest(t)
	shared := base
	shared.Format = "winone.runtime.v1"
	shared.Metadata = runtimeMetadataFixture()
	archive := writeRuntimeArchive(t, shared, publisher, key)
	verified, err := VerifyArchive(ctx, archive, publisher)
	if err != nil {
		t.Fatal(err)
	}
	reference := graphReference(verified.Manifest())
	if _, err = runtimes.Install(ctx, archive, RuntimeInstallOptions{Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: reference.ManifestSHA256}); err != nil {
		t.Fatal(err)
	}
	source, err := NewDependencySource(registry, runtimes, testEnvironment(), func(string) (Publisher, error) { return publisher, nil })
	if err != nil {
		t.Fatal(err)
	}
	_, _, options := runtimeFixture(t)
	options.State, err = modulestate.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	options.Control = modules.NewControl(options.State, nil, options.Service.FeatureCatalog)
	options.Trust = func(string) (Publisher, error) { return publisher, nil }
	options.Dependencies = source
	options.HostPipe = `\\.\pipe\WinOne-dependency-test-host`
	workers := map[string]*RuntimeModule{}
	handles := map[string]windows.Handle{}
	t.Cleanup(func() {
		for _, worker := range workers {
			if err := worker.Close(); err != nil {
				t.Error(err)
			}
		}
		for _, handle := range handles {
			if status, _ := windows.WaitForSingleObject(handle, 0); status != windows.WAIT_OBJECT_0 {
				t.Error("owned fixture process did not exit")
				_ = windows.TerminateProcess(handle, 1)
				_, _ = windows.WaitForSingleObject(handle, 5000)
			}
			_ = windows.CloseHandle(handle)
		}
	})
	for _, id := range []string{"example.first", "example.second"} {
		manifest := base
		manifest.Metadata.SchemaVersion = 4
		manifest.Metadata.Kind = "module"
		manifest.Metadata.Module.ID = id
		manifest.Metadata.Dependencies = []Dependency{reference}
		manifest.Metadata.Settings = &SettingsFiles{Schema: "schema.json", Defaults: "defaults.json"}
		stage, approval := signedRuntimeStageDefined(t, registry, manifest, publisher, key, false)
		approval.Dependencies = source
		inventory, err := registry.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = registry.PrepareModule(ctx, filepath.Base(stage.Directory()), approval, inventory.Revision, options.State); err != nil {
			t.Fatal(err)
		}
		local := options
		local.Pipe = `\\.\pipe\WinOne-package-test-` + rand.Text()
		local.DataDirectory = t.TempDir()
		worker, err := registry.LoadModule(ctx, id, local)
		if err != nil {
			t.Fatal(err)
		}
		workers[id] = worker
		if err = worker.LifecycleGate().SetEnabled(options.State.Snapshot().Revision, true); err != nil {
			t.Fatal(err)
		}
		if worker.Running() {
			t.Fatal("enabling eagerly started a dependency consumer")
		}
	}
	call := func(id string) DependencyLocation {
		t.Helper()
		reply := options.Service.Handle(host.Request{Version: 1, ID: "shared", Method: id + ".echo", Params: json.RawMessage(`"hello"`)})
		var output struct {
			ProcessID    uint32            `json:"processId"`
			Dependencies DependencyContext `json:"dependencies"`
			Contents     map[string]string `json:"contents"`
		}
		if reply.Error != nil || json.Unmarshal(reply.Result, &output) != nil {
			t.Fatal("dependency RPC failed", reply)
		}
		if output.ProcessID == 0 || output.Dependencies.Root.ID != id || output.Dependencies.HostPipe != options.HostPipe || len(output.Dependencies.Packages) != 1 || output.Contents["example.shared/library"] != "inert worker" {
			t.Fatal("worker did not read the pinned shared export", output)
		}
		if handles[id] == 0 {
			h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, output.ProcessID)
			if err != nil {
				t.Fatal(err)
			}
			handles[id] = h
		}
		location := output.Dependencies.Packages[0]
		if location.Dependency != reference {
			t.Fatal("worker used an unpinned dependency", location)
		}
		return location
	}
	first, second := call("example.first"), call("example.second")
	if first.Directory != second.Directory {
		t.Fatal("consumers received separate copies of one version")
	}
	remove := func(id string) {
		t.Helper()
		worker := workers[id]
		if err := worker.LifecycleGate().SetEnabled(options.State.Snapshot().Revision, false); err != nil {
			t.Fatal(err)
		}
		if err := worker.Unload(); err != nil {
			t.Fatal(err)
		}
		if status, err := windows.WaitForSingleObject(handles[id], 0); err != nil || status != windows.WAIT_OBJECT_0 {
			t.Fatal("worker survived disable", status, err)
		}
		inventory, err := registry.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		change := ChangeRequest{ID: "remove-" + id, RegistryRevision: inventory.Revision, StateRevision: options.State.Snapshot().Revision, Current: inventory.Records[id], Dependencies: source}
		if _, err = registry.BeginChange(ctx, change, options.State); err != nil {
			t.Fatal(err)
		}
		if _, err = registry.ApplyChange(ctx, change.ID, options.State, nil); err != nil {
			t.Fatal(err)
		}
		if _, err = registry.FinishChange(ctx, change.ID, options.State, nil); err != nil {
			t.Fatal(err)
		}
	}
	remove("example.first")
	if removed, err := runtimes.Remove(ctx, reference, source, publisher); err == nil || removed {
		t.Fatal("remaining consumer lost shared runtime")
	}
	call("example.second")
	export := filepath.Join(first.Directory, "bin", "worker.exe")
	if file, err := os.OpenFile(export, os.O_WRONLY, 0); err == nil {
		file.Close()
		t.Fatal("remaining consumer lost shared file protection")
	}
	remove("example.second")
	file, err := os.OpenFile(export, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal("last consumer did not release shared files", err)
	}
	file.Close()
	if removed, err := runtimes.Remove(ctx, reference, source, publisher); err != nil || !removed {
		t.Fatal("unused runtime could not be removed", removed, err)
	}
}
