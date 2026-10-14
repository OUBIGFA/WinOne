//go:build windows

package integration

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unsafe"
	"winone/internal/host"
	"winone/internal/processidentity"
	"winone/internal/transport"
)

// The shared profile can already have a Host from the installed copy when a
// portable UI opens. A tray exit must not leave that connected UI behind.
func TestTrayExitClosesConnectedDesktop(t *testing.T) {
	bundle := os.Getenv("WINONE_DESKTOP_EXIT_BUNDLE")
	if bundle == "" {
		t.Skip("explicit owned desktop/Host exit test")
	}
	for _, scenario := range []struct {
		name                     string
		otherInstallation, crash bool
	}{{"false", false, false}, {"true", true, false}, {"crash", false, true}} {
		t.Run(scenario.name, func(t *testing.T) {
			otherInstallation := scenario.otherInstallation
			profile := t.TempDir()
			seedLifecycleConfiguration(t, profile, map[string]any{"schemaVersion": 1, "theme": "light", "language": "zh-CN", "captureShortcut": "", "pinShortcut": "", "ocrShortcut": "", "translateShortcut": "", "selectionShortcut": ""})
			pipe := fmt.Sprintf(`\\.\pipe\WinOne-desktop-exit-%d`, time.Now().UnixNano())
			backend := exec.Command(filepath.Join(bundle, "WinOne.Host.exe"), "--standard-session", "--pipe", pipe, "--data-dir", profile)
			if err := backend.Start(); err != nil {
				t.Fatal(err)
			}
			backendDone := make(chan error, 1)
			go func() { backendDone <- backend.Wait() }()
			t.Cleanup(func() { _ = backend.Process.Kill() })
			waitOverlap(t, "owned Host ready", func() bool {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				r, err := transport.CallCompleting(ctx, pipe, host.Request{Version: 1, ID: "ready", Method: "snapshot"}, time.Second)
				return err == nil && r.Error == nil
			})
			frontend := filepath.Join(bundle, "WinOne.exe")
			if otherInstallation {
				// The component UI embeds its assets. A second copy needs no
				// independent Host: it is connected to the explicit pipe above.
				frontend = filepath.Join(t.TempDir(), "WinOne.exe")
				data, err := os.ReadFile(filepath.Join(bundle, "WinOne.exe"))
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(frontend, data, 0700); err != nil {
					t.Fatal(err)
				}
			}
			ui := exec.Command(frontend, "--standard-session", "--pipe", pipe, "--data-dir", profile)
			if err := ui.Start(); err != nil {
				t.Fatal(err)
			}
			uiDone := make(chan error, 1)
			go func() { uiDone <- ui.Wait() }()
			uiExited := false
			var closeRequestedAt time.Time
			var descendants []windows.Handle
			t.Cleanup(func() {
				if !uiExited {
					if closeRequestedAt.IsZero() {
						closeRequestedAt = time.Now()
					}
					_ = ui.Process.Kill()
					select {
					case <-uiDone:
					case <-time.After(5 * time.Second):
						t.Error("fixture UI failed to exit")
					}
				}
				if len(descendants) > 0 {
					// Use the project's 15-second full UI/runtime shutdown budget
					// (docs/WinOne架构与开发计划.md), measured from the close request.
					remaining := 15*time.Second - time.Since(closeRequestedAt)
					if remaining < 0 {
						remaining = 0
					}
					state, err := windows.WaitForMultipleObjects(descendants, true, uint32(remaining.Milliseconds()))
					if err != nil || state != windows.WAIT_OBJECT_0 {
						t.Errorf("UI helpers did not exit: %d %v", state, err)
						for _, handle := range descendants {
							if status, _ := windows.WaitForSingleObject(handle, 0); status == uint32(windows.WAIT_TIMEOUT) {
								pid, _ := windows.GetProcessId(handle)
								name := make([]uint16, 32768)
								size := uint32(len(name))
								_ = windows.QueryFullProcessImageName(handle, 0, &name[0], &size)
								t.Logf("remaining helper: pid=%d image=%s", pid, windows.UTF16ToString(name[:size]))
							}
						}
					}
					if state == windows.WAIT_OBJECT_0 {
						t.Logf("Complete UI process group exited in %s", time.Since(closeRequestedAt))
					}
					for _, handle := range descendants {
						windows.CloseHandle(handle)
					}
				}
			})
			waitOverlap(t, "owned visible desktop", func() bool { return findOverlapWindow(uint32(ui.Process.Pid), "WinOne.Desktop", true) != 0 })
			descendants = holdDesktopDescendants(t, uint32(ui.Process.Pid))
			tray := findOverlapWindow(uint32(backend.Process.Pid), "SystrayClass", false)
			if tray == 0 {
				t.Fatal("owned Host tray missing")
			}
			started := time.Now()
			closeRequestedAt = started
			if scenario.crash {
				if err := backend.Process.Kill(); err != nil {
					t.Fatal(err)
				}
			} else if ok, _, err := overlapUser32.NewProc("PostMessageW").Call(tray, 0x0111, 3, 0); ok == 0 {
				t.Fatal(err)
			}
			select {
			case err := <-backendDone:
				if (err != nil) != scenario.crash {
					t.Fatal(err)
				}
			case <-time.After(8 * time.Second):
				t.Fatal("tray exit did not end Host")
			}
			if scenario.crash {
				select {
				case <-uiDone:
					uiExited = true
					t.Fatal("Host crash removed the diagnostics window")
				case <-time.After(750 * time.Millisecond):
				}
				window := findOverlapWindow(uint32(ui.Process.Pid), "WinOne.Desktop", true)
				closeRequestedAt = time.Now()
				overlapUser32.NewProc("PostMessageW").Call(window, 0x10, 0, 0)
				select {
				case err := <-uiDone:
					uiExited = true
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("diagnostics window did not close")
				}
				// An explicit/initial refresh may already have recovered the crashed
				// Host. Clean up that exact test-pipe owner, not another WinOne session.
				closeRecoveredDesktopHost(t, pipe, filepath.Join(bundle, "WinOne.Host.exe"))
				return
			}
			select {
			case err := <-uiDone:
				uiExited = true
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("desktop survived the connected Host tray exit")
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := transport.CallCompleting(ctx, pipe, host.Request{Version: 1, ID: "after-quit", Method: "snapshot"}, time.Second)
			if !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
				t.Fatalf("a refresh resurrected the stopped Host: %v", err)
			}
			t.Logf("Host and connected desktop exited in %s", time.Since(started))
		})
	}
}

func closeRecoveredDesktopHost(t *testing.T, pipe, executable string) {
	t.Helper()
	key, _ := windows.UTF16PtrFromString(pipe)
	h, err := windows.CreateFile(key, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED, 0)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	var pid uint32
	ok, _, queryErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetNamedPipeServerProcessId").Call(uintptr(h), uintptr(unsafe.Pointer(&pid)))
	windows.CloseHandle(h)
	if ok == 0 {
		t.Fatal(queryErr)
	}
	process, err := processidentity.Open(pid, executable)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	var tray uintptr
	waitOverlap(t, "recovered fixture tray", func() bool { tray = findOverlapWindow(pid, "SystrayClass", false); return tray != 0 })
	overlapUser32.NewProc("PostMessageW").Call(tray, 0x111, 3, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := process.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

// Retain real process handles before exit; WebView2 teardown can outlive the
// window process. Birth times exclude children of an earlier reused parent PID.
func holdDesktopDescendants(t *testing.T, parent uint32) []windows.Handle {
	t.Helper()
	birth := func(handle windows.Handle) uint64 {
		var created, exited, kernel, user windows.Filetime
		if err := windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
			t.Fatal(err)
		}
		return uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	}
	owner, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, parent)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(owner)
	seen := map[uint32]uint64{parent: birth(owner)}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	var rows []windows.ProcessEntry32
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		rows = append(rows, entry)
	}
	if err != windows.ERROR_NO_MORE_FILES {
		t.Fatal(err)
	}
	var handles []windows.Handle
	for changed := true; changed; {
		changed = false
		for _, row := range rows {
			born, ok := seen[row.ParentProcessID]
			if !ok {
				continue
			}
			if _, ok = seen[row.ProcessID]; ok {
				continue
			}
			h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, row.ProcessID)
			if err == windows.ERROR_INVALID_PARAMETER {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			created := birth(h)
			if created < born {
				windows.CloseHandle(h)
				continue
			}
			seen[row.ProcessID] = created
			handles = append(handles, h)
			changed = true
		}
	}
	if len(handles) > 64 {
		for _, h := range handles {
			windows.CloseHandle(h)
		}
		t.Fatal("unexpected desktop process tree size")
	}
	return handles
}
