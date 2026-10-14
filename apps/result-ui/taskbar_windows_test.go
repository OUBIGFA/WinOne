package resultui

import (
	"golang.org/x/sys/windows"
	"runtime"
	"testing"
	"unsafe"
)

func TestSelectionWindowNeverQualifiesForTaskbar(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	class, _ := windows.UTF16PtrFromString("STATIC")
	create := frameUser.NewProc("CreateWindowExW")
	window, _, err := create.Call(0x40008, uintptr(unsafe.Pointer(class)), 0, 0xcf0000, 0, 0, 420, 420, 0, 0, 0, 0)
	if window == 0 {
		t.Fatal(err)
	}
	defer frameUser.NewProc("DestroyWindow").Call(window)
	for i := 0; i < 2; i++ {
		if err := prepareSelectionToolWindow(window); err != nil {
			t.Fatal(err)
		}
		ex, _, _ := frameStyle.Call(window, ^uintptr(19))
		if ex&0x80 == 0 || ex&0x40000 != 0 {
			t.Fatalf("selection remains taskbar-eligible: %#x", ex)
		}
		if ex&8 == 0 {
			t.Fatal("topmost behavior changed")
		}
		style, _, _ := frameStyle.Call(window, ^uintptr(15))
		if style&0x40000 == 0 {
			t.Fatal("resizable frame removed")
		}
	}
}
