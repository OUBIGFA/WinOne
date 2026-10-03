//go:build windows

package integration

import (
	"bytes"
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

// This verifies the published .NET worker without taking a screenshot or changing the clipboard.
func TestCaptureWorkerProtocol(t *testing.T) {
	exe := os.Getenv("WINONE_CAPTURE_EXE")
	if exe == "" {
		t.Skip("set WINONE_CAPTURE_EXE")
	}
	dir := t.TempDir()
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-capture-test-%d`, time.Now().UnixNano())
	cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", dir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var exitError error
	go func() { exitError = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			_ = cmd.Process.Kill()
			<-done
		}
	})
	call := func(method string, params any) (host.Response, error) {
		data, _ := json.Marshal(params)
		return transport.Call(context.Background(), pipe, host.Request{Version: 1, ID: "capture-test", Method: method, Params: data})
	}
	var response host.Response
	deadline := time.Now().Add(10 * time.Second)
	for {
		r, err := call("snapshot", nil)
		if err == nil && r.Error == nil {
			response = r
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("capture not ready: %+v %v", r, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	var snapshot struct {
		Settings map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(response.Result, &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Settings["outputDirectory"] = filepath.Join(dir, "images")
	snapshot.Settings["copyToClipboard"] = false
	response, err := call("settings.save", snapshot.Settings)
	if err != nil || response.Error != nil {
		t.Fatalf("save: %+v %v", response, err)
	}
	response, err = call("settings.save", snapshot.Settings)
	if err != nil || response.Error == nil {
		t.Fatalf("stale save accepted: %+v %v", response, err)
	}
	response, err = call("pin.last", nil)
	if err != nil || response.Error != nil {
		t.Fatalf("job request: %+v %v", response, err)
	}
	var job struct {
		ID    string `json:"id"`
		State string `json:"state"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(response.Result, &job)
	deadline = time.Now().Add(5 * time.Second)
	for job.State == "running" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		response, err = call("jobs.get", map[string]string{"id": job.ID})
		if err != nil || response.Error != nil {
			t.Fatalf("job polling: %+v %v", response, err)
		}
		_ = json.Unmarshal(response.Result, &job)
	}
	if job.State != "failed" || job.Error == "" {
		t.Fatalf("missing image was not reported: %+v", job)
	}
	response, err = call("shutdown", nil)
	if err != nil || response.Error != nil {
		t.Fatalf("shutdown: %v %v", response.Error, err)
	}
	// Acknowledging shutdown must close admission immediately, before deferred UI teardown.
	response, err = call("copy.last", nil)
	if err == nil && response.Error == nil {
		t.Fatalf("new image job accepted after shutdown: %+v", response)
	}
	select {
	case <-done:
		if exitError != nil {
			t.Fatalf("%v: %s", exitError, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("capture worker did not exit")
	}
}
