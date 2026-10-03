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
	"winone/internal/settings"
	"winone/internal/transport"
	"winone/modules/always-on-top/engine"
)

func TestAOTManagedConfigurationProcess(t *testing.T) {
	exe := os.Getenv("WINONE_AOT_EXE")
	if exe == "" {
		t.Skip("set WINONE_AOT_EXE for native configuration checks")
	}
	directory := t.TempDir()
	projection := filepath.Join(directory, "managed-settings.json")
	value := engine.Defaults()
	value.Revision = 7
	value.FrameThickness = 5
	write := func() {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = settings.WriteAtomic(projection, data); err != nil {
			t.Fatal(err)
		}
	}
	write()
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-aot-config-%d`, time.Now().UnixNano())
	cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", directory)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	call := func(method string, params json.RawMessage) (host.Response, error) {
		return transport.Call(context.Background(), pipe, host.Request{Version: 1, ID: "configuration-test", Method: method, Params: params})
	}
	deadline := time.Now().Add(10 * time.Second)
	var response host.Response
	var err error
	for {
		response, err = call("snapshot", nil)
		if err == nil && response.Error == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not start", err, response.Error)
		}
		time.Sleep(50 * time.Millisecond)
	}
	var snapshot struct {
		Settings engine.Settings `json:"settings"`
		Managed  bool            `json:"configurationManaged"`
	}
	if err = json.Unmarshal(response.Result, &snapshot); err != nil {
		t.Fatal(err)
	}
	if !snapshot.Managed || snapshot.Settings.FrameThickness != 5 {
		t.Fatal(snapshot)
	}
	data, _ := json.Marshal(value)
	response, err = call("settings.save", data)
	if err != nil || response.Error == nil {
		t.Fatal("worker accepted direct save", err)
	}
	value.Revision = 8
	value.FrameThickness = 12
	write()
	response, err = call("settings.reload", nil)
	if err != nil || response.Error != nil {
		t.Fatal(err, response.Error)
	}
	var actual engine.Settings
	if err = json.Unmarshal(response.Result, &actual); err != nil {
		t.Fatal(err)
	}
	if actual.Revision != 8 || actual.FrameThickness != 12 {
		t.Fatal(actual)
	}
	response, err = call("shutdown", nil)
	if err != nil || response.Error != nil {
		t.Fatal(err, response.Error)
	}
}
