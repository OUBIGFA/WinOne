//go:build windows

package systray

import (
	"golang.org/x/sys/windows"
	"os"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

// Explicit system probe: only this test's hidden icon is registered and removed.
// Empty notification content validates the native update without displaying it.
func TestHiddenTrayNotificationRegistration(t *testing.T) {
	if os.Getenv("WINONE_TEST_TRAY_REGISTRATION") != "1" {
		t.Skip("set WINONE_TEST_TRAY_REGISTRATION=1 for the hidden system probe")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	class, _ := windows.UTF16PtrFromString("STATIC")
	window, _, err := pCreateWindowEx.Call(0x08000080, uintptr(unsafe.Pointer(class)), 0, 0x80000000, 0, 0, 1, 1, 0, 0, 0, 0)
	if window == 0 {
		t.Fatal("create hidden owner", err)
	}
	defer pDestroyWindow.Call(window)
	icon, _, err := pLoadIcon.Call(0, 32512)
	if icon == 0 {
		t.Fatal("load shared test icon", err)
	}
	data := notifyIconData{Wnd: windows.Handle(window), ID: 1, Flags: 0x2 | 0x8, Icon: windows.Handle(icon), State: 1, StateMask: 1}
	data.Size = uint32(unsafe.Sizeof(data))
	if err := data.add(); err != nil {
		t.Fatal("register hidden tray icon", err)
	}
	defer func() {
		if err := data.delete(); err != nil {
			t.Error("remove hidden tray icon", err)
		}
	}()
	notice, err := warningNotification(data, "WinOne test", "validation")
	if err != nil {
		t.Fatal(err)
	}
	notice.Info = [256]uint16{}
	if err := notice.modify(); err != nil {
		t.Fatal("update hidden notification data", err)
	}
}

func TestNotifyIconDataMatchesWindowsABI(t *testing.T) {
	var data notifyIconData
	size, title, flags := uintptr(956), uintptr(804), uintptr(932)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		size, title, flags = 976, 820, 948
	}
	if unsafe.Sizeof(data) != size || unsafe.Offsetof(data.InfoTitle) != title || unsafe.Offsetof(data.InfoFlags) != flags {
		t.Fatalf("NOTIFYICONDATAW layout: size=%d title=%d flags=%d", unsafe.Sizeof(data), unsafe.Offsetof(data.InfoTitle), unsafe.Offsetof(data.InfoFlags))
	}
}

func TestNotificationClickUsesTheApplicationOpenAction(t *testing.T) {
	previous := tappedLeft
	defer func() { tappedLeft = previous }()
	calls := 0
	tappedLeft = func() { calls++ }
	tray := winTray{wmSystrayMessage: 0x8123}
	tray.wndProc(0, 0x8123, 0, 0x405) // NIN_BALLOONUSERCLICK.
	if calls != 1 {
		t.Fatal("notification click did not reach the application action")
	}
}

func TestWarningNotificationKeepsIdentityAndDoesNotMutateTrayState(t *testing.T) {
	base := notifyIconData{Wnd: 123, ID: 7, Flags: 0x2 | 0x4 | 0x20, Icon: 456}
	base.Info[0] = 'x'
	base.InfoTitle[0] = 'y'
	next, err := warningNotification(base, "WinOne", "操作未完成")
	if err != nil {
		t.Fatal(err)
	}
	if next.Wnd != base.Wnd || next.ID != base.ID || next.Flags != 0x70 || next.InfoFlags != 0x92 {
		t.Fatal(next)
	}
	if windows.UTF16ToString(next.Info[:]) != "操作未完成" || windows.UTF16ToString(next.InfoTitle[:]) != "WinOne" {
		t.Fatal("incorrect notification text")
	}
	if base.Flags != 0x26 || base.Info[0] != 'x' || base.InfoTitle[0] != 'y' {
		t.Fatal("notification altered persistent icon state")
	}
}

func TestWarningNotificationBoundsUnicodeAndRejectsInvalidText(t *testing.T) {
	next, err := warningNotification(notifyIconData{}, strings.Repeat("a", 62)+"😀", strings.Repeat("b", 254)+"😀")
	if err != nil {
		t.Fatal(err)
	}
	if windows.UTF16ToString(next.InfoTitle[:]) != strings.Repeat("a", 62) || windows.UTF16ToString(next.Info[:]) != strings.Repeat("b", 254) {
		t.Fatal("notification split a surrogate pair")
	}
	for _, message := range []string{"", " ", "text\x00suffix"} {
		if _, err := warningNotification(notifyIconData{}, "WinOne", message); err == nil {
			t.Fatal("invalid notification accepted")
		}
	}
}
