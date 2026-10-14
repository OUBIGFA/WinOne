//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/transport"
)

// Opt-in artifact verification. It reads build metadata only; even a configured
// fixture never starts an HTTP check or opens a browser.
func TestUpdatesBuildMetadataFromRealHost(t *testing.T) {
	executable := os.Getenv("WINONE_UPDATE_METADATA_HOST")
	if executable == "" {
		t.Skip("requires a built Host and expected update metadata")
	}
	version := os.Getenv("WINONE_UPDATE_METADATA_VERSION")
	configured, err := strconv.ParseBool(os.Getenv("WINONE_UPDATE_METADATA_CONFIGURED"))
	if err != nil || version == "" {
		t.Fatal("missing expected metadata")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-update-metadata-%d-%d`, os.Getpid(), time.Now().UnixNano())
	command := exec.CommandContext(ctx, executable, "--headless", "--standard-session", "--pipe", pipe, "--data-dir", t.TempDir())
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	var logs bytes.Buffer
	command.Stdout = &logs
	command.Stderr = &logs
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	var response host.Response
	for {
		response, err = transport.CallWithTimeout(ctx, pipe, host.Request{Version: 1, ID: "metadata", Method: "updates.info"}, time.Second)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var info updateBuildInfo
	if err = json.Unmarshal(response.Result, &info); err != nil {
		t.Fatal(err)
	}
	if info.Version != version || info.Configured != configured {
		t.Fatalf("wrong built metadata: %+v", info)
	}
}
