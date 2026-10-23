//go:build windows

package traybox

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

// Exercise UIPI with an actual lower-integrity sender, without elevation or
// touching Explorer. Missing EPOCH/RESUME filters broke elevated WinOne.
func TestListenerAcceptsLowerIntegrityBridgeMessages(t *testing.T) {
	l, err := startListener(listenerConfig{className: brokerWindowClass, native: true,
		target: func(win.HWND) win.HWND { return 0 }}, "", nil, quietLogger{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.stop()
	var original, token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_DUPLICATE, &original); err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	if err := windows.DuplicateTokenEx(original, windows.TOKEN_ALL_ACCESS, nil, windows.SecurityImpersonation, windows.TokenPrimary, &token); err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	sid, err := windows.StringToSid("S-1-16-4096") // low integrity, below the parent broker
	if err != nil {
		t.Fatal(err)
	}
	label := windows.Tokenmandatorylabel{Label: windows.SIDAndAttributes{Sid: sid, Attributes: windows.SE_GROUP_INTEGRITY}}
	if err := windows.SetTokenInformation(token, windows.TokenIntegrityLevel, (*byte)(unsafe.Pointer(&label)), label.Size()); err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(sid)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestBridgeLowerIntegritySender$", "-test.v")
	cmd.Env = append(os.Environ(), "WINTRAY_TEST_LOW_BROKER="+strconv.FormatUint(uint64(l.hwnd), 10))
	// Set integrity before process startup: USER32 caches the process level.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW, Token: syscall.Token(token)}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("lower-integrity bridge messages: %v\n%s", err, output)
	}
}

func TestBridgeLowerIntegritySender(t *testing.T) {
	value := os.Getenv("WINTRAY_TEST_LOW_BROKER")
	if value == "" {
		t.Skip("subprocess used by TestListenerAcceptsLowerIntegrityBridgeMessages")
	}
	hwnd, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		msg     uint32
		allowed bool
		posted  bool
	}{
		{"epoch", msgBridgeEpoch, true, false},
		{"resume", msgBridgeResume, true, false},
		{"posted resume", msgBridgeResume, true, true},
		{"internal fault stays blocked", msgBridgeFailed, false, false},
		{"unrelated message stays blocked", win.WM_APP + 600, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var result uintptr
			var pin runtime.Pinner
			pin.Pin(&result)
			defer pin.Unpin()
			var ok uintptr
			var err error
			if tc.posted {
				// Native recovery uses PostMessage; validating only SendMessage
				// would leave the actual resume notification untested.
				ok, _, err = procPostIconMessageW.Call(uintptr(hwnd), uintptr(tc.msg), 0, 0)
			} else {
				ok, _, err = procSendMessageTimeoutW.Call(uintptr(hwnd), uintptr(tc.msg), 0, 0, smtoAbortIfHung, 500, uintptr(unsafe.Pointer(&result)))
			}
			if tc.allowed && ok == 0 {
				t.Fatalf("bridge message 0x%X blocked: %v", tc.msg, err)
			}
			if !tc.allowed && (ok != 0 || err != windows.ERROR_ACCESS_DENIED) {
				t.Fatalf("unrelated message: delivered=%d err=%v; want access denied", ok, err)
			}
		})
	}
}
