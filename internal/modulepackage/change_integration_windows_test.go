//go:build windows

package modulepackage

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/modulestate"
)

func TestRuntimePackageChangeUpgradeAndRollback(t *testing.T) {
	if os.Getenv("WINONE_MODULE_RUNTIME_INTEGRATION") != "1" {
		t.Skip("explicit isolated-process integration required")
	}
	for _, broken := range []bool{false, true} {
		t.Run(fmt.Sprintf("broken-%v", broken), func(t *testing.T) {
			ctx := context.Background()
			_, _, options := runtimeFixture(t)
			registry, err := OpenRegistry(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			options.State, err = modulestate.Open(filepath.Join(t.TempDir(), "modules.json"))
			if err != nil {
				t.Fatal(err)
			}
			options.Control = modules.NewControl(options.State, nil, options.Service.FeatureCatalog)
			publisher := signedRuntimeWorker(t, options, registry)
			options.Trust = func(string) (Publisher, error) { return publisher, nil }
			old, err := registry.LoadModule(ctx, "example.tool", options)
			if err != nil {
				t.Fatal(err)
			}
			runtimes := []*RuntimeModule{old}
			var handles []windows.Handle
			t.Cleanup(func() {
				failed := false
				for i := len(runtimes) - 1; i >= 0; i-- {
					if err := runtimes[i].Close(); err != nil {
						t.Error(err)
						failed = true
					}
				}
				for _, handle := range handles {
					if failed {
						if status, _ := windows.WaitForSingleObject(handle, 0); status != windows.WAIT_OBJECT_0 {
							_ = windows.TerminateProcess(handle, 1)
							_, _ = windows.WaitForSingleObject(handle, 5000)
						}
					}
					windows.CloseHandle(handle)
				}
			})
			call := func(id string) host.Response {
				return options.Service.Handle(host.Request{Version: 1, ID: id, Method: "example.tool.echo", Params: json.RawMessage(`"check"`)})
			}
			observe := func(reply host.Response, expectedDirectory string) windows.Handle {
				t.Helper()
				var output struct {
					PID  uint32 `json:"processId"`
					Echo string `json:"echo"`
				}
				if reply.Error != nil || json.Unmarshal(reply.Result, &output) != nil || output.PID == 0 || output.Echo != "check" {
					t.Fatal("worker response failed", reply)
				}
				handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, output.PID)
				if err != nil {
					t.Fatal(err)
				}
				handles = append(handles, handle)
				buffer := make([]uint16, 32768)
				size := uint32(len(buffer))
				if err = windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
					t.Fatal(err)
				}
				if actual := windows.UTF16ToString(buffer[:size]); !strings.EqualFold(actual, filepath.Join(expectedDirectory, "bin", "worker.exe")) {
					t.Fatal("worker executed from the wrong package", actual)
				}
				return handle
			}
			if job := setRuntimeEnabled(t, options, true); job.State != "completed" {
				t.Fatal(job)
			}
			first := observe(call("before"), old.directory)
			settingsPath := filepath.Join(options.DataDirectory, "user-settings.json")
			if err = os.WriteFile(settingsPath, []byte(`{"preserve":"yes"}`), 0600); err != nil {
				t.Fatal(err)
			}
			after, approval := signedRuntimeStage(t, registry, "1.3.0", broken)
			request := changeRequest(t, registry, options.State, &ReplacementStage{ID: filepath.Base(after.Directory()), Options: approval})
			if _, err = registry.BeginChange(ctx, request, options.State); err != nil {
				t.Fatal(err)
			}
			if reply := call("pending"); reply.Error == nil {
				t.Fatal("existing worker accepted work during pending change")
			}
			if job := setRuntimeEnabled(t, options, false); job.State != "completed" {
				t.Fatal(job)
			}
			if err = old.Unload(); err != nil {
				t.Fatal(err)
			}
			if status, err := windows.WaitForSingleObject(first, 0); err != nil || status != windows.WAIT_OBJECT_0 {
				t.Fatal("old worker survived replacement", status, err)
			}
			if _, err = registry.ApplyChange(ctx, request.ID, options.State, &approval); err != nil {
				t.Fatal(err)
			}
			candidateOptions := options
			candidateOptions.Trust = func(string) (Publisher, error) { return approval.Publisher, nil }
			candidateOptions.Change = &ChangePermit{ID: request.ID, RegistryRevision: request.RegistryRevision}
			candidate, err := registry.LoadModule(ctx, "example.tool", candidateOptions)
			if err != nil {
				t.Fatal(err)
			}
			runtimes = append(runtimes, candidate)
			if job := setRuntimeEnabled(t, candidateOptions, true); job.State != "completed" {
				t.Fatal(job)
			}
			reply := call("candidate")
			outcome, version := "committed", "1.3.0"
			var active windows.Handle
			if broken {
				if reply.Error == nil || candidate.Running() {
					t.Fatal("broken signed executable was accepted", reply)
				}
				if reply.Error.Code != "module_unavailable" || reply.Error.Message != candidate.worker.Status().Detail || !strings.Contains(reply.Error.Message, "worker.exe") {
					t.Fatal("native launch failure was not retained", reply.Error)
				}
				t.Logf("signed invalid executable rejected: %s", reply.Error.Message)
				if job := setRuntimeEnabled(t, candidateOptions, false); job.State != "completed" {
					t.Fatal(job)
				}
				if err = candidate.Unload(); err != nil {
					t.Fatal(err)
				}
				previous := StageOptions{Directory: registry.directory, Publisher: publisher, Environment: options.Environment, ExpectedManifestSHA256: request.Current.ManifestSHA256}
				if _, err = registry.RestoreChange(ctx, request.ID, options.State, previous); err != nil {
					t.Fatal(err)
				}
				options.Change = &ChangePermit{ID: request.ID, RegistryRevision: request.RegistryRevision}
				restored, err := registry.LoadModule(ctx, "example.tool", options)
				if err != nil {
					t.Fatal(err)
				}
				runtimes = append(runtimes, restored)
				if job := setRuntimeEnabled(t, options, true); job.State != "completed" {
					t.Fatal(job)
				}
				active = observe(call("restored"), old.directory)
				if _, err = registry.FinishChange(ctx, request.ID, options.State, &previous); err != nil {
					t.Fatal(err)
				}
				outcome, version = "restored", "1.2.0"
			} else {
				active = observe(reply, after.Directory())
				if _, err = registry.FinishChange(ctx, request.ID, options.State, &approval); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := registry.Snapshot()
			if err != nil || snapshot.Pending != nil || snapshot.LastChange.Outcome != outcome || snapshot.Records["example.tool"].Version != version {
				t.Fatal("wrong terminal package state", snapshot, err)
			}
			if !options.State.Snapshot().Modules["example.tool"].Enabled {
				t.Fatal("previous enable preference lost")
			}
			if data, err := os.ReadFile(settingsPath); err != nil || string(data) != `{"preserve":"yes"}` {
				t.Fatal("user settings changed", err)
			}
			if reply := call("public"); reply.Error != nil {
				t.Fatal("completed package remained blocked", reply)
			}
			if job := setRuntimeEnabled(t, options, false); job.State != "completed" {
				t.Fatal(job)
			}
			if status, err := windows.WaitForSingleObject(active, 0); err != nil || status != windows.WAIT_OBJECT_0 {
				t.Fatal("final worker survived disable", status, err)
			}
			t.Logf("%s: version %s, prior enabled preference and settings retained; old and final workers exited", outcome, version)
		})
	}
}
