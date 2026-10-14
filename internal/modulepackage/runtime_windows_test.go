//go:build windows

package modulepackage

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/modulestate"
	"winone/internal/settings"
)

func runtimeFixture(t *testing.T) (*Registry, *StagedPackage, RuntimeOptions) {
	t.Helper()
	registry, stage, stageOptions := registeredFixture(t)
	state, err := modulestate.Open(filepath.Join(t.TempDir(), "modules.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = registry.PrepareModule(context.Background(), filepath.Base(stage.Directory()), stageOptions, 0, state); err != nil {
		t.Fatal(err)
	}
	global, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(global)
	return registry, stage, RuntimeOptions{State: state, Control: modules.NewControl(state, nil, service.FeatureCatalog), Service: service, Environment: stageOptions.Environment, ActionGuard: host.NewConfigurationQueue(service).AcquireAction,
		Trust: func(string) (Publisher, error) { return stageOptions.Publisher, nil },
		Pipe:  `\\.\pipe\WinOne-package-test-` + rand.Text(), DataDirectory: t.TempDir()}
}

func setRuntimeEnabled(t *testing.T, options RuntimeOptions, enabled bool) modules.ControlJob {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"id": "example.tool", "enabled": enabled, "revision": options.State.Snapshot().Revision})
	reply := options.Control.Handle(host.Request{Version: 1, ID: "enable", Method: "setEnabled", Params: params})
	if reply.Error != nil {
		t.Fatal(reply.Error)
	}
	var job modules.ControlJob
	if err := json.Unmarshal(reply.Result, &job); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for job.State == "running" {
		if time.Now().After(deadline) {
			t.Fatal("package lifecycle did not finish")
		}
		time.Sleep(time.Millisecond)
		params, _ = json.Marshal(map[string]string{"id": job.ID})
		reply = options.Control.Handle(host.Request{Version: 1, ID: "poll", Method: "jobs.get", Params: params})
		if reply.Error != nil {
			t.Fatal(reply.Error)
		}
		if err := json.Unmarshal(reply.Result, &job); err != nil {
			t.Fatal(err)
		}
	}
	return job
}

func TestRuntimeLoadPublishesDisabledPackageWithoutStartingWorker(t *testing.T) {
	registry, _, options := runtimeFixture(t)
	runtime, err := registry.LoadModule(context.Background(), "example.tool", options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	})
	if runtime.Running() {
		t.Fatal("loading a package started its worker")
	}
	listed := options.Control.Snapshot().Modules
	if len(listed) != 1 || listed[0].Enabled || listed[0].Runtime.State != "disabled" {
		t.Fatal("package not registered disabled", listed)
	}
	if reply := options.Service.Handle(host.Request{Version: 1, ID: "disabled", Method: "example.tool.echo"}); reply.Error == nil || reply.Error.Code != "module_disabled" {
		t.Fatal(reply)
	}
	if _, err = registry.LoadModule(context.Background(), "example.tool", options); err == nil {
		t.Fatal("duplicate load replaced a live runtime")
	}
}

func TestRecoveryOnlyPreparedRuntimeCannotPublishFeatureRoutes(t *testing.T) {
	registry, _, options := runtimeFixture(t)
	request := changeRequest(t, registry, options.State, nil)
	if _, err := registry.BeginChange(context.Background(), request, options.State); err != nil {
		t.Fatal(err)
	}
	options.Change = &ChangePermit{ID: request.ID, RegistryRevision: request.RegistryRevision}
	if _, err := registry.PrepareRuntime(context.Background(), "example.tool", options); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("ordinary prepared runtime accepted", err)
	}
	options.Change.RecoveryOnly = true
	runtime, err := registry.PrepareRuntime(context.Background(), "example.tool", options)
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Bind(options.Control, options.Service); err == nil {
		t.Fatal("recovery-only runtime published a feature route")
	}
	if runtime.Running() {
		t.Fatal("preparing recovery executed code")
	}
	if reply := options.Service.Handle(host.Request{Version: 1, ID: "denied", Method: "example.tool.echo"}); reply.Error == nil {
		t.Fatal("unbound recovery became callable")
	}
}

func TestRuntimeRechecksTrustAndFilesBeforeEnabling(t *testing.T) {
	for _, mode := range []string{"file", "trust", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			registry, stage, options := runtimeFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			originalTrust := options.Trust
			revoked := false
			options.Trust = func(namespace string) (Publisher, error) {
				if revoked {
					return Publisher{}, ErrUntrustedPublisher
				}
				return originalTrust(namespace)
			}
			runtime, err := registry.LoadModule(ctx, "example.tool", options)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := runtime.Close(); err != nil {
					t.Error(err)
				}
			})
			switch mode {
			case "file":
				if err = os.WriteFile(filepath.Join(stage.Directory(), "bin", "worker.exe"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "trust":
				revoked = true
			case "cancel":
				cancel()
			}
			job := setRuntimeEnabled(t, options, true)
			if job.State != "failed" || job.Error == "" || runtime.Running() {
				t.Fatal("invalid package enabled", job)
			}
			if reply := options.Service.Handle(host.Request{Version: 1, ID: "call", Method: "example.tool.echo"}); reply.Error == nil {
				t.Fatal("failed activation remained callable")
			}
		})
	}
}

func TestRuntimeRejectsInvalidBindingBeforePublishing(t *testing.T) {
	for _, mode := range []string{"trust", "missing-state", "different-state", "relative-data", "package-data", "pipe", "cancel", "guard"} {
		t.Run(mode, func(t *testing.T) {
			registry, stage, options := runtimeFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "trust":
				options.Trust = func(string) (Publisher, error) { return Publisher{}, ErrUntrustedPublisher }
			case "guard":
				options.ActionGuard = nil
			case "missing-state":
				options.State = nil
			case "different-state":
				options.State, _ = modulestate.Open(filepath.Join(t.TempDir(), "other.json"))
			case "relative-data":
				options.DataDirectory = "relative"
			case "package-data":
				options.DataDirectory = filepath.Join(stage.Directory(), "runtime-data")
			case "pipe":
				options.Pipe = "not-a-pipe"
			case "cancel":
				cancel()
			}
			if _, err := registry.LoadModule(ctx, "example.tool", options); err == nil {
				t.Fatal("invalid runtime options accepted")
			}
			if len(options.Control.Snapshot().Modules) != 0 {
				t.Fatal("rejected runtime was exposed")
			}
		})
	}
	registry, _, options := runtimeFixture(t)
	options.Trust = func(string) (Publisher, error) { return Publisher{}, ErrUntrustedPublisher }
	if _, err := registry.LoadModule(context.Background(), "example.tool", options); !errors.Is(err, ErrUntrustedPublisher) {
		t.Fatal("trust failure was hidden", err)
	}
}

func TestRuntimeRechecksPayloadBetweenEnableAndFirstCall(t *testing.T) {
	registry, stage, options := runtimeFixture(t)
	runtime, err := registry.LoadModule(context.Background(), "example.tool", options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	})
	if job := setRuntimeEnabled(t, options, true); job.State != "completed" {
		t.Fatal(job)
	}
	if runtime.Running() {
		t.Fatal("enable started the inert fixture")
	}
	if err = os.WriteFile(filepath.Join(stage.Directory(), "bin", "worker.exe"), []byte("changed after enable"), 0600); err != nil {
		t.Fatal(err)
	}
	reply := options.Service.Handle(host.Request{Version: 1, ID: "changed", Method: "example.tool.echo"})
	if reply.Error == nil || reply.Error.Code != "module_package" || runtime.Running() {
		t.Fatal("first call skipped file verification", reply)
	}
}

func TestRuntimeUnloadPreservesPackageAndRejectsStaleResume(t *testing.T) {
	registry, stage, options := runtimeFixture(t)
	runtime, err := registry.LoadModule(context.Background(), "example.tool", options)
	if err != nil {
		t.Fatal(err)
	}
	before := options.State.Snapshot()
	if err = runtime.Unload(); err != nil {
		t.Fatal(err)
	}
	if options.State.Snapshot().Revision != before.Revision {
		t.Fatal("unload changed enable preferences")
	}
	if _, err = os.Stat(filepath.Join(stage.Directory(), "bin", "worker.exe")); err != nil {
		t.Fatal("unload removed package files", err)
	}
	if len(options.Control.Snapshot().Modules) != 0 {
		t.Fatal("unloaded entry still visible")
	}
	if err = runtime.Resume(); err == nil {
		t.Fatal("retired runtime resumed")
	}
	reloaded, err := registry.LoadModule(context.Background(), "example.tool", options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reloaded.Close(); err != nil {
			t.Error(err)
		}
	})
	if err = runtime.Unload(); err != nil {
		t.Fatal("removal replay failed", err)
	}
	if len(options.Control.Snapshot().Modules) != 1 {
		t.Fatal("old runtime removed replacement")
	}
	if job := setRuntimeEnabled(t, options, true); job.State != "completed" {
		t.Fatal(job)
	}
	if err = reloaded.Unload(); err == nil {
		t.Fatal("enabled runtime was unloaded")
	}
}
