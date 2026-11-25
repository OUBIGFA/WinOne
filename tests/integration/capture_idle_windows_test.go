//go:build windows

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/transport"
)

func TestCaptureIdleExitDespiteStatusPolling(t *testing.T) {
	exe := os.Getenv("WINONE_CAPTURE_EXE")
	if exe == "" || os.Getenv("WINONE_CAPTURE_IDLE_TEST") != "1" {
		t.Skip("opt-in 75-second idle lifecycle check")
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-idle-%d`, time.Now().UnixNano())
	directory := t.TempDir()
	projection, _ := json.Marshal(map[string]any{"schemaVersion": 1, "revision": 0, "outputDirectory": filepath.Join(directory, "images")})
	if err := os.WriteFile(filepath.Join(directory, "managed-settings.json"), projection, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", directory)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	waited := false
	defer func() {
		if !waited {
			cmd.Process.Kill()
			<-done
		}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	timeout := time.NewTimer(75 * time.Second)
	defer timeout.Stop()
	started := time.Now()
	ready := false
	for {
		select {
		case err := <-done:
			waited = true
			if err != nil || !ready || time.Since(started) < 55*time.Second {
				t.Fatalf("unexpected early exit: ready=%t elapsed=%s err=%v", ready, time.Since(started), err)
			}
			return
		case <-timeout.C:
			t.Fatal("read-only queries prevented idle shutdown")
		case <-ticker.C:
			response, err := transport.Call(context.Background(), pipe, host.Request{Version: 1, ID: "idle-probe", Method: "snapshot"})
			if err == nil && response.Error == nil {
				ready = true
			}
		}
	}
}
