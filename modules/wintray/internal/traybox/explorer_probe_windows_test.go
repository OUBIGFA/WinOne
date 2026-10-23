//go:build windows

package traybox

import (
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// Opt-in receiving-end probe. Keep the lease dormant and do not register,
// hide, or modify any user icons. A fake icon for the foreground window
// would affect an unrelated user application.
func TestShellBridgeRealExplorerAttachProbe(t *testing.T) {
	if os.Getenv("WINTRAY_EXPLORER_PROBE") != "1" {
		t.Skip("interactive probe for the 1460 attach failure")
	}
	target, err := currentShellEndpoint()
	if err != nil {
		t.Fatal(err)
	}
	l, err := startListener(listenerConfig{className: brokerWindowClass, target: explorerTray, native: true}, nativeTestSelfPath(t), nil, quietLogger{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.stop()
	native, err := newNativeBridge(nativeTestSelfPath(t), l.hwnd)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	lease, err := native.attach(target)
	t.Logf("dormant Explorer attach: %s err=%v", time.Since(started).Round(time.Millisecond), err)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.cancel(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "Explorer route detached", func() bool {
		property, _, _ := user32.NewProc("GetPropW").Call(uintptr(target.hwnd), uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("WinOne.WinTray.ShellBridge.Route.v3"))))
		return property == 0
	})
}
