//go:build windows

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
	"winone/internal/hotkeys"
	"winone/internal/settings"
)

func TestShortcutRecordingOwnerHelper(t *testing.T) {
	if os.Getenv("WINONE_RECORDING_OWNER") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "ready")
	io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

func TestRecordingRestoresWhenOwnerExits(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "-test.run=^TestShortcutRecordingOwnerHelper$")
	command.Env = append(os.Environ(), "WINONE_RECORDING_OWNER=1")
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	if line, err := bufio.NewReader(output).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatal(line, err)
	}
	registrar := &recordingRegistrar{}
	runtime := &shortcutRuntime{registrar: registrar}
	runtime.applyConfiguration(settings.Settings{CaptureShortcut: "Ctrl+Alt+S"})
	token, err := runtime.beginRecording(uint32(command.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.endRecording(token)
	input.Close()
	if err = command.Wait(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		runtime.mu.Lock()
		restored := runtime.recording == nil && len(runtime.statuses) == 7 && runtime.statuses[0].Registered && runtime.statuses[2].Registered && runtime.statuses[3].Registered && runtime.statuses[6].ID == "open-winone" && runtime.statuses[6].Registered
		runtime.mu.Unlock()
		if restored {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("recording owner exited but bindings stayed suspended")
}

type recordingRegistrar struct{ active []hotkeys.Binding }

func (r *recordingRegistrar) Update(values []hotkeys.Binding) ([]hotkeys.Status, error) {
	r.active = append([]hotkeys.Binding(nil), values...)
	var result []hotkeys.Status
	for _, value := range values {
		result = append(result, hotkeys.Status{Binding: value, Registered: value.Shortcut != ""})
	}
	return result, nil
}
func (r *recordingRegistrar) Events() <-chan string { return nil }
func (r *recordingRegistrar) Close() error          { return nil }
func TestRecordingReleasesAndRestoresLatestBindings(t *testing.T) {
	registrar := &recordingRegistrar{}
	runtime := &shortcutRuntime{registrar: registrar}
	configuration := settings.Settings{SchemaVersion: 1, Theme: "system", CaptureShortcut: "Ctrl+Alt+S", PinShortcut: "Ctrl+Alt+T"}
	if err := runtime.applyConfiguration(configuration); err != nil {
		t.Fatal(err)
	}
	token, err := runtime.beginRecording(uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.endRecording(token)
	if len(registrar.active) != 0 {
		t.Fatal("global shortcuts remained registered")
	}
	if err = runtime.endRecording("wrong"); err == nil {
		t.Fatal("unrelated session ended recording")
	}
	configuration.PinShortcut = "Ctrl+Shift+F9"
	if err = runtime.applyConfiguration(configuration); err != nil {
		t.Fatal(err)
	}
	if len(registrar.active) != 0 {
		t.Fatal("save reactivated shortcuts during recording")
	}
	if err = runtime.endRecording(token); err != nil {
		t.Fatal(err)
	}
	if len(registrar.active) != 7 || registrar.active[1].Shortcut != "Ctrl+Shift+F9" || registrar.active[2].Shortcut != "Shift+C" || registrar.active[3].Shortcut != "Alt+Shift+A" || registrar.active[5].Shortcut != "Ctrl+Shift+Q" || registrar.active[6].ID != "open-winone" || registrar.active[6].Shortcut != "Alt+A" {
		t.Fatal(registrar.active)
	}
}

func TestNativeRecordingReleasesAndRestoresRegistration(t *testing.T) {
	if os.Getenv("WINONE_NATIVE_HOTKEY_TEST") != "1" {
		t.Skip("set WINONE_NATIVE_HOTKEY_TEST=1")
	}
	registrar, competitor := hotkeys.New(), hotkeys.New()
	defer registrar.Close()
	defer competitor.Close()
	runtime := &shortcutRuntime{registrar: registrar}
	// Exercise the selected binding only; do not reserve the user's default
	// OCR, translation or selection keys during this isolated native test.
	empty := ""
	value := settings.Settings{CaptureShortcut: "Ctrl+Alt+Shift+F23", OCRShortcut: &empty, TranslateShortcut: &empty, SelectionShortcut: &empty, QuickAssistantShortcut: &empty, OpenShortcut: &empty}
	captureRegistered := func() bool {
		for _, status := range runtime.statuses {
			if status.ID == "capture" && status.Shortcut == value.CaptureShortcut {
				return status.Registered
			}
		}
		return false
	}
	if err := runtime.applyConfiguration(value); err != nil {
		t.Fatal(err)
	}
	if !captureRegistered() {
		t.Fatal("test shortcut unavailable", runtime.statuses)
	}
	token, err := runtime.beginRecording(uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.endRecording(token)
	status, err := competitor.Update([]hotkeys.Binding{{ID: "probe", Shortcut: value.CaptureShortcut}})
	if err != nil || !status[0].Registered {
		t.Fatal("recording did not release global key", status, err)
	}
	if _, err = competitor.Update(nil); err != nil {
		t.Fatal(err)
	}
	if err = runtime.endRecording(token); err != nil {
		t.Fatal(err)
	}
	if !captureRegistered() {
		t.Fatal("registration did not resume")
	}
}
