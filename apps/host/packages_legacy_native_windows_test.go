//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modulestate"
	"winone/internal/transport"
)

// Runs the production Host and its queued configuration command through the
// real pipe. The unsafe stage must be reported and preserved, so this scenario
// never invokes the system recycle bin or touches real user configuration.
func TestNativeLegacyCleanupPreservesUnverifiedStage(t *testing.T) {
	goExe := os.Getenv("WINONE_LEGACY_CLEANUP_GO")
	if goExe == "" {
		t.Skip("explicit production Host legacy-cleanup check")
	}
	root := t.TempDir()
	if output := os.Getenv("WINONE_LEGACY_CLEANUP_OUTPUT"); output != "" {
		root = output
		if err := os.Mkdir(root, 0700); err != nil {
			t.Fatal(err)
		}
	}
	profile := filepath.Join(root, "profile")
	packages := filepath.Join(profile, "packages")
	if err := os.MkdirAll(packages, 0700); err != nil {
		t.Fatal(err)
	}
	state := modulestate.Snapshot{SchemaVersion: 2, Modules: map[string]modulestate.Record{}}
	for _, module := range catalog.Builtins() {
		state.Modules[module.ID] = modulestate.Record{}
	}
	raw, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(profile, "module-state.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "settings.json"), []byte(`{"schemaVersion":1,"captureShortcut":"","pinShortcut":"","ocrShortcut":"","translateShortcut":"","selectionShortcut":""}`), 0600); err != nil {
		t.Fatal(err)
	}
	archive, publisher := hostPackageArchiveFixture(t, []byte("inert stage; never execute"))
	checked, err := modulepackage.VerifyArchive(context.Background(), archive, publisher)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := modulepackage.StageArchive(context.Background(), archive, modulepackage.StageOptions{Directory: packages, Publisher: publisher, Environment: packageTestEnvironment, ExpectedManifestSHA256: checked.Manifest().Digest()})
	if err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(stage.Directory(), "keep.txt")
	if err = os.WriteFile(keep, []byte("unowned content"), 0600); err != nil {
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
	build := exec.Command("powershell.exe", "-NoProfile", "-File", filepath.Join(workspace, "build", "build-host.ps1"), "-GoExe", goExe, "-OutputDirectory", bundle, "-ModuleTrustFile", trustFile)
	build.Dir = workspace
	build.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("Host build: %v: %s", err, output)
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-legacy-cleanup-%d`, os.Getpid())
	command := exec.Command(filepath.Join(bundle, "WinOne.Host.exe"), "--pipe", pipe, "--data-dir", profile, "--standard-session")
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	log, err := os.Create(filepath.Join(root, "host.log"))
	if err != nil {
		t.Fatal(err)
	}
	command.Stdout = log
	command.Stderr = log
	if err = command.Start(); err != nil {
		log.Close()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	finished := &atomic.Bool{}
	go func() { err := command.Wait(); log.Close(); finished.Store(true); done <- err }()
	defer func() {
		if !finished.Load() {
			_ = command.Process.Kill()
			<-done
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		_, err = transport.Call(ctx, pipe, host.Request{Version: 1, ID: "ready", Method: "snapshot"})
		cancel()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Host not ready", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	call := func(method string, params any) host.Response {
		t.Helper()
		raw, _ := json.Marshal(params)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		reply, err := transport.Call(ctx, pipe, host.Request{Version: 1, ID: "check", Method: method, Params: raw})
		if err != nil {
			t.Fatal(err)
		}
		var job struct {
			ID string `json:"configurationJob"`
		}
		_ = json.Unmarshal(reply.Result, &job)
		for job.ID != "" {
			value, _ := json.Marshal(map[string]string{"id": job.ID})
			polled, err := transport.Call(ctx, pipe, host.Request{Version: 1, ID: "poll", Method: "configuration.jobs.get", Params: value})
			if err != nil {
				t.Fatal(err)
			}
			var status host.ConfigurationJob
			if err = json.Unmarshal(polled.Result, &status); err != nil {
				t.Fatal(err)
			}
			if status.State != "running" {
				if status.Response == nil {
					t.Fatal("missing operation result")
				}
				return *status.Response
			}
			time.Sleep(20 * time.Millisecond)
		}
		return reply
	}
	status := call("modules.snapshot", map[string]any{})
	var pending struct {
		Cleanup packageCleanupStatus `json:"cleanup"`
	}
	if status.Error != nil || json.Unmarshal(status.Result, &pending) != nil || !pending.Cleanup.Pending {
		t.Fatalf("legacy cleanup not visible: response=%s error=%+v", status.Result, status.Error)
	}
	configuration := filepath.Join(profile, "configuration.json")
	before, err := os.ReadFile(configuration)
	if err != nil {
		t.Fatal(err)
	}
	reply := call("modules.cleanupPackages", map[string]any{})
	if reply.Error == nil || !strings.Contains(reply.Error.Message, filepath.Base(stage.Directory())) {
		t.Fatal("unsafe cleanup was not reported", reply)
	}
	if after, err := os.ReadFile(configuration); err != nil || string(after) != string(before) {
		t.Fatal("cleanup changed saved configuration", err)
	}
	if raw, err := os.ReadFile(keep); err != nil || string(raw) != "unowned content" {
		t.Fatal("cleanup changed unrelated file", err)
	}
	requestPackageHostExit(t, uint32(command.Process.Pid))
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Host did not exit")
	}
	raw, _ = json.MarshalIndent(map[string]any{"cleanupVisible": true, "unsafeStageRetained": true, "configurationPreserved": true, "hostExited": true}, "", "  ")
	if err = os.WriteFile(filepath.Join(root, "result.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}
