//go:build windows

package traybox

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"unsafe"

	"github.com/lxn/win"
)

// Re-enabling collection must work after the previous route has detached.
// A lease still on a callback stack is intentionally rejected with 183; it
// is not evidence of the cross-integrity handshake failure reported as 1460.
func TestShellBridgeReattachAfterCancellation(t *testing.T) {
	if os.Getenv("WINTRAY_SHELL_BRIDGE_TEST") != "1" {
		t.Skip("build/native.ps1 -Test and WINTRAY_SHELL_BRIDGE_TEST=1")
	}
	for _, arch := range []string{"x64", "x86"} {
		t.Run(arch, func(t *testing.T) {
			shell := startNativeClient(t, nativeClientPath(t, arch), "--shell")
			client := startNativeClient(t, nativeClientPath(t, arch), "--target", strconv.FormatUint(uint64(shell.hwnd), 10))
			l, err := startListener(listenerConfig{className: brokerWindowClass, target: func(win.HWND) win.HWND { return shell.hwnd }, native: true}, "", nil, quietLogger{})
			if err != nil {
				t.Fatal(err)
			}
			defer l.stop()
			created, err := processCreation(shell.pid)
			if err != nil {
				t.Fatal(err)
			}
			hostCreated, err := processCreation(uint32(os.Getpid()))
			if err != nil {
				t.Fatal(err)
			}
			native := &nativeBridge{broker: l.hwnd, host: bridgeTarget{pid: uint32(os.Getpid()), created: hostCreated}, dir: filepath.Dir(filepath.Dir(nativeClientPath(t, arch))), cached: map[string]string{}}
			target := shellEndpoint{bridgeTarget{shell.pid, created, shell.path}, shell.hwnd}

			for cycle := 0; cycle < 2; cycle++ {
				func() {
					lease, err := native.attach(target)
					if err != nil {
						t.Fatal(err)
					}
					defer func() {
						if err := lease.cancel(); err != nil {
							t.Error(err)
						}
					}()
					if err := lease.selectPaths([]string{client.path}); err != nil {
						t.Fatal(err)
					}
					if err := l.updatePaths([]string{client.path}); err != nil {
						t.Fatal(err)
					}
					if err := lease.activate(); err != nil {
						t.Fatal(err)
					}
					if result := client.request(t, win.WM_APP+110, uintptr(55+cycle), 0); result != 73 || len(l.snapshot()) != 1 {
						t.Fatalf("cycle %d: collection result=%d icons=%d", cycle, result, len(l.snapshot()))
					}
					if err := l.updatePaths(nil); err != nil {
						t.Fatal(err)
					}
				}()
				waitFor(t, "cancelled route detached", func() bool {
					property, _, _ := user32.NewProc("GetPropW").Call(uintptr(shell.hwnd), uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("WinOne.WinTray.ShellBridge.Route.v3"))))
					return property == 0
				})
			}
		})
	}
}
