//go:build windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/modulestate"
	"winone/internal/transport"
)

// Same empty profile/load on two explicit binaries. This measures the cost of
// adding package support, not the cost or performance of a real third-party tool.
func TestProductHostPackageSupportResourceComparison(t *testing.T) {
	if os.Getenv("WINONE_PACKAGE_RESOURCE_COMPARE") != "1" {
		t.Skip("explicit resource comparison")
	}
	baseline, candidate, output := os.Getenv("WINONE_PACKAGE_BASELINE_EXE"), os.Getenv("WINONE_PACKAGE_CANDIDATE_EXE"), os.Getenv("WINONE_PACKAGE_RESOURCE_OUTPUT")
	if baseline == "" || candidate == "" || output == "" {
		t.Fatal("set comparison binaries and output")
	}
	if err := os.Mkdir(output, 0700); err != nil {
		t.Fatal(err)
	}
	workspace, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for index, executable := range []string{baseline, candidate} {
		label := []string{"baseline", "candidate"}[index]
		directory := filepath.Join(output, label)
		if err = os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		states := modulestate.Snapshot{SchemaVersion: 2, Modules: map[string]modulestate.Record{}}
		for _, module := range catalog.Builtins() {
			states.Modules[module.ID] = modulestate.Record{}
		}
		raw, _ := json.Marshal(states)
		if err = os.WriteFile(filepath.Join(directory, "module-state.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(directory, "settings.json"), []byte(`{"schemaVersion":1,"captureShortcut":"","pinShortcut":"","ocrShortcut":"","translateShortcut":"","selectionShortcut":""}`), 0600); err != nil {
			t.Fatal(err)
		}
		pipe := `\\.\pipe\WinOne-package-resources-` + strconv.Itoa(os.Getpid()) + "-" + label
		command := exec.Command(executable, "--pipe", pipe, "--data-dir", directory, "--standard-session")
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
		deadline := time.Now().Add(10 * time.Second)
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			_, err = transport.Call(ctx, pipe, host.Request{Version: 1, ID: "ready", Method: "snapshot"})
			cancel()
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("comparison Host did not start", err)
			}
			time.Sleep(20 * time.Millisecond)
		}
		samplePackageHost(t, workspace, directory, command.Process.Pid, "empty", executable)
		requestPackageHostExit(t, uint32(command.Process.Pid))
		select {
		case err = <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("comparison Host did not exit")
		}
	}
}
