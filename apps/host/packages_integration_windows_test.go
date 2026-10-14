//go:build windows

package main

import (
	"aead.dev/minisign"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"
	"winone/internal/catalog"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modulestate"
	appsettings "winone/internal/settings"
	"winone/internal/transport"
)

func TestMain(m *testing.M) {
	if os.Getenv("WINONE_PACKAGE_HOST_TEST") == "1" && len(os.Args) > 1 && os.Args[1] == "--pipe" {
		os.Exit(runHostPackageWorker(os.Args[1:]))
	}
	os.Exit(m.Run())
}

type hostPackageWorker struct {
	mu           sync.Mutex
	projection   modulepackage.SettingsProjection
	directory    string
	cancel       context.CancelFunc
	dependencies string
}

func (w *hostPackageWorker) reload() error {
	raw, err := os.ReadFile(filepath.Join(w.directory, "settings.json"))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, &w.projection)
}
func (w *hostPackageWorker) Handle(r host.Request) host.Response {
	w.mu.Lock()
	defer w.mu.Unlock()
	reply := host.Response{ID: r.ID}
	var result any
	switch r.Method {
	case "snapshot", "echo":
		output := map[string]any{"processId": os.Getpid(), "settings": w.projection.Values, "revision": w.projection.Revision}
		if w.dependencies != "" {
			raw, err := os.ReadFile(w.dependencies)
			var view modulepackage.DependencyContext
			if err == nil {
				err = json.Unmarshal(raw, &view)
			}
			if err != nil {
				return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture_dependency", Message: err.Error()}}
			}
			exports := map[string]string{}
			for _, location := range view.Packages {
				for _, export := range location.Exports {
					if export.Kind == "file" {
						data, err := os.ReadFile(export.Path)
						if err != nil {
							return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture_dependency", Message: err.Error()}}
						}
						exports[location.ID+"/"+export.Name] = string(data)
					}
				}
			}
			output["dependencyContext"] = view
			output["exports"] = exports
		}
		result = output
	case "settings.reload":
		if err := w.reload(); err != nil {
			return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture", Message: err.Error()}}
		}
		result = map[string]any{"revision": w.projection.Revision}
	case "shutdown":
		if blocked, err := os.ReadFile(filepath.Join(w.directory, "refuse-version-shutdown")); err == nil {
			executable, err := os.Executable()
			if err != nil {
				return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture", Message: err.Error()}}
			}
			raw, err := os.ReadFile(filepath.Join(filepath.Dir(executable), "module.json"))
			if err != nil {
				return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture", Message: err.Error()}}
			}
			var manifest struct{ Metadata struct{ Version string } }
			if err = json.Unmarshal(raw, &manifest); err != nil {
				return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture", Message: err.Error()}}
			}
			if string(blocked) == manifest.Metadata.Version {
				return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture_busy", Message: "selected candidate restoration is pending"}}
			}
		}
		if _, err := os.Stat(filepath.Join(w.directory, "refuse-candidate-shutdown")); err == nil {
			executable, err := os.Executable()
			if err != nil {
				return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture", Message: err.Error()}}
			}
			raw, err := os.ReadFile(filepath.Join(filepath.Dir(executable), "module.json"))
			if err != nil {
				return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture", Message: err.Error()}}
			}
			var manifest struct{ Metadata struct{ Version string } }
			if err = json.Unmarshal(raw, &manifest); err != nil {
				return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture", Message: err.Error()}}
			}
			if manifest.Metadata.Version == "1.1.0" {
				return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture_busy", Message: "candidate restoration is pending"}}
			}
		}
		if _, err := os.Stat(filepath.Join(w.directory, "refuse-shutdown")); err == nil {
			return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture_busy", Message: "fixture resource restoration is pending"}}
		}
		file, err := os.OpenFile(filepath.Join(w.directory, "restored.txt"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture", Message: err.Error()}}
		}
		_, err = file.WriteString("restored\n")
		err2 := file.Close()
		if err != nil || err2 != nil {
			return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture", Message: "restore write failed"}}
		}
		result = map[string]any{}
		w.cancel()
	default:
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "fixture", Message: "unknown fixture method"}}
	}
	reply.Result, _ = json.Marshal(result)
	return reply
}
func runHostPackageWorker(args []string) int {
	flags := flag.NewFlagSet("package-fixture", flag.ContinueOnError)
	pipe := flags.String("pipe", "", "")
	data := flags.String("data-dir", "", "")
	parent := flags.Uint("parent", 0, "")
	dependencies := flags.String("dependency-context", "", "")
	if flags.Parse(args) != nil || *parent == 0 || !filepath.IsAbs(*data) || !strings.HasPrefix(*pipe, `\\.\pipe\WinOne-package-`) {
		return 2
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(*parent))
	if err != nil {
		return 3
	}
	defer windows.CloseHandle(owner)
	go func() { _, _ = windows.WaitForSingleObject(owner, windows.INFINITE); cancel() }()
	worker := &hostPackageWorker{directory: *data, cancel: cancel, dependencies: *dependencies}
	if worker.reload() != nil {
		return 4
	}
	listener, err := transport.Listen(*pipe)
	if err != nil {
		return 5
	}
	defer listener.Close()
	if transport.Serve(ctx, listener, worker) != nil {
		return 6
	}
	return 0
}

func TestProductHostDiscoversSignedPackageAcrossRestart(t *testing.T) {
	if os.Getenv("WINONE_PACKAGE_HOST_TEST") != "1" {
		t.Skip("explicit signed-package Host process test")
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
	for _, manifest := range catalog.Builtins() {
		states.Modules[manifest.ID] = modulestate.Record{}
	}
	raw, _ := json.Marshal(states)
	if err := os.WriteFile(filepath.Join(profile, "module-state.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "settings.json"), []byte(`{"schemaVersion":1,"captureShortcut":"","pinShortcut":"","ocrShortcut":"","translateShortcut":"","selectionShortcut":""}`), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := modulestate.Open(filepath.Join(profile, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	workerBytes, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	installFlow := os.Getenv("WINONE_PACKAGE_INSTALL_TEST") == "1"
	updateMode := os.Getenv("WINONE_PACKAGE_UPDATE_TEST")
	migrationFault := strings.HasPrefix(os.Getenv("WINONE_PACKAGE_FAULT_MODE"), "migration-")
	if migrationFault && !installFlow {
		t.Fatal("migration crash fixture requires installation flow")
	}
	if updateMode != "" && (!installFlow || updateMode != "valid" && updateMode != "invalid-image" && updateMode != "locked-retirement" && updateMode != "disabled" && updateMode != "old-stop-refusal" && updateMode != "candidate-stop-refusal" && updateMode != "migrate" && updateMode != "migrate-invalid" && updateMode != "migrate-reinstall" && updateMode != "migrate-write-failure") {
		t.Fatal("invalid update integration mode")
	}
	var updateArchive string
	var brokenUpdateArchive string
	installFailure := os.Getenv("WINONE_PACKAGE_INSTALL_FAILURE_TEST")
	if installFailure != "" && (!installFlow || installFailure != "configuration.json" && installFailure != "module-state.json") {
		t.Fatal("choose configuration.json or module-state.json with the installation flow")
	}
	var archivePath string
	var publisher modulepackage.Publisher
	if installFlow {
		if updateMode != "" || migrationFault {
			public, private, keyErr := minisign.GenerateKey(rand.Reader)
			if keyErr != nil {
				t.Fatal(keyErr)
			}
			archivePath, publisher = hostPackageArchiveFixtureSigned(t, workerBytes, public, private, "1.0.0")
			candidate := workerBytes
			if updateMode == "invalid-image" || updateMode == "migrate-invalid" {
				candidate = []byte("signed but invalid Windows executable")
			}
			if strings.HasPrefix(updateMode, "migrate") || migrationFault {
				options := hostMigrationArchiveOptions("1.1.0")
				updateArchive, _ = hostPackageArchiveFixtureDefined(t, candidate, public, private, options)
			} else {
				updateArchive, _ = hostPackageArchiveFixtureSigned(t, candidate, public, private, "1.1.0")
			}
			if strings.HasPrefix(updateMode, "migrate") || migrationFault {
				brokenUpdateArchive, _ = hostPackageArchiveFixtureDefined(t, []byte("signed invalid newer executable"), public, private, hostMigrationArchiveOptions("1.2.0"))
			} else {
				brokenUpdateArchive, _ = hostPackageArchiveFixtureSigned(t, []byte("signed invalid newer executable"), public, private, "1.2.0")
			}
		} else {
			archivePath, publisher = hostPackageArchiveFixture(t, workerBytes)
		}
		raw, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatal(err)
		}
		archivePath = filepath.Join(root, "fixture.winone-module")
		if err = os.WriteFile(archivePath, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if updateArchive != "" {
			raw, err := os.ReadFile(updateArchive)
			if err != nil {
				t.Fatal(err)
			}
			updateArchive = filepath.Join(root, "update.winone-module")
			if err = os.WriteFile(updateArchive, raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if brokenUpdateArchive != "" {
			raw, err := os.ReadFile(brokenUpdateArchive)
			if err != nil {
				t.Fatal(err)
			}
			brokenUpdateArchive = filepath.Join(root, "broken-update.winone-module")
			if err = os.WriteFile(brokenUpdateArchive, raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
	} else {
		publisher = installHostPackageFixture(t, profile, state, workerBytes)
	}
	trustFile := filepath.Join(root, "publishers.json")
	raw, _ = json.Marshal(map[string]any{"publishers": []modulepackage.Publisher{publisher}})
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
	for run := 0; run < 2; run++ {
		faultMode := os.Getenv("WINONE_PACKAGE_FAULT_MODE")
		if faultMode == "" && os.Getenv("WINONE_PACKAGE_FAULT_TEST") == "1" {
			faultMode = "payload"
		}
		faultRecovery := run == 1 && faultMode != ""
		var damagedPath string
		var migrationBackupPath string
		var migrationBackup []byte
		if faultRecovery {
			registry, err := modulepackage.OpenRegistry(filepath.Join(profile, "packages"))
			if err != nil {
				t.Fatal(err)
			}
			inventory, err := registry.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(faultMode, "migration-") {
				migrationBackupPath, migrationBackup = migratedConfigurationCrashFixture(t, profile, updateArchive, publisher, registry)
				if faultMode == "migration-corrupt-backup" {
					if err = os.WriteFile(migrationBackupPath, []byte("{}"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			} else if faultMode == "payload" || faultMode == "probe-refusal" {
				damagedPath = filepath.Join(profile, "packages", inventory.Records["example.tool"].StageID, "worker.exe")
				if err = os.WriteFile(damagedPath, []byte("damaged package fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if faultMode == "configuration" {
				file := filepath.Join(profile, "configuration.json")
				raw, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				var disk struct {
					Committed configuration.Document  `json:"committed"`
					Pending   *configuration.Document `json:"pending,omitempty"`
				}
				if err = json.Unmarshal(raw, &disk); err != nil {
					t.Fatal(err)
				}
				copyBytes, _ := json.Marshal(disk.Committed)
				var pending configuration.Document
				_ = json.Unmarshal(copyBytes, &pending)
				pending.Revision++
				pending.Values["example.tool"] = json.RawMessage(`{"size":99}`)
				disk.Pending = &pending
				raw, _ = json.Marshal(disk)
				if err = os.WriteFile(file, raw, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				// The prior Host shutdown and marker already proved resource
				// restoration. Interrupt the durable package change at each phase.
				state, err = modulestate.Open(filepath.Join(profile, "module-state.json"))
				if err != nil {
					t.Fatal(err)
				}
				var replacement *modulepackage.ReplacementStage
				if faultMode == "upgrade" {
					other := t.TempDir()
					otherState, err := modulestate.Open(filepath.Join(other, "module-state.json"))
					if err != nil {
						t.Fatal(err)
					}
					// This candidate is signed, but its publisher is no longer in
					// the current Host's trust set and its bytes are not executable.
					candidatePublisher := installHostPackageFixture(t, other, otherState, []byte("signed invalid candidate"), "1.1.0")
					candidateRegistry, err := modulepackage.OpenRegistry(filepath.Join(other, "packages"))
					if err != nil {
						t.Fatal(err)
					}
					candidateSnapshot, err := candidateRegistry.Snapshot()
					if err != nil {
						t.Fatal(err)
					}
					candidate := candidateSnapshot.Records["example.tool"]
					if err = os.CopyFS(filepath.Join(profile, "packages", candidate.StageID), os.DirFS(filepath.Join(other, "packages", candidate.StageID))); err != nil {
						t.Fatal(err)
					}
					replacement = &modulepackage.ReplacementStage{ID: candidate.StageID, Options: modulepackage.StageOptions{Directory: filepath.Join(profile, "packages"), Publisher: candidatePublisher, Environment: packageTestEnvironment, ExpectedManifestSHA256: candidate.ManifestSHA256}}
				}
				if _, err = registry.BeginChange(context.Background(), modulepackage.ChangeRequest{ID: "interrupted-remove", RegistryRevision: inventory.Revision, StateRevision: state.Snapshot().Revision, Current: inventory.Records["example.tool"], Replacement: replacement}, state); err != nil {
					t.Fatal(err)
				}
				if faultMode == "prepared-stop" || faultMode == "applied" || faultMode == "upgrade" {
					if _, err = state.SetIntent(state.Snapshot().Revision, "example.tool", false); err != nil {
						t.Fatal(err)
					}
				}
				if faultMode == "applied" || faultMode == "upgrade" {
					if _, err = state.FinishStop("example.tool"); err != nil {
						t.Fatal(err)
					}
					var approval *modulepackage.StageOptions
					if replacement != nil {
						approval = &replacement.Options
					}
					if _, err = registry.ApplyChange(context.Background(), "interrupted-remove", state, approval); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		pipe := fmt.Sprintf(`\\.\pipe\WinOne-package-host-%d-%d`, os.Getpid(), run)
		cmd := exec.Command(filepath.Join(bundle, "WinOne.Host.exe"), "--pipe", pipe, "--data-dir", profile, "--standard-session")
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
		log, err := os.Create(filepath.Join(root, fmt.Sprintf("host-%d.log", run)))
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stdout = log
		cmd.Stderr = log
		if err = cmd.Start(); err != nil {
			log.Close()
			t.Fatal(err)
		}
		done := make(chan error, 1)
		finished := &atomic.Bool{}
		go func() { err := cmd.Wait(); log.Close(); finished.Store(true); done <- err }()
		t.Cleanup(func() {
			if !finished.Load() {
				_ = cmd.Process.Kill()
				<-done
			}
		})
		call := func(method string, value any) host.Response {
			t.Helper()
			data, _ := json.Marshal(value)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			reply, err := transport.Call(ctx, pipe, host.Request{Version: 1, ID: "fixture", Method: method, Params: data})
			if err != nil {
				t.Fatal(err)
			}
			var job struct {
				ID string `json:"configurationJob"`
			}
			_ = json.Unmarshal(reply.Result, &job)
			for job.ID != "" {
				response, err := transport.Call(ctx, pipe, host.Request{Version: 1, ID: "poll", Method: "configuration.jobs.get", Params: json.RawMessage(fmt.Sprintf(`{"id":%q}`, job.ID))})
				if err != nil {
					t.Fatal(err)
				}
				var status host.ConfigurationJob
				if json.Unmarshal(response.Result, &status) != nil {
					t.Fatal("invalid job")
				}
				if status.State != "running" {
					if status.Response == nil {
						t.Fatal("missing job result")
					}
					reply = *status.Response
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			return reply
		}
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
			time.Sleep(30 * time.Millisecond)
		}
		installFailureInjected := false
		install := func() {
			t.Helper()
			reply := call("modules.packagePreview.start", map[string]string{"path": archivePath})
			if reply.Error != nil {
				t.Fatal(reply.Error)
			}
			var preview packagePreviewView
			_ = json.Unmarshal(reply.Result, &preview)
			deadline := time.Now().Add(10 * time.Second)
			for preview.State == "checking" {
				reply = call("modules.packagePreview.get", map[string]string{"id": preview.ID})
				if reply.Error != nil {
					t.Fatal(reply.Error)
				}
				_ = json.Unmarshal(reply.Result, &preview)
				if time.Now().After(deadline) {
					t.Fatal("archive preview timed out")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !preview.CanInstall {
				t.Fatal("preview blocked install", preview.Error)
			}
			if run == 1 && !preview.RestoresSettings {
				t.Fatal("reinstall did not recognize retained settings")
			}
			if run == 0 && installFailure != "" && !installFailureInjected {
				// Hold a real file without write/delete sharing. This rejects the
				// atomic commit after package registration, without a production hook.
				path := filepath.Join(profile, installFailure)
				original, readErr := os.ReadFile(path)
				if readErr != nil {
					t.Fatal(readErr)
				}
				name, pathErr := windows.UTF16PtrFromString(path)
				if pathErr != nil {
					t.Fatal(pathErr)
				}
				lock, lockErr := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
				if lockErr != nil {
					t.Fatal(lockErr)
				}
				closed := false
				t.Cleanup(func() {
					if !closed {
						windows.CloseHandle(lock)
					}
				})
				failed := call("modules.installPackage", map[string]string{"id": preview.ID})
				if closeErr := windows.CloseHandle(lock); closeErr != nil {
					t.Fatal(closeErr)
				}
				closed = true
				if failed.Error == nil {
					t.Fatal("locked commit reported successful installation")
				}
				after, readErr := os.ReadFile(path)
				if readErr != nil || string(after) != string(original) {
					t.Fatal("failed commit changed protected file", readErr)
				}
				registry, openErr := modulepackage.OpenRegistry(filepath.Join(profile, "packages"))
				if openErr != nil {
					t.Fatal(openErr)
				}
				inventory, snapshotErr := registry.Snapshot()
				if snapshotErr != nil || inventory.Records["example.tool"].ManifestSHA256 != preview.ManifestSHA256 {
					t.Fatal("failure did not reach the post-registration commit", snapshotErr)
				}
				var listing struct {
					Revision uint64 `json:"revision"`
					Modules  []struct {
						Manifest catalog.Manifest `json:"manifest"`
						Runtime  struct {
							State string `json:"state"`
						} `json:"runtime"`
					} `json:"modules"`
				}
				listed := call("modules.snapshot", map[string]any{})
				if listed.Error != nil || json.Unmarshal(listed.Result, &listing) != nil {
					t.Fatal("failed installation status missing")
				}
				unavailable := false
				for _, item := range listing.Modules {
					if item.Manifest.ID == "example.tool" && item.Runtime.State == "unavailable" {
						unavailable = true
					}
				}
				if !unavailable {
					t.Fatal("failed installation did not expose recovery")
				}
				if denied := call("example.tool.echo", map[string]any{}); denied.Error == nil {
					t.Fatal("incomplete installation exposed worker commands")
				}
				if recovered := call("modules.recoverPackage", map[string]any{"id": "example.tool", "revision": listing.Revision}); recovered.Error != nil {
					t.Fatal("installation could not recover after releasing the file", recovered.Error)
				}
				installFailureInjected = true
				return
			}
			if reply = call("modules.installPackage", map[string]string{"id": preview.ID}); reply.Error != nil {
				t.Fatal("installation through Host failed", reply.Error)
			}
		}
		if installFlow && run == 0 {
			install()
		}
		status := call("packages.snapshot", map[string]any{})
		if status.Error != nil {
			t.Fatal(status.Error)
		}
		var installed struct {
			Packages []struct {
				Running *bool `json:"running"`
			} `json:"packages"`
		}
		if json.Unmarshal(status.Result, &installed) != nil || len(installed.Packages) != 1 || installed.Packages[0].Running == nil || *installed.Packages[0].Running {
			t.Fatal("startup ran an idle package")
		}
		if faultRecovery {
			if denied := call("example.tool.echo", map[string]any{}); denied.Error == nil {
				t.Fatal("corrupt package remained executable")
			}
			snapshot := call("snapshot", map[string]any{})
			var global struct {
				Settings appsettings.Settings `json:"settings"`
			}
			if json.Unmarshal(snapshot.Result, &global) != nil {
				t.Fatal("Host snapshot unreadable")
			}
			global.Settings.Theme = "dark"
			saved := call("settings.save", global.Settings)
			if faultMode == "configuration" {
				if saved.Error == nil {
					t.Fatal("pending configuration recovery allowed writes")
				}
			} else if saved.Error != nil {
				t.Fatal("bad package blocked ordinary settings", saved.Error)
			}
			if logs := call("diagnostics.read", map[string]string{"module": "host"}); logs.Error != nil {
				t.Fatal("bad package blocked global logs", logs.Error)
			}
			if damagedPath != "" {
				if err = os.WriteFile(damagedPath, workerBytes, 0600); err != nil {
					t.Fatal(err)
				}
			}
			modules := call("modules.snapshot", map[string]any{})
			var listing struct {
				Revision uint64 `json:"revision"`
				Modules  []struct {
					Blocked    bool   `json:"blocked"`
					RecoveryID string `json:"recoveryId"`
					Manifest   struct {
						ID string `json:"id"`
					} `json:"manifest"`
				} `json:"modules"`
			}
			if json.Unmarshal(modules.Result, &listing) != nil {
				t.Fatal("module list unreadable")
			}
			found := false
			recoveryID := ""
			for _, entry := range listing.Modules {
				if entry.Manifest.ID == "example.tool" {
					found = entry.Blocked
					recoveryID = entry.RecoveryID
				}
			}
			if !found {
				t.Fatal("bad package was not visible as blocked")
			}
			if faultMode == "probe-refusal" {
				marker := filepath.Join(profile, "modules", "example.tool", "refuse-shutdown")
				if err = os.WriteFile(marker, []byte("pending"), 0600); err != nil {
					t.Fatal(err)
				}
				if failed := call("modules.recoverPackage", map[string]any{"id": "example.tool", "revision": listing.Revision, "recoveryId": recoveryID}); failed.Error == nil {
					t.Fatal("failed resource recovery reported success")
				}
				savedState, err := modulestate.Open(filepath.Join(profile, "module-state.json"))
				if err != nil || !savedState.Snapshot().Modules["example.tool"].PendingStop {
					t.Fatal("recovery failure lost its durable stop marker", err)
				}
				if denied := call("example.tool.echo", map[string]any{}); denied.Error == nil {
					t.Fatal("failed probe published a feature route")
				}
				if err = os.Remove(marker); err != nil {
					t.Fatal(err)
				}
				modules = call("modules.snapshot", map[string]any{})
				_ = json.Unmarshal(modules.Result, &listing)
			}
			if faultMode == "migration-corrupt-backup" {
				if failed := call("modules.recoverPackage", map[string]any{"id": "example.tool", "revision": listing.Revision, "recoveryId": recoveryID}); failed.Error == nil {
					t.Fatal("changed migration backup was accepted")
				}
				if err = os.WriteFile(migrationBackupPath, migrationBackup, 0600); err != nil {
					t.Fatal(err)
				}
				modules = call("modules.snapshot", map[string]any{})
				if err = json.Unmarshal(modules.Result, &listing); err != nil {
					t.Fatal(err)
				}
			}
			if recovered := call("modules.recoverPackage", map[string]any{"id": "example.tool", "revision": listing.Revision, "recoveryId": recoveryID}); recovered.Error != nil {
				t.Fatal("repaired package could not reload", recovered.Error)
			}
			if strings.HasPrefix(faultMode, "migration-") {
				snapshot = call("snapshot", map[string]any{})
				if err = json.Unmarshal(snapshot.Result, &global); err != nil || global.Settings.Theme != "dark" {
					t.Fatal("migration rollback changed later global settings", err)
				}
				if cleanup := call("modules.cleanupPackages", map[string]any{}); cleanup.Error != nil {
					t.Fatal("migration backup cleanup failed", cleanup.Error)
				}
				if _, err = os.Stat(migrationBackupPath); !os.IsNotExist(err) {
					t.Fatal("completed migration backup was not recycled", err)
				}
			}
			if faultMode == "configuration" {
				snapshot = call("snapshot", map[string]any{})
				_ = json.Unmarshal(snapshot.Result, &global)
				global.Settings.Theme = "dark"
				if saved := call("settings.save", global.Settings); saved.Error != nil {
					t.Fatal("recovered configuration remained blocked", saved.Error)
				}
			}
			registry, err := modulepackage.OpenRegistry(filepath.Join(profile, "packages"))
			if err != nil {
				t.Fatal(err)
			}
			recoveredRegistry, err := registry.Snapshot()
			if err != nil || recoveredRegistry.Pending != nil || recoveredRegistry.Records["example.tool"].ModuleID == "" {
				t.Fatal("recovery did not retain installed package", err)
			}
			status = call("packages.snapshot", map[string]any{})
			if status.Error != nil || json.Unmarshal(status.Result, &installed) != nil || installed.Packages[0].Running == nil || *installed.Packages[0].Running {
				t.Fatal("recovery retained its probe worker")
			}
		}
		if run == 0 && os.Getenv("WINONE_PACKAGE_RESOURCE_SAMPLE") == "1" {
			samplePackageHost(t, workspace, root, cmd.Process.Pid, "disabled", filepath.Join(bundle, "WinOne.Host.exe"))
		}
		get := call("packages.settings.get", map[string]string{"id": "example.tool"})
		if get.Error != nil {
			t.Fatal(get.Error)
		}
		var settings struct {
			Revision uint64 `json:"revision"`
			Settings struct {
				Size int `json:"size"`
			} `json:"settings"`
		}
		if json.Unmarshal(get.Result, &settings) != nil || settings.Settings.Size != map[int]int{0: 3, 1: 7}[run] {
			t.Fatal("wrong settings after startup", string(get.Result))
		}
		if run == 0 {
			if denied := call("example.tool.echo", map[string]any{}); denied.Error == nil {
				t.Fatal("disabled package accepted work")
			}
			saved := call("packages.settings.save", map[string]any{"id": "example.tool", "revision": settings.Revision, "settings": map[string]int{"size": 5}})
			if saved.Error != nil {
				t.Fatal(saved.Error)
			}
			moduleState := call("modules.snapshot", map[string]any{})
			var current struct {
				Revision uint64 `json:"revision"`
			}
			_ = json.Unmarshal(moduleState.Result, &current)
			if enabled := call("modules.setEnabled", map[string]any{"id": "example.tool", "revision": current.Revision, "enabled": true}); enabled.Error != nil {
				t.Fatal(enabled.Error)
			}
		}
		echo := call("example.tool.echo", map[string]any{})
		if echo.Error != nil {
			t.Fatal(echo.Error)
		}
		var response struct {
			PID      uint32 `json:"processId"`
			Settings struct {
				Size int `json:"size"`
			} `json:"settings"`
		}
		if json.Unmarshal(echo.Result, &response) != nil || response.PID == 0 || response.Settings.Size != map[int]int{0: 5, 1: 7}[run] {
			t.Fatal("worker did not load canonical settings", string(echo.Result))
		}
		workerHandle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, response.PID)
		if err != nil {
			t.Fatal(err)
		}
		defer windows.CloseHandle(workerHandle)
		var imagePath [32768]uint16
		imageLength := uint32(len(imagePath))
		if err = windows.QueryFullProcessImageName(workerHandle, 0, &imagePath[0], &imageLength); err != nil {
			t.Fatal(err)
		}
		actualImage := windows.UTF16ToString(imagePath[:imageLength])
		registry, err := modulepackage.OpenRegistry(filepath.Join(profile, "packages"))
		if err != nil {
			t.Fatal(err)
		}
		inventory, err := registry.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		expectedImage := filepath.Join(profile, "packages", inventory.Records["example.tool"].StageID, "worker.exe")
		if !strings.EqualFold(actualImage, expectedImage) {
			t.Fatal("worker ran outside the verified package", actualImage)
		}
		if run == 0 && os.Getenv("WINONE_PACKAGE_RESOURCE_SAMPLE") == "1" {
			samplePackageHost(t, workspace, root, cmd.Process.Pid, "active", filepath.Join(bundle, "WinOne.Host.exe"), actualImage)
		}
		if run == 0 {
			get = call("packages.settings.get", map[string]string{"id": "example.tool"})
			_ = json.Unmarshal(get.Result, &settings)
			if saved := call("packages.settings.save", map[string]any{"id": "example.tool", "revision": settings.Revision, "settings": map[string]int{"size": 7}}); saved.Error != nil {
				t.Fatal(saved.Error)
			}
			echo = call("example.tool.echo", map[string]any{})
			_ = json.Unmarshal(echo.Result, &response)
			if response.Settings.Size != 7 {
				t.Fatal("running worker did not reload settings")
			}
		}
		if updateMode != "" && updateMode != "migrate-reinstall" && run == 0 {
			beforePID := response.PID
			if updateMode == "disabled" {
				var current struct{ Revision uint64 }
				status := call("modules.snapshot", map[string]any{})
				if err = json.Unmarshal(status.Result, &current); err != nil {
					t.Fatal(err)
				}
				if stopped := call("modules.setEnabled", map[string]any{"id": "example.tool", "revision": current.Revision, "enabled": false}); stopped.Error != nil {
					t.Fatal(stopped.Error)
				}
			}
			previewReply := call("modules.packagePreview.start", map[string]string{"path": updateArchive})
			if previewReply.Error != nil {
				t.Fatal(previewReply.Error)
			}
			var preview packagePreviewView
			if err = json.Unmarshal(previewReply.Result, &preview); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(10 * time.Second)
			for preview.State == "checking" {
				if time.Now().After(deadline) {
					t.Fatal("update preview timed out")
				}
				time.Sleep(10 * time.Millisecond)
				previewReply = call("modules.packagePreview.get", map[string]string{"id": preview.ID})
				if previewReply.Error != nil {
					t.Fatal(previewReply.Error)
				}
				if err = json.Unmarshal(previewReply.Result, &preview); err != nil {
					t.Fatal(err)
				}
			}
			if !preview.CanInstall || preview.Operation != "update" || preview.PreviousVersion != "1.0.0" || preview.WasEnabled != (updateMode != "disabled") {
				t.Fatal("missing update confirmation", preview)
			}
			if strings.HasPrefix(updateMode, "migrate") && (preview.MigrationFrom != 1 || preview.MigrationTo != 2) {
				t.Fatal("configuration migration was not previewed")
			}
			var fileLock windows.Handle
			var configLock windows.Handle
			var protectedConfig []byte
			if updateMode == "migrate-write-failure" {
				path := filepath.Join(profile, "configuration.json")
				protectedConfig, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				name, err := windows.UTF16PtrFromString(path)
				if err != nil {
					t.Fatal(err)
				}
				configLock, err = windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if configLock != 0 {
						windows.CloseHandle(configLock)
					}
				})
			}
			refusalMarker := ""
			if updateMode == "old-stop-refusal" {
				refusalMarker = filepath.Join(profile, "modules", "example.tool", "refuse-shutdown")
			}
			if updateMode == "candidate-stop-refusal" {
				refusalMarker = filepath.Join(profile, "modules", "example.tool", "refuse-candidate-shutdown")
			}
			if refusalMarker != "" {
				if err = os.WriteFile(refusalMarker, []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if updateMode == "locked-retirement" {
				name, err := windows.UTF16PtrFromString(expectedImage)
				if err != nil {
					t.Fatal(err)
				}
				fileLock, err = windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if fileLock != 0 {
						windows.CloseHandle(fileLock)
					}
				})
			}
			updated := call("modules.updatePackage", map[string]string{"id": preview.ID})
			if configLock != 0 {
				if err = windows.CloseHandle(configLock); err != nil {
					t.Fatal(err)
				}
				configLock = 0
				current, err := os.ReadFile(filepath.Join(profile, "configuration.json"))
				if err != nil || string(current) != string(protectedConfig) {
					t.Fatal("failed migration changed canonical configuration", err)
				}
			}
			if updateMode != "invalid-image" && updateMode != "migrate-invalid" && updateMode != "migrate-write-failure" && refusalMarker == "" && updated.Error != nil {
				t.Fatal("update failed", updated.Error)
			}
			if (updateMode == "invalid-image" || updateMode == "migrate-invalid" || updateMode == "migrate-write-failure") && updated.Error == nil {
				t.Fatal("invalid executable update reported success")
			}
			if refusalMarker != "" {
				if updated.Error == nil {
					t.Fatal("unconfirmed resource restoration reported successful update")
				}
				var listing struct {
					Revision uint64
					Modules  []struct {
						Manifest             catalog.Manifest
						Blocked, PendingStop bool
						RecoveryID           string
					}
				}
				status := call("modules.snapshot", map[string]any{})
				if err = json.Unmarshal(status.Result, &listing); err != nil {
					t.Fatal(err)
				}
				recoveryID := ""
				for _, item := range listing.Modules {
					if item.Manifest.ID == "example.tool" && item.Blocked && item.PendingStop {
						recoveryID = item.RecoveryID
					}
				}
				if recoveryID == "" {
					t.Fatal("failed stop did not expose the package recovery action")
				}
				if err = os.Remove(refusalMarker); err != nil {
					t.Fatal(err)
				}
				if recovered := call("modules.recoverPackage", map[string]any{"id": "example.tool", "revision": listing.Revision, "recoveryId": recoveryID}); recovered.Error != nil {
					t.Fatal("package did not recover after resources were released", recovered.Error)
				}
			}
			var updateResult struct{ Warning string }
			_ = json.Unmarshal(updated.Result, &updateResult)
			if updateMode == "locked-retirement" {
				if updateResult.Warning == "" {
					t.Fatal("locked retired files were reported as cleaned")
				}
				if cleanup := call("modules.cleanupPackages", map[string]any{}); cleanup.Error == nil {
					t.Fatal("cleanup ignored the held file")
				}
				if err := windows.CloseHandle(fileLock); err != nil {
					t.Fatal(err)
				}
				fileLock = 0
			}
			if updateResult.Warning != "" {
				if cleanup := call("modules.cleanupPackages", map[string]any{}); cleanup.Error != nil {
					t.Fatal("retired files could not be cleaned after release", cleanup.Error)
				}
			}
			if status, err := windows.WaitForSingleObject(workerHandle, 0); err != nil || status != windows.WAIT_OBJECT_0 {
				t.Fatal("old worker survived update", err)
			}
			snapshot, err := registry.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			wantVersion, wantOutcome := "1.1.0", "committed"
			if updateMode == "invalid-image" || updateMode == "migrate-invalid" || updateMode == "migrate-write-failure" || refusalMarker != "" {
				wantVersion, wantOutcome = "1.0.0", "restored"
			}
			if snapshot.Pending != nil || snapshot.Records["example.tool"].Version != wantVersion || snapshot.LastChange == nil || snapshot.LastChange.Outcome != wantOutcome {
				t.Fatal("wrong update result", snapshot)
			}
			if updateMode != "invalid-image" && updateMode != "migrate-invalid" && updateMode != "migrate-write-failure" && refusalMarker == "" {
				archivePath = updateArchive
			}
			echo = call("example.tool.echo", map[string]any{})
			if updateMode == "disabled" {
				if echo.Error == nil {
					t.Fatal("update enabled a disabled module")
				}
				var listing struct {
					Revision uint64
					Modules  []struct {
						Manifest catalog.Manifest
						Enabled  bool
					}
				}
				status := call("modules.snapshot", map[string]any{})
				if err = json.Unmarshal(status.Result, &listing); err != nil {
					t.Fatal(err)
				}
				for _, item := range listing.Modules {
					if item.Manifest.ID == "example.tool" && item.Enabled {
						t.Fatal("disabled preference lost")
					}
				}
				if enabled := call("modules.setEnabled", map[string]any{"id": "example.tool", "revision": listing.Revision, "enabled": true}); enabled.Error != nil {
					t.Fatal(enabled.Error)
				}
				echo = call("example.tool.echo", map[string]any{})
			}
			if echo.Error != nil || json.Unmarshal(echo.Result, &response) != nil || response.Settings.Size != 7 || response.PID == beforePID {
				t.Fatal("update lost configuration or enable intent", echo)
			}
			if updateMode != "invalid-image" && updateMode != "migrate-invalid" && updateMode != "migrate-write-failure" && refusalMarker == "" {
				if _, err := os.Stat(expectedImage); !os.IsNotExist(err) {
					t.Fatal("old package was not recycled", err)
				}
			} else if _, err := os.Stat(expectedImage); err != nil {
				t.Fatal("rollback lost the old executable", err)
			}
		}
		if installFlow && run == 1 {
			var control struct {
				Revision uint64 `json:"revision"`
			}
			response := call("modules.snapshot", map[string]any{})
			_ = json.Unmarshal(response.Result, &control)
			if disabled := call("modules.setEnabled", map[string]any{"id": "example.tool", "revision": control.Revision, "enabled": false}); disabled.Error != nil {
				t.Fatal(disabled.Error)
			}
			removal := call("modules.packageRemoval.preview", map[string]string{"id": "example.tool"})
			if removal.Error != nil {
				t.Fatal(removal.Error)
			}
			var approved packagePreviewView
			_ = json.Unmarshal(removal.Result, &approved)
			removed := call("modules.removePackage", map[string]string{"id": approved.ID})
			if removed.Error != nil {
				t.Fatal("Host uninstall failed", removed.Error)
			}
			var completion struct{ State, Warning string }
			_ = json.Unmarshal(removed.Result, &completion)
			if completion.State != "completed" {
				t.Fatal("uninstall left unfinished work", string(removed.Result))
			}
			if completion.Warning != "" {
				if cleanup := call("modules.cleanupPackages", map[string]any{}); cleanup.Error != nil {
					t.Fatal("uninstalled files could not be cleaned", cleanup.Error)
				}
			}
			registry, err := modulepackage.OpenRegistry(filepath.Join(profile, "packages"))
			if err != nil {
				t.Fatal(err)
			}
			inventory, err := registry.Snapshot()
			if err != nil || len(inventory.Records) != 0 || inventory.Pending != nil {
				t.Fatal("uninstall retained registration", err)
			}
			if _, err = os.Stat(expectedImage); !os.IsNotExist(err) {
				t.Fatal("uninstall did not recycle payload", err)
			}
			if updateMode == "migrate-reinstall" {
				archivePath = updateArchive
			}
			install()
			get = call("packages.settings.get", map[string]string{"id": "example.tool"})
			if get.Error != nil || json.Unmarshal(get.Result, &settings) != nil || settings.Settings.Size != 7 {
				t.Fatal("reinstall lost retained settings", get.Error)
			}
			if updateMode == "migrate" || updateMode == "migrate-reinstall" {
				var values struct{ Settings struct{ Label string } }
				if json.Unmarshal(get.Result, &values) != nil || values.Settings.Label != "migrated" {
					t.Fatal("migrated values were lost")
				}
			}
			if reply := call("example.tool.echo", map[string]any{}); reply.Error == nil {
				t.Fatal("reinstallation auto-enabled the module")
			}
			if updateMode != "" && os.Getenv("WINONE_PACKAGE_RESOURCE_SAMPLE") == "1" {
				samplePackageHost(t, workspace, root, cmd.Process.Pid, "after-update-reinstall", filepath.Join(bundle, "WinOne.Host.exe"))
			}
		}
		family := observePackageProcessFamily(t, uint32(cmd.Process.Pid))
		observedWorker := false
		proof := []map[string]any{}
		for _, member := range family {
			observedWorker = observedWorker || member.pid == response.PID
			proof = append(proof, map[string]any{"pid": member.pid, "name": member.name, "createdUnixNano": member.created})
		}
		if !observedWorker && !(installFlow && run == 1) {
			t.Fatal("process family observation missed the signed worker")
		}
		proofJSON, _ := json.Marshal(proof)
		if err = os.WriteFile(filepath.Join(root, fmt.Sprintf("process-family-%d.json", run)), proofJSON, 0600); err != nil {
			t.Fatal(err)
		}
		defer func() {
			for _, member := range family {
				windows.CloseHandle(member.handle)
			}
		}()
		requestPackageHostExit(t, uint32(cmd.Process.Pid))
		select {
		case err = <-done:
			if err != nil {
				t.Fatal("Host exit failed", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Host did not exit")
		}
		if status, err := windows.WaitForSingleObject(workerHandle, 0); err != nil || status != windows.WAIT_OBJECT_0 {
			t.Fatal("package worker survived Host shutdown")
		}
		for _, member := range family {
			if status, err := windows.WaitForSingleObject(member.handle, 5000); err != nil || status != windows.WAIT_OBJECT_0 {
				t.Fatal("observed process survived Host shutdown", member.name, err)
			}
		}
		marker, err := os.ReadFile(filepath.Join(profile, "modules", "example.tool", "restored.txt"))
		expectedRestores := run + 1
		if updateMode != "" && updateMode != "migrate-reinstall" {
			expectedRestores += 2
		}
		if updateMode == "migrate-write-failure" {
			expectedRestores++
		}
		if updateMode == "candidate-stop-refusal" {
			expectedRestores += 2
		}
		if installFailure != "" {
			expectedRestores++
		}
		if installFlow && run == 1 {
			expectedRestores++
		}
		if faultRecovery {
			expectedRestores++
			if faultMode == "prepared-stop" || faultMode == "probe-refusal" {
				expectedRestores++
			}
		}
		if err != nil || strings.Count(string(marker), "restored") != expectedRestores {
			t.Fatalf("worker restoration count: got %d, want %d: %v", strings.Count(string(marker), "restored"), expectedRestores, err)
		}
	}
	if err = os.WriteFile(filepath.Join(root, "result.json"), []byte(`{"restart":true,"defaultDisabled":true,"settingsReload":true,"normalHostExit":true,"workerRestored":true,"observedProcessFamilyExited":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if mode := os.Getenv("WINONE_PACKAGE_FAULT_MODE"); mode != "" {
		raw, _ := json.Marshal(map[string]any{"mode": mode, "hostStartedWhileBlocked": true, "globalSettingsAndLogsAvailable": true, "recoveryThroughHost": true})
		if err = os.WriteFile(filepath.Join(root, "fault-recovery.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if installFlow {
		raw, _ := json.Marshal(map[string]any{"preview": true, "defaultDisabled": true, "uninstallRecycled": true, "reinstallKeptSettings": true, "failedCommitRecovered": installFailure})
		if err = os.WriteFile(filepath.Join(root, "installation.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if updateMode != "" {
		raw, _ := json.Marshal(map[string]any{"mode": updateMode, "configurationPreserved": true, "enableIntentPreserved": true, "restart": true, "oldWorkerExited": true})
		if err = os.WriteFile(filepath.Join(root, "update.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

type packageProcessObservation struct {
	handle  windows.Handle
	pid     uint32
	created int64
	name    string
}

func observePackageProcessFamily(t *testing.T, root uint32) []packageProcessObservation {
	t.Helper()
	open := func(pid uint32) (packageProcessObservation, error) {
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			return packageProcessObservation{}, err
		}
		var created, exited, kernel, user windows.Filetime
		if err = windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
			windows.CloseHandle(handle)
			return packageProcessObservation{}, err
		}
		return packageProcessObservation{handle: handle, pid: pid, created: created.Nanoseconds()}, nil
	}
	parent, err := open(root)
	if err != nil {
		t.Fatal(err)
	}
	parent.name = "Host"
	owned := map[uint32]packageProcessObservation{root: parent}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	observedUntil := time.Now().UnixNano()
	if err != nil {
		windows.CloseHandle(parent.handle)
		t.Fatal(err)
	}
	defer windows.CloseHandle(snapshot)
	var entries []windows.ProcessEntry32
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		entries = append(entries, entry)
	}
	if err != windows.ERROR_NO_MORE_FILES {
		windows.CloseHandle(parent.handle)
		t.Fatal(err)
	}
	for changed := true; changed; {
		changed = false
		for _, entry := range entries {
			if _, seen := owned[entry.ProcessID]; seen {
				continue
			}
			ancestor, belongs := owned[entry.ParentProcessID]
			if !belongs {
				continue
			}
			child, err := open(entry.ProcessID)
			if err == windows.ERROR_INVALID_PARAMETER {
				continue
			}
			if err != nil {
				for _, member := range owned {
					windows.CloseHandle(member.handle)
				}
				t.Fatal(err)
			}
			if child.created < ancestor.created || child.created > observedUntil {
				windows.CloseHandle(child.handle)
				continue
			}
			child.name = windows.UTF16ToString(entry.ExeFile[:])
			owned[entry.ProcessID] = child
			changed = true
		}
	}
	result := make([]packageProcessObservation, 0, len(owned))
	for _, member := range owned {
		result = append(result, member)
	}
	return result
}

func samplePackageHost(t *testing.T, workspace, root string, pid int, scenario string, binaries ...string) {
	t.Helper()
	arguments := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(workspace, "build", "measure-resources.ps1"), "-RootProcessIds", fmt.Sprint(pid), "-OutputPath", filepath.Join(root, "resources-"+scenario+".json"), "-Scenario", "signed-package-" + scenario, "-DurationSeconds", "10"}
	// PowerShell -File cannot bind multiple scalar arguments to an array. The
	// process sampler still records the full family; binary hashes are below.
	command := exec.Command("powershell.exe", arguments...)
	command.Dir = workspace
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("resource sample: %v\n%s", err, output)
	}
	proof := map[string]string{}
	for _, file := range binaries {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		proof[file] = hex.EncodeToString(sum[:])
	}
	raw, _ := json.Marshal(proof)
	if err := os.WriteFile(filepath.Join(root, "resources-"+scenario+"-binaries.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func requestPackageHostExit(t *testing.T, pid uint32) {
	t.Helper()
	user32 := windows.NewLazySystemDLL("user32.dll")
	var tray uintptr
	callback := syscall.NewCallback(func(window uintptr, _ uintptr) uintptr {
		var owner uint32
		user32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&owner)))
		if owner == pid {
			var name [256]uint16
			user32.NewProc("GetClassNameW").Call(window, uintptr(unsafe.Pointer(&name[0])), 256)
			if windows.UTF16ToString(name[:]) == "SystrayClass" {
				tray = window
				return 0
			}
		}
		return 1
	})
	user32.NewProc("EnumWindows").Call(callback, 0)
	if tray == 0 {
		t.Fatal("owned Host tray missing")
	}
	if ok, _, err := user32.NewProc("PostMessageW").Call(tray, 0x0111, 3, 0); ok == 0 {
		t.Fatal(err)
	}
}
