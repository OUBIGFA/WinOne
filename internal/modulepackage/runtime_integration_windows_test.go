//go:build windows

package modulepackage

import (
	"aead.dev/minisign"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/distribution"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/modulestate"
	"winone/internal/transport"
)

// The helper is the current test executable copied into a signed module. No
// installed program, user profile, shortcut or desktop window is touched.
func TestMain(m *testing.M) {
	if os.Getenv("WINONE_MODULE_RUNTIME_INTEGRATION") == "1" && len(os.Args) > 1 && os.Args[1] == "--pipe" {
		os.Exit(runRuntimeTestWorker(os.Args[1:]))
	}
	os.Exit(m.Run())
}

type runtimeTestWorker struct {
	directory    string
	cancel       context.CancelFunc
	dependencies string
}

func (w runtimeTestWorker) Handle(request host.Request) host.Response {
	reply := host.Response{ID: request.ID}
	switch request.Method {
	case "snapshot", "echo":
		output := map[string]any{"processId": os.Getpid(), "echo": json.RawMessage(request.Params)}
		if w.dependencies != "" {
			raw, err := os.ReadFile(w.dependencies)
			var view DependencyContext
			if err == nil {
				err = json.Unmarshal(raw, &view)
			}
			if err != nil {
				reply.Error = &host.RPCError{Code: "dependencies", Message: err.Error()}
				return reply
			}
			contents := map[string]string{}
			for _, location := range view.Packages {
				for _, export := range location.Exports {
					if export.Kind != "file" {
						continue
					}
					data, err := os.ReadFile(export.Path)
					if err != nil {
						reply.Error = &host.RPCError{Code: "dependencies", Message: err.Error()}
						return reply
					}
					contents[location.ID+"/"+export.Name] = string(data)
				}
			}
			output["dependencies"] = view
			output["contents"] = contents
		}
		reply.Result, _ = json.Marshal(output)
	case "settings.reload":
		// Deliberately incomplete protocol reply for the acknowledgement test.
		reply.Result = json.RawMessage(`{}`)
	case "shutdown":
		if _, err := os.Stat(filepath.Join(w.directory, "refuse-shutdown")); err == nil {
			reply.Error = &host.RPCError{Code: "busy", Message: "fixture restoration is pending"}
			return reply
		}
		if err := os.WriteFile(filepath.Join(w.directory, "restored.txt"), []byte("restored"), 0600); err != nil {
			reply.Error = &host.RPCError{Code: "io", Message: err.Error()}
			return reply
		}
		reply.Result = json.RawMessage(`{}`)
		w.cancel()
	default:
		reply.Error = &host.RPCError{Code: "unknown", Message: "fixture command unavailable"}
	}
	return reply
}
func runRuntimeTestWorker(args []string) int {
	flags := flag.NewFlagSet("runtime-fixture", flag.ContinueOnError)
	pipe := flags.String("pipe", "", "")
	directory := flags.String("data-dir", "", "")
	parent := flags.Uint("parent", 0, "")
	dependencies := flags.String("dependency-context", "", "")
	if err := flags.Parse(args); err != nil || *parent == 0 || !filepath.IsAbs(*directory) || !strings.HasPrefix(*pipe, `\\.\pipe\WinOne-package-test-`) {
		return 2
	}
	if _, err := os.Stat(filepath.Join(*directory, "exit-before-probe")); err == nil {
		return 7
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(*parent))
	if err != nil {
		return 3
	}
	defer windows.CloseHandle(owner)
	go func() { _, _ = windows.WaitForSingleObject(owner, windows.INFINITE); cancel() }()
	listener, err := transport.Listen(*pipe)
	if err != nil {
		return 4
	}
	if err = transport.Serve(ctx, listener, runtimeTestWorker{directory: *directory, cancel: cancel, dependencies: *dependencies}); err != nil {
		return 5
	}
	return 0
}

func signedRuntimeWorker(t *testing.T, options RuntimeOptions, registry *Registry) Publisher {
	t.Helper()
	stage, approval := signedRuntimeStage(t, registry, "1.2.0", false)
	if _, err := registry.PrepareModule(context.Background(), filepath.Base(stage.Directory()), approval, 0, options.State); err != nil {
		t.Fatal(err)
	}
	return approval.Publisher
}

func signedRuntimeStage(t *testing.T, registry *Registry, version string, broken bool) (*StagedPackage, StageOptions) {
	manifest, publisher, key := signedTestManifest(t)
	manifest.Metadata.Version = version
	return signedRuntimeStageDefined(t, registry, manifest, publisher, key, broken)
}

func signedRuntimeStageDefined(t *testing.T, registry *Registry, manifest Manifest, publisher Publisher, key minisign.PrivateKey, broken bool) (*StagedPackage, StageOptions) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	worker, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if broken {
		worker = []byte("signed but not an executable")
	}
	payload := map[string][]byte{"bin/worker.exe": worker, "LICENSE.txt": []byte("test license")}
	if manifest.Metadata.Settings != nil {
		payload[manifest.Metadata.Settings.Schema] = []byte(`{"type":"object"}`)
		payload[manifest.Metadata.Settings.Defaults] = []byte(`{}`)
	}
	manifest.Files = nil
	for name, body := range payload {
		sum := sha256.Sum256(body)
		manifest.Files = append(manifest.Files, distribution.File{Path: name, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])})
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	payload["module.json"] = raw
	payload["module.json.minisig"] = minisign.Sign(key, raw)
	for name, body := range payload {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "runtime.winone-module")
	if err = os.WriteFile(archive, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyArchive(context.Background(), archive, publisher)
	if err != nil {
		t.Fatal(err)
	}
	stageOptions := StageOptions{Directory: registry.directory, Publisher: publisher, Environment: testEnvironment(), ExpectedManifestSHA256: verified.Manifest().Digest()}
	stage, err := StageArchive(context.Background(), archive, stageOptions)
	if err != nil {
		t.Fatal(err)
	}
	return stage, stageOptions
}

func TestRuntimeSettingsRequireExplicitZeroRevisionAcknowledgement(t *testing.T) {
	if os.Getenv("WINONE_MODULE_RUNTIME_INTEGRATION") != "1" {
		t.Skip("explicit isolated worker integration")
	}
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
	module, err := registry.LoadModule(context.Background(), "example.tool", options)
	if err != nil {
		t.Fatal(err)
	}
	defer module.Close()
	if job := setRuntimeEnabled(t, options, true); job.State != "completed" {
		t.Fatal(job)
	}
	reply := options.Service.Handle(host.Request{Version: 1, ID: "run", Method: "example.tool.echo", Params: json.RawMessage(`"start"`)})
	if reply.Error != nil {
		t.Fatal(reply.Error)
	}
	if err = module.ProjectSettings(json.RawMessage(`{"size":3}`), 0); err == nil {
		t.Fatal("missing revision falsely acknowledged rollback to revision zero")
	}
}

func TestRuntimeSignedWorkerLifecycle(t *testing.T) {
	if os.Getenv("WINONE_MODULE_RUNTIME_INTEGRATION") != "1" {
		t.Skip("explicit isolated-process integration required")
	}
	_, _, options := runtimeFixture(t)
	// A new registry and intent store are required: the inert unit fixture is
	// never upgraded or silently replaced by this integration package.
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
	revoked := false
	options.Trust = func(string) (Publisher, error) {
		if revoked {
			return Publisher{}, ErrUntrustedPublisher
		}
		return publisher, nil
	}
	queue := host.NewConfigurationQueue(options.Service)
	options.ActionGuard = queue.AcquireAction
	runtime, err := registry.LoadModule(context.Background(), "example.tool", options)
	if err != nil {
		t.Fatal(err)
	}
	var process windows.Handle
	var handles []windows.Handle
	runtimes := []*RuntimeModule{runtime}
	t.Cleanup(func() {
		failed := false
		for i := len(runtimes) - 1; i >= 0; i-- {
			if err := runtimes[i].Close(); err != nil {
				t.Error(err)
				failed = true
			}
		}
		if failed {
			for _, handle := range handles {
				if status, _ := windows.WaitForSingleObject(handle, 0); status == windows.WAIT_OBJECT_0 {
					continue
				}
				if err := windows.TerminateProcess(handle, 1); err != nil {
					t.Error("owned fixture termination", err)
				}
				if status, err := windows.WaitForSingleObject(handle, 5000); err != nil || status != windows.WAIT_OBJECT_0 {
					t.Error("owned fixture cleanup", status, err)
				}
			}
		}
		for _, handle := range handles {
			windows.CloseHandle(handle)
		}
	})
	if runtime.Running() {
		t.Fatal("load started a process")
	}
	if job := setRuntimeEnabled(t, options, true); job.State != "completed" {
		t.Fatal(job)
	}
	if runtime.Running() {
		t.Fatal("enable eagerly started an on-demand worker")
	}
	release, ok := queue.AcquireMaintenance()
	if !ok {
		t.Fatal("maintenance was unexpectedly busy")
	}
	blocked := options.Service.Handle(host.Request{Version: 1, ID: "maintenance", Method: "example.tool.snapshot"})
	release()
	if blocked.Error == nil || blocked.Error.Code != "configuration_busy" || runtime.Running() {
		t.Fatal("snapshot launched a worker during maintenance", blocked)
	}
	reply := options.Service.Handle(host.Request{Version: 1, ID: "echo", Method: "example.tool.echo", Params: json.RawMessage(`"hello"`)})
	var output struct {
		ProcessID uint32 `json:"processId"`
		Echo      string `json:"echo"`
	}
	if reply.Error != nil || json.Unmarshal(reply.Result, &output) != nil || output.ProcessID == 0 || output.Echo != "hello" {
		t.Fatal("real worker call failed", reply)
	}
	process, err = windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, output.ProcessID)
	if err != nil {
		t.Fatal(err)
	}
	handles = append(handles, process)
	if !runtime.Running() {
		t.Fatal("worker was not tracked")
	}
	license := filepath.Join(runtime.directory, "LICENSE.txt")
	assertProtected := func() {
		t.Helper()
		if writer, err := os.OpenFile(license, os.O_WRONLY, 0); err == nil {
			writer.Close()
			t.Fatal("running package payload remained writable")
		}
	}
	assertProtected()
	if err := os.Rename(registry.directory, registry.directory+"-moved"); err == nil {
		t.Fatal("running package store could be replaced")
	}
	revoked = true
	if response := options.Service.Handle(host.Request{Version: 1, ID: "revoked", Method: "example.tool.echo"}); response.Error == nil {
		t.Fatal("revoked publisher was still callable")
	}
	// Revocation blocks new work, but cannot block resource restoration.
	refusal := filepath.Join(options.DataDirectory, "refuse-shutdown")
	if err = os.WriteFile(refusal, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	if job := setRuntimeEnabled(t, options, false); job.State != "failed" || !runtime.Running() {
		t.Fatal("failed restoration reported success", job)
	}
	if !options.State.Snapshot().Modules["example.tool"].PendingStop {
		t.Fatal("failed stop lost recovery state")
	}
	assertProtected()
	if err = os.Remove(refusal); err != nil {
		t.Fatal(err)
	}
	if job := setRuntimeEnabled(t, options, false); job.State != "completed" {
		t.Fatal(job)
	}
	if runtime.Running() {
		t.Fatal("worker survived disable")
	}
	if status, err := windows.WaitForSingleObject(process, 0); err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("OS still reports live worker", status, err)
	}
	if data, err := os.ReadFile(filepath.Join(options.DataDirectory, "restored.txt")); err != nil || string(data) != "restored" {
		t.Fatal("restoration did not complete", err)
	}
	if record := options.State.Snapshot().Modules["example.tool"]; record.Enabled || record.PendingStop {
		t.Fatal(record)
	}
	writer, err := os.OpenFile(license, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal("payload protection survived successful disable", err)
	}
	writer.Close()
	revoked = false
	if job := setRuntimeEnabled(t, options, true); job.State != "completed" {
		t.Fatal(job)
	}
	reply = options.Service.Handle(host.Request{Version: 1, ID: "restart", Method: "example.tool.echo", Params: json.RawMessage(`"again"`)})
	var restarted struct {
		ProcessID uint32 `json:"processId"`
		Echo      string `json:"echo"`
	}
	if reply.Error != nil || json.Unmarshal(reply.Result, &restarted) != nil || restarted.ProcessID == 0 || restarted.ProcessID == output.ProcessID || restarted.Echo != "again" {
		t.Fatal("on-demand restart failed", reply)
	}
	second, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, restarted.ProcessID)
	if err != nil {
		t.Fatal(err)
	}
	handles = append(handles, second)
	if err = runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if !options.State.Snapshot().Modules["example.tool"].Enabled {
		t.Fatal("Host shutdown changed enable intent")
	}
	if status, err := windows.WaitForSingleObject(second, 0); err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("restarted worker survived Host close", status, err)
	}
	if reply := options.Service.Handle(host.Request{Version: 1, ID: "closed", Method: "example.tool.echo"}); reply.Error == nil {
		t.Fatal("closed Host runtime accepted work")
	}
	if err = runtime.Resume(); err != nil {
		t.Fatal("aborted Host shutdown could not resume", err)
	}
	if runtime.Running() {
		t.Fatal("resume started an idle worker")
	}
	if err = runtime.Unload(); err == nil {
		t.Fatal("enabled runtime was removed")
	}
	if job := setRuntimeEnabled(t, options, false); job.State != "completed" {
		t.Fatal(job)
	}
	if err = runtime.Unload(); err != nil {
		t.Fatal(err)
	}
	if len(options.Control.Snapshot().Modules) != 0 {
		t.Fatal("unloaded package remains in controls")
	}
	reloaded, err := registry.LoadModule(context.Background(), "example.tool", options)
	if err != nil {
		t.Fatal(err)
	}
	runtimes = append(runtimes, reloaded)
	if err = runtime.Resume(); err == nil {
		t.Fatal("retired runtime resumed")
	}
	if err = runtime.Unload(); err != nil {
		t.Fatal("old removal replay failed", err)
	}
	if job := setRuntimeEnabled(t, options, true); job.State != "completed" {
		t.Fatal(job)
	}
	reply = options.Service.Handle(host.Request{Version: 1, ID: "reload", Method: "example.tool.echo", Params: json.RawMessage(`"reloaded"`)})
	var thirdOutput struct {
		ProcessID uint32 `json:"processId"`
		Echo      string `json:"echo"`
	}
	if reply.Error != nil || json.Unmarshal(reply.Result, &thirdOutput) != nil || thirdOutput.ProcessID == 0 || thirdOutput.ProcessID == output.ProcessID || thirdOutput.ProcessID == restarted.ProcessID || thirdOutput.Echo != "reloaded" {
		t.Fatal("reloaded package was not callable", reply)
	}
	third, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, thirdOutput.ProcessID)
	if err != nil {
		t.Fatal(err)
	}
	handles = append(handles, third)
	if err = runtime.Unload(); err != nil {
		t.Fatal(err)
	}
	if !reloaded.Running() {
		t.Fatal("old unload stopped the new worker")
	}
	if err = reloaded.Unload(); err == nil {
		t.Fatal("running worker was removed")
	}
	if job := setRuntimeEnabled(t, options, false); job.State != "completed" {
		t.Fatal(job)
	}
	if err = reloaded.Unload(); err != nil {
		t.Fatal(err)
	}
	if status, err := windows.WaitForSingleObject(third, 0); err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("reloaded worker survived disable", status, err)
	}
	fmt.Printf("signed worker PIDs %d/%d/%d: lifecycle, file protection, recovery, unload/reload, stale-reference rejection and OS exit verified\n", output.ProcessID, restarted.ProcessID, thirdOutput.ProcessID)
}
