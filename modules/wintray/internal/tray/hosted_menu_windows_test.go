//go:build windows

package tray

import (
	"bytes"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"winone/modules/wintray/internal/traybox"
)

func activateHostedTestControls(t *testing.T) func() {
	t.Helper()
	// go test has no embedded application manifest. Activate the same common
	// controls dependency as the shipped worker for this hidden owner window.
	manifest, err := filepath.Abs("../../../../build/windows/app.manifest")
	if err != nil {
		t.Fatal(err)
	}
	source, err := windows.UTF16PtrFromString(manifest)
	if err != nil {
		t.Fatal(err)
	}
	activation := win.CreateActCtx(&win.ACTCTX{Source: source})
	if activation == 0 || activation == win.HANDLE(^uintptr(0)) {
		t.Fatal("create activation context")
	}
	cookie, ok := win.ActivateActCtx(activation)
	if !ok {
		windows.NewLazySystemDLL("kernel32.dll").NewProc("ReleaseActCtx").Call(uintptr(activation))
		t.Fatal("activate common controls")
	}
	return func() {
		windows.NewLazySystemDLL("kernel32.dll").NewProc("DeactivateActCtx").Call(0, cookie)
		windows.NewLazySystemDLL("kernel32.dll").NewProc("ReleaseActCtx").Call(uintptr(activation))
	}
}

func currentHostedProcessCreation(t *testing.T) windows.Filetime {
	t.Helper()
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user); err != nil {
		t.Fatal(err)
	}
	return created
}

func TestManagedHostUsesShellEntriesWithoutIndependentTrayIcon(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer activateHostedTestControls(t)()
	host := NewHost("zh-CN", testLogger{}, nil)
	host.SetExternalMenu(true)
	info := HostedWindow{Name: "Test program", ProcessID: uint32(os.Getpid())}
	first, err := host.newItem(info)
	if err != nil {
		t.Fatal(err)
	}
	defer first.disposeResources()
	if first.icon != nil || first.show != nil || first.menuID == "" {
		t.Fatal("managed program created independent tray UI")
	}
	host.items[info.ProcessID] = first
	host.SetLanguage("en-US")
	entries := host.MenuEntries()
	if len(entries) != 1 || entries[0].Name != info.Name || entries[0].ID != first.menuID {
		t.Fatal(entries)
	}
	if _, err := png.Decode(bytes.NewReader(entries[0].Image)); err != nil {
		t.Fatal("invalid menu image", err)
	}
	entries[0].Image[0] = 0
	if host.MenuEntries()[0].Image[0] == 0 {
		t.Fatal("snapshot exposed mutable icon bytes")
	}
	second, err := host.newItem(info)
	if err != nil {
		t.Fatal(err)
	}
	defer second.disposeResources()
	second.createdAt = currentHostedProcessCreation(t)
	host.items[info.ProcessID] = second
	if first.menuID == second.menuID {
		t.Fatal("menu identity reused with process ID")
	}
	if err := host.ActivateMenu(first.menuID, "quit"); err == nil {
		t.Fatal("stale identity accepted")
	}
	if err := host.ActivateMenu(second.menuID, "unknown"); err == nil || errors.Is(err, traybox.ErrIconGone) {
		t.Fatal("unknown action accepted")
	}
}

func TestHostedMenuPreservesRecoveryWhenTargetWindowIsMissing(t *testing.T) {
	host := NewHost("zh-CN", testLogger{}, nil)
	pid := uint32(os.Getpid())
	item := &hostedItem{host: host, info: HostedWindow{ProcessID: pid}, menuID: "hosted:live", createdAt: currentHostedProcessCreation(t), stop: make(chan struct{})}
	host.items[pid] = item
	for _, action := range []string{"open", "hide", "release"} {
		if err := host.ActivateMenu(item.menuID, action); err == nil || errors.Is(err, traybox.ErrIconGone) {
			t.Fatalf("%s did not report missing window: %v", action, err)
		}
		if host.Count() != 1 || item.closed {
			t.Fatal("failed action removed recovery entry")
		}
	}
}

func TestHostedMenuShowsHidesAndReleasesItsOwnOffscreenWindow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer activateHostedTestControls(t)()
	class, _ := windows.UTF16PtrFromString("STATIC")
	// Disabled, off-screen windows cannot take keyboard focus from the user.
	target := win.CreateWindowEx(win.WS_EX_TOOLWINDOW|win.WS_EX_NOACTIVATE, class, nil, win.WS_POPUP|win.WS_DISABLED, -10000, -10000, 10, 10, 0, 0, 0, nil)
	if target == 0 {
		t.Fatal("create test target")
	}
	defer win.DestroyWindow(target)
	host := NewHost("zh-CN", testLogger{}, nil)
	host.SetExternalMenu(true)
	if err := host.TryAdd(HostedWindow{Name: "Offscreen test", ProcessID: uint32(os.Getpid()), Handle: uintptr(target)}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, item := range host.items {
			host.remove(item, false)
		}
	}()
	id := host.MenuEntries()[0].ID
	wait := func(ready func() bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for !ready() && time.Now().Before(deadline) {
			var message win.MSG
			for win.PeekMessage(&message, 0, 0, 0, win.PM_REMOVE) {
				win.TranslateMessage(&message)
				win.DispatchMessage(&message)
			}
			time.Sleep(time.Millisecond)
		}
		if !ready() {
			t.Fatal("native window operation did not complete")
		}
	}
	if err := host.ActivateMenu(id, "open"); err != nil {
		t.Fatal(err)
	}
	wait(func() bool { return win.IsWindowVisible(target) })
	if err := host.ActivateMenu(id, "hide"); err != nil {
		t.Fatal(err)
	}
	wait(func() bool { return !win.IsWindowVisible(target) })
	owner := host.items[uint32(os.Getpid())].form
	ownerHandle := owner.Handle()
	if err := host.ActivateMenu(id, "release"); err != nil {
		t.Fatal(err)
	}
	// Walk's loop, unlike a bare Win32 pump, also executes queued UI callbacks.
	// It exits when successful release disposes this never-shown owner window.
	timer := time.AfterFunc(3*time.Second, func() {
		if hostedWindowMatchesProcess(uintptr(ownerHandle), uint32(os.Getpid())) {
			win.PostMessage(ownerHandle, win.WM_CLOSE, 0, 0)
		}
	})
	owner.Run()
	timer.Stop()
	if host.Count() != 0 {
		t.Fatal("release did not remove the restored entry")
	}
	if !win.IsWindowVisible(target) {
		t.Fatal("release removed entry before restoring window")
	}
	if err := host.ActivateMenu(id, "hide"); !errors.Is(err, traybox.ErrIconGone) {
		t.Fatal("released menu identity still accepted", err)
	}
}

func TestHostedMenuRejectsAnotherProcessGeneration(t *testing.T) {
	host := NewHost("zh-CN", testLogger{}, nil)
	pid := uint32(os.Getpid())
	old := &hostedItem{host: host, info: HostedWindow{ProcessID: pid}, menuID: "hosted:old", createdAt: windows.Filetime{LowDateTime: 1}, stop: make(chan struct{})}
	host.items[pid] = old
	if err := host.ActivateMenu(old.menuID, "hide"); !errors.Is(err, traybox.ErrIconGone) {
		t.Fatalf("reused process accepted: %v", err)
	}
	replacement := &hostedItem{host: host, info: HostedWindow{ProcessID: pid}, menuID: "hosted:new", stop: make(chan struct{})}
	host.items[pid] = replacement
	host.remove(old, false)
	if host.items[pid] != replacement || replacement.closed {
		t.Fatal("late cleanup removed replacement")
	}
}
