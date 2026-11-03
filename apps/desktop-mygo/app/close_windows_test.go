//go:build windows

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"

	"winone/desktopmygo/core"
	"winone/desktopmygo/shell"
	"winone/internal/settings"
	"winone/internal/transport"
)

// These tests run the real window lifecycle in children on a private desktop.
// The only Host is the read-only pipe fixture; all settings/placement files
// stay in the explicitly selected scratch directory. Ordinary go test skips
// them rather than opening windows on the user's desktop.
func TestWindowCloseLifecycle(t *testing.T) {
	root := os.Getenv("WINONE_MYGO_CLOSE_CHECK_DIR")
	if root == "" {
		t.Skip("build/test-desktop-mygo-close.ps1 selects isolated native checks")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var timings []map[string]any
	for _, mode := range []string{"unset", "legacy-on", "legacy-off", "tray", "dirty", "busy", "slow-disposal"} {
		t.Run(mode, func(t *testing.T) {
			directory := filepath.Join(root, mode)
			if err := os.MkdirAll(directory, 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			pipe := fmt.Sprintf(`\\.\pipe\WinOne-Close-Check-%d-%d`, os.Getpid(), time.Now().UnixNano())
			listener, err := transport.Listen(pipe)
			if err != nil {
				t.Fatal(err)
			}
			h := newNativeProbeHost()
			if mode != "unset" {
				keep := mode != "legacy-off"
				h.snapshot.Settings.Startup = &settings.StartupPreferences{KeepUIRunning: &keep, PreferAdministrator: true}
			}
			go transport.Serve(ctx, listener, h)
			cmd := exec.CommandContext(ctx, self, "--close-window-helper", pipe, directory, mode)
			cmd.Dir = directory
			cmd.Env = append(os.Environ(), "APPDATA="+directory, "LOCALAPPDATA="+directory, "MYGO_GPU=0")
			var output bytes.Buffer
			cmd.Stdout, cmd.Stderr = &output, &output
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			exited := make(chan struct{})
			var exitErr error
			go func() { exitErr = cmd.Wait(); close(exited) }()
			defer func() { cancel(); <-exited }()
			await := func(condition func() bool, budget time.Duration, message string) {
				t.Helper()
				deadline := time.Now().Add(budget)
				for !condition() {
					select {
					case <-exited:
						t.Fatalf("%s: child exited %v; %s", message, exitErr, output.String())
					default:
					}
					if time.Now().After(deadline) {
						t.Fatal(message)
					}
					time.Sleep(5 * time.Millisecond)
				}
			}
			window := uintptr(0)
			await(func() bool {
				window = lifecycleWindow(pipe, uint32(cmd.Process.Pid))
				return window != 0 && fileExists(filepath.Join(directory, "ready"))
			}, 4*time.Second, "window never became ready")
			// Another launch must wake the same live instance, not create a new
			// resident window. It must also terminate its own launcher process.
			second := exec.CommandContext(ctx, self, "--close-window-helper", pipe, directory, mode)
			second.Env = cmd.Env
			if data, err := second.CombinedOutput(); err != nil {
				t.Fatalf("wake: %v; %s", err, data)
			}
			if got := lifecycleWindow(pipe, uint32(cmd.Process.Pid)); got != window {
				t.Fatalf("wake replaced the existing window: 0x%X -> 0x%X", window, got)
			}
			started := time.Now()
			wparam := uintptr(0)
			if mode == "tray" {
				wparam = 1 // Host's tray-exit WM_CLOSE contract
			}
			if ok, _, err := user32.NewProc("PostMessageW").Call(window, 0x10, wparam, 0); ok == 0 {
				t.Fatal(err)
			}
			if mode == "dirty" || mode == "busy" {
				time.Sleep(100 * time.Millisecond)
				if lifecycleWindow(pipe, uint32(cmd.Process.Pid)) != window {
					t.Fatal("unapproved close hid/discarded the guarded window")
				}
				if err := os.WriteFile(filepath.Join(directory, "allow"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				started = time.Now()
			}
			if mode == "slow-disposal" {
				await(func() bool { return fileExists(filepath.Join(directory, "disposing")) }, time.Second, "page disposal never started")
				if visible, _, _ := procIsWindowVisible.Call(window); visible != 0 {
					t.Fatal("slow disposal left a frozen visible window")
				}
				if err := os.WriteFile(filepath.Join(directory, "release"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case <-exited:
				if exitErr != nil {
					t.Fatalf("close: %v; %s", exitErr, output.String())
				}
			case <-time.After(2 * time.Second):
				t.Fatal("UI process remained resident after closing")
			}
			if !fileExists(filepath.Join(directory, "disposed")) || !fileExists(filepath.Join(directory, placementFile)) {
				t.Fatal("closing skipped page cleanup or placement persistence")
			}
			// Closing, not just hiding, must release the single-instance mutex.
			mutex, _, _, _ := names(pipe)
			name, _ := windows.UTF16PtrFromString(mutex)
			handle, err := windows.OpenMutex(windows.SYNCHRONIZE, false, name)
			if handle != 0 {
				windows.CloseHandle(handle)
				t.Fatal("closed process retained the instance identity")
			}
			if err != windows.ERROR_FILE_NOT_FOUND {
				t.Fatal(err)
			}
			timings = append(timings, map[string]any{"mode": mode, "closeToProcessExitMs": time.Since(started).Milliseconds(), "processId": cmd.Process.Pid})
		})
	}
	data, _ := json.MarshalIndent(timings, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "close-timings.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

// MyGo's native loop must run on the main goroutine, not testing's test
// goroutine. Only explicitly selected private-desktop children take this path.
func TestMain(m *testing.M) {
	if os.Getenv("WINONE_MYGO_CLOSE_CHECK_DIR") != "" && len(os.Args) == 5 && os.Args[1] == "--close-window-helper" {
		if err := runWindowCloseHelper(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// Each helper has a fresh MyGo app and registry. The slow closer deliberately
// waits for the parent to inspect the real HWND: this tests visibility ordering
// without relying on scheduler-dependent guesses about a sleep duration.
func runWindowCloseHelper() error {
	pipe, directory, mode := os.Args[2], os.Args[3], os.Args[4]
	mygo.App.SetName("WinOne Close Check")
	mygo.App.SetPath(mygo.PathUserData, directory)
	page := &lifecyclePage{directory: directory, mode: mode}
	shell.Register("overview", core.Workspace, func(env *core.Env) core.Page {
		page.env = env
		return page
	})
	if err := runWindow(Options{Pipe: pipe, Directory: directory}, plan{}); err != nil {
		return err
	}
	return page.err
}

type lifecyclePage struct {
	err             error
	env             *core.Env
	directory, mode string
	ready           bool
}

func (p *lifecyclePage) View(c *ui.Context) {
	waiting := !fileExists(filepath.Join(p.directory, "allow"))
	p.env.Activity.Report("lifecycle", p.mode == "dirty" && waiting, p.mode == "busy" && waiting, nil)
	ui.Text(c, "Isolated close check")
	if !p.ready {
		p.ready = true
		if err := os.WriteFile(filepath.Join(p.directory, "ready"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			p.err = err
		}
	}
	if waiting && (p.mode == "dirty" || p.mode == "busy") {
		c.After(10 * time.Millisecond)
	}
}

func (p *lifecyclePage) Close() {
	if p.mode == "slow-disposal" {
		if err := os.WriteFile(filepath.Join(p.directory, "disposing"), nil, 0600); err != nil {
			p.err = err
		}
		deadline := time.Now().Add(3 * time.Second)
		for !fileExists(filepath.Join(p.directory, "release")) && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if err := os.WriteFile(filepath.Join(p.directory, "disposed"), nil, 0600); err != nil {
		p.err = err
	}
}

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }

// Follow the application's single-instance redirect, not the first visible
// top-level window: MyGo also creates tooltip and guard popup HWNDs.
func lifecycleWindow(pipe string, pid uint32) uintptr {
	_, class, title, _ := names(pipe)
	className, _ := windows.UTF16PtrFromString(class)
	titleName, _ := windows.UTF16PtrFromString(title)
	notification, _, _ := procFindWindowExW.Call(hwndMessage, 0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(titleName)))
	window, _, _ := procGetWindowLongPtrW.Call(notification, gwlpUserData)
	var owner uint32
	procGetWindowThreadPID.Call(window, uintptr(unsafe.Pointer(&owner)))
	visible, _, _ := procIsWindowVisible.Call(window)
	if owner == pid && visible != 0 {
		return window
	}
	return 0
}
