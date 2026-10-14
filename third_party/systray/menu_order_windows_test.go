//go:build windows

package systray

import (
	"golang.org/x/sys/windows"
	"testing"
	"unsafe"
)

func TestCollectedItemsStayBeforeFooterAcrossUpdates(t *testing.T) {
	menu, _, err := pCreatePopupMenu.Call()
	if menu == 0 {
		t.Fatal(err)
	}
	wt.menus = map[uint32]windows.Handle{0: windows.Handle(menu)}
	wt.menuOf = make(map[uint32]windows.Handle)
	wt.visibleItems = make(map[uint32][]uint32)
	wt.menuItemIcons = make(map[uint32]windows.Handle)
	wt.initialized.Store(true)
	defer func() { wt.initialized.Store(false); pDestroyMenu.Call(menu) }()
	AddSeparator()
	open := AddMenuItem("Open", "")
	quit := AddMenuItem("Quit", "")
	first := AddMenuItemFirst("First program", "")
	second := AddMenuItemFirst("Second program", "")
	defer open.Remove()
	defer quit.Remove()
	defer first.Remove()
	defer second.Remove()
	first.AddSubMenuItem("Original menu", "")
	getID := func(index int) uint32 {
		t.Helper()
		info := menuItemInfo{Mask: 2}
		info.Size = uint32(unsafe.Sizeof(info))
		ok, _, err := windows.NewLazySystemDLL("user32.dll").NewProc("GetMenuItemInfoW").Call(menu, uintptr(index), 1, uintptr(unsafe.Pointer(&info)))
		if ok == 0 {
			t.Fatal(err)
		}
		return info.ID
	}
	check := func(want ...*MenuItem) {
		t.Helper()
		for index, item := range want {
			got := getID(index)
			if got != item.id {
				t.Fatalf("position %d: %d, want %d", index, got, item.id)
			}
		}
	}
	check(first, second)
	first.Hide()
	first.Show()
	first.SetTitle("Updated program")
	check(first, second)
	second.Remove()
	third := AddMenuItemFirst("Third program", "")
	defer third.Remove()
	check(first, third)
	for index, item := range []*MenuItem{open, quit} {
		if getID(index+3) != item.id {
			t.Fatal("footer moved ahead of collected items")
		}
	}
}
