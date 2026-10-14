//go:build windows

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/transport"
	"winone/modules/always-on-top/engine"
)

// Opt-in real Host/worker test. All windows and image data belong to this test;
// the regular test suite never starts this workload or samples the desktop.
func TestHostOverlappingModuleLifetimes(t *testing.T) {
	output := os.Getenv("WINONE_OVERLAP_OUTPUT")
	if output == "" {
		t.Skip("use build/test-module-lifecycle.ps1 -RunSystemChecks")
	}
	bundle, fixture, consoleFixture := os.Getenv("WINONE_OVERLAP_BUNDLE"), os.Getenv("WINONE_WINDOW_FIXTURE"), os.Getenv("WINONE_CONSOLE_FIXTURE")
	if bundle == "" || fixture == "" || consoleFixture == "" {
		t.Fatal("a complete bundle and owned-window fixture are required")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("choose a new overlap output directory")
	}
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(output, "profile")
	seedLifecycleConfiguration(t, profile, map[string]any{"schemaVersion": 1, "theme": "light", "language": "en-US", "captureShortcut": "", "pinShortcut": "", "ocrShortcut": "", "translateShortcut": "", "selectionShortcut": "", "startup": map[string]bool{"runAtLogon": false, "startMinimized": true}})
	draftID := seedOverlapDraft(t, filepath.Join(profile, "modules", "capture"))
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-overlap-%d`, time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	call := func(method string, params any, result any) error {
		data, err := json.Marshal(params)
		if err != nil {
			return err
		}
		requestCtx, stop := context.WithTimeout(ctx, 15*time.Second)
		defer stop()
		r, err := transport.CallCompleting(requestCtx, pipe, host.Request{Version: 1, ID: strconv.FormatInt(time.Now().UnixNano(), 10), Method: method, Params: data}, 5*time.Second)
		if err != nil {
			return err
		}
		if r.Error != nil {
			return fmt.Errorf("%s: %s", method, r.Error.Message)
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(r.Result, result)
	}
	hostLog, err := os.Create(filepath.Join(output, "host.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer hostLog.Close()
	cmd := exec.Command(filepath.Join(bundle, "WinOne.Host.exe"), "--standard-session", "--pipe", pipe, "--data-dir", profile)
	cmd.Stderr = hostLog
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	hostDone := make(chan error, 1)
	go func() { hostDone <- cmd.Wait() }()
	hostExited := false
	t.Cleanup(func() {
		if !hostExited {
			_ = cmd.Process.Kill()
			<-hostDone
		}
	})
	var inventory modules.ControlSnapshot
	waitOverlap(t, "Host readiness", func() bool { return call("modules.snapshot", nil, &inventory) == nil })
	if len(inventory.Modules) != len(catalog.Builtins()) {
		t.Fatal("incomplete module inventory", inventory)
	}
	for _, item := range inventory.Modules {
		if !item.Enabled {
			t.Fatal("module unexpectedly disabled", item)
		}
	}

	// The fourth module has an in-Host listener, not a resident reader process.
	// Automatic selection reads are disabled in the fixture configuration.
	var selectionStatus host.Module
	if err = call("selection.snapshot", nil, &selectionStatus); err != nil || selectionStatus.State != "ready" {
		t.Fatal("selection listener did not start", selectionStatus, err)
	}

	// Distinct executable paths prevent program discovery from confusing these
	// two fixtures, even if a future enumerator first filters by process name.
	pinExe := copyOverlapFixture(t, fixture, filepath.Join(output, "pin-fixture"), "WinOne.PinFixture.exe")
	managedExe := copyOverlapFixture(t, consoleFixture, filepath.Join(output, "managed-fixture"), "WinOne.ManagedFixture.exe")
	pinIdentity := filepath.Join(output, "pin-window.json")
	pinnedProcess := exec.Command(pinExe, pinIdentity)
	if err = pinnedProcess.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pinnedProcess.Process.Kill(); _ = pinnedProcess.Wait() })
	pinned := readOverlapWindow(t, pinIdentity)
	pinHandle := holdOverlapWindow(t, pinned, pinExe)
	_ = pinHandle
	if overlapTopmost(pinned) {
		t.Fatal("pin fixture began topmost")
	}
	var aot engine.Settings
	if err = call("always-on-top.settings.get", nil, &aot); err != nil {
		t.Fatal(err)
	}
	aot.Frame = true
	aot.BlockInGameMode = false
	aot.Sound = false
	if err = call("always-on-top.settings.save", aot, &aot); err != nil {
		t.Fatal(err)
	}
	if err = call("always-on-top.window.set", map[string]any{"window": pinned, "pinned": true}, nil); err != nil {
		t.Fatal(err)
	}
	aotProcess := moduleChildHandle(t, uint32(cmd.Process.Pid), "WinOne.AlwaysOnTop.exe")
	defer windows.CloseHandle(aotProcess)
	aotPID := overlapProcessID(t, aotProcess)
	waitOverlap(t, "real pin border", func() bool {
		return overlapTopmost(pinned) && findOverlapWindow(aotPID, "AlwaysOnTop_Border", false) != 0
	})

	var programs struct {
		Revision string         `json:"revision"`
		Settings map[string]any `json:"settings"`
	}
	if err = call("wintray.settings.get", nil, &programs); err != nil {
		t.Fatal(err)
	}
	managedIdentity := filepath.Join(output, "managed-window.json")
	// Program handling options belong to the existing startup master switch.
	// Saving it after Host startup must not itself launch this owned console.
	exitEvent := fmt.Sprintf(`Local\WinOne-ConsoleFixture-%d`, time.Now().UnixNano())
	programs.Settings["managedApps"] = []map[string]any{{"id": "owned-fixture", "name": "Owned lifecycle fixture", "exePath": managedExe, "args": windows.ComposeCommandLine([]string{managedIdentity, exitEvent}), "runOnStartup": true, "collectTrayIcon": false, "launchViaLogonTask": false, "launchHiddenInBackground": false, "trayBehavior": map[string]any{"autoMinimizeAndHideOnLaunch": true, "closeDelaySeconds": 0}, "schedule": map[string]any{"enabled": false, "startDelayMinutes": 0, "autoExitMinutes": 0, "frequencyEnabled": false, "frequencyDays": 1, "frequencyRuns": 1}}}
	var job captureDraftJob
	if err = call("wintray.settings.save", programs, &job); err != nil {
		t.Fatal(err)
	}
	if job.State != "completed" {
		t.Fatalf("program preferences did not commit: %+v", job)
	}
	if _, err = os.Stat(managedIdentity); !os.IsNotExist(err) {
		t.Fatal("saving preferences launched a program")
	}
	if err = call("wintray.launch", map[string]string{"id": "owned-fixture"}, &job); err != nil {
		t.Fatal(err)
	}
	launchID := job.ID
	waitOverlap(t, "managed program launch", func() bool {
		if err = call("wintray.jobs.get", map[string]string{"id": launchID}, &job); err != nil {
			t.Fatal(err)
		}
		return job.State != "running"
	})
	if job.State != "completed" {
		t.Fatal("managed launch failed", job)
	}
	managed := readOverlapWindow(t, managedIdentity)
	managedHandle := holdOverlapWindow(t, managed, managedExe)
	var console struct {
		ConsoleOwner     uint32 `json:"consoleOwner"`
		ConsoleClients   uint32 `json:"consoleClients"`
		InputRedirected  bool   `json:"inputRedirected"`
		OutputRedirected bool   `json:"outputRedirected"`
	}
	consoleData, err := os.ReadFile(managedIdentity)
	if err != nil || json.Unmarshal(consoleData, &console) != nil || console.ConsoleClients != 1 {
		t.Fatal("console fixture is not isolated", err)
	}
	t.Logf("owned console: client=%d window-owner=%d", managed.PID, console.ConsoleOwner)
	if console.InputRedirected || console.OutputRedirected {
		t.Fatalf("hosted console lost its interactive standard handles: %+v", console)
	}
	strokeProcess := moduleChildHandle(t, uint32(cmd.Process.Pid), "WinOne.Stroke.exe")
	defer windows.CloseHandle(strokeProcess)
	wintrayProcess := moduleChildHandle(t, uint32(cmd.Process.Pid), "WinOne.WinTray.exe")
	defer windows.CloseHandle(wintrayProcess)
	var icons []struct {
		ID      string `json:"id"`
		Hosted  bool   `json:"hosted"`
		ExePath string `json:"exePath"`
	}
	waitOverlap(t, "managed window collection", func() bool {
		if err = call("wintray.icons", nil, &icons); err != nil {
			t.Fatal(err)
		}
		for _, icon := range icons {
			if icon.Hosted && strings.EqualFold(icon.ExePath, managedExe) {
				return !overlapVisible(managed)
			}
		}
		return false
	})

	// Resume a generated legacy draft through the real native entry. No desktop
	// pixels or clipboard contents are collected by the workload.
	if err = call("capture.draft.resume", map[string]string{"expectedId": draftID}, &job); err != nil {
		t.Fatal(err)
	}
	if job.State != "running" || job.ID == "" {
		t.Fatal("native capture did not start", job)
	}
	captureJob := job.ID
	captureProcess := moduleChildHandle(t, uint32(cmd.Process.Pid), "WinOne.Capture.exe")
	defer windows.CloseHandle(captureProcess)
	capturePID := overlapProcessID(t, captureProcess)
	var captureWindow uintptr
	waitOverlap(t, "native capture window", func() bool {
		captureWindow = findOverlapWindow(capturePID, "WindowsForms10.Window.", true)
		return captureWindow != 0
	})
	t.Cleanup(func() {
		if captureWindow != 0 {
			overlapUser32.NewProc("PostMessageW").Call(captureWindow, 0x0205, 0, 0)
		}
	})
	assertActive := func() {
		t.Helper()
		if findOverlapWindow(capturePID, "WindowsForms10.Window.", true) != captureWindow {
			t.Fatal("capture window is no longer visible")
		}
		if !overlapTopmost(pinned) || overlapVisible(managed) {
			t.Fatal("simultaneous workload lost a window state")
		}
		if err = call("capture.jobs.get", map[string]string{"id": captureJob}, &job); err != nil || job.State != "running" {
			t.Fatal("capture stopped during overlap", job, err)
		}
		if findOverlapWindow(aotPID, "AlwaysOnTop_Border", false) == 0 {
			t.Fatal("pin border disappeared during overlap")
		}
	}
	assertActive()
	productPIDs := []uint32{uint32(cmd.Process.Pid), capturePID, aotPID, overlapProcessID(t, wintrayProcess)}
	writeOverlapJSON(t, filepath.Join(output, "measure-context.json"), map[string]any{"productRoots": productPIDs, "modulesEnabled": []string{"capture", "always-on-top", "wintray", "selection"}, "fixtures": []engine.Window{pinned, managed}, "consoleOwner": console.ConsoleOwner, "captureWindow": strconv.FormatUint(uint64(captureWindow), 10), "capturePID": capturePID, "binaries": []string{filepath.Join(bundle, "WinOne.Host.exe"), filepath.Join(bundle, "capture", "WinOne.Capture.exe"), filepath.Join(bundle, "capture", "WinOne.Capture.dll"), filepath.Join(bundle, "always-on-top", "WinOne.AlwaysOnTop.exe"), filepath.Join(bundle, "always-on-top", "WinOne.AlwaysOnTop.Native.dll"), filepath.Join(bundle, "WinOne.WinTray.exe"), pinExe, managedExe, filepath.Join(bundle, "selection", "WinOne.Selection.exe")}})
	measure := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-File", os.Getenv("WINONE_OVERLAP_MEASURE_SCRIPT"), "-ContextFile", filepath.Join(output, "measure-context.json"))
	if data, err := measure.CombinedOutput(); err != nil {
		t.Fatalf("overlap measurement failed: %v\n%s", err, data)
	}
	assertActive()

	// End capture normally, then disable each module through Host. Observe the
	// OS handles and window state, rather than trusting a completed job alone.
	if ok, _, err := overlapUser32.NewProc("PostMessageW").Call(captureWindow, 0x0205, 0, 0); ok == 0 {
		t.Fatal(err)
	}
	waitOverlap(t, "capture cancellation", func() bool {
		if err = call("capture.jobs.get", map[string]string{"id": captureJob}, &job); err != nil {
			t.Fatal(err)
		}
		return job.State != "running"
	})
	if job.State != "cancelled" {
		t.Fatal("capture was not cancelled", job)
	}
	captureWindow = 0
	disables := make(map[string]float64)
	for _, item := range []struct {
		id      string
		process windows.Handle
	}{{"capture", captureProcess}, {"always-on-top", aotProcess}, {"wintray", wintrayProcess}, {"selection", 0}, {"quick-assistant", 0}, {"stroke", strokeProcess}} {
		if err = call("modules.snapshot", nil, &inventory); err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		var stop modules.ControlJob
		if err = call("modules.setEnabled", map[string]any{"id": item.id, "enabled": false, "revision": inventory.Revision}, &stop); err != nil {
			t.Fatal(err)
		}
		waitOverlap(t, "disable "+item.id, func() bool {
			if stop.State == "running" {
				if err = call("modules.jobs.get", map[string]string{"id": stop.ID}, &stop); err != nil {
					t.Fatal(err)
				}
			}
			return stop.State != "running"
		})
		if stop.State != "completed" {
			t.Fatalf("module disable failed: %+v", stop)
		}
		if item.process != 0 {
			if state, err := windows.WaitForSingleObject(item.process, 5000); err != nil || state != windows.WAIT_OBJECT_0 {
				t.Fatalf("%s did not release its process: %v %d", item.id, err, state)
			}
		}
		disables[item.id] = float64(time.Since(started)) / float64(time.Millisecond)
		if disables[item.id] > 5000 {
			t.Fatalf("%s exceeded the 5-second release budget", item.id)
		}
		if item.id == "capture" && (!overlapTopmost(pinned) || overlapVisible(managed)) {
			t.Fatal("capture disable affected another module")
		}
		if item.id == "always-on-top" && (overlapTopmost(pinned) || findOverlapWindow(aotPID, "AlwaysOnTop_Border", false) != 0 || overlapVisible(managed)) {
			t.Fatal("pin disable failed to restore only its window")
		}
	}
	if !overlapVisible(managed) {
		t.Fatal("program management did not restore its hosted window")
	}
	if state, err := windows.WaitForSingleObject(managedHandle, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("module disable closed the managed program")
	}
	if err = call("modules.snapshot", nil, &inventory); err != nil {
		t.Fatal(err)
	}
	for _, item := range inventory.Modules {
		if item.Enabled || item.PendingStop || item.Runtime.State != "disabled" {
			t.Fatal("incomplete final module state", item)
		}
	}
	// Verify the public logging contract after real native and managed work,
	// not only after a clean startup where no logger has emitted an event.
	for _, name := range []string{"modules/capture/worker.log", "modules/always-on-top/worker.log", "modules/wintray/worker.log", "modules/wintray/wintray.log", "modules/selection/worker.log"} {
		if _, err := os.Stat(filepath.Join(profile, name)); !os.IsNotExist(err) {
			t.Fatal("independent diagnostic log remains", name, err)
		}
	}
	if files, err := filepath.Glob(filepath.Join(profile, "modules", "always-on-top", "native", "native*.log")); err != nil || len(files) != 0 {
		t.Fatal("native engine retained its own log", files, err)
	}
	logData, err := os.ReadFile(filepath.Join(profile, "logs", "host.log"))
	if err != nil || !strings.Contains(string(logData), `"source":"always-on-top"`) || !strings.Contains(string(logData), `"source":"wintray"`) {
		t.Fatal("module events missing from unified log", err)
	}
	measure = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-File", os.Getenv("WINONE_OVERLAP_MEASURE_SCRIPT"), "-ContextFile", filepath.Join(output, "measure-context.json"), "-Closed")
	if data, err := measure.CombinedOutput(); err != nil {
		t.Fatalf("post-workload cleanup or idle budget failed: %v\n%s", err, data)
	}
	tray := findOverlapWindow(uint32(cmd.Process.Pid), "SystrayClass", false)
	if tray == 0 {
		t.Fatal("Host has no usable tray window")
	}
	if ok, _, err := overlapUser32.NewProc("PostMessageW").Call(tray, 0x0111, 3, 0); ok == 0 {
		t.Fatal(err)
	}
	select {
	case err := <-hostDone:
		hostExited = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("Host did not exit normally")
	}
	if state, err := windows.WaitForSingleObject(managedHandle, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) || !overlapVisible(managed) {
		t.Fatal("Host exit closed or hid the restored managed program")
	}
	if !overlapVisible(pinned) || overlapTopmost(pinned) {
		t.Fatal("Host exit changed the restored pin fixture")
	}
	writeOverlapJSON(t, filepath.Join(output, "result.json"), map[string]any{"sixModulesEnabled": true, "selectionListenerStopped": true, "nativeCaptureCancelled": true, "pinnedWindowRestored": true, "hostedWindowRestored": true, "managedProgramPreserved": true, "hostExited": true, "disableMilliseconds": disables, "productProcesses": productPIDs, "scope": "Generated capture draft, one real pinned fixture with border, one real managed fixture, and enabled in-Host selection listener with automatic reading off; no settings UI, OCR/AI task, user windows or clipboard input"})
}

var overlapUser32 = windows.NewLazySystemDLL("user32.dll")

func writeOverlapJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func waitOverlap(t *testing.T, name string, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("timed out: " + name)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
func overlapProcessID(t *testing.T, process windows.Handle) uint32 {
	t.Helper()
	id, err := windows.GetProcessId(process)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func overlapWindowHandle(ref engine.Window) uintptr {
	handle, _ := strconv.ParseUint(ref.Handle, 10, 64)
	return uintptr(handle)
}
func overlapVisible(ref engine.Window) bool {
	value, _, _ := overlapUser32.NewProc("IsWindowVisible").Call(overlapWindowHandle(ref))
	return value != 0
}
func overlapTopmost(ref engine.Window) bool {
	style, _, _ := overlapUser32.NewProc("GetWindowLongPtrW").Call(overlapWindowHandle(ref), ^uintptr(19))
	return style&8 != 0
}

// FindWindowEx avoids allocating an unreclaimable Go callback for every poll.
func findOverlapWindow(pid uint32, class string, prefix bool) uintptr {
	var after uintptr
	for {
		window, _, _ := overlapUser32.NewProc("FindWindowExW").Call(0, after, 0, 0)
		if window == 0 {
			return 0
		}
		after = window
		var owner uint32
		overlapUser32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&owner)))
		if owner != pid {
			continue
		}
		var name [256]uint16
		overlapUser32.NewProc("GetClassNameW").Call(window, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
		actual := windows.UTF16ToString(name[:])
		if actual == class || prefix && strings.HasPrefix(actual, class) {
			if prefix {
				visible, _, _ := overlapUser32.NewProc("IsWindowVisible").Call(window)
				if visible == 0 {
					continue
				}
			}
			return window
		}
	}
}
func readOverlapWindow(t *testing.T, path string) engine.Window {
	t.Helper()
	var ref engine.Window
	waitOverlap(t, "owned fixture identity", func() bool {
		data, err := os.ReadFile(path)
		return err == nil && json.Unmarshal(data, &ref) == nil && ref.Handle != "" && ref.PID != 0
	})
	return ref
}
func holdOverlapWindow(t *testing.T, ref engine.Window, executable string) windows.Handle {
	t.Helper()
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, ref.PID)
	if err != nil {
		t.Fatal(err)
	}
	var path [32768]uint16
	size := uint32(len(path))
	if err = windows.QueryFullProcessImageName(p, 0, &path[0], &size); err != nil {
		windows.CloseHandle(p)
		t.Fatal(err)
	}
	var created, exit, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(p, &created, &exit, &kernel, &user); err != nil {
		windows.CloseHandle(p)
		t.Fatal(err)
	}
	stamp := strconv.FormatUint(uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), 10)
	if !strings.EqualFold(windows.UTF16ToString(path[:size]), executable) || stamp != ref.Created {
		windows.CloseHandle(p)
		t.Fatal("fixture identity mismatch")
	}
	t.Cleanup(func() {
		defer windows.CloseHandle(p)
		state, _ := windows.WaitForSingleObject(p, 0)
		if state == windows.WAIT_OBJECT_0 {
			return
		}
		overlapUser32.NewProc("PostMessageW").Call(overlapWindowHandle(ref), 0x0010, 0, 0)
		if state, _ = windows.WaitForSingleObject(p, 3000); state != windows.WAIT_OBJECT_0 {
			_ = windows.TerminateProcess(p, 1)
			_, _ = windows.WaitForSingleObject(p, 3000)
		}
	})
	return p
}
func copyOverlapFixture(t *testing.T, source, directory, name string) string {
	t.Helper()
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(source))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		target := entry.Name()
		if strings.EqualFold(target, filepath.Base(source)) {
			target = name
		}
		data, err := os.ReadFile(filepath.Join(filepath.Dir(source), entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(directory, target), data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(directory, name)
}
func seedOverlapDraft(t *testing.T, directory string) string {
	t.Helper()
	id := strings.Repeat("a", 32)
	bitmap := image.NewRGBA(image.Rect(0, 0, 800, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 800; x++ {
			bitmap.SetRGBA(x, y, color.RGBA{R: 210, G: 225, B: 240, A: 255})
		}
	}
	for _, sub := range []string{"drafts", "previews"} {
		if err := os.MkdirAll(filepath.Join(directory, sub), 0700); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(filepath.Join(directory, sub, id+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, bitmap)
		closeErr := file.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	writeOverlapJSON(t, filepath.Join(directory, "capture-draft.json"), map[string]any{"schemaVersion": 1, "id": id, "revision": 0, "width": 800, "height": 600, "previewReady": true, "sourceKind": "window", "settings": map[string]any{"outputDirectory": filepath.Join(directory, "exports"), "copyToClipboard": false, "askWhereToSave": false, "openEditor": true}, "document": map[string]any{"version": 1, "width": 800, "height": 600, "marks": []map[string]any{{"kind": "cover", "x": 120, "y": 100, "width": 160, "height": 100, "color": "#cc3344"}}}})
	return id
}
