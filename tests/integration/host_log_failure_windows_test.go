//go:build windows

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/transport"
)

// Only close the hidden tray owner belonging to this test's exact process.
func closeLogFailureTestHost(pid uint32) bool {
	user32 := windows.NewLazySystemDLL("user32.dll")
	posted := false
	callback := windows.NewCallback(func(hwnd, unused uintptr) uintptr {
		var owner uint32
		if _, err := windows.GetWindowThreadProcessId(windows.HWND(hwnd), &owner); err != nil || owner != pid {
			return 1
		}
		var name [128]uint16
		length, _, _ := user32.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
		if length == 0 || windows.UTF16ToString(name[:]) != "SystrayClass" {
			return 1
		}
		result, _, _ := user32.NewProc("PostMessageW").Call(hwnd, 0x10, 0, 0)
		posted = result != 0
		return 0
	})
	user32.NewProc("EnumWindows").Call(callback, 0)
	return posted
}

func TestHostKeepsRunningWithUnavailableDiagnosticLog(t *testing.T) {
	executable := os.Getenv("WINONE_HOST_LOG_FAILURE_EXE")
	if executable == "" {
		t.Skip("set WINONE_HOST_LOG_FAILURE_EXE for an isolated native Host test")
	}
	directory := t.TempDir()
	// Empty shortcuts and a private profile avoid user bindings or startup tasks.
	if err := os.WriteFile(filepath.Join(directory, "settings.json"), []byte(`{"schemaVersion":1,"theme":"system","language":"en-US","captureShortcut":"","pinShortcut":""}`), 0600); err != nil {
		t.Fatal(err)
	}
	marker := []byte("preserve this file")
	if err := os.WriteFile(filepath.Join(directory, "logs"), marker, 0600); err != nil {
		t.Fatal(err)
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-log-failure-%d`, time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "--pipe", pipe, "--data-dir", directory)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	finished := false
	defer func() {
		if !finished {
			_ = cmd.Process.Kill()
			<-done
		}
	}()
	var snapshot host.Snapshot
	for {
		select {
		case err := <-done:
			finished = true
			t.Fatalf("Host exited because logging failed: %v", err)
		default:
		}
		response, err := transport.CallWithTimeout(ctx, pipe, host.Request{Version: 1, ID: "log-startup", Method: "snapshot"}, 250*time.Millisecond)
		if err == nil && response.Error == nil {
			if err = json.Unmarshal(response.Result, &snapshot); err != nil {
				t.Fatal(err)
			}
			break
		}
		if ctx.Err() != nil {
			t.Fatal("Host did not become ready")
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !strings.Contains(strings.ToLower(snapshot.RuntimeWarning), "log") {
		t.Fatal("logging failure was hidden", snapshot.RuntimeWarning)
	}
	snapshot.Settings.Theme = "dark"
	params, _ := json.Marshal(snapshot.Settings)
	response, err := transport.CallCompleting(ctx, pipe, host.Request{Version: 1, ID: "log-save", Method: "settings.save", Params: params}, 5*time.Second)
	if err != nil || response.Error != nil {
		t.Fatal("logging failure blocked settings", response.Error, err)
	}
	committed, err := configuration.ReadCommittedSettings(directory)
	if err != nil || committed.Theme != "dark" {
		t.Fatal("settings were not persisted", err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "logs"))
	if err != nil || string(data) != string(marker) {
		t.Fatal("startup altered the occupied log path", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !closeLogFailureTestHost(uint32(cmd.Process.Pid)) {
		if time.Now().After(deadline) {
			t.Fatal("test Host tray owner not found")
		}
		time.Sleep(25 * time.Millisecond)
	}
	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatal("logging failure changed normal exit", err)
		}
	case <-ctx.Done():
		t.Fatal("test Host did not exit")
	}
	data, err = os.ReadFile(filepath.Join(directory, "logs"))
	if err != nil || string(data) != string(marker) {
		t.Fatal("shutdown altered the occupied log path", err)
	}
}
