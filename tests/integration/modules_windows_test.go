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
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/hotkeys"
	"winone/internal/modules"
	"winone/internal/settings"
	"winone/internal/transport"
)

func TestHostModuleDisablePersistsAndReleasesWorkers(t *testing.T) {
	exe := os.Getenv("WINONE_MODULE_HOST_EXE")
	if exe == "" {
		t.Skip("set WINONE_MODULE_HOST_EXE to a complete test bundle")
	}
	dir := t.TempDir()
	// All five bindings are explicit. Automatic selection reading stays off in
	// the isolated profile, so this lifecycle test cannot read user selections.
	seedLifecycleConfiguration(t, dir, map[string]any{"schemaVersion": 1, "theme": "system", "captureShortcut": "Ctrl+Alt+Shift+F23", "pinShortcut": "Ctrl+Alt+Shift+F24", "ocrShortcut": "Ctrl+Alt+Shift+F21", "translateShortcut": "Ctrl+Alt+Shift+F22", "selectionShortcut": "Ctrl+Alt+Shift+F20", "quickAssistantShortcut": "Ctrl+Alt+Shift+F19", "openShortcut": ""})
	expectedBindings := []hotkeys.Binding{
		{ID: "capture", Shortcut: "Ctrl+Alt+Shift+F23"}, {ID: "pin", Shortcut: "Ctrl+Alt+Shift+F24"},
		{ID: "ocr", Shortcut: "Ctrl+Alt+Shift+F21"}, {ID: "translate", Shortcut: "Ctrl+Alt+Shift+F22"},
		{ID: "selection", Shortcut: "Ctrl+Alt+Shift+F20"}, {ID: "quick-assistant", Shortcut: "Ctrl+Alt+Shift+F19"},
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
		Bindings []hotkeys.Status `json:"bindings"`
	}
	if err := call("shortcuts.snapshot", nil, &initialShortcuts); err != nil {
		t.Fatal(err)
	}
	if len(initialShortcuts.Bindings) != len(expectedBindings)+1 {
		t.Fatalf("missing shortcut registrations: %+v", initialShortcuts)
	}
	for _, actual := range initialShortcuts.Bindings {
		if actual.ID == "open-winone" && (actual.Shortcut != "" || actual.Registered) {
			t.Fatal("fixture unexpectedly enabled the global window shortcut", actual)
		}
	}
	for _, expected := range expectedBindings {
		found := false
		for _, actual := range initialShortcuts.Bindings {
			if actual.ID == expected.ID && actual.Shortcut == expected.Shortcut && actual.Registered {
				found = true
			}
		}
		if !found {
			t.Fatalf("precondition failed: isolated shortcut not registered: %+v; got %+v", expected, initialShortcuts)
		}
	}
	assertAvailable := func(bindings []hotkeys.Binding) {
		t.Helper()
		probe := hotkeys.New()
		defer func() {
			if err := probe.Close(); err != nil {
				t.Error(err)
			}
		}()
		statuses, err := probe.Update(bindings)
		if err != nil || len(statuses) != len(bindings) {
			t.Fatalf("reacquire disabled shortcuts: %v, %+v", err, statuses)
		}
		for _, status := range statuses {
			if !status.Registered {
				t.Fatalf("Windows shortcut was not released: %+v", status)
			}
		}
	}
	for _, id := range []string{"capture", "always-on-top", "wintray", "selection", "quick-assistant", "stroke"} {
		// A read-only snapshot starts the worker without capturing or pinning a
		// user window. Retain its OS handle so PID reuse cannot fake an exit.
		var initial any
		if err := call(id+".snapshot", nil, &initial); err != nil {
			t.Fatalf("start %s: %v", id, err)
		}
		workerNames := map[string]string{"capture": "WinOne.Capture.exe", "always-on-top": "WinOne.AlwaysOnTop.exe", "wintray": "WinOne.WinTray.exe", "stroke": "WinOne.Stroke.exe"}
		// Selection owns hooks in Host and a one-shot reader, not a resident worker.
		var worker windows.Handle
		if id != "selection" && id != "quick-assistant" {
			worker = moduleChildHandle(t, uint32(cmd.Process.Pid), workerNames[id])
			defer windows.CloseHandle(worker)
		}
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
		if worker != 0 {
			if result, err := windows.WaitForSingleObject(worker, 5000); err != nil || result != windows.WAIT_OBJECT_0 {
				t.Fatalf("disabled %s process did not exit: wait=%d error=%v", id, result, err)
			}
		} else {
			assertAvailable([]hotkeys.Binding{expectedBindings[4]})
		}
		if err := call("modules.snapshot", nil, &snapshot); err != nil {
			t.Fatal(err)
		}
		var ignored any
		if err := call(id+".snapshot", nil, &ignored); err == nil {
			t.Fatalf("disabled %s still forwarded commands", id)
		}
		if id == "capture" {
			// Disabling capture releases its actions while pin and selection stay active.
			var shortcuts struct {
				Bindings []hotkeys.Status `json:"bindings"`
			}
			if err := call("shortcuts.snapshot", nil, &shortcuts); err != nil {
				t.Fatal(err)
			}
			if len(shortcuts.Bindings) != len(expectedBindings)+1 {
				t.Fatal("shortcut inventory changed", shortcuts)
			}
			for _, binding := range shortcuts.Bindings {
				if binding.Registered != (binding.ID == "pin" || binding.ID == "selection" || binding.ID == "quick-assistant") {
					t.Fatalf("module boundary crossed: %+v", binding)
				}
			}
			assertAvailable([]hotkeys.Binding{expectedBindings[0], expectedBindings[2], expectedBindings[3]})
		}
	}
	assertDisabled := func() {
		if len(snapshot.Modules) != len(catalog.Builtins()) {
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
		assertAvailable(expectedBindings)
		var current struct {
			Settings settings.Settings `json:"settings"`
		}
		if err := call("snapshot", nil, &current); err != nil {
			t.Fatal(err)
		}
		if current.Settings.CaptureShortcut != expectedBindings[0].Shortcut || current.Settings.PinShortcut != expectedBindings[1].Shortcut || current.Settings.OCRBinding() != expectedBindings[2].Shortcut || current.Settings.TranslateBinding() != expectedBindings[3].Shortcut || current.Settings.SelectionBinding() != expectedBindings[4].Shortcut {
			t.Fatal("module disable erased shortcut preferences")
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
			// Query access also lets overlap probes read the identity of this
			// held process; SYNCHRONIZE alone supports waiting but not GetProcessId.
			handle, openErr := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
			if openErr != nil {
				t.Fatal(openErr)
			}
			return handle
		}
	}
	t.Fatalf("running child %s missing (enumeration: %v)", name, err)
	return 0
}

// Seed the committed configuration before Host starts its hooks. This tests the
// real enabled listener without allowing it to read selections in other apps.
func seedLifecycleConfiguration(t *testing.T, directory string, global map[string]any) {
	t.Helper()
	global["quickAssistantShortcut"] = valueOrDisabled(global, "quickAssistantShortcut")
	global["openShortcut"] = ""
	global["startup"] = map[string]bool{"runAtLogon": false, "startMinimized": true}
	writeOverlapJSON(t, filepath.Join(directory, "configuration.json"), map[string]any{"committed": map[string]any{
		"schemaVersion": 1, "revision": 1, "builtinVersion": 2,
		"values": map[string]any{"host": global, "selection": map[string]any{"schemaVersion": 1, "automatic": false, "keyboard": false, "excludedApps": []string{}}},
	}})
}

// Existing tests can explicitly opt into a binding; missing defaults must not steal real user shortcuts.
func valueOrDisabled(values map[string]any, key string) any {
	if value, ok := values[key]; ok {
		return value
	}
	return ""
}
