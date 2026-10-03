//go:build windows

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"
	"winone/internal/host"
	"winone/internal/transport"
)

func TestCaptureParentSentinel(t *testing.T) {
	if os.Getenv("WINONE_CAPTURE_PARENT_HELPER") != "1" {
		return
	}
	time.Sleep(2 * time.Minute)
}

// The editor opens a generated image. No desktop capture, clipboard write, or user window is involved.
func TestCaptureParentExitPreservesActiveEditor(t *testing.T) {
	exe := os.Getenv("WINONE_CAPTURE_EXE")
	if exe == "" || os.Getenv("WINONE_CAPTURE_NATIVE_TEST") != "1" {
		t.Skip("opt-in editor lifecycle test")
	}
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "fixture.png")
	file, err := os.Create(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	bitmap := image.NewRGBA(image.Rect(0, 0, 64, 64))
	bitmap.Set(1, 1, color.White)
	if err = png.Encode(file, bitmap); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()
	metadata, _ := json.Marshal(map[string]any{"path": imagePath, "width": 64, "height": 64, "copied": false, "pinned": false})
	if err = os.WriteFile(filepath.Join(dir, "last-capture.json"), metadata, 0600); err != nil {
		t.Fatal(err)
	}
	parent := exec.Command(os.Args[0], "-test.run=^TestCaptureParentSentinel$")
	parent.Env = append(os.Environ(), "WINONE_CAPTURE_PARENT_HELPER=1")
	parent.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	if err = parent.Start(); err != nil {
		t.Fatal(err)
	}
	parentWaited := false
	defer func() {
		if !parentWaited {
			parent.Process.Kill()
			parent.Wait()
		}
	}()
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-parent-editor-%d`, time.Now().UnixNano())
	worker := exec.Command(exe, "--pipe", pipe, "--data-dir", dir, "--parent", strconv.Itoa(parent.Process.Pid))
	if err = worker.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- worker.Wait() }()
	waited := false
	defer func() {
		if !waited {
			worker.Process.Kill()
			<-done
		}
	}()
	call := func(method string) (host.Response, error) {
		return transport.Call(context.Background(), pipe, host.Request{Version: 1, ID: "parent-editor", Method: method})
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		r, err := call("snapshot")
		if err == nil && r.Error == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker unavailable")
		}
		time.Sleep(25 * time.Millisecond)
	}
	if r, err := call("edit.last"); err != nil || r.Error != nil {
		t.Fatalf("editor request: %v %v", r.Error, err)
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	var owned []uintptr
	callback := windows.NewCallback(func(h, lparam uintptr) uintptr {
		var pid uint32
		user32.NewProc("GetWindowThreadProcessId").Call(h, uintptr(unsafe.Pointer(&pid)))
		visible, _, _ := user32.NewProc("IsWindowVisible").Call(h)
		if pid == uint32(worker.Process.Pid) && visible != 0 {
			owned = append(owned, h)
		}
		return 1
	})
	deadline = time.Now().Add(5 * time.Second)
	for {
		owned = nil
		user32.NewProc("EnumWindows").Call(callback, 0)
		if len(owned) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("editor did not open")
		}
		time.Sleep(25 * time.Millisecond)
	}
	parent.Process.Kill()
	parent.Wait()
	parentWaited = true
	select {
	case err := <-done:
		waited = true
		t.Fatalf("parent exit discarded the editor: %v", err)
	case <-time.After(time.Second):
	}
	r, err := call("snapshot")
	var state struct {
		Busy bool `json:"busy"`
	}
	if err != nil || r.Error != nil || json.Unmarshal(r.Result, &state) != nil || !state.Busy {
		t.Fatal("active editor was not preserved")
	}
	// Close only this test worker's own editor, simulating the user's cancellation.
	for _, h := range owned {
		user32.NewProc("PostMessageW").Call(h, 0x10, 0, 0)
	}
	select {
	case err := <-done:
		waited = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("orphan worker did not exit after editor closed")
	}
}
