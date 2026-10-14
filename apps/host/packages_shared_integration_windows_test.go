//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modulestate"
	"winone/internal/transport"

	"aead.dev/minisign"
	"golang.org/x/sys/windows"
)

// Actual product Host, signed fixture processes, normal asynchronous APIs and
// two Host lifetimes. All software/configuration lives below a fresh test root.
func TestProductHostSharedDependencies(t *testing.T) {
	if os.Getenv("WINONE_SHARED_HOST_TEST") != "1" {
		t.Skip("explicit isolated shared-dependency product test")
	}
	goExe := os.Getenv("WINONE_PACKAGE_HOST_GO")
	if goExe == "" {
		t.Fatal("set WINONE_PACKAGE_HOST_GO")
	}
	root := t.TempDir()
	if output := os.Getenv("WINONE_PACKAGE_HOST_OUTPUT"); output != "" {
		root = output
		if err := os.Mkdir(root, 0700); err != nil {
			t.Fatal(err)
		}
	}
	profile := filepath.Join(root, "profile")
	if err := os.Mkdir(profile, 0700); err != nil {
		t.Fatal(err)
	}
	states := modulestate.Snapshot{SchemaVersion: 2, Modules: map[string]modulestate.Record{}}
	for _, module := range catalog.Builtins() {
		states.Modules[module.ID] = modulestate.Record{}
	}
	raw, _ := json.Marshal(states)
	if err := os.WriteFile(filepath.Join(profile, "module-state.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "settings.json"), []byte(`{"schemaVersion":1,"captureShortcut":"","pinShortcut":"","ocrShortcut":"","translateShortcut":"","selectionShortcut":""}`), 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	worker, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	public, private, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publisher := modulepackage.Publisher{Namespace: "example", PublicKey: public.String()}
	files := map[string]string{}
	keep := func(name, source string) string {
		t.Helper()
		raw, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(root, name)
		if err = os.WriteFile(destination, raw, 0600); err != nil {
			t.Fatal(err)
		}
		files[name] = destination
		return destination
	}
	refs := []modulepackage.Dependency{}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		file, _ := hostPackageArchiveFixtureDefined(t, []byte("shared-"+version), public, private, hostPackageArchiveOptions{Runtime: true, Version: version})
		file = keep("shared-"+version+".winone-runtime", file)
		checked, err := modulepackage.VerifyArchive(context.Background(), file, publisher)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, modulepackage.Dependency{Kind: "runtime", ID: "example.shared", Version: version, ManifestSHA256: checked.Manifest().Digest()})
	}
	for _, id := range []string{"example.first", "example.second"} {
		file, _ := hostPackageArchiveFixtureDefined(t, worker, public, private, hostPackageArchiveOptions{ID: id, Dependencies: []modulepackage.Dependency{refs[0]}})
		keep(id+".winone-module", file)
	}
	update, _ := hostPackageArchiveFixtureDefined(t, worker, public, private, hostPackageArchiveOptions{ID: "example.first", Version: "1.1.0", Dependencies: []modulepackage.Dependency{refs[1]}})
	keep("update.winone-module", update)
	failedUpdate, _ := hostPackageArchiveFixtureDefined(t, worker, public, private, hostPackageArchiveOptions{ID: "example.first", Version: "1.2.0", Dependencies: []modulepackage.Dependency{refs[0]}})
	keep("pending-update.winone-module", failedUpdate)
	fixtureJSON, _ := json.MarshalIndent(files, "", "  ")
	if err = os.WriteFile(filepath.Join(root, "fixtures.json"), fixtureJSON, 0600); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(map[string]any{"publishers": []modulepackage.Publisher{publisher}})
	trustFile := filepath.Join(root, "publishers.json")
	if err = os.WriteFile(trustFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	workspace, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(root, "bundle")
	build := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(workspace, "build", "build-host.ps1"), "-GoExe", goExe, "-OutputDirectory", bundle, "-ModuleTrustFile", trustFile)
	build.Dir = workspace
	build.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture Host: %v\n%s", err, output)
	}
	var protection []windows.Handle
	t.Cleanup(func() {
		for _, handle := range protection {
			_ = windows.CloseHandle(handle)
		}
	})
	protect := func(path string) {
		t.Helper()
		wide, err := windows.UTF16PtrFromString(path)
		if err != nil {
			t.Fatal(err)
		}
		handle, err := windows.CreateFile(wide, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		protection = append(protection, handle)
	}
	for run := 0; run < 2; run++ {
		pipe := `\\.\pipe\WinOne-shared-host-` + rand.Text()
		command := exec.Command(filepath.Join(bundle, "WinOne.Host.exe"), "--pipe", pipe, "--data-dir", profile, "--standard-session")
		command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
		if err = command.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		finished := &atomic.Bool{}
		go func() { err := command.Wait(); finished.Store(true); done <- err }()
		t.Cleanup(func() {
			if !finished.Load() {
				_ = command.Process.Kill()
				<-done
			}
		})
		deadline := time.Now().Add(15 * time.Second)
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			_, err = transport.Call(ctx, pipe, host.Request{Version: 1, ID: "ready", Method: "snapshot"})
			cancel()
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("Host startup timed out", err)
			}
			time.Sleep(20 * time.Millisecond)
		}
		call := func(method string, value any) host.Response {
			t.Helper()
			return callSharedFixture(t, pipe, method, value)
		}
		install := func(name string, allowed bool, expectedFailure ...bool) {
			t.Helper()
			reply := call("modules.packagePreview.start", map[string]string{"path": files[name]})
			if reply.Error != nil {
				t.Fatal(reply.Error)
			}
			var preview packagePreviewView
			if err = json.Unmarshal(reply.Result, &preview); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(15 * time.Second)
			for preview.State == "checking" {
				if time.Now().After(deadline) {
					t.Fatal("preview timed out")
				}
				time.Sleep(10 * time.Millisecond)
				reply = call("modules.packagePreview.get", map[string]string{"id": preview.ID})
				if reply.Error != nil {
					t.Fatal(reply.Error)
				}
				if err = json.Unmarshal(reply.Result, &preview); err != nil {
					t.Fatal(err)
				}
			}
			if preview.CanInstall != allowed {
				t.Fatal("wrong dependency admission", preview)
			}
			if !allowed {
				if preview.Error == "" {
					t.Fatal("missing dependency had no explanation")
				}
				return
			}
			method := "modules.installPackage"
			if preview.Operation == "update" {
				method = "modules.updatePackage"
			}
			reply = call(method, map[string]string{"id": preview.ID})
			if len(expectedFailure) > 0 && expectedFailure[0] {
				if reply.Error == nil {
					t.Fatal("candidate stop refusal reported success", reply)
				}
				return
			}
			if reply.Error != nil {
				t.Fatal(reply.Error)
			}
			var result struct{ State string }
			if json.Unmarshal(reply.Result, &result) != nil || result.State != "completed" {
				t.Fatal("installation not completed", reply)
			}
		}
		setEnabled := func(id string, enabled bool) {
			t.Helper()
			reply := call("modules.snapshot", nil)
			var snapshot struct{ Revision uint64 }
			if reply.Error != nil || json.Unmarshal(reply.Result, &snapshot) != nil {
				t.Fatal(reply)
			}
			reply = call("modules.setEnabled", map[string]any{"id": id, "revision": snapshot.Revision, "enabled": enabled})
			if reply.Error != nil {
				t.Fatal(reply.Error)
			}
		}
		invoke := func(id, want string) modulepackage.DependencyLocation {
			t.Helper()
			reply := call(id+".echo", nil)
			var output struct {
				PID          uint32                          `json:"processId"`
				Dependencies modulepackage.DependencyContext `json:"dependencyContext"`
				Exports      map[string]string               `json:"exports"`
			}
			if reply.Error != nil || json.Unmarshal(reply.Result, &output) != nil || output.PID == 0 || output.Dependencies.HostPipe != pipe || len(output.Dependencies.Packages) != 1 || output.Exports["example.shared/library"] != "shared-"+want {
				t.Fatal("product Worker used the wrong shared runtime", string(reply.Result), reply.Error)
			}
			return output.Dependencies.Packages[0]
		}
		if run == 0 {
			install("example.first.winone-module", false)
			install("shared-1.0.0.winone-runtime", true)
			install("shared-2.0.0.winone-runtime", true)
			install("example.first.winone-module", true)
			install("example.second.winone-module", true)
			setEnabled("example.first", true)
			setEnabled("example.second", true)
			first, second := invoke("example.first", "1.0.0"), invoke("example.second", "1.0.0")
			if first.Directory != second.Directory {
				t.Fatal("one runtime version was copied per consumer")
			}
			registry, err := modulepackage.OpenRegistry(filepath.Join(profile, "packages"))
			if err != nil {
				t.Fatal(err)
			}
			inventory, err := registry.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			// Keep retired fixture files in this workspace instead of exercising
			// the system recycle bin. The UI must report pending cleanup honestly.
			protect(filepath.Join(profile, "packages", inventory.Records["example.first"].StageID, "LICENSE.txt"))
			install("update.winone-module", true)
			invoke("example.first", "2.0.0")
			invoke("example.second", "1.0.0")
			// A candidate using v1 refuses restoration. Keep both dependency
			// generations referenced until the real recovery transaction finishes.
			refusal := filepath.Join(profile, "modules", "example.first", "refuse-version-shutdown")
			if err = os.WriteFile(refusal, []byte("1.2.0"), 0600); err != nil {
				t.Fatal(err)
			}
			install("pending-update.winone-module", true, true)
			inventory, err = registry.Snapshot()
			if err != nil || inventory.Pending == nil || inventory.Pending.After == nil {
				t.Fatal("failed candidate lost its recovery record", inventory, err)
			}
			protect(filepath.Join(profile, "packages", inventory.Pending.After.StageID, "LICENSE.txt"))
			protect(filepath.Join(profile, "packages", inventory.Pending.Before.StageID, "LICENSE.txt"))
			if err = os.Remove(refusal); err != nil {
				t.Fatal(err)
			}
			reply := call("modules.snapshot", nil)
			var recovering struct {
				Revision uint64
				Modules  []struct {
					Manifest   struct{ ID string }
					RecoveryID string
				}
			}
			if reply.Error != nil || json.Unmarshal(reply.Result, &recovering) != nil {
				t.Fatal(reply)
			}
			recoveryID := ""
			for _, entry := range recovering.Modules {
				if entry.Manifest.ID == "example.first" {
					recoveryID = entry.RecoveryID
				}
			}
			if recoveryID == "" {
				t.Fatal("pending dependency update was not exposed for recovery")
			}
			reply = call("modules.recoverPackage", map[string]any{"id": "example.first", "revision": recovering.Revision, "recoveryId": recoveryID})
			if reply.Error != nil {
				t.Fatal(reply.Error)
			}
			invoke("example.first", "2.0.0")
			invoke("example.second", "1.0.0")
		} else {
			first, second := invoke("example.first", "2.0.0"), invoke("example.second", "1.0.0")
			if first.Directory == second.Directory {
				t.Fatal("distinct runtime versions were merged")
			}
			samplePackageHost(t, workspace, root, command.Process.Pid, "shared-active", filepath.Join(bundle, "WinOne.Host.exe"))
			for _, id := range []string{"example.first", "example.second"} {
				setEnabled(id, false)
				registry, err := modulepackage.OpenRegistry(filepath.Join(profile, "packages"))
				if err != nil {
					t.Fatal(err)
				}
				inventory, err := registry.Snapshot()
				if err != nil {
					t.Fatal(err)
				}
				protect(filepath.Join(profile, "packages", inventory.Records[id].StageID, "LICENSE.txt"))
				reply := call("modules.packageRemoval.preview", map[string]string{"id": id})
				if reply.Error != nil {
					t.Fatal(reply.Error)
				}
				var preview packagePreviewView
				if err = json.Unmarshal(reply.Result, &preview); err != nil {
					t.Fatal(err)
				}
				reply = call("modules.removePackage", map[string]string{"id": preview.ID})
				if reply.Error != nil {
					t.Fatal(reply.Error)
				}
				if id == "example.first" {
					invoke("example.second", "1.0.0")
				}
			}
			for _, location := range []modulepackage.DependencyLocation{first, second} {
				protect(filepath.Join(location.Directory, "LICENSE.txt"))
				reply := call("modules.packageRemoval.preview", map[string]string{"id": location.ID, "kind": "runtime", "manifestSha256": location.ManifestSHA256})
				if reply.Error != nil {
					t.Fatal(reply.Error)
				}
				var preview packagePreviewView
				if err = json.Unmarshal(reply.Result, &preview); err != nil {
					t.Fatal(err)
				}
				reply = call("modules.removePackage", map[string]string{"id": preview.ID})
				if reply.Error != nil {
					t.Fatal(reply.Error)
				}
				var result struct{ State, Warning string }
				if json.Unmarshal(reply.Result, &result) != nil || result.State != "completed" || result.Warning == "" {
					t.Fatal("held runtime cleanup was not reported", string(reply.Result))
				}
			}
			samplePackageHost(t, workspace, root, command.Process.Pid, "shared-disabled", filepath.Join(bundle, "WinOne.Host.exe"))
		}
		family := observePackageProcessFamily(t, uint32(command.Process.Pid))
		requestPackageHostExit(t, uint32(command.Process.Pid))
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("Host did not exit")
		}
		for _, member := range family {
			status, err := windows.WaitForSingleObject(member.handle, 10000)
			windows.CloseHandle(member.handle)
			if err != nil || status != windows.WAIT_OBJECT_0 {
				t.Fatal("shared dependency process survived Host exit", member.pid, status, err)
			}
		}
	}
	fmt.Println("product Host: missing-dependency refusal, shared consumers, side-by-side update, failed-candidate recovery, offline restart, uninstall protection and process-family exit verified")
}

func callSharedFixture(t *testing.T, pipe, method string, value any) host.Response {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	reply, err := transport.Call(ctx, pipe, host.Request{Version: 1, ID: "shared-fixture", Method: method, Params: raw})
	if err != nil {
		t.Fatal(err)
	}
	var job struct {
		ID string `json:"configurationJob"`
	}
	_ = json.Unmarshal(reply.Result, &job)
	for job.ID != "" {
		params, _ := json.Marshal(map[string]string{"id": job.ID})
		response, err := transport.Call(ctx, pipe, host.Request{Version: 1, ID: "job", Method: "configuration.jobs.get", Params: params})
		if err != nil {
			t.Fatal(err)
		}
		var status host.ConfigurationJob
		if response.Error != nil || json.Unmarshal(response.Result, &status) != nil {
			t.Fatal(response)
		}
		if status.State != "running" {
			if status.Response == nil {
				t.Fatal("missing configuration job result")
			}
			return *status.Response
		}
		time.Sleep(20 * time.Millisecond)
	}
	return reply
}
