//go:build windows

package startupprobe

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/transport"
)

func TestWorkerProbeRejectsScratchInsideInstallation(t *testing.T) {
	installation := t.TempDir()
	scratch := filepath.Join(installation, "scratch")
	if err := os.Mkdir(scratch, 0700); err != nil {
		t.Fatal(err)
	}
	if err := CheckWorkers(context.Background(), installation, scratch); err == nil {
		t.Fatal("installation used for probe data")
	}
	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatal("probe wrote inside installation", err)
	}
}

type workerProbeFixture struct {
	cancel context.CancelFunc
	mode   string
}

func (f workerProbeFixture) Handle(r host.Request) host.Response {
	pid := os.Getpid()
	if f.mode == "identity" {
		pid++
	}
	state := "ready"
	if f.mode == "starting" {
		state = "starting"
	}
	data, _ := json.Marshal(map[string]any{"processId": pid, "initializationState": state})
	if r.Method == "shutdown" {
		if f.mode == "shutdown-hang" {
			time.Sleep(30 * time.Second)
		}
		f.cancel()
	}
	return host.Response{ID: r.ID, Result: data}
}

func runWorkerProbeFixture(pipe string) int {
	mode := os.Getenv("WINONE_WORKER_PROBE_TEST")
	if mode == "exit" {
		return 7
	}
	listener, err := transport.Listen(pipe)
	if err != nil {
		return 2
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := transport.Serve(ctx, listener, workerProbeFixture{cancel: cancel, mode: mode}); err != nil {
		return 3
	}
	return 0
}

func TestWorkerProbeFailsOnBadIdentityAndUnfinishedLifecycle(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"ok", "identity", "exit", "starting", "shutdown-hang"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("WINONE_WORKER_PROBE_TEST", mode)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := checkWorker(ctx, "wintray", executable, t.TempDir())
			if (err == nil) != (mode == "ok") {
				t.Fatalf("%s: %v", mode, err)
			}
		})
	}
}

func TestWorkerReadinessRejectsIncompleteOrFailedInitialization(t *testing.T) {
	for _, tc := range []struct {
		id, value     string
		ready, failed bool
	}{
		{"capture", `{"processId":42}`, true, false},
		{"capture", `{"processId":43}`, false, true},
		{"wintray", `{"processId":42,"initializationState":"starting"}`, false, false},
		{"wintray", `{"processId":42,"initializationState":"ready"}`, true, false},
		{"wintray", `{"processId":42,"initializationState":"faulted","initializationError":"failed"}`, false, true},
		{"wintray", `{"processId":42}`, false, true},
		{"wintray", `{"processId":42,"initializationState":"ready","recoveryError":"failed"}`, false, true},
	} {
		ready, err := workerReady(tc.id, 42, json.RawMessage(tc.value))
		if ready != tc.ready || (err != nil) != tc.failed {
			t.Fatalf("%s: %v %v", tc.value, ready, err)
		}
	}
}

func TestNativeWorkersInitializeAndExit(t *testing.T) {
	installation := os.Getenv("WINONE_PROBE_BUNDLE")
	if installation == "" {
		t.Skip("set WINONE_PROBE_BUNDLE for real Worker probes")
	}
	if err := CheckWorkers(context.Background(), installation, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
