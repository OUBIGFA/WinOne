//go:build windows

package systray

import (
	"testing"
	"golang.org/x/sys/windows"
)

func TestMenuWidthFollowsVisibleContent(t *testing.T) {
	root, _, _ := pCreatePopupMenu.Call()
	defer pDestroyMenu.Call(root)
	appendStyleFixture(t, windows.Handle(root), 1, "打开 WinOne", 0, 0, false)
	appendStyleFixture(t, windows.Handle(root), 2, "", 0, 0, true)
	appendStyleFixture(t, windows.Handle(root), 3, "A collected program with a substantially longer name", 0, 0, false)
	for _, dpi := range []uint32{96, 144, 192} {
		style, err := newNativeMenuStyle(windows.Handle(root), false, dpi)
		if err != nil { t.Fatal(err) }
		var widths []uint32
		for _, index := range style.menus[style.root] {
			measure := menuMeasure{Type: 1, Data: styleToken(style, index)}
			if !style.measure(&measure) { t.Fatal("measurement failed") }
			widths = append(widths, measure.Width)
		}
		if widths[0] >= uint32(style.px(224)) || widths[0] <= uint32(style.px(54)) || widths[1] != 0 || widths[2] <= widths[0] {
			t.Fatalf("DPI %d did not follow content: %v", dpi, widths)
		}
		style.dispose()
	}
}

func TestCollectedSeparatorHasNoEmptyPlaceholder(t *testing.T) {
	menu, _, err := pCreatePopupMenu.Call()
	if menu == 0 { t.Fatal(err) }
	wt.menus = map[uint32]windows.Handle{0: windows.Handle(menu)}
	wt.menuOf = make(map[uint32]windows.Handle)
	wt.visibleItems = make(map[uint32][]uint32)
	wt.frontItems = make(map[uint32]bool)
	wt.menuItemIcons = make(map[uint32]windows.Handle)
	wt.initialized.Store(true)
	defer func() { wt.initialized.Store(false); pDestroyMenu.Call(menu) }()
	separator := NewMenuSeparator()
	open, quit := AddMenuItem("打开 WinOne", ""), AddMenuItem("退出 WinOne", "")
	defer open.Remove(); defer quit.Remove()
	check := func(want ...uint32) {
		t.Helper()
		count, _, _ := menuGetCount.Call(menu)
		if int(count) != len(want) { t.Fatalf("menu has %d rows, want %d", count, len(want)) }
		for position, id := range want {
			entry, err := readStyledMenuEntry(windows.Handle(menu), uint32(position))
			if err != nil || entry.id != id { t.Fatalf("row %d: %+v %v", position, entry, err) }
		}
	}
	check(open.id, quit.id)
	for range 3 {
		program := AddMenuItemFirst("Collected program", "")
		if err := separator.SetVisible(true); err != nil { t.Fatal(err) }
		if err := separator.SetVisible(true); err != nil { t.Fatal(err) }
		check(program.id, separator.id, open.id, quit.id)
		entry, err := readStyledMenuEntry(windows.Handle(menu), 1)
		if err != nil || entry.kind&0x800 == 0 { t.Fatal("divider is not a native separator") }
		program.Remove()
		if err := separator.SetVisible(false); err != nil { t.Fatal(err) }
		check(open.id, quit.id)
	}
}
