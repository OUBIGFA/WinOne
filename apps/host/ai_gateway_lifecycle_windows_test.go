//go:build windows

package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
	"winone/internal/host"
)

func TestGatewayViewerProcessHelper(t *testing.T) {
	if os.Getenv("WINONE_GATEWAY_VIEWER_HELPER") != "1" {
		return
	}
	io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

// The UI may close its pipe before a final release RPC reaches the Host.
// Process exit alone must release a temporary gateway viewer.
func TestGatewayViewerExitReleasesPrivateBackend(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("explicit isolated AI runtime")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	viewer := exec.CommandContext(ctx, self, "-test.run=^TestGatewayViewerProcessHelper$")
	viewer.Env = append(os.Environ(), "WINONE_GATEWAY_VIEWER_HELPER=1")
	input, err := viewer.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := viewer.Start(); err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	service := newAIService(ctx, t.TempDir(), "unused.exe")
	service.desktopExecutable = self
	service.translator.Executable = executable
	service.initGateway()
	defer service.closeGatewayBackend()
	response := service.handle(host.Request{ID: "open", Method: "gateway.open", PeerPID: uint32(viewer.Process.Pid), Params: json.RawMessage(`{"viewer":"private-viewer-fixture"}`)})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	service.backend.mu.Lock()
	worker := service.backend.process
	service.backend.mu.Unlock()
	if worker == nil || !worker.Alive() {
		t.Fatal("private gateway did not start")
	}
	input.Close()
	if err := viewer.Wait(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for worker.Alive() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if worker.Alive() {
		t.Fatal("private gateway survived its window without an explicit close RPC")
	}
}

func TestGatewayViewerReleaseKeepsOtherViewersAndEnabledGateway(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("explicit isolated AI runtime")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := newAIService(ctx, t.TempDir(), "unused.exe")
	service.desktopExecutable = self
	service.translator.Executable = executable
	service.initGateway()
	defer service.gateway.Close()
	defer service.closeGatewayBackend()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	settings := service.gateway.Status().GatewaySettings
	settings.Port = listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	settings.Enabled = true
	if _, err := service.gateway.Save(settings, false); err != nil {
		t.Fatal(err)
	}
	worker, err := service.openGatewayBackend("first-viewer-fixture", uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.openGatewayBackend("second-viewer-fixture", uint32(os.Getpid())); err != nil {
		t.Fatal(err)
	}
	service.releaseGatewayBackend("first-viewer-fixture")
	service.releaseGatewayBackend("first-viewer-fixture") // A late duplicate cannot release the newer page.
	if !worker.Alive() || len(service.backend.viewers) != 1 {
		t.Fatal("an old page closed another viewer's gateway")
	}
	service.releaseGatewayBackend("second-viewer-fixture")
	if !worker.Alive() || !service.gateway.Status().Running {
		t.Fatal("closing settings stopped the enabled public gateway")
	}
	settings = service.gateway.Status().GatewaySettings
	settings.Enabled = false
	raw, _ := json.Marshal(map[string]any{"settings": settings})
	response := service.handleGateway(host.Request{Method: "gateway.save", Params: raw})
	if response.Error != nil || worker.Alive() {
		t.Fatal("disabling the unviewed gateway did not release its worker", response.Error)
	}
}
