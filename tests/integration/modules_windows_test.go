//go:build windows

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"
	"winone/internal/host"
	"winone/internal/hotkeys"
	"winone/internal/modules"
	"winone/internal/transport"
)

func TestHostModuleDisablePersistsAndReleasesWorkers(t *testing.T) {
	exe := os.Getenv("WINONE_MODULE_HOST_EXE")
	if exe == "" {
		t.Skip("set WINONE_MODULE_HOST_EXE to a complete test bundle")
	}
	dir := t.TempDir()
	// Reserve uncommon bindings in this isolated profile without sending keys.
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"schemaVersion":1,"theme":"system","captureShortcut":"Ctrl+Alt+Shift+F23","pinShortcut":"Ctrl+Alt+Shift+F24"}`), 0600); err != nil {
		t.Fatal(err)
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-module-control-%d`, time.Now().UnixNano())
	start := func() *exec.Cmd {
		cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", dir)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		return cmd
	}
	cmd := start()
	stopped := false
	defer func() {
		if !stopped {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	call := func(method string, params any, out any) error {
		data, _ := json.Marshal(params)
		response, err := transport.CallCompleting(context.Background(), pipe, host.Request{Version: 1, ID: "module-test", Method: method, Params: data}, 5*time.Second)
		if err != nil {
			return err
		}
		if response.Error != nil {
			return fmt.Errorf("%s", response.Error.Message)
		}
		return json.Unmarshal(response.Result, out)
	}
	var snapshot modules.ControlSnapshot
	ready := func() {
		deadline := time.Now().Add(12 * time.Second)
		for {
			if call("modules.snapshot", nil, &snapshot) == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("module controller unavailable")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	ready()
	var initialShortcuts struct {
		Bindings []struct {
			Registered bool `json:"registered"`
		} `json:"bindings"`
	}
	if err := call("shortcuts.snapshot", nil, &initialShortcuts); err != nil {
		t.Fatal(err)
	}
	if len(initialShortcuts.Bindings) != 2 {
		t.Fatalf("expected two shortcut registrations: %+v", initialShortcuts)
	}
	for _, binding := range initialShortcuts.Bindings {
		if !binding.Registered {
			t.Fatal("precondition failed: isolated shortcut was not registered")
		}
	}
	for _, id := range []string{"capture", "always-on-top", "wintray"} {
		// A read-only snapshot starts the worker without capturing or pinning a
		// user window. Retain its OS handle so PID reuse cannot fake an exit.
		var initial any
		if err := call(id+".snapshot", nil, &initial); err != nil {
			t.Fatalf("start %s: %v", id, err)
		}
		workerNames := map[string]string{"capture": "WinOne.Capture.exe", "always-on-top": "WinOne.AlwaysOnTop.exe", "wintray": "WinOne.WinTray.exe"}
		worker := moduleChildHandle(t, uint32(cmd.Process.Pid), workerNames[id])
		defer windows.CloseHandle(worker)
		var job modules.ControlJob
		if err := call("modules.setEnabled", map[string]any{"id": id, "enabled": false, "revision": snapshot.Revision}, &job); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(20 * time.Second)
		for job.State == "running" && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
			if err := call("modules.jobs.get", map[string]string{"id": job.ID}, &job); err != nil {
				t.Fatal(err)
			}
		}
		if job.State != "completed" {
			t.Fatalf("disable %s: %+v", id, job)
		}
		if result, err := windows.WaitForSingleObject(worker, 5000); err != nil || result != windows.WAIT_OBJECT_0 {
			t.Fatalf("disabled %s process did not exit: wait=%d error=%v", id, result, err)
		}
		if err := call("modules.snapshot", nil, &snapshot); err != nil {
			t.Fatal(err)
		}
		var ignored any
		if err := call(id+".snapshot", nil, &ignored); err == nil {
			t.Fatalf("disabled %s still forwarded commands", id)
		}
	}
	assertDisabled := func() {
		if len(snapshot.Modules) != 3 {
			t.Fatal("base module count changed")
		}
		for _, module := range snapshot.Modules {
			if module.Enabled || module.PendingStop || module.Runtime.State != "disabled" {
				t.Fatalf("incomplete stop: %+v", module)
			}
		}
		var shortcuts struct {
			Bindings []struct {
				Registered bool `json:"registered"`
			} `json:"bindings"`
		}
		if err := call("shortcuts.snapshot", nil, &shortcuts); err != nil {
			t.Fatal(err)
		}
		for _, binding := range shortcuts.Bindings {
			if binding.Registered {
				t.Fatal("disabled module shortcut remained registered")
			}
		}
		// Verify the Windows registrations are free, not just the reported state.
		probe := hotkeys.New()
		defer probe.Close()
		statuses, err := probe.Update([]hotkeys.Binding{{ID: "capture-probe", Shortcut: "Ctrl+Alt+Shift+F23"}, {ID: "pin-probe", Shortcut: "Ctrl+Alt+Shift+F24"}})
		if err != nil || len(statuses) != 2 {
			t.Fatalf("reacquire disabled shortcuts: %v, %+v", err, statuses)
		}
		for _, status := range statuses {
			if !status.Registered {
				t.Fatalf("Windows shortcut was not released: %+v", status)
			}
		}
	}
	assertDisabled()
	cmd.Process.Kill()
	cmd.Wait()
	stopped = true
	cmd = start()
	stopped = false
	ready()
	assertDisabled()
	// Let the tray synchronization timer run; it must not restart disabled workers.
	time.Sleep(1500 * time.Millisecond)
	processes, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(processes)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(processes, &entry); err == nil; err = windows.Process32Next(processes, &entry) {
		if entry.ParentProcessID == uint32(cmd.Process.Pid) && strings.HasPrefix(windows.UTF16ToString(entry.ExeFile[:]), "WinOne.") {
			t.Fatal("disabled worker restarted, PID " + strconv.FormatUint(uint64(entry.ProcessID), 10))
		}
	}
	if err != windows.ERROR_NO_MORE_FILES {
		t.Fatalf("enumerate child processes: %v", err)
	}
}

// moduleChildHandle selects only the exact worker owned by this test's Host.
func moduleChildHandle(t *testing.T, parent uint32, name string) windows.Handle {
	t.Helper()
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ParentProcessID == parent && strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), name) {
			handle, openErr := windows.OpenProcess(windows.SYNCHRONIZE, false, entry.ProcessID)
			if openErr != nil {
				t.Fatal(openErr)
			}
			return handle
		}
	}
	t.Fatalf("running child %s missing (enumeration: %v)", name, err)
	return 0
}
